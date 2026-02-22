package encode

import (
	"github.com/Eyevinn/hi264/pkg/yuv"
)

// EncodeSPS generates a minimal SPS RBSP for Baseline profile.
// width and height are in pixels (must be even; non-16-multiples use frame cropping).
// maxRef is the max_num_ref_frames value (0 for IDR-only, 1+ for P-frames).
// level is the level_idc value (e.g. 30 for Level 3.0); use ChooseLevel to compute.
func EncodeSPS(width, height, maxRef, level int, cs yuv.ColorSpace, rng yuv.Range) []byte {
	w := NewBitWriter()

	mbWidth := (width + 15) / 16
	mbHeight := (height + 15) / 16

	// profile_idc = 66 (Baseline)
	w.WriteBits(66, 8)
	// constraint_set0..5_flags + reserved_zero_2bits = 0xC0
	// constraint_set0=1, constraint_set1=1 (Baseline compatible)
	w.WriteBits(0xC0, 8)
	// level_idc
	w.WriteBits(uint32(level), 8)
	// seq_parameter_set_id = 0
	w.WriteUE(0)
	// log2_max_frame_num_minus4 = 0
	w.WriteUE(0)
	// pic_order_cnt_type = 0
	w.WriteUE(0)
	// log2_max_pic_order_cnt_lsb_minus4 = 0
	w.WriteUE(0)
	// max_num_ref_frames
	w.WriteUE(uint32(maxRef))
	// gaps_in_frame_num_value_allowed_flag = 0
	w.WriteBit(0)
	// pic_width_in_mbs_minus1
	w.WriteUE(uint32(mbWidth - 1))
	// pic_height_in_map_units_minus1
	w.WriteUE(uint32(mbHeight - 1))
	// frame_mbs_only_flag = 1
	w.WriteBit(1)
	// mb_adaptive_frame_field_flag: only present when frame_mbs_only_flag=0, skip
	// direct_8x8_inference_flag = 0
	w.WriteBit(0)
	// frame_cropping_flag
	codedWidth := mbWidth * 16
	codedHeight := mbHeight * 16
	if codedWidth != width || codedHeight != height {
		w.WriteBit(1)                                 // frame_cropping_flag = 1
		w.WriteUE(0)                                  // frame_crop_left_offset
		w.WriteUE(uint32((codedWidth - width) / 2))   // frame_crop_right_offset (CropUnitX=2 for 4:2:0)
		w.WriteUE(0)                                  // frame_crop_top_offset
		w.WriteUE(uint32((codedHeight - height) / 2)) // frame_crop_bottom_offset (CropUnitY=2)
	} else {
		w.WriteBit(0) // frame_cropping_flag = 0
	}

	// vui_parameters_present_flag
	writeVUI(w, cs, rng)

	// RBSP trailing bits
	w.WriteBit(1) // rbsp_stop_one_bit
	w.AlignToByte()

	return w.Bytes()
}

// EncodeSPSMain generates a minimal SPS RBSP for Main profile (profile_idc=77).
// Main profile is required for CABAC entropy coding.
// width and height are in pixels (must be even; non-16-multiples use frame cropping).
// maxRef is the max_num_ref_frames value (0 for IDR-only, 1+ for P-frames).
// level is the level_idc value (e.g. 31 for Level 3.1); use ChooseLevel to compute.
func EncodeSPSMain(width, height, maxRef, level int, cs yuv.ColorSpace, rng yuv.Range) []byte {
	w := NewBitWriter()

	mbWidth := (width + 15) / 16
	mbHeight := (height + 15) / 16

	// profile_idc = 77 (Main)
	w.WriteBits(77, 8)
	// constraint_set0..5_flags + reserved_zero_2bits
	// constraint_set1=1 (Main compatible)
	w.WriteBits(0x40, 8)
	// level_idc
	w.WriteBits(uint32(level), 8)
	// seq_parameter_set_id = 0
	w.WriteUE(0)
	// log2_max_frame_num_minus4 = 0
	w.WriteUE(0)
	// pic_order_cnt_type = 0
	w.WriteUE(0)
	// log2_max_pic_order_cnt_lsb_minus4 = 0
	w.WriteUE(0)
	// max_num_ref_frames
	w.WriteUE(uint32(maxRef))
	// gaps_in_frame_num_value_allowed_flag = 0
	w.WriteBit(0)
	// pic_width_in_mbs_minus1
	w.WriteUE(uint32(mbWidth - 1))
	// pic_height_in_map_units_minus1
	w.WriteUE(uint32(mbHeight - 1))
	// frame_mbs_only_flag = 1
	w.WriteBit(1)
	// direct_8x8_inference_flag = 1 (required by Main profile level >= 3.0)
	w.WriteBit(1)
	// frame_cropping_flag
	codedWidth := mbWidth * 16
	codedHeight := mbHeight * 16
	if codedWidth != width || codedHeight != height {
		w.WriteBit(1)                                 // frame_cropping_flag = 1
		w.WriteUE(0)                                  // frame_crop_left_offset
		w.WriteUE(uint32((codedWidth - width) / 2))   // frame_crop_right_offset (CropUnitX=2 for 4:2:0)
		w.WriteUE(0)                                  // frame_crop_top_offset
		w.WriteUE(uint32((codedHeight - height) / 2)) // frame_crop_bottom_offset (CropUnitY=2)
	} else {
		w.WriteBit(0) // frame_cropping_flag = 0
	}

	// vui_parameters_present_flag
	writeVUI(w, cs, rng)

	// RBSP trailing bits
	w.WriteBit(1) // rbsp_stop_one_bit
	w.AlignToByte()

	return w.Bytes()
}

// writeVUI writes VUI parameters to the SPS bitstream.
// When cs is BT601 and rng is LimitedRange (the defaults), no VUI is written
// to maintain backward compatibility.
func writeVUI(w *BitWriter, cs yuv.ColorSpace, rng yuv.Range) {
	if cs == yuv.BT601 && rng == yuv.LimitedRange {
		w.WriteBit(0) // vui_parameters_present_flag = 0
		return
	}

	w.WriteBit(1) // vui_parameters_present_flag = 1

	// aspect_ratio_info_present_flag = 0
	w.WriteBit(0)

	// overscan_info_present_flag = 0
	w.WriteBit(0)

	// video_signal_type_present_flag = 1
	w.WriteBit(1)
	// video_format = 5 (unspecified)
	w.WriteBits(5, 3)
	// video_full_range_flag
	if rng == yuv.FullRange {
		w.WriteBit(1)
	} else {
		w.WriteBit(0)
	}
	// colour_description_present_flag = 1
	w.WriteBit(1)
	// colour_primaries
	w.WriteBits(uint32(cs.ColourPrimaries()), 8)
	// transfer_characteristics
	w.WriteBits(uint32(cs.TransferCharacteristics()), 8)
	// matrix_coefficients
	w.WriteBits(uint32(cs.MatrixCoefficients()), 8)

	// chroma_loc_info_present_flag = 0
	w.WriteBit(0)

	// timing_info_present_flag = 0
	w.WriteBit(0)

	// nal_hrd_parameters_present_flag = 0
	w.WriteBit(0)

	// vcl_hrd_parameters_present_flag = 0
	w.WriteBit(0)

	// Since neither nal_hrd nor vcl_hrd present, no low_delay_hrd_flag

	// pic_struct_present_flag = 0
	w.WriteBit(0)

	// bitstream_restriction_flag = 0
	w.WriteBit(0)
}
