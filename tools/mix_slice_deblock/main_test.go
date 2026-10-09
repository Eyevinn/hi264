package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"

	"github.com/Eyevinn/hi264/pkg/decoder"
	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/hi264/pkg/yuv"
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

func TestRun(t *testing.T) {
	golden := filepath.Join("..", "..", "testdata", "golden")
	out := filepath.Join(t.TempDir(), "out.264")
	var stdout bytes.Buffer
	if err := run([]string{"mix_slice_deblock", filepath.Join(golden, "slices_midrow.264"), out}, &stdout); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := stdout.String(); got != "rewrote 9 IDR slice headers\n" {
		t.Errorf("stdout = %q", got)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(golden, "slices_mixed_deblock.264"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("output differs from slices_mixed_deblock.264")
	}

	for _, args := range [][]string{
		{"mix_slice_deblock", "in.264"},
		{"mix_slice_deblock", filepath.Join(t.TempDir(), "missing.264"), out},
	} {
		if err := run(args, &stdout); err == nil {
			t.Errorf("run(%q): expected an error", args)
		}
	}
}

func TestMixSliceDeblockParseErrors(t *testing.T) {
	startCode := []byte{0, 0, 0, 1}
	cases := []struct {
		desc    string
		nalu    []byte
		wantErr string
	}{
		{"truncated SPS", []byte{0x67}, "parse SPS"},
		{"truncated PPS", []byte{0x68}, "parse PPS"},
		{"slice without parameter sets", []byte{0x65, 0x88, 0x84, 0x00}, "parse slice header"},
	}
	for _, c := range cases {
		_, _, err := mixSliceDeblock(append(append([]byte{}, startCode...), c.nalu...))
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("%s: got error %v, want one containing %q", c.desc, err, c.wantErr)
		}
	}
}

// hi264's encoder writes pic_order_cnt_type 0 and disable_deblocking_filter_idc
// 0 with zero offsets, which is what the first setting writes, so the
// rewritten stream must decode to the same frame.
func TestMixSliceDeblockPOCType0(t *testing.T) {
	grid, err := yuv.ParseGrid("xyxy,yxyx")
	if err != nil {
		t.Fatal(err)
	}
	for _, cabac := range []bool{false, true} {
		enc := &encode.FrameEncoder{
			Grid:   grid,
			Colors: yuv.ColorMap{'x': {Y: 200, Cb: 100, Cr: 150}, 'y': {Y: 50, Cb: 200, Cr: 80}},
			QP:     26,
			CABAC:  cabac,
		}
		stream, err := enc.Encode()
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		mixed, nrSlices, err := mixSliceDeblock(stream)
		if err != nil {
			t.Fatalf("CABAC %v: mixSliceDeblock: %v", cabac, err)
		}
		if nrSlices != 1 {
			t.Errorf("CABAC %v: rewrote %d slices, want 1", cabac, nrSlices)
		}
		want, err := decoder.New().DecodeAnnexB(stream)
		if err != nil {
			t.Fatalf("CABAC %v: decode input: %v", cabac, err)
		}
		got, err := decoder.New().DecodeAnnexB(mixed)
		if err != nil {
			t.Fatalf("CABAC %v: decode output: %v", cabac, err)
		}
		if !bytes.Equal(got.YUV420Bytes(), want.YUV420Bytes()) {
			t.Errorf("CABAC %v: rewritten stream decodes differently", cabac)
		}
	}
}
