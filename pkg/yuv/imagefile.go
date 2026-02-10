package yuv

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ParseImageFile parses a combined image file containing color definitions and a grid.
// Format:
//
//	# comment lines
//	@rgb           (optional: treat color values as RGB instead of YCbCr)
//	x=235,128,128
//	y=16,128,128
//
//	xyxy
//	yxyx
//
// Color definitions come first (one per line: char=v1,v2,v3).
// A @rgb directive anywhere before the grid makes colors be treated as RGB.
// An empty line separates colors from the grid.
// Grid rows follow (one character per macroblock column).
// The isRGB parameter from the CLI flag is OR'd with the @rgb directive.
func ParseImageFile(r io.Reader, isRGB bool) (*Grid, ColorMap, error) {
	scanner := bufio.NewScanner(r)
	var colorSpecs []string // raw "char=v1,v2,v3" strings, parsed after we know isRGB
	var gridLines []string
	inGrid := false

	for scanner.Scan() {
		line := scanner.Text()

		// Skip comments
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}

		if !inGrid {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				// Empty line separates colors from grid (only if we have colors)
				if len(colorSpecs) > 0 {
					inGrid = true
				}
				continue
			}
			// Check for @rgb directive
			if strings.EqualFold(trimmed, "@rgb") {
				isRGB = true
				continue
			}
			// Try to parse as color spec
			if strings.Contains(trimmed, "=") {
				colorSpecs = append(colorSpecs, trimmed)
			} else {
				// No = sign, must be start of grid without separator
				inGrid = true
				gridLines = append(gridLines, trimmed)
			}
		} else {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			gridLines = append(gridLines, trimmed)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read image file: %w", err)
	}

	// Parse color specs now that we know the final isRGB value
	colors := make(ColorMap)
	for _, spec := range colorSpecs {
		ch, c, err := ParseColorSpec(spec, isRGB)
		if err != nil {
			return nil, nil, fmt.Errorf("parse color: %w", err)
		}
		colors[ch] = c
	}

	if len(gridLines) == 0 {
		return nil, nil, fmt.Errorf("no grid found in image file")
	}

	grid, err := ParseGrid(strings.Join(gridLines, "\n"))
	if err != nil {
		return nil, nil, fmt.Errorf("parse grid: %w", err)
	}

	return grid, colors, nil
}
