package yuv

import (
	"image/jpeg"
	"io"

	"github.com/Eyevinn/hi264/pkg/frame"
)

// WriteJPEG writes a YUV420 frame as a JPEG image with the given quality (1-100).
func WriteJPEG(w io.Writer, f *frame.Frame, quality int) error {
	return WriteJPEGCS(w, f, quality, BT601, LimitedRange)
}

// WriteJPEGCS writes a YUV420 frame as a JPEG image using the specified color space.
func WriteJPEGCS(w io.Writer, f *frame.Frame, quality int, cs ColorSpace, rng Range) error {
	return jpeg.Encode(w, FrameToImageCS(f, cs, rng), &jpeg.Options{Quality: quality})
}
