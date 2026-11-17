package encode

// planeMBAnalysis holds the work needed to encode one PlaneGrid macroblock, a
// 16x16 region of luma (brightness) pixels with corresponding Cb and Cr (color) pixels.
//
// The encoder predicts the region from already reconstructed pixels above and to
// the left. It transforms and quantizes the difference between the prediction and
// the source: quantization reduces coefficient precision to control compression.
// The result contains the chosen prediction modes, quantized coefficients,
// coded-block flags, and reconstructed pixels. Reconstruction applies the
// quantized difference to the prediction, so later blocks use the same pixels
// a decoder will see.
//
// The result does not contain encoded bytes or entropy-coder state. Encoding the
// coefficients still depends on the current macroblock's neighbors.
type planeMBAnalysis struct {
	lumaMode, chromaMode int
	lumaCBP, chromaCBP   int
	quantDC              [16]int32
	quantAC              [16][15]int32
	quantCbDC, quantCrDC [4]int32
	reconY               [256]uint8
	reconCb, reconCr     [64]uint8
}

// analyzeMBPlane overwrites result using the source quadrants and reconstructed neighbors.
// It selects luma and chroma prediction modes, quantizes their residuals, and reconstructs
// pixels for subsequent macroblocks. Entropy coding is performed separately.
func (e *FrameEncoder) analyzeMBPlane(result *planeMBAnalysis,
	mbX, mbY int, lumaVals, cbVals, crVals [4]uint8,
	reconY []uint8, strideY int, reconCb, reconCr []uint8, strideC int) {

	lumaMode, lumaPred := selectLumaModePlane(reconY, strideY, mbX, mbY, lumaVals)
	result.lumaMode = lumaMode
	quantizeLumaPlane(e.QP, lumaVals, &lumaPred, result)
	result.reconY = reconstructLumaPixel(result.quantDC, result.quantAC, lumaPred, e.QP, result.lumaCBP)

	chromaMode, cbPred, crPred := selectChromaModePlane(
		reconCb, reconCr, strideC, mbX, mbY, cbVals, crVals)
	result.chromaMode = chromaMode
	qpc := ChromaQP(e.QP)
	result.quantCbDC = quantizeChromaPlane(qpc, cbVals, cbPred)
	result.quantCrDC = quantizeChromaPlane(qpc, crVals, crPred)
	result.chromaCBP = 0
	for i := range 4 {
		if result.quantCbDC[i] != 0 || result.quantCrDC[i] != 0 {
			result.chromaCBP = 1
			break
		}
	}
	reconstructChromaPixel(result.quantCbDC, cbPred, qpc, result.reconCb[:], 8, 0, 0)
	reconstructChromaPixel(result.quantCrDC, crPred, qpc, result.reconCr[:], 8, 0, 0)
}

// transformLumaPlane transforms each 4x4 residual into raster-ordered DC and
// block-ordered AC coefficients. dc and ac must be zeroed: zero residuals are skipped.
func transformLumaPlane(values [4]uint8, prediction *[256]uint8,
	dc *[16]int32, ac *[16][15]int32) {
	for blk := range 16 {
		bx, by := inverseRasterX4x4[blk], inverseRasterY4x4[blk]
		var residual [16]int32
		for row := range 4 {
			for col := range 4 {
				y, x := by+row, bx+col
				value := values[(y/8)*2+x/8]
				residual[row*4+col] = int32(value) - int32(prediction[y*16+x])
			}
		}
		if residual == [16]int32{} {
			continue
		}
		transformed := ForwardTransform4x4(residual)
		dc[scan2raster[blk]] = transformed[0]
		copy(ac[blk][:], transformed[1:])
	}
}

// quantizeLumaPlane applies the DC Hadamard transform and quantizes nonzero AC blocks.
// It clears prior AC coefficients and the coded-block flag because result can be reused.
func quantizeLumaPlane(qp int, values [4]uint8, prediction *[256]uint8, result *planeMBAnalysis) {

	var dc [16]int32
	var ac [16][15]int32
	transformLumaPlane(values, prediction, &dc, &ac)
	result.quantDC = QuantizeDC4x4(ForwardHadamard4x4(dc), qp, 16)
	result.quantAC = [16][15]int32{}
	result.lumaCBP = 0
	for blk := range 16 {
		if ac[blk] == [15]int32{} {
			continue
		}
		var block [16]int32
		copy(block[1:], ac[blk][:])
		quantized := Quantize4x4(block, qp)
		copy(result.quantAC[blk][:], quantized[1:])
		for _, value := range result.quantAC[blk] {
			if value != 0 {
				result.lumaCBP = 1
				break
			}
		}
	}
}

// quantizeChromaPlane quantizes the Hadamard-transformed DC residuals of four 4x4
// chroma sub-blocks. This PlaneGrid path does not encode chroma AC residuals.
func quantizeChromaPlane(qpc int, values [4]uint8, prediction [64]uint8) [4]int32 {
	var dc [4]int32
	for i := range 4 {
		dc[i] = chromaSubBlockDC(values[i], prediction, (i%2)*4, (i/2)*4)
	}
	return QuantizeChromaDC2x2(ForwardHadamard2x2(dc), qpc)
}
