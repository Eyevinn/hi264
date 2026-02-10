package decoder

import (
	"encoding/binary"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"

	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/hi264/pkg/yuv"
)

func TestDecodeAnnexB(t *testing.T) {
	grid, err := yuv.ParseGrid("x")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 128, Cb: 128, Cr: 128},
	}

	enc := &encode.FrameEncoder{Grid: grid, Colors: colors, QP: 26}
	bs, err := enc.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	dec := New()
	dec.SkipDeblock = true
	f, err := dec.DecodeAnnexB(bs)
	if err != nil {
		t.Fatalf("DecodeAnnexB: %v", err)
	}

	if f.Width != 16 || f.Height != 16 {
		t.Errorf("frame size %dx%d, want 16x16", f.Width, f.Height)
	}

	got := f.GetLumaPixel(0, 0)
	if got != 128 {
		t.Errorf("luma(0,0) = %d, want 128", got)
	}
}

func TestDecodeAllAnnexB(t *testing.T) {
	grid, err := yuv.ParseGrid("x")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 128, Cb: 128, Cr: 128},
	}

	enc := &encode.FrameEncoder{
		Grid:            grid,
		Colors:          colors,
		QP:              26,
		MaxNumRefFrames: 1,
		Width:           16,
		Height:          16,
	}

	// Generate SPS+PPS+IDR+PSkip
	var bs []byte
	spsData, _ := encode.GenerateSPS(encode.EncodeParams{Width: 16, Height: 16, QP: 26, MaxRefFrames: 1})
	ppsData, _ := encode.GeneratePPS(encode.EncodeParams{Width: 16, Height: 16, QP: 26, MaxRefFrames: 1})
	bs = append(bs, spsData...)
	bs = append(bs, ppsData...)

	idr, err := enc.EncodeSlice(0)
	if err != nil {
		t.Fatalf("EncodeSlice: %v", err)
	}
	bs = append(bs, idr...)

	pskip, err := enc.EncodePSkipSlice(1)
	if err != nil {
		t.Fatalf("EncodePSkipSlice: %v", err)
	}
	bs = append(bs, pskip...)

	dec := New()
	dec.SkipDeblock = true
	frames, err := dec.DecodeAllAnnexB(bs)
	if err != nil {
		t.Fatalf("DecodeAllAnnexB: %v", err)
	}

	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}

	for i, f := range frames {
		if f.Width != 16 || f.Height != 16 {
			t.Errorf("frame %d: size %dx%d, want 16x16", i, f.Width, f.Height)
		}
	}
}

func TestDecodeAllAVC(t *testing.T) {
	grid, err := yuv.ParseGrid("x")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 128, Cb: 128, Cr: 128},
	}

	enc := &encode.FrameEncoder{
		Grid:            grid,
		Colors:          colors,
		QP:              26,
		MaxNumRefFrames: 1,
		Width:           16,
		Height:          16,
	}

	// Generate SPS+PPS+IDR+PSkip as Annex-B, then convert to AVC
	var annexB []byte
	spsData, _ := encode.GenerateSPS(encode.EncodeParams{Width: 16, Height: 16, QP: 26, MaxRefFrames: 1})
	ppsData, _ := encode.GeneratePPS(encode.EncodeParams{Width: 16, Height: 16, QP: 26, MaxRefFrames: 1})
	annexB = append(annexB, spsData...)
	annexB = append(annexB, ppsData...)

	idr, err := enc.EncodeSlice(0)
	if err != nil {
		t.Fatalf("EncodeSlice: %v", err)
	}
	annexB = append(annexB, idr...)

	pskip, err := enc.EncodePSkipSlice(1)
	if err != nil {
		t.Fatalf("EncodePSkipSlice: %v", err)
	}
	annexB = append(annexB, pskip...)

	// Convert to AVC format
	annexBNalus := avc.ExtractNalusFromByteStream(annexB)
	var avcData []byte
	for _, nalu := range annexBNalus {
		length := make([]byte, 4)
		binary.BigEndian.PutUint32(length, uint32(len(nalu)))
		avcData = append(avcData, length...)
		avcData = append(avcData, nalu...)
	}

	dec := New()
	dec.SkipDeblock = true
	frames, err := dec.DecodeAllAVC(avcData)
	if err != nil {
		t.Fatalf("DecodeAllAVC: %v", err)
	}

	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}

	for i, f := range frames {
		if f.Width != 16 || f.Height != 16 {
			t.Errorf("frame %d: size %dx%d, want 16x16", i, f.Width, f.Height)
		}
	}
}

func TestDecodeAVC(t *testing.T) {
	grid, err := yuv.ParseGrid("x")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 200, Cb: 128, Cr: 128},
	}

	enc := &encode.FrameEncoder{Grid: grid, Colors: colors, QP: 26}
	bs, err := enc.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Convert Annex-B to AVC format (4-byte length prefix)
	annexBNalus := avc.ExtractNalusFromByteStream(bs)
	var avcData []byte
	for _, nalu := range annexBNalus {
		length := make([]byte, 4)
		binary.BigEndian.PutUint32(length, uint32(len(nalu)))
		avcData = append(avcData, length...)
		avcData = append(avcData, nalu...)
	}

	dec := New()
	dec.SkipDeblock = true
	f, err := dec.DecodeAVC(avcData)
	if err != nil {
		t.Fatalf("DecodeAVC: %v", err)
	}

	if f.Width != 16 || f.Height != 16 {
		t.Errorf("frame size %dx%d, want 16x16", f.Width, f.Height)
	}

	got := f.GetLumaPixel(0, 0)
	if got != 200 {
		t.Errorf("luma(0,0) = %d, want 200", got)
	}
}
