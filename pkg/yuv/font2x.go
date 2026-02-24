package yuv

import (
	"fmt"
	"maps"
	"strings"
)

// Glyph2x is a 6-wide × 10-tall bitmap for 8x8-block-resolution text.
// Same pixel area as Glyph (3×5 at 16x16): 48×80 pixels.
type Glyph2x [10][6]bool

// glyphs2x maps printable ASCII bytes to their 6×10 bitmap definitions.
var glyphs2x map[byte]Glyph2x

func init() {
	// Each glyph is a 60-character string (10 rows × 6 cols).
	// '#' = foreground pixel, anything else = background.
	defs := map[byte]string{
		' ': "......" + "......" + "......" + "......" + "......" +
			"......" + "......" + "......" + "......" + "......",

		'0': ".####." + "##..##" + "##..##" + "##..##" + "##..##" +
			"##..##" + "##..##" + "##..##" + "##..##" + ".####.",
		'1': "..##.." + ".###.." + "..##.." + "..##.." + "..##.." +
			"..##.." + "..##.." + "..##.." + "..##.." + ".####.",
		'2': ".####." + "##..##" + "....##" + "...##." + "..##.." +
			".##..." + "##...." + "##...." + "######" + "######",
		'3': ".####." + "##..##" + "....##" + "....##" + "..###." +
			"....##" + "....##" + "....##" + "##..##" + ".####.",
		'4': "....##" + "...###" + "..####" + ".##.##" + "##..##" +
			"######" + "....##" + "....##" + "....##" + "....##",
		'5': "######" + "######" + "##...." + "##...." + "#####." +
			"....##" + "....##" + "....##" + "##..##" + ".####.",
		'6': ".####." + "##..##" + "##...." + "##...." + "#####." +
			"##..##" + "##..##" + "##..##" + "##..##" + ".####.",
		'7': "######" + "######" + "....##" + "...##." + "..##.." +
			"..##.." + ".##..." + ".##..." + ".##..." + ".##...",
		'8': ".####." + "##..##" + "##..##" + "##..##" + ".####." +
			"##..##" + "##..##" + "##..##" + "##..##" + ".####.",
		'9': ".####." + "##..##" + "##..##" + "##..##" + ".#####" +
			"....##" + "....##" + "....##" + "##..##" + ".####.",

		'A': ".####." + "##..##" + "##..##" + "##..##" + "######" +
			"##..##" + "##..##" + "##..##" + "##..##" + "##..##",
		'B': "#####." + "##..##" + "##..##" + "##..##" + "#####." +
			"##..##" + "##..##" + "##..##" + "##..##" + "#####.",
		'C': ".####." + "##..##" + "##...." + "##...." + "##...." +
			"##...." + "##...." + "##...." + "##..##" + ".####.",
		'D': "#####." + "##..##" + "##..##" + "##..##" + "##..##" +
			"##..##" + "##..##" + "##..##" + "##..##" + "#####.",
		'E': "######" + "######" + "##...." + "##...." + "#####." +
			"##...." + "##...." + "##...." + "######" + "######",
		'F': "######" + "######" + "##...." + "##...." + "#####." +
			"##...." + "##...." + "##...." + "##...." + "##....",
		'G': ".####." + "##..##" + "##...." + "##...." + "##.###" +
			"##..##" + "##..##" + "##..##" + "##..##" + ".####.",
		'H': "##..##" + "##..##" + "##..##" + "##..##" + "######" +
			"##..##" + "##..##" + "##..##" + "##..##" + "##..##",
		'I': ".####." + "..##.." + "..##.." + "..##.." + "..##.." +
			"..##.." + "..##.." + "..##.." + "..##.." + ".####.",
		'J': "...###" + "....##" + "....##" + "....##" + "....##" +
			"....##" + "....##" + "....##" + "##..##" + ".####.",
		'K': "##..##" + "##..##" + "##.##." + "####.." + "###..." +
			"####.." + "##.##." + "##..##" + "##..##" + "##..##",
		'L': "##...." + "##...." + "##...." + "##...." + "##...." +
			"##...." + "##...." + "##...." + "######" + "######",
		'M': "##..##" + "######" + "######" + "##.###" + "##..##" +
			"##..##" + "##..##" + "##..##" + "##..##" + "##..##",
		'N': "##..##" + "###.##" + "######" + "##.###" + "##..##" +
			"##..##" + "##..##" + "##..##" + "##..##" + "##..##",
		'O': ".####." + "##..##" + "##..##" + "##..##" + "##..##" +
			"##..##" + "##..##" + "##..##" + "##..##" + ".####.",
		'P': "#####." + "##..##" + "##..##" + "##..##" + "#####." +
			"##...." + "##...." + "##...." + "##...." + "##....",
		'Q': ".####." + "##..##" + "##..##" + "##..##" + "##..##" +
			"##..##" + "##..##" + "##.###" + "##..##" + ".#####",
		'R': "#####." + "##..##" + "##..##" + "##..##" + "#####." +
			"##.##." + "##..##" + "##..##" + "##..##" + "##..##",
		'S': ".####." + "##..##" + "##...." + ".##..." + "..##.." +
			"...##." + "....##" + "....##" + "##..##" + ".####.",
		'T': "######" + "######" + "..##.." + "..##.." + "..##.." +
			"..##.." + "..##.." + "..##.." + "..##.." + "..##..",
		'U': "##..##" + "##..##" + "##..##" + "##..##" + "##..##" +
			"##..##" + "##..##" + "##..##" + "##..##" + ".####.",
		'V': "##..##" + "##..##" + "##..##" + "##..##" + "##..##" +
			"##..##" + "##..##" + ".####." + "..##.." + "..##..",
		'W': "##..##" + "##..##" + "##..##" + "##..##" + "##..##" +
			"##..##" + "##.###" + "######" + "######" + "##..##",
		'X': "##..##" + "##..##" + ".####." + "..##.." + "..##.." +
			"..##.." + "..##.." + ".####." + "##..##" + "##..##",
		'Y': "##..##" + "##..##" + "##..##" + ".####." + "..##.." +
			"..##.." + "..##.." + "..##.." + "..##.." + "..##..",
		'Z': "######" + "######" + "....##" + "...##." + "..##.." +
			".##..." + "##...." + "##...." + "######" + "######",

		// Common punctuation
		':': "......" + "......" + "..##.." + "..##.." + "......" +
			"......" + "..##.." + "..##.." + "......" + "......",
		'.': "......" + "......" + "......" + "......" + "......" +
			"......" + "......" + "......" + "..##.." + "..##..",
		'-': "......" + "......" + "......" + "......" + "######" +
			"######" + "......" + "......" + "......" + "......",
		'/': "....##" + "....##" + "...##." + "...##." + "..##.." +
			"..##.." + ".##..." + ".##..." + "##...." + "##....",
		'!': "..##.." + "..##.." + "..##.." + "..##.." + "..##.." +
			"..##.." + "..##.." + "......" + "..##.." + "..##..",
		'?': ".####." + "##..##" + "....##" + "...##." + "..##.." +
			"..##.." + "..##.." + "......" + "..##.." + "..##..",
		'%': "##..##" + "##.##." + "..##.." + "..##.." + ".##..." +
			".##..." + "..##.." + "..##.." + ".##.##" + "##..##",
		'+': "......" + "......" + "..##.." + "..##.." + "######" +
			"######" + "..##.." + "..##.." + "......" + "......",
		'=': "......" + "......" + "######" + "######" + "......" +
			"......" + "######" + "######" + "......" + "......",
		'_': "......" + "......" + "......" + "......" + "......" +
			"......" + "......" + "......" + "######" + "######",
		'(': "..##.." + ".##..." + "##...." + "##...." + "##...." +
			"##...." + "##...." + "##...." + ".##..." + "..##..",
		')': "..##.." + "...##." + "....##" + "....##" + "....##" +
			"....##" + "....##" + "....##" + "...##." + "..##..",
		'[': ".####." + ".##..." + ".##..." + ".##..." + ".##..." +
			".##..." + ".##..." + ".##..." + ".##..." + ".####.",
		']': ".####." + "...##." + "...##." + "...##." + "...##." +
			"...##." + "...##." + "...##." + "...##." + ".####.",
		'#': ".#..#." + ".#..#." + "######" + ".#..#." + ".#..#." +
			".#..#." + "######" + ".#..#." + ".#..#." + "......",
	}

	glyphs2x = make(map[byte]Glyph2x, len(defs))
	for ch, s := range defs {
		if len(s) != 60 {
			panic(fmt.Sprintf("glyph2x %q has %d chars, want 60", string(ch), len(s)))
		}
		var g Glyph2x
		for i := range 60 {
			g[i/6][i%6] = s[i] == '#'
		}
		glyphs2x[ch] = g
	}
}

// GlyphPixel2x returns whether (col, row) is foreground for ch in the 6×10 font.
func GlyphPixel2x(ch byte, col, row int) bool {
	g, ok := glyphs2x[ch]
	if !ok {
		return false
	}
	return g[row][col]
}

// HasGlyph2x reports whether a 6×10 glyph exists for ch.
func HasGlyph2x(ch byte) bool {
	_, ok := glyphs2x[ch]
	return ok
}

const (
	glyph2xW   = 6  // glyph width in blocks
	glyph2xH   = 10 // glyph height in blocks
	glyph2xGap = 1  // gap between characters in blocks
	glyph2xLG  = 2  // gap between lines in blocks
)

// lineWidth2x returns the width in blocks at scale 1 for a single line.
func lineWidth2x(line string) int {
	if len(line) == 0 {
		return 0
	}
	return (glyph2xW+glyph2xGap)*len(line) - glyph2xGap
}

// TextWidth2x returns the width in 8x8 blocks at scale 1 for the widest line.
func TextWidth2x(text string) int {
	w := 0
	for _, line := range textLines(text) {
		if lw := lineWidth2x(line); lw > w {
			w = lw
		}
	}
	return w
}

// TextHeight2x returns the height in 8x8 blocks at scale 1.
func TextHeight2x(text string) int {
	lines := textLines(text)
	n := len(lines)
	if n == 0 {
		return 0
	}
	return glyph2xH*n + glyph2xLG*(n-1)
}

// AutoTextScale2x returns the largest integer scale S where the scaled 2x text
// plus a 2-block border fits within the given frame dimensions (in 8x8 block units).
func AutoTextScale2x(text string, blockW, blockH int) int {
	tw := TextWidth2x(text)
	th := TextHeight2x(text)
	if tw == 0 || th == 0 {
		return 1
	}
	s := 1
	for {
		next := s + 1
		w := next*tw + 4 // 2-block border on each side
		h := next*th + 4
		if w > blockW || h > blockH {
			break
		}
		s = next
	}
	return s
}

// renderText2x stamps 6×10 glyph pixels onto chars, centered in blockW×blockH.
func renderText2x(chars [][]byte, blockW, blockH int, text string, scale int, textBg bool) {
	tw := TextWidth2x(text)
	th := TextHeight2x(text)
	textAreaW := scale * tw
	textAreaH := scale * th

	offsetX := (blockW - textAreaW) / 2
	offsetY := (blockH - textAreaH) / 2

	if textBg {
		boxX := offsetX - 2
		boxY := offsetY - 2
		boxW := textAreaW + 4
		boxH := textAreaH + 4
		if boxX >= 0 && boxY >= 0 && boxX+boxW <= blockW && boxY+boxH <= blockH {
			for y := boxY; y < boxY+boxH; y++ {
				for x := boxX; x < boxX+boxW; x++ {
					chars[y][x] = '@'
				}
			}
		}
	}

	lines := textLines(text)
	lineY := offsetY
	for _, line := range lines {
		lw := lineWidth2x(line)
		lineOffsetX := offsetX + (textAreaW-scale*lw)/2
		for i := 0; i < len(line); i++ {
			g, ok := glyphs2x[line[i]]
			if !ok {
				continue
			}
			dx := lineOffsetX + i*(glyph2xW+glyph2xGap)*scale
			for row := range glyph2xH {
				for col := range glyph2xW {
					if g[row][col] {
						for sy := range scale {
							for sx := range scale {
								chars[lineY+row*scale+sy][dx+col*scale+sx] = '#'
							}
						}
					}
				}
			}
		}
		lineY += (glyph2xH + glyph2xLG) * scale
	}
}

// TextGrid2x creates a grid with 6×10 text centered, at 8x8 block resolution.
func TextGrid2x(text string, blockW, blockH, scale int,
	bg, fg Color, textBg *Color) (*Grid, ColorMap, error) {

	text = strings.ToUpper(text)
	tw := TextWidth2x(text)
	th := TextHeight2x(text)
	textAreaW := scale * tw
	textAreaH := scale * th

	if textAreaW > blockW || textAreaH > blockH {
		return nil, nil, fmt.Errorf("frame %dx%d blocks too small for text %q at scale %d (need %dx%d)",
			blockW, blockH, text, scale, textAreaW, textAreaH)
	}

	chars := make([][]byte, blockH)
	for y := range blockH {
		chars[y] = make([]byte, blockW)
		for x := range blockW {
			chars[y][x] = '.'
		}
	}

	renderText2x(chars, blockW, blockH, text, scale, textBg != nil)

	colorMap := ColorMap{
		'.': bg,
		'#': fg,
	}
	if textBg != nil {
		colorMap['@'] = *textBg
	}

	return &Grid{Chars: chars, Width: blockW, Height: blockH}, colorMap, nil
}

// OverlayText2x renders 6×10 text onto an existing grid at 8x8 block resolution.
func OverlayText2x(grid *Grid, colors ColorMap, text string, scale int,
	fg Color, textBg *Color) (*Grid, ColorMap, error) {

	text = strings.ToUpper(text)
	tw := TextWidth2x(text)
	th := TextHeight2x(text)
	textAreaW := scale * tw
	textAreaH := scale * th

	if textAreaW > grid.Width || textAreaH > grid.Height {
		return nil, nil, fmt.Errorf("frame %dx%d blocks too small for text %q at scale %d (need %dx%d)",
			grid.Width, grid.Height, text, scale, textAreaW, textAreaH)
	}

	chars := make([][]byte, grid.Height)
	for y := range grid.Height {
		chars[y] = make([]byte, grid.Width)
		copy(chars[y], grid.Chars[y])
	}

	renderText2x(chars, grid.Width, grid.Height, text, scale, textBg != nil)

	merged := make(ColorMap)
	maps.Copy(merged, colors)
	merged['#'] = fg
	if textBg != nil {
		merged['@'] = *textBg
	}

	return &Grid{Chars: chars, Width: grid.Width, Height: grid.Height}, merged, nil
}
