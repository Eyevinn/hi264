package yuv

import (
	"fmt"

	"github.com/Eyevinn/hi264/pkg/frame"
)

// PlaneGrid holds Y/Cb/Cr values at block resolution — one value per block.
// BlockSize is 16 (one value per macroblock) or 8 (four values per macroblock).
type PlaneGrid struct {
	Y, Cb, Cr [][]uint8 // [row][col] values, one per block
	Width     int       // columns in block units
	Height    int       // rows in block units
	BlockSize int       // 16 or 8 pixels per cell
}

// NewPlaneGrid creates a PlaneGrid with the given dimensions in block units.
func NewPlaneGrid(w, h, blockSize int) *PlaneGrid {
	pg := &PlaneGrid{
		Y:         make([][]uint8, h),
		Cb:        make([][]uint8, h),
		Cr:        make([][]uint8, h),
		Width:     w,
		Height:    h,
		BlockSize: blockSize,
	}
	for i := range h {
		pg.Y[i] = make([]uint8, w)
		pg.Cb[i] = make([]uint8, w)
		pg.Cr[i] = make([]uint8, w)
	}
	return pg
}

// MBWidth returns the number of macroblock columns.
func (pg *PlaneGrid) MBWidth() int {
	if pg.BlockSize == 8 {
		return (pg.Width + 1) / 2
	}
	return pg.Width
}

// MBHeight returns the number of macroblock rows.
func (pg *PlaneGrid) MBHeight() int {
	if pg.BlockSize == 8 {
		return (pg.Height + 1) / 2
	}
	return pg.Height
}

// PixelWidth returns the frame width in pixels.
func (pg *PlaneGrid) PixelWidth() int {
	return pg.MBWidth() * 16
}

// PixelHeight returns the frame height in pixels.
func (pg *PlaneGrid) PixelHeight() int {
	return pg.MBHeight() * 16
}

// MBLumaValues returns the 4 luma block values covering macroblock (mbX, mbY)
// in raster order: TL, TR, BL, BR.
// For BlockSize=16, all 4 are identical.
// For BlockSize=8, they are the 4 quadrant values.
func (pg *PlaneGrid) MBLumaValues(mbX, mbY int) [4]uint8 {
	if pg.BlockSize == 16 {
		v := pg.Y[mbY][mbX]
		return [4]uint8{v, v, v, v}
	}
	// BlockSize=8: 2x2 blocks per MB
	bx := mbX * 2
	by := mbY * 2
	var vals [4]uint8
	vals[0] = pg.blockVal(pg.Y, bx, by)
	vals[1] = pg.blockVal(pg.Y, bx+1, by)
	vals[2] = pg.blockVal(pg.Y, bx, by+1)
	vals[3] = pg.blockVal(pg.Y, bx+1, by+1)
	return vals
}

// MBChromaSub returns per-sub-block chroma values for macroblock (mbX, mbY).
// Layout: TL(0), TR(1), BL(2), BR(3).
// For BlockSize=16, all 4 are identical (one chroma value per MB).
// For BlockSize=8, each 8x8 luma quadrant maps to one 4x4 chroma sub-block.
func (pg *PlaneGrid) MBChromaSub(mbX, mbY int) (cb, cr [4]uint8) {
	if pg.BlockSize == 16 {
		cbv := pg.Cb[mbY][mbX]
		crv := pg.Cr[mbY][mbX]
		return [4]uint8{cbv, cbv, cbv, cbv}, [4]uint8{crv, crv, crv, crv}
	}
	bx := mbX * 2
	by := mbY * 2
	cb[0] = pg.blockVal(pg.Cb, bx, by)
	cb[1] = pg.blockVal(pg.Cb, bx+1, by)
	cb[2] = pg.blockVal(pg.Cb, bx, by+1)
	cb[3] = pg.blockVal(pg.Cb, bx+1, by+1)
	cr[0] = pg.blockVal(pg.Cr, bx, by)
	cr[1] = pg.blockVal(pg.Cr, bx+1, by)
	cr[2] = pg.blockVal(pg.Cr, bx, by+1)
	cr[3] = pg.blockVal(pg.Cr, bx+1, by+1)
	return cb, cr
}

// blockVal returns the value at (bx, by) in the plane, clamping to the last
// row/col if out of bounds (for odd-sized grids).
func (pg *PlaneGrid) blockVal(plane [][]uint8, bx, by int) uint8 {
	if by >= pg.Height {
		by = pg.Height - 1
	}
	if bx >= pg.Width {
		bx = pg.Width - 1
	}
	return plane[by][bx]
}

// GridToPlaneGrid converts an existing Grid+ColorMap to a PlaneGrid with BlockSize=16.
func GridToPlaneGrid(grid *Grid, colors ColorMap) (*PlaneGrid, error) {
	return GridToPlaneGridBS(grid, colors, 16)
}

// GridToPlaneGridBS converts a Grid+ColorMap to a PlaneGrid with the specified block size.
func GridToPlaneGridBS(grid *Grid, colors ColorMap, blockSize int) (*PlaneGrid, error) {
	pg := NewPlaneGrid(grid.Width, grid.Height, blockSize)
	for y := range grid.Height {
		for x := range grid.Width {
			ch := grid.Chars[y][x]
			c, ok := colors[ch]
			if !ok {
				return nil, fmt.Errorf("no color defined for character %q at (%d,%d)", string(ch), x, y)
			}
			pg.Y[y][x] = c.Y
			pg.Cb[y][x] = c.Cb
			pg.Cr[y][x] = c.Cr
		}
	}
	return pg, nil
}

// BuildFrameFromPlaneGrid expands block values to pixel arrays for raw output.
// This is the only place where full pixel arrays are created from PlaneGrid.
func BuildFrameFromPlaneGrid(pg *PlaneGrid) *frame.Frame {
	width := pg.PixelWidth()
	height := pg.PixelHeight()
	f := frame.NewFrame(width, height)

	bs := pg.BlockSize
	chromaBS := bs / 2 // 8 for BlockSize=16, 4 for BlockSize=8

	for by := range pg.Height {
		for bx := range pg.Width {
			yVal := pg.Y[by][bx]
			cbVal := pg.Cb[by][bx]
			crVal := pg.Cr[by][bx]

			// Fill luma block
			px0 := bx * bs
			py0 := by * bs
			for py := range bs {
				off := (py0+py)*f.StrideY + px0
				for px := range bs {
					f.Y[off+px] = yVal
				}
			}

			// Fill chroma block (4:2:0: chroma is half resolution)
			cx0 := bx * chromaBS
			cy0 := by * chromaBS
			for cy := range chromaBS {
				offC := (cy0+cy)*f.StrideC + cx0
				for cx := range chromaBS {
					f.Cb[offC+cx] = cbVal
					f.Cr[offC+cx] = crVal
				}
			}
		}
	}

	return f
}
