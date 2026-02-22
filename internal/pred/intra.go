// Package pred implements intra prediction modes for H.264/AVC decoding
// as specified in sections 8.3.1 through 8.3.4 of the standard.
package pred

// Intra16x16 prediction mode constants.
const (
	Intra16x16Vertical   = 0
	Intra16x16Horizontal = 1
	Intra16x16DC         = 2
	Intra16x16Plane      = 3
)

// Intra4x4 prediction mode constants.
const (
	Intra4x4Vertical       = 0
	Intra4x4Horizontal     = 1
	Intra4x4DC             = 2
	Intra4x4DiagDownLeft   = 3
	Intra4x4DiagDownRight  = 4
	Intra4x4VerticalRight  = 5
	Intra4x4HorizontalDown = 6
	Intra4x4VerticalLeft   = 7
	Intra4x4HorizontalUp   = 8
)

// IntraChroma prediction mode constants.
const (
	IntraChromaDC         = 0
	IntraChromaHorizontal = 1
	IntraChromaVertical   = 2
	IntraChromaPlane      = 3
)

// Predict16x16 generates a 16x16 prediction block.
// top: 16 samples from the row above (nil if not available).
// left: 16 samples from the column to the left (nil if not available).
// topLeft: the sample at position (-1, -1) (only needed for Plane mode).
func Predict16x16(mode int, top, left []uint8, topLeft uint8) [16][16]uint8 {
	var pred [16][16]uint8

	switch mode {
	case Intra16x16Vertical:
		for y := range 16 {
			for x := range 16 {
				pred[y][x] = top[x]
			}
		}
	case Intra16x16Horizontal:
		for y := range 16 {
			for x := range 16 {
				pred[y][x] = left[y]
			}
		}
	case Intra16x16DC:
		sum := 0
		hasTop := top != nil
		hasLeft := left != nil
		if hasTop {
			for x := range 16 {
				sum += int(top[x])
			}
		}
		if hasLeft {
			for y := range 16 {
				sum += int(left[y])
			}
		}
		var dc uint8
		if hasTop && hasLeft {
			dc = uint8((sum + 16) >> 5)
		} else if hasTop || hasLeft {
			dc = uint8((sum + 8) >> 4)
		} else {
			dc = 128
		}
		for y := range 16 {
			for x := range 16 {
				pred[y][x] = dc
			}
		}
	case Intra16x16Plane:
		iH := 0
		iV := 0
		for x := range 8 {
			var pLeft int
			if 6-x >= 0 {
				pLeft = int(top[6-x])
			} else {
				pLeft = int(topLeft) // p[-1,-1]
			}
			iH += (x + 1) * (int(top[8+x]) - pLeft)
		}
		for y := range 8 {
			var pAbove int
			if 6-y >= 0 {
				pAbove = int(left[6-y])
			} else {
				pAbove = int(topLeft) // p[-1,-1]
			}
			iV += (y + 1) * (int(left[8+y]) - pAbove)
		}
		a := 16 * (int(top[15]) + int(left[15]))
		b := (5*iH + 32) >> 6
		c := (5*iV + 32) >> 6

		for y := range 16 {
			for x := range 16 {
				val := (a + b*(x-7) + c*(y-7) + 16) >> 5
				pred[y][x] = clip8(val)
			}
		}
	}

	return pred
}

// Predict4x4 generates a 4x4 prediction block (section 8.3.1).
// ref contains the 13 reference samples: 4 left (L0-L3), top-left (TL),
// 8 top (T0-T7, where T4-T7 are the upper-right).
// ref layout: [L3, L2, L1, L0, TL, T0, T1, T2, T3, T4, T5, T6, T7]
// leftAvail/topAvail indicate whether left/top samples exist (affects DC mode).
func Predict4x4(mode int, ref [13]uint8, leftAvail, topAvail bool) [4][4]uint8 {
	var pred [4][4]uint8

	l0 := int(ref[3])
	l1 := int(ref[2])
	l2 := int(ref[1])
	l3 := int(ref[0])
	tl := int(ref[4])
	t0 := int(ref[5])
	t1 := int(ref[6])
	t2 := int(ref[7])
	t3 := int(ref[8])
	t4 := int(ref[9])
	t5 := int(ref[10])
	t6 := int(ref[11])
	t7 := int(ref[12])

	switch mode {
	case Intra4x4Vertical:
		for y := range 4 {
			pred[y][0] = uint8(t0)
			pred[y][1] = uint8(t1)
			pred[y][2] = uint8(t2)
			pred[y][3] = uint8(t3)
		}
	case Intra4x4Horizontal:
		for x := range 4 {
			pred[0][x] = uint8(l0)
			pred[1][x] = uint8(l1)
			pred[2][x] = uint8(l2)
			pred[3][x] = uint8(l3)
		}
	case Intra4x4DC:
		var dc uint8
		switch {
		case topAvail && leftAvail:
			dc = uint8((t0 + t1 + t2 + t3 + l0 + l1 + l2 + l3 + 4) >> 3)
		case topAvail:
			dc = uint8((t0 + t1 + t2 + t3 + 2) >> 2)
		case leftAvail:
			dc = uint8((l0 + l1 + l2 + l3 + 2) >> 2)
		default:
			dc = 128
		}
		for y := range 4 {
			for x := range 4 {
				pred[y][x] = dc
			}
		}
	case Intra4x4DiagDownLeft:
		pred[0][0] = uint8((t0 + 2*t1 + t2 + 2) >> 2)
		pred[0][1] = uint8((t1 + 2*t2 + t3 + 2) >> 2)
		pred[1][0] = pred[0][1]
		pred[0][2] = uint8((t2 + 2*t3 + t4 + 2) >> 2)
		pred[1][1] = pred[0][2]
		pred[2][0] = pred[0][2]
		pred[0][3] = uint8((t3 + 2*t4 + t5 + 2) >> 2)
		pred[1][2] = pred[0][3]
		pred[2][1] = pred[0][3]
		pred[3][0] = pred[0][3]
		pred[1][3] = uint8((t4 + 2*t5 + t6 + 2) >> 2)
		pred[2][2] = pred[1][3]
		pred[3][1] = pred[1][3]
		pred[2][3] = uint8((t5 + 2*t6 + t7 + 2) >> 2)
		pred[3][2] = pred[2][3]
		pred[3][3] = uint8((t6 + 2*t7 + t7 + 2) >> 2)
	case Intra4x4DiagDownRight:
		pred[3][0] = uint8((l3 + 2*l2 + l1 + 2) >> 2)
		pred[2][0] = uint8((l2 + 2*l1 + l0 + 2) >> 2)
		pred[3][1] = pred[2][0]
		pred[1][0] = uint8((l1 + 2*l0 + tl + 2) >> 2)
		pred[2][1] = pred[1][0]
		pred[3][2] = pred[1][0]
		pred[0][0] = uint8((l0 + 2*tl + t0 + 2) >> 2)
		pred[1][1] = pred[0][0]
		pred[2][2] = pred[0][0]
		pred[3][3] = pred[0][0]
		pred[0][1] = uint8((tl + 2*t0 + t1 + 2) >> 2)
		pred[1][2] = pred[0][1]
		pred[2][3] = pred[0][1]
		pred[0][2] = uint8((t0 + 2*t1 + t2 + 2) >> 2)
		pred[1][3] = pred[0][2]
		pred[0][3] = uint8((t1 + 2*t2 + t3 + 2) >> 2)
	case Intra4x4VerticalRight:
		pred[0][0] = uint8((tl + t0 + 1) >> 1)
		pred[0][1] = uint8((t0 + t1 + 1) >> 1)
		pred[0][2] = uint8((t1 + t2 + 1) >> 1)
		pred[0][3] = uint8((t2 + t3 + 1) >> 1)
		pred[1][0] = uint8((l0 + 2*tl + t0 + 2) >> 2)
		pred[1][1] = uint8((tl + 2*t0 + t1 + 2) >> 2)
		pred[1][2] = uint8((t0 + 2*t1 + t2 + 2) >> 2)
		pred[1][3] = uint8((t1 + 2*t2 + t3 + 2) >> 2)
		pred[2][0] = uint8((tl + 2*l0 + l1 + 2) >> 2)
		pred[2][1] = pred[0][0]
		pred[2][2] = pred[0][1]
		pred[2][3] = pred[0][2]
		pred[3][0] = uint8((l0 + 2*l1 + l2 + 2) >> 2)
		pred[3][1] = pred[1][0]
		pred[3][2] = pred[1][1]
		pred[3][3] = pred[1][2]
	case Intra4x4HorizontalDown:
		pred[0][0] = uint8((tl + l0 + 1) >> 1)
		pred[0][1] = uint8((l0 + 2*tl + t0 + 2) >> 2)
		pred[0][2] = uint8((tl + 2*t0 + t1 + 2) >> 2)
		pred[0][3] = uint8((t0 + 2*t1 + t2 + 2) >> 2)
		pred[1][0] = uint8((l0 + l1 + 1) >> 1)
		pred[1][1] = uint8((tl + 2*l0 + l1 + 2) >> 2)
		pred[1][2] = pred[0][0]
		pred[1][3] = pred[0][1]
		pred[2][0] = uint8((l1 + l2 + 1) >> 1)
		pred[2][1] = uint8((l0 + 2*l1 + l2 + 2) >> 2)
		pred[2][2] = pred[1][0]
		pred[2][3] = pred[1][1]
		pred[3][0] = uint8((l2 + l3 + 1) >> 1)
		pred[3][1] = uint8((l1 + 2*l2 + l3 + 2) >> 2)
		pred[3][2] = pred[2][0]
		pred[3][3] = pred[2][1]
	case Intra4x4VerticalLeft:
		pred[0][0] = uint8((t0 + t1 + 1) >> 1)
		pred[0][1] = uint8((t1 + t2 + 1) >> 1)
		pred[0][2] = uint8((t2 + t3 + 1) >> 1)
		pred[0][3] = uint8((t3 + t4 + 1) >> 1)
		pred[1][0] = uint8((t0 + 2*t1 + t2 + 2) >> 2)
		pred[1][1] = uint8((t1 + 2*t2 + t3 + 2) >> 2)
		pred[1][2] = uint8((t2 + 2*t3 + t4 + 2) >> 2)
		pred[1][3] = uint8((t3 + 2*t4 + t5 + 2) >> 2)
		pred[2][0] = uint8((t1 + t2 + 1) >> 1)
		pred[2][1] = uint8((t2 + t3 + 1) >> 1)
		pred[2][2] = uint8((t3 + t4 + 1) >> 1)
		pred[2][3] = uint8((t4 + t5 + 1) >> 1)
		pred[3][0] = uint8((t1 + 2*t2 + t3 + 2) >> 2)
		pred[3][1] = uint8((t2 + 2*t3 + t4 + 2) >> 2)
		pred[3][2] = uint8((t3 + 2*t4 + t5 + 2) >> 2)
		pred[3][3] = uint8((t4 + 2*t5 + t6 + 2) >> 2)
	case Intra4x4HorizontalUp:
		pred[0][0] = uint8((l0 + l1 + 1) >> 1)
		pred[0][1] = uint8((l0 + 2*l1 + l2 + 2) >> 2)
		pred[0][2] = uint8((l1 + l2 + 1) >> 1)
		pred[0][3] = uint8((l1 + 2*l2 + l3 + 2) >> 2)
		pred[1][0] = uint8((l1 + l2 + 1) >> 1)
		pred[1][1] = uint8((l1 + 2*l2 + l3 + 2) >> 2)
		pred[1][2] = uint8((l2 + l3 + 1) >> 1)
		pred[1][3] = uint8((l2 + 2*l3 + l3 + 2) >> 2)
		pred[2][0] = uint8((l2 + l3 + 1) >> 1)
		pred[2][1] = uint8((l2 + 2*l3 + l3 + 2) >> 2)
		pred[2][2] = uint8(l3)
		pred[2][3] = uint8(l3)
		pred[3][0] = uint8(l3)
		pred[3][1] = uint8(l3)
		pred[3][2] = uint8(l3)
		pred[3][3] = uint8(l3)
	}

	return pred
}

// Predict8x8 generates an 8x8 prediction block using filtered reference samples.
// ref contains 25 samples: 8 left (L7..L0 bottom to top), top-left (TL),
// 8 top (T0..T7), 8 top-right (T8..T15).
// ref layout: [L7..L0, TL, T0..T7, T8..T15]
// All samples should be pre-filtered.
// Predict8x8 generates an 8x8 prediction block (section 8.3.2).
// leftAvail/topAvail indicate whether left/top samples exist (affects DC mode).
func Predict8x8(mode int, ref [25]uint8, leftAvail, topAvail bool) [8][8]uint8 {
	var pred [8][8]uint8

	// Extract named references for clarity
	l := [8]int{int(ref[7]), int(ref[6]), int(ref[5]), int(ref[4]),
		int(ref[3]), int(ref[2]), int(ref[1]), int(ref[0])} // l[0]=L0, l[7]=L7
	tl := int(ref[8])
	t := [16]int{int(ref[9]), int(ref[10]), int(ref[11]), int(ref[12]),
		int(ref[13]), int(ref[14]), int(ref[15]), int(ref[16]),
		int(ref[17]), int(ref[18]), int(ref[19]), int(ref[20]),
		int(ref[21]), int(ref[22]), int(ref[23]), int(ref[24])} // t[0]=T0, t[15]=T15

	switch mode {
	case 0: // Vertical
		for y := range 8 {
			for x := range 8 {
				pred[y][x] = uint8(t[x])
			}
		}
	case 1: // Horizontal
		for y := range 8 {
			for x := range 8 {
				pred[y][x] = uint8(l[y])
			}
		}
	case 2: // DC
		var dc uint8
		switch {
		case topAvail && leftAvail:
			sum := 0
			for i := range 8 {
				sum += t[i] + l[i]
			}
			dc = uint8((sum + 8) >> 4)
		case topAvail:
			sum := 0
			for i := range 8 {
				sum += t[i]
			}
			dc = uint8((sum + 4) >> 3)
		case leftAvail:
			sum := 0
			for i := range 8 {
				sum += l[i]
			}
			dc = uint8((sum + 4) >> 3)
		default:
			dc = 128
		}
		for y := range 8 {
			for x := range 8 {
				pred[y][x] = dc
			}
		}
	case 3: // Diagonal Down-Left
		for y := range 8 {
			for x := range 8 {
				if x+y == 14 {
					pred[y][x] = uint8((t[14] + 3*t[15] + 2) >> 2)
				} else {
					pred[y][x] = uint8((t[x+y] + 2*t[x+y+1] + t[x+y+2] + 2) >> 2)
				}
			}
		}
	case 4: // Diagonal Down-Right
		predict8x8DiagDownRight(&pred, l, tl, t)
	case 5: // Vertical-Right
		predict8x8VerticalRight(&pred, l, tl, t)
	case 6: // Horizontal-Down
		predict8x8HorizontalDown(&pred, l, tl, t)
	case 7: // Vertical-Left
		for y := range 8 {
			for x := range 8 {
				if y%2 == 0 {
					pred[y][x] = uint8((t[x+y/2] + t[x+y/2+1] + 1) >> 1)
				} else {
					pred[y][x] = uint8((t[x+y/2] + 2*t[x+y/2+1] + t[x+y/2+2] + 2) >> 2)
				}
			}
		}
	case 8: // Horizontal-Up
		predict8x8HorizontalUp(&pred, l)
	}

	return pred
}

func predict8x8DiagDownRight(pred *[8][8]uint8, l [8]int, tl int, t [16]int) {
	// Build combined diagonal reference: diagonal samples from bottom-left to top-right
	// d[-7..0..7] where d[0]=tl, d[k>0]=t[k-1], d[k<0]=l[-k-1]
	for y := range 8 {
		for x := range 8 {
			d := x - y // diagonal offset
			var p0, p1, p2 int
			p1 = diagSample(d, l[:], tl, t[:])
			p0 = diagSample(d-1, l[:], tl, t[:])
			p2 = diagSample(d+1, l[:], tl, t[:])
			pred[y][x] = uint8((p0 + 2*p1 + p2 + 2) >> 2)
		}
	}
}

func diagSample(d int, l []int, tl int, t []int) int {
	if d == 0 {
		return tl
	}
	if d > 0 {
		return t[d-1]
	}
	return l[-d-1]
}

func predict8x8VerticalRight(pred *[8][8]uint8, l [8]int, tl int, t [16]int) {
	for y := range 8 {
		for x := range 8 {
			zVR := 2*x - y
			if zVR >= 0 && zVR%2 == 0 {
				i := x - (y >> 1)
				if i == 0 {
					pred[y][x] = uint8((tl + t[0] + 1) >> 1)
				} else {
					pred[y][x] = uint8((t[i-1] + t[i] + 1) >> 1)
				}
			} else if zVR >= 0 && zVR%2 != 0 {
				i := x - (y >> 1) - 1
				switch i {
				case -1:
					pred[y][x] = uint8((l[0] + 2*tl + t[0] + 2) >> 2)
				case 0:
					pred[y][x] = uint8((tl + 2*t[0] + t[1] + 2) >> 2)
				default:
					pred[y][x] = uint8((t[i-1] + 2*t[i] + t[i+1] + 2) >> 2)
				}
			} else if zVR == -1 {
				pred[y][x] = uint8((l[0] + 2*tl + t[0] + 2) >> 2)
			} else { // zVR < -1
				i := y - 2*x - 2
				if i == 0 {
					pred[y][x] = uint8((tl + 2*l[0] + l[1] + 2) >> 2)
				} else {
					pred[y][x] = uint8((l[i-1] + 2*l[i] + l[i+1] + 2) >> 2)
				}
			}
		}
	}
}

func predict8x8HorizontalDown(pred *[8][8]uint8, l [8]int, tl int, t [16]int) {
	for y := range 8 {
		for x := range 8 {
			zHD := 2*y - x
			if zHD >= 0 && zHD%2 == 0 {
				i := y - (x >> 1)
				if i == 0 {
					pred[y][x] = uint8((tl + l[0] + 1) >> 1)
				} else {
					pred[y][x] = uint8((l[i-1] + l[i] + 1) >> 1)
				}
			} else if zHD >= 0 && zHD%2 != 0 {
				i := y - (x >> 1) - 1
				switch i {
				case -1:
					pred[y][x] = uint8((t[0] + 2*tl + l[0] + 2) >> 2)
				case 0:
					pred[y][x] = uint8((tl + 2*l[0] + l[1] + 2) >> 2)
				default:
					pred[y][x] = uint8((l[i-1] + 2*l[i] + l[i+1] + 2) >> 2)
				}
			} else if zHD == -1 {
				pred[y][x] = uint8((t[0] + 2*tl + l[0] + 2) >> 2)
			} else { // zHD < -1
				i := x - 2*y - 2
				if i == 0 {
					pred[y][x] = uint8((tl + 2*t[0] + t[1] + 2) >> 2)
				} else {
					pred[y][x] = uint8((t[i-1] + 2*t[i] + t[i+1] + 2) >> 2)
				}
			}
		}
	}
}

func predict8x8HorizontalUp(pred *[8][8]uint8, l [8]int) {
	for y := range 8 {
		for x := range 8 {
			zHU := x + 2*y
			if zHU < 13 {
				i := y + (x >> 1)
				if zHU%2 == 0 {
					pred[y][x] = uint8((l[i] + l[i+1] + 1) >> 1)
				} else {
					pred[y][x] = uint8((l[i] + 2*l[i+1] + l[i+2] + 2) >> 2)
				}
			} else if zHU == 13 {
				pred[y][x] = uint8((l[6] + 3*l[7] + 2) >> 2)
			} else {
				pred[y][x] = uint8(l[7])
			}
		}
	}
}

// PredictChroma generates a chroma prediction block (8x8 for 4:2:0).
// section 8.3.4.
func PredictChroma(mode int, top, left []uint8, topLeft uint8, blockSize int) [8][8]uint8 {
	var pred [8][8]uint8

	switch mode {
	case IntraChromaDC:
		// For 4:2:0, chroma is 8x8 but divided into four 4x4 sub-blocks
		// Each sub-block uses available neighbors for its DC prediction
		if blockSize == 8 {
			predictChromaDC8x8(&pred, top, left)
		}
	case IntraChromaHorizontal:
		if left != nil {
			for y := range blockSize {
				for x := range blockSize {
					pred[y][x] = left[y]
				}
			}
		}
	case IntraChromaVertical:
		if top != nil {
			for y := range blockSize {
				for x := range blockSize {
					pred[y][x] = top[x]
				}
			}
		}
	case IntraChromaPlane:
		if top != nil && left != nil {
			xCF := 4 // for 8x8
			yCF := 4

			iH := 0
			iV := 0
			for x := range xCF {
				topRight := int(top[xCF+x])
				var topLeftRef int
				if xCF-2-x >= 0 {
					topLeftRef = int(top[xCF-2-x])
				} else {
					topLeftRef = int(topLeft) // p[-1,-1]
				}
				iH += (x + 1) * (topRight - topLeftRef)
			}
			for y := range yCF {
				bottomRef := int(left[yCF+y])
				var topLeftRef int
				if yCF-2-y >= 0 {
					topLeftRef = int(left[yCF-2-y])
				} else {
					topLeftRef = int(topLeft) // p[-1,-1]
				}
				iV += (y + 1) * (bottomRef - topLeftRef)
			}

			a := 16 * (int(top[2*xCF-1]) + int(left[2*yCF-1]))
			b := (34*iH + 32) >> 6
			c := (34*iV + 32) >> 6

			for y := range blockSize {
				for x := range blockSize {
					val := (a + b*(x-3) + c*(y-3) + 16) >> 5
					pred[y][x] = clip8(val)
				}
			}
		}
	}

	return pred
}

// predictChromaDC8x8 handles the DC mode for 8x8 chroma (4:2:0).
// Section 8.3.4.1: Each 4x4 sub-block uses specific neighbor samples:
//   - TL (0,0): top[0..3] + left[0..3] if both available
//   - TR (4,0): only top[4..7] (fallback to left[0..3] if no top)
//   - BL (0,4): only left[4..7] (fallback to top[0..3] if no left)
//   - BR (4,4): top[4..7] + left[4..7] if both available
func predictChromaDC8x8(pred *[8][8]uint8, top, left []uint8) {
	hasTop := top != nil
	hasLeft := left != nil

	// Four 4x4 sub-blocks: TL(0), TR(1), BL(2), BR(3)
	for blk := range 4 {
		x0 := (blk % 2) * 4
		y0 := (blk / 2) * 4

		sum := 0
		count := 0

		// Determine which samples to use per spec section 8.3.4.1
		isTopRow := blk < 2     // blocks 0, 1
		isLeftCol := blk%2 == 0 // blocks 0, 2

		useTop := false
		useLeft := false

		switch {
		case isTopRow && isLeftCol: // Block 0 (TL): use both if available
			useTop = hasTop
			useLeft = hasLeft
		case isTopRow && !isLeftCol: // Block 1 (TR): prefer top, fallback to left
			if hasTop {
				useTop = true
			} else if hasLeft {
				useLeft = true
			}
		case !isTopRow && isLeftCol: // Block 2 (BL): prefer left, fallback to top
			if hasLeft {
				useLeft = true
			} else if hasTop {
				useTop = true
			}
		default: // Block 3 (BR): use both if available
			useTop = hasTop
			useLeft = hasLeft
		}

		if useTop {
			for x := x0; x < x0+4; x++ {
				sum += int(top[x])
				count++
			}
		}

		if useLeft {
			for y := y0; y < y0+4; y++ {
				sum += int(left[y])
				count++
			}
		}

		var dc uint8
		if count > 0 {
			dc = uint8((sum + count/2) / count)
		} else {
			dc = 128
		}

		for y := y0; y < y0+4; y++ {
			for x := x0; x < x0+4; x++ {
				pred[y][x] = dc
			}
		}
	}
}

// clip8 clips a value to [0, 255].
func clip8(val int) uint8 {
	if val < 0 {
		return 0
	}
	if val > 255 {
		return 255
	}
	return uint8(val)
}
