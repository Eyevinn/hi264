package encode

import (
	"testing"

	"github.com/Eyevinn/hi264/internal/transform"
)

func TestForwardHadamard4x4RoundTrip(t *testing.T) {
	// 16 identical DC values
	var dc [16]int32
	for i := range dc {
		dc[i] = 100
	}
	fwd := ForwardHadamard4x4(dc)

	// Only [0,0] should be non-zero: 100 * 16 / 2 = 800
	if fwd[0] != 800 {
		t.Errorf("ForwardHadamard4x4 [0]=%d, want 800", fwd[0])
	}
	for i := 1; i < 16; i++ {
		if fwd[i] != 0 {
			t.Errorf("ForwardHadamard4x4 [%d]=%d, want 0", i, fwd[i])
		}
	}

	// Inverse Hadamard of [800, 0, ...] = [800, 800, ...] (all equal)
	inv := transform.InverseHadamard4x4(fwd)
	expected := int32(800)
	for i := range 16 {
		if inv[i] != expected {
			t.Errorf("inverse[%d]=%d, want %d", i, inv[i], expected)
		}
	}
}

func TestForwardHadamard2x2RoundTrip(t *testing.T) {
	dc := [4]int32{50, 50, 50, 50}
	fwd := ForwardHadamard2x2(dc)

	if fwd[0] != 200 {
		t.Errorf("ForwardHadamard2x2 [0]=%d, want 200", fwd[0])
	}
	for i := 1; i < 4; i++ {
		if fwd[i] != 0 {
			t.Errorf("ForwardHadamard2x2 [%d]=%d, want 0", i, fwd[i])
		}
	}

	inv := transform.InverseHadamard2x2(fwd)
	for i := range 4 {
		if inv[i] != 200 {
			t.Errorf("inverse[%d]=%d, want 200", i, inv[i])
		}
	}
}

func TestForwardTransform4x4Constant(t *testing.T) {
	// Test with a constant block: all values = 42
	var block [16]int32
	for i := range block {
		block[i] = 42
	}
	fwd := ForwardTransform4x4(block)

	// DC should be 16*42 = 672, all AC should be 0
	if fwd[0] != 672 {
		t.Errorf("DC = %d, want 672", fwd[0])
	}
	for i := 1; i < 16; i++ {
		if fwd[i] != 0 {
			t.Errorf("AC[%d] = %d, want 0", i, fwd[i])
		}
	}
}

func TestForwardTransform4x4StepPattern(t *testing.T) {
	// Step pattern: left half = 100, right half = 200
	var block [16]int32
	for r := range 4 {
		for c := range 4 {
			if c < 2 {
				block[r*4+c] = 100
			} else {
				block[r*4+c] = 200
			}
		}
	}

	fwd := ForwardTransform4x4(block)

	// DC = sum of all = 8*100+8*200 = 2400
	if fwd[0] != 2400 {
		t.Errorf("DC = %d, want 2400", fwd[0])
	}

	// Should have non-zero AC coefficients
	hasAC := false
	for i := 1; i < 16; i++ {
		if fwd[i] != 0 {
			hasAC = true
			break
		}
	}
	if !hasAC {
		t.Error("step pattern should produce non-zero AC coefficients")
	}
}

func TestForwardTransform4x4QuantRoundTrip(t *testing.T) {
	// Verify that forward → quant → dequant → inverse gives back the original
	// for low QP (minimal quantization loss).
	var block [16]int32
	for r := range 4 {
		for c := range 4 {
			if c < 2 {
				block[r*4+c] = 10
			} else {
				block[r*4+c] = 20
			}
		}
	}

	fwd := ForwardTransform4x4(block)
	quant := Quantize4x4(fwd, 0)

	// Dequant
	var dequant [16]int32
	qpPer := 0
	qpRem := 0
	for i := range 16 {
		row := i / 4
		col := i % 4
		v := levelScaleIdx(row, col)
		ls := levelScale4x4[qpRem][v] * 16
		if qpPer >= 4 {
			dequant[i] = quant[i] * ls << uint(qpPer-4)
		} else {
			dequant[i] = (quant[i]*ls + (1 << uint(3-qpPer))) >> uint(4-qpPer)
		}
	}

	inv := transform.InverseTransform4x4(dequant)
	for r := range 4 {
		for c := range 4 {
			expected := block[r*4+c]
			got := inv[r*4+c]
			diff := got - expected
			if diff < 0 {
				diff = -diff
			}
			if diff > 1 {
				t.Errorf("[%d,%d] = %d, want %d (diff=%d)", r, c, got, expected, diff)
			}
		}
	}
}

func TestForwardTransformDC4x4Block(t *testing.T) {
	// Uniform: all quadrants same value
	vals := [4]int32{50, 50, 50, 50}
	fwd := ForwardTransformDC4x4Block(vals)
	if fwd[0] != 50*16 {
		t.Errorf("uniform DC = %d, want %d", fwd[0], 50*16)
	}
	for i := 1; i < 16; i++ {
		if fwd[i] != 0 {
			t.Errorf("uniform AC[%d] = %d, want 0", i, fwd[i])
		}
	}

	// Step: left = 100, right = 200
	vals = [4]int32{100, 200, 100, 200}
	fwd = ForwardTransformDC4x4Block(vals)

	// Build the explicit block and compare
	var block [16]int32
	for r := range 4 {
		for c := range 4 {
			qr := r / 2
			qc := c / 2
			block[r*4+c] = vals[qr*2+qc]
		}
	}
	expected := ForwardTransform4x4(block)
	for i := range 16 {
		if fwd[i] != expected[i] {
			t.Errorf("coeff[%d] = %d, want %d", i, fwd[i], expected[i])
		}
	}
}

func TestQuantizeDequantRoundTrip(t *testing.T) {
	// For small DC values, quant+dequant should be close to identity
	for qp := 0; qp <= 30; qp += 6 {
		for dc := int32(-50); dc <= 50; dc += 10 {
			_, recon := ForwardDequantRoundTrip(dc, qp)
			// Reconstructed should be close to original * 800 / 800 = original
			// but with quantization loss
			if dc != 0 && recon == 0 && qp < 12 {
				t.Errorf("QP=%d, dc=%d: quantized to zero unexpectedly", qp, dc)
			}
		}
	}
}
