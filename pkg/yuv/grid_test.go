package yuv

import (
	"strings"
	"testing"
)

func TestParseGrid(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantW   int
		wantH   int
		wantErr bool
	}{
		{"single", "x", 1, 1, false},
		{"comma-sep", "xy,yx", 2, 2, false},
		{"newline-sep", "xy\nyx", 2, 2, false},
		{"3x2", "abc,def", 3, 2, false},
		{"empty", "", 0, 0, true},
		{"unequal", "xy,x", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := ParseGrid(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseGrid(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if g.Width != tt.wantW || g.Height != tt.wantH {
				t.Errorf("got %dx%d, want %dx%d", g.Width, g.Height, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestParseColorSpec(t *testing.T) {
	ch, c, err := ParseColorSpec("x=235,128,128", false)
	if err != nil {
		t.Fatal(err)
	}
	if ch != 'x' || c.Y != 235 || c.Cb != 128 || c.Cr != 128 {
		t.Errorf("got %c=%v, want x={235,128,128}", ch, c)
	}
}

func TestParseColorSpecRGB(t *testing.T) {
	ch, c, err := ParseColorSpec("w=255,255,255", true)
	if err != nil {
		t.Fatal(err)
	}
	if ch != 'w' {
		t.Errorf("got char %c, want w", ch)
	}
	// White RGB should produce Y close to 235
	if c.Y < 230 || c.Y > 240 {
		t.Errorf("white Y=%d, expected ~235", c.Y)
	}
}

func TestRGBToYCbCr(t *testing.T) {
	tests := []struct {
		name    string
		r, g, b uint8
		wantY   uint8
		wantCb  uint8
		wantCr  uint8
	}{
		{"black", 0, 0, 0, 16, 128, 128},
		{"white", 255, 255, 255, 235, 128, 128},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := RGBToYCbCr(tt.r, tt.g, tt.b)
			if c.Y != tt.wantY || c.Cb != tt.wantCb || c.Cr != tt.wantCr {
				t.Errorf("RGBToYCbCr(%d,%d,%d) = {%d,%d,%d}, want {%d,%d,%d}",
					tt.r, tt.g, tt.b, c.Y, c.Cb, c.Cr, tt.wantY, tt.wantCb, tt.wantCr)
			}
		})
	}
}

func TestBuildFrame(t *testing.T) {
	grid, err := ParseGrid("xy,yx")
	if err != nil {
		t.Fatal(err)
	}
	colors := ColorMap{
		'x': {Y: 235, Cb: 128, Cr: 128},
		'y': {Y: 16, Cb: 128, Cr: 128},
	}
	f, err := BuildFrame(grid, colors)
	if err != nil {
		t.Fatal(err)
	}
	if f.Width != 32 || f.Height != 32 {
		t.Errorf("frame size %dx%d, want 32x32", f.Width, f.Height)
	}
	// Check MB (0,0) is 'x'
	if f.GetLumaPixel(0, 0) != 235 {
		t.Errorf("pixel (0,0) = %d, want 235", f.GetLumaPixel(0, 0))
	}
	// Check MB (1,0) is 'y'
	if f.GetLumaPixel(16, 0) != 16 {
		t.Errorf("pixel (16,0) = %d, want 16", f.GetLumaPixel(16, 0))
	}
}

func TestBuildFrameMissingColor(t *testing.T) {
	grid, _ := ParseGrid("x")
	_, err := BuildFrame(grid, ColorMap{})
	if err == nil {
		t.Error("expected error for missing color")
	}
}

func TestParseImageFile(t *testing.T) {
	input := `# test image
x=235,128,128
y=16,128,128

xy
yx
`
	grid, colors, err := ParseImageFile(strings.NewReader(input), false)
	if err != nil {
		t.Fatal(err)
	}
	if grid.Width != 2 || grid.Height != 2 {
		t.Errorf("grid %dx%d, want 2x2", grid.Width, grid.Height)
	}
	if len(colors) != 2 {
		t.Errorf("got %d colors, want 2", len(colors))
	}
	if colors['x'].Y != 235 {
		t.Errorf("color x Y=%d, want 235", colors['x'].Y)
	}
}
