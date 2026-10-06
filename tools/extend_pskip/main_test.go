package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"

	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/hi264/pkg/yuv"
)

// streamBuilder builds a CAVLC Annex-B stream with the reference structure of a B-frame source,
// which hi264's encoder cannot produce on its own.
type streamBuilder struct {
	t   *testing.T
	sps *avc.SPS
	pps *avc.PPS
	buf bytes.Buffer
}

// newStream starts a stream with SPS, PPS and an IDR (frame_num 0, pic_order_cnt_lsb 0).
func newStream(t *testing.T) *streamBuilder {
	t.Helper()
	grid, err := yuv.ParseGrid("xy")
	if err != nil {
		t.Fatal(err)
	}
	enc := &encode.FrameEncoder{
		Grid:   grid,
		Colors: yuv.ColorMap{'x': {Y: 200, Cb: 100, Cr: 150}, 'y': {Y: 50, Cb: 200, Cr: 80}},
		QP:     26, DisableDeblock: 1, MaxNumRefFrames: 1,
	}
	b := &streamBuilder{t: t}
	if err := enc.EncodeSPSPPS(&b.buf); err != nil {
		t.Fatalf("EncodeSPSPPS: %v", err)
	}
	idr, err := enc.EncodeSlice(0)
	if err != nil {
		t.Fatalf("EncodeSlice: %v", err)
	}
	b.buf.Write(idr)
	for _, nalu := range avc.ExtractNalusFromByteStream(b.buf.Bytes()) {
		switch avc.GetNaluType(nalu[0]) {
		case avc.NALU_SPS:
			if b.sps, err = avc.ParseSPSNALUnit(nalu, true); err != nil {
				t.Fatalf("parse SPS: %v", err)
			}
		case avc.NALU_PPS:
			if b.pps, err = avc.ParsePPSNALUnit(nalu, map[uint32]*avc.SPS{b.sps.ParameterID: b.sps}); err != nil {
				t.Fatalf("parse PPS: %v", err)
			}
		}
	}
	return b
}

// ref appends a reference P_Skip picture.
func (b *streamBuilder) ref(frameNum, pocLsb uint32) *streamBuilder {
	b.t.Helper()
	slice, err := encode.EncodePSkipSlice(b.sps, b.pps, frameNum, pocLsb, 1)
	if err != nil {
		b.t.Fatalf("EncodePSkipSlice: %v", err)
	}
	b.buf.Write(slice)
	return b
}

// nonRef appends a non-reference P_Skip picture (nal_ref_idc 0), which stands in for a B picture:
// its slice header has no dec_ref_pic_marking().
func (b *streamBuilder) nonRef(frameNum, pocLsb uint32) *streamBuilder {
	b.t.Helper()
	w := encode.NewBitWriter()
	w.WriteUE(0) // first_mb_in_slice
	w.WriteUE(0) // slice_type (P)
	w.WriteUE(b.pps.PicParameterSetID)
	w.WriteBits(frameNum, int(b.sps.Log2MaxFrameNumMinus4+4))
	w.WriteBits(pocLsb, int(b.sps.Log2MaxPicOrderCntLsbMinus4+4))
	w.WriteBit(0) // num_ref_idx_active_override_flag
	w.WriteBit(0) // ref_pic_list_modification_flag_l0
	w.WriteSE(0)  // slice_qp_delta
	if b.pps.DeblockingFilterControlPresentFlag {
		w.WriteUE(1) // disable_deblocking_filter_idc
	}
	totalMBs := ((b.sps.Width + 15) / 16) * ((b.sps.Height + 15) / 16)
	w.WriteUE(uint32(totalMBs)) // mb_skip_run
	w.WriteBit(1)               // rbsp_stop_one_bit
	w.AlignToByte()
	if err := encode.WriteNALU(&b.buf, 1, 0, w.Bytes()); err != nil {
		b.t.Fatalf("WriteNALU: %v", err)
	}
	return b
}

func (b *streamBuilder) bytes() []byte { return bytes.Clone(b.buf.Bytes()) }

// cutSource is a B-frame stream interrupted mid-reorder: IDR, P(frame_num 1, poc 4), then the
// non-reference b(2, 2) that is displayed before it. Its continuation is frame_num 2 and poc 6.
func cutSource(t *testing.T) *streamBuilder {
	return newStream(t).ref(1, 4).nonRef(2, 2)
}

func writeFile(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "in.264")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunCutAtNonRefVerifies(t *testing.T) {
	// The reference P(2, 8) after the b is what -cut-at-non-ref removes.
	in := writeFile(t, cutSource(t).ref(2, 8).bytes())
	out := filepath.Join(t.TempDir(), "out.264")

	var stdout bytes.Buffer
	if err := run([]string{"extend_pskip", "-cut-at-non-ref", "-verify", in, out, "3"}, &stdout); err != nil {
		t.Fatalf("run: %v\n%s", err, stdout.String())
	}
	for _, want := range []string{
		"verified 3 appended frames: frame_num continues 1, pic_order_cnt_lsb continues 4",
		"extended_slices=6",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, stdout.String())
		}
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	slices, _, err := parseSlices(data)
	if err != nil {
		t.Fatalf("parseSlices: %v", err)
	}
	want := []sliceInfo{
		{isIDR: true, isRef: true, frameNum: 0, pocLsb: 0},
		{isRef: true, frameNum: 1, pocLsb: 4},
		{frameNum: 2, pocLsb: 2},
		{isRef: true, frameNum: 2, pocLsb: 6},
		{isRef: true, frameNum: 3, pocLsb: 8},
		{isRef: true, frameNum: 4, pocLsb: 10},
	}
	if len(slices) != len(want) {
		t.Fatalf("got %d slices, want %d: %+v", len(slices), len(want), slices)
	}
	for i := range want {
		if slices[i] != want[i] {
			t.Errorf("slice %d = %+v, want %+v", i, slices[i], want[i])
		}
	}
}

// TestVerifyAppendedRejectsBadContinuations checks that the oracle used by
// tools/verify_pskip_extend.sh catches the continuations it exists to catch.
func TestVerifyAppendedRejectsBadContinuations(t *testing.T) {
	cases := []struct {
		name     string
		appended func(b *streamBuilder) *streamBuilder
		count    uint32
		wantErr  string
	}{
		{"correct", func(b *streamBuilder) *streamBuilder { return b.ref(2, 6).ref(3, 8) }, 2, ""},
		{"off the last coded slice", func(b *streamBuilder) *streamBuilder { return b.ref(3, 4) }, 1,
			"appended frame 0 has frame_num 3, want 2"},
		{"poc off the last picture", func(b *streamBuilder) *streamBuilder { return b.ref(2, 4) }, 1,
			"appended frame 0 has pic_order_cnt_lsb 4, want 6"},
		{"second frame wrong", func(b *streamBuilder) *streamBuilder { return b.ref(2, 6).ref(2, 8) }, 2,
			"appended frame 1 has frame_num 2, want 3"},
		{"frame missing", func(b *streamBuilder) *streamBuilder { return b.ref(2, 6) }, 2,
			"extended stream has 4 slices, want 5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := cutSource(t)
			source := b.bytes()
			err := verifyAppended(&bytes.Buffer{}, source, c.appended(b).bytes(), c.count)
			switch {
			case c.wantErr == "" && err != nil:
				t.Errorf("unexpected error: %v", err)
			case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
				t.Errorf("got error %v, want one containing %q", err, c.wantErr)
			}
		})
	}
}

func TestCutAfterLastNonReference(t *testing.T) {
	cut, err := cutAfterLastNonReference(cutSource(t).ref(2, 8).ref(3, 12).bytes())
	if err != nil {
		t.Fatalf("cutAfterLastNonReference: %v", err)
	}
	if want := cutSource(t).bytes(); !bytes.Equal(cut, want) {
		t.Errorf("cut stream differs from the stream up to the last non-reference picture")
	}

	if _, err := cutAfterLastNonReference(newStream(t).ref(1, 2).bytes()); err == nil ||
		!strings.Contains(err.Error(), "no non-reference picture") {
		t.Errorf("a stream without non-reference pictures: got %v", err)
	}
}

func TestRunErrors(t *testing.T) {
	in := writeFile(t, newStream(t).ref(1, 2).bytes())
	out := filepath.Join(t.TempDir(), "out.264")
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"missing arguments", []string{in, out}, "expected 3 positional arguments"},
		{"zero count", []string{in, out, "0"}, "count must be a positive integer"},
		{"bad count", []string{in, out, "x"}, "count must be a positive integer"},
		{"missing input", []string{filepath.Join(t.TempDir(), "none.264"), out, "1"}, "none.264"},
		{"nothing to cut", []string{"-cut-at-non-ref", in, out, "1"}, "no non-reference picture"},
		{"unknown flag", []string{"-nope", in, out, "1"}, "-nope"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := run(append([]string{"extend_pskip"}, c.args...), &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("got error %v, want one containing %q", err, c.wantErr)
			}
		})
	}
}

func TestPocLater(t *testing.T) {
	cases := []struct {
		a, b uint32
		want bool
	}{
		{4, 2, true},
		{2, 4, false},
		{4, 4, false},
		{2, 14, true},  // wrapped past 16
		{14, 2, false}, // 14 is before a wrapped 2
	}
	for _, c := range cases {
		if got := pocLater(c.a, c.b, 16); got != c.want {
			t.Errorf("pocLater(%d, %d, 16) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
