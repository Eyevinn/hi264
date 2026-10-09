package decoder

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Eyevinn/hi264/internal/cabac"
	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/hi264/pkg/yuv"
	"github.com/Eyevinn/mp4ff/avc"
)

// cabacStream encodes a CABAC IDR of the given grid followed by a P_Skip frame and returns their
// SPS, PPS, IDR and P_Skip NAL units.
func cabacStream(t *testing.T, gridSpec string) (sps, pps, idr, pSkip []byte) {
	t.Helper()
	grid, err := yuv.ParseGrid(gridSpec)
	if err != nil {
		t.Fatalf("ParseGrid: %v", err)
	}
	enc := &encode.FrameEncoder{
		Grid:   grid,
		Colors: yuv.ColorMap{'x': {Y: 200, Cb: 100, Cr: 150}, 'y': {Y: 50, Cb: 200, Cr: 80}},
		QP:     26,
		CABAC:  true,
	}
	stream, err := enc.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	nalus := avc.ExtractNalusFromByteStream(stream)
	if len(nalus) != 3 {
		t.Fatalf("expected SPS, PPS and IDR NALUs, got %d", len(nalus))
	}
	pSkipAnnexB, err := enc.EncodePSkipSlice(1)
	if err != nil {
		t.Fatalf("EncodePSkipSlice: %v", err)
	}
	return nalus[0], nalus[1], nalus[2], avc.ExtractNalusFromByteStream(pSkipAnnexB)[0]
}

// decodeWithTimeout decodes nalus with DecodeAllFrames, which also decodes the P_Skip slices, and
// returns the number of frames and the error, failing the test if decoding panics or does not
// return.
func decodeWithTimeout(t *testing.T, nalus [][]byte, what string) (int, error) {
	t.Helper()
	type result struct {
		frames   int
		err      error
		panicked any
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- result{panicked: r}
			}
		}()
		frames, err := New().DecodeAllFrames(nalus)
		done <- result{frames: len(frames), err: err}
	}()
	select {
	case r := <-done:
		if r.panicked != nil {
			t.Fatalf("decoding %s panicked: %v", what, r.panicked)
		}
		return r.frames, r.err
	case <-time.After(5 * time.Second):
		t.Fatalf("decoding %s hung", what)
		return 0, nil
	}
}

// A truncated CABAC slice must return an error: never hang, and never a frame decoded from the
// zero bits the CABAC engine reads past the end of the data. Every prefix of a generated IDR
// slice and of a P_Skip slice is decoded.
func TestDecodeTruncatedCABACSlice(t *testing.T) {
	// Without the bound on mb_qp_delta, this IDR slice truncated to 14 of its 64 bytes makes the
	// decoder hang; smaller grids (x, xy,yx) hang at no truncation length.
	sps, pps, idr, _ := cabacStream(t, "xyxy,yxyx")
	// A P_Skip slice of 80x45 macroblocks (1280x720) has enough CABAC data to be cut inside it.
	bigRow := strings.Repeat("x", 80)
	bigGrid := strings.TrimSuffix(strings.Repeat(bigRow+",", 45), ",")
	bigSPS, bigPPS, bigIDR, pSkip := cabacStream(t, bigGrid)
	for _, s := range [][][]byte{{sps, pps, idr}, {bigSPS, bigPPS, bigIDR, pSkip}} {
		if frames, err := decodeWithTimeout(t, s, "a complete stream"); err != nil || frames != len(s)-2 {
			t.Fatalf("a complete stream: %d frames, error %v, want %d frames", frames, err, len(s)-2)
		}
	}

	cases := []struct {
		name   string
		before [][]byte
		slice  []byte
	}{
		{"IDR", [][]byte{sps, pps}, idr},
		{"P_Skip", [][]byte{bigSPS, bigPPS, bigIDR}, pSkip},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			endOfData := 0
			for n := 1; n < len(c.slice); n++ {
				what := fmt.Sprintf("the %s slice truncated to %d of %d bytes", c.name, n, len(c.slice))
				_, err := decodeWithTimeout(t, append(append([][]byte{}, c.before...), c.slice[:n]), what)
				if err == nil {
					t.Errorf("%s returned no error", what)
				}
				if errors.Is(err, cabac.ErrEndOfData) {
					endOfData++
				}
			}
			if endOfData == 0 {
				t.Errorf("no truncation of the %s slice was reported as cabac.ErrEndOfData", c.name)
			}
		})
	}
}
