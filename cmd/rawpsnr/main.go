// Command rawpsnr compares two raw YUV420 files and reports PSNR metrics.
//
// Usage:
//
//	rawpsnr -w WIDTH -h HEIGHT [-per-mb] [-csv out.csv] file1.yuv file2.yuv
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
)

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("rawpsnr", flag.ContinueOnError)
	width := fs.Int("w", 0, "frame width in pixels (required)")
	height := fs.Int("h", 0, "frame height in pixels (required)")
	perMB := fs.Bool("per-mb", false, "output per-16x16-macroblock PSNR")
	csvFile := fs.String("csv", "", "write per-MB PSNR to CSV file")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *width <= 0 || *height <= 0 {
		return fmt.Errorf("-w and -h are required and must be positive")
	}
	remaining := fs.Args()
	if len(remaining) != 2 {
		return fmt.Errorf("usage: rawpsnr -w WIDTH -h HEIGHT file1.yuv file2.yuv")
	}

	w, h := *width, *height
	frameSize := w*h + w*h/2 // YUV420: Y + Cb + Cr

	a, err := os.ReadFile(remaining[0])
	if err != nil {
		return fmt.Errorf("reading %s: %w", remaining[0], err)
	}
	b, err := os.ReadFile(remaining[1])
	if err != nil {
		return fmt.Errorf("reading %s: %w", remaining[1], err)
	}

	if len(a) != len(b) {
		return fmt.Errorf("file sizes differ: %d vs %d", len(a), len(b))
	}
	if len(a)%frameSize != 0 {
		return fmt.Errorf("file size %d not a multiple of frame size %d (%dx%d YUV420)", len(a), frameSize, w, h)
	}

	numFrames := len(a) / frameSize
	chromaW, chromaH := w/2, h/2
	ySize := w * h
	cbSize := chromaW * chromaH

	var totalY, totalCb, totalCr float64

	var cw *csv.Writer
	if *csvFile != "" {
		cf, cerr := os.Create(*csvFile)
		if cerr != nil {
			return cerr
		}
		defer cf.Close()
		cw = csv.NewWriter(cf)
		_ = cw.Write([]string{"frame", "mb_x", "mb_y", "psnr_y"})
		defer cw.Flush()
	}

	for f := range numFrames {
		off := f * frameSize
		yA := a[off : off+ySize]
		yB := b[off : off+ySize]
		cbA := a[off+ySize : off+ySize+cbSize]
		cbB := b[off+ySize : off+ySize+cbSize]
		crA := a[off+ySize+cbSize : off+frameSize]
		crB := b[off+ySize+cbSize : off+frameSize]

		psnrY := computePSNR(yA, yB)
		psnrCb := computePSNR(cbA, cbB)
		psnrCr := computePSNR(crA, crB)
		weighted := (6*psnrY + psnrCb + psnrCr) / 8

		totalY += psnrY
		totalCb += psnrCb
		totalCr += psnrCr

		if numFrames > 1 {
			fmt.Printf("Frame %d: Y=%.2f dB  Cb=%.2f dB  Cr=%.2f dB  Avg=%.2f dB\n",
				f, psnrY, psnrCb, psnrCr, weighted)
		}

		if *perMB || cw != nil {
			mbCols := (w + 15) / 16
			mbRows := (h + 15) / 16

			for my := range mbRows {
				for mx := range mbCols {
					p := blockPSNR(yA, yB, w, h, mx*16, my*16, 16)
					if *perMB {
						fmt.Printf("  MB(%d,%d): Y=%.2f dB\n", mx, my, p)
					}
					if cw != nil {
						_ = cw.Write([]string{
							strconv.Itoa(f),
							strconv.Itoa(mx),
							strconv.Itoa(my),
							fmt.Sprintf("%.2f", p),
						})
					}
				}
			}
		}
	}

	avgY := totalY / float64(numFrames)
	avgCb := totalCb / float64(numFrames)
	avgCr := totalCr / float64(numFrames)
	avgWeighted := (6*avgY + avgCb + avgCr) / 8

	if numFrames > 1 {
		fmt.Println("---")
	}
	fmt.Printf("Y=%.2f dB  Cb=%.2f dB  Cr=%.2f dB  Avg=%.2f dB  (%d frame(s), %dx%d)\n",
		avgY, avgCb, avgCr, avgWeighted, numFrames, w, h)
	return nil
}

func computePSNR(a, b []uint8) float64 {
	var mse float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		mse += d * d
	}
	mse /= float64(len(a))
	if mse == 0 {
		return math.Inf(1)
	}
	return 10.0 * math.Log10(255.0*255.0/mse)
}

func blockPSNR(a, b []uint8, stride, height, x0, y0, size int) float64 {
	var mse float64
	var count int
	for py := range size {
		y := y0 + py
		if y >= height {
			break
		}
		for px := range size {
			x := x0 + px
			if x >= stride {
				break
			}
			idx := y*stride + x
			d := float64(a[idx]) - float64(b[idx])
			mse += d * d
			count++
		}
	}
	if count == 0 {
		return math.Inf(1)
	}
	mse /= float64(count)
	if mse == 0 {
		return math.Inf(1)
	}
	return 10.0 * math.Log10(255.0*255.0/mse)
}
