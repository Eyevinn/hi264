// extend_pskip is a verification helper for tools/verify_pskip_extend.sh.
// It reads an Annex-B bitstream, finds the SPS/PPS and the last slice's
// (frame_num, pic_order_cnt_lsb), and appends N P_Skip frames continuing
// the POC progression with stride 2.
//
// Usage:
//
//	go run ./tools/extend_pskip <input.264> <output.264> <count>
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/Eyevinn/hi264/pkg/encode"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: extend_pskip <input.264> <output.264> <count>")
		os.Exit(2)
	}
	inPath, outPath := os.Args[1], os.Args[2]
	count64, err := strconv.ParseUint(os.Args[3], 10, 32)
	if err != nil || count64 == 0 {
		fmt.Fprintln(os.Stderr, "count must be a positive integer")
		os.Exit(2)
	}

	data, err := os.ReadFile(inPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	out, err := encode.AppendPSkipFrames(data, uint32(count64))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := os.WriteFile(outPath, out, 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d source bytes + %d P_Skip frames = %d bytes)\n",
		outPath, len(data), count64, len(out))
}
