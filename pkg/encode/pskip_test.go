package encode

import (
	"bytes"
	"testing"

	"github.com/Eyevinn/hi264/pkg/decoder"
	"github.com/Eyevinn/hi264/pkg/yuv"
	"github.com/Eyevinn/mp4ff/avc"
)

// TestEncodePSkipSliceRoundTrip generates SPS/PPS, parses them back, encodes
// IDR + P_Skip using the standalone function, decodes both, and verifies the
// P_Skip frame matches the IDR frame.
func TestEncodePSkipSliceRoundTrip(t *testing.T) {
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

	// Generate SPS/PPS and IDR
	var buf bytes.Buffer
	if err := enc.EncodeSPSPPS(&buf); err != nil {
		t.Fatalf("EncodeSPSPPS: %v", err)
	}
	idrSlice, err := enc.EncodeSlice(0)
	if err != nil {
		t.Fatalf("EncodeSlice: %v", err)
	}
	buf.Write(idrSlice)

	// Parse SPS/PPS back from generated bitstream
	spsRBSP := EncodeSPS(32, 32, 1, 30, 0, 0)
	spsNalu := append([]byte{0x67}, spsRBSP...)
	sps, err := avc.ParseSPSNALUnit(spsNalu, true)
	if err != nil {
		t.Fatalf("ParseSPSNALUnit: %v", err)
	}
	spsMap := map[uint32]*avc.SPS{0: sps}

	ppsRBSP := EncodePPS(1)
	ppsNalu := append([]byte{0x68}, ppsRBSP...)
	pps, err := avc.ParsePPSNALUnit(ppsNalu, spsMap)
	if err != nil {
		t.Fatalf("ParsePPSNALUnit: %v", err)
	}

	// Encode P_Skip using the standalone function
	pSkipSlice, err := EncodePSkipSlice(sps, pps, 1, 2, 1)
	if err != nil {
		t.Fatalf("EncodePSkipSlice: %v", err)
	}
	buf.Write(pSkipSlice)

	// Decode all frames
	nalus := avc.ExtractNalusFromByteStream(buf.Bytes())
	dec := decoder.New()
	dec.SkipDeblock = true
	frames, err := dec.DecodeAllFrames(nalus)
	if err != nil {
		t.Fatalf("DecodeAllFrames: %v", err)
	}

	if len(frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames))
	}

	// Verify P_Skip frame is identical to IDR frame
	idrFrame := frames[0]
	pSkipFrame := frames[1]

	if idrFrame.Width != pSkipFrame.Width || idrFrame.Height != pSkipFrame.Height {
		t.Fatalf("frame size mismatch: IDR %dx%d vs P_Skip %dx%d",
			idrFrame.Width, idrFrame.Height, pSkipFrame.Width, pSkipFrame.Height)
	}

	for y := 0; y < idrFrame.Height; y++ {
		for x := 0; x < idrFrame.Width; x++ {
			got := pSkipFrame.GetLumaPixel(x, y)
			want := idrFrame.GetLumaPixel(x, y)
			if got != want {
				t.Errorf("P_Skip luma(%d,%d) = %d, want %d (IDR)", x, y, got, want)
				return
			}
		}
	}

	chromaW := idrFrame.Width / 2
	chromaH := idrFrame.Height / 2
	for y := range chromaH {
		for x := range chromaW {
			for c := range 2 {
				got := pSkipFrame.GetChromaPixel(c, x, y)
				want := idrFrame.GetChromaPixel(c, x, y)
				if got != want {
					t.Errorf("P_Skip chroma[%d](%d,%d) = %d, want %d", c, x, y, got, want)
					return
				}
			}
		}
	}
}

// TestEncodePSkipSliceNonDefaultSPS verifies P_Skip encoding with non-default
// SPS parameters (log2MaxFrameNumMinus4=4, log2MaxPicOrderCntLsbMinus4=2).
func TestEncodePSkipSliceNonDefaultSPS(t *testing.T) {
	const (
		log2MaxFrameNumMinus4       = 4
		log2MaxPicOrderCntLsbMinus4 = 2
	)

	// Build SPS RBSP manually with non-default log2_max values
	sw := NewBitWriter()
	// profile_idc = 66 (Baseline)
	sw.WriteBits(66, 8)
	// constraint_set0..5_flags + reserved_zero_2bits
	sw.WriteBits(0xC0, 8)
	// level_idc = 30
	sw.WriteBits(30, 8)
	// seq_parameter_set_id = 0
	sw.WriteUE(0)
	// log2_max_frame_num_minus4 = 4 (frame_num uses 8 bits)
	sw.WriteUE(log2MaxFrameNumMinus4)
	// pic_order_cnt_type = 0
	sw.WriteUE(0)
	// log2_max_pic_order_cnt_lsb_minus4 = 2 (POC LSB uses 6 bits)
	sw.WriteUE(log2MaxPicOrderCntLsbMinus4)
	// max_num_ref_frames = 1
	sw.WriteUE(1)
	// gaps_in_frame_num_value_allowed_flag = 0
	sw.WriteBit(0)
	// pic_width_in_mbs_minus1 = 0 (1 MB wide = 16 pixels)
	sw.WriteUE(0)
	// pic_height_in_map_units_minus1 = 0 (1 MB tall = 16 pixels)
	sw.WriteUE(0)
	// frame_mbs_only_flag = 1
	sw.WriteBit(1)
	// direct_8x8_inference_flag = 0
	sw.WriteBit(0)
	// frame_cropping_flag = 0
	sw.WriteBit(0)
	// vui_parameters_present_flag = 0
	sw.WriteBit(0)
	// RBSP trailing bits
	sw.WriteBit(1)
	sw.AlignToByte()

	spsRBSP := sw.Bytes()
	spsNalu := append([]byte{0x67}, spsRBSP...)
	sps, err := avc.ParseSPSNALUnit(spsNalu, true)
	if err != nil {
		t.Fatalf("ParseSPSNALUnit: %v", err)
	}

	// Verify parsed values
	if sps.Log2MaxFrameNumMinus4 != log2MaxFrameNumMinus4 {
		t.Fatalf("Log2MaxFrameNumMinus4 = %d, want %d", sps.Log2MaxFrameNumMinus4, log2MaxFrameNumMinus4)
	}
	if sps.Log2MaxPicOrderCntLsbMinus4 != log2MaxPicOrderCntLsbMinus4 {
		t.Fatalf("Log2MaxPicOrderCntLsbMinus4 = %d, want %d",
			sps.Log2MaxPicOrderCntLsbMinus4, log2MaxPicOrderCntLsbMinus4)
	}

	spsMap := map[uint32]*avc.SPS{0: sps}

	ppsRBSP := EncodePPS(1)
	ppsNalu := append([]byte{0x68}, ppsRBSP...)
	pps, err := avc.ParsePPSNALUnit(ppsNalu, spsMap)
	if err != nil {
		t.Fatalf("ParsePPSNALUnit: %v", err)
	}

	// Build a complete bitstream: SPS + PPS + manually-written IDR + P_Skip.
	// The IDR must use the non-default bit widths for frame_num and POC.
	var buf bytes.Buffer
	if err := WriteNALU(&buf, 7, 3, spsRBSP); err != nil {
		t.Fatalf("write SPS: %v", err)
	}
	if err := WriteNALU(&buf, 8, 3, ppsRBSP); err != nil {
		t.Fatalf("write PPS: %v", err)
	}

	// Manually write IDR slice for a 1x1 MB grid (16x16, gray 128, QP=26).
	// With pred=128 and target=128, all residuals are 0, simplifying encoding.
	idrW := NewBitWriter()
	// IDR slice header (matching non-default SPS)
	idrW.WriteUE(0)                                       // first_mb_in_slice
	idrW.WriteUE(2)                                       // slice_type = I
	idrW.WriteUE(0)                                       // pps_id
	idrW.WriteBits(0, int(log2MaxFrameNumMinus4+4))       // frame_num = 0
	idrW.WriteUE(0)                                       // idr_pic_id = 0
	idrW.WriteBits(0, int(log2MaxPicOrderCntLsbMinus4+4)) // poc_lsb = 0
	idrW.WriteBit(0)                                      // no_output_of_prior_pics_flag
	idrW.WriteBit(0)                                      // long_term_reference_flag
	idrW.WriteSE(0)                                       // slice_qp_delta = 0
	idrW.WriteUE(1)                                       // disable_deblocking_filter_idc = 1
	// Single MB: I_16x16 DC prediction, cbpChroma=0, cbpLuma=0 → mb_type=3
	idrW.WriteUE(3) // mb_type
	idrW.WriteUE(0) // intra_chroma_pred_mode = 0 (DC)
	idrW.WriteSE(0) // mb_qp_delta = 0
	// Luma DC residual: all zeros → CAVLC coeff_token(totalCoeff=0,TO=0) at nC=0
	var zeroCoeffs [16]int32
	EncodeResidualBlock(idrW, zeroCoeffs[:], 0, 16)
	// RBSP trailing bits
	idrW.WriteBit(1)
	idrW.AlignToByte()

	if err := WriteNALU(&buf, 5, 3, idrW.Bytes()); err != nil {
		t.Fatalf("write IDR: %v", err)
	}

	// Encode P_Skip with non-default SPS
	pSkipSlice, err := EncodePSkipSlice(sps, pps, 1, 2, 1)
	if err != nil {
		t.Fatalf("EncodePSkipSlice: %v", err)
	}
	buf.Write(pSkipSlice)

	// Decode all frames - decoder must accept the non-default parameters
	nalus := avc.ExtractNalusFromByteStream(buf.Bytes())
	dec := decoder.New()
	dec.SkipDeblock = true
	frames, err := dec.DecodeAllFrames(nalus)
	if err != nil {
		t.Fatalf("DecodeAllFrames: %v", err)
	}

	if len(frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames))
	}

	// Verify P_Skip frame matches IDR (all pixels should be gray 128)
	idrFrame := frames[0]
	pSkipFrame := frames[1]
	for y := 0; y < idrFrame.Height; y++ {
		for x := 0; x < idrFrame.Width; x++ {
			got := pSkipFrame.GetLumaPixel(x, y)
			want := idrFrame.GetLumaPixel(x, y)
			if got != want {
				t.Errorf("P_Skip luma(%d,%d) = %d, want %d", x, y, got, want)
				return
			}
		}
	}
}

// TestEncodePSkipSliceCABACRoundTrip generates Main-profile SPS/PPS (CABAC),
// encodes IDR (CABAC) + P_Skip (CABAC), decodes both, and verifies the
// P_Skip frame matches the IDR frame.
func TestEncodePSkipSliceCABACRoundTrip(t *testing.T) {
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
		CABAC:           true,
		MaxNumRefFrames: 1,
	}

	// Generate SPS/PPS and IDR
	var buf bytes.Buffer
	if err := enc.EncodeSPSPPS(&buf); err != nil {
		t.Fatalf("EncodeSPSPPS: %v", err)
	}
	idrSlice, err := enc.EncodeSlice(0)
	if err != nil {
		t.Fatalf("EncodeSlice: %v", err)
	}
	buf.Write(idrSlice)

	// Parse SPS/PPS back for standalone EncodePSkipSlice
	spsRBSP := EncodeSPSMain(32, 32, 1, 30, 0, 0)
	spsNalu := append([]byte{0x67}, spsRBSP...)
	sps, err := avc.ParseSPSNALUnit(spsNalu, true)
	if err != nil {
		t.Fatalf("ParseSPSNALUnit: %v", err)
	}
	spsMap := map[uint32]*avc.SPS{0: sps}

	ppsRBSP := EncodePPSCABAC(1)
	ppsNalu := append([]byte{0x68}, ppsRBSP...)
	pps, err := avc.ParsePPSNALUnit(ppsNalu, spsMap)
	if err != nil {
		t.Fatalf("ParsePPSNALUnit: %v", err)
	}

	// Encode P_Skip using CABAC
	pSkipSlice, err := EncodePSkipSlice(sps, pps, 1, 2, 1)
	if err != nil {
		t.Fatalf("EncodePSkipSlice CABAC: %v", err)
	}
	buf.Write(pSkipSlice)

	// Decode all frames
	nalus := avc.ExtractNalusFromByteStream(buf.Bytes())
	dec := decoder.New()
	dec.SkipDeblock = true
	frames, err := dec.DecodeAllFrames(nalus)
	if err != nil {
		t.Fatalf("DecodeAllFrames: %v", err)
	}

	if len(frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames))
	}

	// Verify P_Skip frame is identical to IDR frame
	idrFrame := frames[0]
	pSkipFrame := frames[1]

	if idrFrame.Width != pSkipFrame.Width || idrFrame.Height != pSkipFrame.Height {
		t.Fatalf("frame size mismatch: IDR %dx%d vs P_Skip %dx%d",
			idrFrame.Width, idrFrame.Height, pSkipFrame.Width, pSkipFrame.Height)
	}

	for y := 0; y < idrFrame.Height; y++ {
		for x := 0; x < idrFrame.Width; x++ {
			got := pSkipFrame.GetLumaPixel(x, y)
			want := idrFrame.GetLumaPixel(x, y)
			if got != want {
				t.Errorf("CABAC P_Skip luma(%d,%d) = %d, want %d (IDR)", x, y, got, want)
				return
			}
		}
	}

	chromaW := idrFrame.Width / 2
	chromaH := idrFrame.Height / 2
	for y := range chromaH {
		for x := range chromaW {
			for c := range 2 {
				got := pSkipFrame.GetChromaPixel(c, x, y)
				want := idrFrame.GetChromaPixel(c, x, y)
				if got != want {
					t.Errorf("CABAC P_Skip chroma[%d](%d,%d) = %d, want %d", c, x, y, got, want)
					return
				}
			}
		}
	}
}

// TestEncodePSkipSliceCABACNonDefaultSPS verifies CABAC P_Skip encoding with
// non-default SPS parameters produces a valid NALU.
func TestEncodePSkipSliceCABACNonDefaultSPS(t *testing.T) {
	const (
		log2MaxFrameNumMinus4       = 4
		log2MaxPicOrderCntLsbMinus4 = 2
	)

	// Build Main-profile SPS RBSP with non-default log2_max values
	sw := NewBitWriter()
	sw.WriteBits(77, 8)   // profile_idc = 77 (Main)
	sw.WriteBits(0x40, 8) // constraint flags
	sw.WriteBits(30, 8)   // level_idc = 30
	sw.WriteUE(0)         // seq_parameter_set_id = 0
	sw.WriteUE(log2MaxFrameNumMinus4)
	sw.WriteUE(0) // pic_order_cnt_type = 0
	sw.WriteUE(log2MaxPicOrderCntLsbMinus4)
	sw.WriteUE(1)  // max_num_ref_frames = 1
	sw.WriteBit(0) // gaps_in_frame_num_value_allowed_flag
	sw.WriteUE(0)  // pic_width_in_mbs_minus1 = 0 (16 pixels)
	sw.WriteUE(0)  // pic_height_in_map_units_minus1 = 0 (16 pixels)
	sw.WriteBit(1) // frame_mbs_only_flag = 1
	sw.WriteBit(1) // direct_8x8_inference_flag = 1 (Main profile)
	sw.WriteBit(0) // frame_cropping_flag = 0
	sw.WriteBit(0) // vui_parameters_present_flag = 0
	sw.WriteBit(1) // RBSP trailing
	sw.AlignToByte()

	spsNalu := append([]byte{0x67}, sw.Bytes()...)
	sps, err := avc.ParseSPSNALUnit(spsNalu, true)
	if err != nil {
		t.Fatalf("ParseSPSNALUnit: %v", err)
	}
	spsMap := map[uint32]*avc.SPS{0: sps}

	ppsRBSP := EncodePPSCABAC(1)
	ppsNalu := append([]byte{0x68}, ppsRBSP...)
	pps, err := avc.ParsePPSNALUnit(ppsNalu, spsMap)
	if err != nil {
		t.Fatalf("ParsePPSNALUnit: %v", err)
	}

	// Encode P_Skip with CABAC + non-default SPS
	pSkipSlice, err := EncodePSkipSlice(sps, pps, 1, 2, 1)
	if err != nil {
		t.Fatalf("EncodePSkipSlice CABAC: %v", err)
	}

	// Verify NALU structure
	if len(pSkipSlice) < 5 {
		t.Fatalf("P_Skip slice too short: %d bytes", len(pSkipSlice))
	}
	// Annex-B start code
	if pSkipSlice[0] != 0 || pSkipSlice[1] != 0 || pSkipSlice[2] != 0 || pSkipSlice[3] != 1 {
		t.Fatalf("missing start code: %x", pSkipSlice[:4])
	}
	naluHeader := pSkipSlice[4]
	if naluHeader&0x1f != 1 {
		t.Fatalf("NALU type = %d, want 1", naluHeader&0x1f)
	}
	if (naluHeader>>5)&0x3 != 2 {
		t.Fatalf("nal_ref_idc = %d, want 2", (naluHeader>>5)&0x3)
	}
}

// TestEncodePSkipSliceErrorPOCType verifies that POC type != 0 is rejected.
func TestEncodePSkipSliceErrorPOCType(t *testing.T) {
	// Create an SPS with pic_order_cnt_type = 2
	sps := &avc.SPS{
		PicOrderCntType:  2,
		FrameMbsOnlyFlag: true,
		Width:            32,
		Height:           32,
	}
	pps := &avc.PPS{
		EntropyCodingModeFlag:              false,
		DeblockingFilterControlPresentFlag: true,
	}

	_, err := EncodePSkipSlice(sps, pps, 1, 2, 0)
	if err == nil {
		t.Fatal("expected error for POC type 2, got nil")
	}
}
