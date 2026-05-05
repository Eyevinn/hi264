// Command hi264gen generates H.264 bitstreams or raw images from grid patterns.
//
// Each character in a grid maps to one 16x16 macroblock filled with a single
// flat color. For H.264 output (.264, .mp4), macroblocks are encoded as I_16x16
// with DC prediction. For raw output (.png, .jpg, .yuv, .y4m), the grid pattern
// is rendered directly without H.264 encoding.
//
// In grid-only mode (-gi or -gp/-gc, no -w/-h), the frame size equals the grid
// size in macroblocks. With -w/-h, the pattern tiles to fill custom dimensions,
// and -text adds a text overlay using format patterns.
//
// Usage:
//
//	hi264gen -gi pattern.gridimg -o output.264
//	hi264gen -gi pattern.gridimg -w 176 -h 80 -n 10 -text "%03d" -o output.264
//	hi264gen -w 176 -h 80 -n 10 -text "%03d" -o output.264
//	hi264gen -gi pattern.gridimg -o reference.png
//	hi264gen -smpte -w 320 -h 240 -n 100 -text "%03d" -f 264 -o - | ffplay -i -
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"strings"

	"github.com/Eyevinn/hi264/internal"
	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/hi264/pkg/yuv"
	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/mp4"
)

const appName = "hi264gen"

var usg = `%s - generate H.264 bitstreams or raw images from grid patterns.

Usage:

  %s -gi pattern.gridimg -o output.264
  %s -gi photo.png -w 320 -h 240 -text "%%03d" -o output.mp4
  %s -gi pattern.gridimg -w 176 -h 80 -n 10 -text "%%03d" -o output.264
  %s -w 176 -h 80 -n 10 -text "%%03d" -o output.264
  %s -smpte -w 320 -h 240 -n 100 -text "%%03d" -f 264 -o - | ffplay -i -

Output format is detected from the file extension, or set explicitly with -f:
  264        H.264 Annex-B bitstream (SPS+PPS once, then N slices)
  mp4        Fragmented MP4 (CMAF-compatible, configurable fps and fragment duration)
  y4m        Y4M multi-frame file
  yuv        Raw YUV420 (auto-adds _WxH_yuv420p suffix)
  png        PNG image (numbered _NNNN if -n > 1)
  jpg/jpeg   JPEG image (numbered _NNNN if -n > 1, -q for quality)
  %%          Numbered images (e.g. frame_%%03d.png or frame_%%03d.jpg)

Use -o - to write to stdout (requires -f to set the format).

Options:
`

type colorFlags []string

func (c *colorFlags) String() string { return strings.Join(*c, ", ") }
func (c *colorFlags) Set(s string) error {
	*c = append(*c, s)
	return nil
}

type options struct {
	version     bool
	grid        string
	colors      colorFlags
	imgFile     string
	format      string
	rgb         bool
	smpte       bool
	width       int
	height      int
	numFrames   int
	text        string
	textScale   int
	textBg      string
	fg          string
	bg          string
	qp          int
	cabac       bool
	useDeblock  bool
	idrInterval int
	bpp         int
	kbps        int
	jpegQual    int
	fps         int
	fragDur     int
	colorspace  string
	fullRange   bool
	output      string
	cpuProfile  string
	blockSize   int            // grid block size (16 or 8), set from @8x8 directive or -8x8 flag
	use8x8      bool           // CLI flag to enable 8x8 block resolution
	bgPlane     *yuv.PlaneGrid // PNG/JPEG image background (nil for .gridimg)
	bgImage     image.Image    // source image for deferred scaling (nil after scaling)
}

func parseOptions(fs *flag.FlagSet, args []string) (*options, error) {
	var opts options
	fs.BoolVar(&opts.version, "version", false, "Get hi264 version")
	fs.StringVar(&opts.grid, "gp", "", "grid pattern (rows separated by commas)")
	fs.Var(&opts.colors, "gc", "color spec: char=Y,Cb,Cr (or R,G,B with -rgb) (repeatable)")
	fs.StringVar(&opts.imgFile, "gi", "", "grid image file (.gridimg/.png/.jpg/.jpeg, alternative to -gp/-gc)")
	fs.StringVar(&opts.format, "f", "", "output format (264, mp4, y4m, yuv, png, jpg); required with -o -")
	fs.BoolVar(&opts.rgb, "rgb", false, "interpret -gc color values as RGB instead of YCbCr")
	fs.BoolVar(&opts.smpte, "smpte", false, "use 75% SMPTE color bars as pattern")
	fs.IntVar(&opts.width, "w", 0, "frame width in pixels (must be even; default: grid-derived)")
	fs.IntVar(&opts.height, "h", 0, "frame height in pixels (must be even; default: grid-derived)")
	fs.IntVar(&opts.numFrames, "n", 1, "number of frames")
	fs.StringVar(&opts.text, "text", "",
		"text overlay pattern (e.g. \"%03d\"); A-Z 0-9 !#%+-./:[]=?_() (lowercase auto-uppercased)")
	fs.IntVar(&opts.textScale, "text-scale", 0, "text scale factor (0 = auto)")
	fs.StringVar(&opts.textBg, "text-bg", "", "text background box color (R,G,B)")
	fs.StringVar(&opts.fg, "fg", "255,255,255", "foreground RGB color for text (R,G,B)")
	fs.StringVar(&opts.bg, "bg", "0,0,0", "background RGB color for text (R,G,B)")
	fs.IntVar(&opts.qp, "qp", 26, "quantization parameter (0-51)")
	fs.BoolVar(&opts.cabac, "cabac", false, "use CABAC entropy coding (Main profile)")
	fs.BoolVar(&opts.useDeblock, "use-deblock", false, "enable deblocking filter (off by default)")
	fs.IntVar(&opts.idrInterval, "idr-interval", 0, "frames between IDR frames (0 = every frame is IDR)")
	fs.IntVar(&opts.bpp, "bpp", 0, "target bytes per picture (pad with filler NALUs)")
	fs.IntVar(&opts.kbps, "kbps", 0, "target bitrate in kbit/s (converted to bytes per picture using -fps)")
	fs.IntVar(&opts.jpegQual, "q", 85, "JPEG quality (1-100)")
	fs.IntVar(&opts.fps, "fps", 25, "framerate for MP4 and timestamp specifiers")
	fs.IntVar(&opts.fragDur, "frag-dur", 25, "fragment duration in frames for MP4 output")
	fs.StringVar(&opts.colorspace, "colorspace", "bt601", "color space (bt601, bt709, bt2020)")
	fs.BoolVar(&opts.fullRange, "full-range", false, "use full-range YCbCr (0-255)")
	fs.BoolVar(&opts.use8x8, "8x8", false, "use 8x8 block resolution (double-res text glyphs)")
	fs.StringVar(&opts.cpuProfile, "cpuprofile", "", "write CPU profile to file")
	fs.StringVar(&opts.output, "o", "", "output file (required)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, usg, appName, appName, appName, appName, appName, appName)
		fs.PrintDefaults()
	}
	err := fs.Parse(args[1:])
	return &opts, err
}

func parseRGBWithCS(s string, cs yuv.ColorSpace, rng yuv.Range) (yuv.Color, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return yuv.Color{}, fmt.Errorf("expected R,G,B got %q", s)
	}
	var rgb [3]uint8
	for i, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return yuv.Color{}, fmt.Errorf("invalid color component %q: %w", p, err)
		}
		if v < 0 || v > 255 {
			return yuv.Color{}, fmt.Errorf("color component %d out of range [0,255]", v)
		}
		rgb[i] = uint8(v)
	}
	return yuv.RGBToYCbCrCS(rgb[0], rgb[1], rgb[2], cs, rng), nil
}

// bufferedWriteCloser wraps a bufio.Writer around a file, flushing on Close.
type bufferedWriteCloser struct {
	bw     *bufio.Writer
	closer io.Closer // nil for stdout
}

func (b *bufferedWriteCloser) Write(p []byte) (int, error) { return b.bw.Write(p) }

func (b *bufferedWriteCloser) Close() error {
	if err := b.bw.Flush(); err != nil {
		return err
	}
	if b.closer != nil {
		return b.closer.Close()
	}
	return nil
}

func openOutput(path string) (io.WriteCloser, error) {
	if path == "-" {
		return &bufferedWriteCloser{bw: bufio.NewWriter(os.Stdout)}, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &bufferedWriteCloser{bw: bufio.NewWriter(f), closer: f}, nil
}

// resolveFormat determines the output format from -f flag or file extension.
func resolveFormat(output, format string) (string, error) {
	if format != "" {
		f := strings.ToLower(strings.TrimPrefix(format, "."))
		switch f {
		case "264", "mp4", "y4m", "yuv", "png", "jpg", "jpeg":
			return f, nil
		default:
			return "", fmt.Errorf("unknown format %q (use 264, mp4, y4m, yuv, png, or jpg)", f)
		}
	}
	if output == "-" {
		return "", fmt.Errorf("-f is required when writing to stdout (-o -)")
	}
	if strings.Contains(output, "%") {
		return "pattern", nil
	}
	ext := strings.ToLower(filepath.Ext(output))
	switch ext {
	case ".264":
		return "264", nil
	case ".mp4":
		return "mp4", nil
	case ".y4m":
		return "y4m", nil
	case ".yuv":
		return "yuv", nil
	case ".png":
		return "png", nil
	case ".jpg", ".jpeg":
		return "jpg", nil
	default:
		return "", fmt.Errorf("unknown output format %q"+
			" (use .264, .mp4, .y4m, .yuv, .png, .jpg, or %% pattern; or set -f)", ext)
	}
}

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet(appName, flag.ContinueOnError)
	opts, err := parseOptions(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if opts.cpuProfile != "" {
		f, err := os.Create(opts.cpuProfile)
		if err != nil {
			return fmt.Errorf("create CPU profile: %w", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			return fmt.Errorf("start CPU profile: %w", err)
		}
		defer pprof.StopCPUProfile()
	}

	if opts.version {
		fmt.Printf("%s %s\n", appName, internal.GetVersion())
		return nil
	}

	// Convert literal \n sequences to newlines in text pattern.
	opts.text = strings.ReplaceAll(opts.text, `\n`, "\n")

	if opts.output == "" {
		fs.Usage()
		return fmt.Errorf("-o output file is required")
	}

	if (opts.grid != "" || len(opts.colors) > 0) && opts.imgFile != "" {
		return fmt.Errorf("-gi and -gp/-gc are mutually exclusive")
	}
	if opts.smpte && (opts.imgFile != "" || opts.grid != "") {
		return fmt.Errorf("-smpte is mutually exclusive with -gi and -gp/-gc")
	}

	// Parse color space early since it's needed for image file parsing
	cs, err := yuv.ParseColorSpace(opts.colorspace)
	if err != nil {
		return err
	}
	var rng yuv.Range
	if opts.fullRange {
		rng = yuv.FullRange
	}

	// Parse grid input (if any)
	var patGrid *yuv.Grid
	var patColors yuv.ColorMap
	opts.blockSize = 16 // default: each grid char = 16x16 macroblock
	if opts.use8x8 {
		opts.blockSize = 8
	}
	isStdout := opts.output == "-"
	hasGridInput := opts.imgFile != "" || opts.grid != "" || opts.smpte
	if opts.smpte {
		// Deferred: SMPTE grid created after mbWidth is known.
	} else if opts.imgFile != "" {
		ext := strings.ToLower(filepath.Ext(opts.imgFile))
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
			img, ierr := yuv.LoadImage(opts.imgFile)
			if ierr != nil {
				return ierr
			}
			// Store source image; PlaneGrid created after dimensions are known
			opts.bgImage = img
		} else {
			f, ferr := os.Open(opts.imgFile)
			if ferr != nil {
				return ferr
			}
			defer f.Close()
			res, perr := yuv.ParseImageFileFull(f, opts.rgb, cs, rng)
			if perr != nil {
				return fmt.Errorf("parsing image file: %w", perr)
			}
			patGrid = res.Grid
			patColors = res.Colors
			// File @8x8 directive overrides default, but CLI -8x8 flag takes priority
			if !opts.use8x8 {
				opts.blockSize = res.BlockSize
			}
			if opts.colorspace == "bt601" {
				cs = res.CS
			}
		}
	} else if opts.grid != "" {
		patGrid, err = yuv.ParseGrid(opts.grid)
		if err != nil {
			return fmt.Errorf("parsing grid: %w", err)
		}
		patColors = make(yuv.ColorMap)
		for _, cspec := range opts.colors {
			ch, c, cerr := yuv.ParseColorSpec(cspec, opts.rgb)
			if cerr != nil {
				return fmt.Errorf("parsing color: %w", cerr)
			}
			patColors[ch] = c
		}
	}

	tiled := opts.width > 0 || opts.height > 0

	// Validate: need either grid input or -w/-h with -text
	if !hasGridInput && !tiled {
		fs.Usage()
		return fmt.Errorf("either grid input (-gi or -gp/-gc) or -w/-h with -text is required")
	}
	if !hasGridInput && tiled && opts.text == "" {
		return fmt.Errorf("-text is required when using -w/-h without grid input (-gi or -gp/-gc)")
	}

	if tiled {
		if opts.width <= 0 || opts.width%2 != 0 {
			return fmt.Errorf("width must be a positive even number, got %d", opts.width)
		}
		if opts.height <= 0 || opts.height%2 != 0 {
			return fmt.Errorf("height must be a positive even number, got %d", opts.height)
		}
	}
	if opts.numFrames <= 0 {
		return fmt.Errorf("number of frames must be positive, got %d", opts.numFrames)
	}
	if opts.qp < 0 || opts.qp > 51 {
		return fmt.Errorf("QP must be 0-51, got %d", opts.qp)
	}
	if opts.idrInterval < 0 {
		return fmt.Errorf("idr-interval must be non-negative, got %d", opts.idrInterval)
	}
	if opts.bpp < 0 {
		return fmt.Errorf("bpp must be non-negative, got %d", opts.bpp)
	}
	if opts.kbps < 0 {
		return fmt.Errorf("kbps must be non-negative, got %d", opts.kbps)
	}
	if opts.bpp > 0 && opts.kbps > 0 {
		return fmt.Errorf("-bpp and -kbps are mutually exclusive")
	}
	if opts.fps <= 0 {
		return fmt.Errorf("fps must be positive, got %d", opts.fps)
	}
	if opts.kbps > 0 {
		opts.bpp = opts.kbps * 1000 / 8 / opts.fps
	}
	if opts.fragDur <= 0 {
		return fmt.Errorf("frag-dur must be positive, got %d", opts.fragDur)
	}

	fgColor, err := parseRGBWithCS(opts.fg, cs, rng)
	if err != nil {
		return fmt.Errorf("parsing -fg: %w", err)
	}
	bgColor, err := parseRGBWithCS(opts.bg, cs, rng)
	if err != nil {
		return fmt.Errorf("parsing -bg: %w", err)
	}

	// Determine frame dimensions
	var frameW, frameH int
	if tiled {
		frameW = opts.width
		frameH = opts.height
	} else if opts.bgImage != nil {
		// Image mode without -w/-h: derive from image at block resolution
		b := opts.bgImage.Bounds()
		imgW := b.Max.X - b.Min.X
		imgH := b.Max.Y - b.Min.Y
		frameW = (imgW / opts.blockSize) * opts.blockSize
		frameH = (imgH / opts.blockSize) * opts.blockSize
	} else if opts.smpte {
		frameW = 7 * 16
		frameH = 16
	} else {
		frameW = patGrid.Width * opts.blockSize
		frameH = patGrid.Height * opts.blockSize
	}

	mbWidth := (frameW + 15) / 16
	mbHeight := (frameH + 15) / 16

	// Create PlaneGrid from image now that target dimensions are known.
	// With -w/-h: scale image to cover the target. Without: 1:1 block sampling.
	if opts.bgImage != nil {
		blockW := mbWidth * (16 / opts.blockSize)
		blockH := mbHeight * (16 / opts.blockSize)
		if tiled {
			opts.bgPlane = yuv.ScaleImageToPlaneGrid(opts.bgImage, blockW, blockH, opts.blockSize, cs, rng)
		} else {
			opts.bgPlane = yuv.ImageToPlaneGrid(opts.bgImage, opts.blockSize, cs, rng)
		}
		opts.bgImage = nil // release source image
		if opts.bgPlane.Width == 0 || opts.bgPlane.Height == 0 {
			return fmt.Errorf("image %s too small for %dx%d blocks", opts.imgFile, opts.blockSize, opts.blockSize)
		}
	}

	// Create SMPTE grid now that mbWidth is known.
	// In 8x8 mode, distribute bars across block columns (8px granularity).
	if opts.smpte {
		if opts.blockSize == 8 {
			patGrid, patColors = yuv.SMPTEBarsGridCS(mbWidth*2, cs, rng)
		} else {
			patGrid, patColors = yuv.SMPTEBarsGridCS(mbWidth, cs, rng)
		}
	}

	textScale := opts.textScale
	if textScale == 0 && opts.text != "" {
		sampleText := yuv.FormatText(opts.text, 0, opts.fps)
		if opts.blockSize == 8 {
			textScale = yuv.AutoTextScale2x(sampleText, mbWidth*2, mbHeight*2)
		} else {
			textScale = yuv.AutoTextScale(sampleText, mbWidth, mbHeight)
		}
	}

	var textBg *yuv.Color
	if opts.textBg != "" && opts.text != "" {
		c, derr := parseRGBWithCS(opts.textBg, cs, rng)
		if derr != nil {
			return fmt.Errorf("parsing -text-bg: %w", derr)
		}
		textBg = &c
	}

	disableDeblock := 1
	if opts.useDeblock {
		disableDeblock = 0
	}

	// Resolve output format
	outFmt, err := resolveFormat(opts.output, opts.format)
	if err != nil {
		return err
	}

	// Validate stdout restrictions
	if isStdout {
		switch outFmt {
		case "yuv":
			return fmt.Errorf("yuv format cannot be used with stdout (adds _WxH suffix to filename)")
		case "pattern":
			return fmt.Errorf("pattern format cannot be used with stdout")
		case "png", "jpg":
			if opts.numFrames > 1 {
				return fmt.Errorf("%s format with -n > 1 cannot be used with stdout", outFmt)
			}
		}
	}

	switch outFmt {
	case "264":
		return generateH264(opts, frameW, frameH, mbWidth, mbHeight, disableDeblock,
			bgColor, fgColor, patGrid, patColors, tiled, textScale, textBg, cs, rng)
	case "mp4":
		return generateMP4(opts, frameW, frameH, mbWidth, mbHeight, disableDeblock,
			bgColor, fgColor, patGrid, patColors, tiled, textScale, textBg, cs, rng)
	case "y4m":
		return generateY4M(opts, frameW, frameH, mbWidth, mbHeight,
			bgColor, fgColor, patGrid, patColors, tiled, textScale, textBg, cs, rng)
	case "yuv":
		return generateYUV(opts, frameW, frameH, mbWidth, mbHeight,
			bgColor, fgColor, patGrid, patColors, tiled, textScale, textBg)
	case "pattern":
		return generateFormattedImages(opts, frameW, frameH, mbWidth, mbHeight,
			bgColor, fgColor, patGrid, patColors, tiled, textScale, textBg, cs, rng)
	case "png", "jpg", "jpeg":
		return generateNumberedImages(opts, frameW, frameH, mbWidth, mbHeight,
			bgColor, fgColor, patGrid, patColors, tiled, textScale, textBg, cs, rng)
	default:
		return fmt.Errorf("unknown output format %q", outFmt)
	}
}

// buildFrameGrid creates a PlaneGrid for frame i.
// In grid-only mode (tiled=false), the pattern is used directly.
// In text mode (tiled=true, text!=""), a text grid is created,
// optionally with a tiled pattern background.
// In tiled mode without text (tiled=true, text==""), the pattern
// tiles to fill the frame.
// blockSize is the block size for the pattern grid (16 or 8).
// bgPlane is an optional PlaneGrid from a PNG/JPEG image; when set,
// the PlaneGrid-based flow is used (tile + text overlay) instead of Grid.
func buildFrameGrid(i int, patGrid *yuv.Grid, patColors yuv.ColorMap,
	mbW, mbH, scale, fps int, text string, bg, fg yuv.Color,
	textBg *yuv.Color, tiled bool, blockSize int, bgPlane *yuv.PlaneGrid) (*yuv.PlaneGrid, error) {

	// PlaneGrid-based flow for PNG/JPEG backgrounds
	if bgPlane != nil {
		pg := bgPlane
		if tiled {
			// Tile to fill the target frame
			blockW := mbW * (16 / blockSize)
			blockH := mbH * (16 / blockSize)
			if pg.Width != blockW || pg.Height != blockH {
				pg = yuv.TilePlaneGrid(pg, blockW, blockH)
			}
		}
		if text != "" {
			formatted := yuv.FormatText(text, i, fps)
			// Clone so each frame gets a fresh copy for text overlay
			clone := yuv.NewPlaneGrid(pg.Width, pg.Height, pg.BlockSize)
			for y := range pg.Height {
				copy(clone.Y[y], pg.Y[y])
				copy(clone.Cb[y], pg.Cb[y])
				copy(clone.Cr[y], pg.Cr[y])
			}
			if err := yuv.OverlayTextOnPlane(clone, formatted, scale, fg, textBg); err != nil {
				return nil, err
			}
			return clone, nil
		}
		return pg, nil
	}

	var grid *yuv.Grid
	var colors yuv.ColorMap

	if !tiled {
		grid, colors = patGrid, patColors
		return yuv.GridToPlaneGridBS(grid, colors, blockSize)
	} else if text != "" {
		var err error
		formatted := yuv.FormatText(text, i, fps)
		use2xFont := blockSize == 8 || (scale >= 2 && scale%2 == 0)
		if use2xFont {
			// Double-resolution text: 6×10 glyphs at 8x8 block granularity.
			// For blockSize=16 with even scale, render at half scale with 8x8
			// blocks for 4× the glyph detail at the same pixel footprint.
			blockW := mbW * 2
			blockH := mbH * 2
			scale2x := scale
			if blockSize == 16 {
				scale2x = scale / 2
			}
			grid, colors, err = yuv.TextGrid2x(formatted, blockW, blockH, scale2x, bg, fg, textBg)
			if err != nil {
				return nil, err
			}
			if patGrid != nil {
				// When blockSize=16 with even-scale optimization, upscale
				// the MB-resolution pattern to 8x8 block resolution.
				// When blockSize=8, the pattern is already at block resolution.
				tilePat := patGrid
				if blockSize == 16 {
					tilePat = yuv.UpscaleGrid(patGrid, 2)
				}
				grid, colors, err = yuv.TileBackground(grid, colors, tilePat, patColors)
				if err != nil {
					return nil, err
				}
			}
			return yuv.GridToPlaneGridBS(grid, colors, 8)
		}
		grid, colors, err = yuv.TextGrid(formatted, mbW, mbH, scale, bg, fg, textBg)
		if err != nil {
			return nil, err
		}
		if patGrid != nil {
			grid, colors, err = yuv.TileBackground(grid, colors, patGrid, patColors)
			if err != nil {
				return nil, err
			}
		}
	} else {
		// Tiled mode without text: solid bg + tile pattern
		gridW, gridH := mbW, mbH
		bs := 16
		if blockSize == 8 {
			gridW, gridH = mbW*2, mbH*2
			bs = 8
		}
		chars := make([][]byte, gridH)
		for y := range gridH {
			chars[y] = make([]byte, gridW)
			for x := range gridW {
				chars[y][x] = '.'
			}
		}
		bgGrid := &yuv.Grid{Chars: chars, Width: gridW, Height: gridH}
		bgColors := yuv.ColorMap{'.': bg}

		var err error
		grid, colors, err = yuv.TileBackground(bgGrid, bgColors, patGrid, patColors)
		if err != nil {
			return nil, err
		}
		return yuv.GridToPlaneGridBS(grid, colors, bs)
	}

	return yuv.GridToPlaneGrid(grid, colors)
}

func generateH264(opts *options, frameW, frameH, mbWidth, mbHeight, disableDeblock int,
	bg, fg yuv.Color, patGrid *yuv.Grid, patColors yuv.ColorMap, tiled bool,
	textScale int, textBg *yuv.Color, cs yuv.ColorSpace, rng yuv.Range) error {

	f, err := openOutput(opts.output)
	if err != nil {
		return err
	}
	defer f.Close()

	maxRef := 0
	if opts.idrInterval > 0 {
		maxRef = 1
	}

	plane, err := buildFrameGrid(0, patGrid, patColors,
		mbWidth, mbHeight, textScale, opts.fps, opts.text, bg, fg, textBg, tiled, opts.blockSize, opts.bgPlane)
	if err != nil {
		return err
	}

	enc := &encode.FrameEncoder{
		Plane:           plane,
		QP:              opts.qp,
		DisableDeblock:  disableDeblock,
		CABAC:           opts.cabac,
		MaxNumRefFrames: maxRef,
		Width:           frameW,
		Height:          frameH,
		ColorSpace:      cs,
		Range:           rng,
		FPS:             opts.fps,
		Kbps:            opts.kbps,
	}

	// Write SPS+PPS once
	var spspps bytes.Buffer
	if err := enc.EncodeSPSPPS(&spspps); err != nil {
		return err
	}
	if _, err := f.Write(spspps.Bytes()); err != nil {
		return err
	}

	if opts.idrInterval > 0 {
		err = generateH264WithPSkip(f, opts, mbWidth, mbHeight,
			bg, fg, enc, patGrid, patColors, tiled, textScale, textBg)
	} else {
		err = generateH264AllIDR(f, opts, frameW, frameH, mbWidth, mbHeight, disableDeblock,
			bg, fg, patGrid, patColors, tiled, textScale, textBg, cs, rng)
	}
	if err != nil {
		return err
	}

	entropy := "CAVLC"
	if opts.cabac {
		entropy = "CABAC"
	}
	idrInfo := ""
	if opts.idrInterval > 0 {
		idrInfo = fmt.Sprintf(", IDR every %d frames", opts.idrInterval)
	}
	bppInfo := ""
	if opts.bpp > 0 {
		bppInfo = fmt.Sprintf(", bpp=%d", opts.bpp)
	}
	fmt.Fprintf(os.Stderr, "Wrote %d frames %dx%d (%s, QP=%d%s%s) to %s\n",
		opts.numFrames, frameW, frameH, entropy, opts.qp, idrInfo, bppInfo, opts.output)
	return nil
}

// padSlice pads a slice to the target bpp using encode.PadSlice, adding frame context to errors.
func padSlice(slice []byte, bpp int, frameIdx int) ([]byte, error) {
	if bpp <= 0 {
		return slice, nil
	}
	padded, err := encode.PadSlice(slice, bpp)
	if err != nil {
		return nil, fmt.Errorf("frame %d: %w", frameIdx, err)
	}
	return padded, nil
}

func generateH264AllIDR(f io.Writer, opts *options, frameW, frameH, mbWidth, mbHeight, disableDeblock int,
	bg, fg yuv.Color, patGrid *yuv.Grid, patColors yuv.ColorMap, tiled bool,
	textScale int, textBg *yuv.Color, cs yuv.ColorSpace, rng yuv.Range) error {

	for i := range opts.numFrames {
		plane, err := buildFrameGrid(i, patGrid, patColors,
			mbWidth, mbHeight, textScale, opts.fps, opts.text, bg, fg, textBg, tiled, opts.blockSize, opts.bgPlane)
		if err != nil {
			return err
		}
		enc := &encode.FrameEncoder{
			Plane:          plane,
			QP:             opts.qp,
			DisableDeblock: disableDeblock,
			CABAC:          opts.cabac,
			Width:          frameW,
			Height:         frameH,
			ColorSpace:     cs,
			Range:          rng,
		}
		slice, err := enc.EncodeSlice(uint32(i % 2))
		if err != nil {
			return fmt.Errorf("frame %d: %w", i, err)
		}
		if slice, err = padSlice(slice, opts.bpp, i); err != nil {
			return err
		}
		if _, err := f.Write(slice); err != nil {
			return err
		}
	}
	return nil
}

func generateH264WithPSkip(f io.Writer, opts *options, mbWidth, mbHeight int,
	bg, fg yuv.Color, enc *encode.FrameEncoder, patGrid *yuv.Grid, patColors yuv.ColorMap, tiled bool,
	textScale int, textBg *yuv.Color) error {

	idrPicID := uint32(0)
	frameNum := uint32(0)

	for i := range opts.numFrames {
		if i%opts.idrInterval == 0 {
			plane, err := buildFrameGrid(i, patGrid, patColors,
				mbWidth, mbHeight, textScale, opts.fps, opts.text, bg, fg, textBg, tiled, opts.blockSize, opts.bgPlane)
			if err != nil {
				return err
			}
			enc.Plane = plane
			slice, err := enc.EncodeSlice(idrPicID)
			if err != nil {
				return fmt.Errorf("frame %d (IDR): %w", i, err)
			}
			if slice, err = padSlice(slice, opts.bpp, i); err != nil {
				return err
			}
			if _, err := f.Write(slice); err != nil {
				return err
			}
			idrPicID ^= 1
			frameNum = 1
		} else {
			slice, err := enc.EncodePSkipSlice(frameNum)
			if err != nil {
				return fmt.Errorf("frame %d (P_Skip): %w", i, err)
			}
			if slice, err = padSlice(slice, opts.bpp, i); err != nil {
				return err
			}
			if _, err := f.Write(slice); err != nil {
				return err
			}
			frameNum++
		}
	}
	return nil
}

func generateMP4(opts *options, frameW, frameH, mbWidth, mbHeight, disableDeblock int,
	bg, fg yuv.Color, patGrid *yuv.Grid, patColors yuv.ColorMap, tiled bool,
	textScale int, textBg *yuv.Color, cs yuv.ColorSpace, rng yuv.Range) error {

	f, err := openOutput(opts.output)
	if err != nil {
		return err
	}
	defer f.Close()

	maxRef := 0
	if opts.idrInterval > 0 {
		maxRef = 1
	}

	// Build SPS/PPS as raw NALUs (no start codes) for SetAVCDescriptor
	level := encode.ChooseLevel(frameW, frameH, opts.fps, opts.kbps, opts.cabac)
	var spsRBSP, ppsRBSP []byte
	if opts.cabac {
		spsRBSP = encode.EncodeSPSMain(frameW, frameH, maxRef, level, cs, rng)
		ppsRBSP = encode.EncodePPSCABAC(disableDeblock)
	} else {
		spsRBSP = encode.EncodeSPS(frameW, frameH, maxRef, level, cs, rng)
		ppsRBSP = encode.EncodePPS(disableDeblock)
	}
	spsNALU := encode.BuildNALU(7, 3, spsRBSP)
	ppsNALU := encode.BuildNALU(8, 3, ppsRBSP)

	// Create init segment
	init := mp4.CreateEmptyInit()
	init.AddEmptyTrack(uint32(opts.fps), "video", "und")
	trak := init.Moov.Trak
	if err := trak.SetAVCDescriptor("avc1", [][]byte{spsNALU}, [][]byte{ppsNALU}, true); err != nil {
		return fmt.Errorf("set AVC descriptor: %w", err)
	}
	if err := init.Encode(f); err != nil {
		return fmt.Errorf("write init segment: %w", err)
	}

	// Create encoder for frame generation
	plane, err := buildFrameGrid(0, patGrid, patColors,
		mbWidth, mbHeight, textScale, opts.fps, opts.text, bg, fg, textBg, tiled, opts.blockSize, opts.bgPlane)
	if err != nil {
		return err
	}
	enc := &encode.FrameEncoder{
		Plane:           plane,
		QP:              opts.qp,
		DisableDeblock:  disableDeblock,
		CABAC:           opts.cabac,
		MaxNumRefFrames: maxRef,
		Width:           frameW,
		Height:          frameH,
		ColorSpace:      cs,
		Range:           rng,
		FPS:             opts.fps,
		Kbps:            opts.kbps,
	}

	// Encode all frames and collect as MP4 samples
	type frameSample struct {
		data  []byte
		isIDR bool
	}
	samples := make([]frameSample, 0, opts.numFrames)

	if opts.idrInterval > 0 {
		idrPicID := uint32(0)
		frameNum := uint32(0)
		for i := range opts.numFrames {
			if i%opts.idrInterval == 0 {
				pg, err := buildFrameGrid(i, patGrid, patColors,
					mbWidth, mbHeight, textScale, opts.fps, opts.text, bg, fg,
					textBg, tiled, opts.blockSize, opts.bgPlane)
				if err != nil {
					return err
				}
				enc.Plane = pg
				slice, err := enc.EncodeSlice(idrPicID)
				if err != nil {
					return fmt.Errorf("frame %d (IDR): %w", i, err)
				}
				if slice, err = padSlice(slice, opts.bpp, i); err != nil {
					return err
				}
				samples = append(samples, frameSample{data: slice, isIDR: true})
				idrPicID ^= 1
				frameNum = 1
			} else {
				slice, err := enc.EncodePSkipSlice(frameNum)
				if err != nil {
					return fmt.Errorf("frame %d (P_Skip): %w", i, err)
				}
				if slice, err = padSlice(slice, opts.bpp, i); err != nil {
					return err
				}
				samples = append(samples, frameSample{data: slice, isIDR: false})
				frameNum++
			}
		}
	} else {
		for i := range opts.numFrames {
			pg, err := buildFrameGrid(i, patGrid, patColors,
				mbWidth, mbHeight, textScale, opts.fps, opts.text, bg, fg, textBg, tiled, opts.blockSize, opts.bgPlane)
			if err != nil {
				return err
			}
			enc := &encode.FrameEncoder{
				Plane:          pg,
				QP:             opts.qp,
				DisableDeblock: disableDeblock,
				CABAC:          opts.cabac,
				Width:          frameW,
				Height:         frameH,
				ColorSpace:     cs,
				Range:          rng,
			}
			slice, err := enc.EncodeSlice(uint32(i % 2))
			if err != nil {
				return fmt.Errorf("frame %d: %w", i, err)
			}
			if slice, err = padSlice(slice, opts.bpp, i); err != nil {
				return err
			}
			samples = append(samples, frameSample{data: slice, isIDR: true})
		}
	}

	// Write samples as fragmented MP4
	seqNum := uint32(1)
	for fragStart := 0; fragStart < len(samples); fragStart += opts.fragDur {
		fragEnd := min(fragStart+opts.fragDur, len(samples))

		frag, err := mp4.CreateFragment(seqNum, 1)
		if err != nil {
			return fmt.Errorf("create fragment %d: %w", seqNum, err)
		}

		for i := fragStart; i < fragEnd; i++ {
			sampleData := avc.ConvertByteStreamToNaluSample(samples[i].data)
			flags := mp4.SyncSampleFlags
			if !samples[i].isIDR {
				flags = mp4.NonSyncSampleFlags
			}
			frag.AddFullSample(mp4.FullSample{
				Sample: mp4.Sample{
					Flags:                 flags,
					Dur:                   1,
					Size:                  uint32(len(sampleData)),
					CompositionTimeOffset: 0,
				},
				DecodeTime: uint64(i),
				Data:       sampleData,
			})
		}

		seg := mp4.NewMediaSegment()
		seg.AddFragment(frag)
		if err := seg.Encode(f); err != nil {
			return fmt.Errorf("write segment %d: %w", seqNum, err)
		}
		seqNum++
	}

	entropy := "CAVLC"
	if opts.cabac {
		entropy = "CABAC"
	}
	idrInfo := ""
	if opts.idrInterval > 0 {
		idrInfo = fmt.Sprintf(", IDR every %d frames", opts.idrInterval)
	}
	bppInfo := ""
	if opts.bpp > 0 {
		bppInfo = fmt.Sprintf(", bpp=%d", opts.bpp)
	}
	fmt.Fprintf(os.Stderr, "Wrote %d frames %dx%d (%s, QP=%d, %d fps, frag=%d%s%s) to %s\n",
		opts.numFrames, frameW, frameH, entropy, opts.qp, opts.fps, opts.fragDur, idrInfo, bppInfo, opts.output)
	return nil
}

func generateY4M(opts *options, frameW, frameH, mbWidth, mbHeight int,
	bg, fg yuv.Color, patGrid *yuv.Grid, patColors yuv.ColorMap, tiled bool,
	textScale int, textBg *yuv.Color, cs yuv.ColorSpace, rng yuv.Range) error {

	f, err := openOutput(opts.output)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := yuv.WriteY4MHeaderCS(f, frameW, frameH, cs, rng); err != nil {
		return err
	}

	for i := range opts.numFrames {
		pg, err := buildFrameGrid(i, patGrid, patColors,
			mbWidth, mbHeight, textScale, opts.fps, opts.text, bg, fg, textBg, tiled, opts.blockSize, opts.bgPlane)
		if err != nil {
			return err
		}
		fr := yuv.BuildFrameFromPlaneGrid(pg)
		fr.Width = frameW
		fr.Height = frameH
		if err := yuv.WriteY4MFrame(f, fr); err != nil {
			return fmt.Errorf("frame %d: %w", i, err)
		}
	}

	fmt.Fprintf(os.Stderr, "Wrote %d frames %dx%d to %s\n",
		opts.numFrames, frameW, frameH, opts.output)
	return nil
}

func generateFormattedImages(opts *options, frameW, frameH, mbWidth, mbHeight int,
	bg, fg yuv.Color, patGrid *yuv.Grid, patColors yuv.ColorMap, tiled bool,
	textScale int, textBg *yuv.Color, cs yuv.ColorSpace, rng yuv.Range) error {

	ext := strings.ToLower(filepath.Ext(opts.output))
	isJPEG := ext == ".jpg" || ext == ".jpeg"

	for i := range opts.numFrames {
		pg, err := buildFrameGrid(i, patGrid, patColors,
			mbWidth, mbHeight, textScale, opts.fps, opts.text, bg, fg, textBg, tiled, opts.blockSize, opts.bgPlane)
		if err != nil {
			return err
		}
		fr := yuv.BuildFrameFromPlaneGrid(pg)
		fr.Width = frameW
		fr.Height = frameH
		path := fmt.Sprintf(opts.output, i)
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		if isJPEG {
			err = yuv.WriteJPEGCS(f, fr, opts.jpegQual, cs, rng)
		} else {
			err = yuv.WritePNGCS(f, fr, cs, rng)
		}
		f.Close()
		if err != nil {
			return fmt.Errorf("frame %d: %w", i, err)
		}
	}

	fmt.Fprintf(os.Stderr, "Wrote %d frames %dx%d to %s\n",
		opts.numFrames, frameW, frameH, opts.output)
	return nil
}

func generateYUV(opts *options, frameW, frameH, mbWidth, mbHeight int,
	bg, fg yuv.Color, patGrid *yuv.Grid, patColors yuv.ColorMap, tiled bool,
	textScale int, textBg *yuv.Color) error {

	outPath := yuv.AddSuffix(opts.output, frameW, frameH)
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	for i := range opts.numFrames {
		pg, err := buildFrameGrid(i, patGrid, patColors,
			mbWidth, mbHeight, textScale, opts.fps, opts.text, bg, fg, textBg, tiled, opts.blockSize, opts.bgPlane)
		if err != nil {
			return err
		}
		fr := yuv.BuildFrameFromPlaneGrid(pg)
		fr.Width = frameW
		fr.Height = frameH
		if _, err := f.Write(fr.YUV420Bytes()); err != nil {
			return fmt.Errorf("frame %d: %w", i, err)
		}
	}

	fmt.Fprintf(os.Stderr, "Wrote %d frame(s) %dx%d to %s\n", opts.numFrames, frameW, frameH, outPath)
	return nil
}

func generateNumberedImages(opts *options, frameW, frameH, mbWidth, mbHeight int,
	bg, fg yuv.Color, patGrid *yuv.Grid, patColors yuv.ColorMap, tiled bool,
	textScale int, textBg *yuv.Color, cs yuv.ColorSpace, rng yuv.Range) error {

	ext := strings.ToLower(filepath.Ext(opts.output))
	isJPEG := ext == ".jpg" || ext == ".jpeg"

	for i := range opts.numFrames {
		pg, err := buildFrameGrid(i, patGrid, patColors,
			mbWidth, mbHeight, textScale, opts.fps, opts.text, bg, fg, textBg, tiled, opts.blockSize, opts.bgPlane)
		if err != nil {
			return err
		}
		fr := yuv.BuildFrameFromPlaneGrid(pg)
		fr.Width = frameW
		fr.Height = frameH

		path := opts.output
		if opts.numFrames > 1 {
			stem := strings.TrimSuffix(opts.output, filepath.Ext(opts.output))
			width := max(len(fmt.Sprintf("%d", opts.numFrames-1)), 4)
			path = fmt.Sprintf("%s_%0*d%s", stem, width, i, ext)
		}

		f, err := os.Create(path)
		if err != nil {
			return err
		}
		if isJPEG {
			err = yuv.WriteJPEGCS(f, fr, opts.jpegQual, cs, rng)
		} else {
			err = yuv.WritePNGCS(f, fr, cs, rng)
		}
		f.Close()
		if err != nil {
			return fmt.Errorf("frame %d: %w", i, err)
		}
	}

	fmt.Fprintf(os.Stderr, "Wrote %d frame(s) %dx%d to %s\n", opts.numFrames, frameW, frameH, opts.output)
	return nil
}
