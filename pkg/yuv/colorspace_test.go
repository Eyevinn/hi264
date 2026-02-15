package yuv

import (
	"math"
	"testing"
)

func TestRGBToYCbCrBT601KnownValues(t *testing.T) {
	// BT.601 limited range reference values for common colors
	tests := []struct {
		name      string
		r, g, b   uint8
		wantY     uint8
		wantCb    uint8
		wantCr    uint8
		tolerance int // allow ±tolerance due to fixed-point rounding
	}{
		{"black", 0, 0, 0, 16, 128, 128, 1},
		{"white", 255, 255, 255, 235, 128, 128, 1},
		{"red", 255, 0, 0, 81, 90, 240, 1},
		{"green", 0, 255, 0, 145, 54, 34, 1},
		{"blue", 0, 0, 255, 41, 240, 110, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := RGBToYCbCrCS(tt.r, tt.g, tt.b, BT601, LimitedRange)
			if abs(int(c.Y)-int(tt.wantY)) > tt.tolerance {
				t.Errorf("Y = %d, want %d±%d", c.Y, tt.wantY, tt.tolerance)
			}
			if abs(int(c.Cb)-int(tt.wantCb)) > tt.tolerance {
				t.Errorf("Cb = %d, want %d±%d", c.Cb, tt.wantCb, tt.tolerance)
			}
			if abs(int(c.Cr)-int(tt.wantCr)) > tt.tolerance {
				t.Errorf("Cr = %d, want %d±%d", c.Cr, tt.wantCr, tt.tolerance)
			}
		})
	}
}

func TestRGBToYCbCrBT709KnownValues(t *testing.T) {
	// BT.709 limited range: different luma weights
	// Kr=0.2126, Kb=0.0722 → Y for pure green should be higher than BT.601
	tests := []struct {
		name      string
		r, g, b   uint8
		wantY     uint8
		tolerance int
	}{
		{"black", 0, 0, 0, 16, 1},
		{"white", 255, 255, 255, 235, 1},
		// BT.709 green should have higher luma than BT.601 green (Kg is larger)
		{"green", 0, 255, 0, 0, 0}, // just check it's different from BT.601
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := RGBToYCbCrCS(tt.r, tt.g, tt.b, BT709, LimitedRange)
			if tt.name == "green" {
				bt601 := RGBToYCbCrCS(0, 255, 0, BT601, LimitedRange)
				if c.Y == bt601.Y {
					t.Error("BT.709 and BT.601 green Y should differ")
				}
				// BT.709 has larger Kg, so green luma should be higher
				if c.Y < bt601.Y {
					t.Errorf("BT.709 green Y=%d should be >= BT.601 green Y=%d", c.Y, bt601.Y)
				}
				return
			}
			if abs(int(c.Y)-int(tt.wantY)) > tt.tolerance {
				t.Errorf("Y = %d, want %d±%d", c.Y, tt.wantY, tt.tolerance)
			}
		})
	}
}

func TestRGBToYCbCrBT2020KnownValues(t *testing.T) {
	// Sanity: black and white should be same across all standards (limited range)
	black := RGBToYCbCrCS(0, 0, 0, BT2020, LimitedRange)
	if abs(int(black.Y)-16) > 1 {
		t.Errorf("BT.2020 black Y = %d, want ~16", black.Y)
	}
	white := RGBToYCbCrCS(255, 255, 255, BT2020, LimitedRange)
	if abs(int(white.Y)-235) > 1 {
		t.Errorf("BT.2020 white Y = %d, want ~235", white.Y)
	}
}

func TestRoundTripAllColorSpaces(t *testing.T) {
	colorSpaces := []struct {
		name string
		cs   ColorSpace
		rng  Range
	}{
		{"BT601-limited", BT601, LimitedRange},
		{"BT709-limited", BT709, LimitedRange},
		{"BT2020-limited", BT2020, LimitedRange},
		{"BT601-full", BT601, FullRange},
		{"BT709-full", BT709, FullRange},
		{"BT2020-full", BT2020, FullRange},
	}

	// Test a few representative colors
	rgbSamples := [][3]uint8{
		{0, 0, 0},
		{255, 255, 255},
		{128, 128, 128},
		{255, 0, 0},
		{0, 255, 0},
		{0, 0, 255},
		{191, 191, 191},
		{64, 128, 192},
	}

	for _, csTest := range colorSpaces {
		t.Run(csTest.name, func(t *testing.T) {
			maxErr := 0
			for _, rgb := range rgbSamples {
				ycbcr := RGBToYCbCrCS(rgb[0], rgb[1], rgb[2], csTest.cs, csTest.rng)
				rr, rg, rb := YCbCrToRGBCS(int(ycbcr.Y), int(ycbcr.Cb), int(ycbcr.Cr), csTest.cs, csTest.rng)

				errR := abs(int(rr) - int(rgb[0]))
				errG := abs(int(rg) - int(rgb[1]))
				errB := abs(int(rb) - int(rgb[2]))
				maxComp := max(errR, max(errG, errB))
				if maxComp > maxErr {
					maxErr = maxComp
				}

				// Fixed-point rounding can accumulate, especially for
				// saturated primaries in full-range or BT.2020.
				if maxComp > 14 {
					t.Errorf("RGB(%d,%d,%d) → YCbCr(%d,%d,%d) → RGB(%d,%d,%d), max error=%d",
						rgb[0], rgb[1], rgb[2],
						ycbcr.Y, ycbcr.Cb, ycbcr.Cr,
						rr, rg, rb, maxComp)
				}
			}
			t.Logf("max round-trip error: %d", maxErr)
		})
	}
}

func TestBackwardCompatibility(t *testing.T) {
	// RGBToYCbCr (no CS) should match RGBToYCbCrCS with BT601/LimitedRange
	for r := 0; r < 256; r += 51 {
		for g := 0; g < 256; g += 51 {
			for b := 0; b < 256; b += 51 {
				old := RGBToYCbCr(uint8(r), uint8(g), uint8(b))
				new := RGBToYCbCrCS(uint8(r), uint8(g), uint8(b), BT601, LimitedRange)
				if old != new {
					t.Errorf("RGBToYCbCr(%d,%d,%d) = %v, RGBToYCbCrCS = %v", r, g, b, old, new)
				}
			}
		}
	}
}

func TestParseColorSpace(t *testing.T) {
	tests := []struct {
		input string
		want  ColorSpace
		err   bool
	}{
		{"bt601", BT601, false},
		{"BT601", BT601, false},
		{"bt.601", BT601, false},
		{"bt709", BT709, false},
		{"BT709", BT709, false},
		{"bt.709", BT709, false},
		{"bt2020", BT2020, false},
		{"BT2020", BT2020, false},
		{"bt.2020", BT2020, false},
		{"invalid", BT601, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseColorSpace(tt.input)
			if (err != nil) != tt.err {
				t.Errorf("error = %v, wantErr %v", err, tt.err)
			}
			if !tt.err && got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestColorSpaceH264Values(t *testing.T) {
	// Verify the H.264 code values are correct per the spec
	if BT709.ColourPrimaries() != 1 {
		t.Errorf("BT.709 colour_primaries = %d, want 1", BT709.ColourPrimaries())
	}
	if BT709.MatrixCoefficients() != 1 {
		t.Errorf("BT.709 matrix_coefficients = %d, want 1", BT709.MatrixCoefficients())
	}
	if BT601.ColourPrimaries() != 5 {
		t.Errorf("BT.601 colour_primaries = %d, want 5", BT601.ColourPrimaries())
	}
	if BT2020.ColourPrimaries() != 9 {
		t.Errorf("BT.2020 colour_primaries = %d, want 9", BT2020.ColourPrimaries())
	}
	if BT2020.MatrixCoefficients() != 9 {
		t.Errorf("BT.2020 matrix_coefficients = %d, want 9", BT2020.MatrixCoefficients())
	}
}

func TestColorSpaceFromMatrixCoefficients(t *testing.T) {
	tests := []struct {
		mc   uint
		want ColorSpace
	}{
		{1, BT709},
		{9, BT2020},
		{5, BT601},
		{6, BT601},
		{0, BT601}, // unspecified defaults to BT.601
	}
	for _, tt := range tests {
		got := ColorSpaceFromMatrixCoefficients(tt.mc)
		if got != tt.want {
			t.Errorf("ColorSpaceFromMatrixCoefficients(%d) = %v, want %v", tt.mc, got, tt.want)
		}
	}
}

func TestFullRangeConversion(t *testing.T) {
	// Full range: black should be Y=0, white should be Y=255
	black := RGBToYCbCrCS(0, 0, 0, BT601, FullRange)
	if black.Y != 0 {
		t.Errorf("full-range black Y = %d, want 0", black.Y)
	}
	white := RGBToYCbCrCS(255, 255, 255, BT601, FullRange)
	if white.Y != 255 {
		t.Errorf("full-range white Y = %d, want 255", white.Y)
	}
	// Chroma for achromatic colors should still be 128
	if abs(int(black.Cb)-128) > 1 {
		t.Errorf("full-range black Cb = %d, want ~128", black.Cb)
	}
}

func TestColorSpaceString(t *testing.T) {
	tests := []struct {
		cs   ColorSpace
		want string
	}{
		{BT601, "bt601"},
		{BT709, "bt709"},
		{BT2020, "bt2020"},
		{ColorSpace(99), "ColorSpace(99)"},
	}
	for _, tt := range tests {
		if got := tt.cs.String(); got != tt.want {
			t.Errorf("ColorSpace(%d).String() = %q, want %q", int(tt.cs), got, tt.want)
		}
	}
}

func TestRangeString(t *testing.T) {
	tests := []struct {
		r    Range
		want string
	}{
		{LimitedRange, "limited"},
		{FullRange, "full"},
		{Range(99), "Range(99)"},
	}
	for _, tt := range tests {
		if got := tt.r.String(); got != tt.want {
			t.Errorf("Range(%d).String() = %q, want %q", int(tt.r), got, tt.want)
		}
	}
}

func TestTransferCharacteristics(t *testing.T) {
	tests := []struct {
		cs   ColorSpace
		want uint
	}{
		{BT601, 6},
		{BT709, 1},
		{BT2020, 14},
	}
	for _, tt := range tests {
		if got := tt.cs.TransferCharacteristics(); got != tt.want {
			t.Errorf("%s.TransferCharacteristics() = %d, want %d", tt.cs, got, tt.want)
		}
	}
}

func TestColourPrimaries(t *testing.T) {
	if BT601.ColourPrimaries() != 5 {
		t.Errorf("BT601 colour_primaries = %d, want 5", BT601.ColourPrimaries())
	}
}

func TestParseColorSpaceErrors(t *testing.T) {
	invalid := []string{"", "h264", "bt999", "srgb", "rec709"}
	for _, s := range invalid {
		_, err := ParseColorSpace(s)
		if err == nil {
			t.Errorf("ParseColorSpace(%q) should return error", s)
		}
	}
}

func abs(x int) int {
	return int(math.Abs(float64(x)))
}
