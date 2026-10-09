package slice

import "testing"

// A neighbouring macroblock in a different slice is not available, as one
// outside the picture is not (clause 6.4.8).
func TestMBAvailAcrossSlices(t *testing.T) {
	// A picture 4 macroblocks wide; slice 2 starts at macroblock 5:
	//
	//	1 1 1 1
	//	1 2 2 2
	//	2 2 2 2
	sc := NewSliceContext(4, 3, false, false, 1, 8, 8, 0, false)
	for i := range sc.MBs {
		sc.MBs[i].SliceNum = 1
		if i >= 5 {
			sc.MBs[i].SliceNum = 2
		}
	}
	cases := []struct {
		mbIdx      int
		a, b, c, d bool
	}{
		{0, false, false, false, false}, // picture corner
		{4, false, true, true, false},   // left edge, all slice 1
		{5, false, false, false, false}, // first MB of slice 2
		{6, true, false, false, false},
		{8, false, false, true, false}, // B in slice 1, C in slice 2
		{9, true, true, true, false},   // D (MB 4) in slice 1
		{10, true, true, true, true},
		{11, true, true, false, true}, // right edge: no C
	}
	for _, c := range cases {
		got := [4]bool{
			sc.MBAvailA(c.mbIdx) != nil, sc.MBAvailB(c.mbIdx) != nil,
			sc.MBAvailC(c.mbIdx) != nil, sc.MBAvailD(c.mbIdx) != nil,
		}
		want := [4]bool{c.a, c.b, c.c, c.d}
		if got != want {
			t.Errorf("mb %d: availability A, B, C, D = %v, want %v", c.mbIdx, got, want)
		}
	}
}
