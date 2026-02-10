package encode

// CABAC syntax element encoding functions for I_16x16 macroblocks.
// These are the exact inverse of the decoding functions in pkg/slice/mbtype.go
// and pkg/slice/residual.go. Tables are duplicated here to avoid import cycles.

import "github.com/Eyevinn/hi264/internal/cabac"

// Block category constants (matching pkg/slice).
const (
	encCtxBlockCatIntra16x16DC = 0
	encCtxBlockCatIntra16x16AC = 1
	encCtxBlockCatChromaDC     = 3
	encCtxBlockCatChromaAC     = 4
)

// Context offset tables (Table 9-40 and 9-42), duplicated from pkg/slice/residual.go.
var encCBFOffset = [6]int{85, 89, 93, 97, 101, 1012}
var encSigOffset = [6]int{105, 120, 134, 149, 152, 402}
var encLastOffset = [6]int{166, 181, 195, 210, 213, 417}
var encLevelOffset = [6]int{227, 237, 247, 257, 266, 426}

// Node-based context tables for coeff_abs_level_minus1 (section 9.3.3.1.1.5).
var encLevel1Ctx = [8]int{1, 2, 3, 4, 0, 0, 0, 0}
var encLevelGt1Ctx = [8]int{5, 5, 5, 5, 6, 7, 8, 9}
var encLevelTransition = [2][8]int{
	{1, 2, 3, 3, 4, 5, 6, 7},
	{4, 4, 4, 4, 5, 6, 7, 7},
}

// encodeMBTypeI16x16 encodes I_16x16 mb_type using CABAC (ctx 3-10).
// mbType must be 1-24 (I_16x16 range). Decomposed into cbpLuma, cbpChroma, predMode bins.
// leftNotINxN and topNotINxN indicate whether left/top neighbors are NOT I_NxN.
func encodeMBTypeI16x16(enc *cabac.Encoder, ctx []cabac.CtxState,
	mbType int, leftNotINxN, topNotINxN bool) {

	// First bin: 1 (not I_NxN), ctx 3+ctxIdxInc
	ctxIdxInc := 0
	if leftNotINxN {
		ctxIdxInc++
	}
	if topNotINxN {
		ctxIdxInc++
	}
	enc.EncodeDecision(1, &ctx[3+ctxIdxInc])

	// Terminate bin: 0 (not I_PCM)
	enc.EncodeTerminate(0)

	// Decompose mb_type: mb_type = 1 + predMode + 4*cbpChroma + 12*cbpLumaFlag
	v := mbType - 1
	cbpLumaFlag := 0
	if v >= 12 {
		cbpLumaFlag = 1
		v -= 12
	}
	cbpChroma := v / 4
	predMode := v % 4

	// Bin 1 (ctx 6): cbp_luma (0 or non-zero)
	enc.EncodeDecision(uint8(cbpLumaFlag), &ctx[6])

	// Bin 2 (ctx 7): first bit of cbp_chroma (cbpChroma > 0 ?)
	if cbpChroma > 0 {
		enc.EncodeDecision(1, &ctx[7])
		// Bin 3 (ctx 8): second bit of cbp_chroma (cbpChroma == 2 ?)
		if cbpChroma == 2 {
			enc.EncodeDecision(1, &ctx[8])
		} else {
			enc.EncodeDecision(0, &ctx[8])
		}
	} else {
		enc.EncodeDecision(0, &ctx[7])
	}

	// Bin 4,5 (ctx 9, 10): prediction mode (2 bits, FL cMax=3)
	enc.EncodeDecision(uint8((predMode>>1)&1), &ctx[9])
	enc.EncodeDecision(uint8(predMode&1), &ctx[10])
}

// encodeChromaPredMode encodes intra_chroma_pred_mode using CABAC (ctx 64-67).
// TU binarization with cMax=3.
func encodeChromaPredMode(enc *cabac.Encoder, ctx []cabac.CtxState,
	mode int, leftNonZero, topNonZero bool) {

	ctxIdxInc := 0
	if leftNonZero {
		ctxIdxInc++
	}
	if topNonZero {
		ctxIdxInc++
	}

	if mode == 0 {
		enc.EncodeDecision(0, &ctx[64+ctxIdxInc])
		return
	}
	enc.EncodeDecision(1, &ctx[64+ctxIdxInc])
	if mode == 1 {
		enc.EncodeDecision(0, &ctx[67])
		return
	}
	enc.EncodeDecision(1, &ctx[67])
	if mode == 2 {
		enc.EncodeDecision(0, &ctx[67])
		return
	}
	// mode == 3: implicit (max TU value reached)
}

// encodeQPDelta encodes mb_qp_delta using CABAC (ctx 60-63).
func encodeQPDelta(enc *cabac.Encoder, ctx []cabac.CtxState,
	delta int, prevDeltaNonZero bool) {

	if delta == 0 {
		ctxIdx := 60
		if prevDeltaNonZero {
			ctxIdx = 61
		}
		enc.EncodeDecision(0, &ctx[ctxIdx])
		return
	}

	// Map signed delta to unary value:
	// +1 -> 1, -1 -> 2, +2 -> 3, -2 -> 4, etc.
	var unaryVal int
	if delta > 0 {
		unaryVal = 2*delta - 1
	} else {
		unaryVal = -2 * delta
	}

	// First bin
	ctxIdx := 60
	if prevDeltaNonZero {
		ctxIdx = 61
	}
	enc.EncodeDecision(1, &ctx[ctxIdx])

	// Subsequent bins: unary, ctx 62 then 63
	for i := 1; i < unaryVal; i++ {
		ctxIdx := 62
		if i > 1 {
			ctxIdx = 63
		}
		enc.EncodeDecision(1, &ctx[ctxIdx])
	}
	// Terminating zero bin
	ctxIdx = 62
	if unaryVal > 1 {
		ctxIdx = 63
	}
	enc.EncodeDecision(0, &ctx[ctxIdx])
}

// encodeResidualBlockCABAC encodes residual coefficients using CABAC.
// Returns the coded_block_flag value (0 or 1).
func encodeResidualBlockCABAC(enc *cabac.Encoder, ctx []cabac.CtxState,
	cat int, cbfCtxIdx int, coeffs []int32, maxCoeff int) uint8 {

	// Check if all coefficients are zero
	hasNonZero := false
	for i := range maxCoeff {
		if coeffs[i] != 0 {
			hasNonZero = true
			break
		}
	}

	// coded_block_flag
	ctxIdx := encCBFOffset[cat] + cbfCtxIdx
	if !hasNonZero {
		enc.EncodeDecision(0, &ctx[ctxIdx])
		return 0
	}
	enc.EncodeDecision(1, &ctx[ctxIdx])

	// Find significant coefficient positions
	lastSig := -1
	for i := maxCoeff - 1; i >= 0; i-- {
		if coeffs[i] != 0 {
			lastSig = i
			break
		}
	}

	// Encode significant_coeff_flag and last_significant_coeff_flag
	for i := 0; i < maxCoeff-1; i++ {
		sigCtx := encSigOffset[cat] + i
		if coeffs[i] != 0 {
			enc.EncodeDecision(1, &ctx[sigCtx])
			if i == lastSig {
				// last_significant_coeff_flag = 1
				lastCtx := encLastOffset[cat] + i
				enc.EncodeDecision(1, &ctx[lastCtx])
				break
			}
			// last_significant_coeff_flag = 0
			lastCtx := encLastOffset[cat] + i
			enc.EncodeDecision(0, &ctx[lastCtx])
		} else {
			enc.EncodeDecision(0, &ctx[sigCtx])
		}
	}
	// If lastSig == maxCoeff-1, it's implicitly significant (no sig/last flags needed for last pos)

	// Collect significant coefficient indices in reverse order for level encoding
	var sigIndices []int
	for i := lastSig; i >= 0; i-- {
		if coeffs[i] != 0 {
			sigIndices = append(sigIndices, i)
		}
	}

	// Encode coefficient levels using node-based state machine
	nodeCtx := 0
	baseCtx := encLevelOffset[cat]

	for _, idx := range sigIndices {
		absLevel := coeffs[idx]
		sign := uint8(0)
		if absLevel < 0 {
			sign = 1
			absLevel = -absLevel
		}

		if absLevel == 1 {
			// coeff_abs_level_minus1 binIdx=0: encode 0
			ctxIdxInc := encLevel1Ctx[nodeCtx]
			enc.EncodeDecision(0, &ctx[baseCtx+ctxIdxInc])
			nodeCtx = encLevelTransition[0][nodeCtx]
		} else {
			// coeff_abs_level_minus1 binIdx=0: encode 1 (level >= 2)
			ctxIdxInc := encLevel1Ctx[nodeCtx]
			enc.EncodeDecision(1, &ctx[baseCtx+ctxIdxInc])

			// Use gt1 context for prefix bins
			ctxIdxInc = encLevelGt1Ctx[nodeCtx]
			nodeCtx = encLevelTransition[1][nodeCtx]

			// Decoder loop: coeffAbs starts at 2, reads bins while coeffAbs < 15.
			// If bin=1, coeffAbs++. If bin=0, stop.
			// So for absLevel in 2..14: emit (absLevel-2) 1-bins, then 1 0-bin.
			// For absLevel >= 15: emit 13 1-bins, then EG bypass for (absLevel-15).
			if absLevel < 15 {
				for i := int32(0); i < absLevel-2; i++ {
					enc.EncodeDecision(1, &ctx[baseCtx+ctxIdxInc])
				}
				enc.EncodeDecision(0, &ctx[baseCtx+ctxIdxInc])
			} else {
				for range 13 {
					enc.EncodeDecision(1, &ctx[baseCtx+ctxIdxInc])
				}
				encodeExpGolombBypass(enc, uint32(absLevel-15))
			}
		}

		// Sign bit (bypass)
		enc.EncodeBypass(sign)
	}

	return 1
}

// encodeExpGolombBypass encodes a 0th-order Exp-Golomb code using bypass bins.
func encodeExpGolombBypass(enc *cabac.Encoder, val uint32) {
	// 0th order Exp-Golomb: val = (1 << k) - 1 + suffix
	// Find k such that val < (1 << (k+1)) - 1
	k := uint(0)
	tmp := val
	for tmp >= (1 << k) {
		tmp -= 1 << k
		k++
	}
	// Write k 1-bits (prefix)
	for i := uint(0); i < k; i++ {
		enc.EncodeBypass(1)
	}
	// Write terminating 0-bit
	enc.EncodeBypass(0)
	// Write k suffix bits
	for i := k; i > 0; i-- {
		enc.EncodeBypass(uint8((tmp >> (i - 1)) & 1))
	}
}
