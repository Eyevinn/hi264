package yuv

import (
	"bytes"
	"testing"
)

func TestWriteJPEG(t *testing.T) {
	grid, err := ParseGrid("xy,yx")
	if err != nil {
		t.Fatal(err)
	}
	colors := ColorMap{
		'x': {Y: 235, Cb: 128, Cr: 128},
		'y': {Y: 16, Cb: 128, Cr: 128},
	}
	f, err := BuildFrame(grid, colors)
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := WriteJPEG(&buf, f, 85); err != nil {
		t.Fatalf("WriteJPEG: %v", err)
	}

	// JPEG files start with SOI marker 0xFF 0xD8
	data := buf.Bytes()
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		t.Error("output should start with JPEG SOI marker (0xFF 0xD8)")
	}
}
