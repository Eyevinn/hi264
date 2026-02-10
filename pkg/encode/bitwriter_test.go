package encode

import (
	"testing"

	"github.com/Eyevinn/hi264/internal/cavlc"
)

func TestBitWriterRoundTrip(t *testing.T) {
	// Write some bits, read them back
	w := NewBitWriter()
	w.WriteBits(0b10110, 5)
	w.WriteBits(0xFF, 8)
	w.WriteBit(0)
	w.WriteBit(1)
	w.AlignToByte()

	data := w.Bytes()
	r := cavlc.NewBitReader(data)

	v, err := r.ReadBits(5)
	if err != nil {
		t.Fatal(err)
	}
	if v != 0b10110 {
		t.Errorf("got %05b, want 10110", v)
	}

	v, err = r.ReadBits(8)
	if err != nil {
		t.Fatal(err)
	}
	if v != 0xFF {
		t.Errorf("got %08b, want 11111111", v)
	}

	b, err := r.ReadBit()
	if err != nil {
		t.Fatal(err)
	}
	if b != 0 {
		t.Errorf("got bit %d, want 0", b)
	}
	b, err = r.ReadBit()
	if err != nil {
		t.Fatal(err)
	}
	if b != 1 {
		t.Errorf("got bit %d, want 1", b)
	}
}

func TestUERoundTrip(t *testing.T) {
	testVals := []uint32{0, 1, 2, 3, 4, 5, 10, 100, 255, 1000}
	for _, val := range testVals {
		w := NewBitWriter()
		w.WriteUE(val)
		w.AlignToByte()

		r := cavlc.NewBitReader(w.Bytes())
		got, err := r.ReadUE()
		if err != nil {
			t.Fatalf("ReadUE for %d: %v", val, err)
		}
		if got != val {
			t.Errorf("UE round-trip: wrote %d, read %d", val, got)
		}
	}
}

func TestSERoundTrip(t *testing.T) {
	testVals := []int32{0, 1, -1, 2, -2, 10, -10, 100, -100}
	for _, val := range testVals {
		w := NewBitWriter()
		w.WriteSE(val)
		w.AlignToByte()

		r := cavlc.NewBitReader(w.Bytes())
		got, err := r.ReadSE()
		if err != nil {
			t.Fatalf("ReadSE for %d: %v", val, err)
		}
		if got != val {
			t.Errorf("SE round-trip: wrote %d, read %d", val, got)
		}
	}
}

func TestBitsWritten(t *testing.T) {
	w := NewBitWriter()
	if w.BitsWritten() != 0 {
		t.Errorf("initial bits = %d, want 0", w.BitsWritten())
	}
	w.WriteBit(1)
	if w.BitsWritten() != 1 {
		t.Errorf("after 1 bit = %d, want 1", w.BitsWritten())
	}
	w.WriteBits(0, 7)
	if w.BitsWritten() != 8 {
		t.Errorf("after 8 bits = %d, want 8", w.BitsWritten())
	}
}
