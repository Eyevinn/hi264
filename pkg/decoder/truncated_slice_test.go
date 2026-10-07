package decoder

import (
	"testing"
	"time"

	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/hi264/pkg/yuv"
	"github.com/Eyevinn/mp4ff/avc"
)

// A truncated CABAC slice must fail or decode, never hang. Every prefix of a
// generated CABAC IDR slice is decoded under a timeout.
func TestDecodeTruncatedCABACSlice(t *testing.T) {
	grid, err := yuv.ParseGrid("xyxy,yxyx")
	if err != nil {
		t.Fatalf("ParseGrid: %v", err)
	}
	enc := &encode.FrameEncoder{
		Grid:   grid,
		Colors: yuv.ColorMap{'x': {Y: 200, Cb: 100, Cr: 150}, 'y': {Y: 50, Cb: 200, Cr: 80}},
		QP:     26,
		CABAC:  true,
	}
	stream, err := enc.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	nalus := avc.ExtractNalusFromByteStream(stream)
	if len(nalus) != 3 {
		t.Fatalf("expected SPS, PPS and IDR NALUs, got %d", len(nalus))
	}
	sps, pps, idr := nalus[0], nalus[1], nalus[2]

	for n := 1; n < len(idr); n++ {
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() { _ = recover() }()
			_, _ = New().DecodeNALUs([][]byte{sps, pps, idr[:n]})
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("decoding the IDR slice truncated to %d of %d bytes hung", n, len(idr))
		}
	}
}
