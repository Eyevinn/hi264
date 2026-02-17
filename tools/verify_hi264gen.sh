#!/usr/bin/env bash
# Verify hi264gen grid-only output by comparing hi264dec decode with ffmpeg decode.
#
# Usage: bash tools/verify_hi264gen.sh
#
# Requirements: ffmpeg with libx264, go

set -euo pipefail

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

PASS=0
FAIL=0

verify() {
    local name="$1"
    local grid="$2"
    shift 2

    # Check for -cabac flag
    local cabac_flag=""
    local colors=()
    for arg in "$@"; do
        if [ "$arg" = "-cabac" ]; then
            cabac_flag="-cabac"
        else
            colors+=("$arg")
        fi
    done

    echo "=== Test: $name ==="

    local color_flags=""
    for c in "${colors[@]}"; do
        color_flags="$color_flags -gc $c"
    done

    # Parse grid to determine dimensions
    local rows
    IFS=',' read -ra rows <<< "$grid"
    local ncols=${#rows[0]}
    local nrows=${#rows[@]}
    local width=$((ncols * 16))
    local height=$((nrows * 16))

    # 1. Generate .264 bitstream
    local h264="$TMPDIR/${name}.264"
    go run ./cmd/hi264gen -gp "$grid" $color_flags $cabac_flag -no-deblock -o "$h264"

    # 2. Decode with hi264dec (hi264dec adds _WxH_yuv420p suffix for .yuv)
    local go_yuv="$TMPDIR/${name}_go_${width}x${height}_yuv420p.yuv"
    go run ./cmd/hi264dec -no-deblock "$h264" "$TMPDIR/${name}_go.yuv"

    # 3. Decode with ffmpeg
    local ff_yuv="$TMPDIR/${name}_ff.yuv"
    ffmpeg -y -i "$h264" -pix_fmt yuv420p -f rawvideo "$ff_yuv" 2>/dev/null

    # 4. Compare hi264dec vs ffmpeg
    if cmp -s "$go_yuv" "$ff_yuv"; then
        echo "  PASS: hi264dec matches ffmpeg"
    else
        echo "  FAIL: hi264dec differs from ffmpeg"
        echo "  go_yuv: $(wc -c < "$go_yuv") bytes"
        echo "  ff_yuv: $(wc -c < "$ff_yuv") bytes"
        FAIL=$((FAIL + 1))
        return
    fi

    # 5. Generate expected YUV from hi264gen (raw output, also adds _WxH_yuv420p suffix)
    local expected_yuv="$TMPDIR/${name}_expected_${width}x${height}_yuv420p.yuv"
    go run ./cmd/hi264gen -gp "$grid" $color_flags -o "$TMPDIR/${name}_expected.yuv"

    # 6. Compare decoded vs expected (may differ due to quantization)
    if cmp -s "$go_yuv" "$expected_yuv"; then
        echo "  PASS: decoded output matches expected YUV (pixel-perfect)"
    else
        echo "  INFO: decoded output differs from expected (quantization loss expected)"
    fi

    PASS=$((PASS + 1))
}

echo "Building tools..."
go build ./cmd/hi264gen ./cmd/hi264dec

# Test cases
verify "solid_white" "x" "x=235,128,128"
verify "solid_black" "x" "x=16,128,128"
verify "solid_gray" "x" "x=128,128,128"
verify "2x1_bw" "xy" "x=235,128,128" "y=16,128,128"
verify "2x2_checker" "xy,yx" "x=200,100,150" "y=50,200,80"
verify "3x1_rgb" "abc" "a=235,128,128" "b=128,128,128" "c=16,128,128"
verify "2x2_gray" "xy,xy" "x=100,128,128" "y=200,128,128"
verify "3x2_pattern" "abc,cba" "a=235,128,128" "b=128,128,128" "c=16,128,128"

# CABAC test cases
verify "cabac_solid_gray" "x" "x=128,128,128" "-cabac"
verify "cabac_2x1_bw" "xy" "x=235,128,128" "y=16,128,128" "-cabac"
verify "cabac_2x2_checker" "xy,yx" "x=200,100,150" "y=50,200,80" "-cabac"
verify "cabac_3x2_pattern" "abc,cba" "a=235,128,128" "b=128,128,128" "c=16,128,128" "-cabac"

echo ""
echo "Results: $PASS passed, $FAIL failed"
if [ $FAIL -gt 0 ]; then
    exit 1
fi
