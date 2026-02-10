package yuv

import "testing"

func TestAddSuffix(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		width    int
		height   int
		want     string
	}{
		{"yuv", "output.yuv", 176, 80, "output_176x80_yuv420p.yuv"},
		{"with_path", "/tmp/test.yuv", 320, 240, "/tmp/test_320x240_yuv420p.yuv"},
		{"no_ext", "output", 16, 16, "output_16x16_yuv420p"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AddSuffix(tt.filename, tt.width, tt.height)
			if got != tt.want {
				t.Errorf("AddSuffix(%q, %d, %d) = %q, want %q",
					tt.filename, tt.width, tt.height, got, tt.want)
			}
		})
	}
}
