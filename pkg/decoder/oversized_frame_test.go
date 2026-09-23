package decoder

import (
	"strings"
	"testing"

	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/mp4ff/avc"
)

// A crafted SPS can declare enormous picture dimensions. The macroblock count
// derived from them sizes a per-macroblock allocation, so an oversized frame
// must be rejected before the allocation rather than exhausting memory.
func TestDecodeRejectsOversizedFrame(t *testing.T) {
	// 100000x100000 -> 6250*6250 = ~39M macroblocks, far above the largest
	// H.264 level (5.2: 139264).
	spsNALU, err := encode.GenerateSPS(encode.EncodeParams{Width: 100000, Height: 100000, QP: 26})
	if err != nil {
		t.Fatalf("GenerateSPS: %v", err)
	}
	ppsNALU, err := encode.GeneratePPS(encode.EncodeParams{Width: 100000, Height: 100000, QP: 26})
	if err != nil {
		t.Fatalf("GeneratePPS: %v", err)
	}
	// A minimal IDR NALU (type 5) header; the decode must fail on the size
	// check before it tries to read slice data.
	idrNALU := []byte{0x65, 0x88, 0x80, 0x00}

	nalus := [][]byte{
		avc.ExtractNalusFromByteStream(spsNALU)[0],
		avc.ExtractNalusFromByteStream(ppsNALU)[0],
		idrNALU,
	}

	dec := New()
	_, err = dec.DecodeNALUs(nalus)
	if err == nil {
		t.Fatal("expected an error for oversized frame, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("expected a frame-size error, got: %v", err)
	}
}
