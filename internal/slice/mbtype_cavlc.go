package slice

import (
	"fmt"

	"github.com/Eyevinn/hi264/internal/cavlc"
)

// DecodeMBTypeIntraCAVLC decodes mb_type for I-slices using CAVLC (ue(v)).
// The ue(v) value maps directly to mb_type per Table 7-11.
func DecodeMBTypeIntraCAVLC(br *cavlc.BitReader) (int, error) {
	val, err := br.ReadUE()
	if err != nil {
		return 0, fmt.Errorf("mb_type: %w", err)
	}
	if val > 25 {
		return 0, fmt.Errorf("mb_type %d out of range", val)
	}
	return int(val), nil
}

// DecodeTransformSize8x8FlagCAVLC decodes transform_size_8x8_flag using u(1).
func DecodeTransformSize8x8FlagCAVLC(br *cavlc.BitReader) (bool, error) {
	return br.ReadFlag()
}

// DecodeIntra4x4PredModeCAVLC decodes prev_intra4x4_pred_mode_flag and rem_intra4x4_pred_mode.
// Returns (prevFlag, rem).
func DecodeIntra4x4PredModeCAVLC(br *cavlc.BitReader) (bool, int, error) {
	flag, err := br.ReadFlag()
	if err != nil {
		return false, 0, fmt.Errorf("prev_intra4x4_pred_mode_flag: %w", err)
	}
	if flag {
		return true, -1, nil
	}
	rem, err := br.ReadBits(3)
	if err != nil {
		return false, 0, fmt.Errorf("rem_intra4x4_pred_mode: %w", err)
	}
	return false, int(rem), nil
}

// DecodeIntraChromaPredModeCAVLC decodes intra_chroma_pred_mode using ue(v).
func DecodeIntraChromaPredModeCAVLC(br *cavlc.BitReader) (int, error) {
	val, err := br.ReadUE()
	if err != nil {
		return 0, fmt.Errorf("intra_chroma_pred_mode: %w", err)
	}
	if val > 3 {
		return 0, fmt.Errorf("intra_chroma_pred_mode %d out of range", val)
	}
	return int(val), nil
}

// DecodeCBPCAVLC decodes coded_block_pattern using me(v) for I-slices.
// Returns (cbpLuma, cbpChroma).
func DecodeCBPCAVLC(br *cavlc.BitReader) (int, int, error) {
	val, err := br.ReadUE()
	if err != nil {
		return 0, 0, fmt.Errorf("cbp: %w", err)
	}
	if val > 47 {
		return 0, 0, fmt.Errorf("cbp %d out of range for I-slice", val)
	}
	cbp := int(cavlc.GolombToIntra4x4CBP(int(val)))
	cbpLuma := cbp & 0x0F
	cbpChroma := (cbp >> 4) & 0x03
	return cbpLuma, cbpChroma, nil
}

// DecodeQPDeltaCAVLC decodes mb_qp_delta using se(v).
func DecodeQPDeltaCAVLC(br *cavlc.BitReader) (int, error) {
	val, err := br.ReadSE()
	if err != nil {
		return 0, fmt.Errorf("mb_qp_delta: %w", err)
	}
	return int(val), nil
}

// DeriveNC derives the nC value for CAVLC coeff_token table selection.
// Uses the non-zero coefficient counts from left (A) and top (B) neighbor blocks.
func DeriveNC(sc *SliceContext, mbIdx int, blkIdx int, isChromaDC bool) int {
	if isChromaDC {
		return -1
	}

	// Get left and top non-zero coefficient counts
	nA := -1
	nB := -1

	leftIdx := lumaLeftNeighbor[blkIdx]
	if leftIdx >= 0 {
		nA = sc.MBs[mbIdx].NzCoeffLuma[leftIdx]
	} else if mbA := sc.MBAvailA(mbIdx); mbA != nil {
		nA = mbA.NzCoeffLuma[lumaLeftFromMBA[blkIdx]]
	}

	topIdx := lumaTopNeighbor[blkIdx]
	if topIdx >= 0 {
		nB = sc.MBs[mbIdx].NzCoeffLuma[topIdx]
	} else if mbB := sc.MBAvailB(mbIdx); mbB != nil {
		nB = mbB.NzCoeffLuma[lumaTopFromMBB[blkIdx]]
	}

	if nA >= 0 && nB >= 0 {
		return (nA + nB + 1) >> 1
	}
	if nA >= 0 {
		return nA
	}
	if nB >= 0 {
		return nB
	}
	return 0
}

// DeriveChromaNC derives the nC value for CAVLC chroma AC blocks.
// blkIdx is the chroma block index: iCbCr*4 + i (0-7).
func DeriveChromaNC(sc *SliceContext, mbIdx int, blkIdx int) int {
	nA := -1
	nB := -1

	compBase := (blkIdx / 4) * 4 // 0 for Cb, 4 for Cr
	localIdx := blkIdx % 4       // 0-3 within component
	x := localIdx % 2
	y := localIdx / 2

	// Left neighbor
	if x > 0 {
		nA = sc.MBs[mbIdx].NzCoeffChroma[compBase+localIdx-1]
	} else if mbA := sc.MBAvailA(mbIdx); mbA != nil {
		nA = mbA.NzCoeffChroma[compBase+y*2+1]
	}

	// Top neighbor
	if y > 0 {
		nB = sc.MBs[mbIdx].NzCoeffChroma[compBase+localIdx-2]
	} else if mbB := sc.MBAvailB(mbIdx); mbB != nil {
		nB = mbB.NzCoeffChroma[compBase+2+x]
	}

	if nA >= 0 && nB >= 0 {
		return (nA + nB + 1) >> 1
	}
	if nA >= 0 {
		return nA
	}
	if nB >= 0 {
		return nB
	}
	return 0
}
