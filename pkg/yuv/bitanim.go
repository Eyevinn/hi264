package yuv

import (
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// BitAnim holds a sequence of 1-bit animation frames at macroblock resolution.
// Each frame is a grid of on/off blocks, stored as packed bits (MSB first).
// Use LoadBitAnim to load from a gzip-compressed file produced by
// tools/make_bitanim.sh.
type BitAnim struct {
	Width     int // grid columns (source resolution in macroblocks)
	Height    int // grid rows
	NumFrames int
	White     Color // color for "on" pixels
	Black     Color // color for "off" pixels
	bpf       int   // bytes per frame = ceil(Width*Height/8)
	data      []byte
}

// LoadBitAnim loads a 1-bit animation from a gzip-compressed file.
// Format: uint16le width, uint16le height, then ceil(W*H/8) bytes per frame.
// White and black colors are computed from the given color space and range.
func LoadBitAnim(path string, cs ColorSpace, rng Range) (*BitAnim, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("bitanim: not a valid gzip file: %w", err)
	}
	defer gz.Close()

	raw, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("bitanim: read error: %w", err)
	}

	if len(raw) < 4 {
		return nil, fmt.Errorf("bitanim: file too short (%d bytes)", len(raw))
	}

	w := int(binary.LittleEndian.Uint16(raw[0:2]))
	h := int(binary.LittleEndian.Uint16(raw[2:4]))
	if w == 0 || h == 0 {
		return nil, fmt.Errorf("bitanim: invalid dimensions %dx%d", w, h)
	}

	bpf := (w*h + 7) / 8
	data := raw[4:]
	nf := len(data) / bpf
	if nf == 0 {
		return nil, fmt.Errorf("bitanim: no frames (need at least %d bytes per frame, got %d)", bpf, len(data))
	}
	data = data[:nf*bpf]

	return &BitAnim{
		Width:     w,
		Height:    h,
		NumFrames: nf,
		White:     RGBToYCbCrCS(255, 255, 255, cs, rng),
		Black:     RGBToYCbCrCS(0, 0, 0, cs, rng),
		bpf:       bpf,
		data:      data,
	}, nil
}

// Frame returns a Grid and ColorMap for animation frame i, scaled with
// nearest-neighbor sampling to the given macroblock dimensions.
// Frame indices wrap with modulo.
func (a *BitAnim) Frame(i, mbW, mbH int) (*Grid, ColorMap) {
	i = i % a.NumFrames
	off := i * a.bpf
	frame := a.data[off : off+a.bpf]

	chars := make([][]byte, mbH)
	for y := range mbH {
		chars[y] = make([]byte, mbW)
		for x := range mbW {
			srcX := x * a.Width / mbW
			srcY := y * a.Height / mbH
			idx := srcY*a.Width + srcX
			if frame[idx>>3]&(1<<(7-(idx&7))) != 0 {
				chars[y][x] = 'w'
			} else {
				chars[y][x] = 'k'
			}
		}
	}

	return &Grid{Chars: chars, Width: mbW, Height: mbH},
		ColorMap{'w': a.White, 'k': a.Black}
}
