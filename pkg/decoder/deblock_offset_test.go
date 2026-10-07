package decoder

import (
	"strings"
	"testing"

	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/mp4ff/avc"
)

// slice_alpha_c0_offset_div2 and slice_beta_offset_div2 are limited to -6..6,
// and the deblocking filter indexes its alpha, beta and tC0 tables with
// qp + 52 + 2*offset. A slice header with an offset far outside that range
// must be rejected rather than indexing past the tables. The in-range cases
// show that the slice is otherwise valid and reaches the deblocking filter.
func TestDecodeRejectsOutOfRangeDeblockingOffset(t *testing.T) {
	cases := []struct {
		desc        string
		alpha, beta int32
		wantErr     bool
	}{
		{"largest offsets", 6, 6, false},
		{"smallest offsets", -6, -6, false},
		{"alpha too large", 40, 0, true},
		{"alpha too small", -40, 0, true},
		{"beta too large", 0, 40, true},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic on decode: %v", r)
				}
			}()
			nalus := deblockOffsetStream(t, c.alpha, c.beta)
			_, err := New().DecodeNALUs(nalus)
			switch {
			case !c.wantErr && err != nil:
				t.Fatalf("decode: %v", err)
			case c.wantErr && err == nil:
				t.Fatal("expected an error for an out-of-range deblocking offset, got nil")
			case c.wantErr && !strings.Contains(err.Error(), "parse slice header"):
				t.Fatalf("expected a slice header error, got: %v", err)
			}
		})
	}
}

// deblockOffsetStream returns the SPS, PPS and a one-macroblock CAVLC IDR
// slice, at QP 26 and with the given deblocking offsets.
func deblockOffsetStream(t *testing.T, alphaDiv2, betaDiv2 int32) [][]byte {
	t.Helper()
	params := encode.EncodeParams{Width: 16, Height: 16, QP: 26}
	spsAnnexB, err := encode.GenerateSPS(params)
	if err != nil {
		t.Fatalf("GenerateSPS: %v", err)
	}
	ppsAnnexB, err := encode.GeneratePPS(params)
	if err != nil {
		t.Fatalf("GeneratePPS: %v", err)
	}
	spsNALU := avc.ExtractNalusFromByteStream(spsAnnexB)[0]
	ppsNALU := avc.ExtractNalusFromByteStream(ppsAnnexB)[0]
	sps, err := avc.ParseSPSNALUnit(spsNALU, true)
	if err != nil {
		t.Fatalf("ParseSPSNALUnit: %v", err)
	}

	w := encode.NewBitWriter()
	w.WriteUE(0) // first_mb_in_slice
	w.WriteUE(7) // slice_type: I, all slices
	w.WriteUE(0) // pic_parameter_set_id
	w.WriteBits(0, int(sps.Log2MaxFrameNumMinus4+4))
	w.WriteUE(0) // idr_pic_id
	if sps.PicOrderCntType == 0 {
		w.WriteBits(0, int(sps.Log2MaxPicOrderCntLsbMinus4+4))
	}
	w.WriteBit(0) // no_output_of_prior_pics_flag
	w.WriteBit(0) // long_term_reference_flag
	w.WriteSE(0)  // slice_qp_delta
	w.WriteUE(0)  // disable_deblocking_filter_idc
	w.WriteSE(alphaDiv2)
	w.WriteSE(betaDiv2)
	// One I_16x16 macroblock with DC prediction and no residual.
	w.WriteUE(3)  // mb_type: I_16x16_2_0_0
	w.WriteUE(0)  // intra_chroma_pred_mode: DC
	w.WriteSE(0)  // mb_qp_delta
	w.WriteBit(1) // Intra16x16DCLevel coeff_token: no coefficients
	w.WriteBit(1) // rbsp_stop_one_bit
	idrNALU := encode.BuildNALU(byte(avc.NALU_IDR), 3, w.Bytes())

	return [][]byte{spsNALU, ppsNALU, idrNALU}
}
