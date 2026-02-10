package slice

// DecodeMBTypeIntra decodes mb_type for I-slices using CABAC (Table 9-34).
// ctxIdxOffset = 3 for I-slices.
// Returns the mb_type value (0=I_NxN, 1-24=I_16x16_x_y_z, 25=I_PCM).
func DecodeMBTypeIntra(sc *SliceContext, mbIdx int) int {
	d := sc.Cabac
	ctx := sc.Ctx

	// First bin: ctx 3, condTermFlagA/B based on neighbor mb_type != I_NxN
	ctxIdxInc := 0
	if mbA := sc.MBAvailA(mbIdx); mbA != nil && mbA.MBType != MBTypeINxN {
		ctxIdxInc++
	}
	if mbB := sc.MBAvailB(mbIdx); mbB != nil && mbB.MBType != MBTypeINxN {
		ctxIdxInc++
	}

	bin0 := d.DecodeDecision(&ctx[3+ctxIdxInc])
	if bin0 == 0 {
		return MBTypeINxN // I_NxN
	}

	// DecodeTerminate for I_PCM check
	binT := d.DecodeTerminate()
	if binT == 1 {
		return MBTypeIPCM // I_PCM
	}

	// I_16x16 sub-type: bins encode cbp_luma(1bit), cbp_chroma(2bits), pred_mode(2bits)
	// Bin 1 (ctx 6): cbp_luma (0 or non-zero)
	bin1 := d.DecodeDecision(&ctx[6])

	// Bin 2 (ctx 7): first bit of cbp_chroma
	bin2 := d.DecodeDecision(&ctx[7])

	// Bin 3 (ctx 7 or 8): second bit of cbp_chroma (if bin2=1)
	var cbpChroma int
	if bin2 == 1 {
		bin3 := d.DecodeDecision(&ctx[8])
		if bin3 == 1 {
			cbpChroma = 2
		} else {
			cbpChroma = 1
		}
	}

	// Bin 4,5 (ctx 9, 10): prediction mode (2 bits, FL cMax=3)
	bin4 := d.DecodeDecision(&ctx[9])
	bin5 := d.DecodeDecision(&ctx[10])
	predMode := int(bin4)*2 + int(bin5)

	// mb_type = 1 + predMode + 4*cbpChroma + (cbp_luma ? 12 : 0)
	mbType := 1 + predMode + 4*cbpChroma
	if bin1 == 1 {
		mbType += 12
	}

	return mbType
}

// DecodeTransformSize8x8Flag decodes transform_size_8x8_flag using CABAC.
// Context indices 399-401 (ctxIdxOffset=399).
func DecodeTransformSize8x8Flag(sc *SliceContext, mbIdx int) bool {
	ctx := sc.Ctx
	ctxIdxInc := 0
	mbA := sc.MBAvailA(mbIdx)
	mbB := sc.MBAvailB(mbIdx)

	if mbA != nil && mbA.TransformSize8x8 {
		ctxIdxInc++
	}
	if mbB != nil && mbB.TransformSize8x8 {
		ctxIdxInc++
	}
	decision := sc.Cabac.DecodeDecision(&ctx[399+ctxIdxInc])
	return decision == 1
}

// DecodeIntraChromaPredMode decodes intra_chroma_pred_mode using CABAC.
// Context indices 64-67 (ctxIdxOffset=64), TU binarization with cMax=3.
func DecodeIntraChromaPredMode(sc *SliceContext, mbIdx int) int {
	ctx := sc.Ctx

	// ctxIdxInc for first bin depends on neighbor intra_chroma_pred_mode != 0
	ctxIdxInc := 0
	if mbA := sc.MBAvailA(mbIdx); mbA != nil && mbA.IntraChromaPredMode != 0 {
		ctxIdxInc++
	}
	if mbB := sc.MBAvailB(mbIdx); mbB != nil && mbB.IntraChromaPredMode != 0 {
		ctxIdxInc++
	}

	// TU binarization: unary up to cMax=3
	bin0 := sc.Cabac.DecodeDecision(&ctx[64+ctxIdxInc])
	if bin0 == 0 {
		return 0
	}
	bin1 := sc.Cabac.DecodeDecision(&ctx[67]) // ctx offset 3 for subsequent bins
	if bin1 == 0 {
		return 1
	}
	bin2 := sc.Cabac.DecodeDecision(&ctx[67])
	if bin2 == 0 {
		return 2
	}
	return 3
}

// DecodeIntra4x4PredMode decodes prev_intra4x4_pred_mode_flag and rem_intra4x4_pred_mode.
// Context 68 for flag, 69 for rem bits.
func DecodeIntra4x4PredMode(sc *SliceContext) (prevFlag bool, rem int) {
	ctx := sc.Ctx
	flag := sc.Cabac.DecodeDecision(&ctx[68])
	if flag == 1 {
		return true, -1
	}
	// 3 bypass bins for rem_intra4x4_pred_mode (FL, cMax=7)
	rem = int(sc.Cabac.DecodeDecision(&ctx[69]))
	rem |= int(sc.Cabac.DecodeDecision(&ctx[69])) << 1
	rem |= int(sc.Cabac.DecodeDecision(&ctx[69])) << 2
	return false, rem
}

// DecodeIntra8x8PredMode decodes prev_intra8x8_pred_mode_flag and rem_intra8x8_pred_mode.
// Same context indices as 4x4 (68, 69).
func DecodeIntra8x8PredMode(sc *SliceContext) (prevFlag bool, rem int) {
	return DecodeIntra4x4PredMode(sc) // same syntax
}

// DecodeCBP decodes coded_block_pattern for I_NxN using CABAC.
// Luma CBP: ctx 73-76, Chroma CBP: ctx 77-84.
func DecodeCBP(sc *SliceContext, mbIdx int) (cbpLuma, cbpChroma int) {
	ctx := sc.Ctx

	// Luma CBP: 4 bins for 4 8x8 blocks
	// Each bin uses ctx 73+ctxIdxInc where ctxIdxInc depends on neighbor cbp
	for i := 0; i < 4; i++ {
		// Get neighbor CBP bits for context derivation
		// Bit ordering: 0=top-left, 1=top-right, 2=bottom-left, 3=bottom-right
		ctxIdxInc := deriveCBPLumaCtx(sc, mbIdx, i, cbpLuma)
		bin := sc.Cabac.DecodeDecision(&ctx[73+ctxIdxInc])
		if bin == 1 {
			cbpLuma |= 1 << uint(i)
		}
	}

	// Chroma CBP: first bin (ctx 77+ctxIdxInc), then if 1, second bin (ctx 81+ctxIdxInc)
	if sc.ChromaArrayType == 1 || sc.ChromaArrayType == 2 {
		ctxIdxInc := deriveCBPChromaCtx(sc, mbIdx, false)
		bin0 := sc.Cabac.DecodeDecision(&ctx[77+ctxIdxInc])
		if bin0 == 1 {
			ctxIdxInc2 := deriveCBPChromaCtx(sc, mbIdx, true)
			bin1 := sc.Cabac.DecodeDecision(&ctx[81+ctxIdxInc2])
			if bin1 == 1 {
				cbpChroma = 2 // both DC and AC
			} else {
				cbpChroma = 1 // DC only
			}
		}
	}

	return cbpLuma, cbpChroma
}

// deriveCBPLumaCtx derives the ctxIdxInc for coded_block_pattern luma bins.
// blkIdx is 0-3 for the four 8x8 blocks.
// currentCBP is the partially-decoded cbpLuma (bits already decoded for this MB).
func deriveCBPLumaCtx(sc *SliceContext, mbIdx, blkIdx int, currentCBP int) int {
	// condTermFlagA: 0 if left neighbor's corresponding luma CBP bit is 1 (coded), else 1
	// condTermFlagB: 0 if top neighbor's corresponding luma CBP bit is 1 (coded), else 1
	var condA, condB int

	// neighborCBPBit returns 0 if the neighbor's CBP luma bit is set (coded), 1 otherwise.
	// Per spec 9.3.3.1.1.3: when neighbor is not available, condTermFlagN = 0 (treat as coded).
	neighborCBPBit := func(mb *MBData, bit int) int {
		if mb == nil {
			return 0 // not available: treat as coded (spec Table 9-39)
		}
		if mb.CBPLuma&bit != 0 {
			return 0 // coded
		}
		return 1 // not coded
	}

	switch blkIdx {
	case 0: // top-left 8x8: left=mbA bit1, top=mbB bit2
		condA = neighborCBPBit(sc.MBAvailA(mbIdx), 2)
		condB = neighborCBPBit(sc.MBAvailB(mbIdx), 4)
	case 1: // top-right 8x8: left=current bit0, top=mbB bit3
		if currentCBP&1 != 0 {
			condA = 0
		} else {
			condA = 1
		}
		condB = neighborCBPBit(sc.MBAvailB(mbIdx), 8)
	case 2: // bottom-left 8x8: left=mbA bit3, top=current bit0
		condA = neighborCBPBit(sc.MBAvailA(mbIdx), 8)
		if currentCBP&1 != 0 {
			condB = 0
		} else {
			condB = 1
		}
	case 3: // bottom-right 8x8: left=current bit2, top=current bit1
		if currentCBP&4 != 0 {
			condA = 0
		} else {
			condA = 1
		}
		if currentCBP&2 != 0 {
			condB = 0
		} else {
			condB = 1
		}
	}

	return condA + 2*condB
}

// deriveCBPChromaCtx derives ctxIdxInc for chroma CBP bins.
func deriveCBPChromaCtx(sc *SliceContext, mbIdx int, secondBin bool) int {
	var condA, condB int

	if mbA := sc.MBAvailA(mbIdx); mbA != nil {
		if !secondBin {
			if mbA.CBPChroma > 0 {
				condA = 1
			}
		} else {
			if mbA.CBPChroma > 1 {
				condA = 1
			}
		}
	}
	if mbB := sc.MBAvailB(mbIdx); mbB != nil {
		if !secondBin {
			if mbB.CBPChroma > 0 {
				condB = 1
			}
		} else {
			if mbB.CBPChroma > 1 {
				condB = 1
			}
		}
	}

	return condA + 2*condB
}

// DecodeQPDelta decodes mb_qp_delta using CABAC (ctx 60-63).
func DecodeQPDelta(sc *SliceContext) int {
	ctx := sc.Ctx
	d := sc.Cabac

	// First bin: ctx 60 if previous MB qp_delta was 0, ctx 61 otherwise
	ctxIdx := 60
	if sc.PrevMBQPDeltaNonZero {
		ctxIdx = 61
	}

	bin0 := d.DecodeDecision(&ctx[ctxIdx])
	if bin0 == 0 {
		return 0
	}

	// Subsequent bins: unary, ctx 62 then 63
	val := 1
	for {
		ctxIdx := 62
		if val > 1 {
			ctxIdx = 63
		}
		bin := d.DecodeDecision(&ctx[ctxIdx])
		if bin == 0 {
			break
		}
		val++
	}

	// Map unary code to signed value:
	// unary 1 -> +1, 2 -> -1, 3 -> +2, 4 -> -2, etc.
	if val%2 == 1 {
		return (val + 1) / 2
	}
	return -(val / 2)
}

// zScanToBlockX maps z-scan 4x4 block index to 4x4 block column (0-3).
var zScanToBlockX = [16]int{0, 1, 0, 1, 2, 3, 2, 3, 0, 1, 0, 1, 2, 3, 2, 3}

// zScanToBlockY maps z-scan 4x4 block index to 4x4 block row (0-3).
var zScanToBlockY = [16]int{0, 0, 1, 1, 0, 0, 1, 1, 2, 2, 3, 3, 2, 2, 3, 3}

// rasterToZScan maps 4x4 block raster index (by*4+bx) to z-scan index.
var rasterToZScan = [16]int{0, 1, 4, 5, 2, 3, 6, 7, 8, 9, 12, 13, 10, 11, 14, 15}

// derivePredIntra4x4PredMode derives the predicted intra 4x4 prediction mode
// from the modes of the left and top neighbor blocks.
func derivePredIntra4x4PredMode(sc *SliceContext, mbIdx, blkIdx int) int {
	predModeA := -1
	predModeB := -1

	// Convert z-scan block index to 4x4 block coordinates
	bx := zScanToBlockX[blkIdx]
	by := zScanToBlockY[blkIdx]

	if bx > 0 {
		// Left neighbor is within same MB
		leftIdx := rasterToZScan[by*4+(bx-1)]
		predModeA = sc.MBs[mbIdx].Intra4x4PredMode[leftIdx]
	} else if mbA := sc.MBAvailA(mbIdx); mbA != nil {
		if mbA.MBType == MBTypeINxN && mbA.TransformSize8x8 {
			// Left MB uses I_8x8: right-edge 8x8 block at row by
			predModeA = mbA.Intra8x8PredMode[(by/2)*2+1]
		} else if mbA.MBType == MBTypeINxN {
			// Right edge of left MB at same row
			rightIdx := rasterToZScan[by*4+3]
			predModeA = mbA.Intra4x4PredMode[rightIdx]
		} else {
			predModeA = 2 // DC for I_16x16 neighbor
		}
	}

	if by > 0 {
		// Top neighbor is within same MB
		topIdx := rasterToZScan[(by-1)*4+bx]
		predModeB = sc.MBs[mbIdx].Intra4x4PredMode[topIdx]
	} else if mbB := sc.MBAvailB(mbIdx); mbB != nil {
		if mbB.MBType == MBTypeINxN && mbB.TransformSize8x8 {
			// Top MB uses I_8x8: bottom-edge 8x8 block at col bx
			predModeB = mbB.Intra8x8PredMode[2+bx/2]
		} else if mbB.MBType == MBTypeINxN {
			// Bottom edge of top MB at same column
			botIdx := rasterToZScan[3*4+bx]
			predModeB = mbB.Intra4x4PredMode[botIdx]
		} else {
			predModeB = 2 // DC
		}
	}

	if predModeA < 0 || predModeB < 0 {
		return 2 // DC mode as default when neighbor not available
	}
	if predModeA < predModeB {
		return predModeA
	}
	return predModeB
}

// derivePredIntra8x8PredMode derives the predicted intra 8x8 prediction mode
// from the modes of the left and top neighbor blocks.
func derivePredIntra8x8PredMode(sc *SliceContext, mbIdx, blk8x8Idx int) int {
	predModeA := -1
	predModeB := -1

	x := blk8x8Idx % 2
	y := blk8x8Idx / 2

	if x > 0 {
		predModeA = sc.MBs[mbIdx].Intra8x8PredMode[blk8x8Idx-1]
	} else if mbA := sc.MBAvailA(mbIdx); mbA != nil {
		if mbA.MBType == MBTypeINxN && mbA.TransformSize8x8 {
			predModeA = mbA.Intra8x8PredMode[y*2+1]
		} else if mbA.MBType == MBTypeINxN {
			// Right edge of left MB at 4x4 row 2*y: position (3, 2*y) in 4x4 units
			rightIdx := rasterToZScan[2*y*4+3]
			predModeA = mbA.Intra4x4PredMode[rightIdx]
		} else {
			predModeA = 2
		}
	}

	if y > 0 {
		predModeB = sc.MBs[mbIdx].Intra8x8PredMode[blk8x8Idx-2]
	} else if mbB := sc.MBAvailB(mbIdx); mbB != nil {
		if mbB.MBType == MBTypeINxN && mbB.TransformSize8x8 {
			predModeB = mbB.Intra8x8PredMode[2+x]
		} else if mbB.MBType == MBTypeINxN {
			// Bottom edge of top MB at 4x4 col 2*x: position (2*x, 3) in 4x4 units
			botIdx := rasterToZScan[3*4+2*x]
			predModeB = mbB.Intra4x4PredMode[botIdx]
		} else {
			predModeB = 2
		}
	}

	if predModeA < 0 || predModeB < 0 {
		return 2
	}
	if predModeA < predModeB {
		return predModeA
	}
	return predModeB
}
