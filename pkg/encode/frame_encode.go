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
	Plane           *yuv.PlaneGrid // alternative to Grid+Colors; takes priority if set
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

// plane returns the PlaneGrid to use for encoding, converting Grid+Colors if needed.
func (e *FrameEncoder) plane() (*yuv.PlaneGrid, error) {
	if e.Plane != nil {
		return e.Plane, nil
	}
	return yuv.GridToPlaneGrid(e.Grid, e.Colors)
}

// frameDimensions returns the frame width and height in pixels.
func (e *FrameEncoder) frameDimensions() (width, height int) {
	if e.Plane != nil {
		width = e.Plane.PixelWidth()
		height = e.Plane.PixelHeight()
	} else {
		width = e.Grid.Width * 16
		height = e.Grid.Height * 16
	}
	if e.Width > 0 {
		width = e.Width
	}
	if e.Height > 0 {
		height = e.Height
	}
	return width, height
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
	width, height := e.frameDimensions()

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
	width, height := e.frameDimensions()
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
	// FrameEncoder controls both ends of its stream (IDR resets POC each
	// time), so picOrderCntLsb = 2*frame_num is correct here.
	return EncodePSkipSlice(sps, pps, frameNum, frameNum*2, e.DisableDeblock)
}

func (e *FrameEncoder) encodeSliceCAVLC(idrPicID uint32) ([]byte, error) {
	pg, err := e.plane()
	if err != nil {
		return nil, err
	}
	mbWidth := pg.MBWidth()
	mbHeight := pg.MBHeight()
	width := mbWidth * 16
	height := mbHeight * 16

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
			lumaVals := pg.MBLumaValues(mbX, mbY)
			cbVals, crVals := pg.MBChromaSub(mbX, mbY)

			err := e.encodeMBPlane(sliceW, mbX, mbY, lumaVals, cbVals, crVals,
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
	if err := WriteNALU(&buf, 5, 3, sliceRBSP); err != nil {
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
	pg, err := e.plane()
	if err != nil {
		return nil, err
	}
	mbWidth := pg.MBWidth()
	mbHeight := pg.MBHeight()
	width := mbWidth * 16
	height := mbHeight * 16

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
			lumaVals := pg.MBLumaValues(mbX, mbY)
			cbVals, crVals := pg.MBChromaSub(mbX, mbY)

			mbIdx := mbY*mbWidth + mbX
			err := e.encodeMBCABACPlane(enc, ctx, mbStates, mbIdx, mbX, mbY,
				lumaVals, cbVals, crVals,
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
	if err := WriteNALU(&buf, 5, 3, sliceRBSP); err != nil {
		return nil, fmt.Errorf("write IDR: %w", err)
	}

	return buf.Bytes(), nil
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

// lumaPredict16x16 computes the per-pixel I_16x16 prediction array for the given mode.
// This matches the decoder's prediction: V mode uses the full top row, H mode uses
// the full left column, DC mode uses a uniform value.
func lumaPredict16x16(reconY []uint8, strideY, mbX, mbY, mode int) [256]uint8 {
	var pred [256]uint8
	switch mode {
	case 0: // Vertical — each column gets top row pixel
		off := (mbY*16-1)*strideY + mbX*16
		for x := range 16 {
			topVal := reconY[off+x]
			for y := range 16 {
				pred[y*16+x] = topVal
			}
		}
	case 1: // Horizontal — each row gets left column pixel
		for y := range 16 {
			leftVal := reconY[(mbY*16+y)*strideY+mbX*16-1]
			for x := range 16 {
				pred[y*16+x] = leftVal
			}
		}
	case 2: // DC — uniform value
		dcVal := computeLumaDCPred(reconY, strideY, mbX, mbY)
		for i := range pred {
			pred[i] = dcVal
		}
	}
	return pred
}

// chromaPredict8x8 computes the per-pixel chroma prediction array for the given mode.
// This matches the decoder's chroma prediction: DC gives per-sub-block averages,
// V copies the top row, H copies the left column.
func chromaPredict8x8(recon []uint8, strideC, mbX, mbY, mode int) [64]uint8 {
	var pred [64]uint8
	switch mode {
	case 0: // DC — per-sub-block DC prediction
		dcPreds := computeChromaDCPreds4x4(recon, strideC, mbX, mbY)
		for blk := range 4 {
			x0 := (blk % 2) * 4
			y0 := (blk / 2) * 4
			for y := range 4 {
				for x := range 4 {
					pred[(y0+y)*8+x0+x] = dcPreds[blk]
				}
			}
		}
	case 1: // Horizontal — each row gets left column pixel
		for y := range 8 {
			leftVal := recon[(mbY*8+y)*strideC+mbX*8-1]
			for x := range 8 {
				pred[y*8+x] = leftVal
			}
		}
	case 2: // Vertical — each column gets top row pixel
		off := (mbY*8-1)*strideC + mbX*8
		for x := range 8 {
			topVal := recon[off+x]
			for y := range 8 {
				pred[y*8+x] = topVal
			}
		}
	}
	return pred
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
