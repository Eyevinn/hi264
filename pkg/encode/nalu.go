package encode

import (
	"fmt"
	"io"
)

// InsertEBSPPrevention inserts emulation prevention bytes (0x03)
// after each occurrence of 0x00 0x00 in the RBSP data.
// This is the inverse of removeEBSPPrevention in the decoder.
func InsertEBSPPrevention(rbsp []byte) []byte {
	result := make([]byte, 0, len(rbsp)+len(rbsp)/256)
	zeroCount := 0
	for _, b := range rbsp {
		if zeroCount >= 2 && b <= 3 {
			result = append(result, 0x03)
			zeroCount = 0
		}
		result = append(result, b)
		if b == 0 {
			zeroCount++
		} else {
			zeroCount = 0
		}
	}
	return result
}

// BuildNALU returns a complete NALU byte slice (header + EBSP) without Annex-B start codes.
// Useful for MP4 sample descriptions (e.g. SetAVCDescriptor) which want raw NALUs.
func BuildNALU(naluType, refIDC byte, rbsp []byte) []byte {
	nalHeader := (refIDC << 5) | (naluType & 0x1f)
	ebsp := InsertEBSPPrevention(rbsp)
	nalu := make([]byte, 1+len(ebsp))
	nalu[0] = nalHeader
	copy(nalu[1:], ebsp)
	return nalu
}

// FillerNALU returns an Annex-B filler NALU (nal_unit_type 12) of exactly size bytes.
// The minimum size is 6 bytes (4-byte start code + 1-byte header + 1-byte RBSP stop bit).
// Returns an error if size is less than 6.
// Fill bytes are 0xFF which never trigger EBSP prevention.
func FillerNALU(size int) ([]byte, error) {
	if size < 6 {
		return nil, fmt.Errorf("filler NALU minimum size is 6 bytes, got %d", size)
	}
	nalu := make([]byte, size)
	// Start code
	nalu[0] = 0x00
	nalu[1] = 0x00
	nalu[2] = 0x00
	nalu[3] = 0x01
	// NAL header: nal_ref_idc=0, nal_unit_type=12
	nalu[4] = 0x0C
	// Fill bytes
	for i := 5; i < size-1; i++ {
		nalu[i] = 0xFF
	}
	// RBSP stop bit
	nalu[size-1] = 0x80
	return nalu, nil
}

// PadSlice appends a filler NALU to slice so the total is exactly targetBytes.
// Returns an error if the slice already exceeds targetBytes, or if the gap is
// too small for a filler NALU (1-5 bytes).
func PadSlice(slice []byte, targetBytes int) ([]byte, error) {
	if len(slice) > targetBytes {
		return nil, fmt.Errorf("slice size %d exceeds target %d bytes", len(slice), targetBytes)
	}
	if len(slice) == targetBytes {
		return slice, nil
	}
	gap := targetBytes - len(slice)
	filler, err := FillerNALU(gap)
	if err != nil {
		return nil, fmt.Errorf("cannot pad %d-byte slice to target %d: "+
			"gap %d too small for filler NALU (minimum 6)", len(slice), targetBytes, gap)
	}
	return append(slice, filler...), nil
}

// WriteNALU writes an Annex-B framed NALU to w.
// naluType: NALU type (e.g., 5 for IDR, 7 for SPS, 8 for PPS).
// refIDC: nal_ref_idc (0-3).
// rbsp: raw byte sequence payload (before EBSP prevention insertion).
func WriteNALU(w io.Writer, naluType, refIDC byte, rbsp []byte) error {
	// Start code prefix: 0x00 0x00 0x00 0x01
	_, err := w.Write([]byte{0x00, 0x00, 0x00, 0x01})
	if err != nil {
		return err
	}

	// NAL header byte: forbidden_zero_bit(1) | nal_ref_idc(2) | nal_unit_type(5)
	nalHeader := (refIDC << 5) | (naluType & 0x1f)
	_, err = w.Write([]byte{nalHeader})
	if err != nil {
		return err
	}

	// EBSP data
	ebsp := InsertEBSPPrevention(rbsp)
	_, err = w.Write(ebsp)
	return err
}
