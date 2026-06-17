package yuv

import "testing"

func TestTimecodeNonDrop(t *testing.T) {
	tests := []struct {
		name       string
		frame      int64
		rate       int
		h, m, s, f int
	}{
		{"zero", 0, 25, 0, 0, 0, 0},
		{"start-frame example 76@25", 76, 25, 0, 0, 3, 1}, // 76/25=3s, 76%25=1
		{"one hour @25", 90000, 25, 1, 0, 0, 0},
		{"one frame @30", 1, 30, 0, 0, 0, 1},
		{"last frame of second @30", 29, 30, 0, 0, 0, 29},
		{"second rollover @25", 25, 25, 0, 0, 1, 0},
		{"exactly 24h wraps to zero @25", 25 * 86400, 25, 0, 0, 0, 0},
		{"24h + 76 wraps @25", 25*86400 + 76, 25, 0, 0, 3, 1},
		{"negative wraps @25", -25, 25, 23, 59, 59, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, m, s, f, dropped := Timecode(tt.frame, tt.rate, false)
			if h != tt.h || m != tt.m || s != tt.s || f != tt.f {
				t.Errorf("Timecode(%d,%d,false) = %02d:%02d:%02d:%02d, want %02d:%02d:%02d:%02d",
					tt.frame, tt.rate, h, m, s, f, tt.h, tt.m, tt.s, tt.f)
			}
			if dropped {
				t.Errorf("non-drop must never report dropped")
			}
		})
	}
}

func TestTimecodeDropFrame(t *testing.T) {
	tests := []struct {
		name       string
		frame      int64
		rate       int
		h, m, s, f int
		dropped    bool
	}{
		// 29.97 (rate 30, drop 2): labels 00 and 01 skipped at each minute
		// except every tenth.
		{"df30 zero", 0, 30, 0, 0, 0, 0, false},
		{"df30 minute 1 start drops to ;02", 1800, 30, 0, 1, 0, 2, true},
		{"df30 minute 2 start drops to ;02", 3598, 30, 0, 2, 0, 2, true},
		{"df30 tenth minute keeps ;00", 17982, 30, 0, 10, 0, 0, false},
		// 59.94 (rate 60, drop 4): labels 00..03 skipped.
		{"df60 zero", 0, 60, 0, 0, 0, 0, false},
		{"df60 minute 1 start drops to ;04", 3600, 60, 0, 1, 0, 4, true},
		{"df60 tenth minute keeps ;00", 35964, 60, 0, 10, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, m, s, f, dropped := Timecode(tt.frame, tt.rate, true)
			if h != tt.h || m != tt.m || s != tt.s || f != tt.f || dropped != tt.dropped {
				t.Errorf("Timecode(%d,%d,true) = %02d:%02d:%02d;%02d dropped=%v, want %02d:%02d:%02d;%02d dropped=%v",
					tt.frame, tt.rate, h, m, s, f, dropped, tt.h, tt.m, tt.s, tt.f, tt.dropped)
			}
		})
	}
}

func TestFormatTextTCDropFrame(t *testing.T) {
	// Frame 1800 at rate 30 drop-frame is 00:01:00;02 -> %ff == 02.
	if got := FormatTextTC("%mm:%ss.%ff", 1800, 30, true); got != "01:00.02" {
		t.Errorf("drop-frame FormatTextTC = %q, want 01:00.02", got)
	}
	// Same frame non-drop is 00:01:00:00.
	if got := FormatTextTC("%mm:%ss.%ff", 1800, 30, false); got != "01:00.00" {
		t.Errorf("non-drop FormatTextTC = %q, want 01:00.00", got)
	}
	// %d frame number is unaffected by drop-frame.
	if got := FormatTextTC("%d", 1800, 30, true); got != "1800" {
		t.Errorf("FormatTextTC %%d = %q, want 1800", got)
	}
}

// TestTimecodeNonDropContiguous verifies that consecutive frames produce a
// strictly advancing, reversible label in non-drop mode — the property that
// makes concatenated segments line up.
func TestTimecodeNonDropContiguous(t *testing.T) {
	const rate = 25
	for f := int64(0); f < 3*rate*3600; f += 997 { // sample across 3 hours
		h, m, s, ff, _ := Timecode(f, rate, false)
		got := int64(((h*60+m)*60+s)*rate + ff)
		if got != f {
			t.Fatalf("frame %d -> %02d:%02d:%02d:%02d -> %d (not reversible)", f, h, m, s, ff, got)
		}
	}
}
