package yuv

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Eyevinn/hi264/pkg/frame"
)

func TestWriteY4M(t *testing.T) {
	f := frame.NewFrame(16, 16)
	// Fill with a known value
	for i := range f.Y {
		f.Y[i] = 128
	}

	var buf bytes.Buffer
	err := WriteY4M(&buf, f)
	if err != nil {
		t.Fatal(err)
	}

	data := buf.String()
	if !strings.HasPrefix(data, "YUV4MPEG2 W16 H16 F1:1 Ip A1:1 C420mpeg2\n") {
		t.Errorf("unexpected Y4M header: %q", data[:60])
	}
	if !strings.Contains(data, "FRAME\n") {
		t.Error("missing FRAME tag")
	}

	// Expected size: header + "FRAME\n" + YUV data
	yuvSize := 16*16 + 2*(8*8) // Y + Cb + Cr
	headerEnd := strings.Index(data, "FRAME\n") + len("FRAME\n")
	if len(data)-headerEnd != yuvSize {
		t.Errorf("YUV data size = %d, want %d", len(data)-headerEnd, yuvSize)
	}
}
