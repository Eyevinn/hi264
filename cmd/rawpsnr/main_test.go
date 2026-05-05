package main

import (
	"bytes"
	"encoding/csv"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const appName = "rawpsnr"

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

// captureStdout redirects os.Stdout into a buffer for the duration of the test
// and returns a function that restores stdout and yields what was written.
func captureStdout(t *testing.T) func() string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	return func() string {
		_ = w.Close()
		<-done
		os.Stdout = orig
		return buf.String()
	}
}

// writeYUV writes a YUV420 buffer of the given dimensions filled with `fill`
// for every byte and returns its path.
func writeYUV(t *testing.T, dir, name string, w, h, frames int, fill byte) string {
	t.Helper()
	frameSize := w*h + w*h/2
	buf := make([]byte, frameSize*frames)
	for i := range buf {
		buf[i] = fill
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf, 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestRunMissingDimensions(t *testing.T) {
	silenceStdout(t)
	dir := t.TempDir()
	a := writeYUV(t, dir, "a.yuv", 16, 16, 1, 100)
	b := writeYUV(t, dir, "b.yuv", 16, 16, 1, 100)

	err := run([]string{appName, a, b})
	if err == nil {
		t.Fatal("expected error for missing -w/-h")
	}
	if !strings.Contains(err.Error(), "-w and -h are required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunMissingFileArgs(t *testing.T) {
	silenceStdout(t)
	err := run([]string{appName, "-w", "16", "-h", "16"})
	if err == nil {
		t.Fatal("expected error for missing file args")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunSizeMismatch(t *testing.T) {
	silenceStdout(t)
	dir := t.TempDir()
	a := writeYUV(t, dir, "a.yuv", 16, 16, 1, 100)
	b := writeYUV(t, dir, "b.yuv", 16, 16, 2, 100)

	err := run([]string{appName, "-w", "16", "-h", "16", a, b})
	if err == nil || !strings.Contains(err.Error(), "file sizes differ") {
		t.Fatalf("expected size-mismatch error, got %v", err)
	}
}

func TestRunBadFrameSize(t *testing.T) {
	silenceStdout(t)
	dir := t.TempDir()
	// Files with size that isn't a multiple of 32x32 YUV420 frame size (1536).
	path := filepath.Join(dir, "odd.yuv")
	if err := os.WriteFile(path, make([]byte, 1000), 0644); err != nil {
		t.Fatal(err)
	}

	err := run([]string{appName, "-w", "32", "-h", "32", path, path})
	if err == nil || !strings.Contains(err.Error(), "not a multiple of frame size") {
		t.Fatalf("expected frame-size error, got %v", err)
	}
}

func TestRunNonexistentFile(t *testing.T) {
	silenceStdout(t)
	dir := t.TempDir()
	a := writeYUV(t, dir, "a.yuv", 16, 16, 1, 100)
	missing := filepath.Join(dir, "does-not-exist.yuv")

	err := run([]string{appName, "-w", "16", "-h", "16", a, missing})
	if err == nil || !strings.Contains(err.Error(), "reading") {
		t.Fatalf("expected read error, got %v", err)
	}
}

func TestRunIdenticalFiles(t *testing.T) {
	silenceStdout(t)
	dir := t.TempDir()
	a := writeYUV(t, dir, "a.yuv", 32, 16, 1, 128)
	b := writeYUV(t, dir, "b.yuv", 32, 16, 1, 128)

	if err := run([]string{appName, "-w", "32", "-h", "16", a, b}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunDifferentContent(t *testing.T) {
	silenceStdout(t)
	dir := t.TempDir()
	a := writeYUV(t, dir, "a.yuv", 32, 16, 1, 100)
	b := writeYUV(t, dir, "b.yuv", 32, 16, 1, 110)

	if err := run([]string{appName, "-w", "32", "-h", "16", a, b}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunMultiFrame(t *testing.T) {
	silenceStdout(t)
	dir := t.TempDir()
	a := writeYUV(t, dir, "a.yuv", 32, 16, 3, 100)
	b := writeYUV(t, dir, "b.yuv", 32, 16, 3, 110)

	if err := run([]string{appName, "-w", "32", "-h", "16", a, b}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunPerMB(t *testing.T) {
	silenceStdout(t)
	dir := t.TempDir()
	a := writeYUV(t, dir, "a.yuv", 32, 32, 1, 100)
	b := writeYUV(t, dir, "b.yuv", 32, 32, 1, 110)

	if err := run([]string{appName, "-w", "32", "-h", "32", "-per-mb", a, b}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestRunCSVOutput(t *testing.T) {
	silenceStdout(t)
	dir := t.TempDir()
	// 48x32 → 3x2 = 6 macroblocks, 2 frames → 12 data rows + 1 header.
	a := writeYUV(t, dir, "a.yuv", 48, 32, 2, 100)
	b := writeYUV(t, dir, "b.yuv", 48, 32, 2, 110)
	csvPath := filepath.Join(dir, "out.csv")

	if err := run([]string{appName, "-w", "48", "-h", "32", "-csv", csvPath, a, b}); err != nil {
		t.Fatalf("run: %v", err)
	}

	cf, err := os.Open(csvPath)
	if err != nil {
		t.Fatalf("open csv: %v", err)
	}
	defer cf.Close()
	rows, err := csv.NewReader(cf).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	if len(rows) != 13 {
		t.Fatalf("got %d rows, want 13 (1 header + 12 MBs)", len(rows))
	}
	wantHeader := []string{"frame", "mb_x", "mb_y", "psnr_y"}
	for i, c := range wantHeader {
		if rows[0][i] != c {
			t.Errorf("header[%d] = %q, want %q", i, rows[0][i], c)
		}
	}
}

func TestRunCSVCreateError(t *testing.T) {
	silenceStdout(t)
	dir := t.TempDir()
	a := writeYUV(t, dir, "a.yuv", 16, 16, 1, 100)
	b := writeYUV(t, dir, "b.yuv", 16, 16, 1, 110)
	// Path under a non-existent directory: os.Create will fail.
	bogus := filepath.Join(dir, "no-such-dir", "out.csv")

	err := run([]string{appName, "-w", "16", "-h", "16", "-csv", bogus, a, b})
	if err == nil {
		t.Fatal("expected error creating csv in missing directory")
	}
}

// expectedPSNR returns the analytical PSNR for arrays whose bytes differ by a
// constant `delta` (so MSE = delta²).
func expectedPSNR(delta float64) float64 {
	if delta == 0 {
		return math.Inf(1)
	}
	mse := delta * delta
	return 10.0 * math.Log10(255.0*255.0/mse)
}

func TestComputePSNRKnownValues(t *testing.T) {
	cases := []struct {
		name  string
		delta byte
	}{
		{"identical", 0},
		{"diff1", 1},   // MSE=1   → ~48.13 dB
		{"diff10", 10}, // MSE=100 → ~28.13 dB
		{"diff50", 50}, // MSE=2500 → ~14.15 dB
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := bytes.Repeat([]byte{100}, 256)
			b := bytes.Repeat([]byte{100 + tc.delta}, 256)
			got := computePSNR(a, b)
			want := expectedPSNR(float64(tc.delta))
			if math.IsInf(want, 1) {
				if !math.IsInf(got, 1) {
					t.Errorf("got %v, want +Inf", got)
				}
				return
			}
			if math.Abs(got-want) > 1e-9 {
				t.Errorf("got %.6f dB, want %.6f dB", got, want)
			}
		})
	}
}

func TestComputePSNRMixedDeltas(t *testing.T) {
	// 4 samples: deltas 0,1,2,3 → MSE = (0+1+4+9)/4 = 3.5
	a := []byte{50, 50, 50, 50}
	b := []byte{50, 51, 52, 53}
	mse := (0.0 + 1.0 + 4.0 + 9.0) / 4.0
	want := 10.0 * math.Log10(255.0*255.0/mse)
	got := computePSNR(a, b)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %.6f dB, want %.6f dB", got, want)
	}
}

func TestBlockPSNRClampsAtEdges(t *testing.T) {
	// 18x18 frame, request a 16x16 block at (16,16): only a 2x2 region is in
	// bounds (x=16..17, y=16..17), so blockPSNR should average over those 4
	// samples only.
	w, h := 18, 18
	a := make([]byte, w*h)
	b := make([]byte, w*h)
	for i := range a {
		a[i] = 100
		b[i] = 100
	}
	// Set delta=10 only inside the in-bounds 2x2 region.
	for y := 16; y < 18; y++ {
		for x := 16; x < 18; x++ {
			b[y*w+x] = 110
		}
	}
	want := 10.0 * math.Log10(255.0*255.0/100.0) // MSE=100
	got := blockPSNR(a, b, w, h, 16, 16, 16)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %.6f dB, want %.6f dB", got, want)
	}
}

func TestBlockPSNRIdenticalIsInf(t *testing.T) {
	a := bytes.Repeat([]byte{42}, 32*32)
	b := bytes.Repeat([]byte{42}, 32*32)
	if got := blockPSNR(a, b, 32, 32, 0, 0, 16); !math.IsInf(got, 1) {
		t.Errorf("got %v, want +Inf", got)
	}
}

// TestRunPrintedPSNRValue captures stdout while running on inputs whose every
// byte differs by 10 (MSE=100) and verifies the printed Y/Cb/Cr/Avg numbers
// match the analytical PSNR ≈ 28.13 dB.
func TestRunPrintedPSNRValue(t *testing.T) {
	dir := t.TempDir()
	a := writeYUV(t, dir, "a.yuv", 32, 16, 1, 100)
	b := writeYUV(t, dir, "b.yuv", 32, 16, 1, 110)

	restore := captureStdout(t)
	if err := run([]string{appName, "-w", "32", "-h", "16", a, b}); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := restore()

	// "Y=28.13 dB  Cb=28.13 dB  Cr=28.13 dB  Avg=28.13 dB  (1 frame(s), 32x16)"
	want := expectedPSNR(10) // ~28.1308 dB
	re := regexp.MustCompile(`Y=([0-9.]+) dB\s+Cb=([0-9.]+) dB\s+Cr=([0-9.]+) dB\s+Avg=([0-9.]+) dB`)
	m := re.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("could not parse PSNR line from output:\n%s", out)
	}
	for i, label := range []string{"Y", "Cb", "Cr", "Avg"} {
		got, err := strconv.ParseFloat(m[i+1], 64)
		if err != nil {
			t.Fatalf("parse %s: %v", label, err)
		}
		// The program prints with %.2f, so allow 0.01 dB rounding tolerance.
		if math.Abs(got-want) > 0.01 {
			t.Errorf("%s = %.2f dB, want %.2f dB", label, got, want)
		}
	}
}

// TestRunCSVPSNRValue verifies the per-MB PSNR written to CSV matches the
// analytical value when every byte differs by a constant.
func TestRunCSVPSNRValue(t *testing.T) {
	silenceStdout(t)
	dir := t.TempDir()
	a := writeYUV(t, dir, "a.yuv", 32, 32, 1, 100)
	b := writeYUV(t, dir, "b.yuv", 32, 32, 1, 110) // delta=10 everywhere
	csvPath := filepath.Join(dir, "out.csv")

	if err := run([]string{appName, "-w", "32", "-h", "32", "-csv", csvPath, a, b}); err != nil {
		t.Fatalf("run: %v", err)
	}

	cf, err := os.Open(csvPath)
	if err != nil {
		t.Fatalf("open csv: %v", err)
	}
	defer cf.Close()
	rows, err := csv.NewReader(cf).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}
	// 32x32 → 4 macroblocks plus header row.
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5", len(rows))
	}
	want := expectedPSNR(10)
	for _, r := range rows[1:] {
		got, err := strconv.ParseFloat(r[3], 64)
		if err != nil {
			t.Fatalf("parse %q: %v", r[3], err)
		}
		if math.Abs(got-want) > 0.01 {
			t.Errorf("MB(%s,%s): got %.2f dB, want %.2f dB", r[1], r[2], got, want)
		}
	}
}

func TestRunNonMacroblockAlignedDimensions(t *testing.T) {
	silenceStdout(t)
	dir := t.TempDir()
	// 18x18 isn't a multiple of 16; per-MB loop must clamp at edges.
	// YUV420 requires even dims for chroma — use 18x18 (even). Frame size = 486.
	a := writeYUV(t, dir, "a.yuv", 18, 18, 1, 100)
	b := writeYUV(t, dir, "b.yuv", 18, 18, 1, 110)

	if err := run([]string{appName, "-w", "18", "-h", "18", "-per-mb", a, b}); err != nil {
		t.Fatalf("run: %v", err)
	}
}
