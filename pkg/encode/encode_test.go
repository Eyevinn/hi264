package encode

import (
	"bytes"
	"testing"

	"github.com/Eyevinn/hi264/pkg/decoder"
	"github.com/Eyevinn/hi264/pkg/yuv"
	"github.com/Eyevinn/mp4ff/avc"
)

func TestEncodeDecode1x1(t *testing.T) {
	grid, err := yuv.ParseGrid("x")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 128, Cb: 128, Cr: 128},
	}

	enc := &FrameEncoder{Grid: grid, Colors: colors, QP: 26}
	bs, err := enc.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Decode with our decoder
	nalus := avc.ExtractNalusFromByteStream(bs)
	dec := decoder.New()
	dec.SkipDeblock = true
	f, err := dec.DecodeNALUs(nalus)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if f.Width != 16 || f.Height != 16 {
		t.Errorf("frame size %dx%d, want 16x16", f.Width, f.Height)
	}

	// Check pixels
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			got := f.GetLumaPixel(x, y)
			if got != 128 {
				t.Errorf("luma(%d,%d) = %d, want 128", x, y, got)
			}
		}
	}
}

func TestEncodeDecode2x1TwoColors(t *testing.T) {
	grid, err := yuv.ParseGrid("xy")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 235, Cb: 128, Cr: 128},
		'y': {Y: 16, Cb: 128, Cr: 128},
	}

	enc := &FrameEncoder{Grid: grid, Colors: colors, QP: 26, DisableDeblock: 1}
	bs, err := enc.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	nalus := avc.ExtractNalusFromByteStream(bs)
	dec := decoder.New()
	dec.SkipDeblock = true
	f, err := dec.DecodeNALUs(nalus)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if f.Width != 32 || f.Height != 16 {
		t.Errorf("frame size %dx%d, want 32x16", f.Width, f.Height)
	}

	// Build expected frame for comparison
	expected, err := yuv.BuildFrame(grid, colors)
	if err != nil {
		t.Fatal(err)
	}

	// Compare luma
	mismatch := 0
	for y := 0; y < f.Height; y++ {
		for x := 0; x < f.Width; x++ {
			got := f.GetLumaPixel(x, y)
			want := expected.GetLumaPixel(x, y)
			diff := int(got) - int(want)
			if diff < 0 {
				diff = -diff
			}
			if diff > 1 {
				if mismatch < 5 {
					t.Errorf("luma(%d,%d) = %d, want %d (diff=%d)", x, y, got, want, diff)
				}
				mismatch++
			}
		}
	}
	if mismatch > 0 {
		t.Errorf("total luma mismatches: %d", mismatch)
	}
}

func TestEncodeDecode2x2Checkerboard(t *testing.T) {
	grid, err := yuv.ParseGrid("xy,yx")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 200, Cb: 100, Cr: 150},
		'y': {Y: 50, Cb: 200, Cr: 80},
	}

	enc := &FrameEncoder{Grid: grid, Colors: colors, QP: 20, DisableDeblock: 1}
	bs, err := enc.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	nalus := avc.ExtractNalusFromByteStream(bs)
	dec := decoder.New()
	dec.SkipDeblock = true
	f, err := dec.DecodeNALUs(nalus)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if f.Width != 32 || f.Height != 32 {
		t.Errorf("frame size %dx%d, want 32x32", f.Width, f.Height)
	}

	// Build expected and compare
	expected, err := yuv.BuildFrame(grid, colors)
	if err != nil {
		t.Fatal(err)
	}

	mismatch := 0
	for y := 0; y < f.Height; y++ {
		for x := 0; x < f.Width; x++ {
			got := f.GetLumaPixel(x, y)
			want := expected.GetLumaPixel(x, y)
			diff := int(got) - int(want)
			if diff < 0 {
				diff = -diff
			}
			if diff > 2 {
				if mismatch < 5 {
					t.Errorf("luma(%d,%d) = %d, want %d (diff=%d)", x, y, got, want, diff)
				}
				mismatch++
			}
		}
	}
	if mismatch > 0 {
		t.Errorf("total luma mismatches (>2): %d", mismatch)
	}
}

func TestEncodeDecode3Colors(t *testing.T) {
	grid, err := yuv.ParseGrid("abc,bca")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'a': {Y: 235, Cb: 128, Cr: 128}, // white
		'b': {Y: 16, Cb: 128, Cr: 128},  // black
		'c': {Y: 128, Cb: 128, Cr: 128}, // gray
	}

	enc := &FrameEncoder{Grid: grid, Colors: colors, QP: 26, DisableDeblock: 1}
	bs, err := enc.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	nalus := avc.ExtractNalusFromByteStream(bs)
	dec := decoder.New()
	dec.SkipDeblock = true
	f, err := dec.DecodeNALUs(nalus)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if f.Width != 48 || f.Height != 32 {
		t.Errorf("frame size %dx%d, want 48x32", f.Width, f.Height)
	}
}

// CABAC round-trip tests

func TestEncodeDecodeCABAC1x1(t *testing.T) {
	grid, err := yuv.ParseGrid("x")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 128, Cb: 128, Cr: 128},
	}

	enc := &FrameEncoder{Grid: grid, Colors: colors, QP: 26, CABAC: true}
	bs, err := enc.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	nalus := avc.ExtractNalusFromByteStream(bs)
	dec := decoder.New()
	dec.SkipDeblock = true
	f, err := dec.DecodeNALUs(nalus)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if f.Width != 16 || f.Height != 16 {
		t.Errorf("frame size %dx%d, want 16x16", f.Width, f.Height)
	}

	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			got := f.GetLumaPixel(x, y)
			if got != 128 {
				t.Errorf("luma(%d,%d) = %d, want 128", x, y, got)
			}
		}
	}
}

func TestEncodeDecodeCABAC2x1TwoColors(t *testing.T) {
	grid, err := yuv.ParseGrid("xy")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 235, Cb: 128, Cr: 128},
		'y': {Y: 16, Cb: 128, Cr: 128},
	}

	enc := &FrameEncoder{Grid: grid, Colors: colors, QP: 26, DisableDeblock: 1, CABAC: true}
	bs, err := enc.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	nalus := avc.ExtractNalusFromByteStream(bs)
	dec := decoder.New()
	dec.SkipDeblock = true
	f, err := dec.DecodeNALUs(nalus)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if f.Width != 32 || f.Height != 16 {
		t.Errorf("frame size %dx%d, want 32x16", f.Width, f.Height)
	}

	expected, err := yuv.BuildFrame(grid, colors)
	if err != nil {
		t.Fatal(err)
	}

	mismatch := 0
	for y := 0; y < f.Height; y++ {
		for x := 0; x < f.Width; x++ {
			got := f.GetLumaPixel(x, y)
			want := expected.GetLumaPixel(x, y)
			diff := int(got) - int(want)
			if diff < 0 {
				diff = -diff
			}
			if diff > 1 {
				if mismatch < 5 {
					t.Errorf("luma(%d,%d) = %d, want %d (diff=%d)", x, y, got, want, diff)
				}
				mismatch++
			}
		}
	}
	if mismatch > 0 {
		t.Errorf("total luma mismatches: %d", mismatch)
	}
}

func TestEncodeDecodeCABAC2x2Checkerboard(t *testing.T) {
	grid, err := yuv.ParseGrid("xy,yx")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 200, Cb: 100, Cr: 150},
		'y': {Y: 50, Cb: 200, Cr: 80},
	}

	enc := &FrameEncoder{Grid: grid, Colors: colors, QP: 20, DisableDeblock: 1, CABAC: true}
	bs, err := enc.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	nalus := avc.ExtractNalusFromByteStream(bs)
	dec := decoder.New()
	dec.SkipDeblock = true
	f, err := dec.DecodeNALUs(nalus)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if f.Width != 32 || f.Height != 32 {
		t.Errorf("frame size %dx%d, want 32x32", f.Width, f.Height)
	}

	expected, err := yuv.BuildFrame(grid, colors)
	if err != nil {
		t.Fatal(err)
	}

	mismatch := 0
	for y := 0; y < f.Height; y++ {
		for x := 0; x < f.Width; x++ {
			got := f.GetLumaPixel(x, y)
			want := expected.GetLumaPixel(x, y)
			diff := int(got) - int(want)
			if diff < 0 {
				diff = -diff
			}
			if diff > 2 {
				if mismatch < 5 {
					t.Errorf("luma(%d,%d) = %d, want %d (diff=%d)", x, y, got, want, diff)
				}
				mismatch++
			}
		}
	}
	if mismatch > 0 {
		t.Errorf("total luma mismatches (>2): %d", mismatch)
	}
}

func TestEncodeDecodeCABAC3Colors(t *testing.T) {
	grid, err := yuv.ParseGrid("abc,bca")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'a': {Y: 235, Cb: 128, Cr: 128},
		'b': {Y: 16, Cb: 128, Cr: 128},
		'c': {Y: 128, Cb: 128, Cr: 128},
	}

	enc := &FrameEncoder{Grid: grid, Colors: colors, QP: 26, DisableDeblock: 1, CABAC: true}
	bs, err := enc.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	nalus := avc.ExtractNalusFromByteStream(bs)
	dec := decoder.New()
	dec.SkipDeblock = true
	f, err := dec.DecodeNALUs(nalus)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if f.Width != 48 || f.Height != 32 {
		t.Errorf("frame size %dx%d, want 48x32", f.Width, f.Height)
	}
}

// TestEncodePSkipCAVLC verifies IDR + P_Skip round-trip: encode IDR, encode P_Skip,
// decode both, verify P_Skip frame has identical pixels to IDR frame.
func TestEncodePSkipCAVLC(t *testing.T) {
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

	// Write SPS+PPS + IDR + P_Skip
	var buf bytes.Buffer
	if err := enc.EncodeSPSPPS(&buf); err != nil {
		t.Fatalf("EncodeSPSPPS: %v", err)
	}
	idrSlice, err := enc.EncodeSlice(0)
	if err != nil {
		t.Fatalf("EncodeSlice: %v", err)
	}
	buf.Write(idrSlice)

	pSkipSlice, err := enc.EncodePSkipSlice(1)
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

	// Also check chroma
	chromaW := idrFrame.Width / 2
	chromaH := idrFrame.Height / 2
	for y := 0; y < chromaH; y++ {
		for x := 0; x < chromaW; x++ {
			for c := 0; c < 2; c++ {
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

// TestSPSMaxRefFrames verifies SPS with maxRef=0 is unchanged, maxRef=1 differs.
func TestSPSMaxRefFrames(t *testing.T) {
	sps0 := EncodeSPS(32, 32, 0, 0, 0)
	sps1 := EncodeSPS(32, 32, 1, 0, 0)

	if bytes.Equal(sps0, sps1) {
		t.Error("SPS with maxRef=0 and maxRef=1 should differ")
	}

	// Parse both and verify max_num_ref_frames
	nalu0 := append([]byte{0x67}, sps0...)
	parsed0, err := avc.ParseSPSNALUnit(nalu0, true)
	if err != nil {
		t.Fatalf("parse SPS(maxRef=0): %v", err)
	}
	if parsed0.NumRefFrames != 0 {
		t.Errorf("SPS(maxRef=0): NbRefFrames = %d, want 0", parsed0.NumRefFrames)
	}

	nalu1 := append([]byte{0x67}, sps1...)
	parsed1, err := avc.ParseSPSNALUnit(nalu1, true)
	if err != nil {
		t.Fatalf("parse SPS(maxRef=1): %v", err)
	}
	if parsed1.NumRefFrames != 1 {
		t.Errorf("SPS(maxRef=1): NbRefFrames = %d, want 1", parsed1.NumRefFrames)
	}
}

// TestEncodeRefactorEquivalence verifies that EncodeSPSPPS + EncodeSlice(0)
// produces identical output to the original Encode() method.
func TestEncodeRefactorEquivalence(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		name := "CAVLC"
		if cabac {
			name = "CABAC"
		}
		t.Run(name, func(t *testing.T) {
			grid, err := yuv.ParseGrid("xy,yx")
			if err != nil {
				t.Fatal(err)
			}
			colors := yuv.ColorMap{
				'x': {Y: 200, Cb: 100, Cr: 150},
				'y': {Y: 50, Cb: 200, Cr: 80},
			}

			enc := &FrameEncoder{Grid: grid, Colors: colors, QP: 20, DisableDeblock: 1, CABAC: cabac}

			// Method 1: Encode()
			full, err := enc.Encode()
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}

			// Method 2: EncodeSPSPPS + EncodeSlice
			var buf bytes.Buffer
			if err := enc.EncodeSPSPPS(&buf); err != nil {
				t.Fatalf("EncodeSPSPPS: %v", err)
			}
			slice, err := enc.EncodeSlice(0)
			if err != nil {
				t.Fatalf("EncodeSlice: %v", err)
			}
			buf.Write(slice)

			if !bytes.Equal(full, buf.Bytes()) {
				t.Errorf("Encode() != EncodeSPSPPS+EncodeSlice (len %d vs %d)", len(full), buf.Len())
			}
		})
	}
}
