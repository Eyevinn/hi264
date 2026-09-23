package pred

import "testing"

// A malformed bitstream can select vertical/horizontal/plane 16x16 intra
// prediction for a macroblock whose top/left neighbor is unavailable. Those
// modes index into the neighbor sample slices, so a nil neighbor must be
// reported as an error rather than panicking with an out-of-range access.
func TestPredict16x16MissingNeighborsError(t *testing.T) {
	full := make([]uint8, 16)
	cases := []struct {
		name      string
		mode      int
		top, left []uint8
	}{
		{"vertical-no-top", Intra16x16Vertical, nil, full},
		{"horizontal-no-left", Intra16x16Horizontal, full, nil},
		{"plane-no-top", Intra16x16Plane, nil, full},
		{"plane-no-left", Intra16x16Plane, full, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Predict16x16(c.mode, c.top, c.left, 0); err == nil {
				t.Fatalf("expected error for %s, got nil", c.name)
			}
		})
	}
}

// With both neighbors present the modes still succeed.
func TestPredict16x16WithNeighborsOK(t *testing.T) {
	full := make([]uint8, 16)
	for _, mode := range []int{Intra16x16Vertical, Intra16x16Horizontal, Intra16x16DC, Intra16x16Plane} {
		if _, err := Predict16x16(mode, full, full, 0); err != nil {
			t.Fatalf("mode %d with neighbors: unexpected error %v", mode, err)
		}
	}
	// DC mode tolerates missing neighbors (falls back to 128).
	if _, err := Predict16x16(Intra16x16DC, nil, nil, 0); err != nil {
		t.Fatalf("DC with no neighbors: unexpected error %v", err)
	}
}
