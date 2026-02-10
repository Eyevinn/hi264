// Package cavlc implements CAVLC (Context-Adaptive Variable-Length Coding)
// entropy decoding for H.264/AVC.
package cavlc

import "fmt"

// BitReader reads individual bits from a byte slice.
type BitReader struct {
	data    []byte
	bytePos int
	bitPos  uint // 0-7, counts from MSB (0 = MSB)
}

// NewBitReader creates a new BitReader.
func NewBitReader(data []byte) *BitReader {
	return &BitReader{data: data}
}

// BitsRead returns the total number of bits read so far.
func (r *BitReader) BitsRead() int {
	return r.bytePos*8 + int(r.bitPos)
}

// ReadBit reads a single bit.
func (r *BitReader) ReadBit() (uint8, error) {
	if r.bytePos >= len(r.data) {
		return 0, fmt.Errorf("bitreader: end of data at byte %d", r.bytePos)
	}
	bit := (r.data[r.bytePos] >> (7 - r.bitPos)) & 1
	r.bitPos++
	if r.bitPos == 8 {
		r.bitPos = 0
		r.bytePos++
	}
	return bit, nil
}

// ReadBits reads n bits and returns them as a uint32.
func (r *BitReader) ReadBits(n int) (uint32, error) {
	if n == 0 {
		return 0, nil
	}
	if n > 32 || n < 0 {
		return 0, fmt.Errorf("bitreader: invalid n=%d", n)
	}
	var val uint32
	for i := 0; i < n; i++ {
		bit, err := r.ReadBit()
		if err != nil {
			return 0, err
		}
		val = (val << 1) | uint32(bit)
	}
	return val, nil
}

// ReadFlag reads a single bit as a boolean.
func (r *BitReader) ReadFlag() (bool, error) {
	bit, err := r.ReadBit()
	return bit == 1, err
}

// PeekBits reads n bits without advancing the position.
// If fewer than n bits remain, the available bits are left-shifted (padded with zeros on the right).
func (r *BitReader) PeekBits(n int) (uint32, error) {
	savedBytePos := r.bytePos
	savedBitPos := r.bitPos
	var val uint32
	bitsRead := 0
	for i := 0; i < n; i++ {
		bit, err := r.ReadBit()
		if err != nil {
			// Pad remaining bits with zeros
			val <<= uint(n - bitsRead)
			break
		}
		val = (val << 1) | uint32(bit)
		bitsRead++
	}
	r.bytePos = savedBytePos
	r.bitPos = savedBitPos
	return val, nil
}

// SkipBits advances the position by n bits.
func (r *BitReader) SkipBits(n int) {
	totalBit := int(r.bitPos) + n
	r.bytePos += totalBit / 8
	r.bitPos = uint(totalBit % 8)
}

// ReadUE reads an unsigned Exp-Golomb coded value (ue(v)).
func (r *BitReader) ReadUE() (uint32, error) {
	leadingZeros := 0
	for {
		bit, err := r.ReadBit()
		if err != nil {
			return 0, fmt.Errorf("ue(v): %w", err)
		}
		if bit == 1 {
			break
		}
		leadingZeros++
	}
	if leadingZeros == 0 {
		return 0, nil
	}
	suffix, err := r.ReadBits(leadingZeros)
	if err != nil {
		return 0, fmt.Errorf("ue(v) suffix: %w", err)
	}
	return (1 << uint(leadingZeros)) - 1 + suffix, nil
}

// ReadSE reads a signed Exp-Golomb coded value (se(v)).
func (r *BitReader) ReadSE() (int32, error) {
	ue, err := r.ReadUE()
	if err != nil {
		return 0, err
	}
	if ue%2 == 0 {
		return -int32(ue / 2), nil
	}
	return int32((ue + 1) / 2), nil
}

// AlignToByte advances to the next byte boundary.
func (r *BitReader) AlignToByte() {
	if r.bitPos > 0 {
		r.bitPos = 0
		r.bytePos++
	}
}

// SliceHeaderParams holds parameters needed to skip the IDR slice header.
type SliceHeaderParams struct {
	FrameMbsOnly                          bool
	Log2MaxFrameNumMinus4                 uint
	PicOrderCntType                       uint
	Log2MaxPicOrderCntLsbMinus4           uint
	BottomFieldPicOrderInFramePresentFlag bool
	DeblockingFilterControlPresent        bool
	RedundantPicCntPresentFlag            bool
	NumSliceGroupsMinus1                  uint
}

// SkipSliceHeaderIDR advances the reader past an IDR I-slice header to the
// start of slice data. The NALU data (starting with the NAL header byte) must
// be at position 0 in the reader.
func (r *BitReader) SkipSliceHeaderIDR(p SliceHeaderParams) error {
	// 1. NAL header (8 bits)
	_, err := r.ReadBits(8)
	if err != nil {
		return fmt.Errorf("nal header: %w", err)
	}

	// 2. first_mb_in_slice: ue(v)
	_, err = r.ReadUE()
	if err != nil {
		return fmt.Errorf("first_mb_in_slice: %w", err)
	}

	// 3. slice_type: ue(v)
	_, err = r.ReadUE()
	if err != nil {
		return fmt.Errorf("slice_type: %w", err)
	}

	// 4. pic_parameter_set_id: ue(v)
	_, err = r.ReadUE()
	if err != nil {
		return fmt.Errorf("pps_id: %w", err)
	}

	// 5. frame_num: u(log2_max_frame_num_minus4+4)
	_, err = r.ReadBits(int(p.Log2MaxFrameNumMinus4 + 4))
	if err != nil {
		return fmt.Errorf("frame_num: %w", err)
	}

	// 6. field_pic_flag / bottom_field_flag (only if !frame_mbs_only_flag)
	fieldPicFlag := false
	if !p.FrameMbsOnly {
		fieldPicFlag, err = r.ReadFlag()
		if err != nil {
			return fmt.Errorf("field_pic_flag: %w", err)
		}
		if fieldPicFlag {
			_, err = r.ReadBit() // bottom_field_flag
			if err != nil {
				return fmt.Errorf("bottom_field_flag: %w", err)
			}
		}
	}

	// 7. idr_pic_id: ue(v)
	_, err = r.ReadUE()
	if err != nil {
		return fmt.Errorf("idr_pic_id: %w", err)
	}

	// 8. pic_order_cnt (depends on PicOrderCntType)
	if p.PicOrderCntType == 0 {
		_, err = r.ReadBits(int(p.Log2MaxPicOrderCntLsbMinus4 + 4))
		if err != nil {
			return fmt.Errorf("pic_order_cnt_lsb: %w", err)
		}
		if p.BottomFieldPicOrderInFramePresentFlag && !fieldPicFlag {
			_, err = r.ReadSE() // delta_pic_order_cnt_bottom
			if err != nil {
				return fmt.Errorf("delta_pic_order_cnt_bottom: %w", err)
			}
		}
	}

	// 9. redundant_pic_cnt: ue(v) (if flag set)
	if p.RedundantPicCntPresentFlag {
		_, err = r.ReadUE()
		if err != nil {
			return fmt.Errorf("redundant_pic_cnt: %w", err)
		}
	}

	// 10. dec_ref_pic_marking for IDR (nal_ref_idc != 0):
	//     no_output_of_prior_pics_flag: u(1)
	//     long_term_reference_flag: u(1)
	_, err = r.ReadBit()
	if err != nil {
		return fmt.Errorf("no_output_of_prior_pics_flag: %w", err)
	}
	_, err = r.ReadBit()
	if err != nil {
		return fmt.Errorf("long_term_reference_flag: %w", err)
	}

	// 11. cabac_init_idc: NOT present for I-slices (spec 7.3.3)

	// 12. slice_qp_delta: se(v)
	_, err = r.ReadSE()
	if err != nil {
		return fmt.Errorf("slice_qp_delta: %w", err)
	}

	// 13. deblocking_filter_control
	if p.DeblockingFilterControlPresent {
		disableDeblockingFilterIdc, err := r.ReadUE()
		if err != nil {
			return fmt.Errorf("disable_deblocking_filter_idc: %w", err)
		}
		if disableDeblockingFilterIdc != 1 {
			_, err = r.ReadSE() // slice_alpha_c0_offset_div2
			if err != nil {
				return fmt.Errorf("slice_alpha: %w", err)
			}
			_, err = r.ReadSE() // slice_beta_offset_div2
			if err != nil {
				return fmt.Errorf("slice_beta: %w", err)
			}
		}
	}

	return nil
}

// SkipSliceHeaderP advances the reader past a non-IDR P-slice header to the
// start of slice data. The NALU data (starting with the NAL header byte) must
// be at position 0 in the reader. nalRefIdc indicates whether this is a reference
// picture (non-zero nal_ref_idc).
func (r *BitReader) SkipSliceHeaderP(p SliceHeaderParams, nalRefIdc uint8) error {
	// 1. NAL header (8 bits)
	_, err := r.ReadBits(8)
	if err != nil {
		return fmt.Errorf("nal header: %w", err)
	}

	// 2. first_mb_in_slice: ue(v)
	_, err = r.ReadUE()
	if err != nil {
		return fmt.Errorf("first_mb_in_slice: %w", err)
	}

	// 3. slice_type: ue(v)
	_, err = r.ReadUE()
	if err != nil {
		return fmt.Errorf("slice_type: %w", err)
	}

	// 4. pic_parameter_set_id: ue(v)
	_, err = r.ReadUE()
	if err != nil {
		return fmt.Errorf("pps_id: %w", err)
	}

	// 5. frame_num: u(log2_max_frame_num_minus4+4)
	_, err = r.ReadBits(int(p.Log2MaxFrameNumMinus4 + 4))
	if err != nil {
		return fmt.Errorf("frame_num: %w", err)
	}

	// 6. field_pic_flag / bottom_field_flag (only if !frame_mbs_only_flag)
	fieldPicFlag := false
	if !p.FrameMbsOnly {
		fieldPicFlag, err = r.ReadFlag()
		if err != nil {
			return fmt.Errorf("field_pic_flag: %w", err)
		}
		if fieldPicFlag {
			_, err = r.ReadBit() // bottom_field_flag
			if err != nil {
				return fmt.Errorf("bottom_field_flag: %w", err)
			}
		}
	}

	// 7. NO idr_pic_id for non-IDR slices

	// 8. pic_order_cnt (depends on PicOrderCntType)
	if p.PicOrderCntType == 0 {
		_, err = r.ReadBits(int(p.Log2MaxPicOrderCntLsbMinus4 + 4))
		if err != nil {
			return fmt.Errorf("pic_order_cnt_lsb: %w", err)
		}
		if p.BottomFieldPicOrderInFramePresentFlag && !fieldPicFlag {
			_, err = r.ReadSE() // delta_pic_order_cnt_bottom
			if err != nil {
				return fmt.Errorf("delta_pic_order_cnt_bottom: %w", err)
			}
		}
	}

	// 9. redundant_pic_cnt: ue(v) (if flag set)
	if p.RedundantPicCntPresentFlag {
		_, err = r.ReadUE()
		if err != nil {
			return fmt.Errorf("redundant_pic_cnt: %w", err)
		}
	}

	// 10. num_ref_idx_active_override_flag
	overrideFlag, err := r.ReadFlag()
	if err != nil {
		return fmt.Errorf("num_ref_idx_active_override_flag: %w", err)
	}
	if overrideFlag {
		_, err = r.ReadUE() // num_ref_idx_l0_active_minus1
		if err != nil {
			return fmt.Errorf("num_ref_idx_l0_active_minus1: %w", err)
		}
	}

	// 11. ref_pic_list_modification (for P-slices)
	rplmFlag, err := r.ReadFlag()
	if err != nil {
		return fmt.Errorf("ref_pic_list_modification_flag_l0: %w", err)
	}
	if rplmFlag {
		for {
			op, err := r.ReadUE()
			if err != nil {
				return fmt.Errorf("modification_of_pic_nums_idc: %w", err)
			}
			if op == 3 {
				break
			}
			_, err = r.ReadUE() // abs_diff_pic_num_minus1 or long_term_pic_num
			if err != nil {
				return fmt.Errorf("rplm operand: %w", err)
			}
		}
	}

	// 12. dec_ref_pic_marking (non-IDR)
	if nalRefIdc != 0 {
		adaptiveFlag, err := r.ReadFlag()
		if err != nil {
			return fmt.Errorf("adaptive_ref_pic_marking_mode_flag: %w", err)
		}
		if adaptiveFlag {
			for {
				op, err := r.ReadUE()
				if err != nil {
					return fmt.Errorf("mmco op: %w", err)
				}
				if op == 0 {
					break
				}
				// Each MMCO op has 1-2 operands
				_, err = r.ReadUE()
				if err != nil {
					return fmt.Errorf("mmco operand: %w", err)
				}
				if op == 3 {
					_, err = r.ReadUE()
					if err != nil {
						return fmt.Errorf("mmco op3 operand: %w", err)
					}
				}
			}
		}
	}

	// 13. slice_qp_delta: se(v)
	_, err = r.ReadSE()
	if err != nil {
		return fmt.Errorf("slice_qp_delta: %w", err)
	}

	// 14. deblocking_filter_control
	if p.DeblockingFilterControlPresent {
		disableDeblockingFilterIdc, err := r.ReadUE()
		if err != nil {
			return fmt.Errorf("disable_deblocking_filter_idc: %w", err)
		}
		if disableDeblockingFilterIdc != 1 {
			_, err = r.ReadSE() // slice_alpha_c0_offset_div2
			if err != nil {
				return fmt.Errorf("slice_alpha: %w", err)
			}
			_, err = r.ReadSE() // slice_beta_offset_div2
			if err != nil {
				return fmt.Errorf("slice_beta: %w", err)
			}
		}
	}

	return nil
}
