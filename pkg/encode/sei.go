package encode

import (
	"bytes"
	"fmt"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/sei"
)

// PicTiming holds the per-picture data for a Picture Timing SEI (payload
// type 1) carrying a single progressive-frame clock timestamp (HH:MM:SS:FF).
//
// The clock-timestamp fields are always written. CpbRemovalDelay and
// DpbOutputDelay are written only when the accompanying PicTimingConfig.HRD is
// non-nil (i.e. the active SPS signals HRD parameters); otherwise they are
// ignored.
type PicTiming struct {
	Hours   uint8
	Minutes uint8
	Seconds uint8
	Frames  uint8 // frame index within the current second (n_frames)

	// HRD-only fields, used iff PicTimingConfig.HRD != nil.
	CpbRemovalDelay uint
	DpbOutputDelay  uint
}

// HRDDelayLengths carries the bit-lengths of the CPB/DPB delay fields. These
// are fixed by the active SPS's HRD parameters and must match exactly, or a
// conforming decoder cannot parse the message. Obtain them from the source SPS
// via PicTimingConfigFromSPS.
type HRDDelayLengths struct {
	CpbRemovalDelayLenMinus1 uint8
	DpbOutputDelayLenMinus1  uint8
	TimeOffsetLength         uint8
}

// PicTimingConfig describes the SPS-derived syntax context that governs a
// pic_timing message. A pic_timing SEI is only conformant when the active SPS
// signals pic_struct_present_flag and/or HRD parameters, so build the config to
// match that SPS:
//
//   - Self-generated streams: PicTimingConfig{PicStructPresent: true} (no HRD).
//   - Extending foreign content: PicTimingConfigFromSPS(sps).
type PicTimingConfig struct {
	// PicStructPresent must mirror the SPS VUI pic_struct_present_flag. It must
	// be true for the clock-timestamp syntax these helpers emit.
	PicStructPresent bool
	// HRD is non-nil when the SPS signals HRD parameters
	// (CpbDpbDelaysPresentFlag). When set, cpb_removal_delay/dpb_output_delay
	// are written ahead of pic_struct at the given bit-lengths.
	HRD *HRDDelayLengths
}

// PicTimingConfigFromSPS derives the pic_timing syntax context from a parsed
// (possibly foreign) SPS. Use it when extending an existing stream so emitted
// SEIs match what the stream's decoders expect.
func PicTimingConfigFromSPS(sps *avc.SPS) PicTimingConfig {
	cfg := PicTimingConfig{PicStructPresent: sps.PicStructPresent()}
	if sps.CpbDpbDelaysPresent() && sps.VUI != nil {
		hp := sps.VUI.NalHrdParameters
		if hp == nil {
			hp = sps.VUI.VclHrdParameters
		}
		if hp != nil {
			cfg.HRD = &HRDDelayLengths{
				CpbRemovalDelayLenMinus1: uint8(hp.CpbRemovalDelayLengthMinus1),
				DpbOutputDelayLenMinus1:  uint8(hp.DpbOutputDelayLengthMinus1),
				TimeOffsetLength:         uint8(hp.TimeOffsetLength),
			}
		}
	}
	return cfg
}

// GeneratePicTimingSEI returns an Annex-B SEI NALU (nal_unit_type 6,
// nal_ref_idc 0) carrying a pic_timing message for one progressive frame.
// Prepend it to an IDR or P_Skip slice to attach the timing to that picture;
// it composes identically with either slice type.
//
// cfg.PicStructPresent must be true (the active SPS must set
// pic_struct_present_flag), otherwise the message would be non-conformant and
// an error is returned.
func GeneratePicTimingSEI(cfg PicTimingConfig, pt PicTiming) ([]byte, error) {
	if !cfg.PicStructPresent {
		return nil, fmt.Errorf("GeneratePicTimingSEI: cfg.PicStructPresent must be true " +
			"(SPS pic_struct_present_flag) to emit clock timestamps")
	}
	var buf bytes.Buffer
	if err := WriteNALU(&buf, 6, 0, picTimingRBSP(cfg, pt)); err != nil {
		return nil, fmt.Errorf("write SEI NALU: %w", err)
	}
	return buf.Bytes(), nil
}

// BuildPicTimingSEINALU is the raw-NALU form of GeneratePicTimingSEI (header +
// EBSP, no Annex-B start code), suitable for MP4 sample assembly — mirroring
// BuildNALU's role for SPS/PPS.
func BuildPicTimingSEINALU(cfg PicTimingConfig, pt PicTiming) ([]byte, error) {
	if !cfg.PicStructPresent {
		return nil, fmt.Errorf("BuildPicTimingSEINALU: cfg.PicStructPresent must be true " +
			"(SPS pic_struct_present_flag) to emit clock timestamps")
	}
	return BuildNALU(6, 0, picTimingRBSP(cfg, pt)), nil
}

// picTimingMessage builds the mp4ff pic_timing SEI message for the given config
// and per-picture data. pict_struct is 0 (progressive frame), which means
// exactly one clock timestamp.
func picTimingMessage(cfg PicTimingConfig, pt PicTiming) *sei.PicTimingAvcSEI {
	clock := sei.ClockTSAvc{
		ClockTimeStampFlag: true,
		CtType:             0, // 0 = progressive
		FullTimeStampFlag:  true,
		Hours:              pt.Hours,
		Minutes:            pt.Minutes,
		Seconds:            pt.Seconds,
		NFrames:            pt.Frames,
	}
	msg := &sei.PicTimingAvcSEI{
		PictStruct: 0, // 0 = progressive frame -> one clock timestamp
		Clocks:     []sei.ClockTSAvc{clock},
	}
	if cfg.HRD != nil {
		msg.CbpDbpDelay = &sei.CbpDbpDelay{
			CpbRemovalDelay:             pt.CpbRemovalDelay,
			DpbOutputDelay:              pt.DpbOutputDelay,
			CpbRemovalDelayLengthMinus1: cfg.HRD.CpbRemovalDelayLenMinus1,
			DpbOutputDelayLengthMinus1:  cfg.HRD.DpbOutputDelayLenMinus1,
		}
		msg.TimeOffsetLength = cfg.HRD.TimeOffsetLength
		msg.Clocks[0].TimeOffsetLength = cfg.HRD.TimeOffsetLength
	}
	return msg
}

// picTimingRBSP builds the SEI NAL RBSP payload: payloadType, payloadSize, the
// pic_timing payload bytes, and rbsp_trailing_bits. EBSP escaping is left to
// WriteNALU/BuildNALU so it stays consistent with the rest of the encoder.
func picTimingRBSP(cfg PicTimingConfig, pt PicTiming) []byte {
	msg := picTimingMessage(cfg, pt)
	payload := msg.Payload()

	w := NewBitWriter()
	writeSEIValue(w, int(msg.Type())) // payloadType (= 1)
	writeSEIValue(w, len(payload))    // payloadSize
	for _, b := range payload {
		w.WriteBits(uint32(b), 8)
	}
	w.WriteBit(1) // rbsp_trailing_bits: stop one bit
	w.AlignToByte()
	return w.Bytes()
}

// writeSEIValue writes an SEI payloadType/payloadSize value using the 0xFF
// continuation encoding from ISO/IEC 14496-10 Section 7.3.2.3.1.
func writeSEIValue(w *BitWriter, v int) {
	for v >= 0xFF {
		w.WriteBits(0xFF, 8)
		v -= 0xFF
	}
	w.WriteBits(uint32(v), 8)
}
