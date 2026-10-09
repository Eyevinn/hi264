package slice

import (
	"strings"
	"testing"

	"github.com/Eyevinn/hi264/internal/cavlc"
)

// cavlcMB is a CAVLC I_16x16 macroblock with DC prediction and no residual,
// when its neighbours have no coefficients either: mb_type 3 (00100),
// intra_chroma_pred_mode 0 (1), mb_qp_delta 0 (1) and an Intra16x16DCLevel
// coeff_token with no coefficients (1).
const cavlcMB = 0x27

// A CAVLC slice ends at the RBSP trailing bits, and must stay inside the
// picture.
func TestDecodeSliceDataCAVLC(t *testing.T) {
	cases := []struct {
		desc    string
		mbWidth int
		firstMB int
		data    []byte
		wantErr string
	}{
		{"two macroblocks", 2, 0, []byte{cavlcMB, cavlcMB, 0x80}, ""},
		{"starts at the second macroblock", 2, 1, []byte{cavlcMB, 0x80}, ""},
		{"past the last macroblock", 1, 0, []byte{cavlcMB, cavlcMB, 0x80},
			"slice continues past the last macroblock 0"},
		{"first_mb_in_slice outside the picture", 2, 2, []byte{cavlcMB, 0x80},
			"first_mb_in_slice 2 outside a picture of 2 macroblocks"},
		{"corrupt second macroblock", 2, 0, []byte{cavlcMB, 0x00, 0x01}, "mb 1: mb_type"},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			sc := NewSliceContext(c.mbWidth, 1, true, false, 1, 8, 8, 0, false)
			err := sc.DecodeSliceDataCAVLC(cavlc.NewBitReader(c.data), SliceParams{FirstMB: c.firstMB, SliceQPY: 26})
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("DecodeSliceDataCAVLC: %v", err)
				}
				if got, want := sc.DecodedMBs(), c.mbWidth-c.firstMB; got != want {
					t.Errorf("decoded %d macroblocks, want %d", got, want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("got error %v, want one containing %q", err, c.wantErr)
			}
		})
	}
}

// Two slices decode into one picture, each numbering its macroblocks.
func TestDecodeTwoCAVLCSlices(t *testing.T) {
	sc := NewSliceContext(2, 1, true, false, 1, 8, 8, 0, false)
	for firstMB := range 2 {
		err := sc.DecodeSliceDataCAVLC(cavlc.NewBitReader([]byte{cavlcMB, 0x80}),
			SliceParams{FirstMB: firstMB, SliceQPY: 26})
		if err != nil {
			t.Fatalf("slice at macroblock %d: %v", firstMB, err)
		}
	}
	if sc.DecodedMBs() != 2 || len(sc.Slices) != 2 {
		t.Fatalf("decoded %d macroblocks in %d slices, want 2 in 2", sc.DecodedMBs(), len(sc.Slices))
	}
	for mbIdx, mb := range sc.MBs {
		if mb.SliceNum != mbIdx+1 {
			t.Errorf("mb %d: in slice %d, want %d", mbIdx, mb.SliceNum, mbIdx+1)
		}
	}
	if sc.MBAvailA(1) != nil {
		t.Error("mb 1: left neighbour in the other slice is available")
	}
}

func TestDecodeSliceDataFirstMBOutsidePicture(t *testing.T) {
	sc := NewSliceContext(2, 1, false, false, 1, 8, 8, 0, false)
	err := sc.DecodeSliceData([]byte{0, 0}, SliceParams{FirstMB: 2, SliceQPY: 26})
	if err == nil || !strings.Contains(err.Error(), "outside a picture of 2 macroblocks") {
		t.Fatalf("got error %v, want first_mb_in_slice outside the picture", err)
	}
}
