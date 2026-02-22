package cabac

import "fmt"

// CtxState represents a CABAC context model with probability state and MPS value.
type CtxState struct {
	PStateIdx uint8 // probability state index (0-63)
	ValMPS    uint8 // value of the most probable symbol (0 or 1)
}

// Decoder is the CABAC arithmetic decoding engine as specified in section 9.3.3.2.
type Decoder struct {
	codIRange  uint16
	codIOffset uint16
	data       []byte
	pos        int // current byte position in data
	bitsLeft   int // bits left in current byte
}

// NewDecoder creates a new CABAC decoder initialized with the given byte stream.
// Per section 9.3.3.2.1: codIRange = 510, codIOffset = read_bits(9).
func NewDecoder(data []byte) (*Decoder, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("cabac: need at least 2 bytes, got %d", len(data))
	}
	// Read 9 bits for codIOffset: 8 bits from byte 0, 1 bit from byte 1
	codIOffset := (uint16(data[0]) << 1) | uint16(data[1]>>7)

	d := &Decoder{
		codIRange:  510,
		codIOffset: codIOffset,
		data:       data,
		pos:        2,
		bitsLeft:   7,
	}
	return d, nil
}

// readBit reads a single bit from the bitstream.
func (d *Decoder) readBit() uint16 {
	if d.bitsLeft == 0 {
		if d.pos < len(d.data) {
			d.bitsLeft = 8
			d.pos++
		} else {
			return 0
		}
	}
	d.bitsLeft--
	bit := uint16((d.data[d.pos-1] >> uint(d.bitsLeft)) & 1)
	return bit
}

// renormalize performs the renormalization loop (section 9.3.3.2.2).
func (d *Decoder) renormalize() {
	for d.codIRange < 256 {
		d.codIRange <<= 1
		d.codIOffset <<= 1
		d.codIOffset |= d.readBit()
	}
}

// DecodeDecision decodes a single binary decision using the given context model
// (section 9.3.3.2.1).
func (d *Decoder) DecodeDecision(ctx *CtxState) uint8 {
	qCodIRangeIdx := (d.codIRange >> 6) & 3
	codIRangeLPS := rangeTabLPS[ctx.PStateIdx][qCodIRangeIdx]
	d.codIRange -= codIRangeLPS

	var binVal uint8
	if d.codIOffset >= d.codIRange {
		// LPS path
		binVal = 1 - ctx.ValMPS
		d.codIOffset -= d.codIRange
		d.codIRange = codIRangeLPS

		if ctx.PStateIdx == 0 {
			ctx.ValMPS = 1 - ctx.ValMPS
		}
		ctx.PStateIdx = transIdxLPS[ctx.PStateIdx]
	} else {
		// MPS path
		binVal = ctx.ValMPS
		ctx.PStateIdx = transIdxMPS[ctx.PStateIdx]
	}

	d.renormalize()
	return binVal
}

// DecodeBypass decodes a single binary decision using the bypass (equiprobable)
// mode (section 9.3.3.2.3).
func (d *Decoder) DecodeBypass() uint8 {
	d.codIOffset <<= 1
	d.codIOffset |= d.readBit()

	var val uint8
	if d.codIOffset >= d.codIRange {
		d.codIOffset -= d.codIRange
		val = 1
	}
	return val
}

// DecodeTerminate decodes the end_of_slice_flag or I_PCM indicator
// (section 9.3.3.2.4).
func (d *Decoder) DecodeTerminate() uint8 {
	d.codIRange -= 2
	if d.codIOffset >= d.codIRange {
		return 1
	}
	d.renormalize()
	return 0
}

// BitsRead returns the number of bits consumed from the bitstream so far.
func (d *Decoder) BitsRead() int {
	return d.pos*8 - d.bitsLeft
}

// State returns the current arithmetic state (range and offset).
func (d *Decoder) State() (codIRange, codIOffset uint16) {
	return d.codIRange, d.codIOffset
}

// Range returns the current codIRange.
func (d *Decoder) Range() uint16 { return d.codIRange }

// Offset returns the current codIOffset.
func (d *Decoder) Offset() uint16 { return d.codIOffset }

// BytePos returns the current byte position in the bitstream.
func (d *Decoder) BytePos() int {
	return d.pos
}

// AlignToByte aligns the decoder to the next byte boundary (for I_PCM).
func (d *Decoder) AlignToByte() {
	d.bitsLeft = 0
}

// ReadBypassU reads n bypass bins and returns them as an unsigned integer (MSB first).
func (d *Decoder) ReadBypassU(n int) uint32 {
	var val uint32
	for range n {
		val = (val << 1) | uint32(d.DecodeBypass())
	}
	return val
}
