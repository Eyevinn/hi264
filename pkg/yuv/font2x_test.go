package yuv

import "testing"

func TestGlyph2xDefined(t *testing.T) {
	// All digits must be defined
	for ch := byte('0'); ch <= '9'; ch++ {
		if !HasGlyph2x(ch) {
			t.Errorf("missing 2x glyph for %q", string(ch))
		}
	}
	// Common punctuation
	for _, ch := range []byte{':', '.', '-', '/', ' ', '%'} {
		if !HasGlyph2x(ch) {
			t.Errorf("missing 2x glyph for %q", string(ch))
		}
	}
}

func TestGlyph2xPixels(t *testing.T) {
	// '0' top-left should be off (rounded corner), top-center should be on
	if GlyphPixel2x('0', 0, 0) {
		t.Error("'0' pixel (0,0) should be off")
	}
	if !GlyphPixel2x('0', 1, 0) {
		t.Error("'0' pixel (1,0) should be on")
	}
	// Space should be all off
	for row := range 10 {
		for col := range 6 {
			if GlyphPixel2x(' ', col, row) {
				t.Errorf("space pixel (%d,%d) should be off", col, row)
			}
		}
	}
}

func TestTextWidth2x(t *testing.T) {
	// Single char: 6 blocks
	if w := TextWidth2x("0"); w != 6 {
		t.Errorf("TextWidth2x('0') = %d, want 6", w)
	}
	// Two chars: 6 + 1 + 6 = 13
	if w := TextWidth2x("00"); w != 13 {
		t.Errorf("TextWidth2x('00') = %d, want 13", w)
	}
	// Three chars: 6 + 1 + 6 + 1 + 6 = 20
	if w := TextWidth2x("000"); w != 20 {
		t.Errorf("TextWidth2x('000') = %d, want 20", w)
	}
}

func TestTextHeight2x(t *testing.T) {
	if h := TextHeight2x("0"); h != 10 {
		t.Errorf("TextHeight2x('0') = %d, want 10", h)
	}
	// Two lines: 10 + 2 + 10 = 22
	if h := TextHeight2x("0\n0"); h != 22 {
		t.Errorf("TextHeight2x('0\\n0') = %d, want 22", h)
	}
}

func TestAutoTextScale2x(t *testing.T) {
	// "0" needs 6×10 at scale 1, plus 4 border = 10×14
	// At scale 2: 12+4=16 wide, 20+4=24 tall
	s := AutoTextScale2x("0", 16, 24)
	if s != 2 {
		t.Errorf("AutoTextScale2x('0', 16, 24) = %d, want 2", s)
	}
	// Too small for scale 2
	s = AutoTextScale2x("0", 15, 24)
	if s != 1 {
		t.Errorf("AutoTextScale2x('0', 15, 24) = %d, want 1", s)
	}
}

func TestTextGrid2x(t *testing.T) {
	grid, colors, err := TextGrid2x("0", 20, 20, 1, Color{16, 128, 128}, Color{235, 128, 128}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if grid.Width != 20 || grid.Height != 20 {
		t.Errorf("grid %dx%d, want 20x20", grid.Width, grid.Height)
	}
	if len(colors) != 2 {
		t.Errorf("got %d colors, want 2", len(colors))
	}
	// Should have some foreground pixels
	hasFG := false
	for y := range grid.Height {
		for x := range grid.Width {
			if grid.Chars[y][x] == '#' {
				hasFG = true
			}
		}
	}
	if !hasFG {
		t.Error("no foreground pixels in grid")
	}
}

func TestOverlayText2x(t *testing.T) {
	// Create a 30×20 background grid (240×160 pixels at 8x8 blocks)
	bg, bgColors := SolidGrid(240, 160, Color{128, 128, 128})
	// Upscale to 8x8 block resolution (double the grid dimensions)
	bg = UpscaleGrid(bg, 2)

	result, merged, err := OverlayText2x(bg, bgColors, "0", 1,
		Color{235, 128, 128}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Width != bg.Width || result.Height != bg.Height {
		t.Errorf("grid size %dx%d, want %dx%d", result.Width, result.Height, bg.Width, bg.Height)
	}
	if _, ok := merged['#']; !ok {
		t.Error("merged colors missing foreground '#'")
	}
	hasFG := false
	for y := range result.Height {
		for x := range result.Width {
			if result.Chars[y][x] == '#' {
				hasFG = true
			}
		}
	}
	if !hasFG {
		t.Error("no foreground pixels")
	}
}

func TestOverlayText2xWithBg(t *testing.T) {
	bg, bgColors := SolidGrid(240, 160, Color{128, 128, 128})
	bg = UpscaleGrid(bg, 2)
	textBg := Color{16, 128, 128}
	_, merged, err := OverlayText2x(bg, bgColors, "A", 1,
		Color{235, 128, 128}, &textBg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := merged['@']; !ok {
		t.Error("merged colors missing text background '@'")
	}
}

func TestOverlayText2xTooLarge(t *testing.T) {
	bg, bgColors := SolidGrid(16, 16, Color{128, 128, 128})
	// 1x1 grid is way too small for any text
	_, _, err := OverlayText2x(bg, bgColors, "HELLO", 3,
		Color{235, 128, 128}, nil)
	if err == nil {
		t.Error("expected error for text too large")
	}
}

func TestTextGrid2xToPlaneGrid(t *testing.T) {
	grid, colors, err := TextGrid2x("0", 20, 20, 1, Color{16, 128, 128}, Color{235, 128, 128}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pg, err := GridToPlaneGridBS(grid, colors, 8)
	if err != nil {
		t.Fatal(err)
	}
	if pg.BlockSize != 8 {
		t.Errorf("BlockSize = %d, want 8", pg.BlockSize)
	}
	// 20 blocks of 8 = 10 MBs of 16
	if pg.PixelWidth() != 160 || pg.PixelHeight() != 160 {
		t.Errorf("pixel size %dx%d, want 160x160", pg.PixelWidth(), pg.PixelHeight())
	}
}
