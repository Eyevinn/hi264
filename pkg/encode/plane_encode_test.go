package encode

import (
	"crypto/sha256"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/Eyevinn/hi264/pkg/yuv"
)

func TestPlaneEncodeSliceReference(t *testing.T) {
	// Given fingerprints from unmodified v0.12.2, across QP 0-51 and IDR IDs 0-1.
	references := map[string]string{
		"16x16/solid/cabac=false":   "a471583f546da234f6a3ec00a788fc6e3214ac720455b6578116ce8e920ad212",
		"16x16/solid/cabac=true":    "ec9146c0b7dee6fb8f66f276d2ece362ba6c7f99954b3fa485db33e20e10f606",
		"16x16/checker/cabac=false": "1ece2594cc4d7ae5fed1afaed67a9e2b3cb48b87098aaf8835206165eb87aa98",
		"16x16/checker/cabac=true":  "38d67bc05acb228564a20d9c23c589d57b390a9cf7a6af814c92eee0937ccb80",
		"16x16/random/cabac=false":  "5845c94995df8c9c387f8f646c5d0125169e7d50344c99c4f2eade358d0d27de",
		"16x16/random/cabac=true":   "2f318409873cd6b217abac28c1a82368cd4a44c931fa1016d4b304cb0199ae79",
		"32x32/solid/cabac=false":   "0752a087fb8a07a9a294158285150b653ed879af48aee1b81e43ce3eb9a4bd7c",
		"32x32/solid/cabac=true":    "517eac06c0da5e503f503bc1134ca9d0bc25fe2bddc683e65225632becaa7aca",
		"32x32/checker/cabac=false": "c7fe36d4ad8100af3102402240a72279d9e6ced360b1a65c6f2924095d45b28e",
		"32x32/checker/cabac=true":  "e37bd65bbb25d483a07c00973b81e473d5dae77a72e992425d3a209214c42aa0",
		"32x32/random/cabac=false":  "86c88e82083f687dff8ac42233daf7ca6bfdde9780736736f5deed1fec56d50e",
		"32x32/random/cabac=true":   "424664d6ddaa59bec555660e2f724a3a4c9598974500d73fd842de8f2970abdc",
		"34x18/solid/cabac=false":   "413a12a8683504968fdc8f69b46b1e5bfdbc15a90dea2944c0661a4798ae0e9c",
		"34x18/solid/cabac=true":    "c1ff59085da1bcd0b608412a064fae5d390e118ebaec319d2d0e501cda8e5c55",
		"34x18/checker/cabac=false": "1056ead13cefbabed9bff9c18d520b314aeb334d65c6106750b3538a599651b1",
		"34x18/checker/cabac=true":  "b5bb5c8ca7b01f6d2e432cb63b9df4cb8e448762be22acc765e6c2e611e762ac",
		"34x18/random/cabac=false":  "b9c8f129906ff17de593d6359c6121345d530a939940b13099787f8237ea8e9c",
		"34x18/random/cabac=true":   "01dadf2e706f2cc1dbaca412d2ab4249612b3e90d281df3eddf413d9c1a6a592",
		"64x48/solid/cabac=false":   "42bc3c253f64230d60371186194560de8889851a1b7afab0f6dd381ce3a6c328",
		"64x48/solid/cabac=true":    "12dcca8f491e06728874e39caf0352b1bcda0e68936553857952f60e677320c0",
		"64x48/checker/cabac=false": "bc65d85be239c82bbb28dba104ee240ce30664257d3513bd79277efe6760032a",
		"64x48/checker/cabac=true":  "abc0db11760741b271a9b474074cec71aae08262f00ab7fe3e34e798cae6097e",
		"64x48/random/cabac=false":  "e645fcd45872aa4424b801a2d6b034a718f803a7a66dbfd86661fe27694bca1f",
		"64x48/random/cabac=true":   "7269387e25514d0b88520d599b017e42bced8cb39f481494570c82005b29787f",
		"66x50/solid/cabac=false":   "094738b5c2f3b5e61caa068fdab08641d86a3fe9aa099aaa7758a188a403a87b",
		"66x50/solid/cabac=true":    "d3b0112d8b9b9cd4f9dc0f333fd96dae7948b6536df06af1fa752bdd9b451a43",
		"66x50/checker/cabac=false": "deebce3992da0191fd8500c2bb5da7a3de4c2e96ab023c6ec917a66ae43dd155",
		"66x50/checker/cabac=true":  "baec300e73cc40599ff3db123db7da9c99331a8e6ce8e639f67e579c7652319f",
		"66x50/random/cabac=false":  "4bac0ec8c67dba5d133ffb031927e85d00b8ddc9683d17720acda2b9ddf9b974",
		"66x50/random/cabac=true":   "49788ffec685f08145aefa74940ae07b133ca185baab36350b828d7e0df58666",
	}
	for _, size := range [][2]int{{16, 16}, {32, 32}, {34, 18}, {64, 48}, {66, 50}} {
		for _, pattern := range []string{"solid", "checker", "random"} {
			for _, cabac := range []bool{false, true} {
				name := fmt.Sprintf("%dx%d/%s/cabac=%t", size[0], size[1], pattern, cabac)
				t.Run(name, func(t *testing.T) {
					// When each slice is encoded with its reference settings.
					got := planeSliceFingerprint(t, size[0], size[1], pattern, cabac)
					// Then prediction tie-breaking, rounding, and bitstream bytes are unchanged.
					if got != references[name] {
						t.Fatalf("slice fingerprint = %s, want %s", got, references[name])
					}
				})
			}
		}
	}
}

func TestReconstructLumaPixelDCOnly(t *testing.T) {
	// Given signed DC coefficients and predictions that exercise rounding and clipping.
	var dc [16]int32
	var pred [256]uint8
	for i := range dc {
		dc[i] = int32(i*17 - 128)
	}
	for i := range pred {
		pred[i] = uint8(i)
	}
	for qp := range 52 {
		t.Run(fmt.Sprintf("qp=%d", qp), func(t *testing.T) {
			// When reconstruction uses either the DC-only or the general transform path.
			got := reconstructLumaPixel(dc, [16][15]int32{}, pred, qp, 0)
			want := reconstructLumaPixel(dc, [16][15]int32{}, pred, qp, 1)
			// Then every reconstructed pixel is identical.
			if got != want {
				t.Fatalf("DC-only reconstruction differs from the general inverse transform")
			}
		})
	}
}

func BenchmarkPlaneEncodeSlice(b *testing.B) {
	for _, pattern := range []string{"solid", "checker", "random"} {
		for _, cabac := range []bool{false, true} {
			b.Run(fmt.Sprintf("%s/cabac=%t", pattern, cabac), func(b *testing.B) {
				encoder := FrameEncoder{
					Width: 512, Height: 288, QP: 26, CABAC: cabac,
					Plane: planeEncodeTestInput(b, 512, 288, pattern),
				}
				b.ReportAllocs()
				for b.Loop() {
					if _, err := encoder.EncodeSlice(0); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func planeSliceFingerprint(t *testing.T, width, height int, pattern string, cabac bool) string {
	t.Helper()
	plane := planeEncodeTestInput(t, width, height, pattern)
	digest := sha256.New()
	for qp := range 52 {
		for id := range uint32(2) {
			encoder := FrameEncoder{
				Width: width, Height: height, Plane: plane, QP: qp, CABAC: cabac,
			}
			data, err := encoder.EncodeSlice(id)
			if err != nil {
				t.Fatalf("QP=%d IDR=%d: %v", qp, id, err)
			}
			if len(data) == 0 {
				t.Fatalf("QP=%d IDR=%d: empty slice", qp, id)
			}
			if _, err := digest.Write(data); err != nil {
				t.Fatal(err)
			}
		}
	}
	return fmt.Sprintf("%x", digest.Sum(nil))
}

func planeEncodeTestInput(t testing.TB, width, height int, pattern string) *yuv.PlaneGrid {
	t.Helper()
	plane := yuv.NewPlaneGrid((width+15)/16*2, (height+15)/16*2, 8)
	random := rand.New(rand.NewPCG(1, 2))
	for row := range plane.Height {
		for col := range plane.Width {
			switch pattern {
			case "solid":
				plane.Y[row][col], plane.Cb[row][col], plane.Cr[row][col] = 81, 90, 240
			case "checker":
				plane.Y[row][col] = uint8(16 + 219*((row+col)%2))
				plane.Cb[row][col], plane.Cr[row][col] = 128, 128
			case "random":
				plane.Y[row][col] = uint8(random.UintN(256))
				plane.Cb[row][col] = uint8(random.UintN(256))
				plane.Cr[row][col] = uint8(random.UintN(256))
			default:
				t.Fatalf("unsupported plane pattern %q", pattern)
			}
		}
	}
	return plane
}
