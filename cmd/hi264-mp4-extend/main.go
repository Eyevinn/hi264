// Command hi264-mp4-extend extends a fragmented MP4 (CMAF) media segment
// with empty frames — either all P_Skip frames (a freeze on the source's
// last picture), or a black IDR followed by P_Skip copies. The output is
// a self-contained media segment that plays alongside the input init:
//
//	cat init.mp4 out.m4s | ffplay -i -
//
// Usage:
//
//	hi264-mp4-extend -frames N [-black-idr] <init.mp4> <in.m4s> <out.m4s>
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/Eyevinn/mp4ff/mp4"

	"github.com/Eyevinn/hi264/pkg/encode"
	"github.com/Eyevinn/hi264/pkg/yuv"
)

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("hi264-mp4-extend", flag.ContinueOnError)
	frames := fs.Uint("frames", 0, "number of frames to append (required)")
	blackIDR := fs.Bool("black-idr", false, "start the appended span with a black IDR (default: all P_Skip)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(),
			`hi264-mp4-extend — extend a fragmented MP4 (CMAF) media segment with empty frames.

Reads <init.mp4> for SPS/PPS and frame dimensions, reads <in.m4s> for the
existing samples and timing, and writes <out.m4s> as a single fragment
containing all the input samples followed by N appended frames at the
same per-sample duration. By default the appended frames are P_Skip
copies of the source's last reference picture (a freeze). With
-black-idr the first appended frame is a black IDR (POC reset) and the
rest are P_Skip copies of that IDR.

The output is a self-contained media segment to be played alongside the
init segment, e.g.

    cat init.mp4 out.m4s | ffplay -i -

Constraints: SPS pic_order_cnt_type must be 0 or 2 (type 1 unsupported);
8-bit 4:2:0; progressive only.

Usage:
  hi264-mp4-extend -frames N [-black-idr] <init.mp4> <in.m4s> <out.m4s>

Flags:`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 3 {
		fs.Usage()
		return fmt.Errorf("expected 3 positional arguments, got %d", fs.NArg())
	}
	if *frames == 0 {
		return fmt.Errorf("-frames must be a positive integer")
	}
	return extendSegment(fs.Arg(0), fs.Arg(1), fs.Arg(2), uint32(*frames), *blackIDR)
}

// extendSegment is the core operation. It is exported package-internal so
// tests can drive it without going through the flag parser.
func extendSegment(initPath, inSegPath, outSegPath string, count uint32, blackIDR bool) error {
	initParsed, err := decodeFile(initPath)
	if err != nil {
		return fmt.Errorf("read init segment: %w", err)
	}
	if initParsed.Init == nil {
		return fmt.Errorf("init segment missing (no ftyp+moov)")
	}
	sps, pps, err := extractSPSPPS(initParsed.Init)
	if err != nil {
		return fmt.Errorf("read SPS/PPS from init: %w", err)
	}

	segParsed, err := decodeFile(inSegPath)
	if err != nil {
		return fmt.Errorf("read media segment: %w", err)
	}
	if len(segParsed.Segments) == 0 {
		return fmt.Errorf("input segment has no fragments (no moof+mdat)")
	}

	// Reassemble the input as Annex-B so encode.LastFrameState can read POC.
	// avc.ConvertSampleToByteStream mutates its input in-place, so feed it a
	// copy — otherwise it overwrites the 4-byte NALU length prefixes inside
	// the parsed mdat buffer, corrupting samples we later copy back out.
	var annexB bytes.Buffer
	annexB.Write(annexBParameterSets(initParsed.Init))
	for _, seg := range segParsed.Segments {
		for _, frag := range seg.Fragments {
			samples, err := frag.GetFullSamples(nil)
			if err != nil {
				return fmt.Errorf("read input samples: %w", err)
			}
			for _, s := range samples {
				dataCopy := append([]byte(nil), s.Data...)
				annexB.Write(avc.ConvertSampleToByteStream(dataCopy))
			}
		}
	}

	sampleDur := lastSampleDuration(segParsed)
	if sampleDur == 0 {
		return fmt.Errorf("could not determine sample duration from input segment")
	}

	lastFn, lastLsb, err := encode.LastFrameState(annexB.Bytes())
	if err != nil {
		return fmt.Errorf("inspect input tail: %w", err)
	}

	width := int(sps.Width)
	height := int(sps.Height)
	var inputSamples []mp4.FullSample
	for _, seg := range segParsed.Segments {
		for _, frag := range seg.Fragments {
			samples, err := frag.GetFullSamples(nil)
			if err != nil {
				return fmt.Errorf("read input samples: %w", err)
			}
			inputSamples = append(inputSamples, samples...)
		}
	}

	newSamples := make([]mp4.FullSample, 0, count)
	nextDecodeTime := nextDecodeTimeAfter(segParsed)
	cursorFn := lastFn
	cursorLsb := lastLsb
	remaining := count

	if blackIDR {
		blackY := uint8(16)
		if sps.VUI != nil && sps.VUI.VideoFullRangeFlag {
			blackY = 0
		}
		grid, colors := yuv.SolidGrid(width, height, yuv.Color{Y: blackY, Cb: 128, Cr: 128})
		plane, err := yuv.GridToPlaneGrid(grid, colors)
		if err != nil {
			return fmt.Errorf("build black plane: %w", err)
		}
		idrAnnexB, err := encode.GenerateIDRWithSPSPPS(encode.EncodeParams{
			Width:  width,
			Height: height,
			QP:     26,
		}, sps, pps, plane, 0)
		if err != nil {
			return fmt.Errorf("encode black IDR: %w", err)
		}
		nalu := avc.ConvertByteStreamToNaluSample(idrAnnexB)
		newSamples = append(newSamples, mp4.FullSample{
			Sample: mp4.Sample{
				Flags: mp4.SyncSampleFlags,
				Dur:   sampleDur,
				Size:  uint32(len(nalu)),
			},
			DecodeTime: nextDecodeTime,
			Data:       nalu,
		})
		nextDecodeTime += uint64(sampleDur)
		cursorFn = 0 // IDR resets POC tracking in the decoder
		cursorLsb = 0
		remaining--
	}

	for i := uint32(0); i < remaining; i++ {
		fn := cursorFn + 1 + i
		lsb := cursorLsb + 2 + 2*i
		pSkipAnnexB, err := encode.EncodePSkipSlice(sps, pps, fn, lsb, 0)
		if err != nil {
			return fmt.Errorf("encode P_Skip %d: %w", i, err)
		}
		nalu := avc.ConvertByteStreamToNaluSample(pSkipAnnexB)
		newSamples = append(newSamples, mp4.FullSample{
			Sample: mp4.Sample{
				Flags: mp4.NonSyncSampleFlags,
				Dur:   sampleDur,
				Size:  uint32(len(nalu)),
			},
			DecodeTime: nextDecodeTime,
			Data:       nalu,
		})
		nextDecodeTime += uint64(sampleDur)
	}

	// Write a single fragment containing all original samples followed by
	// the appended ones.
	out, err := os.Create(outSegPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outSegPath, err)
	}
	seqNum := firstSeqNum(segParsed)
	frag, err := mp4.CreateFragment(seqNum, defaultTrackID(initParsed.Init))
	if err != nil {
		out.Close()
		return fmt.Errorf("create fragment: %w", err)
	}
	for _, s := range inputSamples {
		frag.AddFullSample(s)
	}
	for _, s := range newSamples {
		frag.AddFullSample(s)
	}
	newSeg := mp4.NewMediaSegment()
	newSeg.AddFragment(frag)
	if err := newSeg.Encode(out); err != nil {
		out.Close()
		return fmt.Errorf("write %s: %w", outSegPath, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", outSegPath, err)
	}

	mode := "P_Skip"
	if blackIDR {
		mode = "black IDR + P_Skip"
	}
	fmt.Printf("appended %d sample(s) (%s, dur=%d each) → %s\n",
		len(newSamples), mode, sampleDur, outSegPath)
	return nil
}

// decodeFile reads an MP4 file fully into memory and parses it. Reading
// fully avoids any risk of mp4ff's lazy mdat loading hitting a closed file
// later when callers walk the parsed structure.
func decodeFile(path string) (*mp4.File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return mp4.DecodeFile(bytes.NewReader(data))
}

func extractSPSPPS(init *mp4.InitSegment) (*avc.SPS, *avc.PPS, error) {
	if init == nil || init.Moov == nil {
		return nil, nil, fmt.Errorf("no moov in init segment")
	}
	for _, trak := range init.Moov.Traks {
		stsd := trak.Mdia.Minf.Stbl.Stsd
		for _, child := range stsd.Children {
			e, ok := child.(*mp4.VisualSampleEntryBox)
			if !ok || e.AvcC == nil {
				continue
			}
			if len(e.AvcC.SPSnalus) == 0 || len(e.AvcC.PPSnalus) == 0 {
				return nil, nil, fmt.Errorf("avcC has no SPS/PPS")
			}
			sps, err := avc.ParseSPSNALUnit(e.AvcC.SPSnalus[0], true)
			if err != nil {
				return nil, nil, fmt.Errorf("parse SPS: %w", err)
			}
			spsMap := map[uint32]*avc.SPS{uint32(sps.ParameterID): sps}
			pps, err := avc.ParsePPSNALUnit(e.AvcC.PPSnalus[0], spsMap)
			if err != nil {
				return sps, nil, fmt.Errorf("parse PPS: %w", err)
			}
			return sps, pps, nil
		}
	}
	return nil, nil, fmt.Errorf("no AVC video track found")
}

// annexBParameterSets writes SPS and PPS NALUs from the init segment into
// an Annex-B bytestream so encode.LastFrameState can resolve slice headers.
func annexBParameterSets(init *mp4.InitSegment) []byte {
	var out bytes.Buffer
	for _, trak := range init.Moov.Traks {
		stsd := trak.Mdia.Minf.Stbl.Stsd
		for _, child := range stsd.Children {
			e, ok := child.(*mp4.VisualSampleEntryBox)
			if !ok || e.AvcC == nil {
				continue
			}
			for _, n := range e.AvcC.SPSnalus {
				out.Write([]byte{0, 0, 0, 1})
				out.Write(n)
			}
			for _, n := range e.AvcC.PPSnalus {
				out.Write([]byte{0, 0, 0, 1})
				out.Write(n)
			}
		}
	}
	return out.Bytes()
}

func lastSampleDuration(file *mp4.File) uint32 {
	var dur uint32
	for _, seg := range file.Segments {
		for _, frag := range seg.Fragments {
			samples, err := frag.GetFullSamples(nil)
			if err != nil {
				continue
			}
			for _, s := range samples {
				if s.Dur != 0 {
					dur = s.Dur
				}
			}
		}
	}
	return dur
}

func nextDecodeTimeAfter(file *mp4.File) uint64 {
	var t uint64
	for _, seg := range file.Segments {
		for _, frag := range seg.Fragments {
			samples, err := frag.GetFullSamples(nil)
			if err != nil || len(samples) == 0 {
				continue
			}
			last := samples[len(samples)-1]
			t = last.DecodeTime + uint64(last.Dur)
		}
	}
	return t
}

// firstSeqNum returns the sequence_number of the first fragment in the
// input, so the output keeps the same identity as the input segment.
func firstSeqNum(file *mp4.File) uint32 {
	for _, seg := range file.Segments {
		for _, frag := range seg.Fragments {
			if frag.Moof != nil && frag.Moof.Mfhd != nil {
				return frag.Moof.Mfhd.SequenceNumber
			}
		}
	}
	return 1
}

func defaultTrackID(init *mp4.InitSegment) uint32 {
	if init == nil || init.Moov == nil {
		return 1
	}
	for _, t := range init.Moov.Traks {
		return t.Tkhd.TrackID
	}
	return 1
}
