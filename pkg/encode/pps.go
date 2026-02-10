package encode

// EncodePPS generates a minimal PPS RBSP for CAVLC encoding.
// disableDeblock: 0=enable (default), 1=disable, 2=disable across slices.
func EncodePPS(disableDeblock int) []byte {
	w := NewBitWriter()

	// pic_parameter_set_id = 0
	w.WriteUE(0)
	// seq_parameter_set_id = 0
	w.WriteUE(0)
	// entropy_coding_mode_flag = 0 (CAVLC)
	w.WriteBit(0)
	// bottom_field_pic_order_in_frame_present_flag = 0
	w.WriteBit(0)
	// num_slice_groups_minus1 = 0
	w.WriteUE(0)
	// num_ref_idx_l0_default_active_minus1 = 0
	w.WriteUE(0)
	// num_ref_idx_l1_default_active_minus1 = 0
	w.WriteUE(0)
	// weighted_pred_flag = 0
	w.WriteBit(0)
	// weighted_bipred_idc = 0
	w.WriteBits(0, 2)
	// pic_init_qp_minus26 = 0
	w.WriteSE(0)
	// pic_init_qs_minus26 = 0
	w.WriteSE(0)
	// chroma_qp_index_offset = 0
	w.WriteSE(0)
	// deblocking_filter_control_present_flag = 1
	w.WriteBit(1)
	// constrained_intra_pred_flag = 0
	w.WriteBit(0)
	// redundant_pic_cnt_present_flag = 0
	w.WriteBit(0)

	// RBSP trailing bits
	w.WriteBit(1) // rbsp_stop_one_bit
	w.AlignToByte()

	_ = disableDeblock // deblock control is in slice header, not PPS

	return w.Bytes()
}

// EncodePPSCABAC generates a minimal PPS RBSP for CABAC encoding.
// entropy_coding_mode_flag = 1 (CABAC).
func EncodePPSCABAC(disableDeblock int) []byte {
	w := NewBitWriter()

	// pic_parameter_set_id = 0
	w.WriteUE(0)
	// seq_parameter_set_id = 0
	w.WriteUE(0)
	// entropy_coding_mode_flag = 1 (CABAC)
	w.WriteBit(1)
	// bottom_field_pic_order_in_frame_present_flag = 0
	w.WriteBit(0)
	// num_slice_groups_minus1 = 0
	w.WriteUE(0)
	// num_ref_idx_l0_default_active_minus1 = 0
	w.WriteUE(0)
	// num_ref_idx_l1_default_active_minus1 = 0
	w.WriteUE(0)
	// weighted_pred_flag = 0
	w.WriteBit(0)
	// weighted_bipred_idc = 0
	w.WriteBits(0, 2)
	// pic_init_qp_minus26 = 0
	w.WriteSE(0)
	// pic_init_qs_minus26 = 0
	w.WriteSE(0)
	// chroma_qp_index_offset = 0
	w.WriteSE(0)
	// deblocking_filter_control_present_flag = 1
	w.WriteBit(1)
	// constrained_intra_pred_flag = 0
	w.WriteBit(0)
	// redundant_pic_cnt_present_flag = 0
	w.WriteBit(0)

	// RBSP trailing bits
	w.WriteBit(1) // rbsp_stop_one_bit
	w.AlignToByte()

	_ = disableDeblock // deblock control is in slice header, not PPS

	return w.Bytes()
}
