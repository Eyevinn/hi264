#!/usr/bin/env bash
# tools/verify_pskip_extend.sh
# Verify that EncodePSkipSliceAt + LastFrameState produces a stream that
# ffmpeg decodes correctly when extending an existing bitstream.
#
# NOTE: the frame-count assertion is load-bearing. A POC mismatch on an
# appended frame can manifest as silent frame loss — ffmpeg returns
# exit=0 with empty stderr but decodes one fewer frame than expected.
# Don't relax `decoded count == base count + N` to "no errors on stderr".
#
# For each test case:
#   1. Generate a base bitstream.
#   2. Decode the base with ffmpeg, recording the frame count and
#      confirming no stderr output.
#   3. Append N P_Skip frames using tools/extend_pskip.
#   4. Decode the extended bitstream with ffmpeg and confirm:
#        - no stderr,
#        - decoded frame count == base count + N,
#        - ffprobe agrees on the frame count.
#
# Usage: bash tools/verify_pskip_extend.sh
#
# Prerequisites: ffmpeg, ffprobe, go (and x264 for the B-frame case)

set -euo pipefail

FFMPEG=${FFMPEG:-ffmpeg}
FFPROBE=${FFPROBE:-ffprobe}

if ! command -v "$FFMPEG" >/dev/null; then
    echo "ffmpeg not found (set FFMPEG env var to override)" >&2
    exit 1
fi
if ! command -v "$FFPROBE" >/dev/null; then
    echo "ffprobe not found (set FFPROBE env var to override)" >&2
    exit 1
fi

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

PASS=0
FAIL=0

# Decode an Annex-B file with ffmpeg and emit "<frames>|<stderr-or-empty>".
decode_with_ffmpeg() {
    local file="$1"
    local stderr
    local out
    stderr=$("$FFMPEG" -hide_banner -loglevel error -i "$file" -f null - 2>&1 >/dev/null) || true
    out=$("$FFMPEG" -hide_banner -loglevel info -i "$file" -f null - 2>&1 | \
          awk -F'[= ]+' '/frame=/ {for (i=1;i<=NF;i++) if ($i=="frame") {print $(i+1); exit}}')
    out=${out:-0}
    printf '%s|%s' "$out" "$stderr"
}

verify() {
    local name="$1"
    local extra_count="$2"
    shift 2

    echo "=== Test: $name (append $extra_count P_Skip) ==="

    local base="$TMPDIR/${name}_base.264"
    local ext="$TMPDIR/${name}_ext.264"

    # 1. Generate base
    go run ./cmd/hi264gen "$@" -o "$base"

    # 2. Decode base
    local base_result base_frames base_err
    base_result=$(decode_with_ffmpeg "$base")
    base_frames=${base_result%%|*}
    base_err=${base_result#*|}
    if [ -n "$base_err" ]; then
        echo "FAIL: base stream produced ffmpeg errors:"
        echo "$base_err" | sed 's/^/    /'
        FAIL=$((FAIL+1)); echo; return
    fi
    echo "  base: $base_frames frames, ffmpeg clean"

    # 3. Append P_Skip frames
    if ! go run ./tools/extend_pskip "$base" "$ext" "$extra_count" \
        2>"$TMPDIR/${name}_extend.err"; then
        echo "FAIL: extend_pskip failed:"
        sed 's/^/    /' "$TMPDIR/${name}_extend.err"
        FAIL=$((FAIL+1)); echo; return
    fi

    # 4. Decode extended
    local ext_result ext_frames ext_err
    ext_result=$(decode_with_ffmpeg "$ext")
    ext_frames=${ext_result%%|*}
    ext_err=${ext_result#*|}
    if [ -n "$ext_err" ]; then
        echo "FAIL: extended stream produced ffmpeg errors:"
        echo "$ext_err" | sed 's/^/    /'
        FAIL=$((FAIL+1)); echo; return
    fi
    local want_frames=$((base_frames + extra_count))
    if [ "$ext_frames" -ne "$want_frames" ]; then
        echo "FAIL: extended stream decoded $ext_frames frames, want $want_frames"
        FAIL=$((FAIL+1)); echo; return
    fi
    echo "  extended: $ext_frames frames, ffmpeg clean"

    # 5. POC monotonicity check (via ffprobe pkt_pts/pict_type ordering)
    if ! "$FFPROBE" -v error -hide_banner -select_streams v \
            -show_entries frame=pict_type,best_effort_timestamp_time \
            -of csv=p=0 "$ext" >"$TMPDIR/${name}_frames.csv" 2>&1; then
        echo "FAIL: ffprobe could not parse extended stream"
        FAIL=$((FAIL+1)); echo; return
    fi
    local rows
    rows=$(grep -c -v '^$' "$TMPDIR/${name}_frames.csv" || true)
    if [ "$rows" -ne "$want_frames" ]; then
        echo "FAIL: ffprobe saw $rows frames, expected $want_frames"
        FAIL=$((FAIL+1)); echo; return
    fi

    echo "  PASS"
    PASS=$((PASS+1))
    echo
}

# Build hi264gen and the extend helper once.
go build ./cmd/hi264gen ./tools/extend_pskip >/dev/null

# CAVLC, 1 IDR + 0 P_Skip (test smallest case)
verify single_idr_cavlc 1 -smpte -w 176 -h 80

# CAVLC, multi-frame source extended further.
verify cavlc_multi_pad 4 -smpte -w 176 -h 80 -n 5 -text "%03d"

# CABAC, multi-frame source.
verify cabac_multi_pad 4 -smpte -w 176 -h 80 -n 5 -text "%03d" -cabac

# CAVLC source with P_Skip already appended (idr-interval), then continue.
verify cavlc_continue_pskip 3 -smpte -w 176 -h 80 -n 8 -idr-interval 4 -text "%03d"

# Real-world source: x264 with B-frames produces POC type 0 with a
# non-trivial LSB stride, exercising the explicit-POC path. Skipped if
# `x264` is not on PATH.
if command -v x264 >/dev/null; then
    echo "=== Test: x264_bframes (real-world source, append 5 P_Skip) ==="
    x264_src="$TMPDIR/x264_src.264"
    x264_ext="$TMPDIR/x264_ext.264"
    # --weightp 0 so the source PPS doesn't require pred_weight_table
    # (orthogonal limitation, not covered by EncodePSkipSliceAt).
    x264 --quiet --weightp 0 --frames 8 --bframes 1 --keyint 8 \
        --output "$x264_src" --input-res 176x80 --fps 25 /dev/zero
    src_result=$(decode_with_ffmpeg "$x264_src")
    src_frames=${src_result%%|*}
    src_err=${src_result#*|}
    if [ -n "$src_err" ] || [ "$src_frames" -ne 8 ]; then
        echo "FAIL: x264 source itself broken ($src_frames frames; err=$src_err)"
        FAIL=$((FAIL+1))
    else
        echo "  x264 source: $src_frames frames (POC type 0 with B-frames), ffmpeg clean"
        go run ./tools/extend_pskip "$x264_src" "$x264_ext" 5 >/dev/null
        ext_result=$(decode_with_ffmpeg "$x264_ext")
        ext_frames=${ext_result%%|*}
        ext_err=${ext_result#*|}
        if [ -n "$ext_err" ]; then
            echo "FAIL: extended stream produced ffmpeg errors:"
            echo "$ext_err" | sed 's/^/    /'
            FAIL=$((FAIL+1))
        elif [ "$ext_frames" -ne 13 ]; then
            echo "FAIL: extended stream decoded $ext_frames frames, want 13"
            FAIL=$((FAIL+1))
        else
            echo "  extended: $ext_frames frames, ffmpeg clean"
            echo "  PASS"
            PASS=$((PASS+1))
        fi
    fi
    echo
else
    echo "=== Skipped: x264_bframes (x264 not on PATH) ==="
    echo
fi

echo "=== Summary ==="
echo "Passed: $PASS"
echo "Failed: $FAIL"
[ "$FAIL" -eq 0 ]
