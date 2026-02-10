package yuv

import (
	"fmt"
	"io"

	"github.com/Eyevinn/hi264/pkg/frame"
)

// WriteY4MHeader writes the YUV4MPEG2 file header to w.
func WriteY4MHeader(w io.Writer, width, height int) error {
	header := fmt.Sprintf("YUV4MPEG2 W%d H%d F1:1 Ip A1:1 C420mpeg2\n", width, height)
	_, err := io.WriteString(w, header)
	if err != nil {
		return fmt.Errorf("write Y4M header: %w", err)
	}
	return nil
}

// WriteY4MFrame writes a single FRAME tag + YUV data to w.
func WriteY4MFrame(w io.Writer, f *frame.Frame) error {
	_, err := io.WriteString(w, "FRAME\n")
	if err != nil {
		return fmt.Errorf("write FRAME tag: %w", err)
	}
	yuv := f.YUV420Bytes()
	_, err = w.Write(yuv)
	if err != nil {
		return fmt.Errorf("write YUV data: %w", err)
	}
	return nil
}

// WriteY4M writes a single-frame Y4M file (YUV4MPEG2 format) to w.
func WriteY4M(w io.Writer, f *frame.Frame) error {
	if err := WriteY4MHeader(w, f.Width, f.Height); err != nil {
		return err
	}
	return WriteY4MFrame(w, f)
}
