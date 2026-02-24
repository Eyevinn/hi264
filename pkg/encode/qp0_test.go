package encode

import (
	"testing"

	"github.com/Eyevinn/hi264/pkg/yuv"
)

func TestQP0FlatMB(t *testing.T) {
	// Create a simple 16x16 PlaneGrid (1 MB) with Y=16 everywhere
	pg := yuv.NewPlaneGrid(1, 1, 16)
	pg.Y[0][0] = 16
	pg.Cb[0][0] = 128
	pg.Cr[0][0] = 128

	enc := &FrameEncoder{
		Plane:  pg,
		QP:     0,
		Width:  16,
		Height: 16,
	}

	slice, err := enc.EncodeSlice(0)
	if err != nil {
		t.Fatalf("EncodeSlice: %v", err)
	}
	t.Logf("Slice size: %d bytes", len(slice))

	// Now decode and check
	// Actually, we can't easily decode here. Instead, let's directly test the reconstruction.
	// Simulate what encodeMBPlane does for MB(0,0):

	qp := 0
	mbX, mbY := 0, 0
	lumaVals := [4]uint8{16, 16, 16, 16}

	strideY := 16
	reconY := make([]uint8, 16*16)

	// DC prediction for first MB (no neighbors): should be 128
	lumaMode, lumaPredArray := selectLumaModePlane(reconY, strideY, mbX, mbY, lumaVals)
	t.Logf("lumaMode=%d, pred[0]=%d pred[1]=%d", lumaMode, lumaPredArray[0], lumaPredArray[1])

	// Forward transform
	var dcMatrix [16]int32
	var acCoeffs [16][15]int32
	lumaCBP := 0

	for blk := range 16 {
		bx := inverseRasterX4x4[blk]
		by := inverseRasterY4x4[blk]
		var res [16]int32
		for r := range 4 {
			for c := range 4 {
				py, px := by+r, bx+c
				qr, qc := py/8, px/8
				val := lumaVals[qr*2+qc]
				res[r*4+c] = int32(val) - int32(lumaPredArray[py*16+px])
			}
		}
		fwd := ForwardTransform4x4(res)
		dcMatrix[scan2raster[blk]] = fwd[0]
		copy(acCoeffs[blk][:], fwd[1:])
	}

	t.Logf("dcMatrix: %v", dcMatrix)

	hadamardResult := ForwardHadamard4x4(dcMatrix)
	t.Logf("hadamard: %v", hadamardResult)

	quantDC := QuantizeDC4x4(hadamardResult, qp, 16)
	t.Logf("quantDC: %v", quantDC)

	var quantAC [16][15]int32
	for blk := range 16 {
		var fullBlock [16]int32
		copy(fullBlock[1:], acCoeffs[blk][:])
		qBlock := Quantize4x4(fullBlock, qp)
		copy(quantAC[blk][:], qBlock[1:])
		for _, v := range quantAC[blk] {
			if v != 0 {
				lumaCBP = 1
				break
			}
		}
	}
	t.Logf("lumaCBP=%d", lumaCBP)

	// Reconstruction
	reconLuma := reconstructLumaPixel(quantDC, quantAC, lumaPredArray, qp, lumaCBP)
	t.Logf("reconLuma[0]=%d, reconLuma[1]=%d, reconLuma[255]=%d", reconLuma[0], reconLuma[1], reconLuma[255])

	if reconLuma[0] != 16 {
		t.Errorf("reconLuma[0] = %d, want 16", reconLuma[0])
	}
}
