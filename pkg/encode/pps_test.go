package encode

import (
	"testing"

	"github.com/Eyevinn/mp4ff/avc"
)

func TestPPSRoundTrip(t *testing.T) {
	rbsp := EncodePPS(0)

	// Construct NALU: NAL header + RBSP
	nalHeader := byte(0x68) // nal_ref_idc=3, type=8 (PPS)
	nalu := append([]byte{nalHeader}, rbsp...)

	// Need SPS for parsing PPS
	spsRBSP := EncodeSPS(32, 32, 0, 30, 0, 0)
	spsNalu := append([]byte{0x67}, spsRBSP...)
	sps, err := avc.ParseSPSNALUnit(spsNalu, true)
	if err != nil {
		t.Fatalf("ParseSPSNALUnit: %v", err)
	}
	spsMap := map[uint32]*avc.SPS{0: sps}

	pps, err := avc.ParsePPSNALUnit(nalu, spsMap)
	if err != nil {
		t.Fatalf("ParsePPSNALUnit: %v", err)
	}

	if pps.EntropyCodingModeFlag {
		t.Error("EntropyCodingModeFlag should be false (CAVLC)")
	}
	if pps.PicInitQpMinus26 != 0 {
		t.Errorf("PicInitQpMinus26 = %d, want 0", pps.PicInitQpMinus26)
	}
	if !pps.DeblockingFilterControlPresentFlag {
		t.Error("DeblockingFilterControlPresentFlag should be true")
	}
}
