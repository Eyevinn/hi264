package transform

// InverseTransform4x4 performs the 4x4 integer inverse transform (section 8.5.12).
// Input: 16 dequantized coefficients in raster order (row*4+col).
// Output: 16 residual samples in raster order.
// Per spec: row transform first, then column transform.
func InverseTransform4x4(coeffs [16]int32) [16]int32 {
	var block [16]int32
	copy(block[:], coeffs[:])

	// Add rounding to DC before transform (equivalent to adding 32 at each output)
	block[0] += 32

	// 1D transform on rows: for each row i, process columns 0-3
	for i := range 4 {
		z0 := block[i*4+0] + block[i*4+2]
		z1 := block[i*4+0] - block[i*4+2]
		z2 := (block[i*4+1] >> 1) - block[i*4+3]
		z3 := block[i*4+1] + (block[i*4+3] >> 1)

		block[i*4+0] = z0 + z3
		block[i*4+1] = z1 + z2
		block[i*4+2] = z1 - z2
		block[i*4+3] = z0 - z3
	}

	// 1D transform on columns
	var result [16]int32
	for j := range 4 {
		z0 := block[0*4+j] + block[2*4+j]
		z1 := block[0*4+j] - block[2*4+j]
		z2 := (block[1*4+j] >> 1) - block[3*4+j]
		z3 := block[1*4+j] + (block[3*4+j] >> 1)

		result[0*4+j] = (z0 + z3) >> 6
		result[1*4+j] = (z1 + z2) >> 6
		result[2*4+j] = (z1 - z2) >> 6
		result[3*4+j] = (z0 - z3) >> 6
	}

	return result
}

// InverseHadamard4x4 performs the 4x4 inverse Hadamard transform
// for Intra16x16 luma DC coefficients (section 8.5.10).
func InverseHadamard4x4(coeffs [16]int32) [16]int32 {
	var temp [16]int32

	// 1D Hadamard on rows
	for i := range 4 {
		s0 := coeffs[i*4+0]
		s1 := coeffs[i*4+1]
		s2 := coeffs[i*4+2]
		s3 := coeffs[i*4+3]

		temp[i*4+0] = s0 + s1 + s2 + s3
		temp[i*4+1] = s0 + s1 - s2 - s3
		temp[i*4+2] = s0 - s1 - s2 + s3
		temp[i*4+3] = s0 - s1 + s2 - s3
	}

	// 1D Hadamard on columns
	var result [16]int32
	for j := range 4 {
		f0 := temp[0*4+j]
		f1 := temp[1*4+j]
		f2 := temp[2*4+j]
		f3 := temp[3*4+j]

		result[0*4+j] = f0 + f1 + f2 + f3
		result[1*4+j] = f0 + f1 - f2 - f3
		result[2*4+j] = f0 - f1 - f2 + f3
		result[3*4+j] = f0 - f1 + f2 - f3
	}

	return result
}

// InverseHadamard2x2 performs the 2x2 inverse Hadamard transform
// for chroma DC coefficients in 4:2:0 (section 8.5.11).
func InverseHadamard2x2(coeffs [4]int32) [4]int32 {
	var result [4]int32

	result[0] = coeffs[0] + coeffs[1] + coeffs[2] + coeffs[3]
	result[1] = coeffs[0] - coeffs[1] + coeffs[2] - coeffs[3]
	result[2] = coeffs[0] + coeffs[1] - coeffs[2] - coeffs[3]
	result[3] = coeffs[0] - coeffs[1] - coeffs[2] + coeffs[3]

	return result
}

// InverseTransform8x8 performs the 8x8 integer inverse transform (section 8.5.13).
func InverseTransform8x8(coeffs [64]int32) [64]int32 {
	var temp [64]int32

	// 1D transform on rows
	for i := range 8 {
		a0 := coeffs[i*8+0]
		a1 := coeffs[i*8+1]
		a2 := coeffs[i*8+2]
		a3 := coeffs[i*8+3]
		a4 := coeffs[i*8+4]
		a5 := coeffs[i*8+5]
		a6 := coeffs[i*8+6]
		a7 := coeffs[i*8+7]

		// 8-point butterfly
		e0 := a0 + a4
		e1 := -a3 + a5 - a7 - (a7 >> 1)
		e2 := a0 - a4
		e3 := a1 + a7 - a3 - (a3 >> 1)
		e4 := (a2 >> 1) - a6
		e5 := -a1 + a7 + a5 + (a5 >> 1)
		e6 := a2 + (a6 >> 1)
		e7 := a3 + a5 + a1 + (a1 >> 1)

		f0 := e0 + e6
		f1 := e1 + (e7 >> 2)
		f2 := e2 + e4
		f3 := e3 + (e5 >> 2)
		f4 := e2 - e4
		f5 := (e3 >> 2) - e5
		f6 := e0 - e6
		f7 := e7 - (e1 >> 2)

		temp[i*8+0] = f0 + f7
		temp[i*8+1] = f2 + f5
		temp[i*8+2] = f4 + f3
		temp[i*8+3] = f6 + f1
		temp[i*8+4] = f6 - f1
		temp[i*8+5] = f4 - f3
		temp[i*8+6] = f2 - f5
		temp[i*8+7] = f0 - f7
	}

	// 1D transform on columns
	var result [64]int32
	for j := range 8 {
		a0 := temp[0*8+j]
		a1 := temp[1*8+j]
		a2 := temp[2*8+j]
		a3 := temp[3*8+j]
		a4 := temp[4*8+j]
		a5 := temp[5*8+j]
		a6 := temp[6*8+j]
		a7 := temp[7*8+j]

		e0 := a0 + a4
		e1 := -a3 + a5 - a7 - (a7 >> 1)
		e2 := a0 - a4
		e3 := a1 + a7 - a3 - (a3 >> 1)
		e4 := (a2 >> 1) - a6
		e5 := -a1 + a7 + a5 + (a5 >> 1)
		e6 := a2 + (a6 >> 1)
		e7 := a3 + a5 + a1 + (a1 >> 1)

		f0 := e0 + e6
		f1 := e1 + (e7 >> 2)
		f2 := e2 + e4
		f3 := e3 + (e5 >> 2)
		f4 := e2 - e4
		f5 := (e3 >> 2) - e5
		f6 := e0 - e6
		f7 := e7 - (e1 >> 2)

		// Add rounding and right shift by 6
		result[0*8+j] = (f0 + f7 + 32) >> 6
		result[1*8+j] = (f2 + f5 + 32) >> 6
		result[2*8+j] = (f4 + f3 + 32) >> 6
		result[3*8+j] = (f6 + f1 + 32) >> 6
		result[4*8+j] = (f6 - f1 + 32) >> 6
		result[5*8+j] = (f4 - f3 + 32) >> 6
		result[6*8+j] = (f2 - f5 + 32) >> 6
		result[7*8+j] = (f0 - f7 + 32) >> 6
	}

	return result
}

// Clip clips a value to the range [0, max].
func Clip(val, max int32) int32 {
	if val < 0 {
		return 0
	}
	if val > max {
		return max
	}
	return val
}
