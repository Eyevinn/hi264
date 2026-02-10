package yuv

import (
	"testing"
)

func TestBitmapDigitPixels(t *testing.T) {
	// Verify each digit renders the correct 3x5 bitmap pattern
	tests := []struct {
		digit int
		// expected foreground positions as (col,row) pairs
		fg [][2]int
	}{
		{0, [][2]int{{0, 0}, {1, 0}, {2, 0}, {0, 1}, {2, 1}, {0, 2}, {2, 2}, {0, 3}, {2, 3}, {0, 4}, {1, 4}, {2, 4}}},
		{1, [][2]int{{1, 0}, {1, 1}, {1, 2}, {1, 3}, {1, 4}}},
		{2, [][2]int{{0, 0}, {1, 0}, {2, 0}, {2, 1}, {0, 2}, {1, 2}, {2, 2}, {0, 3}, {0, 4}, {1, 4}, {2, 4}}},
		{3, [][2]int{{0, 0}, {1, 0}, {2, 0}, {2, 1}, {0, 2}, {1, 2}, {2, 2}, {2, 3}, {0, 4}, {1, 4}, {2, 4}}},
		{4, [][2]int{{0, 0}, {2, 0}, {0, 1}, {2, 1}, {0, 2}, {1, 2}, {2, 2}, {2, 3}, {2, 4}}},
		{5, [][2]int{{0, 0}, {1, 0}, {2, 0}, {0, 1}, {0, 2}, {1, 2}, {2, 2}, {2, 3}, {0, 4}, {1, 4}, {2, 4}}},
		{6, [][2]int{{0, 0}, {1, 0}, {2, 0}, {0, 1}, {0, 2}, {1, 2}, {2, 2}, {0, 3}, {2, 3}, {0, 4}, {1, 4}, {2, 4}}},
		{7, [][2]int{{0, 0}, {1, 0}, {2, 0}, {2, 1}, {2, 2}, {2, 3}, {2, 4}}},
		{8, [][2]int{
			{0, 0}, {1, 0}, {2, 0}, {0, 1}, {2, 1}, {0, 2}, {1, 2},
			{2, 2}, {0, 3}, {2, 3}, {0, 4}, {1, 4}, {2, 4},
		}},
		{9, [][2]int{{0, 0}, {1, 0}, {2, 0}, {0, 1}, {2, 1}, {0, 2}, {1, 2}, {2, 2}, {2, 3}, {0, 4}, {1, 4}, {2, 4}}},
	}

	for _, tc := range tests {
		fgSet := make(map[[2]int]bool)
		for _, p := range tc.fg {
			fgSet[p] = true
		}
		for row := range 5 {
			for col := range 3 {
				got := digitPixel(tc.digit, col, row)
				want := fgSet[[2]int{col, row}]
				if got != want {
					t.Errorf("digit %d pixel(%d,%d) = %v, want %v", tc.digit, col, row, got, want)
				}
			}
		}
	}
}

func TestCounterGridDimensions(t *testing.T) {
	// 11 MBs wide (3*3 + 2 gaps), 5 MBs tall for 3 digits
	mbW, mbH := 11, 5
	grid, colors, err := CounterGrid(0, 3, mbW, mbH, 1, Color{16, 128, 128}, Color{235, 128, 128}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if grid.Width != mbW || grid.Height != mbH {
		t.Errorf("grid size %dx%d, want %dx%d", grid.Width, grid.Height, mbW, mbH)
	}
	if len(colors) != 2 {
		t.Errorf("colormap has %d entries, want 2", len(colors))
	}
}

func TestCounterGridCentering(t *testing.T) {
	// 3 digits = 11 MBs wide, 5 tall
	// With 15 wide, 9 tall: offsetX = (15-11)/2 = 2, offsetY = (9-5)/2 = 2
	mbW, mbH := 15, 9
	bg := Color{16, 128, 128}
	fg := Color{235, 128, 128}
	grid, _, err := CounterGrid(0, 3, mbW, mbH, 1, bg, fg, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Check corners are background
	if grid.Chars[0][0] != '.' {
		t.Error("top-left should be background")
	}
	if grid.Chars[mbH-1][mbW-1] != '.' {
		t.Error("bottom-right should be background")
	}
}

func TestCounterGridTooSmall(t *testing.T) {
	_, _, err := CounterGrid(0, 3, 10, 5, 1, Color{}, Color{}, nil)
	if err == nil {
		t.Error("expected error for frame too small")
	}
}

func TestCounterGridWraps(t *testing.T) {
	// 2 digits: max 100, frameNum 105 should wrap to 5
	bg := Color{16, 128, 128}
	fg := Color{235, 128, 128}
	gridA, _, err := CounterGrid(5, 2, 7, 5, 1, bg, fg, nil)
	if err != nil {
		t.Fatal(err)
	}
	gridB, _, err := CounterGrid(105, 2, 7, 5, 1, bg, fg, nil)
	if err != nil {
		t.Fatal(err)
	}

	for y := range gridA.Height {
		for x := range gridA.Width {
			if gridA.Chars[y][x] != gridB.Chars[y][x] {
				t.Errorf("wrapped grid differs at (%d,%d): %c vs %c",
					x, y, gridA.Chars[y][x], gridB.Chars[y][x])
			}
		}
	}
}

func TestCounterGridDigitPatterns(t *testing.T) {
	// Test that digit "8" has all segments lit (maximum foreground)
	bg := Color{16, 128, 128}
	fg := Color{235, 128, 128}
	grid, _, err := CounterGrid(8, 1, 3, 5, 1, bg, fg, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Count foreground MBs for digit 8 (all bitmap pixels lit)
	fgCount := 0
	for y := range 5 {
		for x := range 3 {
			if grid.Chars[y][x] == '#' {
				fgCount++
			}
		}
	}
	if fgCount != 13 {
		t.Errorf("digit 8 has %d foreground MBs, want 13", fgCount)
	}

	// Test digit "1" has 5 foreground MBs (center column)
	grid, _, err = CounterGrid(1, 1, 3, 5, 1, bg, fg, nil)
	if err != nil {
		t.Fatal(err)
	}
	fgCount = 0
	for y := range 5 {
		for x := range 3 {
			if grid.Chars[y][x] == '#' {
				fgCount++
			}
		}
	}
	if fgCount != 5 {
		t.Errorf("digit 1 has %d foreground MBs, want 5", fgCount)
	}
}

func TestAutoDigitScale(t *testing.T) {
	tests := []struct {
		numDigits, mbW, mbH int
		wantScale           int
	}{
		// 3 digits: digitW = S*(4*3-1) = 11S, digitH = 5S
		// With border: 11S+2 ≤ mbW, 5S+2 ≤ mbH
		{3, 11, 5, 1},     // exact fit at S=1 (no border room), S=1
		{3, 13, 7, 1},     // 11*1+2=13 ≤ 13, 5*1+2=7 ≤ 7 → S=1; S=2: 22+2=24>13
		{3, 24, 12, 2},    // 11*2+2=24 ≤ 24, 5*2+2=12 ≤ 12 → S=2
		{3, 35, 17, 3},    // 11*3+2=35 ≤ 35, 5*3+2=17 ≤ 17 → S=3
		{2, 16, 12, 2},    // 7*2+2=16 ≤ 16, 5*2+2=12 ≤ 12 → S=2
		{1, 5, 7, 1},      // 3*1+2=5 ≤ 5, 5*1+2=7 ≤ 7 → S=1
		{1, 100, 100, 19}, // height-limited: 5*19+2=97 ≤ 100, 5*20+2=102>100
	}
	for _, tc := range tests {
		got := AutoDigitScale(tc.numDigits, tc.mbW, tc.mbH)
		if got != tc.wantScale {
			t.Errorf("AutoDigitScale(%d, %d, %d) = %d, want %d",
				tc.numDigits, tc.mbW, tc.mbH, got, tc.wantScale)
		}
	}
}

func TestCounterGridScale2(t *testing.T) {
	// 1 digit at scale 2: 6×10 MBs, in a 10×14 frame → centered at (2,2)
	bg := Color{16, 128, 128}
	fg := Color{235, 128, 128}
	grid, _, err := CounterGrid(8, 1, 10, 14, 2, bg, fg, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Digit "8" at scale 2: each bitmap pixel is 2×2 MBs
	// 3*2=6 wide, 5*2=10 tall, centered at offsetX=(10-6)/2=2, offsetY=(14-10)/2=2
	// Count foreground: digit 8 has 13 bitmap pixels × 4 MBs each = 52
	fgCount := 0
	for y := range grid.Height {
		for x := range grid.Width {
			if grid.Chars[y][x] == '#' {
				fgCount++
			}
		}
	}
	if fgCount != 52 {
		t.Errorf("digit 8 at scale 2 has %d foreground MBs, want 52", fgCount)
	}
}

func TestCounterGridDigitBg(t *testing.T) {
	// 1 digit at scale 1 in 7×9 frame: digit 3×5 centered at (2,2)
	// Box: (1,1) to (5,7) = 5×7 = 35 cells
	bg := Color{16, 128, 128}
	fg := Color{235, 128, 128}
	dbg := Color{80, 128, 128}
	grid, colors, err := CounterGrid(0, 1, 7, 9, 1, bg, fg, &dbg)
	if err != nil {
		t.Fatal(err)
	}

	// Count '@' cells (box minus digit pixels)
	atCount := 0
	dotCount := 0
	hashCount := 0
	for y := range grid.Height {
		for x := range grid.Width {
			switch grid.Chars[y][x] {
			case '@':
				atCount++
			case '.':
				dotCount++
			case '#':
				hashCount++
			}
		}
	}

	// Digit "0" has 12 fg pixels, box is 5×7=35, so '@' = 35-12 = 23
	if hashCount != 12 {
		t.Errorf("digit 0: %d fg MBs, want 12", hashCount)
	}
	if atCount != 23 {
		t.Errorf("digit 0: %d '@' MBs, want 23", atCount)
	}

	// ColorMap should have 3 entries: '.', '#', '@'
	if len(colors) != 3 {
		t.Errorf("colormap has %d entries, want 3", len(colors))
	}
	if colors['@'] != dbg {
		t.Errorf("'@' color = %v, want %v", colors['@'], dbg)
	}
}

func TestSMPTEBarsGrid(t *testing.T) {
	// Default 7-column case: one macroblock per bar.
	grid, colors := SMPTEBarsGrid(7)

	if grid.Width != 7 || grid.Height != 1 {
		t.Errorf("SMPTE grid size %dx%d, want 7x1", grid.Width, grid.Height)
	}
	if len(colors) != 7 {
		t.Errorf("SMPTE colormap has %d entries, want 7", len(colors))
	}

	// Check that all expected characters are present
	expected := "WYCGMRB"
	for i, ch := range expected {
		if grid.Chars[0][i] != byte(ch) {
			t.Errorf("SMPTE bar %d = %c, want %c", i, grid.Chars[0][i], ch)
		}
		if _, ok := colors[byte(ch)]; !ok {
			t.Errorf("SMPTE color missing for %c", ch)
		}
	}

	// Verify white bar is brightest (Y component should be highest)
	white := colors['W']
	blue := colors['B']
	if white.Y <= blue.Y {
		t.Errorf("white Y=%d should be greater than blue Y=%d", white.Y, blue.Y)
	}

	// Test even distribution for 80 macroblocks (1280px).
	// 80 / 7 = 11 remainder 3, so first 3 bars get 12, rest get 11.
	grid80, _ := SMPTEBarsGrid(80)
	if grid80.Width != 80 {
		t.Fatalf("SMPTE grid width %d, want 80", grid80.Width)
	}
	barWidths := make(map[byte]int)
	for _, ch := range grid80.Chars[0] {
		barWidths[ch]++
	}
	for i, ch := range expected {
		want := 11
		if i < 3 {
			want = 12
		}
		if barWidths[byte(ch)] != want {
			t.Errorf("bar %c width %d, want %d", ch, barWidths[byte(ch)], want)
		}
	}
}
