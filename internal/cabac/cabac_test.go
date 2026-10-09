package cabac

import (
	"errors"
	"math/rand/v2"
	"testing"
)

// TestErrEndOfData checks that decoding a complete CABAC stream never needs bits past its end,
// since decoding end_of_slice_flag equal to 1 ends exactly at the rbsp_stop_one_bit (section
// 9.3.3.2.4), and that the same stream without its last byte, which holds that bit, is reported as
// read past the end.
func TestErrEndOfData(t *testing.T) {
	type bin struct {
		bypass bool
		val    uint8
	}
	rng := rand.New(rand.NewPCG(1, 2))
	bins := make([]bin, 2000)
	for i := range bins {
		bins[i] = bin{bypass: rng.IntN(5) == 0, val: uint8(rng.IntN(4) / 3)}
	}
	initCtx := CtxState{PStateIdx: 20, ValMPS: 0}

	encCtx := initCtx
	enc := NewEncoder()
	for _, b := range bins {
		if b.bypass {
			enc.EncodeBypass(b.val)
		} else {
			enc.EncodeDecision(b.val, &encCtx)
		}
	}
	enc.EncodeTerminate(1)
	data := enc.Flush()

	// decodeAll decodes every bin and the end_of_slice_flag, checking the values if checkValues is set.
	decodeAll := func(data []byte, checkValues bool) *Decoder {
		t.Helper()
		dec, err := NewDecoder(data)
		if err != nil {
			t.Fatalf("NewDecoder: %v", err)
		}
		if err := dec.Err(); err != nil {
			t.Fatalf("Err after NewDecoder: %v", err)
		}
		decCtx := initCtx
		for i, b := range bins {
			var got uint8
			if b.bypass {
				got = dec.DecodeBypass()
			} else {
				got = dec.DecodeDecision(&decCtx)
			}
			if checkValues && got != b.val {
				t.Fatalf("bin %d: got %d, want %d", i, got, b.val)
			}
		}
		if term := dec.DecodeTerminate(); checkValues && term != 1 {
			t.Fatalf("end_of_slice_flag: got %d, want 1", term)
		}
		return dec
	}

	if err := decodeAll(data, true).Err(); err != nil {
		t.Errorf("complete stream of %d bytes: %v", len(data), err)
	}
	if err := decodeAll(data[:len(data)-1], false).Err(); !errors.Is(err, ErrEndOfData) {
		t.Errorf("stream without its last byte: got %v, want ErrEndOfData", err)
	}
}
