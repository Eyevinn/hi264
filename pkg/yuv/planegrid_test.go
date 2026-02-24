package yuv

import (
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
