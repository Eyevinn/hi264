package decoder

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"

	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/hi264/pkg/frame"
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
	// A non-IDR slice; the picture before it must be complete.
	pSlice := []byte{0x41, 0x9a, 0x00, 0x80}
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
			{"P slice follows", [][]byte{s[0], s[1], pSlice}, "cover 160 of 300 macroblocks"},
			{"slice repeated", [][]byte{s[0], s[1], s[1], s[2], s[3]}, "mb 80: already decoded by slice 2"},
		}
		for _, c := range cases {
			t.Run(name+"/"+c.desc, func(t *testing.T) {
				_, err := New().DecodeAllFrames(concatNALUs(ps, c.slices))
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

func TestDecodeNALUsWithoutIDR(t *testing.T) {
	ps, _ := slicesStream(t, "slices_4")
	_, err := New().DecodeNALUs(ps)
	if err == nil || !strings.Contains(err.Error(), "no IDR NALU found") {
		t.Fatalf("got error %v, want no IDR NALU found", err)
	}
}

// Slices are decoded as runs of macroblocks in raster order, so a PPS with
// slice groups (FMO) is rejected rather than decoded wrongly.
func TestDecodeRejectsSliceGroups(t *testing.T) {
	nalus := oneMBStream(t, 0, 0, 0)
	w := encode.NewBitWriter()
	w.WriteUE(0)      // pic_parameter_set_id
	w.WriteUE(0)      // seq_parameter_set_id
	w.WriteBit(0)     // entropy_coding_mode_flag
	w.WriteBit(0)     // bottom_field_pic_order_in_frame_present_flag
	w.WriteUE(1)      // num_slice_groups_minus1
	w.WriteUE(0)      // slice_group_map_type: interleaved
	w.WriteUE(0)      // run_length_minus1[0]
	w.WriteUE(0)      // run_length_minus1[1]
	w.WriteUE(0)      // num_ref_idx_l0_default_active_minus1
	w.WriteUE(0)      // num_ref_idx_l1_default_active_minus1
	w.WriteBit(0)     // weighted_pred_flag
	w.WriteBits(0, 2) // weighted_bipred_idc
	w.WriteSE(0)      // pic_init_qp_minus26
	w.WriteSE(0)      // pic_init_qs_minus26
	w.WriteSE(0)      // chroma_qp_index_offset
	w.WriteBit(1)     // deblocking_filter_control_present_flag
	w.WriteBit(0)     // constrained_intra_pred_flag
	w.WriteBit(0)     // redundant_pic_cnt_present_flag
	w.WriteBit(1)     // rbsp_stop_one_bit
	nalus[1] = encode.BuildNALU(byte(avc.NALU_PPS), 3, w.Bytes())

	_, err := New().DecodeNALUs(nalus)
	if err == nil || !strings.Contains(err.Error(), "slice groups") {
		t.Fatalf("got error %v, want slice groups not supported", err)
	}
}

// The Intra_8x8 reference samples are filtered as clause 8.3.2.2.1 says for
// each combination of available neighbours, also those that slices in raster
// order cannot produce, such as a top-left sample available without the top
// or left ones.
func TestLuma8x8ReferenceFiltering(t *testing.T) {
	f := frame.NewFrame(32, 32)
	for i := range f.Y {
		f.Y[i] = uint8(i * 37)
	}
	const bx, by = 16, 16
	p := func(x, y int) int { return int(f.GetLumaPixel(bx+x, by+y)) } // p[x, y]
	tl, t0, t1, l0, l1 := p(-1, -1), p(0, -1), p(1, -1), p(-1, 0), p(-1, 1)
	all := availability{left: true, top: true, topLeft: true, topRight: true}
	noTopLeft := availability{left: true, top: true, topRight: true}

	// ref[7] = p'[-1, 0], ref[8] = p'[-1, -1], ref[9] = p'[0, -1]
	cases := []struct {
		desc string
		av   availability
		idx  int
		want int
	}{
		{"8-78", all, 9, (tl + 2*t0 + t1 + 2) >> 2},
		{"8-79", noTopLeft, 9, (3*t0 + t1 + 2) >> 2},
		{"8-82", availability{top: true, topLeft: true, topRight: true}, 8, (3*tl + t0 + 2) >> 2},
		{"8-83", availability{left: true, topLeft: true}, 8, (3*tl + l0 + 2) >> 2},
		{"top-left only", availability{topLeft: true}, 8, tl},
		{"8-84", all, 8, (t0 + 2*tl + l0 + 2) >> 2},
		{"8-85", all, 7, (tl + 2*l0 + l1 + 2) >> 2},
		{"8-86", noTopLeft, 7, (3*l0 + l1 + 2) >> 2},
	}
	for _, c := range cases {
		ref := getLuma8x8Neighbors(f, bx, by, c.av)
		if got := int(ref[c.idx]); got != c.want {
			t.Errorf("%s: ref[%d] = %d, want %d", c.desc, c.idx, got, c.want)
		}
	}
}
