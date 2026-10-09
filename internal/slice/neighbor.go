// Package slice implements H.264/AVC slice data parsing including macroblock
// layer syntax and residual decoding using CABAC.
package slice

import (
	"github.com/Eyevinn/hi264/internal/cabac"
	"github.com/Eyevinn/hi264/internal/cavlc"
)

// MBType constants for I-slices (Table 7-11) and P-slices (Table 7-10).
const (
	MBTypePSkip  = -1 // P_Skip (all-skip P macroblock, copies from reference)
	MBTypeINxN   = 0  // I_NxN (I_4x4 or I_8x8 based on transform_size_8x8_flag)
	MBTypeI16x16 = 1  // I_16x16_0_0_0 through I_16x16_3_2_1 (values 1-24)
	MBTypeIPCM   = 25 // I_PCM
)

// I16x16 sub-type extraction from mb_type (1-24).
// mb_type = 1 + IntraPredMode + 4*cbp_chroma + (cbp_luma ? 12 : 0)
// Where IntraPredMode = 0..3, cbp_chroma = 0..2, cbp_luma = 0 or 15.

// I16x16PredMode returns the intra 16x16 prediction mode from mb_type.
func I16x16PredMode(mbType int) int {
	return (mbType - 1) % 4
}

// I16x16CBPLuma returns the luma CBP for I_16x16 (0 or 15).
func I16x16CBPLuma(mbType int) int {
	if (mbType-1)/12 > 0 {
		return 15
	}
	return 0
}

// I16x16CBPChroma returns the chroma CBP for I_16x16 (0, 1, or 2).
func I16x16CBPChroma(mbType int) int {
	return ((mbType - 1) / 4) % 3
}

// MBData stores decoded information for a single macroblock.
type MBData struct {
	SliceNum            int // 1-based number of the slice containing the MB; 0 until a slice decodes it
	MBType              int
	TransformSize8x8    bool
	IntraPredMode16x16  int     // for I_16x16
	Intra4x4PredMode    [16]int // for I_4x4
	Intra8x8PredMode    [4]int  // for I_8x8
	IntraChromaPredMode int
	CBPLuma             int // coded block pattern for luma (0-15, bits for 8x8 blocks)
	CBPChroma           int // coded block pattern for chroma (0, 1, or 2)
	QPY                 int // luma QP after delta
	QPDelta             int
	// Residual coefficients
	Intra16x16DCLevel [16]int32
	Intra16x16ACLevel [16][15]int32
	LumaLevel4x4      [16][16]int32
	LumaLevel8x8      [4][64]int32
	ChromaDCLevel     [2][4]int32 // for 4:2:0
	ChromaACLevel     [2][4][15]int32

	// coded_block_flag tracking per block category
	CodedBlockFlag [6][16]uint8

	// Non-zero coefficient count (for neighbor context)
	NzCoeffLuma   [16]int
	NzCoeffChroma [8]int
	TotalCoeff    [24]int // combined luma+chroma tracking
}

// SliceParams holds the slice header values that decoding a slice and
// deblocking its macroblocks depend on.
type SliceParams struct {
	FirstMB  int // first_mb_in_slice
	SliceQPY int // 26 + pic_init_qp_minus26 + slice_qp_delta

	DisableDeblockingFilterIdc int
	FilterOffsetA              int // slice_alpha_c0_offset_div2 * 2
	FilterOffsetB              int // slice_beta_offset_div2 * 2
}

// SliceContext holds the decoding state of one picture: the macroblock data
// shared by all its slices, and the entropy decoding state of the slice
// being decoded.
type SliceContext struct {
	Cabac    *cabac.Decoder
	Ctx      *[1024]cabac.CtxState
	MBWidth  int
	MBHeight int
	TotalMBs int
	QPY      int // QP of the previous MB in the current slice (QPY,PRED)
	MBs      []MBData

	// Slices holds the parameters of the decoded slices; MBData.SliceNum-1
	// indexes it.
	Slices     []SliceParams
	decodedMBs int // macroblocks assigned to a slice so far

	// CAVLC support
	IsCAVLC bool
	Br      *cavlc.BitReader

	// PPS/SPS parameters needed during decoding
	Transform8x8ModeFlag bool
	ChromaArrayType      int // 0=monochrome, 1=4:2:0, 2=4:2:2, 3=4:4:4
	BitDepthY            int
	BitDepthC            int
	ChromaQpIndexOffset  int // PPS chroma_qp_index_offset

	// Previous MB state for qp_delta context
	PrevMBQPDeltaNonZero bool

	// Debug tracing
	TraceMBCMP bool

	// Reusable buffers for DecodeResidual (avoids per-call allocations)
	coeffBuf   [64]int32
	sigFlags   [64]bool
	sigIndices [64]int
}

// The MBAvail functions return a neighbour of macroblock mbIdx, or nil if it
// is not available (clauses 6.4.8 and 6.4.9). A neighbour is not available
// when it lies outside the picture or in a different slice: each slice is
// decoded as if the other slices of the picture did not exist.

// MBAvailA returns the left neighbor MB data, or nil if not available.
func (sc *SliceContext) MBAvailA(mbIdx int) *MBData {
	if mbIdx%sc.MBWidth == 0 {
		return nil
	}
	return sc.sameSlice(mbIdx, mbIdx-1)
}

// MBAvailB returns the top neighbor MB data, or nil if not available.
func (sc *SliceContext) MBAvailB(mbIdx int) *MBData {
	if mbIdx < sc.MBWidth {
		return nil
	}
	return sc.sameSlice(mbIdx, mbIdx-sc.MBWidth)
}

// MBAvailC returns the top-right neighbor MB data, or nil if not available.
func (sc *SliceContext) MBAvailC(mbIdx int) *MBData {
	if mbIdx < sc.MBWidth || (mbIdx+1)%sc.MBWidth == 0 {
		return nil
	}
	return sc.sameSlice(mbIdx, mbIdx-sc.MBWidth+1)
}

// MBAvailD returns the top-left neighbor MB data, or nil if not available.
func (sc *SliceContext) MBAvailD(mbIdx int) *MBData {
	if mbIdx < sc.MBWidth || mbIdx%sc.MBWidth == 0 {
		return nil
	}
	return sc.sameSlice(mbIdx, mbIdx-sc.MBWidth-1)
}

// sameSlice returns macroblock nIdx if it is in the same slice as macroblock
// mbIdx, and nil otherwise.
func (sc *SliceContext) sameSlice(mbIdx, nIdx int) *MBData {
	n := &sc.MBs[nIdx]
	if n.SliceNum != sc.MBs[mbIdx].SliceNum {
		return nil
	}
	return n
}
