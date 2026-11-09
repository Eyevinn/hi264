package encode

// planeMBKey identifies all inputs used by the current PlaneGrid prediction modes.
// Position is represented by top/left availability and reconstructed edge pixels,
// not by coordinates; add any new prediction inputs here before caching them.
type planeMBKey struct {
	qp                           int
	top, left                    bool
	luma, cb, cr                 [4]uint8
	topY, leftY                  [16]uint8
	topCb, leftCb, topCr, leftCr [8]uint8
}

// planeMBCache holds one analysis while encoding a CAVLC slice (a sequence of
// macroblocks). CABAC, the other supported entropy coding mode, does not use it.
//
// Repeated flat regions often have the same source values and prediction
// neighbors. When those inputs and the quantization parameter (QP) match, the
// encoder can reuse the analysis instead of predicting, transforming, and
// reconstructing the block again. A new cache is created for each slice.
//
// The cache never stores encoded bytes: CAVLC must encode each block using the
// current counts of nonzero coefficients in neighboring blocks.
type planeMBCache struct {
	valid  bool
	source planeMBSource
	key    planeMBKey
	result planeMBAnalysis
}

type planeMBSource struct {
	qp           int
	luma, cb, cr [4]uint8
}

// analyze reuses an analysis only when the source, QP, and prediction neighbors match.
// A source or QP change invalidates the entry without constructing its neighbor key.
// The next matching source establishes a full key; a later match can reuse the result.
// The returned pointer belongs to the cache and may change on the next call.
func (c *planeMBCache) analyze(e *FrameEncoder, mbX, mbY int, luma, cb, cr [4]uint8,
	reconY []uint8, strideY int, reconCb, reconCr []uint8, strideC int) *planeMBAnalysis {

	source := planeMBSource{qp: e.QP, luma: luma, cb: cb, cr: cr}
	if c.source != source {
		c.source, c.valid = source, false
		e.analyzeMBPlane(&c.result, mbX, mbY, luma, cb, cr,
			reconY, strideY, reconCb, reconCr, strideC)
		return &c.result
	}
	key := planeMBInputs(e.QP, mbX, mbY, luma, cb, cr, reconY, strideY, reconCb, reconCr, strideC)
	if !c.valid || c.key != key {
		e.analyzeMBPlane(&c.result, mbX, mbY, luma, cb, cr,
			reconY, strideY, reconCb, reconCr, strideC)
		c.key, c.valid = key, true
	}
	return &c.result
}

// planeMBInputs captures available reconstructed top and left edges for all three planes.
// Unavailable edges remain zero and are distinguished by the availability flags.
func planeMBInputs(qp, mbX, mbY int, luma, cb, cr [4]uint8,
	reconY []uint8, strideY int, reconCb, reconCr []uint8, strideC int) planeMBKey {

	key := planeMBKey{qp: qp, top: mbY > 0, left: mbX > 0, luma: luma, cb: cb, cr: cr}
	if key.top {
		y := (mbY*16-1)*strideY + mbX*16
		c := (mbY*8-1)*strideC + mbX*8
		copy(key.topY[:], reconY[y:y+16])
		copy(key.topCb[:], reconCb[c:c+8])
		copy(key.topCr[:], reconCr[c:c+8])
	}
	if key.left {
		for row := range 16 {
			key.leftY[row] = reconY[(mbY*16+row)*strideY+mbX*16-1]
		}
		for row := range 8 {
			offset := (mbY*8+row)*strideC + mbX*8 - 1
			key.leftCb[row], key.leftCr[row] = reconCb[offset], reconCr[offset]
		}
	}
	return key
}
