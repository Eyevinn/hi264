// Package decoder implements the top-level H.264/AVC decoder orchestration.
package decoder

import (
	"encoding/binary"
	"fmt"

	"github.com/Eyevinn/mp4ff/avc"

	"github.com/Eyevinn/hi264/internal/cabac"
	"github.com/Eyevinn/hi264/internal/cavlc"
	"github.com/Eyevinn/hi264/internal/context"
	"github.com/Eyevinn/hi264/internal/pred"
	"github.com/Eyevinn/hi264/internal/slice"
	"github.com/Eyevinn/hi264/internal/transform"
	"github.com/Eyevinn/hi264/pkg/frame"
)

// maxFrameSizeInMbs is the largest frame size in macroblocks defined by any
// H.264 level (5.2). It bounds allocations sized from untrusted SPS dimensions.
const maxFrameSizeInMbs = 139264

// Decoder is the H.264/AVC decoder.
type Decoder struct {
	spsMap      map[uint32]*avc.SPS
	ppsMap      map[uint32]*avc.PPS
	refFrame    *frame.Frame // most recently decoded reference frame (for P-slice)
	TraceMBCMP  bool         // emit MBCMP lines for FFmpeg comparison
	SkipDeblock bool         // skip deblocking filter for debugging
}

// ScalingMatrices holds effective scaling lists in raster order for dequantization.
// All lists are in raster scan order (row*width + col), converted from the
// zigzag scan order used in the bitstream.
type ScalingMatrices struct {
	IntraY4x4  [16]int32 // Intra luma 4x4 scaling list (SPS/PPS list 0)
	IntraCb4x4 [16]int32 // Intra chroma Cb 4x4 scaling list (SPS/PPS list 1)
	IntraCr4x4 [16]int32 // Intra chroma Cr 4x4 scaling list (SPS/PPS list 2)
	IntraY8x8  [64]int32 // Intra luma 8x8 scaling list (SPS/PPS list 6)
}

// Default 4x4 intra scaling list (Table 7-3) used when seq_scaling_list_present_flag[i]=0
// and the list would fall back to the default. Values are in zigzag scan order.
var defaultScalingList4x4Intra = [16]int{
	6, 13, 13, 20, 20, 20, 28, 28, 28, 28, 32, 32, 32, 37, 37, 42,
}

// Default 8x8 intra scaling list (Table 7-4) in raster scan order.
// Note: unlike the 4x4 default which is in zigzag scan order, the spec
// presents the 8x8 default as an 8x8 matrix (raster order).
var defaultScalingList8x8Intra = [64]int{
	6, 10, 13, 16, 18, 23, 25, 27,
	10, 11, 16, 18, 23, 25, 27, 29,
	13, 16, 18, 23, 25, 27, 29, 31,
	16, 18, 23, 25, 27, 29, 31, 33,
	18, 23, 25, 27, 29, 31, 33, 36,
	23, 25, 27, 29, 31, 33, 36, 38,
	25, 27, 29, 31, 33, 36, 38, 40,
	27, 29, 31, 33, 36, 38, 40, 42,
}

// buildScalingMatrices derives the effective scaling lists from SPS and PPS.
// Implements the fall-back rules from Table 7-2 of the H.264 spec:
//   - When seq/pic_scaling_list_present_flag[i]=0 (mp4ff returns nil),
//     fall back to the default table or copy from a previous list.
//   - Lists in the bitstream are in zigzag scan order; this function
//     converts them to raster scan order for use in dequantization.
func buildScalingMatrices(sps *avc.SPS, pps *avc.PPS) ScalingMatrices {
	sm := ScalingMatrices{}

	// Start with flat defaults (all 16s)
	for i := range sm.IntraY4x4 {
		sm.IntraY4x4[i] = 16
	}
	sm.IntraCb4x4 = sm.IntraY4x4
	sm.IntraCr4x4 = sm.IntraY4x4
	for i := range sm.IntraY8x8 {
		sm.IntraY8x8[i] = 16
	}

	// Apply SPS scaling lists with Table 7-2 fall-back
	if sps.SeqScalingMatrixPresentFlag {
		// List 0 (Intra Y 4x4): fall-back = Default_4x4_Intra
		if !applyScalingList4x4(&sm.IntraY4x4, sps.SeqScalingLists, 0) {
			applyDefault4x4Intra(&sm.IntraY4x4)
		}
		// List 1 (Intra Cb 4x4): fall-back = ScalingList4x4[0]
		if !applyScalingList4x4(&sm.IntraCb4x4, sps.SeqScalingLists, 1) {
			sm.IntraCb4x4 = sm.IntraY4x4
		}
		// List 2 (Intra Cr 4x4): fall-back = ScalingList4x4[1]
		if !applyScalingList4x4(&sm.IntraCr4x4, sps.SeqScalingLists, 2) {
			sm.IntraCr4x4 = sm.IntraCb4x4
		}
		// List 6 (Intra Y 8x8): fall-back = Default_8x8_Intra
		if !applyScalingList8x8(&sm.IntraY8x8, sps.SeqScalingLists, 6) {
			applyDefault8x8Intra(&sm.IntraY8x8)
		}
	}

	// PPS scaling lists override SPS with Table 7-2 fall-back
	if pps.PicScalingMatrixPresentFlag {
		// When pic_scaling_list_present_flag[i]=0:
		//   if seq_scaling_matrix_present_flag: keep SPS value (already in place)
		//   else: use default table / copy from previous list
		if sps.SeqScalingMatrixPresentFlag {
			// SPS values already in sm — only override with explicit PPS lists
			applyScalingList4x4(&sm.IntraY4x4, pps.PicScalingLists, 0)
			applyScalingList4x4(&sm.IntraCb4x4, pps.PicScalingLists, 1)
			applyScalingList4x4(&sm.IntraCr4x4, pps.PicScalingLists, 2)
			applyScalingList8x8(&sm.IntraY8x8, pps.PicScalingLists, 6)
		} else {
			// No SPS scaling — same fall-back as SPS
			if !applyScalingList4x4(&sm.IntraY4x4, pps.PicScalingLists, 0) {
				applyDefault4x4Intra(&sm.IntraY4x4)
			}
			if !applyScalingList4x4(&sm.IntraCb4x4, pps.PicScalingLists, 1) {
				sm.IntraCb4x4 = sm.IntraY4x4
			}
			if !applyScalingList4x4(&sm.IntraCr4x4, pps.PicScalingLists, 2) {
				sm.IntraCr4x4 = sm.IntraCb4x4
			}
			if !applyScalingList8x8(&sm.IntraY8x8, pps.PicScalingLists, 6) {
				applyDefault8x8Intra(&sm.IntraY8x8)
			}
		}
	}

	return sm
}

// applyScalingList4x4 converts a 4x4 scaling list from zigzag to raster order.
// Returns true if the list was present and applied, false if nil/missing.
func applyScalingList4x4(dst *[16]int32, lists []avc.ScalingList, idx int) bool {
	if idx >= len(lists) || lists[idx] == nil || len(lists[idx]) != 16 {
		return false
	}
	sl := lists[idx]
	for k := range 16 {
		dst[zigzag4x4[k]] = int32(sl[k])
	}
	return true
}

// applyScalingList8x8 converts an 8x8 scaling list from zigzag to raster order.
// Returns true if the list was present and applied, false if nil/missing.
func applyScalingList8x8(dst *[64]int32, lists []avc.ScalingList, idx int) bool {
	if idx >= len(lists) || lists[idx] == nil || len(lists[idx]) != 64 {
		return false
	}
	sl := lists[idx]
	for k := range 64 {
		dst[zigzag8x8[k]] = int32(sl[k])
	}
	return true
}

// applyDefault4x4Intra sets dst to the Default_4x4_Intra scaling list (Table 7-3),
// converted from zigzag to raster order.
func applyDefault4x4Intra(dst *[16]int32) {
	for k := range 16 {
		dst[zigzag4x4[k]] = int32(defaultScalingList4x4Intra[k])
	}
}

// applyDefault8x8Intra sets dst to the Default_8x8_Intra scaling list (Table 7-4).
// The 8x8 default is already in raster order (unlike the 4x4 default which is in zigzag).
func applyDefault8x8Intra(dst *[64]int32) {
	for k := range 64 {
		dst[k] = int32(defaultScalingList8x8Intra[k])
	}
}

// New creates a new H.264 decoder.
func New() *Decoder {
	return &Decoder{
		spsMap: make(map[uint32]*avc.SPS),
		ppsMap: make(map[uint32]*avc.PPS),
	}
}

// DecodeNALUs decodes a complete access unit (set of NALUs) and returns the reconstructed frame.
// For multi-frame streams, use DecodeAllFrames instead.
func (d *Decoder) DecodeNALUs(nalus [][]byte) (*frame.Frame, error) {
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		naluType := avc.NaluType(nalu[0] & 0x1f)

		switch naluType {
		case avc.NALU_SPS:
			sps, err := avc.ParseSPSNALUnit(nalu, true)
			if err != nil {
				return nil, fmt.Errorf("parse SPS: %w", err)
			}
			d.spsMap[sps.ParameterID] = sps
		case avc.NALU_PPS:
			pps, err := avc.ParsePPSNALUnit(nalu, d.spsMap)
			if err != nil {
				return nil, fmt.Errorf("parse PPS: %w", err)
			}
			d.ppsMap[pps.PicParameterSetID] = pps
		case avc.NALU_IDR:
			f, err := d.decodeIDR(nalu)
			if err != nil {
				return nil, err
			}
			d.refFrame = f
			return f, nil
		}
	}
	return nil, fmt.Errorf("no IDR NALU found")
}

// DecodeAllFrames decodes all frames from a set of NALUs (IDR and non-IDR).
// Non-IDR slices are decoded as P_Skip (copy from reference); an error is
// returned if a non-IDR slice contains non-skip macroblocks.
// Returns frames in decode order.
func (d *Decoder) DecodeAllFrames(nalus [][]byte) ([]*frame.Frame, error) {
	return d.decodeFrames(nalus, true)
}

// DecodeIDRFrames decodes only IDR frames from a set of NALUs, skipping
// all non-IDR slices. Returns frames in decode order.
func (d *Decoder) DecodeIDRFrames(nalus [][]byte) ([]*frame.Frame, error) {
	return d.decodeFrames(nalus, false)
}

func (d *Decoder) decodeFrames(nalus [][]byte, includeNonIDR bool) ([]*frame.Frame, error) {
	var frames []*frame.Frame

	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		naluType := avc.NaluType(nalu[0] & 0x1f)

		switch naluType {
		case avc.NALU_SPS:
			sps, err := avc.ParseSPSNALUnit(nalu, true)
			if err != nil {
				return frames, fmt.Errorf("parse SPS: %w", err)
			}
			d.spsMap[sps.ParameterID] = sps
		case avc.NALU_PPS:
			pps, err := avc.ParsePPSNALUnit(nalu, d.spsMap)
			if err != nil {
				return frames, fmt.Errorf("parse PPS: %w", err)
			}
			d.ppsMap[pps.PicParameterSetID] = pps
		case avc.NALU_IDR:
			f, err := d.decodeIDR(nalu)
			if err != nil {
				return frames, fmt.Errorf("IDR frame %d: %w", len(frames), err)
			}
			d.refFrame = f
			frames = append(frames, f)
		case 1: // non-IDR coded slice
			if !includeNonIDR {
				continue
			}
			f, err := d.decodePSkip(nalu)
			if err != nil {
				return frames, fmt.Errorf("p frame %d: %w", len(frames), err)
			}
			d.refFrame = f
			frames = append(frames, f)
		}
	}
	return frames, nil
}

// DecodeAnnexB decodes the first IDR frame from an Annex-B byte stream.
func (d *Decoder) DecodeAnnexB(data []byte) (*frame.Frame, error) {
	nalus := avc.ExtractNalusFromByteStream(data)
	return d.DecodeNALUs(nalus)
}

// DecodeAllAnnexB decodes all frames (IDR + P_Skip) from an Annex-B byte stream.
func (d *Decoder) DecodeAllAnnexB(data []byte) ([]*frame.Frame, error) {
	nalus := avc.ExtractNalusFromByteStream(data)
	return d.DecodeAllFrames(nalus)
}

// DecodeIDRAnnexB decodes only IDR frames from an Annex-B byte stream.
func (d *Decoder) DecodeIDRAnnexB(data []byte) ([]*frame.Frame, error) {
	nalus := avc.ExtractNalusFromByteStream(data)
	return d.DecodeIDRFrames(nalus)
}

// DecodeAVC decodes the first IDR frame from AVC-format data
// (each NALU preceded by a 4-byte big-endian length field).
func (d *Decoder) DecodeAVC(data []byte) (*frame.Frame, error) {
	nalus, err := extractAVCNalus(data)
	if err != nil {
		return nil, err
	}
	return d.DecodeNALUs(nalus)
}

// DecodeAllAVC decodes all frames from AVC-format data
// (each NALU preceded by a 4-byte big-endian length field).
func (d *Decoder) DecodeAllAVC(data []byte) ([]*frame.Frame, error) {
	nalus, err := extractAVCNalus(data)
	if err != nil {
		return nil, err
	}
	return d.DecodeAllFrames(nalus)
}

// DecodeIDRAVC decodes only IDR frames from AVC-format data
// (each NALU preceded by a 4-byte big-endian length field).
func (d *Decoder) DecodeIDRAVC(data []byte) ([]*frame.Frame, error) {
	nalus, err := extractAVCNalus(data)
	if err != nil {
		return nil, err
	}
	return d.DecodeIDRFrames(nalus)
}

// extractAVCNalus extracts NALUs from AVC-format data where each NALU
// is preceded by a 4-byte big-endian length field.
func extractAVCNalus(data []byte) ([][]byte, error) {
	var nalus [][]byte
	for len(data) >= 4 {
		length := int(binary.BigEndian.Uint32(data[:4]))
		data = data[4:]
		if length < 0 || length > len(data) {
			return nil, fmt.Errorf("AVC NALU length %d exceeds remaining data %d", length, len(data))
		}
		nalus = append(nalus, data[:length])
		data = data[length:]
	}
	return nalus, nil
}

// cloneFrame creates a deep copy of a frame.
func cloneFrame(src *frame.Frame) *frame.Frame {
	dst := frame.NewFrame(src.Width, src.Height)
	copy(dst.Y, src.Y)
	copy(dst.Cb, src.Cb)
	copy(dst.Cr, src.Cr)
	return dst
}

// decodePSkip decodes a P-slice where all MBs are P_Skip (copy from reference).
func (d *Decoder) decodePSkip(nalu []byte) (*frame.Frame, error) {
	if d.refFrame == nil {
		return nil, fmt.Errorf("P_Skip: no reference frame available")
	}

	// Parse slice header to validate, but for P_Skip we just copy the reference
	sh, err := avc.ParseSliceHeader(nalu, d.spsMap, d.ppsMap)
	if err != nil {
		return nil, fmt.Errorf("parse P-slice header: %w", err)
	}

	pps := d.ppsMap[sh.PicParamID]
	sps := d.spsMap[pps.SeqParameterSetID]

	width := int(sps.Width)
	height := int(sps.Height)

	// For P_Skip, verify all MBs are skip by reading mb_skip_run via CAVLC
	if !pps.EntropyCodingModeFlag {
		// CAVLC path: skip header at bit level, read mb_skip_run
		fullData := removeEBSPPrevention(nalu)
		br := cavlc.NewBitReader(fullData)
		nalRefIdc := (nalu[0] >> 5) & 0x3

		err = br.SkipSliceHeaderP(cavlc.SliceHeaderParams{
			FrameMbsOnly:                          sps.FrameMbsOnlyFlag,
			Log2MaxFrameNumMinus4:                 uint(sps.Log2MaxFrameNumMinus4),
			PicOrderCntType:                       uint(sps.PicOrderCntType),
			Log2MaxPicOrderCntLsbMinus4:           uint(sps.Log2MaxPicOrderCntLsbMinus4),
			BottomFieldPicOrderInFramePresentFlag: pps.BottomFieldPicOrderInFramePresentFlag,
			DeblockingFilterControlPresent:        pps.DeblockingFilterControlPresentFlag,
			RedundantPicCntPresentFlag:            pps.RedundantPicCntPresentFlag,
		}, nalRefIdc)
		if err != nil {
			return nil, fmt.Errorf("skip P-slice header: %w", err)
		}

		// Read mb_skip_run
		mbSkipRun, err := br.ReadUE()
		if err != nil {
			return nil, fmt.Errorf("read mb_skip_run: %w", err)
		}

		totalMBs := ((width + 15) / 16) * ((height + 15) / 16)
		if int(mbSkipRun) != totalMBs {
			return nil, fmt.Errorf("P_Skip: mb_skip_run=%d, expected %d (non-skip P-frames not yet supported)",
				mbSkipRun, totalMBs)
		}
	} else {
		// CABAC path: use parsed header to find CABAC data start
		sliceData := removeEBSPPrevention(nalu[sh.Size:])
		sliceQPY := 26 + int(pps.PicInitQpMinus26) + int(sh.SliceQPDelta)
		dec, err2 := cabac.NewDecoder(sliceData)
		if err2 != nil {
			return nil, fmt.Errorf("CABAC P_Skip: init decoder: %w", err2)
		}
		models := context.InitModels(sliceQPY, 0, int(sh.CabacInitIDC))
		ctx := models[:]

		totalMBs := ((width + 15) / 16) * ((height + 15) / 16)
		for mbIdx := range totalMBs {
			mbSkipFlag := dec.DecodeDecision(&ctx[11]) // ctxIdxInc=0 always for all-skip
			if mbSkipFlag != 1 {
				return nil, fmt.Errorf("CABAC P-frame: non-skip MB %d not supported (mb_skip_flag=%d)",
					mbIdx, mbSkipFlag)
			}
			isLast := mbIdx == totalMBs-1
			term := dec.DecodeTerminate()
			if isLast && term != 1 {
				return nil, fmt.Errorf("CABAC P_Skip: expected end_of_slice at last MB %d", mbIdx)
			}
			if !isLast && term != 0 {
				return nil, fmt.Errorf("CABAC P_Skip: unexpected end_of_slice at MB %d", mbIdx)
			}
		}
	}

	// P_Skip: all macroblocks copy from reference with zero motion vector
	// No deblocking needed (bS=0 for all-skip)
	return cloneFrame(d.refFrame), nil
}

// decodeIDR decodes an IDR frame.
func (d *Decoder) decodeIDR(nalu []byte) (*frame.Frame, error) {
	// Parse slice header
	sh, err := avc.ParseSliceHeader(nalu, d.spsMap, d.ppsMap)
	if err != nil {
		return nil, fmt.Errorf("parse slice header: %w", err)
	}

	pps := d.ppsMap[sh.PicParamID]
	sps := d.spsMap[pps.SeqParameterSetID]

	// Calculate dimensions
	width := int(sps.Width)
	height := int(sps.Height)
	mbWidth := (width + 15) / 16
	mbHeight := (height + 15) / 16

	// The picture dimensions come from the SPS (untrusted input); reject a
	// frame whose macroblock count exceeds the largest defined H.264 level
	// (5.2, MaxFrameSizeInMbs = 139264) before it is used to size per-macroblock
	// allocations, otherwise a crafted SPS can request a huge allocation.
	if mbWidth <= 0 || mbHeight <= 0 || mbWidth*mbHeight > maxFrameSizeInMbs {
		return nil, fmt.Errorf("frame size %dx%d mbs exceeds maximum %d",
			mbWidth, mbHeight, maxFrameSizeInMbs)
	}

	// Calculate slice QP
	sliceQPY := 26 + int(pps.PicInitQpMinus26) + int(sh.SliceQPDelta)

	// Determine chroma array type
	chromaArrayType := 1 // 4:2:0 default
	if sps.ChromaFormatIDC == 0 {
		chromaArrayType = 0 // monochrome
	}

	bitDepthY := 8 + int(sps.BitDepthLumaMinus8)
	bitDepthC := 8 + int(sps.BitDepthChromaMinus8)

	var sc *slice.SliceContext
	if pps.EntropyCodingModeFlag {
		// CABAC path
		sliceData := removeEBSPPrevention(nalu[sh.Size:])
		var err2 error
		sc, err2 = slice.DecodeSliceData(sliceData, sliceQPY, mbWidth, mbHeight,
			pps.Transform8x8ModeFlag, chromaArrayType, bitDepthY, bitDepthC,
			int(pps.ChromaQpIndexOffset), d.TraceMBCMP)
		if err2 != nil {
			return nil, fmt.Errorf("decode slice data (CABAC): %w", err2)
		}
	} else {
		// CAVLC path: operate on full EBSP-decoded NALU, skip header at bit level
		fullData := removeEBSPPrevention(nalu)
		br := cavlc.NewBitReader(fullData)

		err = br.SkipSliceHeaderIDR(cavlc.SliceHeaderParams{
			FrameMbsOnly:                          sps.FrameMbsOnlyFlag,
			Log2MaxFrameNumMinus4:                 uint(sps.Log2MaxFrameNumMinus4),
			PicOrderCntType:                       uint(sps.PicOrderCntType),
			Log2MaxPicOrderCntLsbMinus4:           uint(sps.Log2MaxPicOrderCntLsbMinus4),
			BottomFieldPicOrderInFramePresentFlag: pps.BottomFieldPicOrderInFramePresentFlag,
			DeblockingFilterControlPresent:        pps.DeblockingFilterControlPresentFlag,
			RedundantPicCntPresentFlag:            pps.RedundantPicCntPresentFlag,
		})
		if err != nil {
			return nil, fmt.Errorf("skip slice header (CAVLC): %w", err)
		}

		var err2 error
		sc, err2 = slice.DecodeSliceDataCAVLC(br, sliceQPY, mbWidth, mbHeight,
			pps.Transform8x8ModeFlag, chromaArrayType, bitDepthY, bitDepthC,
			int(pps.ChromaQpIndexOffset), d.TraceMBCMP)
		if err2 != nil {
			return nil, fmt.Errorf("decode slice data (CAVLC): %w", err2)
		}
	}

	// Reconstruct frame
	f := frame.NewFrame(width, height)

	// Extract color space metadata from VUI if present
	if sps.VUI != nil {
		if sps.VUI.ColourDescriptionFlag {
			f.ColorDescriptionValid = true
			f.MatrixCoefficients = sps.VUI.MatrixCoefficients
		}
		f.VideoFullRangeFlag = sps.VUI.VideoFullRangeFlag
	}

	err = reconstructFrame(sc, f, sps, pps)
	if err != nil {
		return nil, fmt.Errorf("reconstruct frame: %w", err)
	}

	// Apply deblocking filter
	if sh.DisableDeblockingFilterIDC != 1 && !d.SkipDeblock {
		frame.Deblock(f, sc,
			int(sh.SliceAlphaC0OffsetDiv2)*2,
			int(sh.SliceBetaOffsetDiv2)*2)
	}

	return f, nil
}

// removeEBSPPrevention removes emulation prevention bytes (0x03 after 0x00 0x00).
func removeEBSPPrevention(data []byte) []byte {
	result := make([]byte, 0, len(data))
	i := 0
	for i < len(data) {
		if i+2 < len(data) && data[i] == 0 && data[i+1] == 0 && data[i+2] == 3 {
			result = append(result, 0, 0)
			i += 3 // skip the 0x03
		} else {
			result = append(result, data[i])
			i++
		}
	}
	return result
}

// reconstructFrame reconstructs the decoded picture from the parsed macroblock data.
func reconstructFrame(sc *slice.SliceContext, f *frame.Frame, sps *avc.SPS, pps *avc.PPS) error {
	sm := buildScalingMatrices(sps, pps)

	for mbIdx := 0; mbIdx < sc.TotalMBs; mbIdx++ {
		mbX := mbIdx % sc.MBWidth
		mbY := mbIdx / sc.MBWidth
		mb := &sc.MBs[mbIdx]

		if mb.MBType >= 1 && mb.MBType <= 24 {
			// I_16x16
			err := reconstructI16x16(sc, f, mbIdx, mbX, mbY, mb, &sm)
			if err != nil {
				return fmt.Errorf("reconstruct I_16x16 mb %d: %w", mbIdx, err)
			}
		} else if mb.MBType == slice.MBTypeINxN {
			if mb.TransformSize8x8 {
				err := reconstructI8x8(sc, f, mbIdx, mbX, mbY, mb, &sm)
				if err != nil {
					return fmt.Errorf("reconstruct I_8x8 mb %d: %w", mbIdx, err)
				}
			} else {
				err := reconstructI4x4(sc, f, mbIdx, mbX, mbY, mb, &sm)
				if err != nil {
					return fmt.Errorf("reconstruct I_4x4 mb %d: %w", mbIdx, err)
				}
			}
		}

		// Reconstruct chroma
		if sc.ChromaArrayType != 0 {
			reconstructChroma(sc, f, mbIdx, mbX, mbY, mb, &sm)
		}
	}

	return nil
}

// reconstructI16x16 reconstructs an I_16x16 macroblock.
func reconstructI16x16(sc *slice.SliceContext, f *frame.Frame,
	mbIdx, mbX, mbY int, mb *slice.MBData, sm *ScalingMatrices) error {
	// 1. Get prediction block
	top, left, topLeft, hasTop, hasLeft := getLuma16x16Neighbors(f, mbX, mbY)
	var topSlice, leftSlice []uint8
	if hasTop {
		topSlice = top[:]
	}
	if hasLeft {
		leftSlice = left[:]
	}
	predBlock, err := pred.Predict16x16(mb.IntraPredMode16x16, topSlice, leftSlice, topLeft)
	if err != nil {
		return err
	}

	// 2. Inverse transform DC coefficients
	// DC coefficients from CABAC are in zigzag scan order; Hadamard expects raster (row-major)
	var dcMatrix [16]int32
	for k := range 16 {
		dcMatrix[zigzag4x4[k]] = mb.Intra16x16DCLevel[k]
	}
	dcTransformed := transform.InverseHadamard4x4(dcMatrix)
	dcScaledRaster := transform.DequantDC4x4(dcTransformed, mb.QPY, sm.IntraY4x4[0])
	// Remap scaled DC from raster to z-scan order for block assignment
	var dcScaled [16]int32
	for i := range 16 {
		dcScaled[i] = dcScaledRaster[zScanToRaster[i]]
	}

	// 3. For each 4x4 block, apply inverse transform and add prediction
	sl := &sm.IntraY4x4
	var lumaBlock [16][16]uint8
	for i := range 16 {
		// Map 4x4 block index to position
		bx := inverseRasterX4x4[i]
		by := inverseRasterY4x4[i]

		// Build coefficient block: DC from Hadamard, AC from residual
		// AC coefficients need zigzag scan conversion from CABAC order to matrix order
		var block4x4 [16]int32
		block4x4[0] = dcScaled[i]
		for j := range 15 {
			block4x4[zigzag4x4AC[j]] = mb.Intra16x16ACLevel[i][j]
		}

		// Dequantize AC coefficients (DC already dequantized)
		dequantBlock := transform.Dequant4x4(block4x4, mb.QPY, sl)
		// Restore the already-scaled DC
		dequantBlock[0] = dcScaled[i]

		// Inverse transform
		residual := transform.InverseTransform4x4(dequantBlock)

		// Add prediction + residual, clip to [0,255]
		for y := range 4 {
			for x := range 4 {
				val := int32(predBlock[by+y][bx+x]) + residual[y*4+x]
				lumaBlock[by+y][bx+x] = clip8(val)
			}
		}
	}

	f.SetLuma16x16(mbX, mbY, lumaBlock)
	return nil
}

// reconstructI4x4 reconstructs an I_4x4 macroblock.
func reconstructI4x4(sc *slice.SliceContext, f *frame.Frame,
	mbIdx, mbX, mbY int, mb *slice.MBData, sm *ScalingMatrices) error {
	x0 := mbX * 16
	y0 := mbY * 16
	sl := &sm.IntraY4x4

	for i := range 16 {
		bx := inverseRasterX4x4[i]
		by := inverseRasterY4x4[i]

		// Get reference samples for this 4x4 block
		ref := getLuma4x4Neighbors(f, x0+bx, y0+by, i, mbX, mbY, sc.MBWidth, sc.MBHeight)

		// Predict
		leftAvail := x0+bx > 0
		topAvail := y0+by > 0
		predBlock := pred.Predict4x4(mb.Intra4x4PredMode[i], ref, leftAvail, topAvail)

		// Apply zigzag scan to convert from CABAC scan order to matrix order
		var coeffs [16]int32
		for j := range 16 {
			coeffs[zigzag4x4[j]] = mb.LumaLevel4x4[i][j]
		}
		// Dequantize + inverse transform
		dequant := transform.Dequant4x4(coeffs, mb.QPY, sl)
		residual := transform.InverseTransform4x4(dequant)

		// Reconstruct
		for y := range 4 {
			for x := range 4 {
				val := int32(predBlock[y][x]) + residual[y*4+x]
				f.SetLumaPixel(x0+bx+x, y0+by+y, clip8(val))
			}
		}
	}

	return nil
}

// reconstructI8x8 reconstructs an I_8x8 macroblock.
func reconstructI8x8(sc *slice.SliceContext, f *frame.Frame,
	mbIdx, mbX, mbY int, mb *slice.MBData, sm *ScalingMatrices) error {
	x0 := mbX * 16
	y0 := mbY * 16
	frameW := sc.MBWidth * 16
	frameH := sc.MBHeight * 16
	sl := &sm.IntraY8x8

	for i := range 4 {
		bx := (i % 2) * 8
		by := (i / 2) * 8

		// 1. Get filtered reference samples
		ref := getLuma8x8Neighbors(f, x0+bx, y0+by, i, frameW, frameH)

		// 2. Predict
		leftAvail := x0+bx > 0
		topAvail := y0+by > 0
		predBlock := pred.Predict8x8(mb.Intra8x8PredMode[i], ref, leftAvail, topAvail)

		// 3. Scan order conversion → matrix raster order
		var coeffs [64]int32
		if sc.IsCAVLC {
			// CAVLC: already stored in raster order by decode
			coeffs = mb.LumaLevel8x8[i]
		} else {
			// CABAC: convert from zigzag scan order to raster
			for j := range 64 {
				coeffs[zigzag8x8[j]] = mb.LumaLevel8x8[i][j]
			}
		}

		// 4. Dequantize
		dequant := transform.Dequant8x8(coeffs, mb.QPY, sl)

		// 5. Inverse transform
		residual := transform.InverseTransform8x8(dequant)

		// 6. Reconstruct: prediction + residual, clip to [0,255]
		for y := range 8 {
			for x := range 8 {
				val := int32(predBlock[y][x]) + residual[y*8+x]
				f.SetLumaPixel(x0+bx+x, y0+by+y, clip8(val))
			}
		}

	}

	return nil
}

// getLuma8x8Neighbors returns 25 filtered reference samples for 8x8 intra prediction.
// Layout: [L7, L6, L5, L4, L3, L2, L1, L0, TL, T0..T7, T8..T15]
// Handles availability, substitution (8.3.2.2.2), and filtering (8.3.2.2.3).
func getLuma8x8Neighbors(f *frame.Frame, blkX, blkY int, i8x8 int, frameW, frameH int) [25]uint8 {
	var ref [25]uint8
	var avail [25]bool

	// Left: ref[0]=L7(bottom)..ref[7]=L0(top) = p[-1,7]..p[-1,0]
	for i := range 8 {
		px, py := blkX-1, blkY+7-i
		if px >= 0 && py >= 0 && py < frameH {
			ref[i] = f.GetLumaPixel(px, py)
			avail[i] = true
		}
	}

	// Top-left: ref[8] = p[-1,-1]
	if blkX > 0 && blkY > 0 {
		ref[8] = f.GetLumaPixel(blkX-1, blkY-1)
		avail[8] = true
	}

	// Top: ref[9]=T0..ref[16]=T7 = p[0,-1]..p[7,-1]
	for i := range 8 {
		px, py := blkX+i, blkY-1
		if py >= 0 && px >= 0 && px < frameW {
			ref[9+i] = f.GetLumaPixel(px, py)
			avail[9+i] = true
		}
	}

	// Top-right: ref[17]=T8..ref[24]=T15 = p[8,-1]..p[15,-1]
	// Not available for 8x8 block index 3 (bottom-right in MB)
	topRightOK := i8x8 != 3
	for i := range 8 {
		px, py := blkX+8+i, blkY-1
		if topRightOK && py >= 0 && px >= 0 && px < frameW {
			ref[17+i] = f.GetLumaPixel(px, py)
			avail[17+i] = true
		}
	}

	// Substitution (section 8.3.2.2.2)
	// Scan order: ref[0] (bottom-left) to ref[24] (top-right)
	firstAvailIdx := -1
	for i := range 25 {
		if avail[i] {
			firstAvailIdx = i
			break
		}
	}
	if firstAvailIdx == -1 {
		// No available samples - use DC value
		for i := range ref {
			ref[i] = 128
		}
	} else {
		// Fill all positions before firstAvailIdx with its value
		for i := 0; i < firstAvailIdx; i++ {
			ref[i] = ref[firstAvailIdx]
		}
		// Fill forward: unavailable positions get previous value
		for i := firstAvailIdx + 1; i < 25; i++ {
			if !avail[i] {
				ref[i] = ref[i-1]
			}
		}
	}

	// Filtering (section 8.3.2.2.3) - 1-2-1 low-pass filter
	return filterRefSamples8x8(ref)
}

// filterRefSamples8x8 applies the 1-2-1 low-pass filter to 8x8 reference samples.
func filterRefSamples8x8(ref [25]uint8) [25]uint8 {
	var f [25]uint8
	// Bottom-left edge: p'[-1,7] = (p[-1,6] + 3*p[-1,7] + 2) >> 2
	f[0] = uint8((int(ref[1]) + 3*int(ref[0]) + 2) >> 2)
	// Interior samples (left column, TL, top row)
	for i := 1; i < 24; i++ {
		f[i] = uint8((int(ref[i-1]) + 2*int(ref[i]) + int(ref[i+1]) + 2) >> 2)
	}
	// Top-right edge: p'[15,-1] = (p[14,-1] + 3*p[15,-1] + 2) >> 2
	f[24] = uint8((int(ref[23]) + 3*int(ref[24]) + 2) >> 2)
	return f
}

// reconstructChroma reconstructs both chroma components for a macroblock.
func reconstructChroma(sc *slice.SliceContext, f *frame.Frame,
	mbIdx, mbX, mbY int, mb *slice.MBData, sm *ScalingMatrices) {
	chromaSL := [2]*[16]int32{&sm.IntraCb4x4, &sm.IntraCr4x4}

	for iCbCr := range 2 {
		sl := chromaSL[iCbCr]

		// Get chroma prediction
		top, left, topLeft, hasTop, hasLeft := getChromaNeighbors(f, iCbCr, mbX, mbY)
		var topSlice, leftSlice []uint8
		if hasTop {
			topSlice = top[:]
		}
		if hasLeft {
			leftSlice = left[:]
		}
		predBlock := pred.PredictChroma(mb.IntraChromaPredMode, topSlice, leftSlice, topLeft, 8)

		// Inverse Hadamard on DC coefficients
		var dcCoeffs [4]int32
		copy(dcCoeffs[:], mb.ChromaDCLevel[iCbCr][:])
		dcTransformed := transform.InverseHadamard2x2(dcCoeffs)

		// Calculate chroma QP
		qpc := chromaQP(mb.QPY + sc.ChromaQpIndexOffset)
		dcScaled := transform.DequantChromaDC2x2(dcTransformed, qpc, sl[0])

		// For each 4x4 chroma block
		var chromaBlock [8][8]uint8
		for blk := range 4 {
			bx := (blk % 2) * 4
			by := (blk / 2) * 4

			var block4x4 [16]int32
			block4x4[0] = dcScaled[blk]
			if mb.CBPChroma > 1 {
				// Apply zigzag scan conversion for AC coefficients
				for j := range 15 {
					block4x4[zigzag4x4AC[j]] = mb.ChromaACLevel[iCbCr][blk][j]
				}
			}

			dequant := transform.Dequant4x4(block4x4, qpc, sl)
			dequant[0] = dcScaled[blk]
			residual := transform.InverseTransform4x4(dequant)

			for y := range 4 {
				for x := range 4 {
					val := int32(predBlock[by+y][bx+x]) + residual[y*4+x]
					chromaBlock[by+y][bx+x] = clip8(val)
				}
			}
		}

		f.SetChroma8x8(iCbCr, mbX, mbY, chromaBlock)
	}
}

// getLuma16x16Neighbors returns the reference samples for 16x16 luma prediction.
func getLuma16x16Neighbors(f *frame.Frame, mbX, mbY int) (
	top [16]uint8, left [16]uint8, topLeft uint8, hasTop, hasLeft bool) {
	x0 := mbX * 16
	y0 := mbY * 16

	if mbY > 0 {
		hasTop = true
		for x := range 16 {
			top[x] = f.GetLumaPixel(x0+x, y0-1)
		}
	}

	if mbX > 0 {
		hasLeft = true
		for y := range 16 {
			left[y] = f.GetLumaPixel(x0-1, y0+y)
		}
	}

	if mbX > 0 && mbY > 0 {
		topLeft = f.GetLumaPixel(x0-1, y0-1)
	}

	return
}

// getLuma4x4Neighbors returns the 13 reference samples for 4x4 prediction.
// topRightNotAvail4x4 lists 4x4 block indices where the upper-right 4 samples
// are never available (the containing block has a higher z-scan index or is in
// the not-yet-decoded right MB).
var topRightNotAvail4x4 = [16]bool{
	false, false, false, true, // blocks 0-3: block 3's TR is in undecoded block 6
	false, false, false, true, // blocks 4-7: block 7's TR is in right MB
	false, false, false, true, // blocks 8-11: block 11's TR is in undecoded block 12
	false, false, false, true, // blocks 12-15: block 15's TR is in right MB
}

func getLuma4x4Neighbors(f *frame.Frame, x0, y0 int, blkIdx int, mbX, mbY int, mbW, mbH int) [13]uint8 {
	var ref [13]uint8
	frameW := mbW * 16
	frameH := mbH * 16
	// ref[0..3] = L3,L2,L1,L0 (left column, bottom to top)
	// ref[4]    = TL (top-left)
	// ref[5..12]= T0..T7 (top row + upper-right)

	for i := range 4 {
		if x0 > 0 && y0+3-i >= 0 && y0+3-i < frameH {
			ref[i] = f.GetLumaPixel(x0-1, y0+3-i)
		} else {
			ref[i] = 128
		}
	}

	if x0 > 0 && y0 > 0 {
		ref[4] = f.GetLumaPixel(x0-1, y0-1)
	} else {
		ref[4] = 128
	}

	// Top samples T0..T3
	for i := range 4 {
		if y0 > 0 && x0+i >= 0 && x0+i < frameW {
			ref[5+i] = f.GetLumaPixel(x0+i, y0-1)
		} else {
			ref[5+i] = 128
		}
	}

	// Top-right samples T4..T7: check availability
	trAvail := true
	if topRightNotAvail4x4[blkIdx] {
		trAvail = false
	} else if blkIdx == 5 && mbX >= mbW-1 {
		// Block 5 (12,0): TR is in MB above-right, not available at right edge
		trAvail = false
	} else if blkIdx == 13 {
		// Block 13 (12,8): TR is in right MB (not decoded) — already handled by table
		trAvail = false
	}

	if trAvail {
		for i := 4; i < 8; i++ {
			if y0 > 0 && x0+i >= 0 && x0+i < frameW {
				ref[5+i] = f.GetLumaPixel(x0+i, y0-1)
			} else {
				ref[5+i] = 128
			}
		}
	} else {
		// When upper-right is not available, fill T4..T7 with T3
		for i := 4; i < 8; i++ {
			ref[5+i] = ref[8] // ref[8] = T3
		}
	}

	return ref
}

// getChromaNeighbors returns reference samples for chroma prediction.
func getChromaNeighbors(f *frame.Frame, comp int, mbX, mbY int) (
	top [8]uint8, left [8]uint8, topLeft uint8, hasTop, hasLeft bool) {
	x0 := mbX * 8
	y0 := mbY * 8

	if mbY > 0 {
		hasTop = true
		for x := range 8 {
			top[x] = f.GetChromaPixel(comp, x0+x, y0-1)
		}
	}

	if mbX > 0 {
		hasLeft = true
		for y := range 8 {
			left[y] = f.GetChromaPixel(comp, x0-1, y0+y)
		}
	}

	if mbX > 0 && mbY > 0 {
		topLeft = f.GetChromaPixel(comp, x0-1, y0-1)
	}

	return
}

// inverseRasterX4x4 maps 4x4 block index (0-15) to x position.
// Uses H.264 spec hierarchical scan (Table 6-2 / equations 6-17, 6-18):
// Outer level: 8x8 blocks in raster scan; Inner level: 4x4 blocks within each 8x8.
var inverseRasterX4x4 = [16]int{
	0, 4, 0, 4, 8, 12, 8, 12,
	0, 4, 0, 4, 8, 12, 8, 12,
}

// inverseRasterY4x4 maps 4x4 block index (0-15) to y position.
var inverseRasterY4x4 = [16]int{
	0, 0, 4, 4, 0, 0, 4, 4,
	8, 8, 12, 12, 8, 8, 12, 12,
}

// zScanToRaster maps z-scan 4x4 block index to raster index (row*4+col).
// This is an involution (self-inverse permutation).
var zScanToRaster = [16]int{0, 1, 4, 5, 2, 3, 6, 7, 8, 9, 12, 13, 10, 11, 14, 15}

// zigzag4x4 maps CABAC scan position to 4x4 matrix position (Table 8-13 of the spec).
var zigzag4x4 = [16]int{
	0, 1, 4, 8, 5, 2, 3, 6,
	9, 12, 13, 10, 7, 11, 14, 15,
}

// zigzag4x4AC maps CABAC scan position to matrix position for AC coefficients
// (positions 1-15, skipping DC at position 0).
var zigzag4x4AC = [15]int{
	1, 4, 8, 5, 2, 3, 6,
	9, 12, 13, 10, 7, 11, 14, 15,
}

// zigzag8x8 maps CABAC scan position to 8x8 matrix position (Table 8-14).
var zigzag8x8 = [64]int{
	0, 1, 8, 16, 9, 2, 3, 10,
	17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34,
	27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36,
	29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46,
	53, 60, 61, 54, 47, 55, 62, 63,
}

// chromaQP maps luma QP to chroma QP (Table 8-15).
func chromaQP(qpY int) int {
	if qpY < 0 {
		qpY = 0
	}
	if qpY < 30 {
		return qpY
	}
	if qpY > 51 {
		return 51
	}
	// QP mapping table for indices 30-51
	qpcTable := []int{
		29, 30, 31, 32, 32, 33, 34, 34,
		35, 35, 36, 36, 37, 37, 37, 38,
		38, 38, 39, 39, 39, 39,
	}
	return qpcTable[qpY-30]
}

func clip8(val int32) uint8 {
	if val < 0 {
		return 0
	}
	if val > 255 {
		return 255
	}
	return uint8(val)
}
