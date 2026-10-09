package decoder

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"
)

// slicesStream returns the parameter sets and the four IDR slices of a
// golden 320x240 picture that x264 split into slices at macroblocks 0, 80,
// 160 and 220.
func slicesStream(t *testing.T, name string) (paramSets, slices [][]byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", name+".264"))
	if err != nil {
		t.Fatalf("read input: %v", err)
	}
	for _, nalu := range avc.ExtractNalusFromByteStream(data) {
		switch avc.GetNaluType(nalu[0]) {
		case avc.NALU_SPS, avc.NALU_PPS:
			paramSets = append(paramSets, nalu)
		case avc.NALU_IDR:
			slices = append(slices, nalu)
		}
	}
	if len(slices) != 4 {
		t.Fatalf("%s: got %d IDR slices, want 4", name, len(slices))
	}
	return paramSets, slices
}

func concatNALUs(groups ...[][]byte) [][]byte {
	var nalus [][]byte
	for _, g := range groups {
		nalus = append(nalus, g...)
	}
	return nalus
}

// A picture is decoded only when its slices cover every macroblock. A missing
// slice, a slice decoded twice, or a new picture that starts before the
// current one is complete must give an error rather than a frame with gaps.
func TestDecodeIncompleteMultiSlicePicture(t *testing.T) {
	for _, name := range []string{"slices_4", "cavlc_slices_4"} {
		ps, s := slicesStream(t, name)
		cases := []struct {
			desc    string
			slices  [][]byte
			wantErr string
		}{
			{"missing middle slice", [][]byte{s[0], s[2], s[3]}, "cover 220 of 300 macroblocks"},
			{"missing last slice", [][]byte{s[0], s[1], s[2]}, "cover 220 of 300 macroblocks"},
			{"next picture starts", [][]byte{s[0], s[1], s[0], s[1], s[2], s[3]},
				"cover 160 of 300 macroblocks"},
			{"slice repeated", [][]byte{s[0], s[1], s[1], s[2], s[3]}, "mb 80: already decoded by slice 2"},
		}
		for _, c := range cases {
			t.Run(name+"/"+c.desc, func(t *testing.T) {
				_, err := New().DecodeNALUs(concatNALUs(ps, c.slices))
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				if !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("got error %q, want it to contain %q", err, c.wantErr)
				}
			})
		}
	}
}

// Consecutive multi-slice IDR pictures are told apart by their slice headers,
// so a stream of two such pictures decodes to two frames. DecodeNALUs stops
// after the first.
func TestDecodeConsecutiveMultiSlicePictures(t *testing.T) {
	for _, name := range []string{"slices_4", "cavlc_slices_4"} {
		t.Run(name, func(t *testing.T) {
			ps, s := slicesStream(t, name)
			want, err := New().DecodeNALUs(concatNALUs(ps, s))
			if err != nil {
				t.Fatalf("DecodeNALUs: %v", err)
			}

			frames, err := New().DecodeIDRFrames(concatNALUs(ps, s, s))
			if err != nil {
				t.Fatalf("DecodeIDRFrames: %v", err)
			}
			if len(frames) != 2 {
				t.Fatalf("got %d frames, want 2", len(frames))
			}
			for i, f := range frames {
				if !bytes.Equal(f.YUV420Bytes(), want.YUV420Bytes()) {
					t.Errorf("frame %d differs from the single-picture decode", i)
				}
			}

			f, err := New().DecodeNALUs(concatNALUs(ps, s, s[:1]))
			if err != nil {
				t.Fatalf("DecodeNALUs with the start of a second picture: %v", err)
			}
			if !bytes.Equal(f.YUV420Bytes(), want.YUV420Bytes()) {
				t.Error("DecodeNALUs frame differs from the single-picture decode")
			}
		})
	}
}
