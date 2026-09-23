package decoder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Eyevinn/mp4ff/avc"
)

// Fuzz the full decode path with mutated real H.264 streams, catching panics.
func FuzzDecodeNALUs(f *testing.F) {
	// Seed with every golden .264 stream.
	goldenDir := filepath.Join("..", "..", "testdata", "golden")
	entries, _ := os.ReadDir(goldenDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".264" {
			if data, err := os.ReadFile(filepath.Join(goldenDir, e.Name())); err == nil {
				f.Add(data)
			}
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic on decode: %v", r)
			}
		}()
		nalus := avc.ExtractNalusFromByteStream(data)
		dec := New()
		// Try both entry points.
		_, _ = dec.DecodeNALUs(nalus)
		dec2 := New()
		_, _ = dec2.DecodeIDRFrames(nalus)
	})
}
