package yuv

import (
	"bufio"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"strings"
)

// ParseImageFile parses a combined image file containing color definitions and a grid.
// Uses BT.601 limited range for any RGB conversions. See ParseImageFileCS for color space control.
func ParseImageFile(r io.Reader, isRGB bool) (*Grid, ColorMap, error) {
	grid, colors, _, err := ParseImageFileCS(r, isRGB, BT601, LimitedRange)
	return grid, colors, err
}

// ImageFileResult holds the parsed result of a .gridimg or image file.
type ImageFileResult struct {
	Grid      *Grid      // set for .gridimg
	Colors    ColorMap   // set for .gridimg
	Plane     *PlaneGrid // set for PNG/JPEG
	CS        ColorSpace
	BlockSize int // 16 (default) or 8
}

// ParseImageFileCS parses a combined image file containing color definitions and a grid,
// with color space support.
//
// Format:
//
//	# comment lines
//	@rgb           (optional: treat color values as RGB instead of YCbCr)
//	@bt709         (optional: use BT.709 for RGB→YCbCr conversion)
//	@bt2020        (optional: use BT.2020 for RGB→YCbCr conversion)
//	@8x8           (optional: grid characters map to 8x8 blocks instead of 16x16)
//	x=235,128,128
//	y=16,128,128
//
//	xyxy
//	yxyx
//
// Color definitions come first (one per line: char=v1,v2,v3).
// A @rgb directive anywhere before the grid makes colors be treated as RGB.
// @bt709 or @bt2020 directives set the color space for RGB→YCbCr conversion.
// @8x8 makes each grid character represent an 8x8 block instead of 16x16.
// An empty line separates colors from the grid.
// Grid rows follow (one character per block column).
// The isRGB parameter from the CLI flag is OR'd with the @rgb directive.
// The cs parameter provides the default color space; file directives override it.
// Returns the grid, color map, effective color space, and any error.
func ParseImageFileCS(r io.Reader, isRGB bool, cs ColorSpace, rng Range) (*Grid, ColorMap, ColorSpace, error) {
	res, err := ParseImageFileFull(r, isRGB, cs, rng)
	if err != nil {
		return nil, nil, cs, err
	}
	return res.Grid, res.Colors, res.CS, nil
}

// ParseImageFileFull parses a .gridimg file and returns the full result including block size.
func ParseImageFileFull(r io.Reader, isRGB bool, cs ColorSpace, rng Range) (*ImageFileResult, error) {
	scanner := bufio.NewScanner(r)
	var colorSpecs []string // raw "char=v1,v2,v3" strings, parsed after we know isRGB
	var gridLines []string
	inGrid := false
	blockSize := 16

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
			// Check for directives
			if strings.HasPrefix(trimmed, "@") {
				lower := strings.ToLower(trimmed)
				switch lower {
				case "@rgb":
					isRGB = true
				case "@bt709", "@bt.709":
					cs = BT709
				case "@bt2020", "@bt.2020":
					cs = BT2020
				case "@bt601", "@bt.601":
					cs = BT601
				case "@8x8":
					blockSize = 8
				default:
					return nil, fmt.Errorf("unknown directive %q", trimmed)
				}
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
		return nil, fmt.Errorf("read image file: %w", err)
	}

	// Parse color specs now that we know the final isRGB and color space
	colors := make(ColorMap)
	for _, spec := range colorSpecs {
		ch, c, err := ParseColorSpecCS(spec, isRGB, cs, rng)
		if err != nil {
			return nil, fmt.Errorf("parse color: %w", err)
		}
		colors[ch] = c
	}

	if len(gridLines) == 0 {
		return nil, fmt.Errorf("no grid found in image file")
	}

	grid, err := ParseGrid(strings.Join(gridLines, "\n"))
	if err != nil {
		return nil, fmt.Errorf("parse grid: %w", err)
	}

	return &ImageFileResult{
		Grid:      grid,
		Colors:    colors,
		CS:        cs,
		BlockSize: blockSize,
	}, nil
}

// LoadImage reads a PNG or JPEG file and returns the decoded image.
func LoadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode image %s: %w", path, err)
	}
	return img, nil
}
