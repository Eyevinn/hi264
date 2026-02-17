package yuv

import "testing"

func TestGlyphPixel(t *testing.T) {
	// Verify 'h' matches logo: #.. #.. ### #.# #.#
	hExpected := [5][3]bool{
		{true, false, false},
		{true, false, false},
		{true, true, true},
		{true, false, true},
		{true, false, true},
	}
	for row := range 5 {
		for col := range 3 {
			got := GlyphPixel('h', col, row)
			if got != hExpected[row][col] {
				t.Errorf("GlyphPixel('h', %d, %d) = %v, want %v", col, row, got, hExpected[row][col])
			}
		}
	}

	// Verify 'i' matches logo: .#. ... .#. .#. .#.
	iExpected := [5][3]bool{
		{false, true, false},
		{false, false, false},
		{false, true, false},
		{false, true, false},
		{false, true, false},
	}
	for row := range 5 {
		for col := range 3 {
			got := GlyphPixel('i', col, row)
			if got != iExpected[row][col] {
				t.Errorf("GlyphPixel('i', %d, %d) = %v, want %v", col, row, got, iExpected[row][col])
			}
		}
	}

	// Unsupported character returns false everywhere
	for row := range 5 {
		for col := range 3 {
			if GlyphPixel(0x7F, col, row) {
				t.Errorf("GlyphPixel(0x7F, %d, %d) should be false", col, row)
			}
		}
	}
}

func TestGlyphDigitsMatchExisting(t *testing.T) {
	for digit := 0; digit <= 9; digit++ {
		ch := byte('0' + digit)
		for row := range 5 {
			for col := range 3 {
				fromGlyph := GlyphPixel(ch, col, row)
				fromDigit := digitPixel(digit, col, row)
				if fromGlyph != fromDigit {
					t.Errorf("digit %d pixel(%d,%d): glyph=%v, digitBitmaps=%v",
						digit, col, row, fromGlyph, fromDigit)
				}
			}
		}
	}
}

func TestHasGlyph(t *testing.T) {
	// Supported
	for _, ch := range []byte("ABCabc012!@# ") {
		if !HasGlyph(ch) {
			t.Errorf("HasGlyph(%q) = false, want true", string(ch))
		}
	}
	// Unsupported (control chars, DEL, high bytes)
	for _, ch := range []byte{0, 1, 0x7F, 0x80, 0xFF} {
		if HasGlyph(ch) {
			t.Errorf("HasGlyph(0x%02X) = true, want false", ch)
		}
	}
}

func TestTextWidth(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"", 0},
		{"A", 3},
		{"AB", 7},
		{"ABC", 11},
		{"hi264", 19},
	}
	for _, tc := range tests {
		got := TextWidth(tc.text)
		if got != tc.want {
			t.Errorf("TextWidth(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func TestTextWidthMultiLine(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"A\nBC", 7},         // max("A"=3, "BC"=7) = 7
		{"AB\nC", 7},         // max("AB"=7, "C"=3) = 7
		{"AB\nCD\nE", 7},     // max(7, 7, 3) = 7
		{"\n", 0},            // two empty lines
		{"A\n", 3},           // "A" + empty line
		{"hello\nworld", 19}, // both 5 chars = 19
	}
	for _, tc := range tests {
		got := TextWidth(tc.text)
		if got != tc.want {
			t.Errorf("TextWidth(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func TestTextHeight(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"A", 5},        // 1 line: 5
		{"A\nB", 11},    // 2 lines: 5+1+5
		{"A\nB\nC", 17}, // 3 lines: 5+1+5+1+5
		{"", 5},         // "" splits to [""], 1 line
	}
	for _, tc := range tests {
		got := TextHeight(tc.text)
		if got != tc.want {
			t.Errorf("TextHeight(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func TestAutoTextScale(t *testing.T) {
	tests := []struct {
		text      string
		mbW, mbH  int
		wantScale int
	}{
		// "hi264" → tw=19. S=2: 19*2+2=40 ≤ 48, 5*2+2=12 ≤ 27 → ok.
		// S=3: 19*3+2=59 > 48 → no.
		{"hi264", 48, 27, 2},
		// Single char: tw=3. S=1: 3+2=5, 5+2=7.
		{"A", 5, 7, 1},
		// Empty text
		{"", 10, 10, 1},
		// Larger frame
		// tw=7; S=14: 7*14+2=100<=100, 5*14+2=72<=100; S=15: 107>100 → S=14
		{"AB", 100, 100, 13},
	}
	// Recalculate "AB" case: tw=7, S=14: 7*14+2=100 ≤ 100, 5*14+2=72 ≤ 100. S=15: 7*15+2=107 > 100. So want 14.
	tests[3].wantScale = 14

	// Multi-line: "AB\nCD" → tw=7, th=11.
	// S=1: 7+2=9<=100, 11+2=13<=100. S=8: 7*8+2=58<=100, 11*8+2=90<=100.
	// S=9: 7*9+2=65<=100, 11*9+2=101>100. So want 8.
	tests = append(tests, struct {
		text      string
		mbW, mbH  int
		wantScale int
	}{"AB\nCD", 100, 100, 8})

	for _, tc := range tests {
		got := AutoTextScale(tc.text, tc.mbW, tc.mbH)
		if got != tc.wantScale {
			t.Errorf("AutoTextScale(%q, %d, %d) = %d, want %d",
				tc.text, tc.mbW, tc.mbH, got, tc.wantScale)
		}
	}
}

func TestTextGrid(t *testing.T) {
	bg := Color{16, 128, 128}
	fg := Color{235, 128, 128}

	// Render "A" at scale 1 in minimal frame (3×5)
	grid, colors, err := TextGrid("A", 3, 5, 1, bg, fg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if grid.Width != 3 || grid.Height != 5 {
		t.Errorf("grid size %dx%d, want 3x5", grid.Width, grid.Height)
	}
	if len(colors) != 2 {
		t.Errorf("colormap has %d entries, want 2", len(colors))
	}

	// Verify 'A' glyph: .#. #.# ### #.# #.#
	// Row 0, col 1 should be '#' (top of A)
	if grid.Chars[0][1] != '#' {
		t.Error("A: row 0, col 1 should be '#'")
	}
	// Row 0, col 0 should be '.'
	if grid.Chars[0][0] != '.' {
		t.Error("A: row 0, col 0 should be '.'")
	}

	// Too small → error
	_, _, err = TextGrid("hello", 10, 3, 1, bg, fg, nil)
	if err == nil {
		t.Error("expected error for frame too small")
	}
}

func TestTextGridMultiLine(t *testing.T) {
	bg := Color{16, 128, 128}
	fg := Color{235, 128, 128}

	// "A\nB" at scale 1: tw=3, th=11 (5+1+5). Need 3×11 minimum.
	grid, _, err := TextGrid("A\nB", 5, 13, 1, bg, fg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if grid.Width != 5 || grid.Height != 13 {
		t.Errorf("grid size %dx%d, want 5x13", grid.Width, grid.Height)
	}

	// With 5×13 frame and 3×11 text area: offsetX=1, offsetY=1
	// Line 1 ("A"): lineWidth=3, centered at offsetX + (3-3)/2 = 1
	// A row 0: .#. → (1,1)='.', (2,1)='#', (3,1)='.'
	if grid.Chars[1][2] != '#' {
		t.Error("A top center should be '#'")
	}

	// Line 2 ("B") starts at lineY = 1 + 6 = 7
	// B row 0: ##. → (1,7)='#', (2,7)='#', (3,7)='.'
	if grid.Chars[7][1] != '#' || grid.Chars[7][2] != '#' {
		t.Error("B top-left should be '##'")
	}
	if grid.Chars[7][3] != '.' {
		t.Error("B top-right should be '.'")
	}

	// Gap row between lines (row 6 = offsetY + 5 = 6) should be background
	if grid.Chars[6][1] != '.' || grid.Chars[6][2] != '.' {
		t.Error("gap row should be background '.'")
	}
}

func TestTextGridMultiLineCentering(t *testing.T) {
	bg := Color{16, 128, 128}
	fg := Color{235, 128, 128}

	// "AB\nC" at scale 1: tw=7 (from "AB"), th=11.
	// Line "AB" width=7, line "C" width=3.
	// In a 9×13 frame: textArea 7×11, offsetX=1, offsetY=1
	// "AB" is full width → lineOffsetX = 1 + (7-7)/2 = 1
	// "C" is centered → lineOffsetX = 1 + (7-3)/2 = 3
	grid, _, err := TextGrid("AB\nC", 9, 13, 1, bg, fg, nil)
	if err != nil {
		t.Fatal(err)
	}

	// "C" starts at row 7 (1 + 6*1), col 3
	// C row 0: .## → (3,7)='.', (4,7)='#', (5,7)='#'
	if grid.Chars[7][4] != '#' || grid.Chars[7][5] != '#' {
		t.Error("C top should be '##' at cols 4-5")
	}
	if grid.Chars[7][3] != '.' {
		t.Error("C top-left col should be '.'")
	}
}

func TestTextGridCentering(t *testing.T) {
	bg := Color{16, 128, 128}
	fg := Color{235, 128, 128}

	// "A" in 7×9: textArea 3×5, offsetX=2, offsetY=2
	grid, _, err := TextGrid("A", 7, 9, 1, bg, fg, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Corners should be background
	if grid.Chars[0][0] != '.' {
		t.Error("top-left should be '.'")
	}
	if grid.Chars[8][6] != '.' {
		t.Error("bottom-right should be '.'")
	}

	// A's top pixel at offset (2,2) + glyph (1,0) = (3,2) should be '#'
	if grid.Chars[2][3] != '#' {
		t.Error("A top center pixel should be '#' at (3,2)")
	}
}

func TestTextGridWithTextBg(t *testing.T) {
	bg := Color{16, 128, 128}
	fg := Color{235, 128, 128}
	tbg := Color{80, 128, 128}

	// "A" at scale 1 in 7×9: text 3×5 centered at (2,2), box (1,1)-(5,7) = 5×7=35
	grid, colors, err := TextGrid("A", 7, 9, 1, bg, fg, &tbg)
	if err != nil {
		t.Fatal(err)
	}

	if len(colors) != 3 {
		t.Errorf("colormap has %d entries, want 3", len(colors))
	}
	if colors['@'] != tbg {
		t.Errorf("'@' color = %v, want %v", colors['@'], tbg)
	}

	// Count cell types
	var dots, hashes, ats int
	for y := range grid.Height {
		for x := range grid.Width {
			switch grid.Chars[y][x] {
			case '.':
				dots++
			case '#':
				hashes++
			case '@':
				ats++
			}
		}
	}

	// A has 10 foreground pixels (.#. #.# ### #.# #.#), box = 5×7=35, '@' = 35-10=25
	if hashes != 10 {
		t.Errorf("'A' has %d '#' cells, want 10", hashes)
	}
	if ats != 25 {
		t.Errorf("'A' has %d '@' cells, want 25", ats)
	}
}

func TestOverlayText(t *testing.T) {
	// Create a simple 11×7 grid filled with 'x'
	chars := make([][]byte, 7)
	for y := range 7 {
		chars[y] = make([]byte, 11)
		for x := range 11 {
			chars[y][x] = 'x'
		}
	}
	baseGrid := &Grid{Chars: chars, Width: 11, Height: 7}
	baseColors := ColorMap{'x': Color{128, 128, 128}}

	fg := Color{235, 128, 128}
	grid, colors, err := OverlayText(baseGrid, baseColors, "A", 1, fg, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Original grid cells should show through where no glyph
	if grid.Chars[0][0] != 'x' {
		t.Error("transparent cell should preserve original 'x'")
	}

	// Foreground color added to merged colormap
	if colors['#'] != fg {
		t.Errorf("'#' color = %v, want %v", colors['#'], fg)
	}
	// Original color preserved
	if colors['x'] != baseColors['x'] {
		t.Errorf("'x' color changed")
	}

	// Glyph pixels should be '#' (A centered in 11×7: offset (4,1))
	// A row 0, col 1 → grid (5, 1)
	if grid.Chars[1][5] != '#' {
		t.Error("A center-top pixel should be '#'")
	}
}

func TestOverlayTextWithBg(t *testing.T) {
	// SMPTE-like background
	smpteGrid, smpteColors := SMPTEBarsGrid(11)
	// Tile to 11×7
	chars := make([][]byte, 7)
	for y := range 7 {
		chars[y] = make([]byte, 11)
		copy(chars[y], smpteGrid.Chars[0])
	}
	tiledGrid := &Grid{Chars: chars, Width: 11, Height: 7}

	fg := Color{235, 128, 128}
	tbg := Color{0, 128, 128}
	grid, colors, err := OverlayText(tiledGrid, smpteColors, "A", 1, fg, &tbg)
	if err != nil {
		t.Fatal(err)
	}

	// Should have '@' in the text background area
	hasAt := false
	for y := range grid.Height {
		for x := range grid.Width {
			if grid.Chars[y][x] == '@' {
				hasAt = true
				break
			}
		}
	}
	if !hasAt {
		t.Error("expected '@' cells for text background")
	}
	if colors['@'] != tbg {
		t.Errorf("'@' color = %v, want %v", colors['@'], tbg)
	}
}

func TestLogoReconstruction(t *testing.T) {
	// The logo is "hi264" at scale 2 on a 48×27 frame with SMPTE bars.
	// Overlay text should place glyphs at the same positions as the hand-drawn logo.
	mbW, mbH := 48, 27
	scale := 2

	smpteGrid, smpteColors := SMPTEBarsGrid(mbW)
	// Tile SMPTE bars to full height
	chars := make([][]byte, mbH)
	for y := range mbH {
		chars[y] = make([]byte, mbW)
		copy(chars[y], smpteGrid.Chars[0])
	}
	tiledGrid := &Grid{Chars: chars, Width: mbW, Height: mbH}

	fg := Color{235, 128, 128} // white-ish
	tbg := Color{16, 128, 128} // black
	grid, _, err := OverlayText(tiledGrid, smpteColors, "hi264", scale, fg, &tbg)
	if err != nil {
		t.Fatal(err)
	}

	// Text "hi264": TextWidth = 19, scaled = 38
	// Centered: offsetX = (48-38)/2 = 5, offsetY = (27-10)/2 = 8
	// Box: (4,7) to (43,18)

	// Verify box border (row 7 should be all '@' from col 4 to 43)
	for x := 4; x <= 43; x++ {
		if grid.Chars[7][x] != '@' {
			t.Errorf("box border row 7, col %d = %q, want '@'", x, string(grid.Chars[7][x]))
		}
	}
	// Outside the box should be original SMPTE character
	if grid.Chars[7][3] == '@' || grid.Chars[7][3] == '#' {
		t.Errorf("col 3, row 7 should be SMPTE bar, got %q", string(grid.Chars[7][3]))
	}

	// Verify 'h' glyph at (5, 8) scale 2:
	// h row 0: #.. → cols 5-6 should be '#', cols 7-10 should be '@'
	if grid.Chars[8][5] != '#' || grid.Chars[8][6] != '#' {
		t.Error("h: top-left 2×2 block should be '#'")
	}
	if grid.Chars[8][7] != '@' {
		t.Errorf("h: col 7, row 8 should be '@', got %q", string(grid.Chars[8][7]))
	}

	// Verify 'i' glyph: starts at offsetX + 1*4*scale = 5 + 8 = 13
	// i row 0: .#. → cols 13-14='@', cols 15-16='#', cols 17-18='@'
	if grid.Chars[8][15] != '#' || grid.Chars[8][16] != '#' {
		t.Error("i: dot (row 0) should be '#' at cols 15-16")
	}
	if grid.Chars[8][13] != '@' || grid.Chars[8][14] != '@' {
		t.Error("i: row 0 cols 13-14 should be '@'")
	}
	// i row 1: ... → all '@' (blank row)
	for x := 13; x <= 18; x++ {
		if grid.Chars[10][x] != '@' {
			t.Errorf("i: blank row 1, col %d should be '@', got %q", x, string(grid.Chars[10][x]))
		}
	}

	// Verify '2' glyph starts at offsetX + 2*4*scale = 5 + 16 = 21
	// 2 row 0: ### → cols 21-26 all '#'
	for x := 21; x <= 26; x++ {
		if grid.Chars[8][x] != '#' {
			t.Errorf("2: top row, col %d should be '#', got %q", x, string(grid.Chars[8][x]))
		}
	}

	// Verify '4' glyph starts at offsetX + 4*4*scale = 5 + 32 = 37
	// 4 row 0: #.# → cols 37-38='#', 39-40='@', 41-42='#'
	if grid.Chars[8][37] != '#' || grid.Chars[8][38] != '#' {
		t.Error("4: top-left should be '#'")
	}
	if grid.Chars[8][39] != '@' || grid.Chars[8][40] != '@' {
		t.Error("4: top-center should be '@'")
	}
	if grid.Chars[8][41] != '#' || grid.Chars[8][42] != '#' {
		t.Error("4: top-right should be '#'")
	}
}
