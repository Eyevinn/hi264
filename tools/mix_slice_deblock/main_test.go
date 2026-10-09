package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"
)

// The golden mixed-deblock streams are the x264 golden streams rewritten by
// this tool; rewriting the x264 stream must reproduce them exactly.
func TestMixSliceDeblockReproducesGolden(t *testing.T) {
	cases := []struct{ input, want string }{
		{"slices_midrow", "slices_mixed_deblock"},
		{"cavlc_slices_midrow", "cavlc_slices_mixed_deblock"},
	}
	golden := filepath.Join("..", "..", "testdata", "golden")
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			in, err := os.ReadFile(filepath.Join(golden, c.input+".264"))
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join(golden, c.want+".264"))
			if err != nil {
				t.Fatal(err)
			}
			got, nrSlices, err := mixSliceDeblock(in)
			if err != nil {
				t.Fatalf("mixSliceDeblock: %v", err)
			}
			if nrSlices != 9 {
				t.Errorf("rewrote %d slices, want 9", nrSlices)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("output differs from %s.264", c.want)
			}

			// The slices carry the settings in turn.
			spsMap := map[uint32]*avc.SPS{}
			ppsMap := map[uint32]*avc.PPS{}
			i := 0
			for _, nalu := range avc.ExtractNalusFromByteStream(got) {
				switch avc.GetNaluType(nalu[0]) {
				case avc.NALU_SPS:
					sps, _ := avc.ParseSPSNALUnit(nalu, true)
					spsMap[sps.ParameterID] = sps
				case avc.NALU_PPS:
					pps, _ := avc.ParsePPSNALUnit(nalu, spsMap)
					ppsMap[pps.PicParameterSetID] = pps
				case avc.NALU_IDR:
					sh, err := avc.ParseSliceHeader(nalu, spsMap, ppsMap)
					if err != nil {
						t.Fatalf("slice %d: %v", i, err)
					}
					s := settings[i%len(settings)]
					if uint(sh.DisableDeblockingFilterIDC) != s.disableDeblockingFilterIdc ||
						int(sh.SliceAlphaC0OffsetDiv2) != s.alphaC0OffsetDiv2 ||
						int(sh.SliceBetaOffsetDiv2) != s.betaOffsetDiv2 {
						t.Errorf("slice %d: idc %d offsets %d, %d; want %+v", i, sh.DisableDeblockingFilterIDC,
							sh.SliceAlphaC0OffsetDiv2, sh.SliceBetaOffsetDiv2, s)
					}
					i++
				}
			}
		})
	}
}
