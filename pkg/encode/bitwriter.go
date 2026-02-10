// Package encode implements H.264/AVC encoding for IDR frames.
package encode

// BitWriter writes individual bits to a byte buffer, MSB first.
// It is the inverse of cavlc.BitReader.
type BitWriter struct {
	buf    []byte
	curBit uint // 0-7, counts from MSB (0 = MSB)
}

// NewBitWriter creates a new BitWriter with pre-allocated buffer capacity.
func NewBitWriter() *BitWriter {
	buf := make([]byte, 1, 64)
	return &BitWriter{buf: buf}
}

// WriteBit writes a single bit (0 or 1).
func (w *BitWriter) WriteBit(b uint8) {
	w.buf[len(w.buf)-1] |= (b & 1) << (7 - w.curBit)
	w.curBit++
	if w.curBit == 8 {
		w.curBit = 0
		w.buf = append(w.buf, 0)
	}
}

// WriteBits writes the lower n bits of val, MSB first.
func (w *BitWriter) WriteBits(val uint32, n int) {
	for i := n - 1; i >= 0; i-- {
		w.WriteBit(uint8((val >> uint(i)) & 1))
	}
}

// WriteUE writes an unsigned Exp-Golomb coded value.
func (w *BitWriter) WriteUE(val uint32) {
	// ue(v): val -> code_num = val
	// code_num = 2^leadingZeros - 1 + suffix
	// Find leadingZeros such that 2^(leadingZeros+1) - 1 > val
	if val == 0 {
		w.WriteBit(1)
		return
	}
	v := val + 1
	leadingZeros := 0
	tmp := v
	for tmp > 1 {
		tmp >>= 1
		leadingZeros++
	}
	// Write leadingZeros zeros
	for i := 0; i < leadingZeros; i++ {
		w.WriteBit(0)
	}
	// Write 1 followed by suffix bits
	w.WriteBits(v, leadingZeros+1)
}

// WriteSE writes a signed Exp-Golomb coded value.
func (w *BitWriter) WriteSE(val int32) {
	var ue uint32
	if val > 0 {
		ue = uint32(2*val - 1)
	} else {
		ue = uint32(-2 * val)
	}
	w.WriteUE(ue)
}

// AlignToByte pads with zero bits to reach the next byte boundary.
func (w *BitWriter) AlignToByte() {
	if w.curBit > 0 {
		w.curBit = 0
		w.buf = append(w.buf, 0)
	}
}

// Bytes returns the written data. If the last byte is partially written,
// it is included (remaining bits are zero-padded).
func (w *BitWriter) Bytes() []byte {
	if w.curBit == 0 {
		// Remove the trailing empty byte
		return w.buf[:len(w.buf)-1]
	}
	return w.buf
}

// BitsWritten returns the total number of bits written.
func (w *BitWriter) BitsWritten() int {
	if w.curBit == 0 {
		return (len(w.buf) - 1) * 8
	}
	return (len(w.buf)-1)*8 + int(w.curBit)
}
