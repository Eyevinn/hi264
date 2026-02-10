package encode

import (
	"testing"

	"github.com/Eyevinn/mp4ff/avc"
)

func TestSPSRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"16x16", 16, 16},
		{"32x32", 32, 32},
		{"320x240", 320, 240},
		{"1280x720", 1280, 720},
		{"100x100", 100, 100},
		{"1920x1080", 1920, 1080},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rbsp := EncodeSPS(tt.width, tt.height, 0)

			// Construct NALU: NAL header + RBSP
			nalHeader := byte(0x67) // nal_ref_idc=3, type=7 (SPS)
			nalu := append([]byte{nalHeader}, rbsp...)

			sps, err := avc.ParseSPSNALUnit(nalu, true)
			if err != nil {
				t.Fatalf("ParseSPSNALUnit: %v", err)
			}

			if sps.Profile != 66 {
				t.Errorf("profile = %d, want 66", sps.Profile)
			}
			if sps.Width != uint(tt.width) {
				t.Errorf("width = %d, want %d", sps.Width, tt.width)
			}
			if sps.Height != uint(tt.height) {
				t.Errorf("height = %d, want %d", sps.Height, tt.height)
			}
			if !sps.FrameMbsOnlyFlag {
				t.Error("FrameMbsOnlyFlag should be true")
			}
		})
	}
}

func TestSPSCropping(t *testing.T) {
	tests := []struct {
		name      string
		width     int
		height    int
		wantCrop  bool
		wantCropR uint
		wantCropB uint
		profile   int
	}{
		{"Baseline 320x240 no crop", 320, 240, false, 0, 0, 66},
		{"Baseline 100x100 crop", 100, 100, true, 6, 6, 66},
		{"Baseline 1920x1080 crop", 1920, 1080, true, 0, 4, 66},
		{"Main 100x100 crop", 100, 100, true, 6, 6, 77},
		{"Main 1920x1080 crop", 1920, 1080, true, 0, 4, 77},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rbsp []byte
			if tt.profile == 77 {
				rbsp = EncodeSPSMain(tt.width, tt.height, 0)
			} else {
				rbsp = EncodeSPS(tt.width, tt.height, 0)
			}

			nalHeader := byte(0x67)
			nalu := append([]byte{nalHeader}, rbsp...)

			sps, err := avc.ParseSPSNALUnit(nalu, true)
			if err != nil {
				t.Fatalf("ParseSPSNALUnit: %v", err)
			}

			if sps.FrameCroppingFlag != tt.wantCrop {
				t.Errorf("FrameCroppingFlag = %v, want %v", sps.FrameCroppingFlag, tt.wantCrop)
			}
			if tt.wantCrop {
				if sps.FrameCropRightOffset != tt.wantCropR {
					t.Errorf("FrameCropRightOffset = %d, want %d", sps.FrameCropRightOffset, tt.wantCropR)
				}
				if sps.FrameCropBottomOffset != tt.wantCropB {
					t.Errorf("FrameCropBottomOffset = %d, want %d", sps.FrameCropBottomOffset, tt.wantCropB)
				}
			}
			if sps.Width != uint(tt.width) {
				t.Errorf("Width = %d, want %d", sps.Width, tt.width)
			}
			if sps.Height != uint(tt.height) {
				t.Errorf("Height = %d, want %d", sps.Height, tt.height)
			}
		})
	}
}
