package yuv

import (
	"fmt"
	"maps"
)

// Bitmap font for digits 0-9, each 3 wide x 5 tall MBs.
// digitBitmaps[digit][row][col] = true means foreground.
var digitBitmaps = [10][5][3]bool{
	// 0: ###  #.#  #.#  #.#  ###
	{{true, true, true}, {true, false, true}, {true, false, true}, {true, false, true}, {true, true, true}},
	// 1: .#.  .#.  .#.  .#.  .#.
	{{false, true, false}, {false, true, false}, {false, true, false}, {false, true, false}, {false, true, false}},
	// 2: ###  ..#  ###  #..  ###
	{{true, true, true}, {false, false, true}, {true, true, true}, {true, false, false}, {true, true, true}},
	// 3: ###  ..#  ###  ..#  ###
	{{true, true, true}, {false, false, true}, {true, true, true}, {false, false, true}, {true, true, true}},
	// 4: #.#  #.#  ###  ..#  ..#
	{{true, false, true}, {true, false, true}, {true, true, true}, {false, false, true}, {false, false, true}},
	// 5: ###  #..  ###  ..#  ###
	{{true, true, true}, {true, false, false}, {true, true, true}, {false, false, true}, {true, true, true}},
	// 6: ###  #..  ###  #.#  ###
	{{true, true, true}, {true, false, false}, {true, true, true}, {true, false, true}, {true, true, true}},
	// 7: ###  ..#  ..#  ..#  ..#
	{{true, true, true}, {false, false, true}, {false, false, true}, {false, false, true}, {false, false, true}},
	// 8: ###  #.#  ###  #.#  ###
	{{true, true, true}, {true, false, true}, {true, true, true}, {true, false, true}, {true, true, true}},
	// 9: ###  #.#  ###  ..#  ###
	{{true, true, true}, {true, false, true}, {true, true, true}, {false, false, true}, {true, true, true}},
}

// digitPixel returns true if the given position within a 3x5 MB digit grid
// should be foreground for the given digit value (0-9).
func digitPixel(digit, col, row int) bool {
	return digitBitmaps[digit][row][col]
}

// AutoDigitScale returns the largest integer scale S where the scaled digits
// plus a 1-MB border fit within the given frame dimensions.
// Digit area at scale S: width = S*(4*numDigits-1), height = 5*S.
// With border: width+2, height+2 must fit within mbWidth, mbHeight.
// Returns at least 1.
func AutoDigitScale(numDigits, mbWidth, mbHeight int) int {
	s := 1
	for {
		next := s + 1
		w := next*(4*numDigits-1) + 2
		h := 5*next + 2
		if w > mbWidth || h > mbHeight {
			break
		}
		s = next
	}
	return s
}

// CounterGrid generates a grid with 7-segment counter digits centered in the frame.
// frameNum is the number to display, numDigits is how many digits to show (zero-padded),
// mbWidth and mbHeight are the frame dimensions in macroblocks,
// scale is an integer multiplier (each bitmap pixel becomes scale×scale MBs),
// bg and fg are the background and foreground colors,
// digitBg, if non-nil, draws a solid box behind the digits using '@'.
func CounterGrid(frameNum, numDigits, mbWidth, mbHeight, scale int,
	bg, fg Color, digitBg *Color) (*Grid, ColorMap, error) {

	// Digit area: each digit is 3*S MBs wide, with S MB gap between digits
	digitAreaWidth := scale * (3*numDigits + (numDigits - 1)) // S*(4n-1)
	digitAreaHeight := 5 * scale

	if digitAreaWidth > mbWidth || digitAreaHeight > mbHeight {
		return nil, nil, fmt.Errorf("frame %dx%d MBs too small for %d digits at scale %d (need %dx%d)",
			mbWidth, mbHeight, numDigits, scale, digitAreaWidth, digitAreaHeight)
	}

	// Wrap frameNum via modulo
	maxVal := 1
	for range numDigits {
		maxVal *= 10
	}
	frameNum = frameNum % maxVal

	// Extract individual digits
	digits := make([]int, numDigits)
	val := frameNum
	for i := numDigits - 1; i >= 0; i-- {
		digits[i] = val % 10
		val /= 10
	}

	// Center the digit area in the frame
	offsetX := (mbWidth - digitAreaWidth) / 2
	offsetY := (mbHeight - digitAreaHeight) / 2

	// Build the grid
	chars := make([][]byte, mbHeight)
	for y := range mbHeight {
		chars[y] = make([]byte, mbWidth)
		for x := range mbWidth {
			chars[y][x] = '.'
		}
	}

	// Draw digit background box if requested
	if digitBg != nil {
		// Box = digit area + 1 MB border on each side
		boxX := offsetX - 1
		boxY := offsetY - 1
		boxW := digitAreaWidth + 2
		boxH := digitAreaHeight + 2
		// Only draw if box fits
		if boxX >= 0 && boxY >= 0 && boxX+boxW <= mbWidth && boxY+boxH <= mbHeight {
			for y := boxY; y < boxY+boxH; y++ {
				for x := boxX; x < boxX+boxW; x++ {
					chars[y][x] = '@'
				}
			}
		}
	}

	// Fill in digit pixels (scaled)
	for d := range numDigits {
		dx := offsetX + d*4*scale // each digit takes 3*S cols + S gap
		for row := range 5 {
			for col := range 3 {
				if digitPixel(digits[d], col, row) {
					// Fill scale×scale block
					for sy := range scale {
						for sx := range scale {
							chars[offsetY+row*scale+sy][dx+col*scale+sx] = '#'
						}
					}
				}
			}
		}
	}

	colorMap := ColorMap{
		'.': bg,
		'#': fg,
	}
	if digitBg != nil {
		colorMap['@'] = *digitBg
	}

	grid := &Grid{
		Chars:  chars,
		Width:  mbWidth,
		Height: mbHeight,
	}

	return grid, colorMap, nil
}

// TileBackground replaces background cells ('.') in a counter grid with
// tiled characters from a pattern grid. Foreground cells ('#') and digit
// background cells ('@') are preserved.
// The pattern is tiled (repeated) to fill the full frame dimensions.
// The returned ColorMap merges pattern colors with the foreground and digit-bg colors.
func TileBackground(grid *Grid, colors ColorMap, pattern *Grid, patternColors ColorMap) (*Grid, ColorMap, error) {
	// Validate pattern doesn't use reserved characters
	for ch := range patternColors {
		if ch == '#' {
			return nil, nil, fmt.Errorf("pattern must not use '#' character (reserved for foreground digits)")
		}
		if ch == '@' {
			return nil, nil, fmt.Errorf("pattern must not use '@' character (reserved for digit background)")
		}
	}

	// Build new grid with tiled pattern replacing background
	chars := make([][]byte, grid.Height)
	for y := range grid.Height {
		chars[y] = make([]byte, grid.Width)
		for x := range grid.Width {
			if grid.Chars[y][x] == '.' {
				chars[y][x] = pattern.Chars[y%pattern.Height][x%pattern.Width]
			} else {
				chars[y][x] = grid.Chars[y][x]
			}
		}
	}

	// Merge color maps: pattern colors + non-'.' colors from original
	merged := make(ColorMap)
	maps.Copy(merged, patternColors)
	for ch, c := range colors {
		if ch != '.' {
			merged[ch] = c
		}
	}

	return &Grid{Chars: chars, Width: grid.Width, Height: grid.Height}, merged, nil
}

// SMPTEBarsGrid returns a grid with 7 SMPTE 75% color bars distributed
// evenly across mbCols macroblocks. Each bar gets mbCols/7 columns,
// with the remainder distributed one extra column to the first bars.
// Colors are: White, Yellow, Cyan, Green, Magenta, Red, Blue.
// Values are BT.601 limited-range YCbCr converted from 75% RGB.
func SMPTEBarsGrid(mbCols int) (*Grid, ColorMap) {
	barChars := []byte{'W', 'Y', 'C', 'G', 'M', 'R', 'B'}
	colors := ColorMap{
		'W': RGBToYCbCr(191, 191, 191),
		'Y': RGBToYCbCr(191, 191, 0),
		'C': RGBToYCbCr(0, 191, 191),
		'G': RGBToYCbCr(0, 191, 0),
		'M': RGBToYCbCr(191, 0, 191),
		'R': RGBToYCbCr(191, 0, 0),
		'B': RGBToYCbCr(0, 0, 191),
	}
	if mbCols < 7 {
		mbCols = 7
	}
	row := make([]byte, mbCols)
	base := mbCols / 7
	extra := mbCols % 7
	pos := 0
	for i, ch := range barChars {
		w := base
		if i < extra {
			w++
		}
		for j := 0; j < w; j++ {
			row[pos] = ch
			pos++
		}
	}
	return &Grid{Chars: [][]byte{row}, Width: mbCols, Height: 1}, colors
}
