package frame

import "github.com/Eyevinn/hi264/internal/slice"

// H.264 deblocking filter lookup tables (Tables 8-16 to 8-18 of the spec).
// Indexed by indexA/indexB = qp + 52 + offset. First 52 entries are zero-padding,
// entries 52-103 are the actual values, entries 104-155 are clamped to max.

var alphaTable = [156]int{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 4, 4, 5, 6,
	7, 8, 9, 10, 12, 13, 15, 17, 20, 22,
	25, 28, 32, 36, 40, 45, 50, 56, 63, 71,
	80, 90, 101, 113, 127, 144, 162, 182, 203, 226,
	255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
}

var betaTable = [156]int{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 2, 2, 2, 3,
	3, 3, 3, 4, 4, 4, 6, 6, 7, 7,
	8, 8, 9, 9, 10, 10, 11, 11, 12, 12,
	13, 13, 14, 14, 15, 15, 16, 16, 17, 17,
	18, 18,
	18, 18, 18, 18, 18, 18, 18, 18, 18, 18, 18, 18, 18,
	18, 18, 18, 18, 18, 18, 18, 18, 18, 18, 18, 18, 18,
	18, 18, 18, 18, 18, 18, 18, 18, 18, 18, 18, 18, 18,
	18, 18, 18, 18, 18, 18, 18, 18, 18, 18, 18, 18, 18,
}

// tc0Table[indexA][bS]. Column 0 is bS=0 (always -1), columns 1-3 are bS=1,2,3.
var tc0Table = [156][4]int{
	{-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0},
	{-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0},
	{-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0},
	{-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0},
	{-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0},
	{-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0},
	{-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0},
	{-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0},
	{-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0},
	{-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0},
	{-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0},
	{-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 0}, {-1, 0, 0, 1},
	{-1, 0, 0, 1}, {-1, 0, 0, 1}, {-1, 0, 0, 1}, {-1, 0, 1, 1}, {-1, 0, 1, 1}, {-1, 1, 1, 1},
	{-1, 1, 1, 1}, {-1, 1, 1, 1}, {-1, 1, 1, 1}, {-1, 1, 1, 2}, {-1, 1, 1, 2}, {-1, 1, 1, 2},
	{-1, 1, 1, 2}, {-1, 1, 2, 3}, {-1, 1, 2, 3}, {-1, 2, 2, 3}, {-1, 2, 2, 4}, {-1, 2, 3, 4},
	{-1, 2, 3, 4}, {-1, 3, 3, 5}, {-1, 3, 4, 6}, {-1, 3, 4, 6}, {-1, 4, 5, 7}, {-1, 4, 5, 8},
	{-1, 4, 6, 9}, {-1, 5, 7, 10}, {-1, 6, 8, 11}, {-1, 6, 8, 13}, {-1, 7, 10, 14}, {-1, 8, 11, 16},
	{-1, 9, 12, 18}, {-1, 10, 13, 20}, {-1, 11, 15, 23}, {-1, 13, 17, 25},
	{-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25},
	{-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25},
	{-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25},
	{-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25},
	{-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25},
	{-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25},
	{-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25},
	{-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25},
	{-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25}, {-1, 13, 17, 25},
}

// Chroma QP mapping table (Table 8-15 of the spec).
var chromaQPFromLuma = [52]int{
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19,
	20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 29, 30, 31, 32, 32, 33, 34, 34,
	35, 35, 36, 36, 37, 37, 37, 38, 38, 38, 39, 39, 39, 39,
}

func chromaQPForDeblock(lumaQP, chromaQPIndexOffset int) int {
	qpi := max(lumaQP+chromaQPIndexOffset, 0)
	if qpi > 51 {
		qpi = 51
	}
	return chromaQPFromLuma[qpi]
}

func clipInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clipPixel(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Deblock applies the H.264 deblocking filter to the frame.
// filterOffsetA and filterOffsetB are SliceAlphaC0OffsetDiv2*2 and SliceBetaOffsetDiv2*2.
func Deblock(f *Frame, sc *slice.SliceContext, filterOffsetA, filterOffsetB int) {
	a := 52 + filterOffsetA
	b := 52 + filterOffsetB
	chromaQpOff := sc.ChromaQpIndexOffset

	for mbIdx := 0; mbIdx < sc.TotalMBs; mbIdx++ {
		mbX := mbIdx % sc.MBWidth
		mbY := mbIdx / sc.MBWidth
		mb := &sc.MBs[mbIdx]

		qp := mb.QPY
		is8x8 := mb.TransformSize8x8

		hasLeft := mbX > 0
		hasTop := mbY > 0

		x0 := mbX * 16
		y0 := mbY * 16
		cx0 := mbX * 8
		cy0 := mbY * 8

		qpc := chromaQPForDeblock(qp, chromaQpOff)

		// === VERTICAL EDGES (filtering across vertical boundaries) ===

		// Edge 0: left MB boundary (bS=4, strong intra filter)
		if hasLeft {
			qpLeft := sc.MBs[mbIdx-1].QPY
			qpAvg := (qp + qpLeft + 1) >> 1
			filterLumaIntraV(f.Y, f.StrideY, x0, y0, qpAvg+a, qpAvg+b)

			qpcLeft := chromaQPForDeblock(qpLeft, chromaQpOff)
			qpcAvg := (qpc + qpcLeft + 1) >> 1
			filterChromaIntraV(f.Cb, f.StrideC, cx0, cy0, qpcAvg+a, qpcAvg+b)
			filterChromaIntraV(f.Cr, f.StrideC, cx0, cy0, qpcAvg+a, qpcAvg+b)
		}

		// Internal vertical luma edges (bS=3, normal filter)
		if is8x8 {
			// 8x8 DCT: only one internal edge at x+8
			filterLumaNormalV(f.Y, f.StrideY, x0+8, y0, qp+a, qp+b, 3)
		} else {
			// 4x4: three internal edges at x+4, x+8, x+12
			filterLumaNormalV(f.Y, f.StrideY, x0+4, y0, qp+a, qp+b, 3)
			filterLumaNormalV(f.Y, f.StrideY, x0+8, y0, qp+a, qp+b, 3)
			filterLumaNormalV(f.Y, f.StrideY, x0+12, y0, qp+a, qp+b, 3)
		}
		// Internal vertical chroma edge at cx+4 (bS=3)
		filterChromaNormalV(f.Cb, f.StrideC, cx0+4, cy0, qpc+a, qpc+b, 3)
		filterChromaNormalV(f.Cr, f.StrideC, cx0+4, cy0, qpc+a, qpc+b, 3)

		// === HORIZONTAL EDGES (filtering across horizontal boundaries) ===

		// Edge 0: top MB boundary (bS=4, strong intra filter)
		if hasTop {
			qpTop := sc.MBs[mbIdx-sc.MBWidth].QPY
			qpAvg := (qp + qpTop + 1) >> 1
			filterLumaIntraH(f.Y, f.StrideY, x0, y0, qpAvg+a, qpAvg+b)

			qpcTop := chromaQPForDeblock(qpTop, chromaQpOff)
			qpcAvg := (qpc + qpcTop + 1) >> 1
			filterChromaIntraH(f.Cb, f.StrideC, cx0, cy0, qpcAvg+a, qpcAvg+b)
			filterChromaIntraH(f.Cr, f.StrideC, cx0, cy0, qpcAvg+a, qpcAvg+b)
		}

		// Internal horizontal luma edges (bS=3, normal filter)
		if is8x8 {
			filterLumaNormalH(f.Y, f.StrideY, x0, y0+8, qp+a, qp+b, 3)
		} else {
			filterLumaNormalH(f.Y, f.StrideY, x0, y0+4, qp+a, qp+b, 3)
			filterLumaNormalH(f.Y, f.StrideY, x0, y0+8, qp+a, qp+b, 3)
			filterLumaNormalH(f.Y, f.StrideY, x0, y0+12, qp+a, qp+b, 3)
		}
		// Internal horizontal chroma edge at cy+4 (bS=3)
		filterChromaNormalH(f.Cb, f.StrideC, cx0, cy0+4, qpc+a, qpc+b, 3)
		filterChromaNormalH(f.Cr, f.StrideC, cx0, cy0+4, qpc+a, qpc+b, 3)
	}
}

// filterLumaIntraV filters a vertical luma edge with the strong intra filter (bS=4).
// edgeX is the x-coordinate of q0 (first pixel to the right of the edge).
// Processes 16 rows starting at y0.
func filterLumaIntraV(plane []uint8, stride, edgeX, y0, indexA, indexB int) {
	alpha := alphaTable[indexA]
	beta := betaTable[indexB]
	if alpha == 0 || beta == 0 {
		return
	}

	for d := range 16 {
		off := (y0+d)*stride + edgeX

		p0 := int(plane[off-1])
		p1 := int(plane[off-2])
		p2 := int(plane[off-3])
		q0 := int(plane[off])
		q1 := int(plane[off+1])
		q2 := int(plane[off+2])

		if absInt(p0-q0) >= alpha || absInt(p1-p0) >= beta || absInt(q1-q0) >= beta {
			continue
		}

		if absInt(p0-q0) < ((alpha >> 2) + 2) {
			if absInt(p2-p0) < beta {
				p3 := int(plane[off-4])
				plane[off-1] = uint8((p2 + 2*p1 + 2*p0 + 2*q0 + q1 + 4) >> 3)
				plane[off-2] = uint8((p2 + p1 + p0 + q0 + 2) >> 2)
				plane[off-3] = uint8((2*p3 + 3*p2 + p1 + p0 + q0 + 4) >> 3)
			} else {
				plane[off-1] = uint8((2*p1 + p0 + q1 + 2) >> 2)
			}
			if absInt(q2-q0) < beta {
				q3 := int(plane[off+3])
				plane[off] = uint8((p1 + 2*p0 + 2*q0 + 2*q1 + q2 + 4) >> 3)
				plane[off+1] = uint8((p0 + q0 + q1 + q2 + 2) >> 2)
				plane[off+2] = uint8((2*q3 + 3*q2 + q1 + q0 + p0 + 4) >> 3)
			} else {
				plane[off] = uint8((2*q1 + q0 + p1 + 2) >> 2)
			}
		} else {
			plane[off-1] = uint8((2*p1 + p0 + q1 + 2) >> 2)
			plane[off] = uint8((2*q1 + q0 + p1 + 2) >> 2)
		}
	}
}

// filterLumaIntraH filters a horizontal luma edge with the strong intra filter (bS=4).
// edgeY is the y-coordinate of q0. Processes 16 columns starting at x0.
func filterLumaIntraH(plane []uint8, stride, x0, edgeY, indexA, indexB int) {
	alpha := alphaTable[indexA]
	beta := betaTable[indexB]
	if alpha == 0 || beta == 0 {
		return
	}

	for d := range 16 {
		x := x0 + d

		p0 := int(plane[(edgeY-1)*stride+x])
		p1 := int(plane[(edgeY-2)*stride+x])
		p2 := int(plane[(edgeY-3)*stride+x])
		q0 := int(plane[edgeY*stride+x])
		q1 := int(plane[(edgeY+1)*stride+x])
		q2 := int(plane[(edgeY+2)*stride+x])

		if absInt(p0-q0) >= alpha || absInt(p1-p0) >= beta || absInt(q1-q0) >= beta {
			continue
		}

		if absInt(p0-q0) < ((alpha >> 2) + 2) {
			if absInt(p2-p0) < beta {
				p3 := int(plane[(edgeY-4)*stride+x])
				plane[(edgeY-1)*stride+x] = uint8((p2 + 2*p1 + 2*p0 + 2*q0 + q1 + 4) >> 3)
				plane[(edgeY-2)*stride+x] = uint8((p2 + p1 + p0 + q0 + 2) >> 2)
				plane[(edgeY-3)*stride+x] = uint8((2*p3 + 3*p2 + p1 + p0 + q0 + 4) >> 3)
			} else {
				plane[(edgeY-1)*stride+x] = uint8((2*p1 + p0 + q1 + 2) >> 2)
			}
			if absInt(q2-q0) < beta {
				q3 := int(plane[(edgeY+3)*stride+x])
				plane[edgeY*stride+x] = uint8((p1 + 2*p0 + 2*q0 + 2*q1 + q2 + 4) >> 3)
				plane[(edgeY+1)*stride+x] = uint8((p0 + q0 + q1 + q2 + 2) >> 2)
				plane[(edgeY+2)*stride+x] = uint8((2*q3 + 3*q2 + q1 + q0 + p0 + 4) >> 3)
			} else {
				plane[edgeY*stride+x] = uint8((2*q1 + q0 + p1 + 2) >> 2)
			}
		} else {
			plane[(edgeY-1)*stride+x] = uint8((2*p1 + p0 + q1 + 2) >> 2)
			plane[edgeY*stride+x] = uint8((2*q1 + q0 + p1 + 2) >> 2)
		}
	}
}

// filterLumaNormalV filters a vertical luma edge with the normal filter (bS < 4).
// edgeX is the x-coordinate of q0. Processes 16 rows starting at y0.
func filterLumaNormalV(plane []uint8, stride, edgeX, y0, indexA, indexB, bS int) {
	alpha := alphaTable[indexA]
	beta := betaTable[indexB]
	if alpha == 0 || beta == 0 {
		return
	}

	tcOrig := tc0Table[indexA][bS]
	if tcOrig < 0 {
		return
	}

	for d := range 16 {
		off := (y0+d)*stride + edgeX

		p0 := int(plane[off-1])
		p1 := int(plane[off-2])
		p2 := int(plane[off-3])
		q0 := int(plane[off])
		q1 := int(plane[off+1])
		q2 := int(plane[off+2])

		if absInt(p0-q0) >= alpha || absInt(p1-p0) >= beta || absInt(q1-q0) >= beta {
			continue
		}

		tc := tcOrig
		if absInt(p2-p0) < beta {
			if tcOrig > 0 {
				plane[off-2] = clipPixel(p1 + clipInt(((p2+((p0+q0+1)>>1))>>1)-p1, -tcOrig, tcOrig))
			}
			tc++
		}
		if absInt(q2-q0) < beta {
			if tcOrig > 0 {
				plane[off+1] = clipPixel(q1 + clipInt(((q2+((p0+q0+1)>>1))>>1)-q1, -tcOrig, tcOrig))
			}
			tc++
		}

		delta := clipInt(((q0-p0)*4+(p1-q1)+4)>>3, -tc, tc)
		plane[off-1] = clipPixel(p0 + delta)
		plane[off] = clipPixel(q0 - delta)
	}
}

// filterLumaNormalH filters a horizontal luma edge with the normal filter (bS < 4).
// edgeY is the y-coordinate of q0. Processes 16 columns starting at x0.
func filterLumaNormalH(plane []uint8, stride, x0, edgeY, indexA, indexB, bS int) {
	alpha := alphaTable[indexA]
	beta := betaTable[indexB]
	if alpha == 0 || beta == 0 {
		return
	}

	tcOrig := tc0Table[indexA][bS]
	if tcOrig < 0 {
		return
	}

	for d := range 16 {
		x := x0 + d

		p0 := int(plane[(edgeY-1)*stride+x])
		p1 := int(plane[(edgeY-2)*stride+x])
		p2 := int(plane[(edgeY-3)*stride+x])
		q0 := int(plane[edgeY*stride+x])
		q1 := int(plane[(edgeY+1)*stride+x])
		q2 := int(plane[(edgeY+2)*stride+x])

		if absInt(p0-q0) >= alpha || absInt(p1-p0) >= beta || absInt(q1-q0) >= beta {
			continue
		}

		tc := tcOrig
		if absInt(p2-p0) < beta {
			if tcOrig > 0 {
				plane[(edgeY-2)*stride+x] = clipPixel(p1 + clipInt(((p2+((p0+q0+1)>>1))>>1)-p1, -tcOrig, tcOrig))
			}
			tc++
		}
		if absInt(q2-q0) < beta {
			if tcOrig > 0 {
				plane[(edgeY+1)*stride+x] = clipPixel(q1 + clipInt(((q2+((p0+q0+1)>>1))>>1)-q1, -tcOrig, tcOrig))
			}
			tc++
		}

		delta := clipInt(((q0-p0)*4+(p1-q1)+4)>>3, -tc, tc)
		plane[(edgeY-1)*stride+x] = clipPixel(p0 + delta)
		plane[edgeY*stride+x] = clipPixel(q0 - delta)
	}
}

// filterChromaIntraV filters a vertical chroma edge with the strong intra filter (bS=4).
// edgeX is the x-coordinate of q0. Processes 8 rows starting at y0.
func filterChromaIntraV(plane []uint8, stride, edgeX, y0, indexA, indexB int) {
	alpha := alphaTable[indexA]
	beta := betaTable[indexB]
	if alpha == 0 || beta == 0 {
		return
	}

	for d := range 8 {
		off := (y0+d)*stride + edgeX

		p0 := int(plane[off-1])
		p1 := int(plane[off-2])
		q0 := int(plane[off])
		q1 := int(plane[off+1])

		if absInt(p0-q0) >= alpha || absInt(p1-p0) >= beta || absInt(q1-q0) >= beta {
			continue
		}

		plane[off-1] = uint8((2*p1 + p0 + q1 + 2) >> 2)
		plane[off] = uint8((2*q1 + q0 + p1 + 2) >> 2)
	}
}

// filterChromaIntraH filters a horizontal chroma edge with the strong intra filter (bS=4).
// edgeY is the y-coordinate of q0. Processes 8 columns starting at x0.
func filterChromaIntraH(plane []uint8, stride, x0, edgeY, indexA, indexB int) {
	alpha := alphaTable[indexA]
	beta := betaTable[indexB]
	if alpha == 0 || beta == 0 {
		return
	}

	for d := range 8 {
		x := x0 + d

		p0 := int(plane[(edgeY-1)*stride+x])
		p1 := int(plane[(edgeY-2)*stride+x])
		q0 := int(plane[edgeY*stride+x])
		q1 := int(plane[(edgeY+1)*stride+x])

		if absInt(p0-q0) >= alpha || absInt(p1-p0) >= beta || absInt(q1-q0) >= beta {
			continue
		}

		plane[(edgeY-1)*stride+x] = uint8((2*p1 + p0 + q1 + 2) >> 2)
		plane[edgeY*stride+x] = uint8((2*q1 + q0 + p1 + 2) >> 2)
	}
}

// filterChromaNormalV filters a vertical chroma edge with the normal filter (bS < 4).
// edgeX is the x-coordinate of q0. Processes 8 rows starting at y0.
func filterChromaNormalV(plane []uint8, stride, edgeX, y0, indexA, indexB, bS int) {
	alpha := alphaTable[indexA]
	beta := betaTable[indexB]
	if alpha == 0 || beta == 0 {
		return
	}

	tc := tc0Table[indexA][bS] + 1
	if tc <= 0 {
		return
	}

	for d := range 8 {
		off := (y0+d)*stride + edgeX

		p0 := int(plane[off-1])
		p1 := int(plane[off-2])
		q0 := int(plane[off])
		q1 := int(plane[off+1])

		if absInt(p0-q0) >= alpha || absInt(p1-p0) >= beta || absInt(q1-q0) >= beta {
			continue
		}

		delta := clipInt(((q0-p0)*4+(p1-q1)+4)>>3, -tc, tc)
		plane[off-1] = clipPixel(p0 + delta)
		plane[off] = clipPixel(q0 - delta)
	}
}

// filterChromaNormalH filters a horizontal chroma edge with the normal filter (bS < 4).
// edgeY is the y-coordinate of q0. Processes 8 columns starting at x0.
func filterChromaNormalH(plane []uint8, stride, x0, edgeY, indexA, indexB, bS int) {
	alpha := alphaTable[indexA]
	beta := betaTable[indexB]
	if alpha == 0 || beta == 0 {
		return
	}

	tc := tc0Table[indexA][bS] + 1
	if tc <= 0 {
		return
	}

	for d := range 8 {
		x := x0 + d

		p0 := int(plane[(edgeY-1)*stride+x])
		p1 := int(plane[(edgeY-2)*stride+x])
		q0 := int(plane[edgeY*stride+x])
		q1 := int(plane[(edgeY+1)*stride+x])

		if absInt(p0-q0) >= alpha || absInt(p1-p0) >= beta || absInt(q1-q0) >= beta {
			continue
		}

		delta := clipInt(((q0-p0)*4+(p1-q1)+4)>>3, -tc, tc)
		plane[(edgeY-1)*stride+x] = clipPixel(p0 + delta)
		plane[edgeY*stride+x] = clipPixel(q0 - delta)
	}
}
