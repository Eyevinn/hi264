package encode

import (
	"bytes"
	"fmt"

	"github.com/Eyevinn/mp4ff/avc"

	"github.com/Eyevinn/hi264/internal/cabac"
	"github.com/Eyevinn/hi264/internal/context"
	"github.com/Eyevinn/hi264/pkg/yuv"
)

// FrameEncoder encodes a grid-based image as an H.264 IDR frame.
type FrameEncoder struct {
	Grid            *yuv.Grid
	Colors          yuv.ColorMap
	QP              int
	DisableDeblock  int            // 0=enable, 1=disable
	CABAC           bool           // use CABAC entropy coding (Main profile) instead of CAVLC (Baseline)
	MaxNumRefFrames int            // max_num_ref_frames in SPS (0 for IDR-only, 1+ for P-frames)
	Width           int            // actual pixel width for SPS (0 = Grid.Width*16)
	Height          int            // actual pixel height for SPS (0 = Grid.Height*16)
	ColorSpace      yuv.ColorSpace // YCbCr matrix standard (default BT601)
	Range           yuv.Range      // sample value range (default LimitedRange)
	FPS             int            // frame rate for level selection (0 = ignore MBPS)
	Kbps            int            // bitrate in kbit/s for level selection (0 = ignore)
}

// Encode produces an Annex-B bitstream containing SPS, PPS, and one IDR slice.
func (e *FrameEncoder) Encode() ([]byte, error) {
	var buf bytes.Buffer
	if err := e.EncodeSPSPPS(&buf); err != nil {
		return nil, err
	}
	slice, err := e.EncodeSlice(0)
	if err != nil {
		return nil, err
	}
	buf.Write(slice)
	return buf.Bytes(), nil
}

// EncodeSPSPPS writes SPS and PPS NALUs to buf (profile-aware).
func (e *FrameEncoder) EncodeSPSPPS(buf *bytes.Buffer) error {
	width := e.Grid.Width * 16
	height := e.Grid.Height * 16
	if e.Width > 0 {
		width = e.Width
	}
	if e.Height > 0 {
		height = e.Height
	}

	level := ChooseLevel(width, height, e.FPS, e.Kbps, e.CABAC)
	if e.CABAC {
		spsRBSP := EncodeSPSMain(width, height, e.MaxNumRefFrames, level, e.ColorSpace, e.Range)
		if err := WriteNALU(buf, 7, 3, spsRBSP); err != nil {
			return fmt.Errorf("write SPS: %w", err)
		}
		ppsRBSP := EncodePPSCABAC(e.DisableDeblock)
		if err := WriteNALU(buf, 8, 3, ppsRBSP); err != nil {
			return fmt.Errorf("write PPS: %w", err)
		}
	} else {
		spsRBSP := EncodeSPS(width, height, e.MaxNumRefFrames, level, e.ColorSpace, e.Range)
		if err := WriteNALU(buf, 7, 3, spsRBSP); err != nil {
			return fmt.Errorf("write SPS: %w", err)
		}
		ppsRBSP := EncodePPS(e.DisableDeblock)
		if err := WriteNALU(buf, 8, 3, ppsRBSP); err != nil {
			return fmt.Errorf("write PPS: %w", err)
		}
	}
	return nil
}

// EncodeSlice encodes a single IDR slice NALU and returns it as Annex-B framed bytes.
// idrPicID is the idr_pic_id value (alternating 0/1 for consecutive IDR pictures).
func (e *FrameEncoder) EncodeSlice(idrPicID uint32) ([]byte, error) {
	if e.CABAC {
		return e.encodeSliceCABAC(idrPicID)
	}
	return e.encodeSliceCAVLC(idrPicID)
}

// EncodePSkipSlice encodes a P_Skip slice (all MBs skip, copying from reference).
// frameNum is the frame_num value for this slice.
// Returns an Annex-B framed non-IDR NALU (type=1, ref_idc=2).
func (e *FrameEncoder) EncodePSkipSlice(frameNum uint32) ([]byte, error) {
	width := e.Grid.Width * 16
	height := e.Grid.Height * 16
	if e.Width > 0 {
		width = e.Width
	}
	if e.Height > 0 {
		height = e.Height
	}
	sps := &avc.SPS{
		Width:            uint(width),
		Height:           uint(height),
		PicOrderCntType:  0,
		FrameMbsOnlyFlag: true,
	}
	pps := &avc.PPS{
		DeblockingFilterControlPresentFlag: true,
		EntropyCodingModeFlag:              e.CABAC,
		PicInitQpMinus26:                   e.QP - 26,
	}
	return EncodePSkipSlice(sps, pps, frameNum, e.DisableDeblock)
}

func (e *FrameEncoder) encodeSliceCAVLC(idrPicID uint32) ([]byte, error) {
	width := e.Grid.Width * 16
	height := e.Grid.Height * 16
	mbWidth := e.Grid.Width
	mbHeight := e.Grid.Height

	qpDelta := int32(e.QP - 26) // pic_init_qp = 26, so delta = QP - 26

	// Build slice (header + MB data) in a single BitWriter to maintain bit alignment
	sliceW := NewBitWriter()
	WriteSliceHeader(sliceW, qpDelta, e.DisableDeblock, idrPicID)

	// Encode macroblocks
	// Track nC (number of non-zero coefficients) per 4x4 block for context
	// nC is computed from neighbors A (left) and B (above)
	nCLuma := make([][]int, mbHeight*4)
	for i := range nCLuma {
		nCLuma[i] = make([]int, mbWidth*4)
	}
	nCCb := make([][]int, mbHeight*2)
	for i := range nCCb {
		nCCb[i] = make([]int, mbWidth*2)
	}
	nCCr := make([][]int, mbHeight*2)
	for i := range nCCr {
		nCCr[i] = make([]int, mbWidth*2)
	}

	// Reconstructed pixel values (needed for DC prediction of neighbors)
	// Use flat slices with stride to minimize allocations.
	strideY := width
	strideC := width / 2
	reconY := make([]uint8, height*strideY)
	reconCb := make([]uint8, (height/2)*strideC)
	reconCr := make([]uint8, (height/2)*strideC)

	for mbY := range mbHeight {
		for mbX := range mbWidth {
			ch := e.Grid.Chars[mbY][mbX]
			c, ok := e.Colors[ch]
			if !ok {
				return nil, fmt.Errorf("no color for char %q at (%d,%d)", string(ch), mbX, mbY)
			}

			err := e.encodeMB(sliceW, mbX, mbY, c, mbWidth, mbHeight,
				nCLuma, nCCb, nCCr, reconY, strideY, reconCb, reconCr, strideC)
			if err != nil {
				return nil, fmt.Errorf("encode MB (%d,%d): %w", mbX, mbY, err)
			}
		}
	}

	// RBSP trailing bits
	sliceW.WriteBit(1) // rbsp_stop_one_bit
	sliceW.AlignToByte()

	var buf bytes.Buffer
	sliceRBSP := sliceW.Bytes()
	err := WriteNALU(&buf, 5, 3, sliceRBSP)
	if err != nil {
		return nil, fmt.Errorf("write IDR: %w", err)
	}

	return buf.Bytes(), nil
}

// encMBState tracks encoder-side MB state for CABAC context derivation.
type encMBState struct {
	mbType              int
	intraChromaPredMode int
	cbpChroma           int
	codedBlockFlag      [6][16]uint8
}

func (e *FrameEncoder) encodeSliceCABAC(idrPicID uint32) ([]byte, error) {
	width := e.Grid.Width * 16
	height := e.Grid.Height * 16
	mbWidth := e.Grid.Width
	mbHeight := e.Grid.Height

	// Build slice header with CABAC alignment
	qpDelta := int32(e.QP - 26)
	headerW := NewBitWriter()
	WriteSliceHeaderCABAC(headerW, qpDelta, e.DisableDeblock, idrPicID)
	headerBytes := headerW.Bytes()

	// Initialize CABAC encoder and context models
	enc := cabac.NewEncoder()
	models := context.InitModels(e.QP, 2, 0) // sliceType=2 (I-slice)
	ctx := models[:]

	// MB state for neighbor context derivation
	mbStates := make([]encMBState, mbWidth*mbHeight)

	// Reconstructed pixel values (needed for DC prediction of neighbors)
	// Use flat slices with stride to minimize allocations.
	strideY := width
	strideC := width / 2
	reconY := make([]uint8, height*strideY)
	reconCb := make([]uint8, (height/2)*strideC)
	reconCr := make([]uint8, (height/2)*strideC)

	for mbY := range mbHeight {
		for mbX := range mbWidth {
			ch := e.Grid.Chars[mbY][mbX]
			c, ok := e.Colors[ch]
			if !ok {
				return nil, fmt.Errorf("no color for char %q at (%d,%d)", string(ch), mbX, mbY)
			}

			mbIdx := mbY*mbWidth + mbX
			err := e.encodeMBCABAC(enc, ctx, mbStates, mbIdx, mbX, mbY, c,
				mbWidth, mbHeight, reconY, strideY, reconCb, reconCr, strideC)
			if err != nil {
				return nil, fmt.Errorf("encode MB (%d,%d): %w", mbX, mbY, err)
			}

			// end_of_slice: terminate(1) for last MB, terminate(0) otherwise
			isLast := mbY == mbHeight-1 && mbX == mbWidth-1
			if isLast {
				enc.EncodeTerminate(1)
			} else {
				enc.EncodeTerminate(0)
			}
		}
	}

	cabacBytes := enc.Flush()

	// Concatenate header + CABAC data as RBSP
	sliceRBSP := make([]byte, 0, len(headerBytes)+len(cabacBytes))
	sliceRBSP = append(sliceRBSP, headerBytes...)
	sliceRBSP = append(sliceRBSP, cabacBytes...)

	var buf bytes.Buffer
	err := WriteNALU(&buf, 5, 3, sliceRBSP)
	if err != nil {
		return nil, fmt.Errorf("write IDR: %w", err)
	}

	return buf.Bytes(), nil
}

func (e *FrameEncoder) encodeMBCABAC(enc *cabac.Encoder, ctx []cabac.CtxState,
	mbStates []encMBState, mbIdx, mbX, mbY int, c yuv.Color,
	mbWidth, mbHeight int,
	reconY []uint8, strideY int, reconCb, reconCr []uint8, strideC int) error {

	qp := e.QP
	qpc := ChromaQP(qp)

	// Select best luma prediction mode
	lumaMode, lumaPred := selectLumaMode(reconY, strideY, mbX, mbY, c.Y)
	lumaResidual := int32(c.Y) - int32(lumaPred)

	dc4x4 := ForwardTransformDC4x4(lumaResidual)
	var dcMatrix [16]int32
	for i := range dcMatrix {
		dcMatrix[i] = dc4x4
	}
	hadamardResult := ForwardHadamard4x4(dcMatrix)
	quantDC := QuantizeDC4x4(hadamardResult, qp, 16)

	lumaCBP := 0
	chromaCBP := 0

	// Select best chroma prediction mode
	chromaMode, cbPreds, crPreds := selectChromaMode(reconCb, reconCr, strideC, mbX, mbY, c.Cb, c.Cr)

	var cbDCMatrix [4]int32
	var crDCMatrix [4]int32
	for i := range 4 {
		cbDCMatrix[i] = ForwardTransformDC4x4(int32(c.Cb) - int32(cbPreds[i]))
		crDCMatrix[i] = ForwardTransformDC4x4(int32(c.Cr) - int32(crPreds[i]))
	}
	cbHadamard := ForwardHadamard2x2(cbDCMatrix)
	crHadamard := ForwardHadamard2x2(crDCMatrix)
	quantCbDC := QuantizeChromaDC2x2(cbHadamard, qpc)
	quantCrDC := QuantizeChromaDC2x2(crHadamard, qpc)

	hasChromaDC := false
	for i := range 4 {
		if quantCbDC[i] != 0 || quantCrDC[i] != 0 {
			hasChromaDC = true
			break
		}
	}
	if hasChromaDC {
		chromaCBP = 1
	}

	mbType := 1 + lumaMode + 4*chromaCBP + 12*lumaCBP

	// --- CABAC encoding of syntax elements ---

	// Neighbor availability
	var leftState, topState *encMBState
	if mbX > 0 {
		leftState = &mbStates[mbIdx-1]
	}
	if mbY > 0 {
		topState = &mbStates[mbIdx-mbWidth]
	}

	// mb_type: all neighbors are I_16x16 (not I_NxN), so leftNotINxN=true if available
	leftNotINxN := leftState != nil // all our MBs are I_16x16, which is not I_NxN
	topNotINxN := topState != nil
	encodeMBTypeI16x16(enc, ctx, mbType, leftNotINxN, topNotINxN)

	// intra_chroma_pred_mode
	leftChromaNZ := leftState != nil && leftState.intraChromaPredMode != 0
	topChromaNZ := topState != nil && topState.intraChromaPredMode != 0
	encodeChromaPredMode(enc, ctx, chromaMode, leftChromaNZ, topChromaNZ)

	// mb_qp_delta = 0
	encodeQPDelta(enc, ctx, 0, false)

	// Residual: Intra16x16DCLevel
	dcCBFCtx := deriveDCCBFCtx(leftState, topState)
	var dcCoeffs [16]int32
	copy(dcCoeffs[:], quantDC[:])
	dcCBF := encodeResidualBlockCABAC(enc, ctx, encCtxBlockCatIntra16x16DC, dcCBFCtx, dcCoeffs[:], 16)

	// Store coded_block_flag for DC
	mbStates[mbIdx].codedBlockFlag[encCtxBlockCatIntra16x16DC][0] = dcCBF

	// Intra16x16ACLevel: all zero for flat blocks, but we still write CBF if lumaCBP != 0
	// For our flat blocks lumaCBP is always 0, so no AC blocks to encode.
	// Set all AC CBF to 0 for neighbor tracking.
	for blk := range 16 {
		mbStates[mbIdx].codedBlockFlag[encCtxBlockCatIntra16x16AC][blk] = 0
	}

	// Chroma DC
	if chromaCBP > 0 {
		// Cb DC
		cbCBFCtx := deriveChromaDCCBFCtx(leftState, topState, 0)
		cbCBF := encodeResidualBlockCABAC(enc, ctx, encCtxBlockCatChromaDC, cbCBFCtx, quantCbDC[:], 4)
		mbStates[mbIdx].codedBlockFlag[encCtxBlockCatChromaDC][0] = cbCBF

		// Cr DC
		crCBFCtx := deriveChromaDCCBFCtx(leftState, topState, 1)
		crCBF := encodeResidualBlockCABAC(enc, ctx, encCtxBlockCatChromaDC, crCBFCtx, quantCrDC[:], 4)
		mbStates[mbIdx].codedBlockFlag[encCtxBlockCatChromaDC][1] = crCBF
	}

	// Chroma AC: all zero for flat blocks (chromaCBP == 1 means DC only)

	// Update MB state
	mbStates[mbIdx].mbType = mbType
	mbStates[mbIdx].intraChromaPredMode = chromaMode
	mbStates[mbIdx].cbpChroma = chromaCBP

	// Update reconstructed pixels
	reconLumaVal := reconstructLumaValue(quantDC, lumaPred, qp)
	for y := range 16 {
		off := (mbY*16+y)*strideY + mbX*16
		for x := range 16 {
			reconY[off+x] = reconLumaVal
		}
	}
	reconCbVals := reconstructChromaValues4x4(quantCbDC, cbPreds, qpc)
	reconCrVals := reconstructChromaValues4x4(quantCrDC, crPreds, qpc)
	for blk := range 4 {
		x0 := (blk % 2) * 4
		y0 := (blk / 2) * 4
		for y := range 4 {
			offCb := (mbY*8+y0+y)*strideC + mbX*8 + x0
			offCr := offCb
			for x := range 4 {
				reconCb[offCb+x] = reconCbVals[blk]
				reconCr[offCr+x] = reconCrVals[blk]
			}
		}
	}

	return nil
}

// deriveDCCBFCtx derives the coded_block_flag context for Intra16x16DC.
// condTermFlagA/B default to 1 when neighbor is not available.
func deriveDCCBFCtx(left, top *encMBState) int {
	condA := 1
	condB := 1
	if left != nil {
		condA = int(left.codedBlockFlag[encCtxBlockCatIntra16x16DC][0])
	}
	if top != nil {
		condB = int(top.codedBlockFlag[encCtxBlockCatIntra16x16DC][0])
	}
	return condA + 2*condB
}

// deriveChromaDCCBFCtx derives the coded_block_flag context for ChromaDC.
// iCbCr: 0 for Cb, 1 for Cr.
func deriveChromaDCCBFCtx(left, top *encMBState, iCbCr int) int {
	condA := 1
	condB := 1
	if left != nil {
		condA = int(left.codedBlockFlag[encCtxBlockCatChromaDC][iCbCr])
	}
	if top != nil {
		condB = int(top.codedBlockFlag[encCtxBlockCatChromaDC][iCbCr])
	}
	return condA + 2*condB
}

func (e *FrameEncoder) encodeMB(w *BitWriter, mbX, mbY int, c yuv.Color,
	mbWidth, mbHeight int,
	nCLuma [][]int, nCCb, nCCr [][]int,
	reconY []uint8, strideY int, reconCb, reconCr []uint8, strideC int) error {

	qp := e.QP
	qpc := ChromaQP(qp)

	// Select best luma prediction mode
	lumaMode, lumaPred := selectLumaMode(reconY, strideY, mbX, mbY, c.Y)
	lumaResidual := int32(c.Y) - int32(lumaPred)

	// For I_16x16: each 4x4 block has DC = 4*residual (forward DCT of constant)
	dc4x4 := ForwardTransformDC4x4(lumaResidual)

	// Forward Hadamard on the 16 identical DC values
	var dcMatrix [16]int32
	for i := range dcMatrix {
		dcMatrix[i] = dc4x4
	}
	hadamardResult := ForwardHadamard4x4(dcMatrix)

	// Forward quantize DC
	quantDC := QuantizeDC4x4(hadamardResult, qp, 16)

	// Check if any AC coefficients are non-zero (for flat blocks, they are all zero)
	// and compute CBP
	lumaCBP := 0   // 0 = no AC coefficients
	chromaCBP := 0 // 0 = no chroma residual

	// Select best chroma prediction mode
	chromaMode, cbPreds, crPreds := selectChromaMode(reconCb, reconCr, strideC, mbX, mbY, c.Cb, c.Cr)

	// Chroma DC: each 4x4 sub-block has its own residual
	var cbDCMatrix [4]int32
	var crDCMatrix [4]int32
	for i := range 4 {
		cbDCMatrix[i] = ForwardTransformDC4x4(int32(c.Cb) - int32(cbPreds[i]))
		crDCMatrix[i] = ForwardTransformDC4x4(int32(c.Cr) - int32(crPreds[i]))
	}
	cbHadamard := ForwardHadamard2x2(cbDCMatrix)
	crHadamard := ForwardHadamard2x2(crDCMatrix)

	quantCbDC := QuantizeChromaDC2x2(cbHadamard, qpc)
	quantCrDC := QuantizeChromaDC2x2(crHadamard, qpc)

	// Check if any chroma DC is non-zero
	hasChromaDC := false
	for i := range 4 {
		if quantCbDC[i] != 0 || quantCrDC[i] != 0 {
			hasChromaDC = true
			break
		}
	}
	if hasChromaDC {
		chromaCBP = 1 // DC only
	}

	// Determine I_16x16 mb_type
	// mb_type = 1 + pred_mode + 4*cbp_chroma + 12*cbp_luma_flag
	mbType := 1 + lumaMode + 4*chromaCBP + 12*lumaCBP

	// Write mb_type as ue(v) — Table 7-11: mb_type IS the ue(v) code value
	w.WriteUE(uint32(mbType))

	// intra_chroma_pred_mode
	w.WriteUE(uint32(chromaMode))

	// mb_qp_delta = 0
	w.WriteSE(0)

	// Intra16x16DCLevel: always present for I_16x16
	// Write the 16 DC coefficients
	var dcCoeffs [16]int32
	copy(dcCoeffs[:], quantDC[:])
	nCDC := computeNC4x4(nCLuma, mbX*4, mbY*4)
	EncodeResidualBlock(w, dcCoeffs[:], nCDC, 16)

	// For flat blocks, all AC coefficients are zero.
	// If lumaCBP == 0, we don't write AC blocks.
	// Even when lumaCBP == 0, we still need to update nC for neighbor tracking.
	for blk := range 16 {
		bx := inverseRasterX4x4[blk]
		by := inverseRasterY4x4[blk]
		nCLuma[mbY*4+by/4][mbX*4+bx/4] = 0
	}

	// Chroma
	if chromaCBP > 0 {
		// Chroma DC blocks
		EncodeResidualBlock(w, quantCbDC[:], -1, 4)
		EncodeResidualBlock(w, quantCrDC[:], -1, 4)

		// Chroma AC blocks (all zero for flat blocks)
		// chromaCBP == 1 means DC only, no AC
	}

	// Update chroma nC (all zero for flat blocks)
	for blk := range 4 {
		bx := (blk % 2)
		by := (blk / 2)
		nCCb[mbY*2+by][mbX*2+bx] = 0
		nCCr[mbY*2+by][mbX*2+bx] = 0
	}

	// Update reconstructed pixels
	// Need to do inverse quant + inverse transform to get actual reconstructed values
	reconLumaVal := reconstructLumaValue(quantDC, lumaPred, qp)
	for y := range 16 {
		off := (mbY*16+y)*strideY + mbX*16
		for x := range 16 {
			reconY[off+x] = reconLumaVal
		}
	}

	reconCbVals := reconstructChromaValues4x4(quantCbDC, cbPreds, qpc)
	reconCrVals := reconstructChromaValues4x4(quantCrDC, crPreds, qpc)
	for blk := range 4 {
		x0 := (blk % 2) * 4
		y0 := (blk / 2) * 4
		for y := range 4 {
			offCb := (mbY*8+y0+y)*strideC + mbX*8 + x0
			offCr := offCb
			for x := range 4 {
				reconCb[offCb+x] = reconCbVals[blk]
				reconCr[offCr+x] = reconCrVals[blk]
			}
		}
	}

	return nil
}

func computeLumaDCPred(reconY []uint8, strideY, mbX, mbY int) uint8 {
	hasTop := mbY > 0
	hasLeft := mbX > 0

	if !hasTop && !hasLeft {
		return 128
	}

	sum := 0
	count := 0

	if hasTop {
		off := (mbY*16-1)*strideY + mbX*16
		for x := range 16 {
			sum += int(reconY[off+x])
		}
		count += 16
	}

	if hasLeft {
		off := mbY*16*strideY + mbX*16 - 1
		for y := range 16 {
			sum += int(reconY[off+y*strideY])
		}
		count += 16
	}

	if count == 32 {
		return uint8((sum + 16) >> 5)
	}
	return uint8((sum + 8) >> 4)
}

// computeLumaVerticalPred returns the prediction value for Vertical mode (mode 0).
// For flat blocks, all top neighbor pixels are identical, so we sample one.
func computeLumaVerticalPred(reconY []uint8, strideY, mbX, mbY int) uint8 {
	return reconY[(mbY*16-1)*strideY+mbX*16]
}

// computeLumaHorizontalPred returns the prediction value for Horizontal mode (mode 1).
// For flat blocks, all left neighbor pixels are identical, so we sample one.
func computeLumaHorizontalPred(reconY []uint8, strideY, mbX, mbY int) uint8 {
	return reconY[mbY*16*strideY+mbX*16-1]
}

// selectLumaMode picks the best I_16x16 luma prediction mode (0=Vert, 1=Horiz, 2=DC)
// for a flat block with target value targetY.
func selectLumaMode(reconY []uint8, strideY, mbX, mbY int, targetY uint8) (mode int, pred uint8) {
	bestMode := 2
	bestPred := computeLumaDCPred(reconY, strideY, mbX, mbY)
	bestErr := absInt(int(targetY) - int(bestPred))

	if mbY > 0 {
		vPred := computeLumaVerticalPred(reconY, strideY, mbX, mbY)
		vErr := absInt(int(targetY) - int(vPred))
		if vErr < bestErr {
			bestMode, bestPred, bestErr = 0, vPred, vErr
		}
	}

	if mbX > 0 {
		hPred := computeLumaHorizontalPred(reconY, strideY, mbX, mbY)
		hErr := absInt(int(targetY) - int(hPred))
		if hErr < bestErr {
			bestMode, bestPred, _ = 1, hPred, hErr
		}
	}

	return bestMode, bestPred
}

// computeChromaVerticalPreds4x4 returns uniform vertical predictions for each 4x4 sub-block.
// For flat blocks, all top neighbor pixels within each 4x4 column are identical.
func computeChromaVerticalPreds4x4(recon []uint8, strideC, mbX, mbY int) [4]uint8 {
	val := recon[(mbY*8-1)*strideC+mbX*8]
	return [4]uint8{val, val, val, val}
}

// computeChromaHorizontalPreds4x4 returns uniform horizontal predictions for each 4x4 sub-block.
// For flat blocks, all left neighbor pixels within each 4x4 row are identical.
func computeChromaHorizontalPreds4x4(recon []uint8, strideC, mbX, mbY int) [4]uint8 {
	val := recon[mbY*8*strideC+mbX*8-1]
	return [4]uint8{val, val, val, val}
}

// selectChromaMode picks the best chroma prediction mode (0=DC, 1=Horiz, 2=Vert)
// for flat blocks with target values targetCb, targetCr.
func selectChromaMode(reconCb, reconCr []uint8, strideC, mbX, mbY int,
	targetCb, targetCr uint8) (mode int, cbPreds, crPreds [4]uint8) {

	bestMode := 0
	bestCbPreds := computeChromaDCPreds4x4(reconCb, strideC, mbX, mbY)
	bestCrPreds := computeChromaDCPreds4x4(reconCr, strideC, mbX, mbY)
	bestErr := chromaPredError(targetCb, targetCr, bestCbPreds, bestCrPreds)

	if mbY > 0 {
		vCb := computeChromaVerticalPreds4x4(reconCb, strideC, mbX, mbY)
		vCr := computeChromaVerticalPreds4x4(reconCr, strideC, mbX, mbY)
		vErr := chromaPredError(targetCb, targetCr, vCb, vCr)
		if vErr < bestErr {
			bestMode, bestCbPreds, bestCrPreds, bestErr = 2, vCb, vCr, vErr
		}
	}

	if mbX > 0 {
		hCb := computeChromaHorizontalPreds4x4(reconCb, strideC, mbX, mbY)
		hCr := computeChromaHorizontalPreds4x4(reconCr, strideC, mbX, mbY)
		hErr := chromaPredError(targetCb, targetCr, hCb, hCr)
		if hErr < bestErr {
			bestMode, bestCbPreds, bestCrPreds, _ = 1, hCb, hCr, hErr
		}
	}

	return bestMode, bestCbPreds, bestCrPreds
}

// chromaPredError computes total absolute error across all 4 sub-blocks for both Cb and Cr.
func chromaPredError(targetCb, targetCr uint8, cbPreds, crPreds [4]uint8) int {
	total := 0
	for i := range 4 {
		total += absInt(int(targetCb)-int(cbPreds[i])) + absInt(int(targetCr)-int(crPreds[i]))
	}
	return total
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// computeChromaDCPreds4x4 computes per-sub-block chroma DC predictions,
// matching the decoder's predictChromaDC8x8 (spec section 8.3.4.1).
// Returns [4]uint8 for sub-blocks: TL(0), TR(1), BL(2), BR(3).
func computeChromaDCPreds4x4(recon []uint8, strideC, mbX, mbY int) [4]uint8 {
	hasTop := mbY > 0
	hasLeft := mbX > 0

	var top [8]int
	var left [8]int
	if hasTop {
		off := (mbY*8-1)*strideC + mbX*8
		for x := range 8 {
			top[x] = int(recon[off+x])
		}
	}
	if hasLeft {
		off := mbY*8*strideC + mbX*8 - 1
		for y := range 8 {
			left[y] = int(recon[off+y*strideC])
		}
	}

	var preds [4]uint8
	for blk := range 4 {
		x0 := (blk % 2) * 4
		y0 := (blk / 2) * 4

		sum := 0
		count := 0

		isTopRow := blk < 2
		isLeftCol := blk%2 == 0

		useTop := false
		useLeft := false

		switch {
		case isTopRow && isLeftCol: // TL: use both if available
			useTop = hasTop
			useLeft = hasLeft
		case isTopRow && !isLeftCol: // TR: prefer top, fallback to left
			if hasTop {
				useTop = true
			} else if hasLeft {
				useLeft = true
			}
		case !isTopRow && isLeftCol: // BL: prefer left, fallback to top
			if hasLeft {
				useLeft = true
			} else if hasTop {
				useTop = true
			}
		default: // BR: use both if available
			useTop = hasTop
			useLeft = hasLeft
		}

		if useTop {
			for x := x0; x < x0+4; x++ {
				sum += top[x]
				count++
			}
		}
		if useLeft {
			for y := y0; y < y0+4; y++ {
				sum += left[y]
				count++
			}
		}

		if count > 0 {
			preds[blk] = uint8((sum + count/2) / count)
		} else {
			preds[blk] = 128
		}
	}
	return preds
}

func computeNC4x4(nCLuma [][]int, blkX, blkY int) int {
	hasA := blkX > 0
	hasB := blkY > 0

	nA, nB := 0, 0
	if hasA {
		nA = nCLuma[blkY][blkX-1]
	}
	if hasB {
		nB = nCLuma[blkY-1][blkX]
	}

	if hasA && hasB {
		return (nA + nB + 1) / 2
	}
	if hasA {
		return nA
	}
	if hasB {
		return nB
	}
	return 0
}

// reconstructLumaValue computes the actual reconstructed luma value for a flat MB.
// This must match what the decoder produces.
func reconstructLumaValue(quantDC [16]int32, predDC uint8, qp int) uint8 {
	// Inverse: dequant DC, inverse Hadamard, inverse transform
	// For flat blocks: only DC[0] matters (all AC = 0)
	dequantMatrix := dequantDC4x4(quantDC, qp, 16)
	invHadamard := inverseHadamard4x4(dequantMatrix)

	// The inverse transform of [DC, 0, 0, ...] just gives DC >> 6 per sample
	// but with rounding: (DC + 32) >> 6
	// Actually for I_16x16 the DC is passed through without the regular dequant
	dcVal := invHadamard[0] // This is in raster order position (0,0)

	// Inverse 4x4 transform of [dcVal, 0, 0, ...]: result = (dcVal + 32) >> 6
	residual := (dcVal + 32) >> 6
	val := int32(predDC) + residual
	return clipU8(int(val))
}

// reconstructChromaValues4x4 reconstructs chroma values for each 4x4 sub-block.
// Must match the decoder order: inverse Hadamard FIRST, then dequant.
func reconstructChromaValues4x4(quantDC [4]int32, preds [4]uint8, qpc int) [4]uint8 {
	invHadamard := inverseHadamard2x2(quantDC)
	dcScaled := dequantChromaDC2x2(invHadamard, qpc, 16)

	var result [4]uint8
	for i := range 4 {
		residual := (dcScaled[i] + 32) >> 6
		val := int32(preds[i]) + residual
		result[i] = clipU8(int(val))
	}
	return result
}

func clipU8(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// Inlined inverse transforms to avoid import cycle with transform package
func dequantDC4x4(coeffs [16]int32, qp int, weightScaleDC int32) [16]int32 {
	var result [16]int32
	qpPer := qp / 6
	qpRem := qp % 6
	levelScale := levelScale4x4[qpRem][0] * weightScaleDC
	if qpPer >= 6 {
		for i := range 16 {
			result[i] = coeffs[i] * levelScale << uint(qpPer-6)
		}
	} else {
		for i := range 16 {
			result[i] = (coeffs[i]*levelScale + (1 << uint(5-qpPer))) >> uint(6-qpPer)
		}
	}
	return result
}

func dequantChromaDC2x2(coeffs [4]int32, qpc int, weightScaleDC int32) [4]int32 {
	var result [4]int32
	qpPer := qpc / 6
	qpRem := qpc % 6
	levelScale := levelScale4x4[qpRem][0] * weightScaleDC
	if qpPer >= 5 {
		for i := range 4 {
			result[i] = coeffs[i] * levelScale << uint(qpPer-5)
		}
	} else {
		for i := range 4 {
			result[i] = (coeffs[i] * levelScale) >> uint(5-qpPer)
		}
	}
	return result
}

func inverseHadamard4x4(coeffs [16]int32) [16]int32 {
	var temp [16]int32
	for i := range 4 {
		s0, s1, s2, s3 := coeffs[i*4], coeffs[i*4+1], coeffs[i*4+2], coeffs[i*4+3]
		temp[i*4+0] = s0 + s1 + s2 + s3
		temp[i*4+1] = s0 + s1 - s2 - s3
		temp[i*4+2] = s0 - s1 - s2 + s3
		temp[i*4+3] = s0 - s1 + s2 - s3
	}
	var result [16]int32
	for j := range 4 {
		f0, f1, f2, f3 := temp[j], temp[4+j], temp[8+j], temp[12+j]
		result[j] = f0 + f1 + f2 + f3
		result[4+j] = f0 + f1 - f2 - f3
		result[8+j] = f0 - f1 - f2 + f3
		result[12+j] = f0 - f1 + f2 - f3
	}
	return result
}

func inverseHadamard2x2(coeffs [4]int32) [4]int32 {
	return [4]int32{
		coeffs[0] + coeffs[1] + coeffs[2] + coeffs[3],
		coeffs[0] - coeffs[1] + coeffs[2] - coeffs[3],
		coeffs[0] + coeffs[1] - coeffs[2] - coeffs[3],
		coeffs[0] - coeffs[1] - coeffs[2] + coeffs[3],
	}
}

var levelScale4x4 = [6][3]int32{
	{10, 13, 16},
	{11, 14, 18},
	{13, 16, 20},
	{14, 18, 23},
	{16, 20, 25},
	{18, 23, 29},
}

// inverseRasterX4x4 maps 4x4 block index (0-15) to x position within MB.
var inverseRasterX4x4 = [16]int{
	0, 4, 0, 4, 8, 12, 8, 12,
	0, 4, 0, 4, 8, 12, 8, 12,
}

// inverseRasterY4x4 maps 4x4 block index (0-15) to y position within MB.
var inverseRasterY4x4 = [16]int{
	0, 0, 4, 4, 0, 0, 4, 4,
	8, 8, 12, 12, 8, 8, 12, 12,
}
