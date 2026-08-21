# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Picture Timing SEI (pic_timing)
- `encode.GeneratePicTimingSEI` / `encode.BuildPicTimingSEINALU`: build an
  H.264 Picture Timing SEI NAL unit (payload type 1) carrying a progressive-frame
  clock timestamp (HH:MM:SS:FF). Annex-B and raw-NALU forms; prepend to an IDR or
  P_Skip slice (composes with either).
- `encode.PicTiming` / `encode.PicTimingConfig` / `encode.HRDDelayLengths`: the
  per-picture data and SPS-derived syntax context. `PicTimingConfigFromSPS`
  derives the context (pic_struct_present, HRD field lengths) from a parsed SPS
  for matching a foreign stream when extending.
- `EncodeParams.PicStructPresent` and `FrameEncoder.PicStructPresent`, plus a new
  `picStructPresent` argument to `EncodeSPS`/`EncodeSPSMain`, set the VUI
  `pic_struct_present_flag` (required to attach pic_timing clock timestamps).
- `hi264gen -pic-timing`: emit a pic_timing SEI timecode (derived from `-fps`)
  per frame for `264`/`mp4` output, for both IDR and P_Skip frames; sets the SPS
  `pic_struct_present_flag`. ffprobe surfaces it as the per-frame `timecode` tag
  (SMPTE 12M side data).
- `hi264gen -start-frame N`: starting frame number that offsets the frame
  counters, timecodes, the pic_timing SEI, and (for `mp4`) the media timeline
  (`tfdt`) and fragment `sequence_number` — so independently generated segments
  concatenate into one continuous frame-number + timecode sequence.
- `hi264gen -fps` now accepts fractional rates — integer (`25`), rational
  (`30000/1001`), or NTSC decimal (`29.97`/`59.94`/`23.976`). Fractional rates
  set the MP4 media timescale to the numerator and the per-sample duration to the
  denominator, so the stream plays at the correct speed. Timecodes count at the
  nominal integer rate (`round(fps)`).
- `hi264gen -drop-frame`: NTSC drop-frame timecode counting (valid only for
  29.97/59.94); skips the timecode labels at minute boundaries and signals it in
  the pic_timing SEI (`counting_type = 4`, `cnt_dropped_flag`).
- `yuv.Timecode(frame, rate, dropFrame)`: 24-hour-wrapping timecode conversion
  with NTSC drop-frame support (rate 30/60); `TimecodeComponents` now wraps at
  24h. `yuv.FormatTextTC` adds rate + drop-frame to the text formatter. Both back
  the `-text` timecode specifiers and the pic_timing SEI.
- `hi264-mp4-extend`: appended frames continue the source's pic_timing timecodes
  when the source SPS signals `pic_struct_present_flag` (non-HRD streams).

### Fixed
- CAVLC `trailing_ones_sign_flag` order: the encoder emitted the sign flags for the trailing ones in reverse. `levels` is collected in reverse scan order, which is already the transmission order (spec 7.3.5.3.2), so the flags must be written front to back. Blocks with two or three trailing ones of mixed sign decoded with those coefficients' signs permuted. For chroma DC this transposes the 2x2 DC array (the TR/BL sub-blocks pick up equal and opposite errors), and since chroma is coded DC-only the error could not be corrected and fed the next macroblock's intra chroma prediction — showing up as colour bleeding streaking down and to the right on PNG/JPEG input. Flat `.gridimg` patterns were unaffected because a single DC coefficient never produces a mixed-sign trailing-one pair.

### Documentation
- Document `hi264gen -text` format specifiers (`%d`/`%Nd`/`%0Nd`, `%hh`/`%mm`/`%ss`/`%ff`/`%ms`, `%%`, `\n`) in both the README and the CLI help text, with copy-pasteable examples for counters, SMPTE timecode, millisecond timestamps, and multi-line overlays.

## [0.10.0] - 2026-05-07

### Added

#### 8x8 block granularity
- 8x8 block resolution for encoder: each grid character maps to an 8×8 block instead of 16×16, with 4 blocks per macroblock and proper AC residual encoding at quadrant boundaries
- `-8x8` CLI flag to enable 8x8 block mode (also settable via `@8x8` directive in `.gridimg`)
- `PlaneGrid` type: direct Y/Cb/Cr value planes with no character-count limit, supporting both 16×16 and 8×8 block granularity
- Forward 4×4 integer DCT (`ForwardTransform4x4`) for non-constant blocks in 8×8 mode
- PlaneGrid-aware encoding paths for both CAVLC and CABAC
- `GenerateIDRFromPlane()` public API for encoding from PlaneGrid
- `@8x8` directive in `.gridimg` format
- `examples/sweden_8x8.gridimg` and `examples/swiss_8x8.gridimg` example files

#### Double-resolution text glyphs
- 6×10 (`Glyph2x`) font bitmaps for 8×8 block mode — 4× the detail of standard 3×5 glyphs at the same pixel footprint (48×80 pixels per character)
- `TextGrid2x()`, `AutoTextScale2x()`, `OverlayText2x()` for 8×8 block text rendering
- Auto-selection of 2× font for even text scales (2, 4, 6…) in 16×16 mode, giving sharper glyphs without requiring `-8x8`
- `UpscaleGrid()` helper to tile 16×16 pattern backgrounds at 8×8 block resolution for the 2× font path

#### PSNR tool
- New `cmd/rawpsnr` CLI: compares two raw YUV420 files and reports overall, per-component, and (with `-per-mb`) per-16×16-macroblock PSNR; optional `-csv` export

#### Stream continuation
- `encode.AppendPSkipFrames(annexB, count)` extends an existing bitstream with N empty P_Skip frames, automatically continuing the source's `frame_num` and `pic_order_cnt_lsb` progression. One-call helper for the common "freeze last frame" use case.
- `encode.LastFrameState(annexB)` returns the `(frame_num, pic_order_cnt_lsb)` of the last slice in a bitstream — for callers that need to pick continuation values themselves.
- `encode.GenerateIDRWithSPSPPS(p, sps, pps, plane, idrPicID)` emits an IDR slice that references foreign SPS/PPS (POC type 0 or 2, arbitrary `pic_init_qp_minus26`, arbitrary `pps_id`). Used for splicing a fresh IDR onto an arbitrary upstream encoder's output.
- Support for `pic_order_cnt_type=2` in `EncodePSkipSlice` and `LastFrameState` (covers x264's default for streams without B-frames; `pic_order_cnt_lsb` is omitted from the slice header for type 2).
- Support for source PPS with `weighted_pred_flag=1`: P_Skip slice now overrides `num_ref_idx_l0_active_minus1=0` and emits a single zero-weights `pred_weight_table()` entry.
- New `cmd/hi264-mp4-extend` CLI: extends a fragmented MP4 (CMAF) media segment with N empty frames — either P_Skip copies (a freeze) or a black IDR + P_Skip tail. Output is a single fragment; concatenate with the input init segment to play (`cat init.mp4 out.m4s | ffplay -i -`).
- `tools/verify_pskip_extend.sh` and `tools/extend_pskip` for ffmpeg/ffprobe-based verification of stream extension across CAVLC, CABAC, and a real-world x264 B-frame source.
- `testdata/frag1s.mp4`, `testdata/init.mp4`, `testdata/seg1s.m4s` — 1-second fmp4 fixture (1280×720, x264 Main, CABAC) plus its split init/media parts.

#### PNG/JPEG image backgrounds
- `-gi photo.png` / `-gi photo.jpg` for using PNG/JPEG images as backgrounds
  - Without `-w`/`-h`: uses native image dimensions at 1:1 block sampling
  - With `-w`/`-h`: scales image to cover target resolution (area averaging directly to block resolution, no pixel-resolution intermediate)
  - Works with all output formats (H.264, MP4, Y4M, YUV, PNG, JPEG)
  - Supports text overlay (`-text`) and 8×8 mode (`-8x8`)
- `pkg/yuv.LoadImage()` for decoding PNG/JPEG files
- `pkg/yuv.ImageToPlaneGrid()` for 1:1 block-resolution sampling
- `pkg/yuv.ScaleImageToPlaneGrid()` for scaling to arbitrary block dimensions
- `pkg/yuv.OverlayTextOnPlane()` for text rendering directly on PlaneGrid
- `pkg/yuv.TilePlaneGrid()` for tiling a PlaneGrid to larger dimensions

### Changed
- **Breaking:** `encode.EncodePSkipSlice` now takes an explicit `picOrderCntLsb` parameter (`func(sps, pps, frameNum, picOrderCntLsb, disableDeblock)`); the previous 4-arg form silently derived `picOrderCntLsb = 2*frame_num`, which produced silently-broken streams when extending arbitrary upstream encoders. Use `AppendPSkipFrames` for the common case, or pass `2*frameNum` explicitly when starting fresh from an IDR.
- **Breaking:** hi264gen deblocking filter is off by default; `-no-deblock` is replaced by `-use-deblock` to opt back in. Block-flat encoder output gets smoothed across intentional boundaries by the loop filter, costing 6–14 dB PSNR depending on QP.
- Minimal Go version 1.24
- Ran `go fix` to modernize the code
- hi264gen uses `bufio.Writer`, reducing write syscalls by ~87%
- **Breaking:** Text character set reduced to match both fonts: A-Z 0-9 and `! # % + - . / : = ? [ ] _ ( )` plus space. Removed lowercase glyphs (a-z) and rarely-used punctuation (`" $ & ' * , ; < > @ \ ^ { | } ~`). Lowercase input is now auto-uppercased.
- SMPTE bars distribute at 8×8 block-column granularity in `-8x8` mode for more even bar widths
- `examples/sweden.gridimg` updated to more widely recognized digital flag colors (#005293 blue, #FECB00 yellow)
- `-gi` flag now accepts `.png`, `.jpg`, `.jpeg` in addition to `.gridimg`
- Internal: `BuildFrame` rewired through PlaneGrid; all CLI output paths use PlaneGrid as intermediate

### Fixed
- Slice header CABAC alignment: only emit `cabac_alignment_one_bit` while the byte is unaligned (per spec 7.3.2.8). The previous "always write 1 + zeros" path was lenient with type-0 IDR / non-deblock-control headers (some decoders don't validate the bit values), but corrupted CABAC parsing for headers that already ended on a byte boundary — surfaced when extending streams with foreign SPS/PPS using `pic_order_cnt_type=2`.
- Signal proper H.264 level depending on resolution, fps, and bitrate
- CAVLC level VLC encoding for large coefficients: fix overflow when `levelCode >> suffixLength >= 15`, and support prefix ≥ 16 for `suffixLength == 0` (needed at QP=0)
- Encoder reconstruction order: inverse Hadamard before dequant (matching H.264 spec and decoder), eliminating rounding errors for non-uniform DC values
- SMPTE bars in `-8x8` mode: bars now distributed at block-column granularity instead of macroblock granularity
- `-8x8` CLI flag correctly takes priority over `.gridimg` default block size

## [0.9.0] - 2026-02-17

### Added
- Text overlay with format patterns (`-text`) replacing digit-only `-digits` flag
  - `%d` / `%03d` / `%3d` for frame numbers (no padding, zero-padded, space-padded)
  - `%hh:%mm:%ss.%ff` for timecodes, `%ms` for milliseconds
  - Multi-line text via `\n` (e.g. `-text '%03d\n%mm:%ss'`), lines centered independently
  - Supports arbitrary text mixed with specifiers (e.g. `-text "Frame %03d"`)
- `-text-scale` and `-text-bg` flags (replacing `-digit-scale` and `-digit-bg`)
- `pkg/yuv.FormatText()` for expanding format patterns with frame number and fps
- `pkg/yuv.TextGrid()` and `pkg/yuv.AutoTextScale()` for general text rendering
- `pkg/yuv.TextHeight()` for multi-line text height calculation
- Full ASCII glyph set (A-Z, a-z, punctuation) in `pkg/yuv/font.go`
- `-kbps` flag for specifying target bitrate in kbit/s (converted to bytes per picture using `-fps`; mutually exclusive with `-bpp`)
- Adaptive intra prediction mode selection for I_16x16 encoder: selects best of Vertical, Horizontal, or DC mode per macroblock based on neighbor similarity, achieving zero residual when a neighbor matches the target color (71% CAVLC / 64% CABAC bitstream size reduction on `sweden.gridimg`)
- Flexible color space support: BT.601 (default), BT.709, and BT.2020
- Full-range and limited-range YCbCr conversion
- H.264 SPS VUI parameters (colour_primaries, transfer_characteristics, matrix_coefficients, video_full_range_flag) written when non-default color space is used
- `-colorspace` flag for hi264gen and hi264dec (`bt601`, `bt709`, `bt2020`)
- `-full-range` flag for hi264gen and hi264dec
- `.gridimg` directives: `@bt709`, `@bt2020`, `@bt601` for per-file color space control
- Y4M output includes `XCOLORSPACE` and `XCOLORRANGE` tags when non-default
- Decoder extracts VUI color metadata from SPS and uses it for YCbCr→RGB conversion
- `pkg/yuv.ColorSpace` type with `RGBToYCbCrCS()` and `YCbCrToRGBCS()` parameterized conversions
- `pkg/yuv.ParseColorSpace()`, `ColorSpaceFromMatrixCoefficients()` helpers
- `pkg/yuv.SolidGrid()` helper for creating uniform single-color grids from pixel dimensions
- `-idr-and-skip` flag for hi264dec to opt in to P_Skip frame decoding (IDR-only by default)
- hi264gen CLI validation error tests and `-smpte`/`-gi` mutual exclusivity test
- Stdout output (`-o -`) for piping into tools like ffplay
- `-f` flag for explicit output format (`264`, `mp4`, `y4m`, `yuv`, `png`, `jpg`); required with `-o -`
- Status messages (`Wrote N frames...`) now go to stderr, keeping stdout clean for piped data

### Fixed
- `tools/verify_hi264gen.sh` now accounts for hi264dec's `_WxH_yuv420p` YUV output suffix

### Changed
- **Breaking:** `-digits`, `-digit-scale`, `-digit-bg` flags replaced by `-text`, `-text-scale`, `-text-bg`
  - Migration: `-digits 3` → `-text "%03d"`, `-digit-scale 3` → `-text-scale 3`, `-digit-bg R,G,B` → `-text-bg R,G,B`
- hi264dec now decodes only IDR frames by default, matching expected behavior for real-world MP4 files
- **Breaking:** hi264gen flag renames: `-f` → `-gi` (grid image), `-grid` → `-gp` (grid pattern), `-c` → `-gc` (grid color)
  - Migration: `-f pattern.gridimg` → `-gi pattern.gridimg`, `-grid "xy,yx"` → `-gp "xy,yx"`, `-c x=235,128,128` → `-gc x=235,128,128`
  - `-f` is now the output format flag (e.g. `-f 264`, `-f mp4`)

## [0.8.0] - 2025-02-15

Initial public release of hi264 — a pure Go H.264/AVC frame decoder and
bitstream generator.

hi264 includes two CLI tools (`hi264dec` and `hi264gen`) and a Go library API.
The decoder handles IDR and P_Skip frames with both CABAC and CAVLC entropy
coding. The generator produces valid H.264 test bitstreams from flat-color
grid patterns — not a general-purpose encoder, but purpose-built for creating
test content.

Typical use cases include:

- Generating test streams for decoder conformance and regression testing
- Creating color bars, frame counters, and pattern sequences for visual verification
- Producing CBR-like streams with exact bytes-per-picture for bitrate testing
- Building ABR ladder test content with color-coded quality tiers
- Generating reference images for pixel-exact comparison with FFmpeg output
- Creating fragmented MP4 (CMAF) test content for streaming pipelines

### Added

#### Decoder (hi264dec)
- CABAC arithmetic engine and context model initialization
- CAVLC (Exp-Golomb + VLC tables) entropy decoding
- Macroblock layer parsing (I_4x4, I_8x8, I_16x16, P_Skip)
- Inverse quantization and transform (4x4, 8x8, DC Hadamard)
- Custom scaling matrices (SPS/PPS with Table 7-2 fall-back)
- Intra prediction (all 4x4, 8x8, 16x16, and chroma modes)
- Frame reconstruction and deblocking filter
- P_Skip frame decoding (copy from reference, CAVLC and CABAC)
- Multi-frame decoding via `DecodeAllFrames` (IDR + P_Skip sequences)
- Auto-detection of Annex-B (.264) and MP4 container input
- Y4M, PNG, JPEG, and raw YUV output

#### Encoder (hi264gen)
- CAVLC I_16x16 IDR frame encoder with DC prediction (Baseline profile)
- CABAC I_16x16 IDR frame encoder with DC prediction (Main profile)
- P_Skip slice encoder (CAVLC and CABAC, all-skip MBs copying from reference)
- Forward Hadamard transform and quantization (QP 0-51)
- SPS/PPS generation (Baseline and Main profiles)
- Grid-based pattern input with RGB or YCbCr color specification
- `.gridimg` file format with `@rgb` directive
- Tiling background patterns to arbitrary frame dimensions
- 7-segment frame counter overlay with auto-scaling
- Built-in 75% SMPTE color bars pattern
- Digit background box for readability over busy patterns
- Multi-frame sequences with configurable IDR interval and P_Skip between
- Filler NAL padding (`-bpp`) for fixed bytes-per-picture / CBR-like streams
- Fragmented MP4 (fMP4/CMAF) output with configurable framerate and fragment duration
- Raw YUV/Y4M/PNG/JPEG output (no H.264 encoding, for reference images)
- BT.601 limited-range RGB-to-YCbCr conversion

#### Library API
- `pkg/decoder` — `DecodeAnnexB`, `DecodeAVC`, `DecodeAllAnnexB` for frame decoding
- `pkg/encode` — `GenerateSPS`, `GeneratePPS`, `GenerateIDR`, `FillerNALU`, `PadSlice`
- `pkg/yuv` — Grid, ColorMap, Color types for encode input
- `pkg/frame` — Frame type for decoded output

#### Verification
- 41+ golden decoder test cases with pixel-perfect FFmpeg match
- 12+ encoder verification tests against FFmpeg decode

[Unreleased]: https://github.com/Eyevinn/hi264/compare/v0.10.0...HEAD
[0.10.0]: https://github.com/Eyevinn/hi264/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/Eyevinn/hi264/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/Eyevinn/hi264/releases/tag/v0.8.0