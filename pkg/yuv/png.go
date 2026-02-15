package yuv

import (
	"image"
	"image/png"
	"io"

	"github.com/Eyevinn/hi264/pkg/frame"
)

// FrameToImage converts a YUV420 frame to an RGBA image using BT.601 limited range.
func FrameToImage(f *frame.Frame) *image.NRGBA {
	return FrameToImageCS(f, BT601, LimitedRange)
}

// FrameToImageCS converts a YUV420 frame to an RGBA image using the specified color space.
func FrameToImageCS(f *frame.Frame, cs ColorSpace, rng Range) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, f.Width, f.Height))
	for y := 0; y < f.Height; y++ {
		cy := y / 2
		for x := 0; x < f.Width; x++ {
			cx := x / 2
			yVal := int(f.Y[y*f.StrideY+x])
			cb := int(f.Cb[cy*f.StrideC+cx])
			cr := int(f.Cr[cy*f.StrideC+cx])
			r, g, b := YCbCrToRGBCS(yVal, cb, cr, cs, rng)
			off := (y-img.Rect.Min.Y)*img.Stride + (x-img.Rect.Min.X)*4
			img.Pix[off+0] = r
			img.Pix[off+1] = g
			img.Pix[off+2] = b
			img.Pix[off+3] = 255
		}
	}
	return img
}

// WritePNG writes a YUV420 frame as a PNG image.
func WritePNG(w io.Writer, f *frame.Frame) error {
	return WritePNGCS(w, f, BT601, LimitedRange)
}

// WritePNGCS writes a YUV420 frame as a PNG image using the specified color space.
func WritePNGCS(w io.Writer, f *frame.Frame, cs ColorSpace, rng Range) error {
	return png.Encode(w, FrameToImageCS(f, cs, rng))
}
