package decoder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"
)

// BenchmarkDecodeCABAC benchmarks decoding a CABAC IDR frame (Main profile).
func BenchmarkDecodeCABAC(b *testing.B) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "profile_main.264"))
	if err != nil {
		b.Fatalf("read input: %v", err)
	}
	nalus := avc.ExtractNalusFromByteStream(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dec := New()
		_, err := dec.DecodeNALUs(nalus)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecodeCAVLC benchmarks decoding a CAVLC IDR frame (Baseline profile).
func BenchmarkDecodeCAVLC(b *testing.B) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "cavlc_baseline.264"))
	if err != nil {
		b.Fatalf("read input: %v", err)
	}
	nalus := avc.ExtractNalusFromByteStream(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dec := New()
		_, err := dec.DecodeNALUs(nalus)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDecodeHD benchmarks decoding a larger HD frame.
func BenchmarkDecodeHD(b *testing.B) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "res_hd.264"))
	if err != nil {
		b.Fatalf("read input: %v", err)
	}
	nalus := avc.ExtractNalusFromByteStream(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dec := New()
		_, err := dec.DecodeNALUs(nalus)
		if err != nil {
			b.Fatal(err)
		}
	}
}
