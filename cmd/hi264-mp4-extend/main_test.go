package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Eyevinn/mp4ff/mp4"
)

const testInit = "../../testdata/init.mp4"
const testSeg = "../../testdata/seg1s.m4s"

// silenceStdout redirects os.Stdout to discard for the duration of the test.
func silenceStdout(t *testing.T) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, r)
		close(done)
	}()
	t.Cleanup(func() {
		_ = w.Close()
		<-done
		os.Stdout = orig
	})
}

func TestRunMissingArgs(t *testing.T) {
	silenceStdout(t)
	err := run([]string{"hi264-mp4-extend", "-frames", "1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "3 positional arguments") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunMissingFramesFlag(t *testing.T) {
	silenceStdout(t)
	err := run([]string{
		"hi264-mp4-extend", testInit, testSeg, t.TempDir() + "/out.m4s",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "-frames") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunNonexistentInit(t *testing.T) {
	silenceStdout(t)
	err := run([]string{
		"hi264-mp4-extend", "-frames", "1",
		"/nonexistent/init.mp4", testSeg, t.TempDir() + "/out.m4s",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "init segment") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestExtendPSkip uses the testdata fragmented MP4 (1280×720, 25 frames,
// POC type 2, weighted_pred=1, CABAC) and appends 25 P_Skip samples.
// Verifies the output trun lists 50 samples and the frame data is preserved.
func TestExtendPSkip(t *testing.T) {
	silenceStdout(t)
	out := filepath.Join(t.TempDir(), "out.m4s")
	if err := extendSegment(testInit, testSeg, out, 25, false); err != nil {
		t.Fatalf("extendSegment: %v", err)
	}
	samples := readTrunSamples(t, out)
	if got := len(samples); got != 50 {
		t.Fatalf("output trun sampleCount = %d, want 50", got)
	}

	// Sample 0 is the original IDR (sync flag).
	if !mp4.IsSyncSampleFlags(samples[0].Flags) {
		t.Errorf("sample 0 is non-sync, want sync (IDR)")
	}
	// Samples 25..49 are appended P_Skips (non-sync).
	for i := 25; i < 50; i++ {
		if mp4.IsSyncSampleFlags(samples[i].Flags) {
			t.Errorf("appended sample %d is sync, want non-sync (P_Skip)", i)
		}
	}

	// All sample durations should match the source's per-sample duration.
	want := samples[0].Dur
	for i, s := range samples {
		if s.Dur != want {
			t.Errorf("sample %d dur=%d, want %d", i, s.Dur, want)
		}
	}
}

// TestExtendBlackIDR appends 5 frames where the first is a black IDR.
// Verifies the appended IDR has the sync-sample flag.
func TestExtendBlackIDR(t *testing.T) {
	silenceStdout(t)
	out := filepath.Join(t.TempDir(), "out.m4s")
	if err := extendSegment(testInit, testSeg, out, 5, true); err != nil {
		t.Fatalf("extendSegment: %v", err)
	}
	samples := readTrunSamples(t, out)
	if got := len(samples); got != 30 {
		t.Fatalf("sampleCount = %d, want 30", got)
	}
	// Sample 25 is the black IDR — should be a sync sample.
	if !mp4.IsSyncSampleFlags(samples[25].Flags) {
		t.Errorf("appended IDR (sample 25) is non-sync, want sync")
	}
	// Samples 26..29 are P_Skips.
	for i := 26; i < 30; i++ {
		if mp4.IsSyncSampleFlags(samples[i].Flags) {
			t.Errorf("sample %d is sync, want non-sync", i)
		}
	}
}

// TestExtendPreservesOriginalSamples verifies that the original samples
// are written to the new mdat byte-for-byte (regression for the
// ConvertSampleToByteStream in-place mutation bug).
func TestExtendPreservesOriginalSamples(t *testing.T) {
	silenceStdout(t)
	out := filepath.Join(t.TempDir(), "out.m4s")
	if err := extendSegment(testInit, testSeg, out, 1, false); err != nil {
		t.Fatalf("extendSegment: %v", err)
	}
	origSamples := readSampleData(t, testSeg)
	outSamples := readSampleData(t, out)
	if len(outSamples) < len(origSamples) {
		t.Fatalf("output has %d samples, want at least %d", len(outSamples), len(origSamples))
	}
	for i, want := range origSamples {
		got := outSamples[i]
		if !bytes.Equal(got, want) {
			t.Fatalf("sample %d differs (got %d bytes, want %d, first byte got=%x want=%x)",
				i, len(got), len(want), got[0], want[0])
		}
	}
}

// TestExtendDecodeTimeContinuation verifies the appended samples' decode
// times start where the source's last sample ended, with no gap.
func TestExtendDecodeTimeContinuation(t *testing.T) {
	silenceStdout(t)
	out := filepath.Join(t.TempDir(), "out.m4s")
	if err := extendSegment(testInit, testSeg, out, 5, false); err != nil {
		t.Fatalf("extendSegment: %v", err)
	}
	samples := readTrunSamples(t, out)
	// Source is 25 samples each with dur=512 at 25 fps, so total source dur = 12800.
	// Appended sample 25 has DecodeTime baked in via tfdt+offset; mp4ff exposes
	// it through GetFullSamples.
	full := readFullSamples(t, out)
	if len(full) != 30 {
		t.Fatalf("got %d samples, want 30", len(full))
	}
	dur := uint64(samples[0].Dur)
	for i := 1; i < len(full); i++ {
		want := full[i-1].DecodeTime + dur
		if full[i].DecodeTime != want {
			t.Errorf("sample %d DecodeTime=%d, want %d (gap of %d)",
				i, full[i].DecodeTime, want, int64(full[i].DecodeTime)-int64(want))
		}
	}
}

// readTrunSamples returns the trun's Sample entries from a .m4s file.
func readTrunSamples(t *testing.T, path string) []mp4.Sample {
	t.Helper()
	parsed := decodeForTest(t, path)
	var out []mp4.Sample
	for _, seg := range parsed.Segments {
		for _, frag := range seg.Fragments {
			for _, trun := range frag.Moof.Traf.Truns {
				out = append(out, trun.Samples...)
			}
		}
	}
	return out
}

func readFullSamples(t *testing.T, path string) []mp4.FullSample {
	t.Helper()
	parsed := decodeForTest(t, path)
	var out []mp4.FullSample
	for _, seg := range parsed.Segments {
		for _, frag := range seg.Fragments {
			samples, err := frag.GetFullSamples(nil)
			if err != nil {
				t.Fatalf("GetFullSamples: %v", err)
			}
			out = append(out, samples...)
		}
	}
	return out
}

// readSampleData returns each sample's raw byte slice (length-prefixed NALUs).
func readSampleData(t *testing.T, path string) [][]byte {
	t.Helper()
	full := readFullSamples(t, path)
	out := make([][]byte, len(full))
	for i, s := range full {
		out[i] = append([]byte(nil), s.Data...)
	}
	return out
}

func decodeForTest(t *testing.T, path string) *mp4.File {
	t.Helper()
	parsed, err := decodeFile(path)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return parsed
}
