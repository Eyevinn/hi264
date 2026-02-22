package encode

import "testing"

func TestChooseLevel(t *testing.T) {
	tests := []struct {
		name      string
		w, h, fps int
		kbps      int
		cabac     bool
		want      int
	}{
		{"176x144@15 -> 10", 176, 144, 15, 0, true, 10},
		{"320x240@30 -> 13", 320, 240, 30, 0, true, 13},
		{"352x288@30 -> 13", 352, 288, 30, 0, true, 13},
		{"720x480@30 -> 30", 720, 480, 30, 0, true, 30},
		{"1280x720@30 -> 31", 1280, 720, 30, 0, true, 31},
		{"1920x1080@30 -> 40", 1920, 1080, 30, 0, true, 40},
		{"1920x1080@60 -> 42", 1920, 1080, 60, 0, true, 42},
		{"3840x2160@30 -> 51", 3840, 2160, 30, 0, true, 51},
		{"16x16 no fps -> 10", 16, 16, 0, 0, false, 10},
		{"1280x720 no fps -> 31", 1280, 720, 0, 0, false, 31},
		{"320x240@30 high bitrate -> 30", 320, 240, 30, 9000, true, 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChooseLevel(tt.w, tt.h, tt.fps, tt.kbps, tt.cabac)
			if got != tt.want {
				t.Errorf("ChooseLevel(%d, %d, %d, %d, %v) = %d, want %d",
					tt.w, tt.h, tt.fps, tt.kbps, tt.cabac, got, tt.want)
			}
		})
	}
}
