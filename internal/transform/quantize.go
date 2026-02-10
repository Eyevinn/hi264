// Package transform implements inverse quantization and inverse transform
// for H.264/AVC decoding as specified in section 8.5 of the standard.
package transform

// LevelScale4x4 is the dequantization scaling factor table for 4x4 blocks.
// Indexed by QP%6 and position (i,j)%3 mapping.
// From Table 8-13 of the standard.
var LevelScale4x4 = [6][3]int32{
	{10, 13, 16},
	{11, 14, 18},
	{13, 16, 20},
	{14, 18, 23},
	{16, 20, 25},
	{18, 23, 29},
}

// LevelScale8x8 contains dequant scale factors for 8x8 (Table 8-14).
// Indexed by QP%6 and scan position mapping.
var LevelScale8x8 = [6][6]int32{
	{20, 18, 32, 19, 25, 24},
	{22, 19, 35, 21, 28, 26},
	{26, 23, 42, 24, 33, 31},
	{28, 25, 45, 26, 35, 33},
	{32, 28, 51, 30, 40, 38},
	{36, 32, 58, 34, 46, 43},
}

// DefaultScalingList4x4 is the flat default 4x4 scaling matrix (all 16s).
var DefaultScalingList4x4 = [16]int32{
	16, 16, 16, 16,
	16, 16, 16, 16,
	16, 16, 16, 16,
	16, 16, 16, 16,
}

// DefaultScalingList8x8 is the flat default 8x8 scaling matrix (all 16s).
// Used when seq_scaling_matrix_present_flag = 0 (no custom scaling lists in SPS).
var DefaultScalingList8x8 = [64]int32{
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
	16, 16, 16, 16, 16, 16, 16, 16,
}

// SpecDefaultScalingList8x8Intra is the spec's non-flat default 8x8 intra scaling matrix.
// Used when seq_scaling_matrix_present_flag = 1 but individual list uses default.
var SpecDefaultScalingList8x8Intra = [64]int32{
	6, 10, 13, 16, 18, 23, 25, 27,
	10, 11, 16, 18, 23, 25, 27, 29,
	13, 16, 18, 23, 25, 27, 29, 31,
	16, 18, 23, 25, 27, 29, 31, 33,
	18, 23, 25, 27, 29, 31, 33, 36,
	23, 25, 27, 29, 31, 33, 36, 38,
	25, 27, 29, 31, 33, 36, 38, 40,
	27, 29, 31, 33, 36, 38, 40, 42,
}

// Dequant4x4 performs inverse quantization on a 4x4 block of coefficients.
// coeffs: 16 coefficients in raster scan order.
// qp: quantization parameter.
// scalingList: 4x4 scaling list (nil = use default flat 16).
// isIntra: true for intra blocks.
func Dequant4x4(coeffs [16]int32, qp int, scalingList *[16]int32) [16]int32 {
	var result [16]int32

	qpPer := qp / 6
	qpRem := qp % 6

	sl := &DefaultScalingList4x4
	if scalingList != nil {
		sl = scalingList
	}

	for i := 0; i < 16; i++ {
		if coeffs[i] == 0 {
			continue
		}
		// Position mapping for LevelScale
		row := i / 4
		col := i % 4
		v := levelScaleIdx(row, col)

		if qpPer >= 4 {
			result[i] = coeffs[i] * LevelScale4x4[qpRem][v] * int32(sl[i]) << uint(qpPer-4)
		} else {
			result[i] = (coeffs[i]*LevelScale4x4[qpRem][v]*int32(sl[i]) + (1 << uint(3-qpPer))) >> uint(4-qpPer)
		}
	}

	return result
}

// levelScaleIdx maps a (row, col) position to the LevelScale index.
// The mapping depends on (i%2, j%2): (0,0)->0, (0,1)or(1,0)->1, (1,1)->2.
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

// Dequant8x8 performs inverse quantization on an 8x8 block.
func Dequant8x8(coeffs [64]int32, qp int, scalingList *[64]int32) [64]int32 {
	var result [64]int32

	qpPer := qp / 6
	qpRem := qp % 6

	sl := &DefaultScalingList8x8
	if scalingList != nil {
		sl = scalingList
	}

	for i := 0; i < 64; i++ {
		if coeffs[i] == 0 {
			continue
		}
		row := i / 8
		col := i % 8
		v := levelScale8x8Idx(row, col)

		if qpPer >= 6 {
			result[i] = coeffs[i] * LevelScale8x8[qpRem][v] * int32(sl[i]) << uint(qpPer-6)
		} else {
			result[i] = (coeffs[i]*LevelScale8x8[qpRem][v]*int32(sl[i]) + (1 << uint(5-qpPer))) >> uint(6-qpPer)
		}
	}

	return result
}

// levelScale8x8Idx maps (row, col) to the 8x8 LevelScale index.
// Uses a 4x4 repeating pattern matching Table 8-14 of the spec.
// Pattern (indexed by row%4, col%4):
//
//	v0  v3  v4  v3
//	v3  v1  v5  v1
//	v4  v5  v2  v5
//	v3  v1  v5  v1
var normAdjust8x8 = [4][4]int{
	{0, 3, 4, 3},
	{3, 1, 5, 1},
	{4, 5, 2, 5},
	{3, 1, 5, 1},
}

func levelScale8x8Idx(row, col int) int {
	return normAdjust8x8[row%4][col%4]
}

// DequantDC4x4 performs inverse quantization on 16x16 intra DC coefficients.
// The DC coefficients get a special scaling after the 4x4 Hadamard transform.
// weightScaleDC is the scaling list value at position (0,0), typically 16 for flat default.
func DequantDC4x4(coeffs [16]int32, qp int, weightScaleDC int32) [16]int32 {
	var result [16]int32

	qpPer := qp / 6
	qpRem := qp % 6
	levelScale := LevelScale4x4[qpRem][0] * weightScaleDC

	if qpPer >= 6 {
		for i := 0; i < 16; i++ {
			result[i] = coeffs[i] * levelScale << uint(qpPer-6)
		}
	} else {
		for i := 0; i < 16; i++ {
			result[i] = (coeffs[i]*levelScale + (1 << uint(5-qpPer))) >> uint(6-qpPer)
		}
	}

	return result
}

// DequantChromaDC2x2 performs inverse quantization on chroma DC coefficients (4:2:0).
// weightScaleDC is the chroma scaling list value at position (0,0), typically 16 for flat default.
func DequantChromaDC2x2(coeffs [4]int32, qpc int, weightScaleDC int32) [4]int32 {
	var result [4]int32

	qpPer := qpc / 6
	qpRem := qpc % 6
	levelScale := LevelScale4x4[qpRem][0] * weightScaleDC

	if qpPer >= 5 {
		for i := 0; i < 4; i++ {
			result[i] = coeffs[i] * levelScale << uint(qpPer-5)
		}
	} else {
		// No rounding bias — matches FFmpeg's (c * qmul) >> 7 truncation.
		for i := 0; i < 4; i++ {
			result[i] = (coeffs[i] * levelScale) >> uint(5-qpPer)
		}
	}

	return result
}
