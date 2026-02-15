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

func TestWriteY4MHeaderCSColorSpaceTags(t *testing.T) {
	tests := []struct {
		name      string
		cs        ColorSpace
		rng       Range
		wantTag   string
		wantNoTag string
	}{
		{"bt601 limited (default, no tags)", BT601, LimitedRange, "", "XCOLORSPACE"},
		{"bt709 limited", BT709, LimitedRange, "XCOLORSPACE=bt709", ""},
		{"bt2020 limited", BT2020, LimitedRange, "XCOLORSPACE=bt2020", ""},
		{"bt601 full", BT601, FullRange, "XCOLORRANGE=FULL", ""},
		{"bt709 full", BT709, FullRange, "XCOLORSPACE=bt709", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteY4MHeaderCS(&buf, 16, 16, tt.cs, tt.rng); err != nil {
				t.Fatal(err)
			}
			header := buf.String()
			if tt.wantTag != "" && !strings.Contains(header, tt.wantTag) {
				t.Errorf("header %q missing tag %q", header, tt.wantTag)
			}
			if tt.wantNoTag != "" && strings.Contains(header, tt.wantNoTag) {
				t.Errorf("header %q should not contain %q", header, tt.wantNoTag)
			}
		})
	}
}
