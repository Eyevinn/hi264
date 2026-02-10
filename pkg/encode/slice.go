package encode

// WriteSliceHeader writes the slice header fields for an IDR I-slice to the given BitWriter.
// qpDelta: slice_qp_delta (se(v)), offset from pic_init_qp.
// disableDeblock: 0=enable (with offsets=0), 1=disable, 2=disable across slices.
// idrPicID: idr_pic_id (ue(v)), used to distinguish consecutive IDR pictures.
// Does NOT include the NALU header byte (that's written separately).
func WriteSliceHeader(w *BitWriter, qpDelta int32, disableDeblock int, idrPicID uint32) {
	// first_mb_in_slice = 0
	w.WriteUE(0)
	// slice_type = 2 (I-slice, not 7 which is "all I")
	w.WriteUE(2)
	// pic_parameter_set_id = 0
	w.WriteUE(0)
	// frame_num = 0 (u(log2_max_frame_num_minus4+4) = u(4))
	w.WriteBits(0, 4)
	// idr_pic_id
	w.WriteUE(idrPicID)
	// pic_order_cnt_lsb = 0 (u(log2_max_pic_order_cnt_lsb_minus4+4) = u(4))
	w.WriteBits(0, 4)
	// dec_ref_pic_marking: no_output_of_prior_pics_flag=0, long_term_reference_flag=0
	w.WriteBit(0)
	w.WriteBit(0)
	// slice_qp_delta
	w.WriteSE(qpDelta)
	// deblocking_filter_control_present_flag=1 in PPS, so:
	// disable_deblocking_filter_idc
	w.WriteUE(uint32(disableDeblock))
	if disableDeblock != 1 {
		// slice_alpha_c0_offset_div2 = 0
		w.WriteSE(0)
		// slice_beta_offset_div2 = 0
		w.WriteSE(0)
	}
}

// WritePSliceHeader writes the slice header for a P-slice (non-IDR).
// frameNum is the frame_num value (increments for each non-IDR frame).
// log2MaxFrameNumMinus4 must match the SPS value.
// log2MaxPicOrderCntLsbMinus4 controls the POC LSB bit width (from SPS).
// ppsID is the pic_parameter_set_id written in the slice header.
// deblockControlPresent controls whether deblocking syntax is written (from PPS).
// cabacInitIDC: when >= 0, write cabac_init_idc (0-2) for CABAC slices; -1 for CAVLC.
func WritePSliceHeader(w *BitWriter, frameNum uint32, qpDelta int32,
	disableDeblock int, log2MaxFrameNumMinus4 uint32,
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
	// pic_order_cnt_lsb: u(log2_max_pic_order_cnt_lsb_minus4 + 4)
	// Use frameNum*2 as POC (simple mapping)
	w.WriteBits(frameNum*2, int(log2MaxPicOrderCntLsbMinus4+4))
	// num_ref_idx_active_override_flag = 0 (use PPS default)
	w.WriteBit(0)
	// ref_pic_list_modification_flag_l0 = 0 (no reordering)
	w.WriteBit(0)
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
func WritePSliceHeaderCABAC(w *BitWriter, frameNum uint32, qpDelta int32,
	disableDeblock int, log2MaxFrameNumMinus4 uint32,
	log2MaxPicOrderCntLsbMinus4 uint32, ppsID uint32, deblockControlPresent bool,
	cabacInitIDC int) {
	WritePSliceHeader(w, frameNum, qpDelta, disableDeblock,
		log2MaxFrameNumMinus4, log2MaxPicOrderCntLsbMinus4,
		ppsID, deblockControlPresent, cabacInitIDC)

	// CABAC alignment: write 1 followed by zero bits to reach byte boundary (section 7.4.3)
	w.WriteBit(1)
	for w.BitsWritten()%8 != 0 {
		w.WriteBit(0)
	}
}

// WriteSliceHeaderCABAC writes the slice header for a CABAC-encoded IDR I-slice.
// After the header fields, it appends CABAC alignment bits (1-bits until byte-aligned)
// as required by section 7.3.2.1.
// idrPicID: idr_pic_id (ue(v)), used to distinguish consecutive IDR pictures.
// Note: For I-slices, cabac_init_idc is NOT present (spec: only for non-I/SI slices),
// so the header fields are identical to CAVLC except for the byte-alignment suffix.
func WriteSliceHeaderCABAC(w *BitWriter, qpDelta int32, disableDeblock int, idrPicID uint32) {
	WriteSliceHeader(w, qpDelta, disableDeblock, idrPicID)

	// CABAC alignment: write 1 followed by zero bits to reach byte boundary (section 7.4.3)
	w.WriteBit(1)
	for w.BitsWritten()%8 != 0 {
		w.WriteBit(0)
	}
}
