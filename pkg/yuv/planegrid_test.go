package yuv

import (
	"image"
	"image/color"
	"testing"
)

func TestNewPlaneGrid(t *testing.T) {
	pg := NewPlaneGrid(3, 2, 16)
	if pg.Width != 3 || pg.Height != 2 || pg.BlockSize != 16 {
		t.Errorf("got %dx%d bs=%d, want 3x2 bs=16", pg.Width, pg.Height, pg.BlockSize)
	}
	if pg.MBWidth() != 3 || pg.MBHeight() != 2 {
		t.Errorf("MBWidth=%d MBHeight=%d, want 3,2", pg.MBWidth(), pg.MBHeight())
	}
	if pg.PixelWidth() != 48 || pg.PixelHeight() != 32 {
		t.Errorf("PixelWidth=%d PixelHeight=%d, want 48,32", pg.PixelWidth(), pg.PixelHeight())
	}
}

func TestNewPlaneGrid8x8(t *testing.T) {
	pg := NewPlaneGrid(4, 4, 8)
	if pg.MBWidth() != 2 || pg.MBHeight() != 2 {
		t.Errorf("MBWidth=%d MBHeight=%d, want 2,2", pg.MBWidth(), pg.MBHeight())
	}
	if pg.PixelWidth() != 32 || pg.PixelHeight() != 32 {
		t.Errorf("PixelWidth=%d PixelHeight=%d, want 32,32", pg.PixelWidth(), pg.PixelHeight())
	}
}

func TestNewPlaneGrid8x8Odd(t *testing.T) {
	pg := NewPlaneGrid(3, 3, 8)
	// 3 blocks -> 2 MBs (ceil(3/2))
	if pg.MBWidth() != 2 || pg.MBHeight() != 2 {
		t.Errorf("MBWidth=%d MBHeight=%d, want 2,2", pg.MBWidth(), pg.MBHeight())
	}
}

func TestGridToPlaneGrid(t *testing.T) {
	grid, err := ParseGrid("xy,yx")
	if err != nil {
		t.Fatal(err)
	}
	colors := ColorMap{
		'x': {Y: 235, Cb: 128, Cr: 128},
		'y': {Y: 16, Cb: 200, Cr: 50},
	}
	pg, err := GridToPlaneGrid(grid, colors)
	if err != nil {
		t.Fatal(err)
	}
	if pg.Width != 2 || pg.Height != 2 || pg.BlockSize != 16 {
		t.Errorf("got %dx%d bs=%d, want 2x2 bs=16", pg.Width, pg.Height, pg.BlockSize)
	}
	if pg.Y[0][0] != 235 || pg.Y[0][1] != 16 || pg.Y[1][0] != 16 || pg.Y[1][1] != 235 {
		t.Errorf("Y values: %v %v", pg.Y[0], pg.Y[1])
	}
	if pg.Cb[0][1] != 200 || pg.Cr[0][1] != 50 {
		t.Errorf("chroma values wrong for (1,0)")
	}
}

func TestGridToPlaneGridMissingColor(t *testing.T) {
	grid, _ := ParseGrid("x")
	_, err := GridToPlaneGrid(grid, ColorMap{})
	if err == nil {
		t.Error("expected error for missing color")
	}
}

func TestMBLumaValues16(t *testing.T) {
	pg := NewPlaneGrid(2, 1, 16)
	pg.Y[0][0] = 100
	pg.Y[0][1] = 200
	vals := pg.MBLumaValues(0, 0)
	if vals != [4]uint8{100, 100, 100, 100} {
		t.Errorf("MBLumaValues(0,0) = %v, want all 100", vals)
	}
	vals = pg.MBLumaValues(1, 0)
	if vals != [4]uint8{200, 200, 200, 200} {
		t.Errorf("MBLumaValues(1,0) = %v, want all 200", vals)
	}
}

func TestMBLumaValues8(t *testing.T) {
	pg := NewPlaneGrid(4, 2, 8)
	pg.Y[0][0] = 10
	pg.Y[0][1] = 20
	pg.Y[0][2] = 30
	pg.Y[0][3] = 40
	pg.Y[1][0] = 50
	pg.Y[1][1] = 60
	pg.Y[1][2] = 70
	pg.Y[1][3] = 80
	// MB (0,0) covers blocks (0,0),(1,0),(0,1),(1,1) => TL=10,TR=20,BL=50,BR=60
	vals := pg.MBLumaValues(0, 0)
	if vals != [4]uint8{10, 20, 50, 60} {
		t.Errorf("MBLumaValues(0,0) = %v, want [10,20,50,60]", vals)
	}
	// MB (1,0) covers blocks (2,0),(3,0),(2,1),(3,1) => TL=30,TR=40,BL=70,BR=80
	vals = pg.MBLumaValues(1, 0)
	if vals != [4]uint8{30, 40, 70, 80} {
		t.Errorf("MBLumaValues(1,0) = %v, want [30,40,70,80]", vals)
	}
}

func TestMBChromaSub16(t *testing.T) {
	pg := NewPlaneGrid(2, 1, 16)
	pg.Cb[0][0] = 200
	pg.Cb[0][1] = 50
	pg.Cr[0][0] = 100
	pg.Cr[0][1] = 150
	cb, cr := pg.MBChromaSub(0, 0)
	if cb != [4]uint8{200, 200, 200, 200} {
		t.Errorf("Cb = %v, want all 200", cb)
	}
	if cr != [4]uint8{100, 100, 100, 100} {
		t.Errorf("Cr = %v, want all 100", cr)
	}
	cb, _ = pg.MBChromaSub(1, 0)
	if cb != [4]uint8{50, 50, 50, 50} {
		t.Errorf("Cb = %v, want all 50", cb)
	}
}

func TestMBChromaSub8(t *testing.T) {
	pg := NewPlaneGrid(4, 2, 8)
	pg.Cb[0][0] = 10
	pg.Cb[0][1] = 20
	pg.Cb[0][2] = 30
	pg.Cb[0][3] = 40
	pg.Cb[1][0] = 50
	pg.Cb[1][1] = 60
	pg.Cb[1][2] = 70
	pg.Cb[1][3] = 80
	pg.Cr[0][0] = 1
	pg.Cr[0][1] = 2
	pg.Cr[1][0] = 3
	pg.Cr[1][1] = 4
	// MB (0,0) covers blocks (0,0),(1,0),(0,1),(1,1)
	cb, cr := pg.MBChromaSub(0, 0)
	if cb != [4]uint8{10, 20, 50, 60} {
		t.Errorf("Cb = %v, want [10,20,50,60]", cb)
	}
	if cr != [4]uint8{1, 2, 3, 4} {
		t.Errorf("Cr = %v, want [1,2,3,4]", cr)
	}
	// MB (1,0) covers blocks (2,0),(3,0),(2,1),(3,1)
	cb, _ = pg.MBChromaSub(1, 0)
	if cb != [4]uint8{30, 40, 70, 80} {
		t.Errorf("Cb = %v, want [30,40,70,80]", cb)
	}
}

func TestBuildFrameFromPlaneGridMatchesBuildFrame(t *testing.T) {
	grid, err := ParseGrid("xy,yx")
	if err != nil {
		t.Fatal(err)
	}
	colors := ColorMap{
		'x': {Y: 235, Cb: 128, Cr: 128},
		'y': {Y: 16, Cb: 200, Cr: 50},
	}

	expected, err := BuildFrame(grid, colors)
	if err != nil {
		t.Fatal(err)
	}

	pg, err := GridToPlaneGrid(grid, colors)
	if err != nil {
		t.Fatal(err)
	}
	got := BuildFrameFromPlaneGrid(pg)

	if got.Width != expected.Width || got.Height != expected.Height {
		t.Fatalf("size %dx%d, want %dx%d", got.Width, got.Height, expected.Width, expected.Height)
	}

	// Compare luma pixel by pixel
	for y := 0; y < expected.Height; y++ {
		for x := 0; x < expected.Width; x++ {
			g := got.GetLumaPixel(x, y)
			e := expected.GetLumaPixel(x, y)
			if g != e {
				t.Errorf("luma(%d,%d) = %d, want %d", x, y, g, e)
			}
		}
	}

	// Compare chroma
	chromaW := expected.Width / 2
	chromaH := expected.Height / 2
	for y := range chromaH {
		for x := range chromaW {
			for c := range 2 {
				g := got.GetChromaPixel(c, x, y)
				e := expected.GetChromaPixel(c, x, y)
				if g != e {
					t.Errorf("chroma[%d](%d,%d) = %d, want %d", c, x, y, g, e)
				}
			}
		}
	}
}

func TestBuildFrameFromPlaneGrid8x8(t *testing.T) {
	pg := NewPlaneGrid(2, 2, 8)
	pg.Y[0][0] = 100
	pg.Y[0][1] = 200
	pg.Y[1][0] = 50
	pg.Y[1][1] = 150

	pg.Cb[0][0] = 128
	pg.Cb[0][1] = 128
	pg.Cb[1][0] = 128
	pg.Cb[1][1] = 128

	pg.Cr[0][0] = 128
	pg.Cr[0][1] = 128
	pg.Cr[1][0] = 128
	pg.Cr[1][1] = 128

	f := BuildFrameFromPlaneGrid(pg)

	// Should be 16x16 (1 MB)
	if f.Width != 16 || f.Height != 16 {
		t.Fatalf("size %dx%d, want 16x16", f.Width, f.Height)
	}

	// Top-left 8x8 should be 100
	if f.GetLumaPixel(0, 0) != 100 {
		t.Errorf("luma(0,0) = %d, want 100", f.GetLumaPixel(0, 0))
	}
	if f.GetLumaPixel(7, 7) != 100 {
		t.Errorf("luma(7,7) = %d, want 100", f.GetLumaPixel(7, 7))
	}

	// Top-right 8x8 should be 200
	if f.GetLumaPixel(8, 0) != 200 {
		t.Errorf("luma(8,0) = %d, want 200", f.GetLumaPixel(8, 0))
	}

	// Bottom-left 8x8 should be 50
	if f.GetLumaPixel(0, 8) != 50 {
		t.Errorf("luma(0,8) = %d, want 50", f.GetLumaPixel(0, 8))
	}

	// Bottom-right 8x8 should be 150
	if f.GetLumaPixel(8, 8) != 150 {
		t.Errorf("luma(8,8) = %d, want 150", f.GetLumaPixel(8, 8))
	}
}

func TestImageToPlaneGrid16(t *testing.T) {
	img, err := LoadImage("../../testdata/sunflowers_640x360.png")
	if err != nil {
		t.Fatal(err)
	}
	pg := ImageToPlaneGrid(img, 16, BT601, LimitedRange)
	// 640/16 = 40, 360/16 = 22 (360 rounds down: 22*16=352)
	if pg.Width != 40 || pg.Height != 22 {
		t.Errorf("got %dx%d, want 40x22", pg.Width, pg.Height)
	}
	if pg.BlockSize != 16 {
		t.Errorf("BlockSize = %d, want 16", pg.BlockSize)
	}
	// Verify non-trivial Y values (image is not all black)
	hasNonZero := false
	for y := range pg.Height {
		for x := range pg.Width {
			if pg.Y[y][x] > 16 {
				hasNonZero = true
				break
			}
		}
		if hasNonZero {
			break
		}
	}
	if !hasNonZero {
		t.Error("all Y values are <=16, expected non-black content")
	}
}

func TestImageToPlaneGrid8(t *testing.T) {
	img, err := LoadImage("../../testdata/sunflowers_640x360.png")
	if err != nil {
		t.Fatal(err)
	}
	pg := ImageToPlaneGrid(img, 8, BT601, LimitedRange)
	// 640/8 = 80, 360/8 = 45
	if pg.Width != 80 || pg.Height != 45 {
		t.Errorf("got %dx%d, want 80x45", pg.Width, pg.Height)
	}
	if pg.BlockSize != 8 {
		t.Errorf("BlockSize = %d, want 8", pg.BlockSize)
	}
}

func TestImageToPlaneGridSyntheticRed(t *testing.T) {
	// Create a 16x16 solid red image
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	pg := ImageToPlaneGrid(img, 16, BT601, LimitedRange)
	if pg.Width != 1 || pg.Height != 1 {
		t.Fatalf("got %dx%d, want 1x1", pg.Width, pg.Height)
	}
	expected := RGBToYCbCrCS(255, 0, 0, BT601, LimitedRange)
	if pg.Y[0][0] != expected.Y || pg.Cb[0][0] != expected.Cb || pg.Cr[0][0] != expected.Cr {
		t.Errorf("got Y=%d Cb=%d Cr=%d, want Y=%d Cb=%d Cr=%d",
			pg.Y[0][0], pg.Cb[0][0], pg.Cr[0][0], expected.Y, expected.Cb, expected.Cr)
	}
}

func TestScaleImageToPlaneGrid(t *testing.T) {
	img, err := LoadImage("../../testdata/sunflowers_640x360.png")
	if err != nil {
		t.Fatal(err)
	}
	// Scale 640x360 image to 20x15 blocks (320x240 pixels at BlockSize=16)
	pg := ScaleImageToPlaneGrid(img, 20, 15, 16, BT601, LimitedRange)
	if pg.Width != 20 || pg.Height != 15 {
		t.Errorf("got %dx%d, want 20x15", pg.Width, pg.Height)
	}
	if pg.BlockSize != 16 {
		t.Errorf("BlockSize = %d, want 16", pg.BlockSize)
	}
	hasNonZero := false
	for y := range pg.Height {
		for x := range pg.Width {
			if pg.Y[y][x] > 16 {
				hasNonZero = true
				break
			}
		}
		if hasNonZero {
			break
		}
	}
	if !hasNonZero {
		t.Error("all Y values are <=16, expected non-black content")
	}
}

func TestScaleImageToPlaneGridUpscale(t *testing.T) {
	// Create 2x2 image, scale to 4x4 blocks
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	pg := ScaleImageToPlaneGrid(img, 4, 4, 8, BT601, LimitedRange)
	if pg.Width != 4 || pg.Height != 4 {
		t.Fatalf("got %dx%d, want 4x4", pg.Width, pg.Height)
	}
	// Top-left 2x2 blocks should all be the red pixel's YCbCr
	red := RGBToYCbCrCS(255, 0, 0, BT601, LimitedRange)
	if pg.Y[0][0] != red.Y || pg.Y[1][0] != red.Y || pg.Y[0][1] != red.Y || pg.Y[1][1] != red.Y {
		t.Errorf("top-left quadrant not uniformly red: Y[0][0]=%d Y[1][0]=%d Y[0][1]=%d Y[1][1]=%d, want %d",
			pg.Y[0][0], pg.Y[1][0], pg.Y[0][1], pg.Y[1][1], red.Y)
	}
}

func TestTilePlaneGrid(t *testing.T) {
	src := NewPlaneGrid(2, 2, 16)
	src.Y[0][0] = 10
	src.Y[0][1] = 20
	src.Y[1][0] = 30
	src.Y[1][1] = 40
	src.Cb[0][0] = 100
	src.Cr[0][0] = 200

	pg := TilePlaneGrid(src, 6, 4)
	if pg.Width != 6 || pg.Height != 4 {
		t.Fatalf("got %dx%d, want 6x4", pg.Width, pg.Height)
	}
	if pg.BlockSize != 16 {
		t.Errorf("BlockSize = %d, want 16", pg.BlockSize)
	}

	// Check modular repetition
	for y := range 4 {
		for x := range 6 {
			want := src.Y[y%2][x%2]
			if pg.Y[y][x] != want {
				t.Errorf("Y[%d][%d] = %d, want %d", y, x, pg.Y[y][x], want)
			}
		}
	}
	// Check chroma tiling
	if pg.Cb[0][0] != 100 || pg.Cb[2][0] != 100 || pg.Cb[0][2] != 100 {
		t.Error("Cb not tiled correctly")
	}
	if pg.Cr[0][0] != 200 || pg.Cr[2][0] != 200 || pg.Cr[0][4] != 200 {
		t.Error("Cr not tiled correctly")
	}
}

func TestOverlayTextOnPlane(t *testing.T) {
	// Create a 10x5 PlaneGrid at BlockSize=16 with known values
	pg := NewPlaneGrid(10, 5, 16)
	for y := range pg.Height {
		for x := range pg.Width {
			pg.Y[y][x] = 100
			pg.Cb[y][x] = 128
			pg.Cr[y][x] = 128
		}
	}

	fg := Color{Y: 235, Cb: 128, Cr: 200}
	err := OverlayTextOnPlane(pg, "A", 1, fg, nil)
	if err != nil {
		t.Fatal(err)
	}

	// After overlay, some cells should have fg color (glyph pixels)
	// and others should remain unchanged (100, 128, 128)
	hasFg := false
	hasOrig := false
	for y := range pg.Height {
		for x := range pg.Width {
			if pg.Y[y][x] == fg.Y && pg.Cr[y][x] == fg.Cr {
				hasFg = true
			}
			if pg.Y[y][x] == 100 {
				hasOrig = true
			}
		}
	}
	if !hasFg {
		t.Error("no foreground pixels found after overlay")
	}
	if !hasOrig {
		t.Error("no original pixels preserved after overlay")
	}
}

func TestOverlayTextOnPlane8x8(t *testing.T) {
	// Create a 20x10 PlaneGrid at BlockSize=8 (10x5 MBs)
	pg := NewPlaneGrid(20, 10, 8)
	for y := range pg.Height {
		for x := range pg.Width {
			pg.Y[y][x] = 50
			pg.Cb[y][x] = 128
			pg.Cr[y][x] = 128
		}
	}

	fg := Color{Y: 235, Cb: 128, Cr: 128}
	err := OverlayTextOnPlane(pg, "A", 1, fg, nil)
	if err != nil {
		t.Fatal(err)
	}

	hasFg := false
	hasOrig := false
	for y := range pg.Height {
		for x := range pg.Width {
			if pg.Y[y][x] == 235 {
				hasFg = true
			}
			if pg.Y[y][x] == 50 {
				hasOrig = true
			}
		}
	}
	if !hasFg {
		t.Error("no foreground pixels found after 8x8 overlay")
	}
	if !hasOrig {
		t.Error("no original pixels preserved after 8x8 overlay")
	}
}

func TestOverlayTextOnPlaneTooLarge(t *testing.T) {
	pg := NewPlaneGrid(2, 2, 16)
	err := OverlayTextOnPlane(pg, "HELLO WORLD", 3, Color{235, 128, 128}, nil)
	if err == nil {
		t.Error("expected error for text too large")
	}
}

func TestOverlayTextOnPlaneWithBg(t *testing.T) {
	pg := NewPlaneGrid(20, 20, 8)
	for y := range pg.Height {
		for x := range pg.Width {
			pg.Y[y][x] = 100
			pg.Cb[y][x] = 128
			pg.Cr[y][x] = 128
		}
	}
	textBg := Color{Y: 42, Cb: 128, Cr: 128}
	err := OverlayTextOnPlane(pg, "A", 1, Color{235, 128, 128}, &textBg)
	if err != nil {
		t.Fatal(err)
	}
	hasBg := false
	for y := range pg.Height {
		for x := range pg.Width {
			if pg.Y[y][x] == 42 {
				hasBg = true
				break
			}
		}
	}
	if !hasBg {
		t.Error("no text background pixels found")
	}
}

func TestImageToPlaneGridTooSmall(t *testing.T) {
	// Image smaller than one block should return 0x0 grid
	img := image.NewRGBA(image.Rect(0, 0, 5, 5))
	pg := ImageToPlaneGrid(img, 16, BT601, LimitedRange)
	if pg.Width != 0 || pg.Height != 0 {
		t.Errorf("got %dx%d, want 0x0", pg.Width, pg.Height)
	}
}

func TestBlockValClamping(t *testing.T) {
	// Create a 3x3 grid (odd) with BlockSize=8 → 1 MB wide, but chroma
	// needs blockVal to clamp bx=2 to 2 (width-1).
	pg := NewPlaneGrid(3, 3, 8)
	pg.Cb[0][2] = 42
	pg.Cb[2][0] = 99
	// Access past bounds should clamp
	cb, _ := pg.MBChromaSub(1, 1)
	// MB(1,1) → bx=2,by=2; bx+1=3 >= Width=3 → clamped to 2
	if cb[3] != pg.Cb[2][2] {
		t.Errorf("Cb BR = %d, want %d (clamped)", cb[3], pg.Cb[2][2])
	}
}

func TestLoadImageError(t *testing.T) {
	_, err := LoadImage("nonexistent.png")
	if err == nil {
		t.Error("expected error for missing file")
	}
}
