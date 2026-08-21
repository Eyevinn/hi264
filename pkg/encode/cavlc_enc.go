package encode

// CAVLC residual block encoding, using the VLC tables from pkg/cavlc/tables.go.
// This file directly embeds the needed tables to avoid circular dependencies.

// coeff_token VLC tables (Table 9-5), copied from cavlc/tables.go
var encCoeffTokenLen = [4][4 * 17]uint8{
	{
		1, 0, 0, 0,
		6, 2, 0, 0, 8, 6, 3, 0, 9, 8, 7, 5, 10, 9, 8, 6,
		11, 10, 9, 7, 13, 11, 10, 8, 13, 13, 11, 9, 13, 13, 13, 10,
		14, 14, 13, 11, 14, 14, 14, 13, 15, 15, 14, 14, 15, 15, 15, 14,
		16, 15, 15, 15, 16, 16, 16, 15, 16, 16, 16, 16, 16, 16, 16, 16,
	},
	{
		2, 0, 0, 0,
		6, 2, 0, 0, 6, 5, 3, 0, 7, 6, 6, 4, 8, 6, 6, 4,
		8, 7, 7, 5, 9, 8, 8, 6, 11, 9, 9, 6, 11, 11, 11, 7,
		12, 11, 11, 9, 12, 12, 12, 11, 12, 12, 12, 11, 13, 13, 13, 12,
		13, 13, 13, 13, 13, 14, 13, 13, 14, 14, 14, 13, 14, 14, 14, 14,
	},
	{
		4, 0, 0, 0,
		6, 4, 0, 0, 6, 5, 4, 0, 6, 5, 5, 4, 7, 5, 5, 4,
		7, 5, 5, 4, 7, 6, 6, 4, 7, 6, 6, 4, 8, 7, 7, 5,
		8, 8, 7, 6, 9, 8, 8, 7, 9, 9, 8, 8, 9, 9, 9, 8,
		10, 9, 9, 9, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10, 10,
	},
	{
		6, 0, 0, 0,
		6, 6, 0, 0, 6, 6, 6, 0, 6, 6, 6, 6, 6, 6, 6, 6,
		6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6,
		6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6,
		6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6,
	},
}

var encCoeffTokenBits = [4][4 * 17]uint8{
	{
		1, 0, 0, 0,
		5, 1, 0, 0, 7, 4, 1, 0, 7, 6, 5, 3, 7, 6, 5, 3,
		7, 6, 5, 4, 15, 6, 5, 4, 11, 14, 5, 4, 8, 10, 13, 4,
		15, 14, 9, 4, 11, 10, 13, 12, 15, 14, 9, 12, 11, 10, 13, 8,
		15, 1, 9, 12, 11, 14, 13, 8, 7, 10, 9, 12, 4, 6, 5, 8,
	},
	{
		3, 0, 0, 0,
		11, 2, 0, 0, 7, 7, 3, 0, 7, 10, 9, 5, 7, 6, 5, 4,
		4, 6, 5, 6, 7, 6, 5, 8, 15, 6, 5, 4, 11, 14, 13, 4,
		15, 10, 9, 4, 11, 14, 13, 12, 8, 10, 9, 8, 15, 14, 13, 12,
		11, 10, 9, 12, 7, 11, 6, 8, 9, 8, 10, 1, 7, 6, 5, 4,
	},
	{
		15, 0, 0, 0,
		15, 14, 0, 0, 11, 15, 13, 0, 8, 12, 14, 12, 15, 10, 11, 11,
		11, 8, 9, 10, 9, 14, 13, 9, 8, 10, 9, 8, 15, 14, 13, 13,
		11, 14, 10, 12, 15, 10, 13, 12, 11, 14, 9, 12, 8, 10, 13, 8,
		13, 7, 9, 12, 9, 12, 11, 10, 5, 8, 7, 6, 1, 4, 3, 2,
	},
	{
		3, 0, 0, 0,
		0, 1, 0, 0, 4, 5, 6, 0, 8, 9, 10, 11, 12, 13, 14, 15,
		16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31,
		32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47,
		48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63,
	},
}

// Chroma DC coeff_token VLC (Table 9-5, nC = -1)
var encChromaDCCoeffTokenLen = [4 * 5]uint8{
	2, 0, 0, 0,
	6, 1, 0, 0,
	6, 6, 3, 0,
	6, 7, 7, 6,
	6, 8, 8, 7,
}

var encChromaDCCoeffTokenBits = [4 * 5]uint8{
	1, 0, 0, 0,
	7, 1, 0, 0,
	4, 6, 1, 0,
	3, 3, 2, 5,
	2, 3, 2, 0,
}

// total_zeros VLC tables (Table 9-7)
var encTotalZerosLen = [16][16]uint8{
	{1, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 9},
	{3, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 6, 6, 6, 6},
	{4, 3, 3, 3, 4, 4, 3, 3, 4, 5, 5, 6, 5, 6},
	{5, 3, 4, 4, 3, 3, 3, 4, 3, 4, 5, 5, 5},
	{4, 4, 4, 3, 3, 3, 3, 3, 4, 5, 4, 5},
	{6, 5, 3, 3, 3, 3, 3, 3, 4, 3, 6},
	{6, 5, 3, 3, 3, 2, 3, 4, 3, 6},
	{6, 4, 5, 3, 2, 2, 3, 3, 6},
	{6, 6, 4, 2, 2, 3, 2, 5},
	{5, 5, 3, 2, 2, 2, 4},
	{4, 4, 3, 3, 1, 3},
	{4, 4, 2, 1, 3},
	{3, 3, 1, 2},
	{2, 2, 1},
	{1, 1},
}

var encTotalZerosBits = [16][16]uint8{
	{1, 3, 2, 3, 2, 3, 2, 3, 2, 3, 2, 3, 2, 3, 2, 1},
	{7, 6, 5, 4, 3, 5, 4, 3, 2, 3, 2, 3, 2, 1, 0},
	{5, 7, 6, 5, 4, 3, 4, 3, 2, 3, 2, 1, 1, 0},
	{3, 7, 5, 4, 6, 5, 4, 3, 3, 2, 2, 1, 0},
	{5, 4, 3, 7, 6, 5, 4, 3, 2, 1, 1, 0},
	{1, 1, 7, 6, 5, 4, 3, 2, 1, 1, 0},
	{1, 1, 5, 4, 3, 3, 2, 1, 1, 0},
	{1, 1, 1, 3, 3, 2, 2, 1, 0},
	{1, 0, 1, 3, 2, 1, 1, 1},
	{1, 0, 1, 3, 2, 1, 1},
	{0, 1, 1, 2, 1, 3},
	{0, 1, 1, 1, 1},
	{0, 1, 1, 1},
	{0, 1, 1},
	{0, 1},
}

// Chroma DC total_zeros VLC (Table 9-9)
var encChromaDCTotalZerosLen = [3][4]uint8{
	{1, 2, 3, 3},
	{1, 2, 2, 0},
	{1, 1, 0, 0},
}

var encChromaDCTotalZerosBits = [3][4]uint8{
	{1, 1, 1, 0},
	{1, 1, 0, 0},
	{1, 0, 0, 0},
}

// run_before VLC tables (Table 9-10)
var encRunBeforeLen = [7][16]uint8{
	{1, 1},
	{1, 2, 2},
	{2, 2, 2, 2},
	{2, 2, 2, 3, 3},
	{2, 2, 3, 3, 3, 3},
	{2, 3, 3, 3, 3, 3, 3},
	{3, 3, 3, 3, 3, 3, 3, 4, 5, 6, 7, 8, 9, 10, 11},
}

var encRunBeforeBits = [7][16]uint8{
	{1, 0},
	{1, 1, 0},
	{3, 2, 1, 0},
	{3, 2, 1, 1, 0},
	{3, 2, 3, 2, 1, 0},
	{3, 0, 1, 3, 2, 5, 4},
	{7, 6, 5, 4, 3, 2, 1, 1, 1, 1, 1, 1, 1, 1, 1},
}

// EncodeResidualBlock encodes a CAVLC residual block.
// coeffs: coefficients in scan order (zigzag for 4x4).
// nC: context number for coeff_token table selection (-1 for chroma DC, >=0 for others).
// maxNumCoeff: 4 (chroma DC), 15 (AC blocks), or 16 (4x4 blocks).
// Returns the number of non-zero coefficients (totalCoeff) for nC tracking.
func EncodeResidualBlock(w *BitWriter, coeffs []int32, nC, maxNumCoeff int) int {
	// Count non-zero coefficients and trailing ones
	totalCoeff := 0
	trailingOnes := 0
	var levels []int32
	var runs []int

	// Scan from last to first to find non-zero coefficients
	lastNonZero := -1
	for i := maxNumCoeff - 1; i >= 0; i-- {
		if coeffs[i] != 0 {
			lastNonZero = i
			break
		}
	}

	if lastNonZero < 0 {
		// All zeros - write coeff_token for totalCoeff=0
		writeCoeffToken(w, 0, 0, nC, maxNumCoeff)
		return 0
	}

	// Collect non-zero coefficients in reverse scan order
	for i := lastNonZero; i >= 0; i-- {
		if coeffs[i] != 0 {
			totalCoeff++
			levels = append(levels, coeffs[i])
		}
	}

	// Count trailing ones (up to 3, must be +/-1 at end of reverse-scan)
	for i := 0; i < len(levels) && i < 3; i++ {
		if levels[i] == 1 || levels[i] == -1 {
			trailingOnes++
		} else {
			break
		}
	}

	// Compute run lengths
	totalZeros := 0
	pos := lastNonZero
	coeffIdx := 0
	for i := lastNonZero; i >= 0; i-- {
		if coeffs[i] != 0 {
			if coeffIdx > 0 {
				runBefore := pos - i - 1
				runs = append(runs, runBefore)
				totalZeros += runBefore
				pos = i
			} else {
				pos = i
			}
			coeffIdx++
		}
	}
	// Last run (from last coeff to position 0)
	if totalCoeff > 1 {
		runs = append(runs, pos)
		totalZeros += pos
	} else {
		totalZeros = lastNonZero
	}

	// 1. Write coeff_token
	writeCoeffToken(w, totalCoeff, trailingOnes, nC, maxNumCoeff)

	// 2. Write trailing_ones_sign_flag for each trailing one.
	// levels is already in reverse scan order (highest scan position first),
	// which is the order the sign flags are transmitted in (section 7.3.5.3.2).
	for i := range trailingOnes {
		if levels[i] < 0 {
			w.WriteBit(1)
		} else {
			w.WriteBit(0)
		}
	}

	// 3. Write remaining levels (from index trailingOnes onward)
	suffixLength := 0
	if totalCoeff > 10 && trailingOnes < 3 {
		suffixLength = 1
	}

	for i := trailingOnes; i < totalCoeff; i++ {
		level := levels[i]
		// Adjust level code: first non-T1 level is offset when T1<3
		levelCode := int(level)
		if levelCode > 0 {
			levelCode = 2 * (levelCode - 1)
		} else {
			levelCode = -2*levelCode - 1
		}
		if i == trailingOnes && trailingOnes < 3 {
			levelCode -= 2 // first non-trailing level gets a -2 adjustment
		}

		writeLevelVLC(w, levelCode, suffixLength)

		// Update suffixLength
		if suffixLength == 0 {
			suffixLength = 1
		}
		absLevel := level
		if absLevel < 0 {
			absLevel = -absLevel
		}
		if int(absLevel) > (3<<uint(suffixLength-1)) && suffixLength < 6 {
			suffixLength++
		}
	}

	// 4. Write total_zeros
	if totalCoeff < maxNumCoeff {
		writeTotalZeros(w, totalZeros, totalCoeff, nC, maxNumCoeff)
	}

	// 5. Write run_before
	zerosLeft := totalZeros
	for i := 0; i < len(runs)-1 && zerosLeft > 0; i++ {
		writeRunBefore(w, runs[i], zerosLeft)
		zerosLeft -= runs[i]
	}

	return totalCoeff
}

func writeCoeffToken(w *BitWriter, totalCoeff, trailingOnes, nC, maxNumCoeff int) {
	idx := 4*totalCoeff + trailingOnes
	if nC == -1 && maxNumCoeff == 4 {
		// Chroma DC
		bits := encChromaDCCoeffTokenBits[idx]
		length := encChromaDCCoeffTokenLen[idx]
		w.WriteBits(uint32(bits), int(length))
	} else {
		tableIdx := coeffTokenTableIdx(nC)
		bits := encCoeffTokenBits[tableIdx][idx]
		length := encCoeffTokenLen[tableIdx][idx]
		w.WriteBits(uint32(bits), int(length))
	}
}

func coeffTokenTableIdx(nC int) int {
	switch {
	case nC <= 1:
		return 0
	case nC <= 3:
		return 1
	case nC <= 7:
		return 2
	default:
		return 3
	}
}

func writeLevelVLC(w *BitWriter, levelCode, suffixLength int) {
	// Level prefix + suffix encoding (section 9.2.2)
	//
	// When suffixLength == 0:
	//   levelCode  0-13: prefix = levelCode, no suffix
	//   levelCode 14-29: prefix = 14, 4-bit suffix = levelCode - 14
	//   levelCode >= 30: prefix >= 15, (prefix-3)-bit suffix
	//
	// When suffixLength > 0:
	//   prefix = levelCode >> suffixLength
	//   prefix < 14:  suffix = levelCode & ((1<<suffixLength)-1), suffixLength bits
	//   prefix == 14: suffix = levelCode - (14<<suffixLength), suffixLength bits
	//   prefix >= 15: (prefix-3)-bit suffix
	//
	// For prefix >= 15, the decoder computes:
	//   levelCode = min(15,prefix)<<suffixLength + suffix
	//   if prefix >= 15 && suffixLength == 0: levelCode += 15
	//   if prefix >= 16: levelCode += (1<<(prefix-3)) - 4096

	var levelPrefix int
	if suffixLength > 0 {
		levelPrefix = levelCode >> uint(suffixLength)
		if levelPrefix >= 15 {
			// Need escape coding. Find the correct prefix >= 15.
			// prefix=15: range [(15<<sL), (15<<sL) + 4095]
			// prefix=P (P>=16): base = (15<<sL) + (1<<(P-3)) - 4096
			//   range [base, base + (1<<(P-3)) - 1]
			levelPrefix = 15
			base := 15 << uint(suffixLength)
			for levelCode >= base+(1<<uint(levelPrefix-3)) {
				base += 1 << uint(levelPrefix-3)
				levelPrefix++
			}
		}
	} else {
		if levelCode < 14 {
			levelPrefix = levelCode
		} else if levelCode < 30 {
			levelPrefix = 14
		} else {
			// Find the correct escape prefix for large levelCodes.
			// prefix=15: levelCode range [30, 30+4095]
			// prefix=P (P>=16): base = 30 + (1<<(P-3)) - 4096
			//   levelCode range [base, base + (1<<(P-3)) - 1]
			levelPrefix = 15
			base := 30
			for levelCode >= base+(1<<uint(levelPrefix-3)) {
				base += 1 << uint(levelPrefix-3)
				levelPrefix++
			}
		}
	}

	if levelPrefix < 14 {
		// Write levelPrefix zeros followed by 1
		for i := 0; i < levelPrefix; i++ {
			w.WriteBit(0)
		}
		w.WriteBit(1)
		// Write suffix
		if suffixLength > 0 {
			suffix := levelCode - (levelPrefix << uint(suffixLength))
			w.WriteBits(uint32(suffix), suffixLength)
		}
	} else if levelPrefix == 14 {
		// Write 14 zeros + 1
		for range 14 {
			w.WriteBit(0)
		}
		w.WriteBit(1)
		if suffixLength > 0 {
			suffix := levelCode - (14 << uint(suffixLength))
			w.WriteBits(uint32(suffix), suffixLength)
		} else {
			// When suffixLength=0, level_prefix=14 uses 4-bit suffix
			suffix := levelCode - 14
			w.WriteBits(uint32(suffix), 4)
		}
	} else {
		// level_prefix >= 15: escape coding with (prefix-3) suffix bits
		for range levelPrefix {
			w.WriteBit(0)
		}
		w.WriteBit(1)
		suffBits := levelPrefix - 3

		// Compute the base levelCode for this prefix (inverse of decoder formula)
		var base int
		if suffixLength > 0 {
			base = 15 << uint(suffixLength)
		} else {
			base = 30
		}
		if levelPrefix >= 16 {
			base += (1 << uint(levelPrefix-3)) - 4096
		}

		suffix := levelCode - base
		w.WriteBits(uint32(suffix), suffBits)
	}
}

func writeTotalZeros(w *BitWriter, totalZeros, totalCoeff, nC, maxNumCoeff int) {
	if nC == -1 && maxNumCoeff == 4 {
		// Chroma DC total_zeros (Table 9-9)
		idx := totalCoeff - 1
		bits := encChromaDCTotalZerosBits[idx][totalZeros]
		length := encChromaDCTotalZerosLen[idx][totalZeros]
		w.WriteBits(uint32(bits), int(length))
	} else {
		// Normal total_zeros (Table 9-7)
		idx := totalCoeff - 1
		bits := encTotalZerosBits[idx][totalZeros]
		length := encTotalZerosLen[idx][totalZeros]
		w.WriteBits(uint32(bits), int(length))
	}
}

func writeRunBefore(w *BitWriter, runBefore, zerosLeft int) {
	idx := min(zerosLeft-1, 6)
	bits := encRunBeforeBits[idx][runBefore]
	length := encRunBeforeLen[idx][runBefore]
	w.WriteBits(uint32(bits), int(length))
}
