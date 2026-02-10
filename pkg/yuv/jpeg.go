package yuv

import (
	"image/jpeg"
	"io"

	"github.com/Eyevinn/hi264/pkg/frame"
)

// WriteJPEG writes a YUV420 frame as a JPEG image with the given quality (1-100).
func WriteJPEG(w io.Writer, f *frame.Frame, quality int) error {
	return jpeg.Encode(w, FrameToImage(f), &jpeg.Options{Quality: quality})
}
