#!/bin/bash
# tools/gen_and_verify.sh
# Generate H.264 IDR test files with varied x264 settings and verify
# byte-exact YUV match between FFmpeg and hi264.
#
# All tests stay within supported features: CABAC, progressive, 8-bit, 4:2:0, I-slices only.
#
# Prerequisites:
#   - FFmpeg with libx264 support (default: /opt/homebrew/bin/ffmpeg)
#   - Go toolchain (for building hi264)
#
# Each test case:
#   1. Encodes a single IDR frame with ffmpeg + libx264 using specific options
#   2. Decodes with system ffmpeg to produce reference YUV
#   3. Decodes with hi264 (this project) to produce test YUV
#   4. Compares byte-for-byte — any difference = FAIL
#
# Generated files go to /tmp/hi264_tests/:
#   <name>.264       — encoded H.264 bitstream (Annex B)
#   <name>_ref.yuv   — FFmpeg-decoded reference YUV
#   <name>_go.yuv    — hi264-decoded YUV
#
# Usage:
#   bash tools/gen_and_verify.sh              # run all tests
#   bash tools/gen_and_verify.sh <pattern>    # run tests matching pattern (grep -E)

set -euo pipefail

FFMPEG=/opt/homebrew/bin/ffmpeg
H264DEC=./hi264dec
OUTDIR=/tmp/hi264_tests
MIX_SLICE_DEBLOCK="${OUTDIR}/mix_slice_deblock"
FILTER="${1:-}"

PASS=0
FAIL=0
SKIP=0
ERRORS=()

mkdir -p "$OUTDIR"

# Build decoder and slice header rewriter
echo "=== Building hi264dec ==="
go build -o "$H264DEC" ./cmd/hi264dec
go build -o "$MIX_SLICE_DEBLOCK" ./tools/mix_slice_deblock
echo ""

# skip_test returns success, and counts the test as skipped, if the filter
# excludes it.
skip_test() {
    local name=$1
    if [[ -n "$FILTER" ]] && ! echo "$name" | grep -qE "$FILTER"; then
        SKIP=$((SKIP + 1))
        return 0
    fi
    return 1
}

# encode_test encodes 1 IDR frame with ffmpeg + libx264 into an Annex-B file.
encode_test() {
    local name=$1 width=$2 height=$3 source=$4 profile=$5 h264=$6
    shift 6
    local x264extra="$*"

    local base_opts="keyint=1:min-keyint=1:bframes=0:no-scenecut:cabac=1"
    local opts="$base_opts"
    if [[ -n "$x264extra" ]]; then
        opts="${base_opts}:${x264extra}"
    fi

    # Build lavfi source string: use ':' if source already has '=' (e.g. color=c=black),
    # otherwise use '=' as the filter-name/params separator
    local lavfi
    if [[ "$source" == *=* ]]; then
        lavfi="${source}:s=${width}x${height}:r=1"
    else
        lavfi="${source}=s=${width}x${height}:r=1"
    fi

    if ! $FFMPEG -y -loglevel error \
        -f lavfi -i "$lavfi" \
        -t 1 -pix_fmt yuv420p -frames:v 1 \
        -c:v libx264 -profile:v "$profile" \
        -x264opts "$opts" -f h264 \
        "$h264" 2>"${OUTDIR}/${name}_encode.log"; then
        echo "FAIL (encode error)"
        FAIL=$((FAIL + 1))
        ERRORS+=("$name: ffmpeg encode failed — see ${OUTDIR}/${name}_encode.log")
        return 1
    fi
}

# verify_test decodes <name>.264 with FFmpeg and hi264 and compares the output.
verify_test() {
    local name=$1 width=$2 height=$3
    local base="${OUTDIR}/${name}"
    local h264="${base}.264"
    local ref_yuv="${base}_ref.yuv"
    # hi264dec appends _WxH_yuv420p to the .yuv name it is given
    local go_yuv="${base}_go_${width}x${height}_yuv420p.yuv"

    # Decode with FFmpeg (reference)
    if ! $FFMPEG -y -loglevel error \
        -i "$h264" -pix_fmt yuv420p \
        -f rawvideo "$ref_yuv" 2>"${base}_ffdec.log"; then
        echo "FAIL (ffmpeg decode error)"
        FAIL=$((FAIL + 1))
        ERRORS+=("$name: ffmpeg decode failed — see ${base}_ffdec.log")
        return
    fi

    # Decode with hi264
    if ! $H264DEC "$h264" "${base}_go.yuv" >"${base}_godec.log" 2>&1; then
        echo "FAIL (hi264 error)"
        FAIL=$((FAIL + 1))
        ERRORS+=("$name: hi264 failed — see ${base}_godec.log")
        return
    fi

    # Compare byte-for-byte
    if cmp -s "$ref_yuv" "$go_yuv"; then
        echo "PASS"
        PASS=$((PASS + 1))
    else
        local ref_size go_size
        ref_size=$(wc -c < "$ref_yuv" | tr -d ' ')
        go_size=$(wc -c < "$go_yuv" | tr -d ' ')
        if [[ "$ref_size" -ne "$go_size" ]]; then
            echo "FAIL (size mismatch: ref=${ref_size} go=${go_size})"
        else
            # Find first differing byte offset (cmp returns 1 for differing files)
            local diff_info
            diff_info=$(cmp "$ref_yuv" "$go_yuv" 2>&1 || true)
            echo "FAIL (${diff_info})"
        fi
        FAIL=$((FAIL + 1))
        ERRORS+=("$name: YUV mismatch (ref=${ref_size} go=${go_size})")
    fi
}

# run_test encodes a test case with x264 and verifies it.
run_test() {
    local name=$1 width=$2 height=$3 source=$4 profile=$5
    shift 5
    skip_test "$name" && return

    printf "%-50s " "$name"
    encode_test "$name" "$width" "$height" "$source" "$profile" "${OUTDIR}/${name}.264" "$@" || return 0
    verify_test "$name" "$width" "$height"
}

# run_mixed_deblock_test encodes a multi-slice test case with x264, gives its
# slices different deblocking controls with tools/mix_slice_deblock (x264
# uses the same for all), and verifies the result.
run_mixed_deblock_test() {
    local name=$1 width=$2 height=$3 source=$4 profile=$5
    shift 5
    skip_test "$name" && return

    printf "%-50s " "$name"
    local x264_out="${OUTDIR}/${name}.x264"
    encode_test "$name" "$width" "$height" "$source" "$profile" "$x264_out" "$@" || return 0
    if ! $MIX_SLICE_DEBLOCK "$x264_out" "${OUTDIR}/${name}.264" >"${OUTDIR}/${name}_mix.log" 2>&1; then
        echo "FAIL (mix_slice_deblock error)"
        FAIL=$((FAIL + 1))
        ERRORS+=("$name: mix_slice_deblock failed — see ${OUTDIR}/${name}_mix.log")
        return
    fi
    verify_test "$name" "$width" "$height"
}

echo "=== Running H.264 IDR decode verification tests ==="
echo "Output directory: $OUTDIR"
echo ""

# ---------------------------------------------------------------------------
# Group 1: Content sweep — 5 content sources x high_default @ 320x240
# ---------------------------------------------------------------------------
echo "--- Group 1: Content sweep (high_default @ 320x240) ---"
run_test "content_black"      320 240 "color=c=black"  high ""
run_test "content_red"        320 240 "color=c=red"    high ""
run_test "content_testsrc2"   320 240 "testsrc2"       high ""
run_test "content_smptebars"  320 240 "smptebars"      high ""
run_test "content_mandelbrot" 320 240 "mandelbrot"     high ""
echo ""

# ---------------------------------------------------------------------------
# Group 2: Profile & partition — testsrc2 @ 320x240
# ---------------------------------------------------------------------------
echo "--- Group 2: Profile & partition ---"
run_test "profile_main"    320 240 "testsrc2" main ""
run_test "part_none"       320 240 "testsrc2" high "partitions=none"
run_test "part_i4x4_only"  320 240 "testsrc2" main "partitions=i4x4"
echo ""

# ---------------------------------------------------------------------------
# Group 3: QP sweep — testsrc2 @ 320x240
# ---------------------------------------------------------------------------
echo "--- Group 3: QP sweep ---"
run_test "qp10"  320 240 "testsrc2" high "qp=10:aq-mode=0"
run_test "qp40"  320 240 "testsrc2" high "qp=40:aq-mode=0"
run_test "qp51"  320 240 "testsrc2" high "qp=51:aq-mode=0"
echo ""

# ---------------------------------------------------------------------------
# Group 4: Adaptive QP — testsrc2 @ 320x240
# ---------------------------------------------------------------------------
echo "--- Group 4: Adaptive QP ---"
run_test "aq_variance"      320 240 "testsrc2" high "aq-mode=1:aq-strength=1.0"
run_test "aq_autovar_strong" 320 240 "testsrc2" high "aq-mode=2:aq-strength=1.5"
echo ""

# ---------------------------------------------------------------------------
# Group 5: Chroma QP offset — testsrc2 @ 320x240
# ---------------------------------------------------------------------------
echo "--- Group 5: Chroma QP offset ---"
run_test "cqp_offset_neg4"  320 240 "testsrc2" high "qp=26:chroma-qp-offset=-4"
run_test "cqp_offset_pos4"  320 240 "testsrc2" high "qp=26:chroma-qp-offset=4"
echo ""

# ---------------------------------------------------------------------------
# Group 6: Trellis — testsrc2 @ 320x240
# ---------------------------------------------------------------------------
echo "--- Group 6: Trellis ---"
run_test "trellis0"  320 240 "testsrc2" high "trellis=0"
run_test "trellis2"  320 240 "testsrc2" high "trellis=2"
echo ""

# ---------------------------------------------------------------------------
# Group 7: Quant matrix — testsrc2 @ 320x240
# ---------------------------------------------------------------------------
echo "--- Group 7: Quant matrix ---"
run_test "cqm_jvt"  320 240 "testsrc2" high "cqm=jvt"
echo ""

# ---------------------------------------------------------------------------
# Group 8: Deblocking — testsrc2 @ 320x240
# ---------------------------------------------------------------------------
echo "--- Group 8: Deblocking ---"
run_test "deblock_strong"  320 240 "testsrc2" high "deblock=3,3"
run_test "deblock_off"     320 240 "testsrc2" high "no-deblock"
run_test "deblock_neg"     320 240 "testsrc2" high "deblock=-3,-3"
echo ""

# ---------------------------------------------------------------------------
# Group 9: Deadzone & DCT decimate — testsrc2 @ 320x240
# ---------------------------------------------------------------------------
echo "--- Group 9: Deadzone & DCT decimate ---"
run_test "dz_min"       320 240 "testsrc2" high "deadzone-intra=0"
run_test "dz_max"       320 240 "testsrc2" high "deadzone-intra=32"
run_test "no_dct_dec"   320 240 "testsrc2" high "no-dct-decimate"
echo ""

# ---------------------------------------------------------------------------
# Group 10: Prediction control — testsrc2 @ 320x240
# ---------------------------------------------------------------------------
echo "--- Group 10: Prediction control ---"
run_test "constrained_intra"  320 240 "testsrc2" high "constrained-intra"
run_test "no_psy"             320 240 "testsrc2" high "no-psy"
run_test "subme1"             320 240 "testsrc2" high "subme=1"
run_test "subme11"            320 240 "testsrc2" high "subme=11:trellis=2:aq-mode=1"
echo ""

# ---------------------------------------------------------------------------
# Group 11: Resolution sweep — testsrc2 x high_default
# ---------------------------------------------------------------------------
echo "--- Group 11: Resolution sweep ---"
run_test "res_tiny"    64   64  "testsrc2" high ""
run_test "res_qcif"    176  144 "testsrc2" high ""
run_test "res_wide"    640  360 "testsrc2" high ""
run_test "res_hd"      1280 720 "testsrc2" high ""
echo ""

# ---------------------------------------------------------------------------
# Group 12: CAVLC entropy coding — cabac=0 overrides base_opts' cabac=1
# ---------------------------------------------------------------------------
echo "--- Group 12: CAVLC ---"
run_test "cavlc_baseline"      320 240 "testsrc2"      baseline "cabac=0"
run_test "cavlc_main"           320 240 "testsrc2"      main     "cabac=0"
run_test "cavlc_high"           320 240 "testsrc2"      high     "cabac=0"
run_test "cavlc_qp10"           320 240 "testsrc2"      high     "cabac=0:qp=10:aq-mode=0"
run_test "cavlc_qp40"           320 240 "testsrc2"      high     "cabac=0:qp=40:aq-mode=0"
run_test "cavlc_black"          320 240 "color=c=black" high     "cabac=0"
run_test "cavlc_tiny"            64  64 "testsrc2"      high     "cabac=0"
run_test "cavlc_hd"            1280 720 "testsrc2"      high     "cabac=0"
run_test "cavlc_cqm"           320 240 "testsrc2"      high     "cabac=0:cqm=jvt"
echo ""

# ---------------------------------------------------------------------------
# Group 13: Multi-slice pictures — slice starts at row starts and mid-row,
# one macroblock per slice, and slices of a byte budget. The mixed tests give
# each slice its own disable_deblocking_filter_idc (0, 1, 2), filter offsets
# and, for CAVLC, slice QP.
# ---------------------------------------------------------------------------
echo "--- Group 13: Multi-slice ---"
run_test "slices_4"              320 240 "testsrc2"   high     "slices=4"
run_test "slices_midrow"         320 240 "testsrc2"   high     "slice-max-mbs=37"
run_test "slices_mb1"            176 144 "testsrc2"   main     "slice-max-mbs=1"
run_test "slices_bytes"          320 240 "mandelbrot" high     "slice-max-size=400"
run_test "cavlc_slices_4"        320 240 "testsrc2"   high     "cabac=0:slices=4"
run_test "cavlc_slices_midrow"   320 240 "testsrc2"   high     "cabac=0:slice-max-mbs=37"
run_test "cavlc_slices_baseline" 320 240 "testsrc2"   baseline "cabac=0:slice-max-mbs=29"
run_mixed_deblock_test "slices_mixed_deblock"       320 240 "testsrc2" high "slice-max-mbs=37"
run_mixed_deblock_test "cavlc_slices_mixed_deblock" 320 240 "testsrc2" high "cabac=0:slice-max-mbs=37"
echo ""

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo "==========================================="
echo "  RESULTS: $PASS passed, $FAIL failed, $SKIP skipped"
echo "==========================================="

if [[ ${#ERRORS[@]} -gt 0 ]]; then
    echo ""
    echo "Failures:"
    for err in "${ERRORS[@]}"; do
        echo "  - $err"
    done
fi

if [[ $FAIL -gt 0 ]]; then
    exit 1
fi
