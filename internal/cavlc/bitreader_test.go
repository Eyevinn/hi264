package cavlc

import "testing"

func TestReadBits(t *testing.T) {
	// 0xA5 = 10100101, 0x3C = 00111100
	br := NewBitReader([]byte{0xA5, 0x3C})

	// Read 4 bits: 1010 = 10
	val, err := br.ReadBits(4)
	if err != nil {
		t.Fatal(err)
	}
	if val != 10 {
		t.Errorf("got %d, want 10", val)
	}

	// Read 8 bits: 01010011 = 83
	val, err = br.ReadBits(8)
	if err != nil {
		t.Fatal(err)
	}
	if val != 0x53 {
		t.Errorf("got 0x%02X, want 0x53", val)
	}

	if br.BitsRead() != 12 {
		t.Errorf("BitsRead() = %d, want 12", br.BitsRead())
	}
}

func TestPeekBits(t *testing.T) {
	br := NewBitReader([]byte{0xA5})

	val, err := br.PeekBits(4)
	if err != nil {
		t.Fatal(err)
	}
	if val != 10 {
		t.Errorf("got %d, want 10", val)
	}

	// Position should not have advanced
	if br.BitsRead() != 0 {
		t.Errorf("BitsRead() = %d, want 0", br.BitsRead())
	}

	// Reading same bits should give same result
	val2, err := br.ReadBits(4)
	if err != nil {
		t.Fatal(err)
	}
	if val2 != val {
		t.Errorf("ReadBits after Peek: got %d, want %d", val2, val)
	}
}

func TestReadUE(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want uint32
	}{
		{"0", []byte{0x80}, 0},   // 1 -> 0
		{"1", []byte{0x40}, 1},   // 010 -> 1
		{"2", []byte{0x60}, 2},   // 011 -> 2
		{"3", []byte{0x20}, 3},   // 00100 -> 3
		{"4", []byte{0x28}, 4},   // 00101 -> 4
		{"5", []byte{0x30}, 5},   // 00110 -> 5
		{"6", []byte{0x38}, 6},   // 00111 -> 6
		{"7", []byte{0x10}, 7},   // 0001000 -> 7
		{"8", []byte{0x12}, 8},   // 0001001 -> 8 (actually 0x12 = 00010010)
		{"14", []byte{0x1E}, 14}, // 0001111 -> 14 (0x1E = 00011110)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := NewBitReader(tt.data)
			got, err := br.ReadUE()
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("ReadUE() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestReadSE(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want int32
	}{
		{"0", []byte{0x80}, 0},   // ue=0 -> se=0
		{"+1", []byte{0x40}, 1},  // ue=1 -> se=+1
		{"-1", []byte{0x60}, -1}, // ue=2 -> se=-1
		{"+2", []byte{0x20}, 2},  // ue=3 -> se=+2
		{"-2", []byte{0x28}, -2}, // ue=4 -> se=-2
		{"+3", []byte{0x30}, 3},  // ue=5 -> se=+3
		{"-3", []byte{0x38}, -3}, // ue=6 -> se=-3
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := NewBitReader(tt.data)
			got, err := br.ReadSE()
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("ReadSE() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAlignToByte(t *testing.T) {
	br := NewBitReader([]byte{0xFF, 0x00})

	// Read 3 bits
	_, _ = br.ReadBits(3)
	if br.BitsRead() != 3 {
		t.Errorf("BitsRead() = %d, want 3", br.BitsRead())
	}

	// Align to byte
	br.AlignToByte()
	if br.BitsRead() != 8 {
		t.Errorf("BitsRead() = %d, want 8 after align", br.BitsRead())
	}

	// Already aligned, should not advance
	br.AlignToByte()
	if br.BitsRead() != 8 {
		t.Errorf("BitsRead() = %d, want 8 after second align", br.BitsRead())
	}
}
