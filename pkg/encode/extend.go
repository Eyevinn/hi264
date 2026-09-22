package encode

import (
	"fmt"

	"github.com/Eyevinn/mp4ff/avc"
)

// LastFrameState returns the (frame_num, pic_order_cnt_lsb) of the last slice
// in an Annex-B bitstream, exactly as coded.
//
// The values are taken from the very last coded slice regardless of its
// reference flag, which makes them the wrong basis for a continuation when the
// stream ends on a non-reference picture or when an earlier picture has a
// higher picture order count, as happens in a stream with B frames. Use
// AppendPSkipFrames or PSkipExtender, which follow the reference structure, unless
// you specifically want the raw last-slice values:
//
//	frameNum, picOrderCntLsb, _ := encode.LastFrameState(stream)
//	pSkip, _ := encode.EncodePSkipSlice(sps, pps, frameNum+1, picOrderCntLsb+2, 0)
//
// Returns an error if the stream has no slices, or if the SPS uses a
// pic_order_cnt_type other than 0 or 2 (the types EncodePSkipSlice supports).
//
// For a live stream, where the history is not available and the numbering has
// to be tracked as access units go past, use PSkipExtender instead.
func LastFrameState(annexB []byte) (frameNum uint32, picOrderCntLsb uint32, err error) {
	nalus := avc.ExtractNalusFromByteStream(annexB)
	spsMap := make(map[uint32]*avc.SPS)
	ppsMap := make(map[uint32]*avc.PPS)
	var lastSPS *avc.SPS
	var lastSlice *avc.SliceHeader
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		switch avc.GetNaluType(nalu[0]) {
		case avc.NALU_SPS:
			s, perr := avc.ParseSPSNALUnit(nalu, true)
			if perr != nil {
				return 0, 0, fmt.Errorf("LastFrameState: parse SPS: %w", perr)
			}
			spsMap[uint32(s.ParameterID)] = s
			lastSPS = s
		case avc.NALU_PPS:
			p, perr := avc.ParsePPSNALUnit(nalu, spsMap)
			if perr != nil {
				return 0, 0, fmt.Errorf("LastFrameState: parse PPS: %w", perr)
			}
			ppsMap[p.PicParameterSetID] = p
		case avc.NALU_IDR, avc.NALU_NON_IDR:
			sh, perr := avc.ParseSliceHeader(nalu, spsMap, ppsMap)
			if perr != nil {
				return 0, 0, fmt.Errorf("LastFrameState: parse slice header: %w", perr)
			}
			lastSlice = sh
		}
	}
	if lastSlice == nil {
		return 0, 0, fmt.Errorf("LastFrameState: no coded slice found")
	}
	if lastSPS != nil && lastSPS.PicOrderCntType != 0 && lastSPS.PicOrderCntType != 2 {
		return 0, 0, fmt.Errorf("LastFrameState: pic_order_cnt_type=%d not supported (only types 0 and 2)",
			lastSPS.PicOrderCntType)
	}
	// For pic_order_cnt_type=2 the slice header has no pic_order_cnt_lsb;
	// callers should treat the returned LSB as unused in that case.
	return uint32(lastSlice.FrameNum), uint32(lastSlice.PicOrderCntLsb), nil
}

// AppendPSkipFrames extends an existing Annex-B bitstream with `count` empty
// P_Skip frames whose frame_num and pic_order_cnt_lsb continue the source's
// last picture (stride 2 per appended frame). The source's SPS and PPS are
// reused as-is, so each appended frame copies pixels from the source's last
// reference picture.
//
// Returns the concatenated stream (original bytes + appended slices).
// Requires the source to contain at least one SPS, one PPS, and one slice,
// and the SPS to use pic_order_cnt_type 0 or 2. Slices use disable_deblocking=0.
//
// The continuation follows the stream's reference structure: frame_num advances
// over the last reference picture (a trailing non-reference picture does not
// move it) and the picture order count continues from the highest value in the
// stream, which in a stream with B frames is not the last picture in decode
// order. Both wrap at their SPS-defined maxima.
//
// For a live stream, where the history is not available, use PSkipExtender, which
// tracks the same state as access units go past. For more control (custom
// frame_num, custom POC stride, custom deblocking), use LastFrameState +
// EncodePSkipSlice directly.
func AppendPSkipFrames(annexB []byte, count uint32) ([]byte, error) {
	if count == 0 {
		return annexB, nil
	}
	sps, pps, err := parseSPSPPS(annexB)
	if err != nil {
		return nil, err
	}
	ext, err := NewPSkipExtender(sps, pps)
	if err != nil {
		return nil, fmt.Errorf("AppendPSkipFrames: %w", err)
	}
	if err := ext.ObserveAnnexB(annexB); err != nil {
		return nil, fmt.Errorf("AppendPSkipFrames: %w", err)
	}
	slices, err := ext.NextSlices(int(count))
	if err != nil {
		return nil, fmt.Errorf("AppendPSkipFrames: %w", err)
	}
	out := make([]byte, len(annexB), len(annexB)+int(count)*64)
	copy(out, annexB)
	for _, pSkip := range slices {
		out = append(out, pSkip...)
	}
	return out, nil
}

func parseSPSPPS(data []byte) (*avc.SPS, *avc.PPS, error) {
	nalus := avc.ExtractNalusFromByteStream(data)
	spsMap := make(map[uint32]*avc.SPS)
	var sps *avc.SPS
	var pps *avc.PPS
	for _, nalu := range nalus {
		if len(nalu) == 0 {
			continue
		}
		switch avc.GetNaluType(nalu[0]) {
		case avc.NALU_SPS:
			s, perr := avc.ParseSPSNALUnit(nalu, true)
			if perr != nil {
				return nil, nil, fmt.Errorf("parse SPS: %w", perr)
			}
			sps = s
			spsMap[uint32(s.ParameterID)] = s
		case avc.NALU_PPS:
			p, perr := avc.ParsePPSNALUnit(nalu, spsMap)
			if perr != nil {
				return nil, nil, fmt.Errorf("parse PPS: %w", perr)
			}
			pps = p
		}
	}
	if sps == nil {
		return nil, nil, fmt.Errorf("no SPS in stream")
	}
	if pps == nil {
		return nil, nil, fmt.Errorf("no PPS in stream")
	}
	return sps, pps, nil
}
