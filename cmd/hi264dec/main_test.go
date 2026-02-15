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

	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/hi264/pkg/yuv"
)

// generateTestBitstream creates a valid H.264 Annex-B bitstream and returns its path.
func generateTestBitstream(t *testing.T, dir string) string {
	t.Helper()
	grid, err := yuv.ParseGrid("xy,yx")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 235, Cb: 128, Cr: 128},
		'y': {Y: 16, Cb: 128, Cr: 128},
	}
	enc := &encode.FrameEncoder{Grid: grid, Colors: colors, QP: 26, Width: 32, Height: 32}
	bs, err := enc.Encode()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "test.264")
	if err := os.WriteFile(path, bs, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// generateTestMP4 creates a fragmented MP4 file and returns its path.
func generateTestMP4(t *testing.T, dir string) string {
	t.Helper()

	spsRBSP := encode.EncodeSPS(32, 32, 0, 0, 0)
	ppsRBSP := encode.EncodePPS(0)
	spsNALU := encode.BuildNALU(7, 3, spsRBSP)
	ppsNALU := encode.BuildNALU(8, 3, ppsRBSP)

	grid, err := yuv.ParseGrid("xy,yx")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 235, Cb: 128, Cr: 128},
		'y': {Y: 16, Cb: 128, Cr: 128},
	}
	enc := &encode.FrameEncoder{Grid: grid, Colors: colors, QP: 26, Width: 32, Height: 32}
	sliceAnnexB, err := enc.EncodeSlice(0)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "test.mp4")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Init segment
	init := mp4.CreateEmptyInit()
	init.AddEmptyTrack(25, "video", "und")
	trak := init.Moov.Trak
	if err := trak.SetAVCDescriptor("avc1", [][]byte{spsNALU}, [][]byte{ppsNALU}, true); err != nil {
		t.Fatal(err)
	}
	if err := init.Encode(f); err != nil {
		t.Fatal(err)
	}

	// Media segment with one IDR sample
	sampleData := avc.ConvertByteStreamToNaluSample(sliceAnnexB)
	frag, err := mp4.CreateFragment(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	frag.AddFullSample(mp4.FullSample{
		Sample: mp4.Sample{
			Flags:                 mp4.SyncSampleFlags,
			Dur:                   1,
			Size:                  uint32(len(sampleData)),
			CompositionTimeOffset: 0,
		},
		DecodeTime: 0,
		Data:       sampleData,
	})
	seg := mp4.NewMediaSegment()
	seg.AddFragment(frag)
	if err := seg.Encode(f); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestDecodeAnnexBToPNG(t *testing.T) {
	dir := t.TempDir()
	input := generateTestBitstream(t, dir)
	output := filepath.Join(dir, "out.png")

	if err := run([]string{appName, input, output}); err != nil {
		t.Fatalf("run: %v", err)
	}

	info, err := os.Stat(output)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("empty PNG output")
	}
}

func TestDecodeAnnexBToJPEG(t *testing.T) {
	dir := t.TempDir()
	input := generateTestBitstream(t, dir)
	output := filepath.Join(dir, "out.jpg")

	if err := run([]string{appName, "-q", "90", input, output}); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		t.Error("output should start with JPEG SOI marker")
	}
}

func TestDecodeAnnexBToY4M(t *testing.T) {
	dir := t.TempDir()
	input := generateTestBitstream(t, dir)
	output := filepath.Join(dir, "out.y4m")

	if err := run([]string{appName, input, output}); err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("YUV4MPEG2")) {
		t.Error("Y4M output should start with YUV4MPEG2 header")
	}
	if !strings.Contains(string(data), "FRAME\n") {
		t.Error("Y4M output should contain FRAME tag")
	}
}

func TestDecodeAnnexBToYUV(t *testing.T) {
	dir := t.TempDir()
	input := generateTestBitstream(t, dir)
	output := filepath.Join(dir, "out.yuv")

	if err := run([]string{appName, input, output}); err != nil {
		t.Fatalf("run: %v", err)
	}

	outPath := filepath.Join(dir, "out_32x32_yuv420p.yuv")
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// YUV420: 32*32 * 3/2 = 1536 bytes
	if info.Size() != 1536 {
		t.Errorf("file size %d, want 1536", info.Size())
	}
}

func TestDecodeInfoOnly(t *testing.T) {
	dir := t.TempDir()
	input := generateTestBitstream(t, dir)

	if err := run([]string{appName, input}); err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestDecodeNoDeblock(t *testing.T) {
	dir := t.TempDir()
	input := generateTestBitstream(t, dir)
	output := filepath.Join(dir, "out.png")

	if err := run([]string{appName, "-no-deblock", input, output}); err != nil {
		t.Fatalf("run: %v", err)
	}

	info, err := os.Stat(output)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("empty PNG output")
	}
}

func TestDecodeMultiFramePNG(t *testing.T) {
	dir := t.TempDir()

	p := encode.EncodeParams{Width: 16, Height: 16, QP: 26, MaxRefFrames: 1}
	sps, _ := encode.GenerateSPS(p)
	pps, _ := encode.GeneratePPS(p)

	grid, err := yuv.ParseGrid("x")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{'x': {Y: 128, Cb: 128, Cr: 128}}
	enc := &encode.FrameEncoder{
		Grid: grid, Colors: colors, QP: 26,
		MaxNumRefFrames: 1, Width: 16, Height: 16,
	}
	idr, err := enc.EncodeSlice(0)
	if err != nil {
		t.Fatal(err)
	}
	pskip, err := enc.EncodePSkipSlice(1)
	if err != nil {
		t.Fatal(err)
	}

	var bs []byte
	bs = append(bs, sps...)
	bs = append(bs, pps...)
	bs = append(bs, idr...)
	bs = append(bs, pskip...)

	input := filepath.Join(dir, "multi.264")
	if err := os.WriteFile(input, bs, 0644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "frame.png")
	if err := run([]string{appName, "-n", "2", input, output}); err != nil {
		t.Fatalf("run: %v", err)
	}

	for i := range 2 {
		path := filepath.Join(dir, fmt.Sprintf("frame_%04d.png", i))
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("frame %d: %v", i, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("frame %d: empty", i)
		}
	}
}

func TestDecodeMissingInput(t *testing.T) {
	if err := run([]string{appName}); err == nil {
		t.Error("expected error for missing input")
	}
}

func TestDecodeUnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	input := generateTestBitstream(t, dir)
	output := filepath.Join(dir, "out.bmp")

	if err := run([]string{appName, input, output}); err == nil {
		t.Error("expected error for unsupported output format")
	}
}

func TestDecodeFragmentedMP4(t *testing.T) {
	dir := t.TempDir()
	input := generateTestMP4(t, dir)
	output := filepath.Join(dir, "out.png")

	if err := run([]string{appName, input, output}); err != nil {
		t.Fatalf("run: %v", err)
	}

	info, err := os.Stat(output)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("empty PNG output")
	}
}

func TestDecodeProgressiveMP4(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "out.png")

	if err := run([]string{appName, "../../testdata/progressive_32x32.mp4", output}); err != nil {
		t.Fatalf("run: %v", err)
	}

	info, err := os.Stat(output)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("empty PNG output")
	}
}
