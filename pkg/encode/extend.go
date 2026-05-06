package encode

import (
	"fmt"

	"github.com/Eyevinn/mp4ff/avc"
)

// LastFrameState returns the (frame_num, pic_order_cnt_lsb) of the last slice
// in an Annex-B bitstream. Useful for picking continuation values when
// extending an existing stream with EncodePSkipSlice. For the common case of
// appending one or more empty frames, AppendPSkipFrames wraps this whole flow
// into a single call.
//
// The returned values are taken from the very last coded slice (regardless of
// reference flag). When appending one P_Skip you typically want
//
//	frameNum, picOrderCntLsb, _ := encode.LastFrameState(stream)
//	pSkip, _ := encode.EncodePSkipSlice(sps, pps, frameNum+1, picOrderCntLsb+2, 0)
//
// Returns an error if the stream has no slices, or if the SPS uses
// pic_order_cnt_type != 0 (the only type supported by EncodePSkipSlice).
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
	if lastSPS != nil && lastSPS.PicOrderCntType != 0 {
		return 0, 0, fmt.Errorf("LastFrameState: pic_order_cnt_type=%d not supported (only type 0)",
			lastSPS.PicOrderCntType)
	}
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
// and the SPS to use pic_order_cnt_type=0. Slices use disable_deblocking=0.
//
// For more control (custom frame_num, custom POC stride, custom deblocking),
// use LastFrameState + EncodePSkipSlice directly.
func AppendPSkipFrames(annexB []byte, count uint32) ([]byte, error) {
	if count == 0 {
		return annexB, nil
	}
	sps, pps, err := parseSPSPPS(annexB)
	if err != nil {
		return nil, err
	}
	lastFn, lastLsb, err := LastFrameState(annexB)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(annexB), len(annexB)+int(count)*64)
	copy(out, annexB)
	for i := range count {
		pSkip, err := EncodePSkipSlice(sps, pps, lastFn+1+i, lastLsb+2+2*i, 0)
		if err != nil {
			return nil, fmt.Errorf("AppendPSkipFrames: %w", err)
		}
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
