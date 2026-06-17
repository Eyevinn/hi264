package encode

import (
	"bytes"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/sei"
)

// seiPayloadFromNALU extracts the SEI rbsp messages from an Annex-B SEI NALU.
func seiPayloadFromNALU(t *testing.T, annexB []byte) []sei.SEIData {
	t.Helper()
	nalus := avc.ExtractNalusFromByteStream(annexB)
	if len(nalus) != 1 {
		t.Fatalf("expected 1 NALU, got %d", len(nalus))
	}
	if got := nalus[0][0] & 0x1f; got != 6 {
		t.Fatalf("nal_unit_type = %d, want 6 (SEI)", got)
	}
	if refIDC := (nalus[0][0] >> 5) & 0x3; refIDC != 0 {
		t.Fatalf("nal_ref_idc = %d, want 0 for SEI", refIDC)
	}
	sd, err := sei.ExtractSEIData(bytes.NewReader(nalus[0][1:]))
	if err != nil {
		t.Fatalf("ExtractSEIData: %v", err)
	}
	return sd
}

func TestGeneratePicTimingSEI_Timecode(t *testing.T) {
	cfg := PicTimingConfig{PicStructPresent: true}
	pt := PicTiming{Hours: 1, Minutes: 2, Seconds: 3, Frames: 4}

	annexB, err := GeneratePicTimingSEI(cfg, pt)
	if err != nil {
		t.Fatalf("GeneratePicTimingSEI: %v", err)
	}

	sd := seiPayloadFromNALU(t, annexB)
	if len(sd) != 1 {
		t.Fatalf("expected 1 SEI message, got %d", len(sd))
	}
	if sd[0].Type() != sei.SEIPicTimingType {
		t.Fatalf("payloadType = %d, want %d", sd[0].Type(), sei.SEIPicTimingType)
	}

	msg, err := sei.DecodePicTimingAvcSEI(&sd[0])
	if err != nil {
		t.Fatalf("DecodePicTimingAvcSEI: %v", err)
	}
	pic := msg.(*sei.PicTimingAvcSEI)
	if pic.PictStruct != 0 {
		t.Errorf("PictStruct = %d, want 0", pic.PictStruct)
	}
	if len(pic.Clocks) != 1 {
		t.Fatalf("expected 1 clock timestamp, got %d", len(pic.Clocks))
	}
	c := pic.Clocks[0]
	if c.Hours != 1 || c.Minutes != 2 || c.Seconds != 3 || c.NFrames != 4 {
		t.Errorf("timecode = %02d:%02d:%02d:%02d, want 01:02:03:04",
			c.Hours, c.Minutes, c.Seconds, c.NFrames)
	}
	if !c.ClockTimeStampFlag || !c.FullTimeStampFlag {
		t.Errorf("ClockTimeStampFlag=%v FullTimeStampFlag=%v, want both true",
			c.ClockTimeStampFlag, c.FullTimeStampFlag)
	}
}

func TestGeneratePicTimingSEI_HRD(t *testing.T) {
	hrd := &HRDDelayLengths{
		CpbRemovalDelayLenMinus1: 23, // 24-bit fields
		DpbOutputDelayLenMinus1:  23,
		TimeOffsetLength:         0,
	}
	cfg := PicTimingConfig{PicStructPresent: true, HRD: hrd}
	pt := PicTiming{
		Hours: 0, Minutes: 0, Seconds: 5, Frames: 12,
		CpbRemovalDelay: 250,
		DpbOutputDelay:  4,
	}

	annexB, err := GeneratePicTimingSEI(cfg, pt)
	if err != nil {
		t.Fatalf("GeneratePicTimingSEI: %v", err)
	}
	sd := seiPayloadFromNALU(t, annexB)

	// Decoding the HRD variant requires telling the decoder the field lengths.
	cbp := &sei.CbpDbpDelay{
		CpbRemovalDelayLengthMinus1: hrd.CpbRemovalDelayLenMinus1,
		DpbOutputDelayLengthMinus1:  hrd.DpbOutputDelayLenMinus1,
	}
	msg, err := sei.DecodePicTimingAvcSEIHRD(&sd[0], cbp, hrd.TimeOffsetLength)
	if err != nil {
		t.Fatalf("DecodePicTimingAvcSEIHRD: %v", err)
	}
	pic := msg.(*sei.PicTimingAvcSEI)
	if pic.CbpDbpDelay == nil {
		t.Fatalf("CbpDbpDelay not decoded")
	}
	if pic.CbpDbpDelay.CpbRemovalDelay != 250 || pic.CbpDbpDelay.DpbOutputDelay != 4 {
		t.Errorf("delays = cpb %d / dpb %d, want 250 / 4",
			pic.CbpDbpDelay.CpbRemovalDelay, pic.CbpDbpDelay.DpbOutputDelay)
	}
	c := pic.Clocks[0]
	if c.Seconds != 5 || c.NFrames != 12 {
		t.Errorf("timecode seconds=%d nframes=%d, want 5 / 12", c.Seconds, c.NFrames)
	}
}

func TestBuildPicTimingSEINALU_RawForm(t *testing.T) {
	cfg := PicTimingConfig{PicStructPresent: true}
	pt := PicTiming{Hours: 10, Minutes: 20, Seconds: 30, Frames: 5}

	raw, err := BuildPicTimingSEINALU(cfg, pt)
	if err != nil {
		t.Fatalf("BuildPicTimingSEINALU: %v", err)
	}
	if raw[0] != 0x06 {
		t.Fatalf("raw NALU header = 0x%02x, want 0x06 (type 6, ref_idc 0)", raw[0])
	}
	// The raw form (no start code) must decode to the same message as the
	// Annex-B form.
	sd, err := sei.ExtractSEIData(bytes.NewReader(raw[1:]))
	if err != nil {
		t.Fatalf("ExtractSEIData: %v", err)
	}
	msg, err := sei.DecodePicTimingAvcSEI(&sd[0])
	if err != nil {
		t.Fatalf("DecodePicTimingAvcSEI: %v", err)
	}
	c := msg.(*sei.PicTimingAvcSEI).Clocks[0]
	if c.Hours != 10 || c.Minutes != 20 || c.Seconds != 30 || c.NFrames != 5 {
		t.Errorf("timecode = %02d:%02d:%02d:%02d, want 10:20:30:05",
			c.Hours, c.Minutes, c.Seconds, c.NFrames)
	}
}

func TestGeneratePicTimingSEI_RequiresPicStruct(t *testing.T) {
	if _, err := GeneratePicTimingSEI(PicTimingConfig{}, PicTiming{}); err == nil {
		t.Fatal("expected error when PicStructPresent is false")
	}
	if _, err := BuildPicTimingSEINALU(PicTimingConfig{}, PicTiming{}); err == nil {
		t.Fatal("expected error when PicStructPresent is false")
	}
}

func TestPicTimingConfigFromSPS(t *testing.T) {
	// Round-trip an SPS that signals pic_struct_present_flag (no HRD).
	rbsp := EncodeSPS(320, 240, 0, 30, 0, 0, true)
	nalu := append([]byte{0x67}, rbsp...)
	sps, err := avc.ParseSPSNALUnit(nalu, true)
	if err != nil {
		t.Fatalf("ParseSPSNALUnit: %v", err)
	}
	cfg := PicTimingConfigFromSPS(sps)
	if !cfg.PicStructPresent {
		t.Errorf("PicStructPresent = false, want true")
	}
	if cfg.HRD != nil {
		t.Errorf("HRD = %+v, want nil (no HRD signaled)", cfg.HRD)
	}

	// An SPS without pic_struct must report PicStructPresent=false.
	rbsp2 := EncodeSPS(320, 240, 0, 30, 0, 0, false)
	nalu2 := append([]byte{0x67}, rbsp2...)
	sps2, err := avc.ParseSPSNALUnit(nalu2, true)
	if err != nil {
		t.Fatalf("ParseSPSNALUnit: %v", err)
	}
	if PicTimingConfigFromSPS(sps2).PicStructPresent {
		t.Errorf("PicStructPresent = true, want false")
	}
}
