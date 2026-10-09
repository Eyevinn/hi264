package slice

import (
	"testing"

	"github.com/Eyevinn/hi264/internal/cabac"
)

// The Exp-Golomb suffix of coeff_abs_level_minus1 is read one bypass bin at a time, so a corrupt
// slice could keep its prefix going for as long as it has data. A prefix up to the longest a
// conforming stream can have decodes; a longer one returns an error.
func TestDecodeExpGolombBypassBound(t *testing.T) {
	maxPrefix := coeffAbsLevelMaxPrefix(baseBitDepth)
	if maxPrefix != 16 {
		t.Fatalf("coeffAbsLevelMaxPrefix(%d) = %d, want 16", baseBitDepth, maxPrefix)
	}

	// encode writes a 0th-order Exp-Golomb code with the given prefix length and suffix bits.
	encode := func(prefix uint, suffix uint32) []byte {
		enc := cabac.NewEncoder()
		for range prefix {
			enc.EncodeBypass(1)
		}
		enc.EncodeBypass(0)
		for i := int(prefix) - 1; i >= 0; i-- {
			enc.EncodeBypass(uint8(suffix>>uint(i)) & 1)
		}
		enc.EncodeTerminate(1)
		return enc.Flush()
	}

	cases := []struct {
		name    string
		prefix  uint
		suffix  uint32
		want    uint32
		wantErr bool
	}{
		{"zero", 0, 0, 0, false},
		{"prefix 3", 3, 5, 7 + 5, false},
		{"longest conforming prefix", maxPrefix, 12345, 1<<maxPrefix - 1 + 12345, false},
		{"one bin too long", maxPrefix + 1, 0, 0, true},
		{"far too long", 40, 0, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := cabac.NewDecoder(encode(c.prefix, c.suffix))
			if err != nil {
				t.Fatalf("NewDecoder: %v", err)
			}
			got, err := decodeExpGolombBypass(d, maxPrefix)
			switch {
			case c.wantErr && err == nil:
				t.Fatalf("got %d, want an error", got)
			case !c.wantErr && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case !c.wantErr && got != c.want:
				t.Fatalf("got %d, want %d", got, c.want)
			}
		})
	}
}

// The limits follow the bit depth: 10-bit video extends the QP range by 12 and the level range by
// a factor of 4.
func TestLimitsFollowBitDepth(t *testing.T) {
	cases := []struct {
		bitDepth          int
		qpBdOffset        int
		qpDeltaMaxUnary   int
		levelMaxPrefixLen uint
	}{
		{8, 0, 52, 16},
		{10, 12, 64, 18},
	}
	for _, c := range cases {
		if got := qpBdOffset(c.bitDepth); got != c.qpBdOffset {
			t.Errorf("qpBdOffset(%d) = %d, want %d", c.bitDepth, got, c.qpBdOffset)
		}
		if got := qpDeltaMaxUnary(c.bitDepth); got != c.qpDeltaMaxUnary {
			t.Errorf("qpDeltaMaxUnary(%d) = %d, want %d", c.bitDepth, got, c.qpDeltaMaxUnary)
		}
		if got := coeffAbsLevelMaxPrefix(c.bitDepth); got != c.levelMaxPrefixLen {
			t.Errorf("coeffAbsLevelMaxPrefix(%d) = %d, want %d", c.bitDepth, got, c.levelMaxPrefixLen)
		}
	}
}
