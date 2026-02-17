package yuv

import (
	"fmt"
	"maps"
	"strings"
)

// Glyph is a 3-wide × 5-tall bitmap. [row][col], true = foreground.
type Glyph [5][3]bool

// glyphs maps printable ASCII bytes to their bitmap definitions.
var glyphs map[byte]Glyph

func init() {
	// Each glyph is a 15-character string (5 rows × 3 cols).
	// '#' = foreground pixel, anything else = background.
	defs := map[byte]string{
		// Space
		' ': "..." + "..." + "..." + "..." + "...",

		// Digits (matching digitBitmaps exactly)
		'0': "###" + "#.#" + "#.#" + "#.#" + "###",
		'1': ".#." + ".#." + ".#." + ".#." + ".#.",
		'2': "###" + "..#" + "###" + "#.." + "###",
		'3': "###" + "..#" + "###" + "..#" + "###",
		'4': "#.#" + "#.#" + "###" + "..#" + "..#",
		'5': "###" + "#.." + "###" + "..#" + "###",
		'6': "###" + "#.." + "###" + "#.#" + "###",
		'7': "###" + "..#" + "..#" + "..#" + "..#",
		'8': "###" + "#.#" + "###" + "#.#" + "###",
		'9': "###" + "#.#" + "###" + "..#" + "###",

		// Uppercase A-Z
		'A': ".#." + "#.#" + "###" + "#.#" + "#.#",
		'B': "##." + "#.#" + "##." + "#.#" + "##.",
		'C': ".##" + "#.." + "#.." + "#.." + ".##",
		'D': "##." + "#.#" + "#.#" + "#.#" + "##.",
		'E': "###" + "#.." + "###" + "#.." + "###",
		'F': "###" + "#.." + "##." + "#.." + "#..",
		'G': ".##" + "#.." + "#.#" + "#.#" + ".##",
		'H': "#.#" + "#.#" + "###" + "#.#" + "#.#",
		'I': "###" + ".#." + ".#." + ".#." + "###",
		'J': "..#" + "..#" + "..#" + "#.#" + ".#.",
		'K': "#.#" + "#.#" + "##." + "#.#" + "#.#",
		'L': "#.." + "#.." + "#.." + "#.." + "###",
		'M': "#.#" + "###" + "#.#" + "#.#" + "#.#",
		'N': "#.#" + "###" + "###" + "#.#" + "#.#",
		'O': ".#." + "#.#" + "#.#" + "#.#" + ".#.",
		'P': "##." + "#.#" + "##." + "#.." + "#..",
		'Q': ".#." + "#.#" + "#.#" + "#.#" + ".##",
		'R': "##." + "#.#" + "##." + "#.#" + "#.#",
		'S': ".##" + "#.." + ".#." + "..#" + "##.",
		'T': "###" + ".#." + ".#." + ".#." + ".#.",
		'U': "#.#" + "#.#" + "#.#" + "#.#" + ".#.",
		'V': "#.#" + "#.#" + "#.#" + ".#." + ".#.",
		'W': "#.#" + "#.#" + "###" + "###" + "#.#",
		'X': "#.#" + "#.#" + ".#." + "#.#" + "#.#",
		'Y': "#.#" + "#.#" + ".#." + ".#." + ".#.",
		'Z': "###" + "..#" + ".#." + "#.." + "###",

		// Lowercase a-z (distinct from uppercase where feasible)
		'a': "..." + "..." + ".##" + "#.#" + ".##",
		'b': "#.." + "#.." + "##." + "#.#" + "##.",
		'c': "..." + "..." + ".##" + "#.." + ".##",
		'd': "..#" + "..#" + ".##" + "#.#" + ".##",
		'e': "..." + ".#." + "###" + "#.." + ".##",
		'f': ".##" + "#.." + "###" + "#.." + "#..",
		'g': "..." + ".#." + "#.#" + ".##" + "##.",
		'h': "#.." + "#.." + "###" + "#.#" + "#.#", // matches logo
		'i': ".#." + "..." + ".#." + ".#." + ".#.", // matches logo
		'j': ".#." + "..." + ".#." + ".#." + "#..",
		'k': "#.." + "#.." + "#.#" + "##." + "#.#",
		'l': "#.." + "#.." + "#.." + "#.." + "##.",
		'm': "..." + "..." + "#.#" + "###" + "#.#",
		'n': "..." + "..." + "##." + "#.#" + "#.#",
		'o': "..." + "..." + ".#." + "#.#" + ".#.",
		'p': "..." + "##." + "#.#" + "##." + "#..",
		'q': "..." + ".##" + "#.#" + ".##" + "..#",
		'r': "..." + "..." + ".##" + "#.." + "#..",
		's': "..." + "..." + ".##" + ".#." + "##.",
		't': ".#." + "###" + ".#." + ".#." + "..#",
		'u': "..." + "..." + "#.#" + "#.#" + ".##",
		'v': "..." + "..." + "#.#" + "#.#" + ".#.",
		'w': "..." + "..." + "#.#" + "###" + ".#.",
		'x': "..." + "..." + "#.#" + ".#." + "#.#",
		'y': "..." + "#.#" + "#.#" + ".##" + "##.",
		'z': "..." + "..." + "###" + ".#." + "###",

		// Punctuation and symbols
		'!':  ".#." + ".#." + ".#." + "..." + ".#.",
		'"':  "#.#" + "#.#" + "..." + "..." + "...",
		'#':  "#.#" + "###" + "#.#" + "###" + "#.#",
		'$':  ".#." + ".##" + ".#." + "##." + ".#.",
		'%':  "#.#" + "..#" + ".#." + "#.." + "#.#",
		'&':  ".#." + "#.#" + ".#." + "#.#" + ".##",
		'\'': ".#." + ".#." + "..." + "..." + "...",
		'(':  ".#." + "#.." + "#.." + "#.." + ".#.",
		')':  ".#." + "..#" + "..#" + "..#" + ".#.",
		'*':  "..." + "#.#" + ".#." + "#.#" + "...",
		'+':  "..." + ".#." + "###" + ".#." + "...",
		',':  "..." + "..." + "..." + ".#." + "#..",
		'-':  "..." + "..." + "###" + "..." + "...",
		'.':  "..." + "..." + "..." + "..." + ".#.",
		'/':  "..#" + "..#" + ".#." + "#.." + "#..",
		':':  "..." + ".#." + "..." + ".#." + "...",
		';':  "..." + ".#." + "..." + ".#." + "#..",
		'<':  "..#" + ".#." + "#.." + ".#." + "..#",
		'=':  "..." + "###" + "..." + "###" + "...",
		'>':  "#.." + ".#." + "..#" + ".#." + "#..",
		'?':  "###" + "..#" + ".#." + "..." + ".#.",
		'@':  ".##" + "#.#" + "###" + "#.." + ".##",
		'[':  "##." + "#.." + "#.." + "#.." + "##.",
		'\\': "#.." + "#.." + ".#." + "..#" + "..#",
		']':  ".##" + "..#" + "..#" + "..#" + ".##",
		'^':  ".#." + "#.#" + "..." + "..." + "...",
		'_':  "..." + "..." + "..." + "..." + "###",
		'`':  "#.." + ".#." + "..." + "..." + "...",
		'{':  ".##" + ".#." + "##." + ".#." + ".##",
		'|':  ".#." + ".#." + ".#." + ".#." + ".#.",
		'}':  "##." + ".#." + ".##" + ".#." + "##.",
		'~':  "..." + "..." + ".##" + "##." + "...",
	}

	glyphs = make(map[byte]Glyph, len(defs))
	for ch, s := range defs {
		if len(s) != 15 {
			panic(fmt.Sprintf("glyph %q has %d chars, want 15", string(ch), len(s)))
		}
		var g Glyph
		for i := range 15 {
			g[i/3][i%3] = s[i] == '#'
		}
		glyphs[ch] = g
	}
}

// GlyphPixel returns whether (col, row) is foreground for ch.
// Unsupported characters are treated as space (always false).
func GlyphPixel(ch byte, col, row int) bool {
	g, ok := glyphs[ch]
	if !ok {
		return false
	}
	return g[row][col]
}

// HasGlyph reports whether a glyph exists for ch.
func HasGlyph(ch byte) bool {
	_, ok := glyphs[ch]
	return ok
}

// textLines splits text on newlines and returns the lines.
func textLines(text string) []string {
	return strings.Split(text, "\n")
}

// lineWidth returns the width in MBs at scale 1 for a single line: 4*len-1, or 0 for empty.
func lineWidth(line string) int {
	if len(line) == 0 {
		return 0
	}
	return 4*len(line) - 1
}

// TextWidth returns the width in MBs at scale 1 for the widest line.
// For multi-line text (containing \n), returns the width of the longest line.
func TextWidth(text string) int {
	w := 0
	for _, line := range textLines(text) {
		if lw := lineWidth(line); lw > w {
			w = lw
		}
	}
	return w
}

// TextHeight returns the height in MBs at scale 1.
// Single line = 5. Each additional line adds 6 (1 gap row + 5 glyph rows).
func TextHeight(text string) int {
	lines := textLines(text)
	n := len(lines)
	if n == 0 {
		return 0
	}
	return 5*n + (n - 1)
}

// AutoTextScale returns the largest integer scale S where the scaled text
// plus a 1-MB border fits within the given frame dimensions.
// Text area at scale S: width = S*TextWidth(text), height = S*TextHeight(text).
// With border: width+2, height+2 must fit within mbWidth, mbHeight.
// Returns at least 1.
func AutoTextScale(text string, mbWidth, mbHeight int) int {
	tw := TextWidth(text)
	th := TextHeight(text)
	if tw == 0 || th == 0 {
		return 1
	}
	s := 1
	for {
		next := s + 1
		w := next*tw + 2
		h := next*th + 2
		if w > mbWidth || h > mbHeight {
			break
		}
		s = next
	}
	return s
}

// renderText stamps glyph pixels onto chars, centered in mbWidth×mbHeight.
// Writes '#' for foreground glyph pixels. If textBg is true, first draws
// a 1-MB-border '@' box behind the text area.
// Multi-line text (containing \n) renders each line centered horizontally
// within the overall text block, with a 1-row gap between lines.
func renderText(chars [][]byte, mbWidth, mbHeight int, text string, scale int, textBg bool) {
	tw := TextWidth(text)
	th := TextHeight(text)
	textAreaWidth := scale * tw
	textAreaHeight := scale * th

	offsetX := (mbWidth - textAreaWidth) / 2
	offsetY := (mbHeight - textAreaHeight) / 2

	// Draw text background box if requested
	if textBg {
		boxX := offsetX - 1
		boxY := offsetY - 1
		boxW := textAreaWidth + 2
		boxH := textAreaHeight + 2
		if boxX >= 0 && boxY >= 0 && boxX+boxW <= mbWidth && boxY+boxH <= mbHeight {
			for y := boxY; y < boxY+boxH; y++ {
				for x := boxX; x < boxX+boxW; x++ {
					chars[y][x] = '@'
				}
			}
		}
	}

	// Draw glyph pixels (scaled), line by line
	lines := textLines(text)
	lineY := offsetY
	for _, line := range lines {
		lw := lineWidth(line)
		// Center this line horizontally within the text area
		lineOffsetX := offsetX + (textAreaWidth-scale*lw)/2
		for i := 0; i < len(line); i++ {
			g, ok := glyphs[line[i]]
			if !ok {
				continue
			}
			dx := lineOffsetX + i*4*scale
			for row := range 5 {
				for col := range 3 {
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
		lineY += 6 * scale // 5 glyph rows + 1 gap row, all scaled
	}
}

// TextGrid creates a new grid with rendered text centered in the frame.
// Uses '#' for foreground, '.' for background, '@' for optional text background box.
func TextGrid(text string, mbWidth, mbHeight, scale int,
	bg, fg Color, textBg *Color) (*Grid, ColorMap, error) {

	tw := TextWidth(text)
	th := TextHeight(text)
	textAreaWidth := scale * tw
	textAreaHeight := scale * th

	if textAreaWidth > mbWidth || textAreaHeight > mbHeight {
		return nil, nil, fmt.Errorf("frame %dx%d MBs too small for text %q at scale %d (need %dx%d)",
			mbWidth, mbHeight, text, scale, textAreaWidth, textAreaHeight)
	}

	chars := make([][]byte, mbHeight)
	for y := range mbHeight {
		chars[y] = make([]byte, mbWidth)
		for x := range mbWidth {
			chars[y][x] = '.'
		}
	}

	renderText(chars, mbWidth, mbHeight, text, scale, textBg != nil)

	colorMap := ColorMap{
		'.': bg,
		'#': fg,
	}
	if textBg != nil {
		colorMap['@'] = *textBg
	}

	return &Grid{Chars: chars, Width: mbWidth, Height: mbHeight}, colorMap, nil
}

// OverlayText renders text onto an existing grid, centered.
// Foreground glyph pixels replace cells with '#'. Optional textBg draws '@' box.
// Non-glyph pixels are transparent (original grid shows through).
func OverlayText(grid *Grid, colors ColorMap, text string, scale int,
	fg Color, textBg *Color) (*Grid, ColorMap, error) {

	tw := TextWidth(text)
	th := TextHeight(text)
	textAreaWidth := scale * tw
	textAreaHeight := scale * th

	if textAreaWidth > grid.Width || textAreaHeight > grid.Height {
		return nil, nil, fmt.Errorf("frame %dx%d MBs too small for text %q at scale %d (need %dx%d)",
			grid.Width, grid.Height, text, scale, textAreaWidth, textAreaHeight)
	}

	// Clone the grid
	chars := make([][]byte, grid.Height)
	for y := range grid.Height {
		chars[y] = make([]byte, grid.Width)
		copy(chars[y], grid.Chars[y])
	}

	renderText(chars, grid.Width, grid.Height, text, scale, textBg != nil)

	// Merge color maps
	merged := make(ColorMap)
	maps.Copy(merged, colors)
	merged['#'] = fg
	if textBg != nil {
		merged['@'] = *textBg
	}

	return &Grid{Chars: chars, Width: grid.Width, Height: grid.Height}, merged, nil
}
