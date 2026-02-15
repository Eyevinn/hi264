package encode

import (
	"bytes"
	"fmt"

	"github.com/Eyevinn/mp4ff/avc"

	"github.com/Eyevinn/hi264/pkg/yuv"
)

// EncodeParams holds H.264 encoding parameters.
type EncodeParams struct {
	Width          int            // frame width in pixels (must be even)
	Height         int            // frame height in pixels (must be even)
	QP             int            // quantization parameter (0-51, default 26)
	CABAC          bool           // true=Main profile CABAC, false=Baseline CAVLC
	DisableDeblock int            // 0=enable, 1=disable
	MaxRefFrames   int            // max_num_ref_frames (0=IDR-only, 1+=P-frames)
	ColorSpace     yuv.ColorSpace // YCbCr matrix standard (default BT601)
	Range          yuv.Range      // sample value range (default LimitedRange)
}

func (p *EncodeParams) validate() error {
	if p.Width <= 0 || p.Width%2 != 0 {
		return fmt.Errorf("width must be positive and even, got %d", p.Width)
	}
	if p.Height <= 0 || p.Height%2 != 0 {
		return fmt.Errorf("height must be positive and even, got %d", p.Height)
	}
	if p.QP < 0 || p.QP > 51 {
		return fmt.Errorf("QP must be 0-51, got %d", p.QP)
	}
	return nil
}

func (p *EncodeParams) qp() int {
	if p.QP == 0 {
		return 26
	}
	return p.QP
}

// GenerateSPS returns SPS NALU bytes in Annex-B format.
func GenerateSPS(p EncodeParams) ([]byte, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	var rbsp []byte
	if p.CABAC {
		rbsp = EncodeSPSMain(p.Width, p.Height, p.MaxRefFrames, p.ColorSpace, p.Range)
	} else {
		rbsp = EncodeSPS(p.Width, p.Height, p.MaxRefFrames, p.ColorSpace, p.Range)
	}
	var buf bytes.Buffer
	if err := WriteNALU(&buf, 7, 3, rbsp); err != nil {
		return nil, fmt.Errorf("write SPS: %w", err)
	}
	return buf.Bytes(), nil
}

// GeneratePPS returns PPS NALU bytes in Annex-B format.
func GeneratePPS(p EncodeParams) ([]byte, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	var rbsp []byte
	if p.CABAC {
		rbsp = EncodePPSCABAC(p.DisableDeblock)
	} else {
		rbsp = EncodePPS(p.DisableDeblock)
	}
	var buf bytes.Buffer
	if err := WriteNALU(&buf, 8, 3, rbsp); err != nil {
		return nil, fmt.Errorf("write PPS: %w", err)
	}
	return buf.Bytes(), nil
}

// GenerateIDR encodes a flat-color IDR frame from a Grid/ColorMap.
// Returns the IDR slice NALU in Annex-B format (no SPS/PPS).
func GenerateIDR(p EncodeParams, grid *yuv.Grid, colors yuv.ColorMap, idrPicID uint32) ([]byte, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	enc := &FrameEncoder{
		Grid:            grid,
		Colors:          colors,
		QP:              p.qp(),
		DisableDeblock:  p.DisableDeblock,
		CABAC:           p.CABAC,
		MaxNumRefFrames: p.MaxRefFrames,
		Width:           p.Width,
		Height:          p.Height,
		ColorSpace:      p.ColorSpace,
		Range:           p.Range,
	}
	return enc.EncodeSlice(idrPicID)
}

// GeneratePSkip returns a P_Skip slice NALU in Annex-B format.
func GeneratePSkip(p EncodeParams, frameNum uint32) ([]byte, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	sps := &avc.SPS{
		Width:            uint(p.Width),
		Height:           uint(p.Height),
		PicOrderCntType:  0,
		FrameMbsOnlyFlag: true,
	}
	pps := &avc.PPS{
		DeblockingFilterControlPresentFlag: true,
		EntropyCodingModeFlag:              p.CABAC,
		PicInitQpMinus26:                   p.qp() - 26,
	}
	return EncodePSkipSlice(sps, pps, frameNum, p.DisableDeblock)
}
