package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/Eyevinn/hi264/pkg/decoder"
	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/hi264/pkg/yuv"
)

// spsNALU returns a Baseline SPS NAL unit with the given ID, picture size in macroblocks and
// log2_max_frame_num_minus4 (also used for log2_max_pic_order_cnt_lsb_minus4).
func spsNALU(id, mbWidth, mbHeight, log2MaxMinus4 uint32) []byte {
	w := encode.NewBitWriter()
	w.WriteBits(66, 8)   // profile_idc: Baseline
	w.WriteBits(0xC0, 8) // constraint_set0_flag and constraint_set1_flag
	w.WriteBits(30, 8)   // level_idc
	w.WriteUE(id)        // seq_parameter_set_id
	w.WriteUE(log2MaxMinus4)
	w.WriteUE(0) // pic_order_cnt_type
	w.WriteUE(log2MaxMinus4)
	w.WriteUE(1)            // max_num_ref_frames
	w.WriteBit(0)           // gaps_in_frame_num_value_allowed_flag
	w.WriteUE(mbWidth - 1)  // pic_width_in_mbs_minus1
	w.WriteUE(mbHeight - 1) // pic_height_in_map_units_minus1
	w.WriteBit(1)           // frame_mbs_only_flag
	w.WriteBit(0)           // direct_8x8_inference_flag
	w.WriteBit(0)           // frame_cropping_flag
	w.WriteBit(0)           // vui_parameters_present_flag
	w.WriteBit(1)           // rbsp_stop_one_bit
	w.AlignToByte()
	return encode.BuildNALU(byte(avc.NALU_SPS), 3, w.Bytes())
}

// ppsNALU returns a CAVLC PPS NAL unit that refers to SPS spsID.
func ppsNALU(id, spsID uint32) []byte {
	w := encode.NewBitWriter()
	w.WriteUE(id)     // pic_parameter_set_id
	w.WriteUE(spsID)  // seq_parameter_set_id
	w.WriteBit(0)     // entropy_coding_mode_flag
	w.WriteBit(0)     // bottom_field_pic_order_in_frame_present_flag
	w.WriteUE(0)      // num_slice_groups_minus1
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
	w.AlignToByte()
	return encode.BuildNALU(byte(avc.NALU_PPS), 3, w.Bytes())
}

// The sample entry lists SPS 0 (32x32) and its PPS first, but the segment's slices use SPS 1
// (48x32) and PPS 1. The appended black IDR and P_Skip frames must use those, so that they have
// the size of the source pictures.
func TestExtendUsesParameterSetsOfLastSlice(t *testing.T) {
	silenceStdout(t)
	dir := t.TempDir()
	spsNALUs := [][]byte{spsNALU(0, 2, 2, 0), spsNALU(1, 3, 2, 4)}
	ppsNALUs := [][]byte{ppsNALU(0, 0), ppsNALU(1, 1)}

	init := mp4.CreateEmptyInit()
	init.AddEmptyTrack(25, "video", "und")
	trak := init.Moov.Trak
	if err := trak.SetAVCDescriptor("avc1", spsNALUs, ppsNALUs, true); err != nil {
		t.Fatalf("SetAVCDescriptor: %v", err)
	}
	initPath := filepath.Join(dir, "init.mp4")
	writeBox(t, initPath, func(f *os.File) error { return init.Encode(f) })

	spsMap := map[uint32]*avc.SPS{}
	for _, nalu := range spsNALUs {
		sps, err := avc.ParseSPSNALUnit(nalu, true)
		if err != nil {
			t.Fatalf("ParseSPSNALUnit: %v", err)
		}
		spsMap[sps.ParameterID] = sps
	}
	pps1, err := avc.ParsePPSNALUnit(ppsNALUs[1], spsMap)
	if err != nil {
		t.Fatalf("ParsePPSNALUnit: %v", err)
	}
	const width, height = 48, 32
	grid, colors := yuv.SolidGrid(width, height, yuv.Color{Y: 180, Cb: 128, Cr: 128})
	plane, err := yuv.GridToPlaneGrid(grid, colors)
	if err != nil {
		t.Fatalf("GridToPlaneGrid: %v", err)
	}
	idr, err := encode.GenerateIDRWithSPSPPS(encode.EncodeParams{Width: width, Height: height, QP: 26},
		spsMap[1], pps1, plane, 0)
	if err != nil {
		t.Fatalf("GenerateIDRWithSPSPPS: %v", err)
	}
	sample := avc.ConvertByteStreamToNaluSample(idr)
	frag, err := mp4.CreateFragment(1, trak.Tkhd.TrackID)
	if err != nil {
		t.Fatalf("CreateFragment: %v", err)
	}
	frag.AddFullSample(mp4.FullSample{
		Sample: mp4.Sample{Flags: mp4.SyncSampleFlags, Dur: 1, Size: uint32(len(sample))},
		Data:   sample,
	})
	seg := mp4.NewMediaSegment()
	seg.AddFragment(frag)
	segPath := filepath.Join(dir, "seg.m4s")
	writeBox(t, segPath, func(f *os.File) error { return seg.Encode(f) })

	out := filepath.Join(dir, "out.m4s")
	if err := extendSegment(initPath, segPath, out, 3, true); err != nil {
		t.Fatalf("extendSegment: %v", err)
	}

	nalus := append(append([][]byte{}, spsNALUs...), ppsNALUs...)
	for _, s := range readFullSamples(t, out) {
		sampleNALUs, err := avc.GetNalusFromSample(s.Data)
		if err != nil {
			t.Fatalf("GetNalusFromSample: %v", err)
		}
		nalus = append(nalus, sampleNALUs...)
	}
	dec := decoder.New()
	dec.SkipDeblock = true
	frames, err := dec.DecodeAllFrames(nalus)
	if err != nil {
		t.Fatalf("DecodeAllFrames: %v", err)
	}
	if len(frames) != 4 {
		t.Fatalf("decoded %d frames, want 4", len(frames))
	}
	for i, f := range frames {
		if f.Width != width || f.Height != height {
			t.Errorf("frame %d is %dx%d, want %dx%d", i, f.Width, f.Height, width, height)
		}
	}
	if y := frames[1].Y[0]; y != 16 {
		t.Errorf("black IDR has luma %d, want 16", y)
	}
}
