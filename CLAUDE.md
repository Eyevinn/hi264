# H.264/AVC Frame Decoder & Bitstream Generator in Pure Go

## Project Status

Pure Go H.264/AVC decoder for IDR and P_Skip frames with CABAC and CAVLC entropy
coding, plus a bitstream generator that produces valid H.264 test content from
grid patterns (I_16x16 DC prediction). Supports 16x16 macroblock and 8x8 block
granularity via PlaneGrid (direct Y/Cb/Cr planes, no character indirection).
Not a general-purpose encoder. Supports both CAVLC (Baseline) and CABAC (Main
profile), with P_Skip frame generation for efficient multi-frame sequences.
All processing is 8-bit 4:2:0 only (no 10-bit or 4:2:2/4:4:4 support).
Pixel-perfect match with FFmpeg across 41 golden decoder test cases and 12+
encoder verification tests.

See README.md for CLI usage, architecture, dependencies, and library examples.

### Key Reference Files
- FFmpeg: `external/ffmpeg/libavcodec/h264_cabac.c`, `h264_cavlc.c`
- Standard: `references/ISO_IEC_DIS_14496-10_Ed11.pdf`

### Golden test regeneration

Golden tests verify byte-exact match with FFmpeg. To regenerate:

```bash
# Run all verification tests (requires ffmpeg with libx264)
bash tools/gen_and_verify.sh

# Regenerate golden bitstreams and print updated checksums
bash tools/update_golden.sh
# Then paste the printed Go map into pkg/decoder/decoder_test.go
```

Never add a golden bitstream unless hi264dec output is identical to FFmpeg decode.

### Encoder verification

```bash
# Verify hi264gen grid-only output matches FFmpeg decode across all test patterns
bash tools/verify_hi264gen.sh

# Verify EncodePSkipSliceAt/LastFrameState extends a stream cleanly
# (ffmpeg reports no errors, frame count matches, POC stays monotonic)
bash tools/verify_pskip_extend.sh
```

### Debugging

```bash
# Decode without deblocking (isolates per-MB errors)
go run ./cmd/hi264dec -no-deblock input.264 output.yuv

# Compare two raw YUV files (overall, per-component, per-MB PSNR)
go run ./cmd/rawpsnr -w 320 -h 240 a.yuv b.yuv
go run ./cmd/rawpsnr -w 320 -h 240 -per-mb a.yuv b.yuv
go run ./cmd/rawpsnr -w 320 -h 240 -csv mb.csv a.yuv b.yuv

# Extend a fragmented MP4 (CMAF) segment with empty frames
# (P_Skip freeze; with -black-idr a black IDR + P_Skip tail)
go run ./cmd/hi264-mp4-extend -frames 25 init.mp4 seg1s.m4s seg2s.m4s
go run ./cmd/hi264-mp4-extend -frames 25 -black-idr init.mp4 seg1s.m4s seg2s.m4s
cat init.mp4 seg2s.m4s | ffplay -i -

# Decode multiple frames
go run ./cmd/hi264dec -n 10 input.264 output.y4m

# Emit MBCMP comparison lines for FFmpeg cross-check
TRACE_MBCMP=1 go run ./cmd/hi264dec input.264 output.yuv
```

