package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/Eyevinn/hi264/internal"
	"github.com/Eyevinn/hi264/pkg/decoder"
	"github.com/Eyevinn/hi264/pkg/frame"
	"github.com/Eyevinn/hi264/pkg/yuv"
)

const appName = "hi264dec"

var usg = `%s - decode H.264 IDR frames from raw Annex-B .264 files or MP4 containers.

Usage:

  %s [options] <input> [output]

Input auto-detection:
  .mp4, .m4v  → MP4 container (progressive + fragmented)
  everything  → Annex-B raw byte stream

Output format (from extension):
  .png        → PNG image (numbered if -n > 1)
  .jpg/.jpeg  → JPEG image (numbered if -n > 1, -q for quality)
  .y4m        → Y4M (multi-frame: single file)
  .yuv        → Raw YUV420 (auto-adds _WxH_yuv420p suffix, multi-frame: concatenated)
  (none)      → decode only, print info

Options:
`

type options struct {
	version    bool
	noDeblock  bool
	idrAndSkip bool
	n          int
	jpegQual   int
	colorspace string
}

func parseOptions(fs *flag.FlagSet, args []string) (*options, error) {
	var opts options
	fs.BoolVar(&opts.version, "version", false, "Get hi264 version")
	fs.BoolVar(&opts.noDeblock, "no-deblock", false, "skip deblocking filter")
	fs.BoolVar(&opts.idrAndSkip, "idr-and-skip", false, "decode P_Skip frames in addition to IDR")
	fs.IntVar(&opts.n, "n", 1, "max frames to decode")
	fs.IntVar(&opts.jpegQual, "q", 85, "JPEG quality (1-100)")
	fs.StringVar(&opts.colorspace, "colorspace", "", "override color space (bt601, bt709, bt2020)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, usg, appName, appName)
		fs.PrintDefaults()
	}
	err := fs.Parse(args[1:])
	return &opts, err
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

	if opts.version {
		fmt.Printf("%s %s\n", appName, internal.GetVersion())
		return nil
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("input file is required")
	}

	inputFile := fs.Arg(0)
	outputFile := fs.Arg(1) // "" if not provided

	dec := decoder.New()
	dec.TraceMBCMP = os.Getenv("TRACE_MBCMP") != ""
	dec.SkipDeblock = opts.noDeblock || os.Getenv("SKIP_DEBLOCK") != ""

	var frames []*frame.Frame
	if isMP4(inputFile) {
		frames, err = decodeMP4(inputFile, dec, opts.n, opts.idrAndSkip)
	} else {
		frames, err = decodeAnnexB(inputFile, dec, opts.n, opts.idrAndSkip)
	}
	if err != nil {
		return err
	}

	fmt.Printf("Decoded %d frame(s)\n", len(frames))
	if len(frames) > 0 {
		fmt.Printf("Frame size: %dx%d\n", frames[0].Width, frames[0].Height)
	}

	if outputFile != "" {
		// Determine color space: CLI override > frame VUI > default BT.601
		cs := yuv.BT601
		var rng yuv.Range
		if opts.colorspace != "" {
			var cerr error
			cs, cerr = yuv.ParseColorSpace(opts.colorspace)
			if cerr != nil {
				return cerr
			}
		} else if len(frames) > 0 && frames[0].ColorDescriptionValid {
			cs = yuv.ColorSpaceFromMatrixCoefficients(frames[0].MatrixCoefficients)
		}
		if len(frames) > 0 && frames[0].VideoFullRangeFlag {
			rng = yuv.FullRange
		}
		return writeFrames(frames, outputFile, opts, cs, rng)
	}
	return nil
}

func isMP4(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".mp4" || ext == ".m4v"
}

func decodeAnnexB(inputFile string, dec *decoder.Decoder, n int, idrAndSkip bool) ([]*frame.Frame, error) {
	data, err := os.ReadFile(inputFile)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}

	nalus := avc.ExtractNalusFromByteStream(data)
	fmt.Printf("Found %d NALUs\n", len(nalus))
	printNALUInfo(nalus)

	if n == 1 && !idrAndSkip {
		f, err := dec.DecodeAnnexB(data)
		if err != nil {
			return nil, fmt.Errorf("decode: %w", err)
		}
		return []*frame.Frame{f}, nil
	}

	var frames []*frame.Frame
	if idrAndSkip {
		frames, err = dec.DecodeAllAnnexB(data)
	} else {
		frames, err = dec.DecodeIDRAnnexB(data)
	}
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(frames) > n {
		frames = frames[:n]
	}
	return frames, nil
}

func decodeMP4(inputFile string, dec *decoder.Decoder, n int, idrAndSkip bool) ([]*frame.Frame, error) {
	f, err := os.Open(inputFile)
	if err != nil {
		return nil, fmt.Errorf("open input: %w", err)
	}
	defer f.Close()

	mp4File, err := mp4.DecodeFile(f, mp4.WithDecodeMode(mp4.DecModeLazyMdat))
	if err != nil {
		return nil, fmt.Errorf("decode mp4: %w", err)
	}

	if mp4File.Moov == nil {
		return nil, fmt.Errorf("no moov box found")
	}

	// Find video track
	var videoTrack *mp4.TrakBox
	for _, trak := range mp4File.Moov.Traks {
		if trak.Mdia != nil && trak.Mdia.Hdlr != nil && trak.Mdia.Hdlr.HandlerType == "vide" {
			videoTrack = trak
			break
		}
	}
	if videoTrack == nil {
		return nil, fmt.Errorf("no video track found")
	}

	// Extract SPS/PPS from AvcC
	stbl := videoTrack.Mdia.Minf.Stbl
	if stbl.Stsd.AvcX == nil {
		return nil, fmt.Errorf("no AVC sample entry found (not an H.264 track)")
	}
	avcC := stbl.Stsd.AvcX.AvcC
	if avcC == nil {
		return nil, fmt.Errorf("no avcC box found")
	}

	spsNALUs := avcC.SPSnalus
	ppsNALUs := avcC.PPSnalus
	fmt.Printf("Found %d SPS, %d PPS in avcC\n", len(spsNALUs), len(ppsNALUs))

	// Try progressive MP4 first
	nrSamples := videoTrack.GetNrSamples()
	if nrSamples > 0 {
		return extractProgressive(mp4File, videoTrack, f, spsNALUs, ppsNALUs, dec, n, idrAndSkip)
	}

	// Fall back to fragmented MP4
	if len(mp4File.Segments) > 0 {
		return extractFragmented(mp4File, f, spsNALUs, ppsNALUs, dec, n, videoTrack, idrAndSkip)
	}

	return nil, fmt.Errorf("no samples found (neither progressive nor fragmented)")
}

func extractProgressive(mp4File *mp4.File, videoTrack *mp4.TrakBox, f *os.File,
	spsNALUs, ppsNALUs [][]byte, dec *decoder.Decoder, n int, idrAndSkip bool) ([]*frame.Frame, error) {

	nrSamples := videoTrack.GetNrSamples()
	stss := videoTrack.Mdia.Minf.Stbl.Stss
	fmt.Printf("Track has %d samples\n", nrSamples)

	var frames []*frame.Frame
	for sampleNr := uint32(1); sampleNr <= nrSamples && len(frames) < n; sampleNr++ {
		if !idrAndSkip && stss != nil && !stss.IsSyncSample(sampleNr) {
			continue
		}

		ranges, err := videoTrack.GetRangesForSampleInterval(sampleNr, sampleNr)
		if err != nil {
			return nil, fmt.Errorf("get range for sample %d: %w", sampleNr, err)
		}

		var sampleData []byte
		for _, dr := range ranges {
			data, err := mp4File.Mdat.ReadData(int64(dr.Offset), int64(dr.Size), f)
			if err != nil {
				return nil, fmt.Errorf("read mdat for sample %d: %w", sampleNr, err)
			}
			sampleData = append(sampleData, data...)
		}

		fr, err := decodeSample(sampleData, spsNALUs, ppsNALUs, dec, idrAndSkip)
		if err != nil {
			return nil, fmt.Errorf("decode sample %d: %w", sampleNr, err)
		}
		frames = append(frames, fr)
	}

	if len(frames) == 0 {
		return nil, fmt.Errorf("no IDR frames found")
	}
	return frames, nil
}

func extractFragmented(mp4File *mp4.File, f *os.File,
	spsNALUs, ppsNALUs [][]byte, dec *decoder.Decoder, n int,
	videoTrack *mp4.TrakBox, idrAndSkip bool) ([]*frame.Frame, error) {

	trackID := videoTrack.Tkhd.TrackID
	fmt.Printf("Fragmented MP4: %d segments, trackID=%d\n", len(mp4File.Segments), trackID)

	var trex *mp4.TrexBox
	if mp4File.Moov.Mvex != nil {
		trex, _ = mp4File.Moov.Mvex.GetTrex(trackID)
	}

	var frames []*frame.Frame
	sampleNr := uint32(0)
	for _, seg := range mp4File.Segments {
		for _, frag := range seg.Fragments {
			for _, traf := range frag.Moof.Trafs {
				if traf.Tfhd.TrackID != trackID {
					continue
				}
				for _, trun := range traf.Truns {
					trun.AddSampleDefaultValues(traf.Tfhd, trex)

					baseOffset := frag.Moof.StartPos
					if traf.Tfhd.HasBaseDataOffset() {
						baseOffset = traf.Tfhd.BaseDataOffset
					}
					if trun.HasDataOffset() {
						baseOffset = uint64(int64(baseOffset) + int64(trun.DataOffset))
					}

					sampleOffset := uint64(0)
					for i := uint32(0); i < trun.SampleCount(); i++ {
						sampleNr++
						sample := trun.Samples[i]

						if idrAndSkip || sample.IsSync() {
							data := make([]byte, sample.Size)
							_, err := f.ReadAt(data, int64(baseOffset+sampleOffset))
							if err != nil {
								return nil, fmt.Errorf("read fragment sample %d: %w", sampleNr, err)
							}

							fr, err := decodeSample(data, spsNALUs, ppsNALUs, dec, idrAndSkip)
							if err != nil {
								return nil, fmt.Errorf("decode sample %d: %w", sampleNr, err)
							}
							frames = append(frames, fr)
							if len(frames) >= n {
								return frames, nil
							}
						}
						sampleOffset += uint64(sample.Size)
					}
				}
			}
		}
	}

	if len(frames) == 0 {
		return nil, fmt.Errorf("no IDR frames found")
	}
	return frames, nil
}

func decodeSample(sampleData []byte, spsNALUs, ppsNALUs [][]byte,
	dec *decoder.Decoder, idrAndSkip bool) (*frame.Frame, error) {
	sampleNALUs, err := avc.GetNalusFromSample(sampleData)
	if err != nil {
		return nil, fmt.Errorf("get nalus from sample: %w", err)
	}

	var nalus [][]byte
	nalus = append(nalus, spsNALUs...)
	nalus = append(nalus, ppsNALUs...)
	nalus = append(nalus, sampleNALUs...)

	if idrAndSkip {
		frames, err := dec.DecodeAllFrames(nalus)
		if err != nil {
			return nil, err
		}
		if len(frames) == 0 {
			return nil, fmt.Errorf("no decodable frames in sample")
		}
		return frames[0], nil
	}
	return dec.DecodeNALUs(nalus)
}

func writeFrames(frames []*frame.Frame, outputFile string, opts *options, cs yuv.ColorSpace, rng yuv.Range) error {
	ext := strings.ToLower(filepath.Ext(outputFile))

	switch ext {
	case ".y4m":
		return writeY4M(frames, outputFile, cs, rng)
	case ".yuv":
		return writeYUV(frames, outputFile)
	case ".png":
		return writeImages(frames, outputFile, opts, cs, rng)
	case ".jpg", ".jpeg":
		return writeImages(frames, outputFile, opts, cs, rng)
	default:
		return fmt.Errorf("unsupported output format: %s", ext)
	}
}

func writeY4M(frames []*frame.Frame, outputFile string, cs yuv.ColorSpace, rng yuv.Range) error {
	of, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer of.Close()

	if err := yuv.WriteY4MHeaderCS(of, frames[0].Width, frames[0].Height, cs, rng); err != nil {
		return fmt.Errorf("write Y4M header: %w", err)
	}
	for _, f := range frames {
		if err := yuv.WriteY4MFrame(of, f); err != nil {
			return fmt.Errorf("write Y4M frame: %w", err)
		}
	}
	fmt.Printf("Wrote %d frame(s) to %s\n", len(frames), outputFile)
	return nil
}

func writeYUV(frames []*frame.Frame, outputFile string) error {
	outPath := yuv.AddSuffix(outputFile, frames[0].Width, frames[0].Height)
	of, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer of.Close()

	for _, f := range frames {
		if _, err := of.Write(f.YUV420Bytes()); err != nil {
			return fmt.Errorf("write YUV frame: %w", err)
		}
	}
	fmt.Printf("Wrote %d frame(s) to %s\n", len(frames), outPath)
	return nil
}

func writeImages(frames []*frame.Frame, outputFile string, opts *options, cs yuv.ColorSpace, rng yuv.Range) error {
	ext := strings.ToLower(filepath.Ext(outputFile))
	isJPEG := ext == ".jpg" || ext == ".jpeg"

	for i, f := range frames {
		outPath := outputFile
		if len(frames) > 1 {
			outPath = numberedPath(outputFile, i)
		}

		of, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("create %s: %w", outPath, err)
		}

		if isJPEG {
			err = yuv.WriteJPEGCS(of, f, opts.jpegQual, cs, rng)
		} else {
			err = yuv.WritePNGCS(of, f, cs, rng)
		}
		of.Close()
		if err != nil {
			return fmt.Errorf("write %s: %w", outPath, err)
		}
		fmt.Printf("Wrote frame %d to %s\n", i, outPath)
	}
	return nil
}

func numberedPath(base string, index int) string {
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return fmt.Sprintf("%s_%04d%s", stem, index, ext)
}

func printNALUInfo(nalus [][]byte) {
	spsMap := make(map[uint32]*avc.SPS)
	ppsMap := make(map[uint32]*avc.PPS)

	for i, nalu := range nalus {
		naluType := avc.NaluType(nalu[0] & 0x1f)
		fmt.Printf("  NALU %d: type=%d (%s), size=%d bytes\n", i, naluType, naluTypeName(naluType), len(nalu))
	}

	for _, nalu := range nalus {
		naluType := avc.NaluType(nalu[0] & 0x1f)
		switch naluType {
		case avc.NALU_SPS:
			sps, _ := avc.ParseSPSNALUnit(nalu, true)
			if sps != nil {
				spsMap[sps.ParameterID] = sps
				fmt.Printf("  SPS: %dx%d, profile=%d, level=%d, chromaFmt=%d, bitDepthY=%d, transform8x8=%v\n",
					sps.Width, sps.Height, sps.Profile, sps.Level, sps.ChromaFormatIDC,
					8+sps.BitDepthLumaMinus8, sps.SeqScalingMatrixPresentFlag)
			}
		case avc.NALU_PPS:
			pps, _ := avc.ParsePPSNALUnit(nalu, spsMap)
			if pps != nil {
				ppsMap[pps.PicParameterSetID] = pps
				fmt.Printf("  PPS: entropy=%v, transform8x8=%v, picInitQpMinus26=%d, chromaQpOffset=%d\n",
					pps.EntropyCodingModeFlag, pps.Transform8x8ModeFlag,
					pps.PicInitQpMinus26, pps.ChromaQpIndexOffset)
			}
		case avc.NALU_IDR:
			sh, _ := avc.ParseSliceHeader(nalu, spsMap, ppsMap)
			if sh != nil {
				fmt.Printf("  SliceHeader: size=%d, type=%d, qpDelta=%d, cabacInitIDC=%d\n",
					sh.Size, sh.SliceType, sh.SliceQPDelta, sh.CabacInitIDC)
				pps := ppsMap[sh.PicParamID]
				sliceQPY := 26 + int(pps.PicInitQpMinus26) + int(sh.SliceQPDelta)
				fmt.Printf("  SliceQPY = 26 + %d + %d = %d\n", pps.PicInitQpMinus26, sh.SliceQPDelta, sliceQPY)
				end := min(sh.Size+10, uint32(len(nalu)))
				fmt.Printf("  SliceData starts at byte %d: %s\n", sh.Size, hex.EncodeToString(nalu[sh.Size:end]))
			}
		}
	}
}

func naluTypeName(t avc.NaluType) string {
	switch t {
	case avc.NALU_SPS:
		return "SPS"
	case avc.NALU_PPS:
		return "PPS"
	case avc.NALU_IDR:
		return "IDR"
	case avc.NALU_SEI:
		return "SEI"
	case avc.NALU_AUD:
		return "AUD"
	default:
		return fmt.Sprintf("type_%d", t)
	}
}
