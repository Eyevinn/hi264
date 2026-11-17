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

	// Check the production analysis path's reconstruction.
	strideY := 16
	reconY := make([]uint8, 16*16)
	strideC := 8
	reconCb, reconCr := make([]uint8, 8*8), make([]uint8, 8*8)
	cbVals, crVals := pg.MBChromaSub(0, 0)
	var analysis planeMBAnalysis
	enc.analyzeMBPlane(&analysis, 0, 0, pg.MBLumaValues(0, 0), cbVals, crVals,
		reconY, strideY, reconCb, reconCr, strideC)

	if analysis.reconY[0] != 16 {
		t.Errorf("analysis.reconY[0] = %d, want 16", analysis.reconY[0])
	}
}
