package encode

import (
	"github.com/Eyevinn/hi264/internal/cabac"
)

// scan2raster maps block scan index (0-15) to raster order (row*4+col) in the 4x4 grid.
// Computed from inverseRasterX4x4/Y4x4: raster = (by/4)*4 + (bx/4).
var scan2raster = [16]int{
	0, 1, 4, 5, 2, 3, 6, 7, 8, 9, 12, 13, 10, 11, 14, 15,
}

// zigzag4x4 maps zig-zag scan position to raster position (Table 8-13).
var zigzag4x4 = [16]int{
	0, 1, 4, 8, 5, 2, 3, 6, 9, 12, 13, 10, 7, 11, 14, 15,
}

// rasterToZigzag4x4 reorders a 16-element array from raster order to zig-zag scan order.
func rasterToZigzag4x4(raster [16]int32) [16]int32 {
	var zz [16]int32
	for scanPos, rasterPos := range zigzag4x4 {
		zz[scanPos] = raster[rasterPos]
	}
	return zz
}

// invZigzag4x4AC maps raster position (1-15) to zig-zag AC scan position (0-14).
// For AC coefficients, DC (position 0) is excluded, so zig-zag positions are 0-14
// mapping to raster positions {1, 4, 8, 5, 2, 3, 6, 9, 12, 13, 10, 7, 11, 14, 15}.
var zigzag4x4AC = [15]int{
	1, 4, 8, 5, 2, 3, 6,
	9, 12, 13, 10, 7, 11, 14, 15,
}

// rasterACToZigzag converts 15 AC coefficients from raster order (indices 1-15)
// to zig-zag scan order for the bitstream.
func rasterACToZigzag(rasterAC [15]int32) [15]int32 {
	var zz [15]int32
	for zzPos, rasterPos := range zigzag4x4AC {
		zz[zzPos] = rasterAC[rasterPos-1] // rasterAC is 0-indexed (positions 1-15 → indices 0-14)
	}
	return zz
}

// selectLumaModePlane picks the best I_16x16 luma prediction mode by evaluating
// per-pixel prediction error for all available modes. Returns the mode and the
// full per-pixel prediction array matching the decoder's behavior.
func selectLumaModePlane(reconY []uint8, strideY, mbX, mbY int, lumaVals [4]uint8) (mode int, predArray [256]uint8) {
	bestMode := 2
	bestPred := lumaPredict16x16(reconY, strideY, mbX, mbY, 2)
	bestErr := lumaPredErrorPlane(lumaVals, bestPred)

	if mbY > 0 {
		vPred := lumaPredict16x16(reconY, strideY, mbX, mbY, 0)
		vErr := lumaPredErrorPlane(lumaVals, vPred)
		if vErr < bestErr {
			bestMode, bestPred, bestErr = 0, vPred, vErr
		}
	}

	if mbX > 0 {
		hPred := lumaPredict16x16(reconY, strideY, mbX, mbY, 1)
		hErr := lumaPredErrorPlane(lumaVals, hPred)
		if hErr < bestErr {
			bestMode, bestPred, _ = 1, hPred, hErr
		}
	}

	return bestMode, bestPred
}

// lumaPredErrorPlane computes total absolute error between lumaVals (4 quadrants) and prediction array.
func lumaPredErrorPlane(lumaVals [4]uint8, pred [256]uint8) int {
	total := 0
	for y := range 16 {
		for x := range 16 {
			qr, qc := y/8, x/8
			val := int(lumaVals[qr*2+qc])
			total += absInt(val - int(pred[y*16+x]))
		}
	}
	return total
}

// selectChromaModePlane picks the best chroma prediction mode using per-pixel predictions.
func selectChromaModePlane(reconCb, reconCr []uint8, strideC, mbX, mbY int,
	cbVals, crVals [4]uint8) (mode int, cbPred, crPred [64]uint8) {

	bestMode := 0
	bestCbPred := chromaPredict8x8(reconCb, strideC, mbX, mbY, 0)
	bestCrPred := chromaPredict8x8(reconCr, strideC, mbX, mbY, 0)
	bestErr := chromaPredErrorPlane(cbVals, crVals, bestCbPred, bestCrPred)

	if mbY > 0 {
		vCb := chromaPredict8x8(reconCb, strideC, mbX, mbY, 2)
		vCr := chromaPredict8x8(reconCr, strideC, mbX, mbY, 2)
		vErr := chromaPredErrorPlane(cbVals, crVals, vCb, vCr)
		if vErr < bestErr {
			bestMode, bestCbPred, bestCrPred, bestErr = 2, vCb, vCr, vErr
		}
	}

	if mbX > 0 {
		hCb := chromaPredict8x8(reconCb, strideC, mbX, mbY, 1)
		hCr := chromaPredict8x8(reconCr, strideC, mbX, mbY, 1)
		hErr := chromaPredErrorPlane(cbVals, crVals, hCb, hCr)
		if hErr < bestErr {
			bestMode, bestCbPred, bestCrPred, _ = 1, hCb, hCr, hErr
		}
	}

	return bestMode, bestCbPred, bestCrPred
}

// chromaPredErrorPlane computes total prediction error for chroma using per-pixel predictions.
func chromaPredErrorPlane(cbVals, crVals [4]uint8, cbPred, crPred [64]uint8) int {
	total := 0
	for blk := range 4 {
		x0 := (blk % 2) * 4
		y0 := (blk / 2) * 4
		for y := range 4 {
			for x := range 4 {
				idx := (y0+y)*8 + x0 + x
				total += absInt(int(cbVals[blk]) - int(cbPred[idx]))
				total += absInt(int(crVals[blk]) - int(crPred[idx]))
			}
		}
	}
	return total
}

// chromaSubBlockDC computes the chroma DC coefficient for one 4x4 sub-block
// using per-pixel predictions. Returns the forward DCT DC = sum of residuals.
func chromaSubBlockDC(target uint8, predArray [64]uint8, x0, y0 int) int32 {
	sum := int32(0)
	for y := range 4 {
		for x := range 4 {
			sum += int32(target) - int32(predArray[(y0+y)*8+x0+x])
		}
	}
	return sum
}

// reconstructLumaPixel performs per-pixel luma reconstruction using a per-pixel prediction array.
// This matches the decoder's behavior for all I_16x16 prediction modes.
func reconstructLumaPixel(quantDC [16]int32, quantAC [16][15]int32,
	predArray [256]uint8, qp int, lumaCBP int) [256]uint8 {

	var result [256]uint8

	invHadamard := inverseHadamard4x4(quantDC)
	dequantMatrix := dequantDC4x4(invHadamard, qp, 16)

	for blk := range 16 {
		bx := inverseRasterX4x4[blk]
		by := inverseRasterY4x4[blk]

		var dqBlock [16]int32
		dqBlock[0] = dequantMatrix[scan2raster[blk]]

		if lumaCBP != 0 {
			qpPer := qp / 6
			qpRem := qp % 6
			for i := 1; i < 16; i++ {
				row := i / 4
				col := i % 4
				v := levelScaleIdx(row, col)
				ls := levelScale4x4[qpRem][v] * 16
				if qpPer >= 4 {
					dqBlock[i] = quantAC[blk][i-1] * ls << uint(qpPer-4)
				} else {
					dqBlock[i] = (quantAC[blk][i-1]*ls + (1 << uint(3-qpPer))) >> uint(4-qpPer)
				}
			}
		}

		dqBlock[0] += 32
		var temp [16]int32
		for i := range 4 {
			z0 := dqBlock[i*4+0] + dqBlock[i*4+2]
			z1 := dqBlock[i*4+0] - dqBlock[i*4+2]
			z2 := (dqBlock[i*4+1] >> 1) - dqBlock[i*4+3]
			z3 := dqBlock[i*4+1] + (dqBlock[i*4+3] >> 1)
			temp[i*4+0] = z0 + z3
			temp[i*4+1] = z1 + z2
			temp[i*4+2] = z1 - z2
			temp[i*4+3] = z0 - z3
		}
		for j := range 4 {
			z0 := temp[0*4+j] + temp[2*4+j]
			z1 := temp[0*4+j] - temp[2*4+j]
			z2 := (temp[1*4+j] >> 1) - temp[3*4+j]
			z3 := temp[1*4+j] + (temp[3*4+j] >> 1)

			px := bx + j
			for iy, residual := range [4]int32{
				(z0 + z3) >> 6,
				(z1 + z2) >> 6,
				(z1 - z2) >> 6,
				(z0 - z3) >> 6,
			} {
				py := by + iy
				val := int32(predArray[py*16+px]) + residual
				result[py*16+px] = clipU8(int(val))
			}
		}
	}

	return result
}

// reconstructChromaPixel performs per-pixel chroma reconstruction for one plane.
// Each 4x4 sub-block gets a uniform DC residual added to the per-pixel prediction.
func reconstructChromaPixel(quantDC [4]int32, predArray [64]uint8, qpc int,
	recon []uint8, strideC, mbX, mbY int) {

	invH := inverseHadamard2x2(quantDC)
	dcScaled := dequantChromaDC2x2(invH, qpc, 16)

	for blk := range 4 {
		x0 := (blk % 2) * 4
		y0 := (blk / 2) * 4
		residual := (dcScaled[blk] + 32) >> 6
		for y := range 4 {
			off := (mbY*8+y0+y)*strideC + mbX*8 + x0
			for x := range 4 {
				pred := int32(predArray[(y0+y)*8+x0+x])
				recon[off+x] = clipU8(int(pred + residual))
			}
		}
	}
}

// encodeMBPlane encodes a macroblock using PlaneGrid quad values (CAVLC).
func (e *FrameEncoder) encodeMBPlane(w *BitWriter, mbX, mbY int,
	lumaVals [4]uint8, cbVals, crVals [4]uint8,
	nCLuma [][]int, nCCb, nCCr [][]int,
	reconY []uint8, strideY int, reconCb, reconCr []uint8, strideC int) error {

	qp := e.QP
	qpc := ChromaQP(qp)

	// Select best luma prediction mode using per-pixel predictions
	lumaMode, lumaPredArray := selectLumaModePlane(reconY, strideY, mbX, mbY, lumaVals)

	// Compute per-4x4-block DC and AC coefficients using per-pixel residuals
	var dcMatrix [16]int32
	var acCoeffs [16][15]int32
	lumaCBP := 0

	for blk := range 16 {
		bx := inverseRasterX4x4[blk]
		by := inverseRasterY4x4[blk]
		var res [16]int32
		for r := range 4 {
			for c := range 4 {
				py, px := by+r, bx+c
				qr, qc := py/8, px/8
				val := lumaVals[qr*2+qc]
				res[r*4+c] = int32(val) - int32(lumaPredArray[py*16+px])
			}
		}
		fwd := ForwardTransform4x4(res)
		dcMatrix[scan2raster[blk]] = fwd[0]
		copy(acCoeffs[blk][:], fwd[1:])
	}

	hadamardResult := ForwardHadamard4x4(dcMatrix)
	quantDC := QuantizeDC4x4(hadamardResult, qp, 16)

	// Quantize AC coefficients and check if any are non-zero
	var quantAC [16][15]int32
	for blk := range 16 {
		var fullBlock [16]int32
		copy(fullBlock[1:], acCoeffs[blk][:])
		qBlock := Quantize4x4(fullBlock, qp)
		copy(quantAC[blk][:], qBlock[1:])
		for _, v := range quantAC[blk] {
			if v != 0 {
				lumaCBP = 1
				break
			}
		}
	}

	// Select best chroma prediction mode using per-pixel predictions
	chromaMode, cbPredArray, crPredArray := selectChromaModePlane(reconCb, reconCr, strideC, mbX, mbY, cbVals, crVals)

	// Chroma DC: per-sub-block residuals using exact sum
	var cbDCMatrix [4]int32
	var crDCMatrix [4]int32
	for i := range 4 {
		x0 := (i % 2) * 4
		y0 := (i / 2) * 4
		cbDCMatrix[i] = chromaSubBlockDC(cbVals[i], cbPredArray, x0, y0)
		crDCMatrix[i] = chromaSubBlockDC(crVals[i], crPredArray, x0, y0)
	}
	cbHadamard := ForwardHadamard2x2(cbDCMatrix)
	crHadamard := ForwardHadamard2x2(crDCMatrix)
	quantCbDC := QuantizeChromaDC2x2(cbHadamard, qpc)
	quantCrDC := QuantizeChromaDC2x2(crHadamard, qpc)

	chromaCBP := 0
	for i := range 4 {
		if quantCbDC[i] != 0 || quantCrDC[i] != 0 {
			chromaCBP = 1
			break
		}
	}

	// Determine I_16x16 mb_type
	mbType := 1 + lumaMode + 4*chromaCBP + 12*lumaCBP

	// Write mb_type as ue(v)
	w.WriteUE(uint32(mbType))

	// intra_chroma_pred_mode
	w.WriteUE(uint32(chromaMode))

	// mb_qp_delta = 0
	w.WriteSE(0)

	// Intra16x16DCLevel — bitstream uses zig-zag scan order
	dcZZ := rasterToZigzag4x4(quantDC)
	nCDC := computeNC4x4(nCLuma, mbX*4, mbY*4)
	EncodeResidualBlock(w, dcZZ[:], nCDC, 16)

	// Intra16x16ACLevel — bitstream uses zig-zag scan order (skipping DC)
	if lumaCBP != 0 {
		for blk := range 16 {
			bx := inverseRasterX4x4[blk]
			by := inverseRasterY4x4[blk]
			acZZ := rasterACToZigzag(quantAC[blk])
			nC := computeNC4x4(nCLuma, mbX*4+bx/4, mbY*4+by/4)
			nNZ := EncodeResidualBlock(w, acZZ[:], nC, 15)
			nCLuma[mbY*4+by/4][mbX*4+bx/4] = nNZ
		}
	} else {
		for blk := range 16 {
			bx := inverseRasterX4x4[blk]
			by := inverseRasterY4x4[blk]
			nCLuma[mbY*4+by/4][mbX*4+bx/4] = 0
		}
	}

	// Chroma
	if chromaCBP > 0 {
		EncodeResidualBlock(w, quantCbDC[:], -1, 4)
		EncodeResidualBlock(w, quantCrDC[:], -1, 4)
	}

	// Update chroma nC (all zero for DC-only chroma blocks)
	for blk := range 4 {
		bx := (blk % 2)
		by := (blk / 2)
		nCCb[mbY*2+by][mbX*2+bx] = 0
		nCCr[mbY*2+by][mbX*2+bx] = 0
	}

	// Update reconstructed luma pixels (per-pixel prediction)
	reconLuma := reconstructLumaPixel(quantDC, quantAC, lumaPredArray, qp, lumaCBP)
	for y := range 16 {
		off := (mbY*16+y)*strideY + mbX*16
		copy(reconY[off:off+16], reconLuma[y*16:y*16+16])
	}

	// Update reconstructed chroma pixels (per-pixel prediction)
	reconstructChromaPixel(quantCbDC, cbPredArray, qpc, reconCb, strideC, mbX, mbY)
	reconstructChromaPixel(quantCrDC, crPredArray, qpc, reconCr, strideC, mbX, mbY)

	return nil
}

// encodeMBCABACPlane encodes a macroblock using PlaneGrid quad values (CABAC).
func (e *FrameEncoder) encodeMBCABACPlane(enc *cabac.Encoder, ctx []cabac.CtxState,
	mbStates []encMBState, mbIdx, mbX, mbY int,
	lumaVals [4]uint8, cbVals, crVals [4]uint8,
	mbWidth, _ int,
	reconY []uint8, strideY int, reconCb, reconCr []uint8, strideC int) error {

	qp := e.QP
	qpc := ChromaQP(qp)

	// Select best luma prediction mode using per-pixel predictions
	lumaMode, lumaPredArray := selectLumaModePlane(reconY, strideY, mbX, mbY, lumaVals)

	// Compute per-4x4-block DC and AC coefficients using per-pixel residuals
	var dcMatrix [16]int32
	var acCoeffs [16][15]int32
	lumaCBP := 0

	for blk := range 16 {
		bx := inverseRasterX4x4[blk]
		by := inverseRasterY4x4[blk]
		var res [16]int32
		for r := range 4 {
			for c := range 4 {
				py, px := by+r, bx+c
				qr, qc := py/8, px/8
				val := lumaVals[qr*2+qc]
				res[r*4+c] = int32(val) - int32(lumaPredArray[py*16+px])
			}
		}
		fwd := ForwardTransform4x4(res)
		dcMatrix[scan2raster[blk]] = fwd[0]
		copy(acCoeffs[blk][:], fwd[1:])
	}

	hadamardResult := ForwardHadamard4x4(dcMatrix)
	quantDC := QuantizeDC4x4(hadamardResult, qp, 16)

	// Quantize AC coefficients and check if any are non-zero
	var quantAC [16][15]int32
	for blk := range 16 {
		var fullBlock [16]int32
		copy(fullBlock[1:], acCoeffs[blk][:])
		qBlock := Quantize4x4(fullBlock, qp)
		copy(quantAC[blk][:], qBlock[1:])
		for _, v := range quantAC[blk] {
			if v != 0 {
				lumaCBP = 1
				break
			}
		}
	}

	// Select best chroma prediction mode using per-pixel predictions
	chromaMode, cbPredArray, crPredArray := selectChromaModePlane(reconCb, reconCr, strideC, mbX, mbY, cbVals, crVals)

	// Chroma DC: per-sub-block residuals using exact sum
	var cbDCMatrix [4]int32
	var crDCMatrix [4]int32
	for i := range 4 {
		x0 := (i % 2) * 4
		y0 := (i / 2) * 4
		cbDCMatrix[i] = chromaSubBlockDC(cbVals[i], cbPredArray, x0, y0)
		crDCMatrix[i] = chromaSubBlockDC(crVals[i], crPredArray, x0, y0)
	}
	cbHadamard := ForwardHadamard2x2(cbDCMatrix)
	crHadamard := ForwardHadamard2x2(crDCMatrix)
	quantCbDC := QuantizeChromaDC2x2(cbHadamard, qpc)
	quantCrDC := QuantizeChromaDC2x2(crHadamard, qpc)

	chromaCBP := 0
	for i := range 4 {
		if quantCbDC[i] != 0 || quantCrDC[i] != 0 {
			chromaCBP = 1
			break
		}
	}

	mbType := 1 + lumaMode + 4*chromaCBP + 12*lumaCBP

	// --- CABAC encoding of syntax elements ---

	var leftState, topState *encMBState
	if mbX > 0 {
		leftState = &mbStates[mbIdx-1]
	}
	if mbY > 0 {
		topState = &mbStates[mbIdx-mbWidth]
	}

	leftNotINxN := leftState != nil
	topNotINxN := topState != nil
	encodeMBTypeI16x16(enc, ctx, mbType, leftNotINxN, topNotINxN)

	leftChromaNZ := leftState != nil && leftState.intraChromaPredMode != 0
	topChromaNZ := topState != nil && topState.intraChromaPredMode != 0
	encodeChromaPredMode(enc, ctx, chromaMode, leftChromaNZ, topChromaNZ)

	encodeQPDelta(enc, ctx, 0, false)

	// Residual: Intra16x16DCLevel — bitstream uses zig-zag scan order
	dcCBFCtx := deriveDCCBFCtx(leftState, topState)
	dcZZ := rasterToZigzag4x4(quantDC)
	dcCBF := encodeResidualBlockCABAC(enc, ctx, encCtxBlockCatIntra16x16DC, dcCBFCtx, dcZZ[:], 16)
	mbStates[mbIdx].codedBlockFlag[encCtxBlockCatIntra16x16DC][0] = dcCBF

	// Intra16x16ACLevel — bitstream uses zig-zag scan order (skipping DC)
	if lumaCBP != 0 {
		for blk := range 16 {
			cbfCtx := deriveACCBFCtx(mbStates, mbIdx, mbX, mbY, mbWidth, blk)
			acZZ := rasterACToZigzag(quantAC[blk])
			cbf := encodeResidualBlockCABAC(enc, ctx, encCtxBlockCatIntra16x16AC, cbfCtx, acZZ[:], 15)
			mbStates[mbIdx].codedBlockFlag[encCtxBlockCatIntra16x16AC][blk] = cbf
		}
	} else {
		for blk := range 16 {
			mbStates[mbIdx].codedBlockFlag[encCtxBlockCatIntra16x16AC][blk] = 0
		}
	}

	// Chroma DC
	if chromaCBP > 0 {
		cbCBFCtx := deriveChromaDCCBFCtx(leftState, topState, 0)
		cbCBF := encodeResidualBlockCABAC(enc, ctx, encCtxBlockCatChromaDC, cbCBFCtx, quantCbDC[:], 4)
		mbStates[mbIdx].codedBlockFlag[encCtxBlockCatChromaDC][0] = cbCBF

		crCBFCtx := deriveChromaDCCBFCtx(leftState, topState, 1)
		crCBF := encodeResidualBlockCABAC(enc, ctx, encCtxBlockCatChromaDC, crCBFCtx, quantCrDC[:], 4)
		mbStates[mbIdx].codedBlockFlag[encCtxBlockCatChromaDC][1] = crCBF
	}

	// Update MB state
	mbStates[mbIdx].mbType = mbType
	mbStates[mbIdx].intraChromaPredMode = chromaMode
	mbStates[mbIdx].cbpChroma = chromaCBP

	// Update reconstructed luma pixels (per-pixel prediction)
	reconLuma := reconstructLumaPixel(quantDC, quantAC, lumaPredArray, qp, lumaCBP)
	for y := range 16 {
		off := (mbY*16+y)*strideY + mbX*16
		copy(reconY[off:off+16], reconLuma[y*16:y*16+16])
	}

	// Update reconstructed chroma pixels (per-pixel prediction)
	reconstructChromaPixel(quantCbDC, cbPredArray, qpc, reconCb, strideC, mbX, mbY)
	reconstructChromaPixel(quantCrDC, crPredArray, qpc, reconCr, strideC, mbX, mbY)

	return nil
}

// deriveACCBFCtx derives the coded_block_flag context for Intra16x16AC blocks.
func deriveACCBFCtx(mbStates []encMBState, mbIdx, mbX, mbY, mbWidth, blk int) int {
	bx := inverseRasterX4x4[blk] / 4 // 0-3 column index within MB
	by := inverseRasterY4x4[blk] / 4 // 0-3 row index within MB

	condA := 1 // default when neighbor unavailable
	condB := 1

	// Left neighbor
	if bx > 0 {
		// Same MB: find the block to the left
		leftBlk := findBlock4x4(bx-1, by)
		condA = int(mbStates[mbIdx].codedBlockFlag[encCtxBlockCatIntra16x16AC][leftBlk])
	} else if mbX > 0 {
		// Previous MB: rightmost column (bx=3)
		leftBlk := findBlock4x4(3, by)
		condA = int(mbStates[mbIdx-1].codedBlockFlag[encCtxBlockCatIntra16x16AC][leftBlk])
	}

	// Top neighbor
	if by > 0 {
		topBlk := findBlock4x4(bx, by-1)
		condB = int(mbStates[mbIdx].codedBlockFlag[encCtxBlockCatIntra16x16AC][topBlk])
	} else if mbY > 0 {
		topBlk := findBlock4x4(bx, 3)
		condB = int(mbStates[mbIdx-mbWidth].codedBlockFlag[encCtxBlockCatIntra16x16AC][topBlk])
	}

	return condA + 2*condB
}

// findBlock4x4 returns the block index for the 4x4 block at position (bx, by) within an MB.
// bx and by are in 4x4-block units (0-3).
func findBlock4x4(bx, by int) int {
	// Inverse of inverseRasterX4x4/inverseRasterY4x4: find block index from (x,y) position.
	// The 4x4 block scan order maps (bx,by) → block index.
	return rasterScan4x4[by*4+bx]
}

// rasterScan4x4 maps (row*4+col) in 4x4-block units to block scan index.
var rasterScan4x4 = [16]int{
	0, 1, 4, 5,
	2, 3, 6, 7,
	8, 9, 12, 13,
	10, 11, 14, 15,
}
