package encode

import (
	"math/rand"
	"testing"

	"github.com/Eyevinn/hi264/internal/cavlc"
)

// roundTripResidual encodes a coefficient block with EncodeResidualBlock and
// decodes it with the CAVLC reader, which is verified against FFmpeg.
func roundTripResidual(t *testing.T, in []int32, nC, maxNumCoeff int) {
	t.Helper()

	w := NewBitWriter()
	EncodeResidualBlock(w, in, nC, maxNumCoeff)
	w.WriteBit(1) // rbsp_stop_one_bit so the reader never runs dry
	w.AlignToByte()

	out, _, err := cavlc.DecodeResidualBlock(cavlc.NewBitReader(w.Bytes()), nC, maxNumCoeff)
	if err != nil {
		t.Fatalf("nC=%d maxNumCoeff=%d in=%v: decode: %v", nC, maxNumCoeff, in, err)
	}
	for i := range maxNumCoeff {
		if out[i] != in[i] {
			t.Fatalf("nC=%d maxNumCoeff=%d round-trip mismatch:\n in  = %v\n out = %v",
				nC, maxNumCoeff, in, out[:maxNumCoeff])
			return
		}
	}
}

// TestChromaDCTrailingOnesSigns covers trailing_ones_sign_flag ordering.
// Two trailing ones with opposite signs used to come back swapped, which showed
// up as chroma prediction drift streaking down and to the right in PNG/JPEG
// encodes (grid patterns rarely produce mixed-sign trailing-one pairs).
func TestChromaDCTrailingOnesSigns(t *testing.T) {
	for _, in := range [][]int32{
		{0, -1, 1, 0},
		{0, 1, -1, 0},
		{1, -1, 1, -1},
		{-1, 1, -1, 1},
		{0, 1, 1, -1},
		{2, 1, -1, 0},
	} {
		roundTripResidual(t, in, -1, 4)
	}
}

// TestChromaDCRoundTripExhaustive covers every sign/zero combination of
// magnitude-1 chroma DC coefficients, where trailing ones dominate.
func TestChromaDCRoundTripExhaustive(t *testing.T) {
	vals := []int32{0, 1, -1, 2}
	for _, a := range vals {
		for _, b := range vals {
			for _, c := range vals {
				for _, d := range vals {
					roundTripResidual(t, []int32{a, b, c, d}, -1, 4)
				}
			}
		}
	}
}

// TestResidualRoundTripRandom fuzzes every block category the encoder emits.
func TestResidualRoundTripRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(20240521))
	specs := []struct{ nC, maxNumCoeff int }{
		{-1, 4}, {0, 15}, {2, 15}, {0, 16}, {4, 16}, {8, 16},
	}
	for _, spec := range specs {
		for range 2000 {
			blk := make([]int32, spec.maxNumCoeff)
			for i := range blk {
				switch rng.Intn(4) {
				case 0:
					blk[i] = int32(rng.Intn(3) - 1) // bias toward +/-1 to hit trailing ones
				case 1:
					blk[i] = int32(rng.Intn(41) - 20)
				}
			}
			roundTripResidual(t, blk, spec.nC, spec.maxNumCoeff)
		}
	}
}
