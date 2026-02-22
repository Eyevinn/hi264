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
