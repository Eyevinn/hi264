// Package context implements CABAC context model initialization for H.264/AVC
// as specified in section 9.3.1 of the standard.
package context

import "github.com/Eyevinn/hi264/internal/cabac"

// Models holds all 1024 CABAC context models for a slice.
type Models [1024]cabac.CtxState

// clip3 clamps val to the range [min, max].
func clip3(min, max, val int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

// InitModels initializes all 1024 context models for the given slice parameters.
// sliceQPY is the luma QP for the slice. sliceType: 0=P, 1=B, 2=I.
// cabacInitIDC is 0-2 for P/B slices (unused for I slices).
// This implements section 9.3.1.1, formula 9-5 to 9-8.
func InitModels(sliceQPY int, sliceType int, cabacInitIDC int) Models {
	var models Models

	// Select initialization table based on slice type
	var tab *[1024][2]int8
	if sliceType == 2 || sliceType == 7 { // I-slice (2) or I-slice+5 (7)
		tab = &cabacContextInitI
	} else {
		tab = &cabacContextInitPB[cabacInitIDC]
	}

	qp := clip3(0, 51, sliceQPY)

	for i := range 1024 {
		m := int(tab[i][0])
		n := int(tab[i][1])

		preCtxState := clip3(1, 126, ((m*qp)>>4)+n)

		if preCtxState <= 63 {
			models[i].PStateIdx = uint8(63 - preCtxState)
			models[i].ValMPS = 0
		} else {
			models[i].PStateIdx = uint8(preCtxState - 64)
			models[i].ValMPS = 1
		}
	}

	return models
}
