package encode

import (
	"testing"

	"github.com/Eyevinn/hi264/pkg/decoder"
	"github.com/Eyevinn/hi264/pkg/yuv"
)

func TestGenerateSPS(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cabac bool
	}{
		{"CAVLC", false},
		{"CABAC", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := GenerateSPS(EncodeParams{Width: 320, Height: 240, QP: 26, CABAC: tc.cabac})
			if err != nil {
				t.Fatalf("GenerateSPS: %v", err)
			}
			// Check Annex-B start code
			if len(data) < 5 {
				t.Fatalf("SPS too short: %d bytes", len(data))
			}
			if data[0] != 0 || data[1] != 0 || data[2] != 0 || data[3] != 1 {
				t.Error("missing Annex-B start code")
			}
			// NALU type should be 7 (SPS)
			naluType := data[4] & 0x1f
			if naluType != 7 {
				t.Errorf("NALU type = %d, want 7 (SPS)", naluType)
			}
		})
	}
}

func TestGeneratePPS(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cabac bool
	}{
		{"CAVLC", false},
		{"CABAC", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := GeneratePPS(EncodeParams{Width: 320, Height: 240, QP: 26, CABAC: tc.cabac})
			if err != nil {
				t.Fatalf("GeneratePPS: %v", err)
			}
			if len(data) < 5 {
				t.Fatalf("PPS too short: %d bytes", len(data))
			}
			if data[0] != 0 || data[1] != 0 || data[2] != 0 || data[3] != 1 {
				t.Error("missing Annex-B start code")
			}
			naluType := data[4] & 0x1f
			if naluType != 8 {
				t.Errorf("NALU type = %d, want 8 (PPS)", naluType)
			}
		})
	}
}

func TestGenerateIDR(t *testing.T) {
	grid, err := yuv.ParseGrid("x")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 128, Cb: 128, Cr: 128},
	}

	p := EncodeParams{Width: 16, Height: 16, QP: 26}
	idr, err := GenerateIDR(p, grid, colors, 0)
	if err != nil {
		t.Fatalf("GenerateIDR: %v", err)
	}

	// Check start code + NALU type 5 (IDR)
	if len(idr) < 5 {
		t.Fatalf("IDR too short: %d bytes", len(idr))
	}
	naluType := idr[4] & 0x1f
	if naluType != 5 {
		t.Errorf("NALU type = %d, want 5 (IDR)", naluType)
	}

	// Decode it
	sps, _ := GenerateSPS(p)
	pps, _ := GeneratePPS(p)
	var bs []byte
	bs = append(bs, sps...)
	bs = append(bs, pps...)
	bs = append(bs, idr...)

	dec := decoder.New()
	dec.SkipDeblock = true
	f, err := dec.DecodeAnnexB(bs)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if f.Width != 16 || f.Height != 16 {
		t.Errorf("frame size %dx%d, want 16x16", f.Width, f.Height)
	}
}

func TestGeneratePSkip(t *testing.T) {
	grid, err := yuv.ParseGrid("x")
	if err != nil {
		t.Fatal(err)
	}
	colors := yuv.ColorMap{
		'x': {Y: 128, Cb: 128, Cr: 128},
	}

	p := EncodeParams{Width: 16, Height: 16, QP: 26, MaxRefFrames: 1}

	sps, _ := GenerateSPS(p)
	pps, _ := GeneratePPS(p)
	idr, err := GenerateIDR(p, grid, colors, 0)
	if err != nil {
		t.Fatalf("GenerateIDR: %v", err)
	}
	pskip, err := GeneratePSkip(p, 1)
	if err != nil {
		t.Fatalf("GeneratePSkip: %v", err)
	}

	// Check start code + NALU type 1 (non-IDR)
	naluType := pskip[4] & 0x1f
	if naluType != 1 {
		t.Errorf("NALU type = %d, want 1 (non-IDR)", naluType)
	}

	var bs []byte
	bs = append(bs, sps...)
	bs = append(bs, pps...)
	bs = append(bs, idr...)
	bs = append(bs, pskip...)

	dec := decoder.New()
	dec.SkipDeblock = true
	frames, err := dec.DecodeAllAnnexB(bs)
	if err != nil {
		t.Fatalf("DecodeAllAnnexB: %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
}

func TestRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cabac bool
	}{
		{"CAVLC", false},
		{"CABAC", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			grid, err := yuv.ParseGrid("ab\ncd")
			if err != nil {
				t.Fatal(err)
			}
			colors := yuv.ColorMap{
				'a': {Y: 235, Cb: 128, Cr: 128},
				'b': {Y: 16, Cb: 128, Cr: 128},
				'c': {Y: 128, Cb: 200, Cr: 100},
				'd': {Y: 80, Cb: 100, Cr: 200},
			}

			p := EncodeParams{Width: 32, Height: 32, QP: 26, CABAC: tc.cabac, DisableDeblock: 1}

			sps, err := GenerateSPS(p)
			if err != nil {
				t.Fatalf("GenerateSPS: %v", err)
			}
			pps, err := GeneratePPS(p)
			if err != nil {
				t.Fatalf("GeneratePPS: %v", err)
			}
			idr, err := GenerateIDR(p, grid, colors, 0)
			if err != nil {
				t.Fatalf("GenerateIDR: %v", err)
			}

			var bs []byte
			bs = append(bs, sps...)
			bs = append(bs, pps...)
			bs = append(bs, idr...)

			dec := decoder.New()
			dec.SkipDeblock = true
			f, err := dec.DecodeAnnexB(bs)
			if err != nil {
				t.Fatalf("DecodeAnnexB: %v", err)
			}

			if f.Width != 32 || f.Height != 32 {
				t.Errorf("frame size %dx%d, want 32x32", f.Width, f.Height)
			}

			// Verify pixel values (check center of each macroblock)
			expected, err := yuv.BuildFrame(grid, colors)
			if err != nil {
				t.Fatal(err)
			}

			for mbY := 0; mbY < 2; mbY++ {
				for mbX := 0; mbX < 2; mbX++ {
					cx := mbX*16 + 8
					cy := mbY*16 + 8
					gotY := f.GetLumaPixel(cx, cy)
					wantY := expected.GetLumaPixel(cx, cy)
					if gotY != wantY {
						t.Errorf("luma(%d,%d) = %d, want %d", cx, cy, gotY, wantY)
					}
				}
			}
		})
	}
}
