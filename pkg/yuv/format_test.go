package yuv

import "testing"

func TestFormatText(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		frame    int
		fps      int
		expected string
	}{
		// Basic frame number
		{"bare %d", "%d", 42, 25, "42"},
		{"zero-padded %03d", "%03d", 5, 25, "005"},
		{"zero-padded %04d", "%04d", 123, 25, "0123"},
		{"%d no pad", "%d", 0, 25, "0"},

		// Timestamp specifiers at 25fps
		{"hours", "%hh", 90000, 25, "01"},          // 90000/25=3600s = 1h
		{"minutes", "%mm", 3750, 25, "02"},         // 3750/25=150s = 2m30s
		{"seconds", "%ss", 3750, 25, "30"},         // 150%60=30
		{"frame in second", "%ff", 75, 25, "00"},   // 75%25=0
		{"frame in second 2", "%ff", 76, 25, "01"}, // 76%25=1
		{"milliseconds", "%ms", 76, 25, "040"},     // 1*1000/25=40

		// Compound patterns
		{"timecode", "%mm:%ss.%ff", 75, 25, "00:03.00"},
		{"timecode ms", "%hh:%mm:%ss.%ms", 3751, 25, "00:02:30.040"},
		{"counter with label", "Frame %03d", 7, 25, "Frame 007"},

		// Literal percent
		{"literal %%", "100%%", 0, 25, "100%"},
		{"double %%%%", "%%%%", 0, 25, "%%"},

		// Edge cases
		{"fps=1", "%ss", 5, 1, "05"},
		{"frame 0", "%03d", 0, 25, "000"},
		{"no specifiers", "hello", 99, 25, "hello"},
		{"empty pattern", "", 42, 25, ""},
		{"just text", "static", 0, 25, "static"},

		// Unknown specifier passes through
		{"unknown %q", "%q", 0, 25, "%q"},
		{"trailing %", "end%", 0, 25, "end%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatText(tt.pattern, tt.frame, tt.fps)
			if got != tt.expected {
				t.Errorf("FormatText(%q, %d, %d) = %q, want %q",
					tt.pattern, tt.frame, tt.fps, got, tt.expected)
			}
		})
	}
}
