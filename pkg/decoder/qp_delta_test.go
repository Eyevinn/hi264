package decoder

import (
	"strings"
	"testing"
)

// mb_qp_delta is limited to -26..25 at 8 bits (section 7.4.5). Further out,
// the QPY update of equation 7-37 can turn negative, and the dequantisation
// then indexes its tables with qp%6 = -1. A slice with such an mb_qp_delta
// must be rejected rather than panic. The in-range cases show that the slice
// is otherwise valid.
func TestDecodeRejectsOutOfRangeQPDelta(t *testing.T) {
	cases := []struct {
		desc      string
		mbQPDelta int32
		wantErr   bool
	}{
		{"largest mb_qp_delta", 25, false},
		{"smallest mb_qp_delta", -26, false},
		{"mb_qp_delta too large", 26, true},
		{"mb_qp_delta too small", -27, true},
		{"mb_qp_delta giving a negative QPY", -100, true},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic on decode: %v", r)
				}
			}()
			nalus := oneMBStream(t, 0, 0, c.mbQPDelta)
			_, err := New().DecodeNALUs(nalus)
			switch {
			case !c.wantErr && err != nil:
				t.Fatalf("decode: %v", err)
			case c.wantErr && err == nil:
				t.Fatal("expected an error for an out-of-range mb_qp_delta, got nil")
			case c.wantErr && !strings.Contains(err.Error(), "mb_qp_delta"):
				t.Fatalf("expected an mb_qp_delta error, got: %v", err)
			}
		})
	}
}
