// extend_pskip is a verification helper for tools/verify_pskip_extend.sh.
// It reads an Annex-B bitstream and appends N P_Skip frames with
// encode.AppendPSkipFrames.
//
// Usage:
//
//	go run ./tools/extend_pskip [flags] <input.264> <output.264> <count>
//
// Flags:
//
//	-cut-at-non-ref  truncate the input after its last non-reference picture
//	                 before extending, which is what a live stream cut
//	                 mid-reorder looks like
//	-verify          check the appended slice headers against the rules the
//	                 continuation must satisfy (see verifyAppended)
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/Eyevinn/mp4ff/avc"

	"github.com/Eyevinn/hi264/pkg/encode"
)

// sliceInfo is what the checks need from one coded slice.
type sliceInfo struct {
	isIDR    bool
	isRef    bool
	frameNum uint32
	pocLsb   uint32
}

func main() {
	if err := run(os.Args, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("extend_pskip", flag.ContinueOnError)
	cutAtNonRef := fs.Bool("cut-at-non-ref", false,
		"truncate the input after its last non-reference picture before extending")
	verify := fs.Bool("verify", false,
		"check the appended slice headers continue the source correctly")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: extend_pskip [flags] <input.264> <output.264> <count>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 3 {
		fs.Usage()
		return fmt.Errorf("expected 3 positional arguments, got %d", fs.NArg())
	}
	inPath, outPath := fs.Arg(0), fs.Arg(1)
	count64, err := strconv.ParseUint(fs.Arg(2), 10, 32)
	if err != nil || count64 == 0 {
		return fmt.Errorf("count must be a positive integer")
	}
	count := uint32(count64)

	data, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}

	if *cutAtNonRef {
		data, err = cutAfterLastNonReference(data)
		if err != nil {
			return err
		}
	}

	out, err := encode.AppendPSkipFrames(data, count)
	if err != nil {
		return err
	}

	if *verify {
		if err := verifyAppended(stdout, data, out, count); err != nil {
			return err
		}
	}

	// The slice count lets the caller assert the decoded frame count without having to know how
	// much of the input survived a cut.
	if slices, _, err := parseSlices(out); err == nil {
		fmt.Fprintf(stdout, "extended_slices=%d\n", len(slices))
	}

	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s (%d source bytes + %d P_Skip frames = %d bytes)\n",
		outPath, len(data), count, len(out))
	return nil
}

// parseSlices returns one entry per coded slice in an Annex-B stream, in decode order.
func parseSlices(annexB []byte) ([]sliceInfo, *avc.SPS, error) {
	spsMap := make(map[uint32]*avc.SPS)
	ppsMap := make(map[uint32]*avc.PPS)
	var sps *avc.SPS
	var out []sliceInfo
	for _, nalu := range avc.ExtractNalusFromByteStream(annexB) {
		if len(nalu) == 0 {
			continue
		}
		switch avc.GetNaluType(nalu[0]) {
		case avc.NALU_SPS:
			s, err := avc.ParseSPSNALUnit(nalu, true)
			if err != nil {
				return nil, nil, fmt.Errorf("parse SPS: %w", err)
			}
			spsMap[s.ParameterID] = s
			sps = s
		case avc.NALU_PPS:
			p, err := avc.ParsePPSNALUnit(nalu, spsMap)
			if err != nil {
				return nil, nil, fmt.Errorf("parse PPS: %w", err)
			}
			ppsMap[p.PicParameterSetID] = p
		case avc.NALU_IDR, avc.NALU_NON_IDR:
			sh, err := avc.ParseSliceHeader(nalu, spsMap, ppsMap)
			if err != nil {
				return nil, nil, fmt.Errorf("parse slice header: %w", err)
			}
			out = append(out, sliceInfo{
				isIDR:    avc.GetNaluType(nalu[0]) == avc.NALU_IDR,
				isRef:    nalu[0]&0x60 != 0,
				frameNum: sh.FrameNum,
				pocLsb:   sh.PicOrderCntLsb,
			})
		}
	}
	if sps == nil {
		return nil, nil, fmt.Errorf("no SPS in stream")
	}
	return out, sps, nil
}

// cutAfterLastNonReference truncates the stream after its last non-reference picture, so that the
// tail is a picture the continuation must not count. A complete stream from an encoder ends on a
// reference picture, so this is the shape only an interrupted stream has.
func cutAfterLastNonReference(annexB []byte) ([]byte, error) {
	slices, _, err := parseSlices(annexB)
	if err != nil {
		return nil, err
	}
	keep := -1
	for i, s := range slices {
		if !s.isRef {
			keep = i
		}
	}
	if keep < 1 {
		return nil, fmt.Errorf("stream has no non-reference picture to cut after " +
			"(encode it with B-frames, e.g. x264 --bframes 2 --b-pyramid none)")
	}

	var out []byte
	seen := 0
	for _, nalu := range avc.ExtractNalusFromByteStream(annexB) {
		if len(nalu) == 0 {
			continue
		}
		out = append(out, 0, 0, 0, 1)
		out = append(out, nalu...)
		switch avc.GetNaluType(nalu[0]) {
		case avc.NALU_IDR, avc.NALU_NON_IDR:
			if seen == keep {
				return out, nil
			}
			seen++
		}
	}
	return out, nil
}

// verifyAppended checks the appended slices against the two rules a continuation must satisfy,
// derived here straight from the source stream rather than from the code under test:
//
//   - frame_num identifies reference pictures, so the first appended picture continues the last
//     reference picture of the source, not its last coded slice.
//   - the picture order count must exceed every count already in the source, or an appended frame
//     lands before the source's final picture in output order, or duplicates one outright.
func verifyAppended(stdout io.Writer, source, extended []byte, count uint32) error {
	srcSlices, sps, err := parseSlices(source)
	if err != nil {
		return fmt.Errorf("verify: source: %w", err)
	}
	extSlices, _, err := parseSlices(extended)
	if err != nil {
		return fmt.Errorf("verify: extended: %w", err)
	}
	if len(extSlices) != len(srcSlices)+int(count) {
		return fmt.Errorf("verify: extended stream has %d slices, want %d",
			len(extSlices), len(srcSlices)+int(count))
	}
	maxFrameNum := uint32(1) << (sps.Log2MaxFrameNumMinus4 + 4)
	maxPOCLsb := uint32(1) << (sps.Log2MaxPicOrderCntLsbMinus4 + 4)

	// state of the source at the cut
	lastRefFrameNum := uint32(0)
	maxPoc := uint32(0)
	for _, s := range srcSlices {
		if s.isIDR {
			lastRefFrameNum, maxPoc = s.frameNum, s.pocLsb
			continue
		}
		if s.isRef {
			lastRefFrameNum = s.frameNum
		}
		if pocLater(s.pocLsb, maxPoc, maxPOCLsb) {
			maxPoc = s.pocLsb
		}
	}

	wantFrameNum := (lastRefFrameNum + 1) % maxFrameNum
	wantPoc := (maxPoc + 2) % maxPOCLsb
	for i, s := range extSlices[len(srcSlices):] {
		if s.frameNum != wantFrameNum {
			return fmt.Errorf("verify: appended frame %d has frame_num %d, want %d "+
				"(last reference picture in the source is %d)",
				i, s.frameNum, wantFrameNum, lastRefFrameNum)
		}
		if s.pocLsb != wantPoc {
			return fmt.Errorf("verify: appended frame %d has pic_order_cnt_lsb %d, want %d "+
				"(highest in the source is %d)", i, s.pocLsb, wantPoc, maxPoc)
		}
		wantFrameNum = (wantFrameNum + 1) % maxFrameNum
		wantPoc = (wantPoc + 2) % maxPOCLsb
	}
	fmt.Fprintf(stdout, "verified %d appended frames: frame_num continues %d, "+
		"pic_order_cnt_lsb continues %d\n", count, lastRefFrameNum, maxPoc)
	return nil
}

// pocLater reports whether a comes after b modulo max.
func pocLater(a, b, max uint32) bool {
	half := int64(max / 2)
	d := int64(a) - int64(b)
	switch {
	case d > half:
		d -= int64(max)
	case d < -half:
		d += int64(max)
	}
	return d > 0
}
