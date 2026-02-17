package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunH264Output(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	err := run([]string{appName, "-w", "176", "-h", "80", "-n", "3", "-text", "%03d", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	// Should start with Annex-B start code
	if !bytes.HasPrefix(data, []byte{0, 0, 0, 1}) {
		t.Error("output should start with Annex-B start code")
	}

	// Count start codes (SPS + PPS + 3 IDR slices = 5)
	startCodes := countStartCodes(data)
	if startCodes != 5 {
		t.Errorf("expected 5 NALUs (SPS+PPS+3 slices), got %d", startCodes)
	}
}

func TestRunH264CABAC(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test_cabac.264")

	err := run([]string{appName, "-w", "176", "-h", "80", "-n", "2", "-text", "%03d", "-cabac", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	startCodes := countStartCodes(data)
	if startCodes != 4 {
		t.Errorf("expected 4 NALUs (SPS+PPS+2 slices), got %d", startCodes)
	}
}

func TestRunY4MOutput(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.y4m")

	err := run([]string{appName, "-w", "176", "-h", "80", "-n", "3", "-text", "%03d", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	// Should start with Y4M header
	if !bytes.HasPrefix(data, []byte("YUV4MPEG2")) {
		t.Error("Y4M output should start with YUV4MPEG2 header")
	}

	// Count FRAME tags
	frameCount := strings.Count(string(data), "FRAME\n")
	if frameCount != 3 {
		t.Errorf("expected 3 FRAME tags, got %d", frameCount)
	}
}

func TestRunPNGOutput(t *testing.T) {
	dir := t.TempDir()
	pattern := filepath.Join(dir, "frame_%03d.png")

	err := run([]string{appName, "-w", "48", "-h", "80", "-n", "2", "-text", "%d", "-o", pattern})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Check both PNG files exist
	for i := range 2 {
		path := filepath.Join(dir, fmt.Sprintf("frame_%03d.png", i))
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("frame %d: %v", i, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("frame %d: empty file", i)
		}
	}
}

func TestRunMissingOutput(t *testing.T) {
	err := run([]string{appName, "-w", "176", "-h", "80", "-text", "%03d"})
	if err == nil {
		t.Error("expected error for missing -o")
	}
}

func TestRunInvalidWidth(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	err := run([]string{appName, "-w", "101", "-h", "80", "-text", "%03d", "-o", out})
	if err == nil {
		t.Error("expected error for odd width")
	}
}

func TestRunNon16MultipleWidth(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	err := run([]string{appName, "-w", "100", "-h", "100", "-n", "2", "-text", "%d", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	// Should start with Annex-B start code
	if !bytes.HasPrefix(data, []byte{0, 0, 0, 1}) {
		t.Error("output should start with Annex-B start code")
	}

	// SPS + PPS + 2 IDR slices = 4 NALUs
	startCodes := countStartCodes(data)
	if startCodes != 4 {
		t.Errorf("expected 4 NALUs, got %d", startCodes)
	}
}

func TestRunY4MNon16Multiple(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.y4m")

	err := run([]string{appName, "-w", "100", "-h", "100", "-n", "2", "-text", "%d", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	// Y4M header should have actual (non-coded) dimensions
	header := string(data[:bytes.IndexByte(data, '\n')])
	if !strings.Contains(header, "W100") || !strings.Contains(header, "H100") {
		t.Errorf("Y4M header should contain W100 H100, got: %s", header)
	}

	frameCount := strings.Count(string(data), "FRAME\n")
	if frameCount != 2 {
		t.Errorf("expected 2 FRAME tags, got %d", frameCount)
	}
}

func TestRunFrameTooSmall(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	// 3 digits needs 11x5 MBs = 176x80, but we give 48x80 (3 MBs wide)
	err := run([]string{appName, "-w", "48", "-h", "80", "-text", "%03d", "-o", out})
	if err == nil {
		t.Error("expected error for frame too small for text")
	}
}

func TestRunIDRInterval(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	// 10 frames with IDR every 5: frames 0,5 are IDR, frames 1-4,6-9 are P_Skip
	err := run([]string{appName, "-w", "176", "-h", "80", "-n", "10", "-text", "%03d",
		"-idr-interval", "5", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	// Count NALU types
	naluTypes := countNALUTypes(data)
	// Expect: SPS(1) + PPS(1) + IDR(2) + non-IDR P-slices(8) = 12 NALUs total
	if naluTypes[7] != 1 {
		t.Errorf("expected 1 SPS NALU, got %d", naluTypes[7])
	}
	if naluTypes[8] != 1 {
		t.Errorf("expected 1 PPS NALU, got %d", naluTypes[8])
	}
	if naluTypes[5] != 2 {
		t.Errorf("expected 2 IDR NALUs, got %d", naluTypes[5])
	}
	if naluTypes[1] != 8 {
		t.Errorf("expected 8 non-IDR NALUs, got %d", naluTypes[1])
	}
}

func TestRunIDRIntervalSmall(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	// 3 frames with IDR every 3: frame 0 is IDR, frames 1-2 are P_Skip
	err := run([]string{appName, "-w", "176", "-h", "80", "-n", "3", "-text", "%03d",
		"-idr-interval", "3", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	naluTypes := countNALUTypes(data)
	if naluTypes[5] != 1 {
		t.Errorf("expected 1 IDR NALU, got %d", naluTypes[5])
	}
	if naluTypes[1] != 2 {
		t.Errorf("expected 2 non-IDR NALUs, got %d", naluTypes[1])
	}
}

func TestRunMP4Output(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.mp4")

	err := run([]string{appName, "-w", "176", "-h", "80", "-n", "3", "-text", "%03d", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	// Should start with ftyp box
	if len(data) < 8 || string(data[4:8]) != "ftyp" {
		t.Error("MP4 output should start with ftyp box")
	}
}

func TestRunMP4WithPSkip(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.mp4")

	err := run([]string{appName, "-w", "176", "-h", "80", "-n", "10", "-text", "%03d",
		"-idr-interval", "5", "-fps", "30", "-frag-dur", "5", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	if len(data) < 8 || string(data[4:8]) != "ftyp" {
		t.Error("MP4 output should start with ftyp box")
	}

	// Should have moof boxes (fragmented MP4)
	moofCount := countBoxType(data, "moof")
	// 10 frames / 5 per fragment = 2 fragments
	if moofCount != 2 {
		t.Errorf("expected 2 moof boxes, got %d", moofCount)
	}
}

// countBoxType counts occurrences of a 4-byte box type in MP4 data.
func countBoxType(data []byte, boxType string) int {
	bt := []byte(boxType)
	count := 0
	for i := 4; i <= len(data)-4; i++ {
		if data[i] == bt[0] && data[i+1] == bt[1] && data[i+2] == bt[2] && data[i+3] == bt[3] {
			count++
		}
	}
	return count
}

func TestRunIDRIntervalCABAC(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	err := run([]string{appName, "-w", "176", "-h", "80", "-n", "5", "-text", "%03d",
		"-cabac", "-idr-interval", "3", "-o", out})
	if err != nil {
		t.Fatalf("unexpected error for CABAC + idr-interval: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	naluTypes := countNALUTypes(data)
	// 5 frames with idr-interval=3: IDR at 0,3 → 2 IDR (type 5), P_Skip at 1,2,4 → 3 non-IDR (type 1)
	if naluTypes[5] != 2 {
		t.Errorf("expected 2 IDR NALUs, got %d", naluTypes[5])
	}
	if naluTypes[1] != 3 {
		t.Errorf("expected 3 non-IDR NALUs, got %d", naluTypes[1])
	}
}

// countNALUTypes counts NALUs by type in an Annex-B stream.
func countNALUTypes(data []byte) map[int]int {
	types := make(map[int]int)
	for i := 0; i <= len(data)-4; i++ {
		if data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
			if i+4 < len(data) {
				naluType := int(data[i+4] & 0x1f)
				types[naluType]++
			}
		}
	}
	return types
}

func TestRunSMPTE(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "smpte.264")

	err := run([]string{appName, "-smpte", "-w", "176", "-h", "80", "-n", "2", "-text", "%03d", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.HasPrefix(data, []byte{0, 0, 0, 1}) {
		t.Error("output should start with Annex-B start code")
	}
	startCodes := countStartCodes(data)
	if startCodes != 4 { // SPS + PPS + 2 IDR slices
		t.Errorf("expected 4 NALUs, got %d", startCodes)
	}
}

func TestRunSMPTEExclusive(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	// -smpte with -grid should error
	err := run([]string{appName, "-smpte", "-grid", "xy", "-w", "176", "-h", "80", "-text", "%d", "-o", out})
	if err == nil {
		t.Error("expected error for -smpte with -grid")
	}
}

func TestRunTextBg(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	err := run([]string{appName, "-w", "176", "-h", "80", "-n", "1", "-text", "%03d",
		"-text-bg", "128,128,128", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.HasPrefix(data, []byte{0, 0, 0, 1}) {
		t.Error("output should start with Annex-B start code")
	}
}

func TestRunTextScale(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	err := run([]string{appName, "-w", "352", "-h", "288", "-n", "1", "-text", "%02d",
		"-text-scale", "3", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.HasPrefix(data, []byte{0, 0, 0, 1}) {
		t.Error("output should start with Annex-B start code")
	}
}

func TestRunSMPTENon16Multiple(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "smpte.264")

	// 100x100 is not a multiple of 16 (rounds to 7x7 MBs = 112x112 coded)
	err := run([]string{appName, "-smpte", "-w", "100", "-h", "100", "-n", "1", "-text", "%02d", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.HasPrefix(data, []byte{0, 0, 0, 1}) {
		t.Error("output should start with Annex-B start code")
	}
}

func TestRunTextBgNon16Multiple(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	// 100x100 with text-bg
	err := run([]string{appName, "-w", "100", "-h", "100", "-n", "1", "-text", "%02d",
		"-text-bg", "64,64,64", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.HasPrefix(data, []byte{0, 0, 0, 1}) {
		t.Error("output should start with Annex-B start code")
	}
}

func TestRunTextScaleNon16Multiple(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	// 300x200: 19x13 MBs, 2 digits at auto scale
	err := run([]string{appName, "-w", "300", "-h", "200", "-n", "1", "-text", "%02d", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.HasPrefix(data, []byte{0, 0, 0, 1}) {
		t.Error("output should start with Annex-B start code")
	}
}

func TestRunBackgroundPattern(t *testing.T) {
	dir := t.TempDir()

	// Create a 2x2 checkerboard pattern file
	patternFile := filepath.Join(dir, "checker.gridimg")
	patternContent := "@rgb\nR=255,0,0\nB=0,0,255\n\nRB\nBR\n"
	if err := os.WriteFile(patternFile, []byte(patternContent), 0644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "test.264")
	err := run([]string{appName, "-w", "176", "-h", "80", "-n", "3", "-text", "%03d",
		"-f", patternFile, "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	// Should start with Annex-B start code
	if !bytes.HasPrefix(data, []byte{0, 0, 0, 1}) {
		t.Error("output should start with Annex-B start code")
	}

	// SPS + PPS + 3 IDR slices = 5 NALUs
	startCodes := countStartCodes(data)
	if startCodes != 5 {
		t.Errorf("expected 5 NALUs (SPS+PPS+3 slices), got %d", startCodes)
	}
}

func TestRunGridOnly(t *testing.T) {
	dir := t.TempDir()

	// Create a simple grid pattern file
	patternFile := filepath.Join(dir, "test.gridimg")
	patternContent := "@rgb\nR=255,0,0\nB=0,0,255\n\nRB\nBR\n"
	if err := os.WriteFile(patternFile, []byte(patternContent), 0644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "test.264")
	// Grid-only mode: no -w/-h, frame size = 2x2 MBs = 32x32
	err := run([]string{appName, "-f", patternFile, "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.HasPrefix(data, []byte{0, 0, 0, 1}) {
		t.Error("output should start with Annex-B start code")
	}

	// SPS + PPS + 1 IDR slice = 3 NALUs (default -n 1)
	startCodes := countStartCodes(data)
	if startCodes != 3 {
		t.Errorf("expected 3 NALUs (SPS+PPS+1 slice), got %d", startCodes)
	}
}

func TestRunGridOnlyCABAC(t *testing.T) {
	dir := t.TempDir()

	patternFile := filepath.Join(dir, "test.gridimg")
	patternContent := "@rgb\nR=255,0,0\nB=0,0,255\n\nRB\nBR\n"
	if err := os.WriteFile(patternFile, []byte(patternContent), 0644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "test.264")
	err := run([]string{appName, "-f", patternFile, "-cabac", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	startCodes := countStartCodes(data)
	if startCodes != 3 {
		t.Errorf("expected 3 NALUs (SPS+PPS+1 slice), got %d", startCodes)
	}
}

func TestRunGridOnlyWithFlags(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	// Grid-only mode using -grid/-c flags
	err := run([]string{appName, "-grid", "xy,yx", "-c", "x=235,128,128", "-c", "y=16,128,128", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.HasPrefix(data, []byte{0, 0, 0, 1}) {
		t.Error("output should start with Annex-B start code")
	}

	// SPS + PPS + 1 IDR slice = 3 NALUs
	startCodes := countStartCodes(data)
	if startCodes != 3 {
		t.Errorf("expected 3 NALUs (SPS+PPS+1 slice), got %d", startCodes)
	}
}

func TestRunNoInputError(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	// No grid input and no -w/-h
	err := run([]string{appName, "-o", out})
	if err == nil {
		t.Error("expected error when no grid input and no -w/-h")
	}
}

func TestRunTextWithoutTextError(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	// -w/-h without -text and no grid input
	err := run([]string{appName, "-w", "176", "-h", "80", "-o", out})
	if err == nil {
		t.Error("expected error when using -w/-h without -text and no grid input")
	}
}

func TestRunYUVOutput(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.yuv")

	err := run([]string{appName, "-w", "176", "-h", "80", "-n", "2", "-text", "%03d", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Output file gets _WxH_yuv420p suffix
	outPath := filepath.Join(dir, "test_176x80_yuv420p.yuv")
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	// YUV420: W*H * 3/2 per frame, 2 frames
	expectedSize := int64(176 * 80 * 3 / 2 * 2)
	if info.Size() != expectedSize {
		t.Errorf("file size %d, want %d", info.Size(), expectedSize)
	}
}

func TestRunNumberedPNG(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "frame.png")

	err := run([]string{appName, "-w", "48", "-h", "80", "-n", "3", "-text", "%d", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// With -n > 1, files are numbered: frame_0000.png, frame_0001.png, frame_0002.png
	for i := range 3 {
		path := filepath.Join(dir, fmt.Sprintf("frame_%04d.png", i))
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("frame %d: %v", i, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("frame %d: empty file", i)
		}
	}
}

func TestRunNumberedJPEG(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "frame.jpg")

	err := run([]string{appName, "-w", "48", "-h", "80", "-n", "2", "-text", "%d", "-q", "90", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	for i := range 2 {
		path := filepath.Join(dir, fmt.Sprintf("frame_%04d.jpg", i))
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("frame %d: %v", i, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("frame %d: empty file", i)
		}
	}
}

func TestRunSinglePNG(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "frame.png")

	// Single frame (-n 1 default) should produce frame.png without numbering
	err := run([]string{appName, "-w", "48", "-h", "80", "-text", "%d", "-o", out})
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("empty file")
	}
}

func TestRunSMPTEWithFileExclusive(t *testing.T) {
	dir := t.TempDir()
	patternFile := filepath.Join(dir, "test.gridimg")
	if err := os.WriteFile(patternFile, []byte("@rgb\nR=255,0,0\n\nR\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "test.264")

	err := run([]string{appName, "-smpte", "-f", patternFile, "-w", "176", "-h", "80", "-text", "%d", "-o", out})
	if err == nil {
		t.Error("expected error for -smpte with -f")
	}
}

func TestRunValidationErrors(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "test.264")

	// b returns base args with extra flags appended.
	b := func(extra ...string) []string {
		base := []string{
			appName, "-w", "176", "-h", "80", "-text", "%d", "-o", out,
		}
		return append(base, extra...)
	}
	bmp := filepath.Join(dir, "test.bmp")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"odd width", b("-w", "101"), "positive even"},
		{"zero width", b("-w", "0"), "positive even"},
		{"odd height", b("-h", "81"), "positive even"},
		{"zero height", b("-h", "0"), "positive even"},
		{"negative frames", b("-n", "-1"), "positive"},
		{"qp too low", b("-qp", "-1"), "QP must be 0-51"},
		{"qp too high", b("-qp", "52"), "QP must be 0-51"},
		{"negative idr-interval",
			b("-idr-interval", "-1"), "non-negative"},
		{"negative bpp", b("-bpp", "-1"), "non-negative"},
		{"negative kbps", b("-kbps", "-1"), "non-negative"},
		{"bpp and kbps", b("-bpp", "5000", "-kbps", "1000"), "mutually exclusive"},
		{"zero fps", b("-fps", "0"), "fps must be positive"},
		{"zero frag-dur",
			b("-frag-dur", "0"), "frag-dur must be positive"},
		{"invalid colorspace",
			b("-colorspace", "bt420"), "bt420"},
		{"unknown output format", []string{
			appName, "-w", "176", "-h", "80",
			"-text", "%d", "-o", bmp,
		}, "unknown output format"},
		{"invalid fg", b("-fg", "abc"), "expected R,G,B"},
		{"invalid bg", b("-bg", "1,2"), "expected R,G,B"},
		{"f with grid", []string{
			appName, "-f", "x.gridimg", "-grid", "xy", "-o", out,
		}, "mutually exclusive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(tt.args)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q should contain %q", err, tt.want)
			}
		})
	}
}

func countStartCodes(data []byte) int {
	count := 0
	for i := 0; i <= len(data)-4; i++ {
		if data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
			count++
		}
	}
	return count
}
