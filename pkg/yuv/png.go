package yuv

import (
	"image"
	"image/png"
	"io"

	"github.com/Eyevinn/hi264/pkg/frame"
)

// FrameToImage converts a YUV420 frame to an RGBA image using BT.601 limited range.
func FrameToImage(f *frame.Frame) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, f.Width, f.Height))
	for y := 0; y < f.Height; y++ {
		cy := y / 2
		for x := 0; x < f.Width; x++ {
			cx := x / 2
			yVal := int(f.Y[y*f.StrideY+x])
			cb := int(f.Cb[cy*f.StrideC+cx])
			cr := int(f.Cr[cy*f.StrideC+cx])
			r, g, b := ycbcrToRGB(yVal, cb, cr)
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
	return png.Encode(w, FrameToImage(f))
}

// ycbcrToRGB converts BT.601 limited-range YCbCr to RGB.
// Y in [16,235], Cb/Cr in [16,240].
func ycbcrToRGB(y, cb, cr int) (uint8, uint8, uint8) {
	// BT.601 limited range to full range:
	//   R = 1.164*(Y-16) + 1.596*(Cr-128)
	//   G = 1.164*(Y-16) - 0.392*(Cb-128) - 0.813*(Cr-128)
	//   B = 1.164*(Y-16) + 2.017*(Cb-128)
	// Using fixed-point: multiply by 256
	c := y - 16
	d := cb - 128
	e := cr - 128
	r := (298*c + 409*e + 128) >> 8
	g := (298*c - 100*d - 208*e + 128) >> 8
	b := (298*c + 516*d + 128) >> 8
	return clampByte(r), clampByte(g), clampByte(b)
}

func clampByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
