package encode

import "github.com/Eyevinn/hi264/internal/transform"

// ForwardHadamard4x4 performs the forward 4x4 Hadamard transform for luma DC.
// The Hadamard transform is self-inverse (up to scaling), so we use the same
// butterfly structure as the inverse.
func ForwardHadamard4x4(dc [16]int32) [16]int32 {
	var temp [16]int32

	// 1D Hadamard on rows
	for i := range 4 {
		s0 := dc[i*4+0]
		s1 := dc[i*4+1]
		s2 := dc[i*4+2]
		s3 := dc[i*4+3]

		temp[i*4+0] = s0 + s1 + s2 + s3
		temp[i*4+1] = s0 + s1 - s2 - s3
		temp[i*4+2] = s0 - s1 - s2 + s3
		temp[i*4+3] = s0 - s1 + s2 - s3
	}

	// 1D Hadamard on columns, with /2 normalization
	var result [16]int32
	for j := range 4 {
		f0 := temp[0*4+j]
		f1 := temp[1*4+j]
		f2 := temp[2*4+j]
		f3 := temp[3*4+j]

		result[0*4+j] = (f0 + f1 + f2 + f3) / 2
		result[1*4+j] = (f0 + f1 - f2 - f3) / 2
		result[2*4+j] = (f0 - f1 - f2 + f3) / 2
		result[3*4+j] = (f0 - f1 + f2 - f3) / 2
	}

	return result
}

// ForwardHadamard2x2 performs the forward 2x2 Hadamard transform for chroma DC.
// Self-inverse (up to scaling).
func ForwardHadamard2x2(dc [4]int32) [4]int32 {
	return [4]int32{
		dc[0] + dc[1] + dc[2] + dc[3],
		dc[0] - dc[1] + dc[2] - dc[3],
		dc[0] + dc[1] - dc[2] - dc[3],
		dc[0] - dc[1] - dc[2] + dc[3],
	}
}

// MF4x4 is the forward quantization multiplication factor table.
// Indexed by QP%6 and position category (same as dequant LevelScale).
// These are the standard forward quantization factors from JM reference.
var MF4x4 = [6][3]int32{
	{13107, 8066, 5243},
	{11916, 7490, 4660},
	{10082, 6554, 4194},
	{9362, 5825, 3647},
	{8192, 5243, 3355},
	{7282, 4559, 2893},
}

// Quantize4x4 performs forward quantization on a 4x4 block.
// Returns quantized coefficients (levels).
func Quantize4x4(coeffs [16]int32, qp int) [16]int32 {
	var result [16]int32

	qpPer := qp / 6
	qpRem := qp % 6
	qBits := 15 + qpPer
	add := int32(1) << uint(qBits) / 3 // rounding for intra

	for i := range 16 {
		row := i / 4
		col := i % 4
		v := levelScaleIdx(row, col)
		mf := MF4x4[qpRem][v]

		sign := int32(1)
		c := coeffs[i]
		if c < 0 {
			sign = -1
			c = -c
		}
		result[i] = sign * ((c*mf + add) >> uint(qBits))
	}
	return result
}

// QuantizeDC4x4 performs forward quantization on 16x16 luma DC coefficients.
// wsDC is the scaling list DC value (typically 16 for flat default).
func QuantizeDC4x4(coeffs [16]int32, qp int, wsDC int32) [16]int32 {
	var result [16]int32

	qpPer := qp / 6
	qpRem := qp % 6
	mf := MF4x4[qpRem][0]
	// Adjust mf for scaling list: mf = mf * 16 / wsDC
	if wsDC != 16 {
		mf = mf * 16 / wsDC
	}
	qBits := 15 + qpPer + 1 // extra +1 for DC after Hadamard normalization
	add := int32(1) << uint(qBits) / 3

	for i := range 16 {
		sign := int32(1)
		c := coeffs[i]
		if c < 0 {
			sign = -1
			c = -c
		}
		result[i] = sign * ((c*mf + add) >> uint(qBits))
	}
	return result
}

// QuantizeChromaDC2x2 performs forward quantization on chroma DC coefficients (4:2:0).
func QuantizeChromaDC2x2(coeffs [4]int32, qpc int) [4]int32 {
	var result [4]int32

	qpPer := qpc / 6
	qpRem := qpc % 6
	mf := MF4x4[qpRem][0]
	qBits := 15 + qpPer + 1
	add := int32(1) << uint(qBits) / 3

	for i := range 4 {
		sign := int32(1)
		c := coeffs[i]
		if c < 0 {
			sign = -1
			c = -c
		}
		result[i] = sign * ((c*mf + add) >> uint(qBits))
	}
	return result
}

// levelScaleIdx maps (row, col) to LevelScale/MF index.
func levelScaleIdx(row, col int) int {
	r := row % 2
	c := col % 2
	if r == 0 && c == 0 {
		return 0
	}
	if r == 1 && c == 1 {
		return 2
	}
	return 1
}

// ChromaQP maps luma QP to chroma QP (Table 8-15).
func ChromaQP(qpY int) int {
	if qpY < 0 {
		qpY = 0
	}
	if qpY < 30 {
		return qpY
	}
	if qpY > 51 {
		return 51
	}
	qpcTable := []int{
		29, 30, 31, 32, 32, 33, 34, 34,
		35, 35, 36, 36, 37, 37, 37, 38,
		38, 38, 39, 39, 39, 39,
	}
	return qpcTable[qpY-30]
}

// ForwardTransformDC4x4Const computes the DC coefficient of a 4x4 block
// from a constant residual value. For a constant block with value R:
// DC = 16*R (forward 4x4 DCT: row sum 4R, column sum 4*4R = 16R).
func ForwardTransformDC4x4Const(residual int32) int32 {
	return 16 * residual
}

// ForwardTransformDC4x4 is an alias for ForwardTransformDC4x4Const for backward compatibility.
func ForwardTransformDC4x4(residual int32) int32 {
	return ForwardTransformDC4x4Const(residual)
}

// ForwardTransform4x4 performs the forward 4x4 integer DCT.
// Input: 16 residual samples in raster order (row*4+col).
// Output: 16 transform coefficients in raster order.
// This is the inverse of InverseTransform4x4 in internal/transform.
func ForwardTransform4x4(block [16]int32) [16]int32 {
	var temp [16]int32

	// 1D transform on rows
	for i := range 4 {
		s0 := block[i*4+0]
		s1 := block[i*4+1]
		s2 := block[i*4+2]
		s3 := block[i*4+3]

		p0 := s0 + s3
		p1 := s1 + s2
		p2 := s1 - s2
		p3 := s0 - s3

		temp[i*4+0] = p0 + p1
		temp[i*4+1] = p2 + (p3 << 1)
		temp[i*4+2] = p0 - p1
		temp[i*4+3] = p3 - (p2 << 1)
	}

	// 1D transform on columns
	var result [16]int32
	for j := range 4 {
		s0 := temp[0*4+j]
		s1 := temp[1*4+j]
		s2 := temp[2*4+j]
		s3 := temp[3*4+j]

		p0 := s0 + s3
		p1 := s1 + s2
		p2 := s1 - s2
		p3 := s0 - s3

		result[0*4+j] = p0 + p1
		result[1*4+j] = p2 + (p3 << 1)
		result[2*4+j] = p0 - p1
		result[3*4+j] = p3 - (p2 << 1)
	}

	return result
}

// ForwardTransformDC4x4Block computes the full 4x4 DCT coefficients for a block
// made of at most 4 constant-value quadrants. The block is composed of:
//
//	vals[0] vals[1]   (top-left 2x2, top-right 2x2)
//	vals[2] vals[3]   (bottom-left 2x2, bottom-right 2x2)
//
// This avoids building a 16-element pixel array by computing the transform
// analytically from the 4 quadrant values.
func ForwardTransformDC4x4Block(vals [4]int32) [16]int32 {
	// Build the 4x4 residual block from the quadrant values
	var block [16]int32
	for r := range 4 {
		for c := range 4 {
			qr := r / 2 // 0 for rows 0-1, 1 for rows 2-3
			qc := c / 2 // 0 for cols 0-1, 1 for cols 2-3
			block[r*4+c] = vals[qr*2+qc]
		}
	}
	return ForwardTransform4x4(block)
}

// ForwardDequantRoundTrip verifies that forward quant + dequant of a DC value
// round-trips correctly. Used only for testing.
func ForwardDequantRoundTrip(dc int32, qp int) (quantized, dequantized int32) {
	// Forward Hadamard of 16 identical DC values
	var dcMatrix [16]int32
	for i := range dcMatrix {
		dcMatrix[i] = dc
	}
	hadamard := ForwardHadamard4x4(dcMatrix)

	// Forward quantize
	quantizedMatrix := QuantizeDC4x4(hadamard, qp, 16)

	// Inverse: dequant then inverse Hadamard
	dequantMatrix := transform.DequantDC4x4(quantizedMatrix, qp, 16)
	invHadamard := transform.InverseHadamard4x4(dequantMatrix)

	return quantizedMatrix[0], invHadamard[0]
}
