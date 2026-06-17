package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/Eyevinn/mp4ff/sei"
)

// seiTimecodes returns the pic_timing timecode (HH:MM:SS:FF) for each access
// unit in an Annex-B stream that carries a pic_timing SEI, in order.
func seiTimecodes(t *testing.T, annexB []byte) []string {
	t.Helper()
	var out []string
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
			out = append(out, fmt.Sprintf("%02d:%02d:%02d:%02d", c.Hours, c.Minutes, c.Seconds, c.NFrames))
		}
	}
	return out
}

// firstSEICountingType returns the counting_type of the first pic_timing SEI
// clock timestamp in an Annex-B stream.
func firstSEICountingType(t *testing.T, annexB []byte) byte {
	t.Helper()
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
			return msg.(*sei.PicTimingAvcSEI).Clocks[0].CountingType
		}
	}
	t.Fatal("no pic_timing SEI found")
	return 0
}

// TestPicTimingStartFrame verifies the -pic-timing + -start-frame timecodes:
// start-frame 76 at 25 fps yields 00:00:03:01 for the first frame, advancing.
func TestPicTimingStartFrame(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sf.264")
	if err := run([]string{appName, "-smpte", "-w", "176", "-h", "80", "-n", "3",
		"-fps", "25", "-pic-timing", "-start-frame", "76", "-o", out}); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := seiTimecodes(t, data)
	want := []string{"00:00:03:01", "00:00:03:02", "00:00:03:03"}
	if len(got) != len(want) {
		t.Fatalf("got %d SEI timecodes %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("frame %d timecode = %s, want %s", i, got[i], want[i])
		}
	}
}

// TestPicTimingSegmentsConcatenate verifies that two segments generated with
// adjacent -start-frame offsets produce a continuous timecode sequence when
// concatenated.
func TestPicTimingSegmentsConcatenate(t *testing.T) {
	dir := t.TempDir()
	s0 := filepath.Join(dir, "s0.264")
	s1 := filepath.Join(dir, "s1.264")
	mk := func(path string, start int) {
		if err := run([]string{appName, "-smpte", "-w", "176", "-h", "80", "-n", "48",
			"-fps", "25", "-pic-timing", "-start-frame", fmt.Sprint(start), "-o", path}); err != nil {
			t.Fatalf("run start=%d: %v", start, err)
		}
	}
	mk(s0, 0)
	mk(s1, 48)
	d0, _ := os.ReadFile(s0)
	d1, _ := os.ReadFile(s1)

	tc := seiTimecodes(t, append(append([]byte{}, d0...), d1...))
	if len(tc) != 96 {
		t.Fatalf("got %d timecodes, want 96", len(tc))
	}
	// The join: frame 47 -> 00:00:01:22, frame 48 -> 00:00:01:23 (continuous).
	if tc[47] != "00:00:01:22" || tc[48] != "00:00:01:23" {
		t.Errorf("join timecodes = %s,%s, want 00:00:01:22,00:00:01:23", tc[47], tc[48])
	}
}

// TestFractionalFPSMP4Timescale verifies that -fps 30000/1001 produces an MP4
// with media timescale 30000 and per-sample duration 1001.
func TestFractionalFPSMP4Timescale(t *testing.T) {
	out := filepath.Join(t.TempDir(), "f.mp4")
	if err := run([]string{appName, "-smpte", "-w", "176", "-h", "80", "-n", "3",
		"-fps", "30000/1001", "-o", out}); err != nil {
		t.Fatalf("run: %v", err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	parsed, err := mp4.DecodeFile(f)
	if err != nil {
		t.Fatalf("DecodeFile: %v", err)
	}
	ts := parsed.Init.Moov.Trak.Mdia.Mdhd.Timescale
	if ts != 30000 {
		t.Errorf("media timescale = %d, want 30000", ts)
	}
	// Each sample should last 1001 ticks.
	for _, seg := range parsed.Segments {
		for _, frag := range seg.Fragments {
			samples, err := frag.GetFullSamples(nil)
			if err != nil {
				t.Fatalf("GetFullSamples: %v", err)
			}
			for i, s := range samples {
				if s.Dur != 1001 {
					t.Errorf("sample %d dur = %d, want 1001", i, s.Dur)
				}
			}
		}
	}
}

// TestPicTimingRejectsNonH264 verifies -pic-timing is rejected for raw formats.
func TestPicTimingRejectsNonH264(t *testing.T) {
	out := filepath.Join(t.TempDir(), "x.y4m")
	err := run([]string{appName, "-smpte", "-w", "176", "-h", "80", "-n", "2",
		"-pic-timing", "-o", out})
	if err == nil || !strings.Contains(err.Error(), "pic-timing") {
		t.Fatalf("expected pic-timing rejection error, got %v", err)
	}
}

func TestParseFPS(t *testing.T) {
	ok := []struct {
		in       string
		num, den int
	}{
		{"25", 25, 1},
		{"30", 30, 1},
		{"30000/1001", 30000, 1001},
		{"29.97", 30000, 1001},
		{"59.94", 60000, 1001},
		{"23.976", 24000, 1001},
	}
	for _, c := range ok {
		n, d, err := parseFPS(c.in)
		if err != nil || n != c.num || d != c.den {
			t.Errorf("parseFPS(%q) = %d/%d, %v; want %d/%d", c.in, n, d, err, c.num, c.den)
		}
	}
	for _, bad := range []string{"abc", "30/0", "30/-1", "24.5", ""} {
		if _, _, err := parseFPS(bad); err == nil {
			t.Errorf("parseFPS(%q) expected error", bad)
		}
	}
}

// TestPicTimingDropFrame verifies 29.97 drop-frame timecodes (labels ;00 and
// ;01 skipped at the minute boundary) and that the SEI signals counting_type 4.
func TestPicTimingDropFrame(t *testing.T) {
	out := filepath.Join(t.TempDir(), "df.264")
	if err := run([]string{appName, "-smpte", "-w", "176", "-h", "80", "-n", "5",
		"-fps", "29.97", "-drop-frame", "-pic-timing", "-start-frame", "1798", "-o", out}); err != nil {
		t.Fatalf("run: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := seiTimecodes(t, data)
	want := []string{"00:00:59:28", "00:00:59:29", "00:01:00:02", "00:01:00:03", "00:01:00:04"}
	if len(got) != len(want) {
		t.Fatalf("got %d timecodes %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("frame %d timecode = %s, want %s", i, got[i], want[i])
		}
	}
	// First clock must carry NTSC drop-frame counting_type (4).
	if ct := firstSEICountingType(t, data); ct != 4 {
		t.Errorf("counting_type = %d, want 4 (drop-frame)", ct)
	}
}

// TestStartFrameNegativeRejected verifies a negative -start-frame is rejected.
func TestStartFrameNegativeRejected(t *testing.T) {
	out := filepath.Join(t.TempDir(), "x.264")
	err := run([]string{appName, "-smpte", "-w", "176", "-h", "80",
		"-start-frame", "-1", "-o", out})
	if err == nil || !strings.Contains(err.Error(), "start-frame") {
		t.Fatalf("expected start-frame rejection error, got %v", err)
	}
}
