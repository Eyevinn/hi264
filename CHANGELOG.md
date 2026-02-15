# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
