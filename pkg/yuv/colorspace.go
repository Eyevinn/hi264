package yuv

import (
	"fmt"
	"strings"
)

// ColorSpace identifies the YCbCr color matrix standard.
type ColorSpace int

const (
	BT601  ColorSpace = iota // SD (ITU-R BT.601)
	BT709                    // HD (ITU-R BT.709)
	BT2020                   // UHD/HDR (ITU-R BT.2020)
)

// String returns the name of the color space.
func (cs ColorSpace) String() string {
	switch cs {
	case BT601:
		return "bt601"
	case BT709:
		return "bt709"
	case BT2020:
		return "bt2020"
	default:
		return fmt.Sprintf("ColorSpace(%d)", int(cs))
	}
}

// ParseColorSpace parses a color space name (case-insensitive).
func ParseColorSpace(s string) (ColorSpace, error) {
	switch strings.ToLower(s) {
	case "bt601", "bt.601":
		return BT601, nil
	case "bt709", "bt.709":
		return BT709, nil
	case "bt2020", "bt.2020":
		return BT2020, nil
	default:
		return BT601, fmt.Errorf("unknown color space %q (use bt601, bt709, or bt2020)", s)
	}
}

// Range identifies the sample value range.
type Range int

const (
	LimitedRange Range = iota // Y: 16-235, C: 16-240
	FullRange                 // Y/C: 0-255
)

// String returns the name of the range.
func (r Range) String() string {
	switch r {
	case LimitedRange:
		return "limited"
	case FullRange:
		return "full"
	default:
		return fmt.Sprintf("Range(%d)", int(r))
	}
}

// Fixed-point coefficients (scaled by 256) for RGB→YCbCr conversion.
// Limited range formula:
//
//	Y  = ((Kr_y*R + Kg_y*G + Kb_y*B + 128) >> 8) + 16
//	Cb = ((Kr_cb*R + Kg_cb*G + Kb_cb*B + 128) >> 8) + 128
//	Cr = ((Kr_cr*R + Kg_cr*G + Kb_cr*B + 128) >> 8) + 128
type fwdCoeffs struct {
	kr_y, kg_y, kb_y    int // luma
	kr_cb, kg_cb, kb_cb int // chroma blue
	kr_cr, kg_cr, kb_cr int // chroma red
}

// Fixed-point coefficients (scaled by 256) for YCbCr→RGB conversion (limited range).
//
//	c = Y - 16, d = Cb - 128, e = Cr - 128
//	R = (c_y*c + c_cr*e + 128) >> 8
//	G = (c_y*c + c_cb*d + c_cr_g*e + 128) >> 8
//	B = (c_y*c + c_cb_b*d + 128) >> 8
type invCoeffs struct {
	c_y    int // 1.164 * 256 = 298 for limited range
	c_cr   int // Cr→R coefficient
	c_cb   int // Cb→G coefficient (negative)
	c_cr_g int // Cr→G coefficient (negative)
	c_cb_b int // Cb→B coefficient
}

// BT.601 limited range coefficients (the original hardcoded values).
var fwdBT601 = fwdCoeffs{66, 129, 25, -38, -74, 112, 112, -94, -18}
var invBT601 = invCoeffs{298, 409, -100, -208, 516}

// BT.709 limited range coefficients.
// Kr=0.2126, Kb=0.0722, Kg=1-Kr-Kb=0.7152
var fwdBT709 = fwdCoeffs{47, 157, 16, -26, -87, 112, 112, -102, -10}
var invBT709 = invCoeffs{298, 459, -55, -136, 541}

// BT.2020 limited range coefficients.
// Kr=0.2627, Kb=0.0593, Kg=1-Kr-Kb=0.6780
var fwdBT2020 = fwdCoeffs{58, 149, 13, -32, -80, 112, 112, -96, -16}
var invBT2020 = invCoeffs{298, 430, -48, -167, 548}

// Full range forward coefficients.
// Y  = (Kr_y*R + Kg_y*G + Kb_y*B + 128) >> 8
// Cb = (Kr_cb*R + Kg_cb*G + Kb_cb*B + 128) >> 8 + 128
// Cr = (Kr_cr*R + Kg_cr*G + Kb_cr*B + 128) >> 8 + 128
var fwdBT601Full = fwdCoeffs{77, 150, 29, -43, -85, 128, 128, -107, -21}
var fwdBT709Full = fwdCoeffs{54, 183, 18, -29, -99, 128, 128, -116, -12}
var fwdBT2020Full = fwdCoeffs{67, 174, 15, -37, -91, 128, 128, -110, -18}

// Full range inverse coefficients.
// c = Y, d = Cb - 128, e = Cr - 128
// R = c + c_cr*e >> 8
// etc. (c_y = 256 for full range, no offset)
var invBT601Full = invCoeffs{256, 351, -86, -179, 443}
var invBT709Full = invCoeffs{256, 394, -47, -117, 465}
var invBT2020Full = invCoeffs{256, 369, -41, -143, 471}

func getFwdCoeffs(cs ColorSpace, rng Range) fwdCoeffs {
	if rng == FullRange {
		switch cs {
		case BT709:
			return fwdBT709Full
		case BT2020:
			return fwdBT2020Full
		default:
			return fwdBT601Full
		}
	}
	switch cs {
	case BT709:
		return fwdBT709
	case BT2020:
		return fwdBT2020
	default:
		return fwdBT601
	}
}

func getInvCoeffs(cs ColorSpace, rng Range) invCoeffs {
	if rng == FullRange {
		switch cs {
		case BT709:
			return invBT709Full
		case BT2020:
			return invBT2020Full
		default:
			return invBT601Full
		}
	}
	switch cs {
	case BT709:
		return invBT709
	case BT2020:
		return invBT2020
	default:
		return invBT601
	}
}

// RGBToYCbCrCS converts RGB to YCbCr using the specified color space and range.
func RGBToYCbCrCS(r, g, b uint8, cs ColorSpace, rng Range) Color {
	ri, gi, bi := int(r), int(g), int(b)
	c := getFwdCoeffs(cs, rng)
	if rng == FullRange {
		y := (c.kr_y*ri + c.kg_y*gi + c.kb_y*bi + 128) >> 8
		cb := ((c.kr_cb*ri + c.kg_cb*gi + c.kb_cb*bi + 128) >> 8) + 128
		cr := ((c.kr_cr*ri + c.kg_cr*gi + c.kb_cr*bi + 128) >> 8) + 128
		return Color{Y: clipU8(y), Cb: clipU8(cb), Cr: clipU8(cr)}
	}
	y := ((c.kr_y*ri + c.kg_y*gi + c.kb_y*bi + 128) >> 8) + 16
	cb := ((c.kr_cb*ri + c.kg_cb*gi + c.kb_cb*bi + 128) >> 8) + 128
	cr := ((c.kr_cr*ri + c.kg_cr*gi + c.kb_cr*bi + 128) >> 8) + 128
	return Color{Y: clipU8(y), Cb: clipU8(cb), Cr: clipU8(cr)}
}

// YCbCrToRGBCS converts YCbCr to RGB using the specified color space and range.
func YCbCrToRGBCS(y, cb, cr int, cs ColorSpace, rng Range) (uint8, uint8, uint8) {
	ic := getInvCoeffs(cs, rng)
	d := cb - 128
	e := cr - 128
	if rng == FullRange {
		rv := (ic.c_y*y + ic.c_cr*e + 128) >> 8
		gv := (ic.c_y*y + ic.c_cb*d + ic.c_cr_g*e + 128) >> 8
		bv := (ic.c_y*y + ic.c_cb_b*d + 128) >> 8
		return clampByte(rv), clampByte(gv), clampByte(bv)
	}
	c := y - 16
	rv := (ic.c_y*c + ic.c_cr*e + 128) >> 8
	gv := (ic.c_y*c + ic.c_cb*d + ic.c_cr_g*e + 128) >> 8
	bv := (ic.c_y*c + ic.c_cb_b*d + 128) >> 8
	return clampByte(rv), clampByte(gv), clampByte(bv)
}

func clampByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// ColorSpaceFromMatrixCoefficients maps H.264 matrix_coefficients to a ColorSpace.
func ColorSpaceFromMatrixCoefficients(mc uint) ColorSpace {
	switch mc {
	case 1:
		return BT709
	case 9:
		return BT2020
	default:
		return BT601
	}
}

// ColourPrimaries returns the H.264 colour_primaries value for this color space.
func (cs ColorSpace) ColourPrimaries() uint {
	switch cs {
	case BT709:
		return 1
	case BT2020:
		return 9
	default:
		return 5 // BT.601 (625-line, also matches 6 for 525-line)
	}
}

// TransferCharacteristics returns the H.264 transfer_characteristics value.
func (cs ColorSpace) TransferCharacteristics() uint {
	switch cs {
	case BT709:
		return 1
	case BT2020:
		return 14 // BT.2020-2 (10/12-bit)
	default:
		return 6 // BT.601
	}
}

// MatrixCoefficients returns the H.264 matrix_coefficients value.
func (cs ColorSpace) MatrixCoefficients() uint {
	switch cs {
	case BT709:
		return 1
	case BT2020:
		return 9
	default:
		return 5 // BT.601 (625-line)
	}
}
