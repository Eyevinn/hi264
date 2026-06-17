package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/Eyevinn/mp4ff/sei"

	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/hi264/pkg/yuv"
)

// buildPicTimingInputs writes an init.mp4 (whose SPS sets pic_struct_present_flag)
// and a one-IDR media segment, at fps frames/second (media timescale = fps,
// per-sample duration = 1).
func buildPicTimingInputs(t *testing.T, dir string, fps uint32) (initPath, segPath string) {
	t.Helper()
	const w, h = 32, 32
	level := encode.ChooseLevel(w, h, int(fps), 0, false)
	spsRBSP := encode.EncodeSPS(w, h, 1, level, 0, 0, true) // pic_struct_present=true, maxRef=1
	ppsRBSP := encode.EncodePPS(0)
	spsNALU := encode.BuildNALU(7, 3, spsRBSP)
	ppsNALU := encode.BuildNALU(8, 3, ppsRBSP)

	init := mp4.CreateEmptyInit()
	init.AddEmptyTrack(fps, "video", "und")
	trak := init.Moov.Trak
	if err := trak.SetAVCDescriptor("avc1", [][]byte{spsNALU}, [][]byte{ppsNALU}, true); err != nil {
		t.Fatalf("SetAVCDescriptor: %v", err)
	}
	initPath = filepath.Join(dir, "init.mp4")
	writeBox(t, initPath, func(f *os.File) error { return init.Encode(f) })

	grid, colors := yuv.SolidGrid(w, h, yuv.Color{Y: 16, Cb: 128, Cr: 128})
	plane, err := yuv.GridToPlaneGrid(grid, colors)
	if err != nil {
		t.Fatalf("GridToPlaneGrid: %v", err)
	}
	enc := &encode.FrameEncoder{Plane: plane, QP: 26, MaxNumRefFrames: 1, Width: w, Height: h, PicStructPresent: true}
	idrAnnexB, err := enc.EncodeSlice(0)
	if err != nil {
		t.Fatalf("EncodeSlice: %v", err)
	}
	sample := avc.ConvertByteStreamToNaluSample(idrAnnexB)

	frag, err := mp4.CreateFragment(1, trak.Tkhd.TrackID)
	if err != nil {
		t.Fatalf("CreateFragment: %v", err)
	}
	frag.AddFullSample(mp4.FullSample{
		Sample:     mp4.Sample{Flags: mp4.SyncSampleFlags, Dur: 1, Size: uint32(len(sample))},
		DecodeTime: 0,
		Data:       sample,
	})
	seg := mp4.NewMediaSegment()
	seg.AddFragment(frag)
	segPath = filepath.Join(dir, "seg.m4s")
	writeBox(t, segPath, func(f *os.File) error { return seg.Encode(f) })
	return initPath, segPath
}

func writeBox(t *testing.T, path string, encode func(*os.File) error) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := encode(f); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
}

// samplePicTimingTimecode returns the HH:MM:SS:FF timecode from a sample's
// pic_timing SEI, or "" if the sample carries no pic_timing SEI.
func samplePicTimingTimecode(t *testing.T, fs mp4.FullSample) string {
	t.Helper()
	annexB := avc.ConvertSampleToByteStream(append([]byte(nil), fs.Data...))
	for _, nalu := range avc.ExtractNalusFromByteStream(annexB) {
		if len(nalu) == 0 || nalu[0]&0x1f != 6 {
			continue
		}
		sd, err := sei.ExtractSEIData(bytes.NewReader(nalu[1:]))
		if err != nil {
			t.Fatalf("ExtractSEIData: %v", err)
		}
		for i := range sd {
			if sd[i].Type() != sei.SEIPicTimingType {
				continue
			}
			msg, err := sei.DecodePicTimingAvcSEI(&sd[i])
			if err != nil {
				t.Fatalf("DecodePicTimingAvcSEI: %v", err)
			}
			c := msg.(*sei.PicTimingAvcSEI).Clocks[0]
			return fmt.Sprintf("%02d:%02d:%02d:%02d", c.Hours, c.Minutes, c.Seconds, c.NFrames)
		}
	}
	return ""
}

// TestExtendContinuesPicTiming verifies that, when the source SPS signals
// pic_struct_present_flag (and no HRD), the appended frames carry pic_timing
// SEIs with timecodes continuing the source timeline (including second
// rollover), while the original sample is left untouched.
func TestExtendContinuesPicTiming(t *testing.T) {
	silenceStdout(t)
	dir := t.TempDir()
	initPath, segPath := buildPicTimingInputs(t, dir, 25)
	out := filepath.Join(dir, "out.m4s")

	if err := extendSegment(initPath, segPath, out, 27, false); err != nil {
		t.Fatalf("extendSegment: %v", err)
	}
	full := readFullSamples(t, out)
	if len(full) != 28 {
		t.Fatalf("got %d samples, want 28 (1 original + 27 appended)", len(full))
	}

	// The original sample was encoded without a pic_timing SEI and must remain
	// untouched.
	if tc := samplePicTimingTimecode(t, full[0]); tc != "" {
		t.Errorf("original sample gained a pic_timing SEI %q, want none", tc)
	}

	// Appended samples 1..27 at 25 fps: decodeTime == index, so the frame index
	// within the second rolls over at sample 25.
	want := map[int]string{
		1:  "00:00:00:01",
		24: "00:00:00:24",
		25: "00:00:01:00",
		27: "00:00:01:02",
	}
	for idx, wantTC := range want {
		if tc := samplePicTimingTimecode(t, full[idx]); tc != wantTC {
			t.Errorf("appended sample %d timecode = %q, want %q", idx, tc, wantTC)
		}
	}
}

// TestExtendNoPicTimingWhenSPSLacksFlag verifies that no SEIs are added when the
// source SPS does not signal pic_struct_present_flag (the testdata stream).
func TestExtendNoPicTimingWhenSPSLacksFlag(t *testing.T) {
	silenceStdout(t)
	out := filepath.Join(t.TempDir(), "out.m4s")
	if err := extendSegment(testInit, testSeg, out, 3, false); err != nil {
		t.Fatalf("extendSegment: %v", err)
	}
	for i, fs := range readFullSamples(t, out) {
		if tc := samplePicTimingTimecode(t, fs); tc != "" {
			t.Errorf("sample %d unexpectedly has pic_timing SEI %q", i, tc)
		}
	}
}
