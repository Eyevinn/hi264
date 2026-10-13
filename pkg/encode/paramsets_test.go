package encode

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"

	"github.com/Eyevinn/hi264/pkg/yuv"
)

// A stream may carry several SPSs and PPSs. Only one SPS is active in a coded video sequence, and
// each picture uses a PPS that refers to it, so a continuation must use the parameter sets of the
// stream's last slice rather than the last ones that appear in the stream. These tests use a stream
// whose slices use SPS 1 and PPS 1 but that ends its parameter sets with SPS 0 and PPS 0. SPS 1
// numbers frames modulo 256 and SPS 0 modulo 16, so slices written with the wrong SPS show up as
// an early wrap of frame_num, besides carrying the wrong pic_parameter_set_id.

const multiPSSize = 32 // width and height of the test pictures

// spsNALU returns a Baseline SPS NAL unit for a multiPSSize square picture. log2MaxMinus4 sets
// both log2_max_frame_num_minus4 and, for pic_order_cnt_type 0, log2_max_pic_order_cnt_lsb_minus4.
func spsNALU(id, log2MaxMinus4, pocType uint32) []byte {
	w := NewBitWriter()
	w.WriteBits(66, 8)   // profile_idc: Baseline
	w.WriteBits(0xC0, 8) // constraint_set0_flag and constraint_set1_flag
	w.WriteBits(30, 8)   // level_idc
	w.WriteUE(id)        // seq_parameter_set_id
	w.WriteUE(log2MaxMinus4)
	w.WriteUE(pocType)
	switch pocType {
	case 0:
		w.WriteUE(log2MaxMinus4) // log2_max_pic_order_cnt_lsb_minus4
	case 1:
		w.WriteBit(1) // delta_pic_order_always_zero_flag
		w.WriteSE(0)  // offset_for_non_ref_pic
		w.WriteSE(0)  // offset_for_top_to_bottom_field
		w.WriteUE(0)  // num_ref_frames_in_pic_order_cnt_cycle
	}
	w.WriteUE(1)                  // max_num_ref_frames
	w.WriteBit(0)                 // gaps_in_frame_num_value_allowed_flag
	w.WriteUE(multiPSSize/16 - 1) // pic_width_in_mbs_minus1
	w.WriteUE(multiPSSize/16 - 1) // pic_height_in_map_units_minus1
	w.WriteBit(1)                 // frame_mbs_only_flag
	w.WriteBit(0)                 // direct_8x8_inference_flag
	w.WriteBit(0)                 // frame_cropping_flag
	w.WriteBit(0)                 // vui_parameters_present_flag
	w.WriteBit(1)                 // rbsp_stop_one_bit
	w.AlignToByte()
	return BuildNALU(byte(avc.NALU_SPS), 3, w.Bytes())
}

// ppsNALU returns a CAVLC PPS NAL unit that refers to SPS spsID.
func ppsNALU(id, spsID uint32) []byte {
	w := NewBitWriter()
	w.WriteUE(id)     // pic_parameter_set_id
	w.WriteUE(spsID)  // seq_parameter_set_id
	w.WriteBit(0)     // entropy_coding_mode_flag
	w.WriteBit(0)     // bottom_field_pic_order_in_frame_present_flag
	w.WriteUE(0)      // num_slice_groups_minus1
	w.WriteUE(0)      // num_ref_idx_l0_default_active_minus1
	w.WriteUE(0)      // num_ref_idx_l1_default_active_minus1
	w.WriteBit(0)     // weighted_pred_flag
	w.WriteBits(0, 2) // weighted_bipred_idc
	w.WriteSE(0)      // pic_init_qp_minus26
	w.WriteSE(0)      // pic_init_qs_minus26
	w.WriteSE(0)      // chroma_qp_index_offset
	w.WriteBit(1)     // deblocking_filter_control_present_flag
	w.WriteBit(0)     // constrained_intra_pred_flag
	w.WriteBit(0)     // redundant_pic_cnt_present_flag
	w.WriteBit(1)     // rbsp_stop_one_bit
	w.AlignToByte()
	return BuildNALU(byte(avc.NALU_PPS), 3, w.Bytes())
}

// paramSets holds parameter-set NAL units and their parsed forms.
type paramSets struct {
	spsNALUs, ppsNALUs [][]byte
	sps                map[uint32]*avc.SPS
	pps                map[uint32]*avc.PPS
}

// addSPS and addPPS add a parameter-set NAL unit, in stream order.
func (p *paramSets) addSPS(t *testing.T, nalu []byte) {
	t.Helper()
	sps, err := avc.ParseSPSNALUnit(nalu, true)
	if err != nil {
		t.Fatalf("ParseSPSNALUnit: %v", err)
	}
	if p.sps == nil {
		p.sps, p.pps = map[uint32]*avc.SPS{}, map[uint32]*avc.PPS{}
	}
	p.sps[sps.ParameterID] = sps
	p.spsNALUs = append(p.spsNALUs, nalu)
}

func (p *paramSets) addPPS(t *testing.T, nalu []byte) {
	t.Helper()
	pps, err := avc.ParsePPSNALUnit(nalu, p.sps)
	if err != nil {
		t.Fatalf("ParsePPSNALUnit: %v", err)
	}
	p.pps[pps.PicParameterSetID] = pps
	p.ppsNALUs = append(p.ppsNALUs, nalu)
}

// annexB returns the SPSs and then the PPSs as an Annex-B stream.
func (p *paramSets) annexB() []byte {
	var buf bytes.Buffer
	for _, nalu := range append(append([][]byte{}, p.spsNALUs...), p.ppsNALUs...) {
		buf.Write([]byte{0, 0, 0, 1})
		buf.Write(nalu)
	}
	return buf.Bytes()
}

// twoSPSStream returns the parameter sets SPS 1, SPS 0, PPS 1 (on SPS 1) and PPS 0 (on SPS 0),
// and a stream of them followed by a grey IDR that uses PPS 1. sps0POCType sets
// pic_order_cnt_type of SPS 0.
func twoSPSStream(t *testing.T, sps0POCType uint32) (*paramSets, []byte) {
	t.Helper()
	ps := &paramSets{}
	ps.addSPS(t, spsNALU(1, 4, 0))
	ps.addSPS(t, spsNALU(0, 0, sps0POCType))
	ps.addPPS(t, ppsNALU(1, 1))
	ps.addPPS(t, ppsNALU(0, 0))

	grid, colors := yuv.SolidGrid(multiPSSize, multiPSSize, yuv.Color{Y: 128, Cb: 128, Cr: 128})
	plane, err := yuv.GridToPlaneGrid(grid, colors)
	if err != nil {
		t.Fatalf("GridToPlaneGrid: %v", err)
	}
	idr, err := GenerateIDRWithSPSPPS(EncodeParams{Width: multiPSSize, Height: multiPSSize, QP: 26},
		ps.sps[1], ps.pps[1], plane, 0)
	if err != nil {
		t.Fatalf("GenerateIDRWithSPSPPS: %v", err)
	}
	return ps, append(ps.annexB(), idr...)
}

// requireSliceHeaders checks that stream has wantSlices slices, which use PPS 1 and number frames
// 0, 1, 2, ...
func requireSliceHeaders(t *testing.T, stream []byte, wantSlices int) {
	t.Helper()
	ps := &paramSets{}
	slices := 0
	for _, nalu := range avc.ExtractNalusFromByteStream(stream) {
		switch avc.GetNaluType(nalu[0]) {
		case avc.NALU_SPS:
			ps.addSPS(t, nalu)
		case avc.NALU_PPS:
			ps.addPPS(t, nalu)
		case avc.NALU_IDR, avc.NALU_NON_IDR:
			sh, err := avc.ParseSliceHeader(nalu, ps.sps, ps.pps)
			if err != nil {
				t.Fatalf("ParseSliceHeader: %v", err)
			}
			if sh.PicParamID != 1 || sh.FrameNum != uint32(slices) {
				t.Fatalf("slice %d uses PPS %d and frame_num %d, want PPS 1 and frame_num %d",
					slices, sh.PicParamID, sh.FrameNum, slices)
			}
			slices++
		}
	}
	if slices != wantSlices {
		t.Fatalf("got %d slices, want %d", slices, wantSlices)
	}
}

func TestPSkipExtenderFollowsSliceParameterSets(t *testing.T) {
	ps, stream := twoSPSStream(t, 0)
	// Created with SPS 0 and PPS 0, as from a sample entry that lists them first.
	ext, err := NewPSkipExtender(ps.sps[0], ps.pps[0])
	if err != nil {
		t.Fatalf("NewPSkipExtender: %v", err)
	}
	if err := ext.ObserveAnnexB(stream); err != nil {
		t.Fatalf("ObserveAnnexB: %v", err)
	}
	if sps, pps := ext.ParameterSets(); sps.ParameterID != 1 || pps.PicParameterSetID != 1 {
		t.Fatalf("ParameterSets = SPS %d, PPS %d, want SPS 1, PPS 1", sps.ParameterID, pps.PicParameterSetID)
	}
	const n = 20 // past the frame_num wrap of SPS 0
	slices, err := ext.NextSlices(n)
	if err != nil {
		t.Fatalf("NextSlices: %v", err)
	}
	extended := bytes.Join(append([][]byte{stream}, slices...), nil)
	requireSliceHeaders(t, extended, n+1)
	frames := decodeAll(t, extended)
	if len(frames) != n+1 {
		t.Fatalf("decoded %d frames, want %d", len(frames), n+1)
	}
	requireSamePicture(t, frames[n], frames[0], "last generated frame")
}

func TestAppendPSkipFramesFollowsSliceParameterSets(t *testing.T) {
	_, stream := twoSPSStream(t, 0)
	const n = 20
	extended, err := AppendPSkipFrames(stream, n)
	if err != nil {
		t.Fatalf("AppendPSkipFrames: %v", err)
	}
	requireSliceHeaders(t, extended, n+1)
	if frames := decodeAll(t, extended); len(frames) != n+1 {
		t.Fatalf("decoded %d frames, want %d", len(frames), n+1)
	}
}

// LastFrameState checks pic_order_cnt_type in the SPS of the last slice, not in the last SPS.
func TestLastFrameStateChecksSliceSPS(t *testing.T) {
	_, stream := twoSPSStream(t, 1) // SPS 0, which no slice uses, has pic_order_cnt_type 1
	if _, _, err := LastFrameState(stream); err != nil {
		t.Fatalf("LastFrameState: %v", err)
	}
}

// When the stream moves to an SPS that P_Skip slices cannot be generated for, NextSlice reports
// it rather than continuing with the previous SPS.
func TestPSkipExtenderRejectsUnsupportedSliceSPS(t *testing.T) {
	ps, stream := twoSPSStream(t, 1)
	ext, err := NewPSkipExtender(ps.sps[1], ps.pps[1])
	if err != nil {
		t.Fatalf("NewPSkipExtender: %v", err)
	}
	if err := ext.ObserveAnnexB(stream); err != nil {
		t.Fatalf("ObserveAnnexB: %v", err)
	}
	// An IDR on PPS 0, whose SPS has pic_order_cnt_type 1, with no picture order count fields.
	w := NewBitWriter()
	w.WriteUE(0)      // first_mb_in_slice
	w.WriteUE(7)      // slice_type: I
	w.WriteUE(0)      // pic_parameter_set_id
	w.WriteBits(0, 4) // frame_num
	w.WriteUE(1)      // idr_pic_id
	w.WriteBit(0)     // no_output_of_prior_pics_flag
	w.WriteBit(0)     // long_term_reference_flag
	w.WriteSE(0)      // slice_qp_delta
	w.WriteUE(1)      // disable_deblocking_filter_idc
	w.WriteBit(1)     // rbsp_stop_one_bit
	w.AlignToByte()
	if err := ext.ObserveNALU(BuildNALU(byte(avc.NALU_IDR), 3, w.Bytes())); err != nil {
		t.Fatalf("ObserveNALU: %v", err)
	}
	if _, err := ext.NextSlice(); err == nil || !strings.Contains(err.Error(), "pic_order_cnt_type=1") {
		t.Fatalf("NextSlice error = %v, want one about pic_order_cnt_type=1", err)
	}
}

// NewPSkipExtenderFromDecConfRec starts with the first PPS and the SPS it refers to, which here is
// not the first SPS.
func TestPSkipExtenderFromDecConfRecPairsFirstPPS(t *testing.T) {
	ps := &paramSets{}
	ps.addSPS(t, spsNALU(0, 0, 0))
	ps.addSPS(t, spsNALU(1, 4, 0))
	ps.addPPS(t, ppsNALU(1, 1))
	ps.addPPS(t, ppsNALU(0, 0))
	rec, err := avc.CreateAVCDecConfRec(ps.spsNALUs, ps.ppsNALUs, true)
	if err != nil {
		t.Fatalf("CreateAVCDecConfRec: %v", err)
	}
	var buf bytes.Buffer
	if err := rec.Encode(&buf); err != nil {
		t.Fatalf("encode avcC: %v", err)
	}
	ext, err := NewPSkipExtenderFromDecConfRec(buf.Bytes())
	if err != nil {
		t.Fatalf("NewPSkipExtenderFromDecConfRec: %v", err)
	}
	if sps, pps := ext.ParameterSets(); sps.ParameterID != 1 || pps.PicParameterSetID != 1 {
		t.Fatalf("ParameterSets = SPS %d, PPS %d, want SPS 1, PPS 1", sps.ParameterID, pps.PicParameterSetID)
	}
}

func TestNewPSkipExtenderRejectsMismatchedPPS(t *testing.T) {
	ps, _ := twoSPSStream(t, 0)
	_, err := NewPSkipExtender(ps.sps[0], ps.pps[1])
	if err == nil || !strings.Contains(err.Error(), "PPS 1 refers to SPS 1, not 0") {
		t.Fatalf("NewPSkipExtender error = %v, want one about PPS 1 referring to SPS 1", err)
	}
}
