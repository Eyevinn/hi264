package decoder

import (
	"strings"
	"testing"

	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/hi264/pkg/yuv"
	"github.com/Eyevinn/mp4ff/avc"
)

// A crafted SPS can declare enormous picture dimensions. The macroblock count
// derived from them sizes a per-macroblock allocation, so an oversized frame
// must be rejected before the allocation rather than exhausting memory.
func TestDecodeRejectsOversizedFrame(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		name := "CAVLC"
		if cabac {
			name = "CABAC"
		}
		t.Run(name, func(t *testing.T) {
			// 100000x100000 -> 6250*6250 = ~39M macroblocks, far above the
			// largest H.264 level (6.2: 139264).
			huge := encode.EncodeParams{Width: 100000, Height: 100000, QP: 26, CABAC: cabac}
			spsNALU, err := encode.GenerateSPS(huge)
			if err != nil {
				t.Fatalf("GenerateSPS: %v", err)
			}
			ppsNALU, err := encode.GeneratePPS(huge)
			if err != nil {
				t.Fatalf("GeneratePPS: %v", err)
			}
			// A real IDR slice: its slice header does not depend on the picture
			// size, so decoding reaches the frame-size check.
			small := encode.EncodeParams{Width: 16, Height: 16, QP: 26, CABAC: cabac}
			grid, err := yuv.ParseGrid("x")
			if err != nil {
				t.Fatalf("ParseGrid: %v", err)
			}
			idr, err := encode.GenerateIDR(small, grid, yuv.ColorMap{'x': {Y: 128, Cb: 128, Cr: 128}}, 0)
			if err != nil {
				t.Fatalf("GenerateIDR: %v", err)
			}

			nalus := [][]byte{
				avc.ExtractNalusFromByteStream(spsNALU)[0],
				avc.ExtractNalusFromByteStream(ppsNALU)[0],
				avc.ExtractNalusFromByteStream(idr)[0],
			}

			dec := New()
			_, err = dec.DecodeNALUs(nalus)
			if err == nil {
				t.Fatal("expected an error for oversized frame, got nil")
			}
			if !strings.Contains(err.Error(), "exceeds maximum") {
				t.Fatalf("expected a frame-size error, got: %v", err)
			}
		})
	}
}
