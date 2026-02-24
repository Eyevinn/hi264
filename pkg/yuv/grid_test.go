package yuv

import (
	"strings"
	"testing"
)

func TestParseGrid(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantW   int
		wantH   int
		wantErr bool
	}{
		{"single", "x", 1, 1, false},
		{"comma-sep", "xy,yx", 2, 2, false},
		{"newline-sep", "xy\nyx", 2, 2, false},
		{"3x2", "abc,def", 3, 2, false},
		{"empty", "", 0, 0, true},
		{"unequal", "xy,x", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := ParseGrid(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseGrid(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if g.Width != tt.wantW || g.Height != tt.wantH {
				t.Errorf("got %dx%d, want %dx%d", g.Width, g.Height, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestParseColorSpec(t *testing.T) {
	ch, c, err := ParseColorSpec("x=235,128,128", false)
	if err != nil {
		t.Fatal(err)
	}
	if ch != 'x' || c.Y != 235 || c.Cb != 128 || c.Cr != 128 {
		t.Errorf("got %c=%v, want x={235,128,128}", ch, c)
	}
}

func TestParseColorSpecRGB(t *testing.T) {
	ch, c, err := ParseColorSpec("w=255,255,255", true)
	if err != nil {
		t.Fatal(err)
	}
	if ch != 'w' {
		t.Errorf("got char %c, want w", ch)
	}
	// White RGB should produce Y close to 235
	if c.Y < 230 || c.Y > 240 {
		t.Errorf("white Y=%d, expected ~235", c.Y)
	}
}

func TestRGBToYCbCr(t *testing.T) {
	tests := []struct {
		name    string
		r, g, b uint8
		wantY   uint8
		wantCb  uint8
		wantCr  uint8
	}{
		{"black", 0, 0, 0, 16, 128, 128},
		{"white", 255, 255, 255, 235, 128, 128},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := RGBToYCbCr(tt.r, tt.g, tt.b)
			if c.Y != tt.wantY || c.Cb != tt.wantCb || c.Cr != tt.wantCr {
				t.Errorf("RGBToYCbCr(%d,%d,%d) = {%d,%d,%d}, want {%d,%d,%d}",
					tt.r, tt.g, tt.b, c.Y, c.Cb, c.Cr, tt.wantY, tt.wantCb, tt.wantCr)
			}
		})
	}
}

func TestBuildFrame(t *testing.T) {
	grid, err := ParseGrid("xy,yx")
	if err != nil {
		t.Fatal(err)
	}
	colors := ColorMap{
		'x': {Y: 235, Cb: 128, Cr: 128},
		'y': {Y: 16, Cb: 128, Cr: 128},
	}
	f, err := BuildFrame(grid, colors)
	if err != nil {
		t.Fatal(err)
	}
	if f.Width != 32 || f.Height != 32 {
		t.Errorf("frame size %dx%d, want 32x32", f.Width, f.Height)
	}
	// Check MB (0,0) is 'x'
	if f.GetLumaPixel(0, 0) != 235 {
		t.Errorf("pixel (0,0) = %d, want 235", f.GetLumaPixel(0, 0))
	}
	// Check MB (1,0) is 'y'
	if f.GetLumaPixel(16, 0) != 16 {
		t.Errorf("pixel (16,0) = %d, want 16", f.GetLumaPixel(16, 0))
	}
}

func TestBuildFrameMissingColor(t *testing.T) {
	grid, _ := ParseGrid("x")
	_, err := BuildFrame(grid, ColorMap{})
	if err == nil {
		t.Error("expected error for missing color")
	}
}

func TestParseImageFile(t *testing.T) {
	input := `# test image
x=235,128,128
y=16,128,128

xy
yx
`
	grid, colors, err := ParseImageFile(strings.NewReader(input), false)
	if err != nil {
		t.Fatal(err)
	}
	if grid.Width != 2 || grid.Height != 2 {
		t.Errorf("grid %dx%d, want 2x2", grid.Width, grid.Height)
	}
	if len(colors) != 2 {
		t.Errorf("got %d colors, want 2", len(colors))
	}
	if colors['x'].Y != 235 {
		t.Errorf("color x Y=%d, want 235", colors['x'].Y)
	}
}

func TestParseImageFileCSDirectives(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		wantCS ColorSpace
	}{
		{"bt709 directive", "@rgb\n@bt709\nw=255,255,255\n\nw\n", BT709},
		{"bt2020 directive", "@rgb\n@bt2020\nw=255,255,255\n\nw\n", BT2020},
		{"bt601 directive", "@rgb\n@bt601\nw=255,255,255\n\nw\n", BT601},
		{"bt.709 directive", "@rgb\n@bt.709\nw=255,255,255\n\nw\n", BT709},
		{"default is bt601", "@rgb\nw=255,255,255\n\nw\n", BT601},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, cs, err := ParseImageFileCS(strings.NewReader(tt.input), false, BT601, LimitedRange)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cs != tt.wantCS {
				t.Errorf("color space = %v, want %v", cs, tt.wantCS)
			}
		})
	}
}

func TestParseImageFileCSUnknownDirective(t *testing.T) {
	input := "@unknown\nw=255,255,255\n\nw\n"
	_, _, _, err := ParseImageFileCS(strings.NewReader(input), false, BT601, LimitedRange)
	if err == nil {
		t.Error("expected error for unknown directive")
	}
}

func TestParseImageFileCSNoGrid(t *testing.T) {
	input := "x=235,128,128\n"
	_, _, _, err := ParseImageFileCS(strings.NewReader(input), false, BT601, LimitedRange)
	if err == nil {
		t.Error("expected error when no grid is present")
	}
}

func TestParseImageFileCSBadColor(t *testing.T) {
	input := "x=abc\n\nx\n"
	_, _, _, err := ParseImageFileCS(strings.NewReader(input), false, BT601, LimitedRange)
	if err == nil {
		t.Error("expected error for invalid color spec")
	}
}

func TestParseImageFileCSGridWithoutSeparator(t *testing.T) {
	// Grid starts without blank line separator when no = in line
	input := "x=235,128,128\nxy\nyx\n"
	grid, _, _, err := ParseImageFileCS(strings.NewReader(input), false, BT601, LimitedRange)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if grid.Width != 2 || grid.Height != 2 {
		t.Errorf("grid %dx%d, want 2x2", grid.Width, grid.Height)
	}
}

func TestParseImageFileFull8x8(t *testing.T) {
	input := "@rgb\n@8x8\nw=255,255,255\nb=0,0,0\n\nwb\nbw\n"
	res, err := ParseImageFileFull(strings.NewReader(input), false, BT601, LimitedRange)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.BlockSize != 8 {
		t.Errorf("block size = %d, want 8", res.BlockSize)
	}
	if res.Grid.Width != 2 || res.Grid.Height != 2 {
		t.Errorf("grid %dx%d, want 2x2", res.Grid.Width, res.Grid.Height)
	}

	pg, err := GridToPlaneGridBS(res.Grid, res.Colors, res.BlockSize)
	if err != nil {
		t.Fatalf("GridToPlaneGridBS: %v", err)
	}
	if pg.BlockSize != 8 {
		t.Errorf("PlaneGrid.BlockSize = %d, want 8", pg.BlockSize)
	}
	if pg.PixelWidth() != 16 || pg.PixelHeight() != 16 {
		t.Errorf("pixel size %dx%d, want 16x16", pg.PixelWidth(), pg.PixelHeight())
	}
}

func TestParseColorSpecCSInvalidFormat(t *testing.T) {
	badSpecs := []string{
		"",          // no = sign (handled by caller)
		"x=1,2",     // too few values
		"x=1,2,3,4", // too many values
		"x=a,b,c",   // non-numeric
		"=1,2,3",    // empty char
	}
	for _, spec := range badSpecs {
		_, _, err := ParseColorSpecCS(spec, false, BT601, LimitedRange)
		if err == nil {
			t.Errorf("ParseColorSpecCS(%q) should return error", spec)
		}
	}
}

func TestSolidGrid(t *testing.T) {
	g, colors := SolidGrid(32, 32, Color{235, 128, 128})
	if g.Width != 2 || g.Height != 2 {
		t.Errorf("got %dx%d, want 2x2", g.Width, g.Height)
	}
	if colors['.'].Y != 235 || colors['.'].Cb != 128 || colors['.'].Cr != 128 {
		t.Errorf("color = %v, want {235,128,128}", colors['.'])
	}
	for y := range g.Height {
		for x := range g.Width {
			if g.Chars[y][x] != '.' {
				t.Errorf("Chars[%d][%d] = %c, want '.'", y, x, g.Chars[y][x])
			}
		}
	}
}

func TestSolidGridRoundsUp(t *testing.T) {
	g, _ := SolidGrid(33, 17, Color{128, 128, 128})
	if g.Width != 3 || g.Height != 2 {
		t.Errorf("got %dx%d, want 3x2", g.Width, g.Height)
	}
}

func TestUpscaleGrid(t *testing.T) {
	grid, err := ParseGrid("xy,yx")
	if err != nil {
		t.Fatal(err)
	}
	scaled := UpscaleGrid(grid, 2)
	if scaled.Width != 4 || scaled.Height != 4 {
		t.Errorf("got %dx%d, want 4x4", scaled.Width, scaled.Height)
	}
	// Original (0,0)='x' should fill 2x2 block at (0,0)
	for _, pos := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		if scaled.Chars[pos[1]][pos[0]] != 'x' {
			t.Errorf("Chars[%d][%d] = %c, want 'x'", pos[1], pos[0], scaled.Chars[pos[1]][pos[0]])
		}
	}
	// Original (1,0)='y' should fill 2x2 block at (2,0)
	if scaled.Chars[0][2] != 'y' || scaled.Chars[0][3] != 'y' {
		t.Error("top-right block should be 'y'")
	}
}

func TestParseColorSpecCSWithColorSpace(t *testing.T) {
	// RGB white with BT.709 should produce different Y than BT.601 for saturated colors
	_, c601, err := ParseColorSpecCS("r=255,0,0", true, BT601, LimitedRange)
	if err != nil {
		t.Fatal(err)
	}
	_, c709, err := ParseColorSpecCS("r=255,0,0", true, BT709, LimitedRange)
	if err != nil {
		t.Fatal(err)
	}
	// BT.709 red should have different luma than BT.601 red
	if c601.Y == c709.Y {
		t.Error("BT.601 and BT.709 red Y should differ")
	}
}
