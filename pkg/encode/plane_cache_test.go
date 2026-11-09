package encode

import (
	"bytes"
	"testing"
)

func TestPlaneCacheMatchesFreshAnalysis(t *testing.T) {
	tests := []struct {
		name          string
		qp, mbX, mbY  int
		plane, offset int
		source        int
		zeroNeighbors bool
	}{
		{"repeat", 26, 1, 1, -1, 0, -1, false},
		{"quantization", 31, 1, 1, -1, 0, -1, false},
		{"top-luma", 26, 1, 1, 0, 15*32 + 16, -1, false},
		{"left-luma", 26, 1, 1, 0, 16*32 + 15, -1, false},
		{"top-cb", 26, 1, 1, 1, 7*16 + 8, -1, false},
		{"left-cb", 26, 1, 1, 1, 8*16 + 7, -1, false},
		{"top-cr", 26, 1, 1, 2, 7*16 + 8, -1, false},
		{"left-cr", 26, 1, 1, 2, 8*16 + 7, -1, false},
		{"source-luma", 26, 1, 1, -1, 0, 0, false},
		{"source-cb", 26, 1, 1, -1, 0, 1, false},
		{"source-cr", 26, 1, 1, -1, 0, 2, false},
		{"unavailable-neighbors", 26, 0, 0, -1, 0, -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given an analysis cached for an interior macroblock.
			planes := [][]uint8{
				bytes.Repeat([]byte{128}, 32*32),
				bytes.Repeat([]byte{128}, 16*16),
				bytes.Repeat([]byte{128}, 16*16),
			}
			if tt.zeroNeighbors {
				for _, plane := range planes {
					clear(plane)
				}
			}
			values := [3][4]uint8{{128, 128, 128, 128}, {128, 128, 128, 128}, {128, 128, 128, 128}}
			encoder := FrameEncoder{QP: 26}
			var cache planeMBCache
			for range 2 {
				cache.analyze(&encoder, 1, 1, values[0], values[1], values[2],
					planes[0], 32, planes[1], planes[2], 16)
			}
			// When source values, QP, neighbor pixels, or neighbor availability change.
			encoder.QP = tt.qp
			if tt.plane >= 0 {
				planes[tt.plane][tt.offset] = 0
			}
			if tt.source >= 0 {
				values[tt.source][0] = 16
			}
			got := *cache.analyze(&encoder, tt.mbX, tt.mbY, values[0], values[1], values[2],
				planes[0], 32, planes[1], planes[2], 16)
			var want planeMBAnalysis
			encoder.analyzeMBPlane(&want, tt.mbX, tt.mbY, values[0], values[1], values[2],
				planes[0], 32, planes[1], planes[2], 16)
			// Then cache lookup and fresh analysis produce identical reconstruction and coefficients.
			if got != want {
				t.Fatal("cached analysis differs from fresh analysis")
			}
		})
	}
}

func TestPlaneCacheZeroInput(t *testing.T) {
	// Given the zero-valued cache and a zero-valued key at the top-left macroblock.
	encoder := FrameEncoder{QP: 0}
	var cache planeMBCache
	luma, cb, cr := make([]uint8, 16*16), make([]uint8, 8*8), make([]uint8, 8*8)
	// When the first macroblock is analyzed.
	got := *cache.analyze(&encoder, 0, 0, [4]uint8{}, [4]uint8{}, [4]uint8{},
		luma, 16, cb, cr, 8)
	var want planeMBAnalysis
	encoder.analyzeMBPlane(&want, 0, 0, [4]uint8{}, [4]uint8{}, [4]uint8{},
		luma, 16, cb, cr, 8)
	// Then an uninitialized entry is not mistaken for a cached result.
	if got != want {
		t.Fatal("zero-valued cache returned an uninitialized analysis")
	}
}

func TestPlaneCacheClearsCoefficients(t *testing.T) {
	// Given cached nonzero AC and chroma coefficients from nonuniform prediction pixels.
	encoder := FrameEncoder{QP: 0}
	var cache planeMBCache
	luma, cb, cr := make([]uint8, 32*32), make([]uint8, 16*16), make([]uint8, 16*16)
	for x := range 16 {
		luma[15*32+16+x] = 128
	}
	luma[15*32+16] = 96
	values := [4]uint8{128, 128, 128, 128}
	first := *cache.analyze(&encoder, 1, 1, values, values, values, luma, 32, cb, cr, 16)
	if first.lumaCBP == 0 || first.chromaCBP == 0 {
		t.Fatal("fixture did not produce both nonzero luma AC and chroma coefficients")
	}
	// When all neighbors now predict the source exactly.
	for _, plane := range [][]uint8{luma, cb, cr} {
		for i := range plane {
			plane[i] = 128
		}
	}
	got := *cache.analyze(&encoder, 1, 1, values, values, values, luma, 32, cb, cr, 16)
	var want planeMBAnalysis
	encoder.analyzeMBPlane(&want, 1, 1, values, values, values, luma, 32, cb, cr, 16)
	// Then no coefficient or coded-block flag survives from the previous analysis.
	if got != want {
		t.Fatal("cached analysis retained stale coefficients or coded-block flags")
	}
}
