package encode

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Eyevinn/hi264/pkg/yuv"
)

func makeEncoder(gridStr string, cabac bool) *FrameEncoder {
	grid, _ := yuv.ParseGrid(gridStr)
	colors := yuv.ColorMap{
		'x': {Y: 200, Cb: 100, Cr: 150},
		'y': {Y: 50, Cb: 200, Cr: 80},
		'a': {Y: 235, Cb: 128, Cr: 128},
		'b': {Y: 16, Cb: 128, Cr: 128},
	}
	return &FrameEncoder{Grid: grid, Colors: colors, QP: 26, DisableDeblock: 1, CABAC: cabac}
}

// BenchmarkEncodeCAVLC2x2 benchmarks CAVLC encoding of a small 2x2 MB frame.
func BenchmarkEncodeCAVLC2x2(b *testing.B) {
	enc := makeEncoder("xy,yx", false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := enc.Encode()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEncodeCABAC2x2 benchmarks CABAC encoding of a small 2x2 MB frame.
func BenchmarkEncodeCABAC2x2(b *testing.B) {
	enc := makeEncoder("xy,yx", true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := enc.Encode()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEncodeCAVLC10x6 benchmarks CAVLC encoding of a 10x6 MB frame (160x96).
func BenchmarkEncodeCAVLC10x6(b *testing.B) {
	row1 := "xyxyxyxyxy"
	row2 := "yxyxyxyxyx"
	var gridStr strings.Builder
	for i := range 6 {
		if i > 0 {
			gridStr.WriteString(",")
		}
		if i%2 == 0 {
			gridStr.WriteString(row1)
		} else {
			gridStr.WriteString(row2)
		}
	}
	enc := makeEncoder(gridStr.String(), false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := enc.Encode()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEncodeCABAC10x6 benchmarks CABAC encoding of a 10x6 MB frame (160x96).
func BenchmarkEncodeCABAC10x6(b *testing.B) {
	row1 := "xyxyxyxyxy"
	row2 := "yxyxyxyxyx"
	var gridStr strings.Builder
	for i := range 6 {
		if i > 0 {
			gridStr.WriteString(",")
		}
		if i%2 == 0 {
			gridStr.WriteString(row1)
		} else {
			gridStr.WriteString(row2)
		}
	}
	enc := makeEncoder(gridStr.String(), true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := enc.Encode()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEncodeSPSPPS benchmarks SPS+PPS generation.
func BenchmarkEncodeSPSPPS(b *testing.B) {
	enc := makeEncoder("xy,yx", false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_ = enc.EncodeSPSPPS(&buf)
	}
}
