package encode

import (
	"bytes"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"

	"github.com/Eyevinn/hi264/pkg/decoder"
	"github.com/Eyevinn/hi264/pkg/yuv"
)

// TestExtendStreamWithPSkipAt verifies that a bitstream whose POC progression
// does not follow the default pic_order_cnt_lsb=2*frame_num convention can be
// extended with P_Skip frames using EncodePSkipSlice + LastFrameState. The
// extended stream must decode to the expected number of frames with monotonic
// POC ordering (no dropped or reordered frames).
func TestExtendStreamWithPSkipAt(t *testing.T) {
	// Build a base stream: SPS + PPS + IDR + a P_Skip whose POC LSB does NOT
	// follow frameNum*2 (simulating what an external encoder might emit).
	// Source bitstream: IDR (frame_num=0, poc_lsb=0) + P_Skip (frame_num=1, poc_lsb=10).
	// log2MaxPicOrderCntLsbMinus4=0 → MaxPocLsb=16.
	grid, err := yuv.ParseGrid("xy,yx")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 200, Cb: 100, Cr: 150},
		'y': {Y: 50, Cb: 200, Cr: 80},
	}

	enc := &FrameEncoder{
		Grid:            grid,
		Colors:          colors,
		QP:              26,
		DisableDeblock:  1,
		MaxNumRefFrames: 1,
	}

	var buf bytes.Buffer
	if err := enc.EncodeSPSPPS(&buf); err != nil {
		t.Fatalf("EncodeSPSPPS: %v", err)
	}
	idrSlice, err := enc.EncodeSlice(0)
	if err != nil {
		t.Fatalf("EncodeSlice: %v", err)
	}
	buf.Write(idrSlice)

	// Parse SPS/PPS from generated stream
	sps, pps := mustParseSPSPPS(t, buf.Bytes())

	// Append a P_Skip with frame_num=1, poc_lsb=10 (not 2 = 1*2). This mimics
	// an arbitrary upstream POC stride.
	pSkip1, err := EncodePSkipSlice(sps, pps, 1, 10, 1)
	if err != nil {
		t.Fatalf("EncodePSkipSlice: %v", err)
	}
	buf.Write(pSkip1)

	// Now extend the stream: extract last state and append three more P_Skips
	// continuing the POC stride.
	lastFn, lastLsb, err := LastFrameState(buf.Bytes())
	if err != nil {
		t.Fatalf("LastFrameState: %v", err)
	}
	if lastFn != 1 || lastLsb != 10 {
		t.Fatalf("LastFrameState = (%d, %d), want (1, 10)", lastFn, lastLsb)
	}

	for i := range uint32(3) {
		nextFn := lastFn + 1 + i
		nextLsb := lastLsb + 2 + 2*i // continue with stride 2
		pSkip, err := EncodePSkipSlice(sps, pps, nextFn, nextLsb, 1)
		if err != nil {
			t.Fatalf("EncodePSkipSlice: %v", err)
		}
		buf.Write(pSkip)
	}

	// Decode the full stream and verify monotonic POC LSB across decoded slices.
	gotFNs, gotPocLsbs := parseFrameNumAndPoc(t, buf.Bytes())
	wantFNs := []uint32{0, 1, 2, 3, 4}
	wantPocLsbs := []uint32{0, 10, 12, 14, 0} // 16 mod 16 = 0
	if !equalU32(gotFNs, wantFNs) {
		t.Errorf("frame_nums = %v, want %v", gotFNs, wantFNs)
	}
	if !equalU32(gotPocLsbs, wantPocLsbs) {
		t.Errorf("pic_order_cnt_lsbs = %v, want %v", gotPocLsbs, wantPocLsbs)
	}

	// Round-trip through hi264 decoder: should produce 5 frames identical to IDR.
	nalus := avc.ExtractNalusFromByteStream(buf.Bytes())
	dec := decoder.New()
	dec.SkipDeblock = true
	frames, err := dec.DecodeAllFrames(nalus)
	if err != nil {
		t.Fatalf("DecodeAllFrames: %v", err)
	}
	if len(frames) != 5 {
		t.Fatalf("expected 5 frames, got %d", len(frames))
	}
	idr := frames[0]
	for f := 1; f < 5; f++ {
		for y := 0; y < idr.Height; y++ {
			for x := 0; x < idr.Width; x++ {
				if frames[f].GetLumaPixel(x, y) != idr.GetLumaPixel(x, y) {
					t.Fatalf("frame %d differs from IDR at luma(%d,%d)", f, x, y)
				}
			}
		}
	}
}

// TestAppendPSkipFrames verifies the high-level one-call helper extends an
// existing stream by N P_Skip frames that decode in order.
func TestAppendPSkipFrames(t *testing.T) {
	grid, err := yuv.ParseGrid("xy,yx")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 200, Cb: 100, Cr: 150},
		'y': {Y: 50, Cb: 200, Cr: 80},
	}
	enc := &FrameEncoder{
		Grid: grid, Colors: colors, QP: 26, DisableDeblock: 1, MaxNumRefFrames: 1,
	}
	var buf bytes.Buffer
	if err := enc.EncodeSPSPPS(&buf); err != nil {
		t.Fatalf("EncodeSPSPPS: %v", err)
	}
	idr, err := enc.EncodeSlice(0)
	if err != nil {
		t.Fatalf("EncodeSlice: %v", err)
	}
	buf.Write(idr)

	// Append 4 frames in one call.
	extended, err := AppendPSkipFrames(buf.Bytes(), 4)
	if err != nil {
		t.Fatalf("AppendPSkipFrames: %v", err)
	}

	gotFNs, _ := parseFrameNumAndPoc(t, extended)
	wantFNs := []uint32{0, 1, 2, 3, 4}
	if !equalU32(gotFNs, wantFNs) {
		t.Errorf("frame_nums = %v, want %v", gotFNs, wantFNs)
	}

	nalus := avc.ExtractNalusFromByteStream(extended)
	dec := decoder.New()
	dec.SkipDeblock = true
	frames, err := dec.DecodeAllFrames(nalus)
	if err != nil {
		t.Fatalf("DecodeAllFrames: %v", err)
	}
	if len(frames) != 5 {
		t.Fatalf("expected 5 frames, got %d", len(frames))
	}
}

// TestAppendPSkipFrames_ZeroCount returns the input unchanged.
func TestAppendPSkipFrames_ZeroCount(t *testing.T) {
	p := EncodeParams{Width: 32, Height: 32, QP: 26}
	sps, _ := GenerateSPS(p)
	pps, _ := GeneratePPS(p)
	idr, _ := GenerateIDR(p, mustGrid(t), mustColors(), 0)
	stream := append(append([]byte{}, sps...), pps...)
	stream = append(stream, idr...)

	out, err := AppendPSkipFrames(stream, 0)
	if err != nil {
		t.Fatalf("AppendPSkipFrames: %v", err)
	}
	if !bytes.Equal(out, stream) {
		t.Fatal("zero-count append must return input unchanged")
	}
}

func mustGrid(t *testing.T) *yuv.Grid {
	t.Helper()
	g, err := yuv.ParseGrid("x")
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func mustColors() yuv.ColorMap {
	return yuv.ColorMap{'x': {Y: 128, Cb: 128, Cr: 128}}
}

// TestLastFrameState_NoSlices verifies the helper rejects an SPS/PPS-only stream.
func TestLastFrameState_NoSlices(t *testing.T) {
	p := EncodeParams{Width: 32, Height: 32, QP: 26}
	sps, _ := GenerateSPS(p)
	pps, _ := GeneratePPS(p)
	stream := append(sps, pps...)
	_, _, err := LastFrameState(stream)
	if err == nil {
		t.Fatal("expected error for stream without slices")
	}
}

// TestLastFrameState_PocType1Rejected verifies that streams using
// pic_order_cnt_type 1 are rejected. Types 0 and 2 are supported.
func TestLastFrameState_PocType1Rejected(t *testing.T) {
	// Hand-build an SPS with PicOrderCntType=1. (Types 0 and 2 are accepted.)
	sw := NewBitWriter()
	sw.WriteBits(66, 8) // profile_idc=Baseline
	sw.WriteBits(0xC0, 8)
	sw.WriteBits(30, 8) // level_idc
	sw.WriteUE(0)       // seq_parameter_set_id
	sw.WriteUE(0)       // log2_max_frame_num_minus4
	sw.WriteUE(1)       // pic_order_cnt_type = 1
	// pic_order_cnt_type=1 needs more SPS fields, but ParseSPSNALUnit will
	// still set PicOrderCntType=1 which is what LastFrameState checks.
	sw.WriteBit(1) // delta_pic_order_always_zero_flag
	sw.WriteSE(0)  // offset_for_non_ref_pic
	sw.WriteSE(0)  // offset_for_top_to_bottom_field
	sw.WriteUE(0)  // num_ref_frames_in_pic_order_cnt_cycle
	sw.WriteUE(1)  // max_num_ref_frames
	sw.WriteBit(0) // gaps_in_frame_num_value_allowed_flag
	sw.WriteUE(0)  // pic_width_in_mbs_minus1
	sw.WriteUE(0)  // pic_height_in_map_units_minus1
	sw.WriteBit(1) // frame_mbs_only_flag
	sw.WriteBit(0) // direct_8x8_inference_flag
	sw.WriteBit(0) // frame_cropping_flag
	sw.WriteBit(0) // vui_parameters_present_flag
	sw.WriteBit(1) // rbsp_stop_one_bit
	sw.AlignToByte()
	var buf bytes.Buffer
	if err := WriteNALU(&buf, 7, 3, sw.Bytes()); err != nil {
		t.Fatal(err)
	}
	// Append a fabricated NON_IDR header so the parser hits a slice.
	idrW := NewBitWriter()
	idrW.WriteUE(0) // first_mb_in_slice
	idrW.WriteUE(2) // slice_type = I
	idrW.WriteUE(0) // pps_id
	idrW.WriteBits(0, 4)
	idrW.WriteUE(0)
	idrW.WriteBit(0)
	idrW.WriteBit(0)
	idrW.WriteSE(0)
	idrW.WriteUE(1)
	idrW.WriteBit(1)
	idrW.AlignToByte()
	// Need a PPS so slice header parses. Use the standard one.
	ppsRBSP := EncodePPS(1)
	if err := WriteNALU(&buf, 8, 3, ppsRBSP); err != nil {
		t.Fatal(err)
	}
	if err := WriteNALU(&buf, 5, 3, idrW.Bytes()); err != nil {
		t.Fatal(err)
	}
	_, _, err := LastFrameState(buf.Bytes())
	if err == nil {
		t.Fatal("expected error for pic_order_cnt_type=1")
	}
}

func mustParseSPSPPS(t *testing.T, data []byte) (*avc.SPS, *avc.PPS) {
	t.Helper()
	nalus := avc.ExtractNalusFromByteStream(data)
	spsMap := make(map[uint32]*avc.SPS)
	var sps *avc.SPS
	var pps *avc.PPS
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		switch avc.GetNaluType(nalu[0]) {
		case avc.NALU_SPS:
			s, err := avc.ParseSPSNALUnit(nalu, true)
			if err != nil {
				t.Fatalf("ParseSPSNALUnit: %v", err)
			}
			sps = s
			spsMap[uint32(s.ParameterID)] = s
		case avc.NALU_PPS:
			p, err := avc.ParsePPSNALUnit(nalu, spsMap)
			if err != nil {
				t.Fatalf("ParsePPSNALUnit: %v", err)
			}
			pps = p
		}
	}
	if sps == nil || pps == nil {
		t.Fatal("SPS or PPS not found")
	}
	return sps, pps
}

func parseFrameNumAndPoc(t *testing.T, data []byte) (frameNums []uint32, pocLsbs []uint32) {
	t.Helper()
	nalus := avc.ExtractNalusFromByteStream(data)
	spsMap := make(map[uint32]*avc.SPS)
	ppsMap := make(map[uint32]*avc.PPS)
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		switch avc.GetNaluType(nalu[0]) {
		case avc.NALU_SPS:
			s, err := avc.ParseSPSNALUnit(nalu, true)
			if err != nil {
				t.Fatalf("ParseSPSNALUnit: %v", err)
			}
			spsMap[uint32(s.ParameterID)] = s
		case avc.NALU_PPS:
			p, err := avc.ParsePPSNALUnit(nalu, spsMap)
			if err != nil {
				t.Fatalf("ParsePPSNALUnit: %v", err)
			}
			ppsMap[p.PicParameterSetID] = p
		case avc.NALU_IDR, avc.NALU_NON_IDR:
			sh, err := avc.ParseSliceHeader(nalu, spsMap, ppsMap)
			if err != nil {
				t.Fatalf("ParseSliceHeader: %v", err)
			}
			frameNums = append(frameNums, uint32(sh.FrameNum))
			pocLsbs = append(pocLsbs, uint32(sh.PicOrderCntLsb))
		}
	}
	return frameNums, pocLsbs
}

func equalU32(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
