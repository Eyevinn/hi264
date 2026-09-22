package encode

import (
	"fmt"

	"github.com/Eyevinn/mp4ff/avc"
)

// PSkipExtender continues an existing H.264 stream with P_Skip frames, tracking the frame
// numbering as access units go past instead of re-parsing the stream. Where AppendPSkipFrames
// rewrites a complete Annex-B bitstream, a PSkipExtender can be asked for continuation frames at
// any point, which is what a live pipeline needs when its source stalls: a handful of bytes per
// frame holds the last picture on screen until real video returns, so the receiver never sees the
// stream stop.
//
//	ext, err := encode.NewPSkipExtenderFromDecConfRec(avcC)
//	for _, au := range accessUnits { // every real access unit, as it is sent
//	    err = ext.ObserveAVCCSample(au)
//	}
//	filler, err := ext.NextSlices(25) // one second at 25 fps, continuing the stream
//
// The source's SPS and PPS are reused verbatim, so no parameter set is re-signalled and a decoder
// sees no configuration change. Two responsibilities stay with the caller:
//
//   - Timing. The first generated frame must be placed after the highest presentation time the
//     source reached, not merely after its last decode time. With B frames those differ by the
//     reorder delay, and a frame placed between them is displayed out of order.
//   - Resuming. Real video must resume at an IDR. The generated frames advance frame_num and the
//     picture order count past what the source encoder will emit next, and only an IDR resets both.
//
// Supported streams are progressive, with pic_order_cnt_type 0 or 2, in CAVLC or CABAC.
type PSkipExtender struct {
	sps *avc.SPS
	pps *avc.PPS

	spsMap map[uint32]*avc.SPS
	ppsMap map[uint32]*avc.PPS

	maxFrameNum uint32
	maxPOCLsb   uint32

	have      bool
	frameNum  uint32 // frame_num of the most recent reference picture
	pocLsb    uint32 // highest pic_order_cnt_lsb seen since the last IDR
	generated uint64
}

// NewPSkipExtender returns a PSkipExtender that generates slices from the given parameter sets.
// They must be the ones the stream's slices reference.
func NewPSkipExtender(sps *avc.SPS, pps *avc.PPS) (*PSkipExtender, error) {
	if sps == nil || pps == nil {
		return nil, fmt.Errorf("NewPSkipExtender: sps and pps are required")
	}
	if sps.PicOrderCntType != 0 && sps.PicOrderCntType != 2 {
		return nil, fmt.Errorf("NewPSkipExtender: pic_order_cnt_type=%d not supported (only types 0 and 2)",
			sps.PicOrderCntType)
	}
	if !sps.FrameMbsOnlyFlag {
		return nil, fmt.Errorf("NewPSkipExtender: interlaced not supported (frame_mbs_only_flag=0)")
	}
	e := &PSkipExtender{
		sps:         sps,
		pps:         pps,
		spsMap:      map[uint32]*avc.SPS{sps.ParameterID: sps},
		ppsMap:      map[uint32]*avc.PPS{pps.PicParameterSetID: pps},
		maxFrameNum: 1 << (sps.Log2MaxFrameNumMinus4 + 4),
		maxPOCLsb:   1 << (sps.Log2MaxPicOrderCntLsbMinus4 + 4),
	}
	return e, nil
}

// NewPSkipExtenderFromDecConfRec returns a PSkipExtender built from an
// AVCDecoderConfigurationRecord, the form fragmented MP4 and RTMP carry parameter sets in. The
// first SPS and PPS are used.
func NewPSkipExtenderFromDecConfRec(avcC []byte) (*PSkipExtender, error) {
	rec, err := avc.DecodeAVCDecConfRec(avcC)
	if err != nil {
		return nil, fmt.Errorf("NewPSkipExtenderFromDecConfRec: %w", err)
	}
	if len(rec.SPSnalus) == 0 || len(rec.PPSnalus) == 0 {
		return nil, fmt.Errorf("NewPSkipExtenderFromDecConfRec: record has no SPS or PPS")
	}
	sps, err := avc.ParseSPSNALUnit(rec.SPSnalus[0], true)
	if err != nil {
		return nil, fmt.Errorf("NewPSkipExtenderFromDecConfRec: parse SPS: %w", err)
	}
	spsMap := map[uint32]*avc.SPS{sps.ParameterID: sps}
	pps, err := avc.ParsePPSNALUnit(rec.PPSnalus[0], spsMap)
	if err != nil {
		return nil, fmt.Errorf("NewPSkipExtenderFromDecConfRec: parse PPS: %w", err)
	}
	e, err := NewPSkipExtender(sps, pps)
	if err != nil {
		return nil, err
	}
	// Any further parameter sets in the record are kept so that slices referencing them still parse.
	for _, nalu := range rec.SPSnalus[1:] {
		if s, perr := avc.ParseSPSNALUnit(nalu, true); perr == nil {
			e.spsMap[s.ParameterID] = s
		}
	}
	for _, nalu := range rec.PPSnalus[1:] {
		if p, perr := avc.ParsePPSNALUnit(nalu, e.spsMap); perr == nil {
			e.ppsMap[p.PicParameterSetID] = p
		}
	}
	return e, nil
}

// Ready reports whether a coded slice has been observed, which is required before frames can be
// generated.
func (e *PSkipExtender) Ready() bool { return e.have }

// State returns the frame_num of the most recent reference picture and the highest
// pic_order_cnt_lsb seen since the last IDR. The next generated frame continues from these.
func (e *PSkipExtender) State() (frameNum, picOrderCntLsb uint32) { return e.frameNum, e.pocLsb }

// Generated returns how many frames the PSkipExtender has produced.
func (e *PSkipExtender) Generated() uint64 { return e.generated }

// ObserveNALU tracks one NAL unit, given without a start code or length prefix. Parameter sets are
// remembered so that later slices parse; coded slices update the numbering. Anything else is
// ignored, so a whole access unit can be passed through NAL unit by NAL unit.
func (e *PSkipExtender) ObserveNALU(nalu []byte) error {
	if len(nalu) == 0 {
		return nil
	}
	switch avc.GetNaluType(nalu[0]) {
	case avc.NALU_SPS:
		sps, err := avc.ParseSPSNALUnit(nalu, true)
		if err != nil {
			return fmt.Errorf("PSkipExtender: parse SPS: %w", err)
		}
		e.spsMap[sps.ParameterID] = sps
	case avc.NALU_PPS:
		pps, err := avc.ParsePPSNALUnit(nalu, e.spsMap)
		if err != nil {
			return fmt.Errorf("PSkipExtender: parse PPS: %w", err)
		}
		e.ppsMap[pps.PicParameterSetID] = pps
	case avc.NALU_IDR, avc.NALU_NON_IDR:
		sh, err := avc.ParseSliceHeader(nalu, e.spsMap, e.ppsMap)
		if err != nil {
			return fmt.Errorf("PSkipExtender: parse slice header: %w", err)
		}
		e.observeSlice(sh, avc.GetNaluType(nalu[0]) == avc.NALU_IDR, nalu[0]&0x60 != 0)
	}
	return nil
}

// ObserveAVCCSample tracks one access unit in AVCC form, with 4-byte length prefixes, as carried by
// fragmented MP4 and by FLV or RTMP video messages.
func (e *PSkipExtender) ObserveAVCCSample(sample []byte) error {
	nalus, err := avc.GetNalusFromSample(sample)
	if err != nil {
		return fmt.Errorf("PSkipExtender: split access unit: %w", err)
	}
	for _, nalu := range nalus {
		if err := e.ObserveNALU(nalu); err != nil {
			return err
		}
	}
	return nil
}

// ObserveAnnexB tracks every NAL unit of an Annex-B byte stream.
func (e *PSkipExtender) ObserveAnnexB(stream []byte) error {
	for _, nalu := range avc.ExtractNalusFromByteStream(stream) {
		if err := e.ObserveNALU(nalu); err != nil {
			return err
		}
	}
	return nil
}

// observeSlice folds one coded slice into the tracked numbering.
//
// frame_num identifies the most recent reference picture, so a non-reference picture must not
// advance it. The picture order count continues from the highest value seen rather than from the
// most recent picture, because in a stream with B frames the last picture in decode order is not
// the last in display order. An IDR resets both, and discards anything seen before it.
func (e *PSkipExtender) observeSlice(sh *avc.SliceHeader, isIDR, isReference bool) {
	switch {
	case isIDR:
		e.frameNum = sh.FrameNum
		e.pocLsb = sh.PicOrderCntLsb
	case !e.have:
		e.frameNum = sh.FrameNum
		e.pocLsb = sh.PicOrderCntLsb
	default:
		if isReference {
			e.frameNum = sh.FrameNum
		}
		if pocIsLater(sh.PicOrderCntLsb, e.pocLsb, e.maxPOCLsb) {
			e.pocLsb = sh.PicOrderCntLsb
		}
	}
	e.have = true
}

// NextSlice generates one P_Skip frame continuing the stream. It is a reference picture, so the
// numbering advances and successive calls continue where the last one left off.
func (e *PSkipExtender) NextSlice() ([]byte, error) {
	if !e.have {
		return nil, fmt.Errorf("PSkipExtender: no coded slice observed yet")
	}
	e.frameNum = (e.frameNum + 1) % e.maxFrameNum
	e.pocLsb = (e.pocLsb + 2) % e.maxPOCLsb
	slice, err := EncodePSkipSlice(e.sps, e.pps, e.frameNum, e.pocLsb, 0)
	if err != nil {
		return nil, fmt.Errorf("PSkipExtender: %w", err)
	}
	e.generated++
	return slice, nil
}

// NextSlices generates count P_Skip frames in order.
func (e *PSkipExtender) NextSlices(count int) ([][]byte, error) {
	if count <= 0 {
		return nil, nil
	}
	out := make([][]byte, 0, count)
	for range count {
		slice, err := e.NextSlice()
		if err != nil {
			return nil, err
		}
		out = append(out, slice)
	}
	return out, nil
}

// pocIsLater reports whether a comes after b in a modulo-max picture order count sequence.
func pocIsLater(a, b, max uint32) bool {
	if max == 0 {
		return false
	}
	half := int64(max / 2)
	d := int64(a) - int64(b)
	switch {
	case d > half:
		d -= int64(max)
	case d < -half:
		d += int64(max)
	}
	return d > 0
}
