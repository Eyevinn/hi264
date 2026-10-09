package slice

// Bit-depth-dependent limits of the standard. They are written in terms of the bit depth so that
// they stay right if bit depths above 8 are supported.
const (
	// baseBitDepth is the bit depth the QP ranges are expressed relative to.
	baseBitDepth = 8
	// qpBdOffsetPerBit is the extension of the QP range per bit above baseBitDepth
	// (QpBdOffset = 6 * (bitDepth - 8), equations 7-4 and 7-6).
	qpBdOffsetPerBit = 6
	// numQPBase is the number of QP values at baseBitDepth: QPY is in 0..51.
	numQPBase = 52

	// transformRangeLog2Offset gives the range of the scaled coefficients in clause 8.5:
	// -2^(transformRangeLog2Offset + bitDepth)..2^(transformRangeLog2Offset + bitDepth) - 1.
	transformRangeLog2Offset = 7
	// minScaleLog2 is a lower bound on log2 of the smallest factor that scales a coefficient level:
	// an 8x8 block at qP 0 with a scaling list weight of 1 gives 18/64, which is above 2^-2.
	minScaleLog2 = -2
)

// qpBdOffset returns QpBdOffset for a bit depth (equations 7-4 and 7-6).
func qpBdOffset(bitDepth int) int {
	return qpBdOffsetPerBit * (bitDepth - baseBitDepth)
}

// qpDeltaRange returns the range of a conforming mb_qp_delta:
// -(26 + QpBdOffsetY/2)..+(25 + QpBdOffsetY/2) (section 7.4.5).
func qpDeltaRange(bitDepthY int) (lo, hi int) {
	half := qpBdOffset(bitDepthY) / 2
	return -(numQPBase/2 + half), numQPBase/2 - 1 + half
}

// qpDeltaMaxUnary returns the longest unary code of a conforming mb_qp_delta. The unary value maps
// 1 to +1, 2 to -1, and so on, so the most negative value of qpDeltaRange, -n, takes 2n bins.
func qpDeltaMaxUnary(bitDepthY int) int {
	lo, _ := qpDeltaRange(bitDepthY)
	return -2 * lo
}

// coeffAbsLevelMaxPrefix returns the longest Exp-Golomb prefix in the suffix of a conforming
// coeff_abs_level_minus1. Clause 8.5 keeps the scaled coefficients below
// 2^(transformRangeLog2Offset + bitDepth) in magnitude, and even the smallest scaling then keeps a
// level below 2^(transformRangeLog2Offset + bitDepth - minScaleLog2). A prefix of k bins means a
// level of at least 2^k + 14, as the suffix starts at coeff_abs_level_minus1 = 14 (section 9.3.2.3),
// so k is at most one less than that exponent.
func coeffAbsLevelMaxPrefix(bitDepth int) uint {
	return uint(transformRangeLog2Offset + bitDepth - minScaleLog2 - 1)
}
