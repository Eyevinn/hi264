package encode

import (
	"bytes"
	"fmt"

	"github.com/Eyevinn/mp4ff/avc"

	"github.com/Eyevinn/hi264/internal/cabac"
	"github.com/Eyevinn/hi264/internal/context"
)

// EncodePSkipSlice encodes a P_Skip slice (all MBs skip, copying from reference)
// using parameters from parsed SPS and PPS structures.
// Supports both CAVLC and CABAC entropy coding based on PPS.EntropyCodingModeFlag.
// frameNum is the frame_num value for this slice.
// disableDeblock: 0=enable (with offsets=0), 1=disable, 2=disable across slices.
// Returns an Annex-B framed non-IDR NALU (type=1, ref_idc=2).
func EncodePSkipSlice(sps *avc.SPS, pps *avc.PPS, frameNum uint32, disableDeblock int) ([]byte, error) {
	// Validate: only POC type 0
	if sps.PicOrderCntType != 0 {
		return nil, fmt.Errorf("EncodePSkipSlice: pic_order_cnt_type=%d "+
			"not supported (only type 0)", sps.PicOrderCntType)
	}
	// Validate: progressive only
	if !sps.FrameMbsOnlyFlag {
		return nil, fmt.Errorf("EncodePSkipSlice: interlaced not supported (frame_mbs_only_flag=0)")
	}

	mbWidth := (int(sps.Width) + 15) / 16
	mbHeight := (int(sps.Height) + 15) / 16
	totalMBs := mbWidth * mbHeight

	if pps.EntropyCodingModeFlag {
		return encodePSkipSliceCABAC(sps, pps, frameNum, disableDeblock, totalMBs)
	}
	return encodePSkipSliceCAVLC(sps, pps, frameNum, disableDeblock, totalMBs)
}

func encodePSkipSliceCAVLC(sps *avc.SPS, pps *avc.PPS,
	frameNum uint32, disableDeblock int, totalMBs int) ([]byte, error) {
	w := NewBitWriter()
	WritePSliceHeader(w, frameNum, 0, disableDeblock,
		uint32(sps.Log2MaxFrameNumMinus4),
		uint32(sps.Log2MaxPicOrderCntLsbMinus4),
		pps.PicParameterSetID,
		pps.DeblockingFilterControlPresentFlag, -1)

	// mb_skip_run = totalMBs (all MBs are P_Skip)
	w.WriteUE(uint32(totalMBs))

	// RBSP trailing bits
	w.WriteBit(1)
	w.AlignToByte()

	var buf bytes.Buffer
	sliceRBSP := w.Bytes()
	// NALU type=1 (non-IDR coded slice), nal_ref_idc=2
	err := WriteNALU(&buf, 1, 2, sliceRBSP)
	if err != nil {
		return nil, fmt.Errorf("write P_Skip CAVLC: %w", err)
	}

	return buf.Bytes(), nil
}

func encodePSkipSliceCABAC(sps *avc.SPS, pps *avc.PPS,
	frameNum uint32, disableDeblock int, totalMBs int) ([]byte, error) {
	const cabacInitIDC = 0

	// Build slice header with CABAC alignment
	headerW := NewBitWriter()
	WritePSliceHeaderCABAC(headerW, frameNum, 0, disableDeblock,
		uint32(sps.Log2MaxFrameNumMinus4),
		uint32(sps.Log2MaxPicOrderCntLsbMinus4),
		pps.PicParameterSetID,
		pps.DeblockingFilterControlPresentFlag,
		cabacInitIDC)
	headerBytes := headerW.Bytes()

	// Initialize CABAC encoder and P-slice context models
	enc := cabac.NewEncoder()
	sliceQPY := 26 + int(pps.PicInitQpMinus26) // qpDelta=0
	models := context.InitModels(sliceQPY, 0, cabacInitIDC)
	ctx := models[:]

	// Encode each MB as skip
	for mbIdx := range totalMBs {
		// mb_skip_flag = 1
		// Context index for mb_skip_flag in P-slices: ctxIdxOffset=11
		// ctxIdxInc = condTermFlagA + condTermFlagB * 2
		// condTermFlagN = (mbN available && mbN is NOT skip) ? 1 : 0
		// Since all MBs ARE skip: condTermFlagN=0 for available neighbors
		// Unavailable neighbors also give 0. So ctxIdxInc=0 always → ctx index 11.
		enc.EncodeDecision(1, &ctx[11])

		// end_of_slice_flag
		if mbIdx == totalMBs-1 {
			enc.EncodeTerminate(1)
		} else {
			enc.EncodeTerminate(0)
		}
	}

	cabacBytes := enc.Flush()

	// Concatenate header + CABAC data as RBSP
	sliceRBSP := make([]byte, 0, len(headerBytes)+len(cabacBytes))
	sliceRBSP = append(sliceRBSP, headerBytes...)
	sliceRBSP = append(sliceRBSP, cabacBytes...)

	var buf bytes.Buffer
	err := WriteNALU(&buf, 1, 2, sliceRBSP)
	if err != nil {
		return nil, fmt.Errorf("write P_Skip CABAC: %w", err)
	}

	return buf.Bytes(), nil
}
