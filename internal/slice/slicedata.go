package slice

import (
	"fmt"

	"github.com/Eyevinn/hi264/internal/cabac"
	"github.com/Eyevinn/hi264/internal/cavlc"
	"github.com/Eyevinn/hi264/internal/context"
)

// NewSliceContext returns the context for decoding the slices of one picture
// of mbWidth x mbHeight macroblocks. Each slice is decoded into it with
// DecodeSliceData or DecodeSliceDataCAVLC.
func NewSliceContext(mbWidth, mbHeight int, isCAVLC, transform8x8ModeFlag bool, chromaArrayType int,
	bitDepthY, bitDepthC int, chromaQpIndexOffset int, traceMBCMP bool) *SliceContext {
	totalMBs := mbWidth * mbHeight
	return &SliceContext{
		MBWidth:              mbWidth,
		MBHeight:             mbHeight,
		TotalMBs:             totalMBs,
		MBs:                  make([]MBData, totalMBs),
		IsCAVLC:              isCAVLC,
		Transform8x8ModeFlag: transform8x8ModeFlag,
		ChromaArrayType:      chromaArrayType,
		BitDepthY:            bitDepthY,
		BitDepthC:            bitDepthC,
		ChromaQpIndexOffset:  chromaQpIndexOffset,
		TraceMBCMP:           traceMBCMP,
	}
}

// DecodedMBs returns the number of macroblocks that the slices have decoded.
func (sc *SliceContext) DecodedMBs() int {
	return sc.decodedMBs
}

// beginSlice records the slice p and resets the per-slice decoding state.
// It returns the slice's number.
func (sc *SliceContext) beginSlice(p SliceParams) (int, error) {
	if p.FirstMB < 0 || p.FirstMB >= sc.TotalMBs {
		return 0, fmt.Errorf("first_mb_in_slice %d outside a picture of %d macroblocks", p.FirstMB, sc.TotalMBs)
	}
	sc.Slices = append(sc.Slices, p)
	sc.QPY = p.SliceQPY
	sc.PrevMBQPDeltaNonZero = false
	return len(sc.Slices), nil
}

// beginMB assigns macroblock mbIdx to slice sliceNum.
func (sc *SliceContext) beginMB(mbIdx, sliceNum int) error {
	if mbIdx >= sc.TotalMBs {
		return fmt.Errorf("slice continues past the last macroblock %d", sc.TotalMBs-1)
	}
	mb := &sc.MBs[mbIdx]
	if mb.SliceNum != 0 {
		return fmt.Errorf("mb %d: already decoded by slice %d", mbIdx, mb.SliceNum)
	}
	mb.SliceNum = sliceNum
	sc.decodedMBs++
	return nil
}

// DecodeSliceData decodes the macroblocks of one CABAC I-slice into sc.
// sliceData is the raw slice data bytes (after slice header, EBSP-decoded).
func (sc *SliceContext) DecodeSliceData(sliceData []byte, p SliceParams) error {
	sliceNum, err := sc.beginSlice(p)
	if err != nil {
		return err
	}

	// Initialize context models for I-slice
	models := context.InitModels(p.SliceQPY, 2, 0) // sliceType=2 for I

	// Initialize CABAC decoder
	dec, err := cabac.NewDecoder(sliceData)
	if err != nil {
		return fmt.Errorf("cabac init: %w", err)
	}
	sc.Cabac = dec
	sc.Ctx = (*[1024]cabac.CtxState)(&models)

	// Decode macroblocks until end_of_slice_flag
	for mbIdx := p.FirstMB; ; mbIdx++ {
		if err := sc.beginMB(mbIdx, sliceNum); err != nil {
			return err
		}
		if err := decodeMacroblock(sc, mbIdx); err != nil {
			return fmt.Errorf("mb %d: %w", mbIdx, err)
		}
		if err := dec.Err(); err != nil {
			return fmt.Errorf("mb %d: %w", mbIdx, err)
		}

		endOfSlice := dec.DecodeTerminate()
		if err := dec.Err(); err != nil {
			return fmt.Errorf("mb %d: %w", mbIdx, err)
		}
		if endOfSlice == 1 {
			return nil
		}
	}
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
			for i := range 4 {
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
			for i := range 16 {
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
		qpDelta, err := DecodeQPDelta(sc)
		if err != nil {
			return err
		}
		mb.QPDelta = qpDelta
		// Equation 7-37: QPY = ((QPY_PREV + mb_qp_delta + 52 + 2*QpBdOffsetY) % (52 + QpBdOffsetY)) - QpBdOffsetY
		qpBdOffsetY := qpBdOffset(sc.BitDepthY)
		qpRange := numQPBase + qpBdOffsetY
		mb.QPY = ((sc.QPY + mb.QPDelta + qpRange + 2*qpBdOffsetY) % qpRange) - qpBdOffsetY
		sc.PrevMBQPDeltaNonZero = mb.QPDelta != 0
		sc.QPY = mb.QPY
	} else {
		sc.PrevMBQPDeltaNonZero = false
		mb.QPY = sc.QPY // propagate QP from previous MB
	}

	// Decode residual
	if err := decodeResidualMB(sc, mbIdx); err != nil {
		return err
	}

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
func decodeResidualMB(sc *SliceContext, mbIdx int) error {
	mb := &sc.MBs[mbIdx]

	if mb.MBType >= 1 && mb.MBType <= 24 {
		// I_16x16: decode DC level, then AC levels for each 4x4 block

		// Intra16x16 DC level (16 coefficients)
		dcCoeffs, err := DecodeResidual(sc, mbIdx, CtxBlockCatIntra16x16DC, 0, 16)
		if err != nil {
			return err
		}
		for i := range 16 {
			mb.Intra16x16DCLevel[i] = dcCoeffs[i]
		}

		// Intra16x16 AC levels (15 coefficients per 4x4 block)
		if mb.CBPLuma > 0 {
			for i := range 16 {
				// Check if the 8x8 block containing this 4x4 block has coded coeffs
				i8x8 := i / 4
				if mb.CBPLuma&(1<<uint(i8x8)) != 0 {
					acCoeffs, err := DecodeResidual(sc, mbIdx, CtxBlockCatIntra16x16AC, i, 15)
					if err != nil {
						return err
					}
					for j := range 15 {
						mb.Intra16x16ACLevel[i][j] = acCoeffs[j]
					}
				}
			}
		}
	} else if mb.MBType == MBTypeINxN {
		if mb.TransformSize8x8 {
			// I_8x8: decode 8x8 luma blocks
			for i := range 4 {
				if mb.CBPLuma&(1<<uint(i)) != 0 {
					coeffs, err := DecodeResidual(sc, mbIdx, CtxBlockCatLuma8x8, i, 64)
					if err != nil {
						return err
					}
					for j := range 64 {
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
			for i := range 16 {
				i8x8 := i / 4
				if mb.CBPLuma&(1<<uint(i8x8)) != 0 {
					coeffs, err := DecodeResidual(sc, mbIdx, CtxBlockCatLuma4x4, i, 16)
					if err != nil {
						return err
					}
					for j := range 16 {
						mb.LumaLevel4x4[i][j] = coeffs[j]
					}
				}
			}
		}
	}

	// Chroma residual
	if sc.ChromaArrayType != 0 && (mb.CBPChroma > 0 || (mb.MBType >= 1 && mb.MBType <= 24 && mb.CBPChroma > 0)) {
		// Chroma DC for each component
		for iCbCr := range 2 {
			if mb.CBPChroma > 0 {
				numDC := 4 // for 4:2:0
				dcCoeffs, err := DecodeResidual(sc, mbIdx, CtxBlockCatChromaDC, iCbCr, numDC)
				if err != nil {
					return err
				}
				for j := range numDC {
					mb.ChromaDCLevel[iCbCr][j] = dcCoeffs[j]
				}
			}
		}

		// Chroma AC for each component and block
		if mb.CBPChroma > 1 {
			for iCbCr := range 2 {
				for i := range 4 { // 4 blocks per component for 4:2:0
					blkIdx := iCbCr*4 + i
					acCoeffs, err := DecodeResidual(sc, mbIdx, CtxBlockCatChromaAC, blkIdx, 15)
					if err != nil {
						return err
					}
					for j := range 15 {
						mb.ChromaACLevel[iCbCr][i][j] = acCoeffs[j]
					}
				}
			}
		}
	}
	return nil
}

// decodeIPCM handles I_PCM macroblock type.
func decodeIPCM(sc *SliceContext, mbIdx int) error {
	// I_PCM: raw sample values follow, byte-aligned
	// For now, skip with warning
	fmt.Printf("WARNING: Skipping I_PCM macroblock at index %d\n", mbIdx)
	return nil
}

// DecodeSliceDataCAVLC decodes the macroblocks of one CAVLC I-slice into sc.
// br is positioned at the start of slice data (after header skip).
func (sc *SliceContext) DecodeSliceDataCAVLC(br *cavlc.BitReader, p SliceParams) error {
	sliceNum, err := sc.beginSlice(p)
	if err != nil {
		return err
	}
	sc.Br = br

	// Decode macroblocks until the RBSP trailing bits (CAVLC has no end_of_slice_flag)
	for mbIdx := p.FirstMB; ; mbIdx++ {
		if err := sc.beginMB(mbIdx, sliceNum); err != nil {
			return err
		}
		if err := decodeMacroblockCAVLC(sc, mbIdx); err != nil {
			return fmt.Errorf("mb %d: %w", mbIdx, err)
		}
		if !br.MoreRBSPData() {
			return nil
		}
	}
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
			for i := range 4 {
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
			for i := range 16 {
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
		qpBdOffsetY := qpBdOffset(sc.BitDepthY)
		qpRange := numQPBase + qpBdOffsetY
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
		for i := range 16 {
			mb.Intra16x16DCLevel[i] = dcCoeffs[i]
		}
		// Store totalCoeff for DC (used for nC of AC blocks)
		_ = totalCoeff

		// AC levels (15 coefficients per 4x4 block)
		if mb.CBPLuma > 0 {
			for i := range 16 {
				i8x8 := i / 4
				if mb.CBPLuma&(1<<uint(i8x8)) != 0 {
					nC := DeriveNC(sc, mbIdx, i, false)
					acCoeffs, tc, err := cavlc.DecodeResidualBlock(br, nC, 15)
					if err != nil {
						return fmt.Errorf("i16x16 AC[%d]: %w", i, err)
					}
					for j := range 15 {
						mb.Intra16x16ACLevel[i][j] = acCoeffs[j]
					}
					mb.NzCoeffLuma[i] = tc
				}
			}
		}
	} else if mb.MBType == MBTypeINxN {
		if mb.TransformSize8x8 {
			// I_8x8: decode as 4 groups of 4 sub-blocks (each 16 coefficients)
			for i8x8 := range 4 {
				if mb.CBPLuma&(1<<uint(i8x8)) != 0 {
					for i4x4 := range 4 {
						blkIdx := i8x8*4 + i4x4
						nC := DeriveNC(sc, mbIdx, blkIdx, false)
						subCoeffs, tc, err := cavlc.DecodeResidualBlock(br, nC, 16)
						if err != nil {
							return fmt.Errorf("i8x8[%d] sub[%d]: %w", i8x8, i4x4, err)
						}
						// CAVLC sub-blocks use zigzagScan8x8CAVLC to map to raster positions.
						// Store directly in raster order.
						for j := range 16 {
							mb.LumaLevel8x8[i8x8][zigzagScan8x8CAVLC[i4x4*16+j]] = subCoeffs[j]
						}
						mb.NzCoeffLuma[blkIdx] = tc
					}
				}
			}
		} else {
			// I_4x4: decode 16 4x4 luma blocks
			for i := range 16 {
				i8x8 := i / 4
				if mb.CBPLuma&(1<<uint(i8x8)) != 0 {
					nC := DeriveNC(sc, mbIdx, i, false)
					coeffs, tc, err := cavlc.DecodeResidualBlock(br, nC, 16)
					if err != nil {
						return fmt.Errorf("i4x4[%d]: %w", i, err)
					}
					for j := range 16 {
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
		for iCbCr := range 2 {
			dcCoeffs, _, err := cavlc.DecodeResidualBlock(br, -1, 4)
			if err != nil {
				return fmt.Errorf("chroma DC[%d]: %w", iCbCr, err)
			}
			for j := range 4 {
				mb.ChromaDCLevel[iCbCr][j] = dcCoeffs[j]
			}
		}

		// Chroma AC for each component and block
		if mb.CBPChroma > 1 {
			for iCbCr := range 2 {
				for i := range 4 {
					blkIdx := iCbCr*4 + i
					nC := DeriveChromaNC(sc, mbIdx, blkIdx)
					acCoeffs, tc, err := cavlc.DecodeResidualBlock(br, nC, 15)
					if err != nil {
						return fmt.Errorf("chroma AC[%d][%d]: %w", iCbCr, i, err)
					}
					for j := range 15 {
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
