package slice

import (
	"testing"
	"time"

	"github.com/Eyevinn/hi264/internal/cabac"
)

// With the bitstream exhausted the CABAC engine keeps returning the MPS. If the
// mb_qp_delta contexts all have MPS 1, the unary suffix never ends on its own;
// DecodeQPDelta must give up with an error instead of looping forever.
func TestDecodeQPDeltaExhaustedBitstream(t *testing.T) {
	d, err := cabac.NewDecoder([]byte{0, 0})
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	var ctx [1024]cabac.CtxState
	for _, i := range []int{60, 62, 63} {
		ctx[i] = cabac.CtxState{PStateIdx: 62, ValMPS: 1}
	}
	sc := &SliceContext{Cabac: d, Ctx: &ctx, BitDepthY: 8}

	done := make(chan error, 1)
	go func() {
		_, err := DecodeQPDelta(sc)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error for an over-long mb_qp_delta, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DecodeQPDelta did not return: unbounded unary loop")
	}
}
