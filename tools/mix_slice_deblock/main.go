// mix_slice_deblock is a verification helper for tools/gen_and_verify.sh.
// It rewrites the IDR slice headers of an Annex-B bitstream so that the
// slices of a picture use different deblocking filter controls:
// disable_deblocking_filter_idc 0, 1 and 2 with a range of filter offsets,
// which x264 cannot produce. In a CAVLC stream it also varies slice_qp_delta
// from slice to slice; CABAC slice data cannot be kept when the slice QP
// changes, since the QP selects the initial CABAC context states.
//
// Usage:
//
//	go run ./tools/mix_slice_deblock <input.264> <output.264>
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/bits"
)

// sliceSetting is what one slice header gets.
type sliceSetting struct {
	disableDeblockingFilterIdc uint
	alphaC0OffsetDiv2          int
	betaOffsetDiv2             int
	qpDeltaChange              int // added to slice_qp_delta in CAVLC slices
}

// settings are applied to the IDR slices in turn.
var settings = []sliceSetting{
	{0, 0, 0, 0},
	{2, 3, -2, 4},
	{1, 0, 0, -3},
	{2, -4, 5, 0},
	{0, 6, 6, 2},
	{0, -6, -6, -5},
	{2, 0, 0, 7},
}

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: mix_slice_deblock <input.264> <output.264>")
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	out, nrSlices, err := mixSliceDeblock(data)
	if err != nil {
		return err
	}
	if err := os.WriteFile(args[2], out, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "rewrote %d IDR slice headers\n", nrSlices)
	return nil
}

// mixSliceDeblock returns the Annex-B stream data with the IDR slice headers
// rewritten with settings in turn, and the number of slices rewritten.
func mixSliceDeblock(data []byte) ([]byte, int, error) {
	spsMap := make(map[uint32]*avc.SPS)
	ppsMap := make(map[uint32]*avc.PPS)
	var out []byte
	nrSlices := 0
	for _, nalu := range avc.ExtractNalusFromByteStream(data) {
		switch avc.GetNaluType(nalu[0]) {
		case avc.NALU_SPS:
			sps, err := avc.ParseSPSNALUnit(nalu, true)
			if err != nil {
				return nil, 0, fmt.Errorf("parse SPS: %w", err)
			}
			spsMap[sps.ParameterID] = sps
		case avc.NALU_PPS:
			pps, err := avc.ParsePPSNALUnit(nalu, spsMap)
			if err != nil {
				return nil, 0, fmt.Errorf("parse PPS: %w", err)
			}
			ppsMap[pps.PicParameterSetID] = pps
		case avc.NALU_IDR:
			sh, err := avc.ParseSliceHeader(nalu, spsMap, ppsMap)
			if err != nil {
				return nil, 0, fmt.Errorf("parse slice header: %w", err)
			}
			pps := ppsMap[sh.PicParamID]
			sps := spsMap[pps.SeqParameterSetID]
			nalu, err = rewriteSlice(nalu, sps, pps, settings[nrSlices%len(settings)])
			if err != nil {
				return nil, 0, fmt.Errorf("IDR slice %d: %w", nrSlices, err)
			}
			nrSlices++
		}
		out = append(out, 0, 0, 0, 1)
		out = append(out, nalu...)
	}
	return out, nrSlices, nil
}

// rewriteSlice returns the IDR I-slice nalu with the deblocking filter fields
// of its header, and for CAVLC its slice_qp_delta, changed as s says.
func rewriteSlice(nalu []byte, sps *avc.SPS, pps *avc.PPS, s sliceSetting) ([]byte, error) {
	if !sps.FrameMbsOnlyFlag || !pps.DeblockingFilterControlPresentFlag {
		return nil, fmt.Errorf("need frame_mbs_only_flag and deblocking_filter_control_present_flag")
	}
	r := bits.NewEBSPReaderFromSlice(nalu[1:])
	var buf bytes.Buffer
	w := bits.NewEBSPWriter(&buf)
	copyBits := func(n int) { w.Write(r.Read(n), n) }
	copyUE := func() { w.WriteExpGolomb(r.ReadExpGolomb()) }
	copySE := func() { writeSE(w, r.ReadSignedGolomb()) }

	copyUE() // first_mb_in_slice
	copyUE() // slice_type
	copyUE() // pic_parameter_set_id
	copyBits(int(sps.Log2MaxFrameNumMinus4 + 4))
	copyUE() // idr_pic_id
	switch sps.PicOrderCntType {
	case 0:
		copyBits(int(sps.Log2MaxPicOrderCntLsbMinus4 + 4))
		if pps.BottomFieldPicOrderInFramePresentFlag {
			copySE() // delta_pic_order_cnt_bottom
		}
	case 1:
		if !sps.DeltaPicOrderAlwaysZeroFlag {
			copySE() // delta_pic_order_cnt[0]
			if pps.BottomFieldPicOrderInFramePresentFlag {
				copySE() // delta_pic_order_cnt[1]
			}
		}
	}
	if pps.RedundantPicCntPresentFlag {
		copyUE()
	}
	copyBits(2) // no_output_of_prior_pics_flag, long_term_reference_flag

	sliceQPDelta := r.ReadSignedGolomb()
	if !pps.EntropyCodingModeFlag {
		sliceQPDelta += s.qpDeltaChange
	}
	writeSE(w, sliceQPDelta)

	// Replace the deblocking filter fields
	if r.ReadExpGolomb() != 1 {
		r.ReadSignedGolomb() // slice_alpha_c0_offset_div2
		r.ReadSignedGolomb() // slice_beta_offset_div2
	}
	w.WriteExpGolomb(s.disableDeblockingFilterIdc)
	if s.disableDeblockingFilterIdc != 1 {
		writeSE(w, s.alphaC0OffsetDiv2)
		writeSE(w, s.betaOffsetDiv2)
	}
	if err := r.AccError(); err != nil {
		return nil, fmt.Errorf("read slice header: %w", err)
	}

	if pps.EntropyCodingModeFlag {
		// cabac_alignment_one_bit until byte aligned, then the slice data bytes
		r.Read((8 - r.NrBitsRead()%8) % 8)
		for w.NrBitsInBuffer() != 0 {
			w.Write(1, 1)
		}
		for {
			b := r.Read(8)
			if r.AccError() != nil {
				break
			}
			w.Write(b, 8)
		}
	} else {
		// The macroblock layer bits up to the RBSP trailing bits
		for {
			more, err := r.MoreRbspData()
			if err != nil {
				return nil, err
			}
			if !more {
				break
			}
			copyBits(1)
		}
		w.WriteRbspTrailingBits()
	}
	if err := w.AccError(); err != nil {
		return nil, err
	}
	return append([]byte{nalu[0]}, buf.Bytes()...), nil
}

// writeSE writes v as se(v).
func writeSE(w *bits.EBSPWriter, v int) {
	if v > 0 {
		w.WriteExpGolomb(uint(2*v - 1))
	} else {
		w.WriteExpGolomb(uint(-2 * v))
	}
}
