package encode

// IDRSliceHeader collects the SPS/PPS-derived fields needed to write a
// well-formed IDR slice header against arbitrary parameter sets.
type IDRSliceHeader struct {
	QPDelta                     int32  // slice_qp_delta = sliceQP - (26 + pic_init_qp_minus26)
	DisableDeblock              int    // 0=enable, 1=disable, 2=disable across slices
	IDRPicID                    uint32 // idr_pic_id
	PPSID                       uint32 // pic_parameter_set_id (from PPS)
	Log2MaxFrameNumMinus4       uint32 // SPS
	Log2MaxPicOrderCntLsbMinus4 uint32 // SPS (ignored when PicOrderCntType != 0)
	PicOrderCntType             uint8  // 0 writes pic_order_cnt_lsb; 2 omits it
	DeblockControlPresent       bool   // PPS.DeblockingFilterControlPresentFlag
}

// WriteSliceHeader writes the slice header fields for an IDR I-slice using
// the supplied parameter-set-derived values. Does NOT include the NALU
// header byte (that's written separately).
func WriteSliceHeader(w *BitWriter, h IDRSliceHeader) {
	// first_mb_in_slice = 0
	w.WriteUE(0)
	// slice_type = 2 (I-slice, not 7 which is "all I")
	w.WriteUE(2)
	// pic_parameter_set_id
	w.WriteUE(h.PPSID)
	// frame_num = 0 (u(log2_max_frame_num_minus4+4))
	w.WriteBits(0, int(h.Log2MaxFrameNumMinus4+4))
	// idr_pic_id
	w.WriteUE(h.IDRPicID)
	// pic_order_cnt_lsb = 0 (only when pic_order_cnt_type == 0)
	if h.PicOrderCntType == 0 {
		w.WriteBits(0, int(h.Log2MaxPicOrderCntLsbMinus4+4))
	}
	// dec_ref_pic_marking: no_output_of_prior_pics_flag=0, long_term_reference_flag=0
	w.WriteBit(0)
	w.WriteBit(0)
	// slice_qp_delta
	w.WriteSE(h.QPDelta)
	// deblocking filter syntax (only when PPS sets deblocking_filter_control_present_flag=1)
	if h.DeblockControlPresent {
		w.WriteUE(uint32(h.DisableDeblock))
		if h.DisableDeblock != 1 {
			w.WriteSE(0) // slice_alpha_c0_offset_div2
			w.WriteSE(0) // slice_beta_offset_div2
		}
	}
}

// WritePSliceHeader writes the slice header for a P-slice (non-IDR).
// frameNum is the frame_num value (increments for each reference frame).
// picOrderCntLsb is the pic_order_cnt_lsb value (only written when
// picOrderCntType == 0, otherwise ignored), masked to
// log2MaxPicOrderCntLsbMinus4+4 bits.
// picOrderCntType selects the POC mode from the SPS: 0 writes the LSB,
// 2 omits it (POC is derived from frame_num by the decoder). Type 1 is
// not supported.
// weightedPredFlag (from PPS) signals that pred_weight_table() syntax must
// be written. We always emit a "no weights" table (denominators=0, all
// weight flags=0).
// log2MaxFrameNumMinus4 must match the SPS value.
// log2MaxPicOrderCntLsbMinus4 controls the POC LSB bit width (from SPS).
// ppsID is the pic_parameter_set_id written in the slice header.
// deblockControlPresent controls whether deblocking syntax is written (from PPS).
// cabacInitIDC: when >= 0, write cabac_init_idc (0-2) for CABAC slices; -1 for CAVLC.
func WritePSliceHeader(w *BitWriter, frameNum uint32, picOrderCntLsb uint32,
	picOrderCntType uint8, weightedPredFlag bool,
	qpDelta int32, disableDeblock int, log2MaxFrameNumMinus4 uint32,
	log2MaxPicOrderCntLsbMinus4 uint32, ppsID uint32, deblockControlPresent bool,
	cabacInitIDC int) {
	// first_mb_in_slice = 0
	w.WriteUE(0)
	// slice_type = 0 (P-slice)
	w.WriteUE(0)
	// pic_parameter_set_id
	w.WriteUE(ppsID)
	// frame_num: u(log2_max_frame_num_minus4 + 4)
	w.WriteBits(uint32(frameNum), int(log2MaxFrameNumMinus4+4))
	if picOrderCntType == 0 {
		// pic_order_cnt_lsb: u(log2_max_pic_order_cnt_lsb_minus4 + 4)
		w.WriteBits(picOrderCntLsb, int(log2MaxPicOrderCntLsbMinus4+4))
	}
	// num_ref_idx_active_override_flag: when weighted_pred_flag=1 we override
	// to 1 ref so pred_weight_table() needs only one entry (independent of
	// the PPS default num_ref_idx_l0_default_active_minus1).
	if weightedPredFlag {
		w.WriteBit(1) // num_ref_idx_active_override_flag = 1
		w.WriteUE(0)  // num_ref_idx_l0_active_minus1 = 0 (1 active ref)
	} else {
		w.WriteBit(0) // use PPS default
	}
	// ref_pic_list_modification_flag_l0 = 0 (no reordering)
	w.WriteBit(0)
	// pred_weight_table(): present when weighted_pred_flag=1 for P/SP slices.
	// Emit a zero-weights table for the single active ref.
	if weightedPredFlag {
		w.WriteUE(0)  // luma_log2_weight_denom
		w.WriteUE(0)  // chroma_log2_weight_denom (ChromaArrayType=1, 4:2:0)
		w.WriteBit(0) // luma_weight_l0_flag (ref 0)
		w.WriteBit(0) // chroma_weight_l0_flag (ref 0)
	}
	// dec_ref_pic_marking (non-IDR, nal_ref_idc != 0):
	// adaptive_ref_pic_marking_mode_flag = 0 (sliding window)
	w.WriteBit(0)
	// cabac_init_idc: present for non-I/SI slices when entropy_coding_mode_flag=1 (spec 7.3.3)
	if cabacInitIDC >= 0 {
		w.WriteUE(uint32(cabacInitIDC))
	}
	// slice_qp_delta
	w.WriteSE(qpDelta)
	// deblocking filter syntax (only if deblocking_filter_control_present_flag=1 in PPS)
	if deblockControlPresent {
		// disable_deblocking_filter_idc
		w.WriteUE(uint32(disableDeblock))
		if disableDeblock != 1 {
			// slice_alpha_c0_offset_div2 = 0
			w.WriteSE(0)
			// slice_beta_offset_div2 = 0
			w.WriteSE(0)
		}
	}
}

// WritePSliceHeaderCABAC writes a CABAC P-slice header with alignment bits.
// cabacInitIDC is the cabac_init_idc value (0-2).
// picOrderCntType: 0 writes pic_order_cnt_lsb, 2 omits it (POC derived from frame_num).
// weightedPredFlag: if true, emits a zero-weights pred_weight_table() (required when PPS sets weighted_pred_flag=1).
func WritePSliceHeaderCABAC(w *BitWriter, frameNum uint32, picOrderCntLsb uint32,
	picOrderCntType uint8, weightedPredFlag bool,
	qpDelta int32, disableDeblock int, log2MaxFrameNumMinus4 uint32,
	log2MaxPicOrderCntLsbMinus4 uint32, ppsID uint32, deblockControlPresent bool,
	cabacInitIDC int) {
	WritePSliceHeader(w, frameNum, picOrderCntLsb, picOrderCntType, weightedPredFlag,
		qpDelta, disableDeblock,
		log2MaxFrameNumMinus4, log2MaxPicOrderCntLsbMinus4,
		ppsID, deblockControlPresent, cabacInitIDC)

	// CABAC alignment (spec 7.3.2.8/7.3.4): while not byte-aligned, write
	// cabac_alignment_one_bit (= 1). If the header already ends on a byte
	// boundary, no alignment bits are emitted.
	for w.BitsWritten()%8 != 0 {
		w.WriteBit(1)
	}
}

// WriteSliceHeaderCABAC writes the slice header for a CABAC-encoded IDR I-slice.
// After the header fields, it appends CABAC alignment bits (1-bits until byte-aligned)
// as required by section 7.3.2.1.
// Note: For I-slices, cabac_init_idc is NOT present (spec: only for non-I/SI slices),
// so the header fields are identical to CAVLC except for the byte-alignment suffix.
func WriteSliceHeaderCABAC(w *BitWriter, h IDRSliceHeader) {
	WriteSliceHeader(w, h)

	// CABAC alignment (spec 7.3.2.8/7.3.4): while not byte-aligned, write
	// cabac_alignment_one_bit (= 1). If the header already ends on a byte
	// boundary, no alignment bits are emitted.
	for w.BitsWritten()%8 != 0 {
		w.WriteBit(1)
	}
}
