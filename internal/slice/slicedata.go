package slice

import (
	"fmt"

	"github.com/Eyevinn/hi264/internal/cabac"
	"github.com/Eyevinn/hi264/internal/cavlc"
	"github.com/Eyevinn/hi264/internal/context"
)

// DecodeSliceData decodes all macroblocks in an I-slice.
// sliceData is the raw slice data bytes (after slice header, EBSP-decoded).
// Returns the slice context with all decoded MB data.
func DecodeSliceData(sliceData []byte, sliceQPY int, mbWidth, mbHeight int,
	transform8x8ModeFlag bool, chromaArrayType int,
	bitDepthY, bitDepthC int, chromaQpIndexOffset int, traceMBCMP bool) (*SliceContext, error) {

	totalMBs := mbWidth * mbHeight

	// Initialize context models for I-slice
	models := context.InitModels(sliceQPY, 2, 0) // sliceType=2 for I

	// Initialize CABAC decoder
	dec, err := cabac.NewDecoder(sliceData)
	if err != nil {
		return nil, fmt.Errorf("cabac init: %w", err)
	}

	sc := &SliceContext{
		Cabac:                dec,
		Ctx:                  (*[1024]cabac.CtxState)(&models),
		MBWidth:              mbWidth,
		MBHeight:             mbHeight,
		TotalMBs:             totalMBs,
		QPY:                  sliceQPY,
		MBs:                  make([]MBData, totalMBs),
		Transform8x8ModeFlag: transform8x8ModeFlag,
		ChromaArrayType:      chromaArrayType,
		BitDepthY:            bitDepthY,
		BitDepthC:            bitDepthC,
		ChromaQpIndexOffset:  chromaQpIndexOffset,
		TraceMBCMP:           traceMBCMP,
	}

	// Initialize all MB QP to slice QP
	for i := range sc.MBs {
		sc.MBs[i].QPY = sliceQPY
	}

	// Decode each macroblock
	for mbIdx := 0; mbIdx < totalMBs; mbIdx++ {
		err := decodeMacroblock(sc, mbIdx)
		if err != nil {
			return sc, fmt.Errorf("mb %d: %w", mbIdx, err)
		}

		// Check end_of_slice_flag
		if mbIdx < totalMBs-1 {
			endOfSlice := dec.DecodeTerminate()
			if endOfSlice == 1 {
				break
			}
		}
	}

	return sc, nil
}

// decodeMacroblock decodes a single macroblock.
func decodeMacroblock(sc *SliceContext, mbIdx int) error {
	mb := &sc.MBs[mbIdx]

	// Decode mb_type
	mb.MBType = DecodeMBTypeIntra(sc, mbIdx)

	if mb.MBType == MBTypeIPCM {
		return decodeIPCM(sc, mbIdx)
	}

	if mb.MBType == MBTypeINxN {
		// I_NxN: decode transform_size_8x8_flag if enabled
		if sc.Transform8x8ModeFlag {
			mb.TransformSize8x8 = DecodeTransformSize8x8Flag(sc, mbIdx)
		}

		if mb.TransformSize8x8 {
			// I_8x8: decode 4 8x8 prediction modes
			for i := 0; i < 4; i++ {
				prevFlag, rem := DecodeIntra8x8PredMode(sc)
				predicted := derivePredIntra8x8PredMode(sc, mbIdx, i)
				if prevFlag {
					mb.Intra8x8PredMode[i] = predicted
				} else {
					if rem >= predicted {
						mb.Intra8x8PredMode[i] = rem + 1
					} else {
						mb.Intra8x8PredMode[i] = rem
					}
				}
			}
		} else {
			// I_4x4: decode 16 4x4 prediction modes
			for i := 0; i < 16; i++ {
				prevFlag, rem := DecodeIntra4x4PredMode(sc)
				predicted := derivePredIntra4x4PredMode(sc, mbIdx, i)
				if prevFlag {
					mb.Intra4x4PredMode[i] = predicted
				} else {
					if rem >= predicted {
						mb.Intra4x4PredMode[i] = rem + 1
					} else {
						mb.Intra4x4PredMode[i] = rem
					}
				}
			}
		}

		// Decode intra_chroma_pred_mode
		if sc.ChromaArrayType != 0 {
			mb.IntraChromaPredMode = DecodeIntraChromaPredMode(sc, mbIdx)
		}

		// Decode CBP for I_NxN
		mb.CBPLuma, mb.CBPChroma = DecodeCBP(sc, mbIdx)
	} else {
		// I_16x16: prediction mode and CBP are embedded in mb_type
		mb.IntraPredMode16x16 = I16x16PredMode(mb.MBType)
		mb.CBPLuma = I16x16CBPLuma(mb.MBType)
		mb.CBPChroma = I16x16CBPChroma(mb.MBType)

		// Decode intra_chroma_pred_mode
		if sc.ChromaArrayType != 0 {
			mb.IntraChromaPredMode = DecodeIntraChromaPredMode(sc, mbIdx)
		}
	}

	// Decode mb_qp_delta if there are any coded coefficients
	if mb.CBPLuma > 0 || mb.CBPChroma > 0 || (mb.MBType >= 1 && mb.MBType <= 24) {
		mb.QPDelta = DecodeQPDelta(sc)
		// Equation 7-37: QPY = ((QPY_PREV + mb_qp_delta + 52 + 2*QpBdOffsetY) % (52 + QpBdOffsetY)) - QpBdOffsetY
		qpBdOffsetY := 6 * (sc.BitDepthY - 8)
		qpRange := 52 + qpBdOffsetY
		mb.QPY = ((sc.QPY + mb.QPDelta + qpRange + 2*qpBdOffsetY) % qpRange) - qpBdOffsetY
		sc.PrevMBQPDeltaNonZero = mb.QPDelta != 0
		sc.QPY = mb.QPY
	} else {
		sc.PrevMBQPDeltaNonZero = false
		mb.QPY = sc.QPY // propagate QP from previous MB
	}

	// Decode residual
	decodeResidualMB(sc, mbIdx)

	// MBCMP trace (matches FFmpeg format — emitted AFTER residual, like FFmpeg)
	if sc.TraceMBCMP {
		cbp := mb.CBPLuma | (mb.CBPChroma << 4)
		pred := mb.IntraPredMode16x16
		if mb.MBType == MBTypeINxN {
			pred = 255
		}
		t8x8 := ""
		if mb.TransformSize8x8 {
			t8x8 = " 8x8"
		}
		fmt.Printf("MBCMP[%d] type=%d cbp=0x%02x pred=%d cpred=%d qp=%d R=%d O=%d%s\n",
			mbIdx, mb.MBType, cbp, pred, mb.IntraChromaPredMode, mb.QPY,
			sc.Cabac.Range(), sc.Cabac.Offset(), t8x8)
	}

	return nil
}

// decodeResidualMB decodes all residual data for a macroblock.
func decodeResidualMB(sc *SliceContext, mbIdx int) {
	mb := &sc.MBs[mbIdx]

	if mb.MBType >= 1 && mb.MBType <= 24 {
		// I_16x16: decode DC level, then AC levels for each 4x4 block

		// Intra16x16 DC level (16 coefficients)
		dcCoeffs := DecodeResidual(sc, mbIdx, CtxBlockCatIntra16x16DC, 0, 16)
		for i := 0; i < 16; i++ {
			mb.Intra16x16DCLevel[i] = dcCoeffs[i]
		}

		// Intra16x16 AC levels (15 coefficients per 4x4 block)
		if mb.CBPLuma > 0 {
			for i := 0; i < 16; i++ {
				// Check if the 8x8 block containing this 4x4 block has coded coeffs
				i8x8 := i / 4
				if mb.CBPLuma&(1<<uint(i8x8)) != 0 {
					acCoeffs := DecodeResidual(sc, mbIdx, CtxBlockCatIntra16x16AC, i, 15)
					for j := 0; j < 15; j++ {
						mb.Intra16x16ACLevel[i][j] = acCoeffs[j]
					}
				}
			}
		}
	} else if mb.MBType == MBTypeINxN {
		if mb.TransformSize8x8 {
			// I_8x8: decode 8x8 luma blocks
			for i := 0; i < 4; i++ {
				if mb.CBPLuma&(1<<uint(i)) != 0 {
					coeffs := DecodeResidual(sc, mbIdx, CtxBlockCatLuma8x8, i, 64)
					for j := 0; j < 64; j++ {
						mb.LumaLevel8x8[i][j] = coeffs[j]
					}
					// Mark 8x8 block as coded for neighbor CBF context derivation.
					// coded_block_flag is not decoded for 8x8 blocks (spec 9.3.3.1.1.9),
					// but neighbors need to know this block has non-zero coefficients.
					mb.CodedBlockFlag[CtxBlockCatLuma8x8][i] = 1
				}
			}
		} else {
			// I_4x4: decode 16 4x4 luma blocks
			// Block indices use H.264 hierarchical scan: i/4 gives 8x8 block index
			for i := 0; i < 16; i++ {
				i8x8 := i / 4
				if mb.CBPLuma&(1<<uint(i8x8)) != 0 {
					coeffs := DecodeResidual(sc, mbIdx, CtxBlockCatLuma4x4, i, 16)
					for j := 0; j < 16; j++ {
						mb.LumaLevel4x4[i][j] = coeffs[j]
					}
				}
			}
		}
	}

	// Chroma residual
	if sc.ChromaArrayType != 0 && (mb.CBPChroma > 0 || (mb.MBType >= 1 && mb.MBType <= 24 && mb.CBPChroma > 0)) {
		// Chroma DC for each component
		for iCbCr := 0; iCbCr < 2; iCbCr++ {
			if mb.CBPChroma > 0 {
				numDC := 4 // for 4:2:0
				dcCoeffs := DecodeResidual(sc, mbIdx, CtxBlockCatChromaDC, iCbCr, numDC)
				for j := 0; j < numDC; j++ {
					mb.ChromaDCLevel[iCbCr][j] = dcCoeffs[j]
				}
			}
		}

		// Chroma AC for each component and block
		if mb.CBPChroma > 1 {
			for iCbCr := 0; iCbCr < 2; iCbCr++ {
				for i := 0; i < 4; i++ { // 4 blocks per component for 4:2:0
					blkIdx := iCbCr*4 + i
					acCoeffs := DecodeResidual(sc, mbIdx, CtxBlockCatChromaAC, blkIdx, 15)
					for j := 0; j < 15; j++ {
						mb.ChromaACLevel[iCbCr][i][j] = acCoeffs[j]
					}
				}
			}
		}
	}
}

// decodeIPCM handles I_PCM macroblock type.
func decodeIPCM(sc *SliceContext, mbIdx int) error {
	// I_PCM: raw sample values follow, byte-aligned
	// For now, skip with warning
	fmt.Printf("WARNING: Skipping I_PCM macroblock at index %d\n", mbIdx)
	return nil
}

// DecodeSliceDataCAVLC decodes all macroblocks in an I-slice using CAVLC entropy coding.
// br is positioned at the start of slice data (after header skip).
func DecodeSliceDataCAVLC(br *cavlc.BitReader, sliceQPY int, mbWidth, mbHeight int,
	transform8x8ModeFlag bool, chromaArrayType int,
	bitDepthY, bitDepthC int, chromaQpIndexOffset int, traceMBCMP bool) (*SliceContext, error) {

	totalMBs := mbWidth * mbHeight

	sc := &SliceContext{
		IsCAVLC:              true,
		Br:                   br,
		MBWidth:              mbWidth,
		MBHeight:             mbHeight,
		TotalMBs:             totalMBs,
		QPY:                  sliceQPY,
		MBs:                  make([]MBData, totalMBs),
		Transform8x8ModeFlag: transform8x8ModeFlag,
		ChromaArrayType:      chromaArrayType,
		BitDepthY:            bitDepthY,
		BitDepthC:            bitDepthC,
		ChromaQpIndexOffset:  chromaQpIndexOffset,
		TraceMBCMP:           traceMBCMP,
	}

	// Initialize all MB QP to slice QP
	for i := range sc.MBs {
		sc.MBs[i].QPY = sliceQPY
	}

	// Decode each macroblock (CAVLC has no end_of_slice_flag)
	for mbIdx := 0; mbIdx < totalMBs; mbIdx++ {
		err := decodeMacroblockCAVLC(sc, mbIdx)
		if err != nil {
			return sc, fmt.Errorf("mb %d: %w", mbIdx, err)
		}
	}

	return sc, nil
}

// decodeMacroblockCAVLC decodes a single macroblock using CAVLC.
func decodeMacroblockCAVLC(sc *SliceContext, mbIdx int) error {
	mb := &sc.MBs[mbIdx]
	br := sc.Br

	// Decode mb_type: ue(v)
	mbType, err := DecodeMBTypeIntraCAVLC(br)
	if err != nil {
		return fmt.Errorf("mb_type: %w", err)
	}
	mb.MBType = mbType

	if mb.MBType == MBTypeIPCM {
		return decodeIPCMCAVLC(sc, mbIdx)
	}

	if mb.MBType == MBTypeINxN {
		// I_NxN: decode transform_size_8x8_flag if enabled
		if sc.Transform8x8ModeFlag {
			flag, err := DecodeTransformSize8x8FlagCAVLC(br)
			if err != nil {
				return err
			}
			mb.TransformSize8x8 = flag
		}

		if mb.TransformSize8x8 {
			// I_8x8: decode 4 8x8 prediction modes
			for i := 0; i < 4; i++ {
				prevFlag, rem, err := DecodeIntra4x4PredModeCAVLC(br)
				if err != nil {
					return err
				}
				predicted := derivePredIntra8x8PredMode(sc, mbIdx, i)
				if prevFlag {
					mb.Intra8x8PredMode[i] = predicted
				} else {
					if rem >= predicted {
						mb.Intra8x8PredMode[i] = rem + 1
					} else {
						mb.Intra8x8PredMode[i] = rem
					}
				}
			}
		} else {
			// I_4x4: decode 16 4x4 prediction modes
			for i := 0; i < 16; i++ {
				prevFlag, rem, err := DecodeIntra4x4PredModeCAVLC(br)
				if err != nil {
					return err
				}
				predicted := derivePredIntra4x4PredMode(sc, mbIdx, i)
				if prevFlag {
					mb.Intra4x4PredMode[i] = predicted
				} else {
					if rem >= predicted {
						mb.Intra4x4PredMode[i] = rem + 1
					} else {
						mb.Intra4x4PredMode[i] = rem
					}
				}
			}
		}

		// Decode intra_chroma_pred_mode
		if sc.ChromaArrayType != 0 {
			mode, err := DecodeIntraChromaPredModeCAVLC(br)
			if err != nil {
				return err
			}
			mb.IntraChromaPredMode = mode
		}

		// Decode CBP for I_NxN
		cbpLuma, cbpChroma, err := DecodeCBPCAVLC(br)
		if err != nil {
			return err
		}
		mb.CBPLuma = cbpLuma
		mb.CBPChroma = cbpChroma
	} else {
		// I_16x16: prediction mode and CBP are embedded in mb_type
		mb.IntraPredMode16x16 = I16x16PredMode(mb.MBType)
		mb.CBPLuma = I16x16CBPLuma(mb.MBType)
		mb.CBPChroma = I16x16CBPChroma(mb.MBType)

		// Decode intra_chroma_pred_mode
		if sc.ChromaArrayType != 0 {
			mode, err := DecodeIntraChromaPredModeCAVLC(br)
			if err != nil {
				return err
			}
			mb.IntraChromaPredMode = mode
		}
	}

	// Decode mb_qp_delta if there are any coded coefficients
	if mb.CBPLuma > 0 || mb.CBPChroma > 0 || (mb.MBType >= 1 && mb.MBType <= 24) {
		qpDelta, err := DecodeQPDeltaCAVLC(br)
		if err != nil {
			return err
		}
		mb.QPDelta = qpDelta
		qpBdOffsetY := 6 * (sc.BitDepthY - 8)
		qpRange := 52 + qpBdOffsetY
		mb.QPY = ((sc.QPY + mb.QPDelta + qpRange + 2*qpBdOffsetY) % qpRange) - qpBdOffsetY
		sc.QPY = mb.QPY
	} else {
		mb.QPY = sc.QPY
	}

	// Decode residual
	err = decodeResidualMBCAVLC(sc, mbIdx)
	if err != nil {
		return fmt.Errorf("residual: %w", err)
	}

	// MBCMP trace
	if sc.TraceMBCMP {
		cbp := mb.CBPLuma | (mb.CBPChroma << 4)
		pred := mb.IntraPredMode16x16
		if mb.MBType == MBTypeINxN {
			pred = 255
		}
		t8x8 := ""
		if mb.TransformSize8x8 {
			t8x8 = " 8x8"
		}
		fmt.Printf("MBCMP[%d] type=%d cbp=0x%02x pred=%d cpred=%d qp=%d B=%d%s\n",
			mbIdx, mb.MBType, cbp, pred, mb.IntraChromaPredMode, mb.QPY, sc.Br.BitsRead(), t8x8)
	}

	return nil
}

// decodeResidualMBCAVLC decodes all residual data for a macroblock using CAVLC.
func decodeResidualMBCAVLC(sc *SliceContext, mbIdx int) error {
	mb := &sc.MBs[mbIdx]
	br := sc.Br

	if mb.MBType >= 1 && mb.MBType <= 24 {
		// I_16x16: decode DC level, then AC levels

		// DC coefficients (16 coefficients, nC derived from block 0 neighbors)
		nC := DeriveNC(sc, mbIdx, 0, false)
		dcCoeffs, totalCoeff, err := cavlc.DecodeResidualBlock(br, nC, 16)
		if err != nil {
			return fmt.Errorf("i16x16 DC: %w", err)
		}
		for i := 0; i < 16; i++ {
			mb.Intra16x16DCLevel[i] = dcCoeffs[i]
		}
		// Store totalCoeff for DC (used for nC of AC blocks)
		_ = totalCoeff

		// AC levels (15 coefficients per 4x4 block)
		if mb.CBPLuma > 0 {
			for i := 0; i < 16; i++ {
				i8x8 := i / 4
				if mb.CBPLuma&(1<<uint(i8x8)) != 0 {
					nC := DeriveNC(sc, mbIdx, i, false)
					acCoeffs, tc, err := cavlc.DecodeResidualBlock(br, nC, 15)
					if err != nil {
						return fmt.Errorf("i16x16 AC[%d]: %w", i, err)
					}
					for j := 0; j < 15; j++ {
						mb.Intra16x16ACLevel[i][j] = acCoeffs[j]
					}
					mb.NzCoeffLuma[i] = tc
				}
			}
		}
	} else if mb.MBType == MBTypeINxN {
		if mb.TransformSize8x8 {
			// I_8x8: decode as 4 groups of 4 sub-blocks (each 16 coefficients)
			for i8x8 := 0; i8x8 < 4; i8x8++ {
				if mb.CBPLuma&(1<<uint(i8x8)) != 0 {
					for i4x4 := 0; i4x4 < 4; i4x4++ {
						blkIdx := i8x8*4 + i4x4
						nC := DeriveNC(sc, mbIdx, blkIdx, false)
						subCoeffs, tc, err := cavlc.DecodeResidualBlock(br, nC, 16)
						if err != nil {
							return fmt.Errorf("i8x8[%d] sub[%d]: %w", i8x8, i4x4, err)
						}
						// CAVLC sub-blocks use zigzagScan8x8CAVLC to map to raster positions.
						// Store directly in raster order.
						for j := 0; j < 16; j++ {
							mb.LumaLevel8x8[i8x8][zigzagScan8x8CAVLC[i4x4*16+j]] = subCoeffs[j]
						}
						mb.NzCoeffLuma[blkIdx] = tc
					}
				}
			}
		} else {
			// I_4x4: decode 16 4x4 luma blocks
			for i := 0; i < 16; i++ {
				i8x8 := i / 4
				if mb.CBPLuma&(1<<uint(i8x8)) != 0 {
					nC := DeriveNC(sc, mbIdx, i, false)
					coeffs, tc, err := cavlc.DecodeResidualBlock(br, nC, 16)
					if err != nil {
						return fmt.Errorf("i4x4[%d]: %w", i, err)
					}
					for j := 0; j < 16; j++ {
						mb.LumaLevel4x4[i][j] = coeffs[j]
					}
					mb.NzCoeffLuma[i] = tc
				}
			}
		}
	}

	// Chroma residual
	if sc.ChromaArrayType != 0 && mb.CBPChroma > 0 {
		// Chroma DC for each component
		for iCbCr := 0; iCbCr < 2; iCbCr++ {
			dcCoeffs, _, err := cavlc.DecodeResidualBlock(br, -1, 4)
			if err != nil {
				return fmt.Errorf("chroma DC[%d]: %w", iCbCr, err)
			}
			for j := 0; j < 4; j++ {
				mb.ChromaDCLevel[iCbCr][j] = dcCoeffs[j]
			}
		}

		// Chroma AC for each component and block
		if mb.CBPChroma > 1 {
			for iCbCr := 0; iCbCr < 2; iCbCr++ {
				for i := 0; i < 4; i++ {
					blkIdx := iCbCr*4 + i
					nC := DeriveChromaNC(sc, mbIdx, blkIdx)
					acCoeffs, tc, err := cavlc.DecodeResidualBlock(br, nC, 15)
					if err != nil {
						return fmt.Errorf("chroma AC[%d][%d]: %w", iCbCr, i, err)
					}
					for j := 0; j < 15; j++ {
						mb.ChromaACLevel[iCbCr][i][j] = acCoeffs[j]
					}
					mb.NzCoeffChroma[blkIdx] = tc
				}
			}
		}
	}

	return nil
}

// decodeIPCMCAVLC handles I_PCM macroblock type for CAVLC.
func decodeIPCMCAVLC(sc *SliceContext, mbIdx int) error {
	fmt.Printf("WARNING: Skipping I_PCM macroblock at index %d\n", mbIdx)
	return nil
}

// zigzagScan8x8CAVLC maps CAVLC 8x8 sequential position to raster position.
// Organized as 4 sub-blocks of 16 coefficients each.
// From FFmpeg h264_slice.c: zigzag_scan8x8_cavlc[i] = zigzag_scan8x8[(i/4) + 16*(i%4)]
var zigzagScan8x8CAVLC = [64]int{
	// Sub-block 0
	0, 9, 17, 18, 12, 40, 27, 7,
	35, 57, 29, 30, 58, 38, 53, 47,
	// Sub-block 1
	1, 2, 24, 11, 19, 48, 20, 14,
	42, 50, 22, 37, 59, 31, 60, 55,
	// Sub-block 2
	8, 3, 32, 4, 26, 41, 13, 21,
	49, 43, 15, 44, 52, 39, 61, 62,
	// Sub-block 3
	16, 10, 25, 5, 33, 34, 6, 28,
	56, 36, 23, 51, 45, 46, 54, 63,
}
