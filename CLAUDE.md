# H.264/AVC Frame Decoder & Bitstream Generator in Pure Go

## Conventions

- Never add "Co-Authored-By: Claude" to commit messages or pull request messages

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

### Implemented Features

#### Decoder
- CABAC arithmetic engine + context model initialization
- CAVLC (Exp-Golomb + VLC tables) entropy decoding
- Macroblock layer parsing (I_4x4, I_8x8, I_16x16, P_Skip)
- Inverse quantization and transform (4x4, 8x8, DC Hadamard)
- Custom scaling matrices (SPS/PPS with Table 7-2 fall-back)
- Intra prediction (all 4x4, 8x8, 16x16, and chroma modes)
- Frame reconstruction + deblocking filter
- P_Skip frame decoding (copy from reference, CAVLC and CABAC)
- Multi-frame decoding via `DecodeAllFrames` (IDR + P_Skip sequences)
- Y4M, PNG, and JPEG output support

#### Encoder (hi264gen)
- CAVLC I_16x16 IDR frame encoder with DC prediction (Baseline profile)
- CABAC I_16x16 IDR frame encoder with DC prediction (Main profile)
- CABAC arithmetic encoder engine (inverse of decoder)
- CABAC syntax element encoding (mb_type, chroma pred, qp_delta, residual)
- Per-sub-block chroma DC prediction (matching decoder's 4x4 sub-block logic)
- Forward Hadamard transform + quantization (QP 0-51)
- SPS/PPS generation (Baseline and Main profiles, configurable max_num_ref_frames)
- PlaneGrid: direct Y/Cb/Cr value planes with 16x16 or 8x8 block granularity
- PNG/JPEG image backgrounds (downsampled to block-resolution PlaneGrid)
- Grid-based pattern input with RGB or YCbCr color specification
- 8x8 block support: per-4x4-block AC residual encoding at quadrant boundaries
- Forward 4x4 integer DCT (inverse of decoder's InverseTransform4x4)
- FFmpeg-verified output across diverse test patterns (CAVLC and CABAC)
- P_Skip slice encoder (CAVLC and CABAC, all-skip MBs copying from reference)
- P-slice header writer (non-IDR, with ref list and marking syntax, CABAC alignment)
- Efficient multi-frame sequences: IDR at configurable intervals, P_Skip between
- ~76% bitstream size reduction vs all-IDR for repeated frames
- Fragmented MP4 (fMP4/CMAF) output with configurable framerate and fragment duration
- Tiling background pattern from `.gridimg` files (`-gi` with `-w`/`-h`)
- Text overlay with format patterns (`-text "%03d"`, `"%mm:%ss.%ff"`, etc.)
- Text character set: A-Z 0-9 and `! # % + - . / : = ? [ ] _ ( )` (lowercase auto-uppercased)
- Auto-scaling text (`-text-scale 0`) to fill available frame space
- Text background box (`-text-bg R,G,B`) for readability over busy patterns
- Built-in 75% SMPTE color bars pattern (`-smpte`)
- Filler NAL padding (`-bpp`) for fixed bytes-per-picture / CBR-like streams
- Stdout output (`-o -`) with explicit format flag (`-f 264`, `-f mp4`, etc.)

#### Color Space Support
- BT.601 (SD, default), BT.709 (HD), BT.2020 (UHD) color spaces
- Limited-range (Y: 16-235) and full-range (Y: 0-255) YCbCr conversion
- H.264 SPS VUI parameters signaling (colour_primaries, transfer_characteristics, matrix_coefficients, video_full_range_flag)
- Decoder extracts VUI color metadata from SPS for correct YCbCr→RGB conversion
- `.gridimg` directives: `@bt709`, `@bt2020`, `@bt601` for per-file color space; `@8x8` for 8x8 block granularity
- Y4M output with XCOLORSPACE/XCOLORRANGE tags

#### Raw Output (hi264gen)
- Raw YUV/Y4M/PNG/JPEG output from the same grid patterns (no H.264 encoding)
- Configurable color space for RGB↔YCbCr conversion (BT.601/BT.709/BT.2020)
- `.gridimg` file format with `@rgb`, `@bt709`/`@bt2020`, and `@8x8` directives

### Dependencies
- `github.com/Eyevinn/mp4ff` — SPS/PPS/SliceHeader parsing, NAL extraction, fragmented MP4 creation

### Key Reference Files
- FFmpeg: `external/ffmpeg/libavcodec/h264_cabac.c`, `h264_cavlc.c`
- Standard: `references/ISO_IEC_DIS_14496-10_Ed11.pdf`

## Build & Test

```bash
go build ./...
go test ./...
```

### CLI tools

```bash
# Decode H.264 (auto-detects .264 vs .mp4/.m4v input)
go run ./cmd/hi264dec input.264 output.yuv      # raw YUV (auto-adds _WxH_yuv420p suffix)
go run ./cmd/hi264dec input.264 output.png      # PNG (auto-detected from extension)
go run ./cmd/hi264dec input.264 output.jpg      # JPEG (-q for quality, default 85)
go run ./cmd/hi264dec input.264 output.y4m      # Y4M

# Decode IDR frames from MP4 container
go run ./cmd/hi264dec input.mp4 output.png
go run ./cmd/hi264dec -n 5 input.mp4 frames.png # extract 5 IDR frames (frames_0000.png, ...)

# Generate H.264 IDR frame from grid pattern (CAVLC, Baseline profile)
go run ./cmd/hi264gen -gi examples/sweden.gridimg -o sweden.264

# Generate H.264 IDR frame with CABAC entropy coding (Main profile)
go run ./cmd/hi264gen -gi examples/sweden.gridimg -cabac -o sweden_cabac.264

# Generate H.264 IDR frame from inline grid/colors
go run ./cmd/hi264gen -gp "xy,yx" -gc x=235,128,128 -gc y=16,128,128 -o checker.264

# Generate multi-frame H.264 sequence with frame counter
go run ./cmd/hi264gen -w 176 -h 80 -n 10 -text "%03d" -o counter.264

# Generate sequence with P_Skip frames (IDR every 50 frames, P_Skip between, CAVLC)
go run ./cmd/hi264gen -w 1280 -h 720 -n 121 -text "%03d" -idr-interval 50 -o counter.264

# With CABAC P_Skip frames (Main profile)
go run ./cmd/hi264gen -w 1280 -h 720 -n 121 -text "%03d" -cabac -idr-interval 50 -o counter.264

# Generate fragmented MP4 (25 fps, fragment every 25 frames)
go run ./cmd/hi264gen -w 176 -h 80 -n 50 -text "%03d" -o counter.mp4

# Generate MP4 with custom framerate and fragment duration
go run ./cmd/hi264gen -w 320 -h 240 -n 75 -text "%03d" -fps 30 -frag-dur 30 -o counter.mp4

# Generate sequence with tiled background pattern
go run ./cmd/hi264gen -gi examples/checker4x4.gridimg -w 176 -h 80 -n 10 -text "%03d" -o counter.264

# SMPTE color bars with counter overlay
go run ./cmd/hi264gen -smpte -w 176 -h 80 -n 10 -text "%03d" -o smpte.264

# SMPTE bars with text background box and explicit scale
go run ./cmd/hi264gen -smpte -w 352 -h 288 -n 1 -text "%02d" -text-scale 3 -text-bg 0,0,0 -o smpte_big.264

# Timestamp overlay
go run ./cmd/hi264gen -smpte -w 512 -h 240 -n 75 -fps 25 -text "%mm:%ss.%ff" -o timestamp.264

# Fixed bytes per picture (pad with filler NALUs for CBR-like streams)
go run ./cmd/hi264gen -smpte -w 176 -h 80 -bpp 5000 -o padded.264
go run ./cmd/hi264gen -w 320 -h 240 -n 50 -text "%03d" -bpp 8000 -o cbr_counter.mp4

# Pipe to stdout (requires -f to specify format)
go run ./cmd/hi264gen -smpte -w 320 -h 240 -n 100 -text "%03d" -f 264 -o - | ffplay -i -
go run ./cmd/hi264gen -smpte -w 320 -h 240 -n 100 -text "%03d" -f mp4 -o - | ffplay -i -

# PNG/JPEG image as background (downsampled to block resolution)
go run ./cmd/hi264gen -gi photo.png -o photo.264
go run ./cmd/hi264gen -gi photo.png -w 320 -h 240 -o photo_scaled.264  # scale to cover
go run ./cmd/hi264gen -gi photo.jpg -8x8 -o photo_8x8.264
go run ./cmd/hi264gen -gi photo.png -w 320 -h 240 -text "%03d" -n 10 -o counter.mp4

# Generate reference image from grid pattern (raw, no H.264 encoding)
go run ./cmd/hi264gen -gi examples/sweden.gridimg -o expected.png

# Generate tiled pattern with custom dimensions
go run ./cmd/hi264gen -gi examples/checker4x4.gridimg -w 176 -h 80 -o tiled.png

# Generate multi-frame with counter
go run ./cmd/hi264gen -gi examples/checker4x4.gridimg -w 176 -h 80 -n 5 -text "%03d" -o counter.png

# SMPTE bars reference image
go run ./cmd/hi264gen -smpte -w 176 -h 80 -o smpte.png

# Raw YUV output
go run ./cmd/hi264gen -gi examples/sweden.gridimg -o sweden.yuv

# JPEG output
go run ./cmd/hi264gen -gi examples/sweden.gridimg -q 95 -o sweden.jpg

# Color space: generate BT.709 stream with VUI signaling
go run ./cmd/hi264gen -gi examples/sweden.gridimg -colorspace bt709 -o sweden_709.264

# Full-range BT.709
go run ./cmd/hi264gen -smpte -w 320 -h 240 -colorspace bt709 -full-range -o smpte_709.264

# Decode with color space override (when VUI is absent)
go run ./cmd/hi264dec -colorspace bt709 input.264 output.png
```

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

## Architecture

```
pkg/decoder/       — Public: top-level decoder API (DecodeAnnexB, DecodeAVC, etc.)
pkg/encode/        — Public: encoder API (EncodeParams, GenerateSPS/PPS/IDR/PSkip)
pkg/frame/         — Public: Frame type (decoded output)
pkg/yuv/           — Public: Grid, ColorMap, PlaneGrid (encode input), YUV/Y4M/PNG output
internal/cabac/    — Internal: CABAC arithmetic decoder and encoder engines
internal/cavlc/    — Internal: CAVLC bitstream reader, VLC tables, residual decoder
internal/context/  — Internal: Context model initialization (1024 contexts)
internal/slice/    — Internal: Slice data parsing, MB type decoding, residual decoding
internal/transform/— Internal: Inverse quantization and transform (4x4, 8x8, DC)
internal/pred/     — Internal: Intra prediction modes (4x4, 8x8, 16x16, chroma)
cmd/hi264dec/      — CLI: decode H.264 from raw .264 or MP4 containers
cmd/hi264gen/      — CLI: generate H.264 test bitstreams or raw images from grid patterns
cmd/rawpsnr/       — CLI: compare two raw YUV420 files (overall / per-component / per-MB PSNR)
cmd/hi264-mp4-extend/ — CLI: extend a fragmented MP4 (CMAF) segment with empty P_Skip frames or a black IDR + P_Skip tail
examples/          — Example grid image files (.gridimg)
tools/             — Test generation and verification scripts
testdata/          — Golden H.264 bitstreams for regression testing
```

## Library Usage

External projects can use the decoder and encoder packages directly:

```go
import (
    "github.com/Eyevinn/hi264/pkg/decoder"
    "github.com/Eyevinn/hi264/pkg/encode"
    "github.com/Eyevinn/hi264/pkg/yuv"
)

// Decode an Annex-B byte stream
dec := decoder.New()
frame, err := dec.DecodeAnnexB(annexBData)

// Decode AVC-format data (4-byte length-prefixed NALUs)
frame, err = dec.DecodeAVC(avcData)

// Decode multi-frame stream
frames, err := dec.DecodeAllAnnexB(annexBData)

// Generate an IDR frame from Grid+ColorMap
p := encode.EncodeParams{Width: 320, Height: 240, QP: 26}
sps, _ := encode.GenerateSPS(p)
pps, _ := encode.GeneratePPS(p)
idr, _ := encode.GenerateIDR(p, grid, colors, 0)

// Generate an IDR frame from PlaneGrid (supports 8x8 blocks)
plane, _ := yuv.GridToPlaneGridBS(grid, colors, 8)
idr, _ = encode.GenerateIDRFromPlane(p, plane, 0)
```
