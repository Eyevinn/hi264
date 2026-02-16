![Logo](images/logo.png)

![Test](https://github.com/Eyevinn/hi264/workflows/Go/badge.svg)
[![Coverage Status](https://coveralls.io/repos/github/Eyevinn/hi264/badge.svg?branch=main)](https://coveralls.io/github/Eyevinn/hi264?branch=main)
[![Go Reference](https://pkg.go.dev/badge/github.com/Eyevinn/hi264.svg)](https://pkg.go.dev/github.com/Eyevinn/hi264)
[![Go Report Card](https://goreportcard.com/badge/github.com/Eyevinn/hi264)](https://goreportcard.com/report/github.com/Eyevinn/hi264)
[![license](https://img.shields.io/github/license/Eyevinn/hi264.svg)](https://github.com/Eyevinn/hi264/blob/main/LICENSE)
[![Badge OSC](https://img.shields.io/badge/Evaluate-24243B?style=for-the-badge&logo=data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMjQiIGhlaWdodD0iMjQiIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPGNpcmNsZSBjeD0iMTIiIGN5PSIxMiIgcj0iMTIiIGZpbGw9InVybCgjcGFpbnQwX2xpbmVhcl8yODIxXzMxNjcyKSIvPgo8Y2lyY2xlIGN4PSIxMiIgY3k9IjEyIiByPSI3IiBzdHJva2U9ImJsYWNrIiBzdHJva2Utd2lkdGg9IjIiLz4KPGRlZnM%2BCjxsaW5lYXJHcmFkaWVudCBpZD0icGFpbnQwX2xpbmVhcl8yODIxXzMxNjcyIiB4MT0iMTIiIHkxPSIwIiB4Mj0iMTIiIHkyPSIyNCIgZ3JhZGllbnRVbml0cz0idXNlclNwYWNlT25Vc2UiPgo8c3RvcCBzdG9wLWNvbG9yPSIjQzE4M0ZGIi8%2BCjxzdG9wIG9mZnNldD0iMSIgc3RvcC1jb2xvcj0iIzREQzlGRiIvPgo8L2xpbmVhckdyYWRpZW50Pgo8L2RlZnM%2BCjwvc3ZnPgo%3D)](https://app.osaas.io/browse/eyevinn-mp4ff)

## Pure Go H.264/AVC IDR Decoder & Bitstream Generator
A pure Go H.264/AVC decoder for IDR (and P_Skip) frames with both CABAC and CAVLC
entropy coding, plus a bitstream generator for producing valid H.264 test
content with IDR and empty P-frames from flat-color 16x16 macroblock patterns.
It can also be used to extend video with extra frames without a change of SPS/PPS.

This is **not** a general-purpose video encoder — it does not accept arbitrary
pixel input or perform motion estimation. The encoder produces I_16x16 DC
prediction frames where each macroblock is a single flat color, defined by a
grid pattern. This is useful for generating test bitstreams, color bars, frame
counters, and reference content for decoder verification.

All processing is currently 8-bit 4:2:0 only (no 10-bit or 4:2:2/4:4:4 support).

Pixel-perfect match with FFmpeg IDR decoding across 41+ golden test cases covering varied
content, profiles, QP ranges, scaling matrices, deblocking, resolutions, and
both entropy coding modes.

## Build & Test

```bash
go build ./...
go test ./...
```

## CLI Tools

### hi264dec — Decode H.264 IDR frames from raw .264 or MP4

Auto-detects input format by extension (`.mp4`/`.m4v` = MP4, else Annex-B raw bitstream).
Output format detected from output extension: `.png`, `.jpg`/`.jpeg`, `.y4m`, `.yuv`.

```bash
# Raw Annex-B .264 input
go run ./cmd/hi264dec input.264 output.png             # PNG output
go run ./cmd/hi264dec input.264 output.jpg             # JPEG output
go run ./cmd/hi264dec input.264 output.y4m             # Y4M output
go run ./cmd/hi264dec input.264 output.yuv             # raw YUV (auto-adds _WxH_yuv420p suffix)

# MP4 input
go run ./cmd/hi264dec input.mp4 output.png             # decode first IDR frame
go run ./cmd/hi264dec -n 5 input.mp4 frames.png        # extract 5 IDR frames (frames_0000.png, ...)
go run ./cmd/hi264dec -n 3 input.mp4 output.y4m        # 3 frames in single Y4M file

# Options
go run ./cmd/hi264dec -no-deblock input.264 output.yuv # skip deblocking filter
go run ./cmd/hi264dec -q 95 input.264 output.jpg       # JPEG quality (default 85)
go run ./cmd/hi264dec -colorspace bt709 input.264 output.png  # override color space
go run ./cmd/hi264dec input.264                        # decode only, print info
```

### hi264gen — H.264 bitstream generator for test content

Generates valid H.264 bitstreams from grid-based patterns. Each character in a
grid maps to one 16x16 macroblock filled with a single flat color, encoded as
I_16x16 with DC prediction. This is not a general-purpose encoder — it produces
test content from color patterns, not from arbitrary video frames.

Output formats:

| Extension | Format | Notes |
|---|---|---|
| `.264` | Annex-B | Raw H.264 bitstream |
| `.mp4` | Fragmented MP4 | fMP4/CMAF with configurable fps and fragment duration |
| `.y4m` | Y4M | YUV4MPEG2 container |
| `.yuv` | Raw YUV | 4:2:0 planar (auto-adds `_WxH_yuv420p` suffix) |
| `.png` | PNG | Raw grid output (no H.264 encoding) |
| `.jpg` | JPEG | Raw grid output (`-q` for quality, default 85) |

For H.264 output, supports both CAVLC
(Baseline profile) and CABAC (Main profile) entropy coding. Multi-frame
sequences use P_Skip frames between IDR keyframes to copy the reference frame
unchanged (huge size reduction vs all-IDR). Image formats (YUV, Y4M, PNG, JPEG)
output the grid pattern directly without H.264 encoding, useful as reference
images for encode-decode chain verification.

```bash
# Grid-only: single IDR frame from grid pattern (frame size = grid size)
go run ./cmd/hi264gen -f examples/sweden.gridimg -o sweden.264
go run ./cmd/hi264gen -f examples/sweden.gridimg -cabac -o sweden_cabac.264
go run ./cmd/hi264gen -grid "xy,yx" -c x=235,128,128 -c y=16,128,128 -o checker.264
go run ./cmd/hi264gen -grid "ab" -c a=255,0,0 -c b=0,0,255 -rgb -qp 20 -no-deblock -o test.264

# Counter: frame counter digits on solid background
go run ./cmd/hi264gen -w 176 -h 80 -n 10 -digits 3 -o counter.264

# With P_Skip frames (IDR every 50 frames, P_Skip copies between, CAVLC)
go run ./cmd/hi264gen -w 1280 -h 720 -n 121 -digits 3 -idr-interval 50 -o counter.264

# With CABAC P_Skip frames (Main profile)
go run ./cmd/hi264gen -w 1280 -h 720 -n 121 -digits 3 -cabac -idr-interval 50 -o counter.264

# Fragmented MP4 output (25 fps default, fragment every 25 frames)
go run ./cmd/hi264gen -w 176 -h 80 -n 50 -digits 3 -o counter.mp4

# MP4 with custom framerate and fragment duration
go run ./cmd/hi264gen -w 320 -h 240 -n 75 -digits 3 -fps 30 -frag-dur 30 -o counter.mp4

# Tiled: grid pattern tiled to fill custom dimensions, with optional counter
go run ./cmd/hi264gen -f examples/checker4x4.gridimg -w 176 -h 80 -n 10 -digits 3 -o counter.264

# SMPTE color bars with counter overlay
go run ./cmd/hi264gen -smpte -w 176 -h 80 -n 10 -digits 3 -o smpte.264

# SMPTE bars with digit background box and explicit scale
go run ./cmd/hi264gen -smpte -w 352 -h 288 -n 1 -digits 2 -digit-scale 3 -digit-bg 0,0,0 -o smpte_big.264

# Fixed bytes per picture (pad with H.264 filler NALUs for CBR-like streams)
go run ./cmd/hi264gen -smpte -w 176 -h 80 -bpp 5000 -o padded.264
go run ./cmd/hi264gen -w 320 -h 240 -n 50 -digits 3 -bpp 8000 -o cbr_counter.mp4

# Raw image output (no H.264 encoding, useful as decoder reference)
go run ./cmd/hi264gen -f examples/sweden.gridimg -o sweden.png
go run ./cmd/hi264gen -f examples/sweden.gridimg -o sweden.yuv
go run ./cmd/hi264gen -f examples/sweden.gridimg -q 95 -o sweden.jpg
go run ./cmd/hi264gen -w 176 -h 80 -n 5 -digits 3 -o output.y4m
go run ./cmd/hi264gen -w 176 -h 80 -n 5 -digits 3 -o frame_%03d.png
```

```bash
# Color space: generate BT.709 stream (VUI signaled in SPS)
go run ./cmd/hi264gen -f examples/sweden.gridimg -colorspace bt709 -o sweden_709.264

# Full-range BT.709
go run ./cmd/hi264gen -smpte -w 320 -h 240 -colorspace bt709 -full-range -o smpte_709.264
```

Flags:

| Flag | Description | Default |
|---|---|---|
| `-f` | Grid image file (`.gridimg`) | — |
| `-grid` | Inline grid string (e.g. `"xy,yx"`) | — |
| `-c` | Color mapping (repeatable, e.g. `x=235,128,128`) | — |
| `-rgb` | Treat `-c` values as RGB instead of YCbCr | off |
| `-smpte` | Use built-in 75% SMPTE color bars pattern | off |
| `-w` | Frame width in pixels | grid width |
| `-h` | Frame height in pixels | grid height |
| `-n` | Number of frames | 1 |
| `-digits` | Counter digit count (0 = no counter) | 0 |
| `-digit-scale` | Digit scale factor (0 = auto-fit) | 0 |
| `-digit-bg` | Digit background box color (R,G,B) | none |
| `-fg` | Foreground color (R,G,B) | — |
| `-bg` | Background color (R,G,B) | — |
| `-qp` | Quantization parameter | 26 |
| `-cabac` | Use CABAC entropy coding (Main profile) | off (CAVLC) |
| `-no-deblock` | Disable deblocking filter | off |
| `-q` | JPEG quality | 85 |
| `-idr-interval` | Frames between IDR keyframes (0 = all-IDR) | 0 |
| `-bpp` | Bytes per picture (filler NAL padding) | 0 (off) |
| `-colorspace` | Color space (`bt601`/`bt709`/`bt2020`) | `bt601` |
| `-full-range` | Full-range YCbCr (0-255) | off (limited) |
| `-fps` | MP4 framerate | 25 |
| `-frag-dur` | MP4 fragment duration in frames | 25 |
| `-o` | Output file | — |

### Constant bitrate testing with `-bpp`

The `-bpp` flag pads each picture to an exact byte count using H.264 filler data
NAL units (NAL type 12, per spec section 7.3.2.7). This is useful for testing
bitrate-sensitive scenarios such as ABR ladder switching, buffer management, and
segment size constraints.

The target bitrate in kbit/s is: `bpp * 8 * fps / 1000`. For example, `-bpp 5000`
at 25 fps gives 1000 kbit/s. An error is returned if a frame's encoded slice
already exceeds the target (use a higher QP or larger `-bpp` value).

A practical pattern is to use different background colors or patterns for
different bitrate tiers so the current quality level is visually obvious during
playback:

```bash
# 500 kbit/s tier — green background
go run ./cmd/hi264gen -w 320 -h 240 -n 50 -digits 3 -bg 0,128,0 -bpp 2500 -o low.mp4

# 1500 kbit/s tier — blue background
go run ./cmd/hi264gen -w 640 -h 360 -n 50 -digits 3 -bg 0,0,200 -bpp 7500 -o mid.mp4

# 3000 kbit/s tier — red background
go run ./cmd/hi264gen -w 1280 -h 720 -n 50 -digits 3 -bg 200,0,0 -bpp 15000 -o high.mp4
```

This makes it easy to verify that an ABR player switches between the correct
renditions — you can tell which bitrate tier is active just by looking at the
background color.

## Image File Format

The `.gridimg` format combines color definitions and a grid layout in one file:

```
# Comments start with #
@rgb
@bt709
# Colors: char=v1,v2,v3 (YCbCr by default, RGB with @rgb directive or -rgb flag)
B=0,106,167
Y=254,204,0

BBBBBYYBBBBBBBBB
BBBBBYYBBBBBBBBB
YYYYYYYYYYYYYYYY
YYYYYYYYYYYYYYYY
BBBBBYYBBBBBBBBB
BBBBBYYBBBBBBBBB
```

Each character in the grid maps to one 16x16 macroblock. Supported directives:
`@rgb` (treat values as RGB), `@bt601`/`@bt709`/`@bt2020` (color space for
RGB-to-YCbCr conversion). See `examples/` for complete examples.

## Example Patterns

The `examples/` directory contains several `.gridimg` files:

| File | Description | Size (MBs) |
|---|---|---|
| `sweden.gridimg` | Swedish flag with official NCS colors | 16x10 |
| `france.gridimg` | French tricolore | 9x6 |
| `japan.gridimg` | Japanese flag (Hinomaru) | 12x8 |
| `rainbow_stripe.gridimg` | Vertical rainbow (6 colors) | 6x2 |
| `checker4x4.gridimg` | Red/cyan checkerboard | 4x4 |
| `gradient5.gridimg` | 5-shade gray gradient | 5x3 |
| `dark_saturated.gridimg` | Extreme chroma values | 4x4 |
| `logo.gridimg` | hi264 logo: SMPTE bars with text | 48x27 |

```bash
# Encode to H.264
go run ./cmd/hi264gen -f examples/sweden.gridimg -o sweden.264

# Decode to PNG
go run ./cmd/hi264dec sweden.264 sweden.png

# Generate reference PNG for comparison (raw output, no H.264)
go run ./cmd/hi264gen -f examples/sweden.gridimg -o expected.png

# Cross-verify with FFmpeg (raw YUV)
go run ./cmd/hi264dec sweden.264 sweden.yuv
ffmpeg -i sweden.264 -pix_fmt yuv420p -f rawvideo ff.yuv
cmp sweden.yuv ff.yuv  # should be identical

# Run all encoder verification tests
bash tools/verify_hi264gen.sh
```

## Library Usage

The `pkg/` packages provide a public API for use as a Go library. Implementation
details are in `internal/` and not accessible to external callers.

```go
import (
    "github.com/Eyevinn/hi264/pkg/decoder"
    "github.com/Eyevinn/hi264/pkg/encode"
    "github.com/Eyevinn/hi264/pkg/yuv"
)

// Decode an Annex-B byte stream (e.g. .264 file contents)
dec := decoder.New()
frame, err := dec.DecodeAnnexB(data)

// Decode AVC-format data (4-byte length-prefixed NALUs, e.g. from MP4 samples)
frame, err = dec.DecodeAVC(sampleData)

// Decode multi-frame stream (IDR + P_Skip)
frames, err := dec.DecodeAllAnnexB(data)

// Generate H.264 test bitstream (flat-color I_16x16 macroblocks from grid pattern)
p := encode.EncodeParams{Width: 320, Height: 240, QP: 26}
sps, _ := encode.GenerateSPS(p)
pps, _ := encode.GeneratePPS(p)
idr, _ := encode.GenerateIDR(p, grid, colors, 0)
```

### Appending frames to an existing bitstream

This example parses SPS/PPS from an existing H.264 bitstream, then appends
a black IDR frame and a P\_Skip frame that are compatible with the original
parameter sets:

```go
import (
    "github.com/Eyevinn/mp4ff/avc"
    "github.com/Eyevinn/hi264/pkg/encode"
    "github.com/Eyevinn/hi264/pkg/yuv"
)

// Parse parameter sets from the existing bitstream
nalus := avc.ExtractNalusFromByteStream(existingStream)
spsMap := make(map[uint32]*avc.SPS)
var sps *avc.SPS
var pps *avc.PPS
for _, nalu := range nalus {
    if len(nalu) < 1 {
        continue
    }
    naluType := nalu[0] & 0x1f
    switch naluType {
    case 7: // SPS
        sps, _ = avc.ParseSPSNALUnit(nalu, true)
        spsMap[sps.ParameterID] = sps
    case 8: // PPS
        pps, _ = avc.ParsePPSNALUnit(nalu, spsMap)
    }
}

// Create a single-color black grid matching the frame dimensions
w := int(sps.Width)
h := int(sps.Height)
blackY := uint8(16)  // limited range black
if sps.VUI != nil && sps.VUI.VideoFullRangeFlag {
    blackY = 0       // full range black
}
grid, colors := yuv.SolidGrid(w, h, yuv.Color{Y: blackY, Cb: 128, Cr: 128})

// Encode a black IDR frame using parameters matching the existing SPS/PPS
p := encode.EncodeParams{
    Width:  w,
    Height: h,
    QP:     26,
    CABAC:  pps.EntropyCodingModeFlag,
}
idrSlice, _ := encode.GenerateIDR(p, grid, colors, 0)

// Encode a P_Skip slice (copies the IDR frame unchanged)
pSkipSlice, _ := encode.EncodePSkipSlice(sps, pps, 1, 0)

// Append to the original stream
stream := append(existingStream, idrSlice...)
stream = append(stream, pSkipSlice...)
```

## Architecture

```
pkg/decoder/       — Public: top-level decoder API (DecodeAnnexB, DecodeAVC, etc.)
pkg/encode/        — Public: bitstream generator API (flat-color I_16x16 IDR + P_Skip)
pkg/frame/         — Public: Frame type (decoded output)
pkg/yuv/           — Public: Grid, ColorMap, Color (encode input), YUV/Y4M/PNG output
internal/cabac/    — Internal: CABAC arithmetic decoder and encoder engines
internal/cavlc/    — Internal: CAVLC bitstream reader, VLC tables, residual decoder
internal/context/  — Internal: Context model initialization (1024 contexts)
internal/slice/    — Internal: Slice data parsing, MB type decoding, residual decoding
internal/transform/— Internal: Inverse quantization and transform (4x4, 8x8, DC)
internal/pred/     — Internal: Intra prediction modes (4x4, 8x8, 16x16, chroma)
cmd/hi264dec/      — CLI: decode H.264 from raw .264 or MP4 containers
cmd/hi264gen/      — CLI: generate H.264 bitstreams or raw images from grid patterns
examples/          — Example grid image files
tools/             — Test generation and verification scripts
testdata/          — Golden H.264 bitstreams for regression testing
```

## Dependencies

- [`github.com/Eyevinn/mp4ff`](https://github.com/Eyevinn/mp4ff) — SPS/PPS/SliceHeader parsing, MP4 container, NAL extraction, fragmented MP4 creation

## Support

Join our [community on Slack](http://slack.streamingtech.se) where you can post any questions regarding any of our open source projects. Eyevinn's consulting business can also offer you:

* Further development of this component
* Customization and integration of this component into your platform
* Support and maintenance agreement

Contact [sales@eyevinn.se](mailto:sales@eyevinn.se) if you are interested.

## About Eyevinn Technology

[Eyevinn Technology](https://www.eyevinntechnology.se) is an independent consultant firm specialized in video and streaming. Independent in a way that we are not commercially tied to any platform or technology vendor. As our way to innovate and push the industry forward we develop proof-of-concepts and tools. The things we learn and the code we write we share with the industry in [blogs](https://dev.to/video) and by open sourcing the code we have written.

Want to know more about Eyevinn and how it is to work here. Contact us at work@eyevinn.se!
