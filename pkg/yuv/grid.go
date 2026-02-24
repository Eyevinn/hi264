// Package yuv provides grid-based YUV image generation for testing.
// It parses ASCII grid descriptions with color mappings to produce
// YUV 4:2:0 frames where each grid cell maps to a 16x16 macroblock.
package yuv

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Eyevinn/hi264/pkg/frame"
)

// Grid represents a macroblock layout described by character identifiers.
type Grid struct {
	Chars  [][]byte // [row][col] character identifiers
	Width  int      // macroblock columns
	Height int      // macroblock rows
}

// Color represents a YCbCr color value in restricted/limited range.
type Color struct {
	Y, Cb, Cr uint8
}

// ColorMap maps single-byte characters to YCbCr colors.
type ColorMap map[byte]Color

// ParseGrid parses a grid string into a Grid.
// Rows are separated by commas or newlines. All rows must have equal length.
func ParseGrid(input string) (*Grid, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty grid")
	}

	// Split by comma or newline
	var rows []string
	if strings.Contains(input, ",") {
		rows = strings.Split(input, ",")
	} else {
		rows = strings.Split(input, "\n")
	}

	// Filter empty rows
	var filtered []string
	for _, r := range rows {
		r = strings.TrimSpace(r)
		if r != "" {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no grid rows found")
	}

	width := len(filtered[0])
	if width == 0 {
		return nil, fmt.Errorf("empty grid row")
	}

	chars := make([][]byte, len(filtered))
	for i, row := range filtered {
		if len(row) != width {
			return nil, fmt.Errorf("row %d has length %d, expected %d", i, len(row), width)
		}
		chars[i] = []byte(row)
	}

	return &Grid{
		Chars:  chars,
		Width:  width,
		Height: len(chars),
	}, nil
}

// ParseColorSpec parses a color spec string like "x=128,128,128".
// If isRGB is true, the values are treated as RGB and converted to YCbCr using BT.601.
func ParseColorSpec(s string, isRGB bool) (byte, Color, error) {
	return ParseColorSpecCS(s, isRGB, BT601, LimitedRange)
}

// ParseColorSpecCS parses a color spec string like "x=128,128,128".
// If isRGB is true, the values are treated as RGB and converted to YCbCr
// using the specified color space and range.
func ParseColorSpecCS(s string, isRGB bool, cs ColorSpace, rng Range) (byte, Color, error) {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 {
		return 0, Color{}, fmt.Errorf("invalid color spec %q: expected char=v1,v2,v3", s)
	}
	if len(parts[0]) != 1 {
		return 0, Color{}, fmt.Errorf("invalid color spec %q: char must be a single character", s)
	}
	ch := parts[0][0]

	vals := strings.Split(parts[1], ",")
	if len(vals) != 3 {
		return 0, Color{}, fmt.Errorf("invalid color spec %q: expected 3 values", s)
	}

	v := [3]int{}
	for i, vs := range vals {
		n, err := strconv.Atoi(strings.TrimSpace(vs))
		if err != nil {
			return 0, Color{}, fmt.Errorf("invalid color spec %q: %w", s, err)
		}
		if n < 0 || n > 255 {
			return 0, Color{}, fmt.Errorf("invalid color spec %q: value %d out of range [0,255]", s, n)
		}
		v[i] = n
	}

	if isRGB {
		c := RGBToYCbCrCS(uint8(v[0]), uint8(v[1]), uint8(v[2]), cs, rng)
		return ch, c, nil
	}

	return ch, Color{Y: uint8(v[0]), Cb: uint8(v[1]), Cr: uint8(v[2])}, nil
}

// RGBToYCbCr converts RGB to YCbCr using BT.601 limited range.
func RGBToYCbCr(r, g, b uint8) Color {
	return RGBToYCbCrCS(r, g, b, BT601, LimitedRange)
}

func clipU8(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// SolidGrid creates a uniform grid of a single color, sized to cover the
// given pixel dimensions (rounded up to whole macroblocks).
func SolidGrid(width, height int, c Color) (*Grid, ColorMap) {
	mbW := (width + 15) / 16
	mbH := (height + 15) / 16
	chars := make([][]byte, mbH)
	for i := range chars {
		row := make([]byte, mbW)
		for j := range row {
			row[j] = '.'
		}
		chars[i] = row
	}
	return &Grid{Width: mbW, Height: mbH, Chars: chars},
		ColorMap{'.': c}
}

// UpscaleGrid scales a grid by factor f: each cell becomes f×f cells.
// The ColorMap is unchanged since the same characters are used.
func UpscaleGrid(g *Grid, f int) *Grid {
	newW := g.Width * f
	newH := g.Height * f
	chars := make([][]byte, newH)
	for y := range newH {
		chars[y] = make([]byte, newW)
		for x := range newW {
			chars[y][x] = g.Chars[y/f][x/f]
		}
	}
	return &Grid{Chars: chars, Width: newW, Height: newH}
}

// BuildFrame creates a frame from a grid and color map, filling each MB with solid color.
func BuildFrame(grid *Grid, colors ColorMap) (*frame.Frame, error) {
	pg, err := GridToPlaneGrid(grid, colors)
	if err != nil {
		return nil, err
	}
	return BuildFrameFromPlaneGrid(pg), nil
}
