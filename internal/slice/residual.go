package slice

import (
	"github.com/Eyevinn/hi264/internal/cabac"
)

// Block category constants (Table 9-42).
const (
	CtxBlockCatIntra16x16DC = 0
	CtxBlockCatIntra16x16AC = 1
	CtxBlockCatLuma4x4      = 2
	CtxBlockCatChromaDC     = 3
	CtxBlockCatChromaAC     = 4
	CtxBlockCatLuma8x8      = 5
)

// ctxIdxBlockCatOffset for coded_block_flag (Table 9-40).
var codedBlockFlagOffset = [6]int{85, 89, 93, 97, 101, 1012}

// ctxIdxBlockCatOffset for significant_coeff_flag (Table 9-42, frame mode).
var significantCoeffFlagOffset = [6]int{105, 120, 134, 149, 152, 402}

// ctxIdxBlockCatOffset for last_significant_coeff_flag (Table 9-42, frame mode).
var lastSignificantCoeffFlagOffset = [6]int{166, 181, 195, 210, 213, 417}

// ctxIdxBlockCatOffset for coeff_abs_level_minus1 (Table 9-42).
var coeffAbsLevelMinus1Offset = [6]int{227, 237, 247, 257, 266, 426}

// lastCoeffFlagOffset8x8 maps position to context offset for 8x8 last_significant_coeff_flag.
var lastCoeffFlagOffset8x8 = [63]int{
	0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2,
	3, 3, 3, 3, 3, 3, 3, 3, 4, 4, 4, 4, 4, 4, 4, 4,
	5, 5, 5, 5, 6, 6, 6, 6, 7, 7, 7, 7, 8, 8, 8,
}

// Node-based context tables for coeff_abs_level_minus1 (section 9.3.3.1.1.5).
// node_ctx: 0-3 = numAbsLevelEq1 (with numAbsLevelGt1==0)
//           4-7 = numAbsLevelGt1 + 3 (numAbsLevelEq1 doesn't matter)

// coeffAbsLevel1Ctx maps node_ctx to CABAC ctxIdxInc for binIdx=0.
var coeffAbsLevel1Ctx = [8]int{1, 2, 3, 4, 0, 0, 0, 0}

// coeffAbsLevelGt1Ctx maps node_ctx to CABAC ctxIdxInc for binIdx>0.
var coeffAbsLevelGt1Ctx = [8]int{5, 5, 5, 5, 6, 7, 8, 9}

// coeffAbsLevelTransition[0] = after level==1, [1] = after level>1.
var coeffAbsLevelTransition = [2][8]int{
	{1, 2, 3, 3, 4, 5, 6, 7},
	{4, 4, 4, 4, 5, 6, 7, 7},
}

// DecodeResidual decodes residual coefficients for a block using CABAC.
// ctxBlockCat identifies the type of block (DC, AC, 4x4, 8x8, etc.).
// blkIdx is the block index within the MB for coded_block_flag context derivation.
// Returns the decoded coefficient levels in scan order.
func DecodeResidual(sc *SliceContext, mbIdx int, ctxBlockCat int, blkIdx int, maxCoeff int) []int32 {
	// Use pre-allocated buffer on SliceContext; clear only the portion we need
	coeffs := sc.coeffBuf[:maxCoeff]
	for i := range coeffs {
		coeffs[i] = 0
	}

	// coded_block_flag
	if ctxBlockCat != CtxBlockCatLuma8x8 {
		cbfCtx := deriveCodedBlockFlagCtx(sc, mbIdx, ctxBlockCat, blkIdx)
		ctxIdx := codedBlockFlagOffset[ctxBlockCat] + cbfCtx
		cbf := sc.Cabac.DecodeDecision(&sc.Ctx[ctxIdx])
		sc.MBs[mbIdx].CodedBlockFlag[ctxBlockCat][blkIdx] = cbf
		if cbf == 0 {
			return coeffs
		}
	}

	// Determine significant coefficient positions
	numCoeff := maxCoeff
	if ctxBlockCat == CtxBlockCatLuma8x8 {
		numCoeff = 64
	}

	// significant_coeff_flag and last_significant_coeff_flag
	// Use pre-allocated buffer; clear only the portion we need
	sigFlags := sc.sigFlags[:numCoeff]
	for i := range sigFlags {
		sigFlags[i] = false
	}
	lastIdx := numCoeff - 1 // last position is implicitly significant

	for i := 0; i < numCoeff-1; i++ {
		sigCtx := deriveSignificantCoeffFlagCtx(ctxBlockCat, i)
		sigCtxIdx := significantCoeffFlagOffset[ctxBlockCat] + sigCtx
		sig := sc.Cabac.DecodeDecision(&sc.Ctx[sigCtxIdx])

		if sig == 1 {
			sigFlags[i] = true

			lastCtx := deriveLastSignificantCoeffFlagCtx(ctxBlockCat, i)
			lastCtxIdx := lastSignificantCoeffFlagOffset[ctxBlockCat] + lastCtx
			last := sc.Cabac.DecodeDecision(&sc.Ctx[lastCtxIdx])
			if last == 1 {
				lastIdx = i
				break
			}
		}
	}

	if lastIdx == numCoeff-1 {
		sigFlags[lastIdx] = true
	}

	// Collect significant coefficient indices (reverse order for level decoding)
	nSig := 0
	for i := lastIdx; i >= 0; i-- {
		if sigFlags[i] {
			sc.sigIndices[nSig] = i
			nSig++
		}
	}

	// Decode coefficient levels (in reverse scan order) using node-based state machine
	nodeCtx := 0
	baseCtx := coeffAbsLevelMinus1Offset[ctxBlockCat]

	for _, idx := range sc.sigIndices[:nSig] {
		// binIdx=0: decode first bin using level1 context
		ctxIdxInc := coeffAbsLevel1Ctx[nodeCtx]
		bin := sc.Cabac.DecodeDecision(&sc.Ctx[baseCtx+ctxIdxInc])

		if bin == 0 {
			// level == 1
			nodeCtx = coeffAbsLevelTransition[0][nodeCtx]
			sign := sc.Cabac.DecodeBypass()
			if sign == 1 {
				coeffs[idx] = -1
			} else {
				coeffs[idx] = 1
			}
		} else {
			// level >= 2: use gt1 context for subsequent prefix bins
			ctxIdxInc = coeffAbsLevelGt1Ctx[nodeCtx]
			nodeCtx = coeffAbsLevelTransition[1][nodeCtx]

			coeffAbs := int32(2)
			for coeffAbs < 15 {
				bin := sc.Cabac.DecodeDecision(&sc.Ctx[baseCtx+ctxIdxInc])
				if bin == 0 {
					break
				}
				coeffAbs++
			}

			if coeffAbs >= 15 {
				suffix := decodeExpGolombBypass(sc.Cabac)
				coeffAbs += int32(suffix)
			}

			sign := sc.Cabac.DecodeBypass()
			if sign == 1 {
				coeffs[idx] = -coeffAbs
			} else {
				coeffs[idx] = coeffAbs
			}
		}
	}

	return coeffs
}

// decodeExpGolombBypass decodes a 0th-order Exp-Golomb code using bypass bins.
func decodeExpGolombBypass(d *cabac.Decoder) uint32 {
	k := uint(0)
	for {
		bin := d.DecodeBypass()
		if bin == 0 {
			break
		}
		k++
	}
	// Read k bits for the suffix
	var val uint32
	if k > 0 {
		val = d.ReadBypassU(int(k))
	}
	return (1 << k) - 1 + val
}

// deriveCodedBlockFlagCtx derives the context index increment for coded_block_flag.
func deriveCodedBlockFlagCtx(sc *SliceContext, mbIdx int, ctxBlockCat int, blkIdx int) int {
	// condTermFlagA: coded_block_flag of left neighbor block
	// condTermFlagB: coded_block_flag of top neighbor block
	// Both default to 1 if not available

	condA := 1
	condB := 1

	// For simplicity in I-frame decoding, use available neighbor data
	// The spec has complex block-level neighbor derivation (section 9.3.3.1.1.9)
	// For now, use a simplified version based on MB-level neighbors

	switch ctxBlockCat {
	case CtxBlockCatIntra16x16DC:
		if mbA := sc.MBAvailA(mbIdx); mbA != nil {
			condA = int(mbA.CodedBlockFlag[ctxBlockCat][0])
		}
		if mbB := sc.MBAvailB(mbIdx); mbB != nil {
			condB = int(mbB.CodedBlockFlag[ctxBlockCat][0])
		}
	case CtxBlockCatIntra16x16AC, CtxBlockCatLuma4x4:
		// Block-level neighbor derivation
		condA, condB = deriveLumaBlockNeighborCBF(sc, mbIdx, ctxBlockCat, blkIdx)
	case CtxBlockCatChromaDC:
		if mbA := sc.MBAvailA(mbIdx); mbA != nil {
			condA = int(mbA.CodedBlockFlag[ctxBlockCat][blkIdx])
		}
		if mbB := sc.MBAvailB(mbIdx); mbB != nil {
			condB = int(mbB.CodedBlockFlag[ctxBlockCat][blkIdx])
		}
	case CtxBlockCatChromaAC:
		condA, condB = deriveChromaACBlockNeighborCBF(sc, mbIdx, blkIdx)
	}

	return condA + 2*condB
}

// Luma 4x4 block spatial neighbor tables.
// H.264 4x4 blocks use Z-scan within 8x8 blocks, NOT simple raster scan.
// Block layout:  0  1  4  5
//
//	 2  3  6  7
//	 8  9 12 13
//	10 11 14 15
//
// -1 means neighbor is in adjacent MB.
var lumaLeftNeighbor = [16]int{-1, 0, -1, 2, 1, 4, 3, 6, -1, 8, -1, 10, 9, 12, 11, 14}
var lumaTopNeighbor = [16]int{-1, -1, 0, 1, -1, -1, 4, 5, 2, 3, 8, 9, 6, 7, 12, 13}

// When left neighbor is in MB_A, use these block indices (right column at each y row).
var lumaLeftFromMBA = [16]int{5, -1, 7, -1, -1, -1, -1, -1, 13, -1, 15, -1, -1, -1, -1, -1}

// When top neighbor is in MB_B, use these block indices (bottom row at each x column).
var lumaTopFromMBB = [16]int{10, 11, -1, -1, 14, 15, -1, -1, -1, -1, -1, -1, -1, -1, -1, -1}

// getLumaBlockCBF returns the coded_block_flag for a luma 4x4 block,
// checking the appropriate ctxBlockCat based on the neighbor MB's type.
// This is needed because I_16x16 stores CBF under category 1, I_4x4 under category 2,
// and I_8x8 under category 5, but the spec requires checking the neighbor's actual CBF
// regardless of the current block's category.
func getLumaBlockCBF(mb *MBData, blkIdx int) int {
	if mb.MBType == MBTypeIPCM {
		return 1
	}
	if mb.MBType >= 1 && mb.MBType <= 24 {
		// I_16x16: CBF stored under Intra16x16AC (category 1)
		return int(mb.CodedBlockFlag[CtxBlockCatIntra16x16AC][blkIdx])
	}
	if mb.MBType == MBTypeINxN {
		if mb.TransformSize8x8 {
			// I_8x8: CBF stored per 8x8 block under Luma8x8 (category 5)
			return int(mb.CodedBlockFlag[CtxBlockCatLuma8x8][blkIdx/4])
		}
		// I_4x4: CBF stored per 4x4 block under Luma4x4 (category 2)
		return int(mb.CodedBlockFlag[CtxBlockCatLuma4x4][blkIdx])
	}
	return 0
}

// deriveLumaBlockNeighborCBF derives coded_block_flag context for luma blocks.
func deriveLumaBlockNeighborCBF(sc *SliceContext, mbIdx int, ctxBlockCat int, blkIdx int) (condA, condB int) {
	condA = 1
	condB = 1

	// Left neighbor
	leftIdx := lumaLeftNeighbor[blkIdx]
	if leftIdx >= 0 {
		// Same MB: use current ctxBlockCat
		condA = int(sc.MBs[mbIdx].CodedBlockFlag[ctxBlockCat][leftIdx])
	} else if mbA := sc.MBAvailA(mbIdx); mbA != nil {
		// Different MB: use neighbor's actual luma CBF category
		condA = getLumaBlockCBF(mbA, lumaLeftFromMBA[blkIdx])
	}

	// Top neighbor
	topIdx := lumaTopNeighbor[blkIdx]
	if topIdx >= 0 {
		// Same MB: use current ctxBlockCat
		condB = int(sc.MBs[mbIdx].CodedBlockFlag[ctxBlockCat][topIdx])
	} else if mbB := sc.MBAvailB(mbIdx); mbB != nil {
		// Different MB: use neighbor's actual luma CBF category
		condB = getLumaBlockCBF(mbB, lumaTopFromMBB[blkIdx])
	}

	return condA, condB
}

// deriveChromaACBlockNeighborCBF derives coded_block_flag context for chroma AC blocks.
// blkIdx is iCbCr*4 + i, where i is the component-local block index (0-3 for 4:2:0).
func deriveChromaACBlockNeighborCBF(sc *SliceContext, mbIdx int, blkIdx int) (condA, condB int) {
	condA = 1
	condB = 1

	// Derive component and component-local index
	compBase := (blkIdx / 4) * 4 // 0 for Cb, 4 for Cr
	localIdx := blkIdx % 4       // 0-3 within component

	// For 4:2:0: 4 chroma blocks per component in 2x2 grid
	// Block layout: 0 1
	//               2 3
	x := localIdx % 2
	y := localIdx / 2

	if x > 0 {
		condA = int(sc.MBs[mbIdx].CodedBlockFlag[CtxBlockCatChromaAC][compBase+localIdx-1])
	} else if mbA := sc.MBAvailA(mbIdx); mbA != nil {
		condA = int(mbA.CodedBlockFlag[CtxBlockCatChromaAC][compBase+y*2+1])
	}

	if y > 0 {
		condB = int(sc.MBs[mbIdx].CodedBlockFlag[CtxBlockCatChromaAC][compBase+localIdx-2])
	} else if mbB := sc.MBAvailB(mbIdx); mbB != nil {
		condB = int(mbB.CodedBlockFlag[CtxBlockCatChromaAC][compBase+2+x])
	}

	return condA, condB
}

// deriveSignificantCoeffFlagCtx derives the context index increment for significant_coeff_flag.
func deriveSignificantCoeffFlagCtx(ctxBlockCat int, scanIdx int) int {
	if ctxBlockCat < 5 {
		// For categories 0-4, ctxIdxInc = scanIdx (limited by maxNumCoeff-1)
		return scanIdx
	}
	// For category 5 (8x8), use mapping table (6 contexts for 63 positions)
	if scanIdx < 63 {
		return significantCoeffCtx8x8[scanIdx]
	}
	return 0
}

// significantCoeffCtx8x8 maps scan position to context for 8x8 blocks (frame mode).
var significantCoeffCtx8x8 = [63]int{
	0, 1, 2, 3, 4, 5, 5, 4, 4, 3, 3, 4, 4, 4, 5, 5,
	4, 4, 4, 4, 3, 3, 6, 7, 7, 7, 8, 9, 10, 9, 8, 7,
	7, 6, 11, 12, 13, 11, 6, 7, 8, 9, 14, 10, 9, 8, 6, 11,
	12, 13, 11, 6, 9, 14, 10, 9, 11, 12, 13, 11, 14, 10, 12,
}

// deriveLastSignificantCoeffFlagCtx derives the context index increment for last_significant_coeff_flag.
func deriveLastSignificantCoeffFlagCtx(ctxBlockCat int, scanIdx int) int {
	if ctxBlockCat < 5 {
		return scanIdx
	}
	// For 8x8
	if scanIdx < 63 {
		return lastCoeffFlagOffset8x8[scanIdx]
	}
	return 0
}
