package cabac

import (
	"testing"
)

// TestRoundTripDecisions verifies that encoding then decoding decisions produces the original values.
func TestRoundTripDecisions(t *testing.T) {
	bins := []uint8{0, 1, 0, 0, 1, 1, 0, 1, 0, 0}

	// Encode
	enc := NewEncoder()
	ctx := CtxState{PStateIdx: 26, ValMPS: 1}
	encCtx := ctx // copy for encoder
	for _, b := range bins {
		enc.EncodeDecision(b, &encCtx)
	}
	enc.EncodeTerminate(1)
	data := enc.Flush()

	// Decode
	dec, err := NewDecoder(data)
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	decCtx := ctx // fresh copy with same initial state
	for i, want := range bins {
		got := dec.DecodeDecision(&decCtx)
		if got != want {
			t.Errorf("bin[%d]: got %d, want %d", i, got, want)
		}
	}
	term := dec.DecodeTerminate()
	if term != 1 {
		t.Errorf("terminate: got %d, want 1", term)
	}
}

// TestRoundTripBypass verifies bypass bin round-trip.
func TestRoundTripBypass(t *testing.T) {
	bins := []uint8{1, 0, 1, 1, 0, 0, 1, 0}

	enc := NewEncoder()
	for _, b := range bins {
		enc.EncodeBypass(b)
	}
	enc.EncodeTerminate(1)
	data := enc.Flush()

	dec, err := NewDecoder(data)
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	for i, want := range bins {
		got := dec.DecodeBypass()
		if got != want {
			t.Errorf("bypass[%d]: got %d, want %d", i, got, want)
		}
	}
	term := dec.DecodeTerminate()
	if term != 1 {
		t.Errorf("terminate: got %d, want 1", term)
	}
}

// TestRoundTripMixed verifies a mixed sequence of decisions, bypass, and terminate.
func TestRoundTripMixed(t *testing.T) {
	// Simulate a realistic I_16x16 MB pattern:
	// mb_type bins (decisions), bypass sign bits, terminate

	enc := NewEncoder()

	// Some decision bins with different contexts
	ctx1 := CtxState{PStateIdx: 10, ValMPS: 0}
	ctx2 := CtxState{PStateIdx: 40, ValMPS: 1}
	encCtx1 := ctx1
	encCtx2 := ctx2

	decisionBins := []struct {
		val uint8
		ctx *CtxState
	}{
		{1, &encCtx1},
		{0, &encCtx1},
		{1, &encCtx2},
		{1, &encCtx2},
		{0, &encCtx1},
	}

	for _, db := range decisionBins {
		enc.EncodeDecision(db.val, db.ctx)
	}

	// Some bypass bins
	bypassBins := []uint8{0, 1, 1, 0}
	for _, b := range bypassBins {
		enc.EncodeBypass(b)
	}

	// Non-terminating bin
	enc.EncodeTerminate(0)

	// More decisions
	enc.EncodeDecision(1, &encCtx1)
	enc.EncodeDecision(0, &encCtx2)

	// Final terminate
	enc.EncodeTerminate(1)
	data := enc.Flush()

	// Decode
	dec, err := NewDecoder(data)
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	decCtx1 := ctx1
	decCtx2 := ctx2

	decisionExpected := []struct {
		val uint8
		ctx *CtxState
	}{
		{1, &decCtx1},
		{0, &decCtx1},
		{1, &decCtx2},
		{1, &decCtx2},
		{0, &decCtx1},
	}

	for i, db := range decisionExpected {
		got := dec.DecodeDecision(db.ctx)
		if got != db.val {
			t.Errorf("decision[%d]: got %d, want %d", i, got, db.val)
		}
	}

	for i, want := range bypassBins {
		got := dec.DecodeBypass()
		if got != want {
			t.Errorf("bypass[%d]: got %d, want %d", i, got, want)
		}
	}

	term0 := dec.DecodeTerminate()
	if term0 != 0 {
		t.Errorf("terminate(0): got %d, want 0", term0)
	}

	got1 := dec.DecodeDecision(&decCtx1)
	if got1 != 1 {
		t.Errorf("decision after term(0): got %d, want 1", got1)
	}
	got2 := dec.DecodeDecision(&decCtx2)
	if got2 != 0 {
		t.Errorf("decision after term(0): got %d, want 0", got2)
	}

	term1 := dec.DecodeTerminate()
	if term1 != 1 {
		t.Errorf("final terminate: got %d, want 1", term1)
	}
}

// TestRoundTripTerminateOnly tests just a terminate(1) bin.
func TestRoundTripTerminateOnly(t *testing.T) {
	enc := NewEncoder()
	enc.EncodeTerminate(1)
	data := enc.Flush()

	if len(data) == 0 {
		t.Fatal("empty output from EncodeTerminate(1)")
	}

	dec, err := NewDecoder(data)
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	term := dec.DecodeTerminate()
	if term != 1 {
		t.Errorf("terminate: got %d, want 1", term)
	}
}

// TestRoundTripManyDecisions tests a longer sequence to exercise carry propagation.
func TestRoundTripManyDecisions(t *testing.T) {
	// Generate a sequence with many LPS events to stress carry propagation
	bins := make([]uint8, 100)
	for i := range bins {
		if i%3 == 0 || i%7 == 0 {
			bins[i] = 1
		}
	}

	enc := NewEncoder()
	ctx := CtxState{PStateIdx: 30, ValMPS: 0}
	encCtx := ctx
	for _, b := range bins {
		enc.EncodeDecision(b, &encCtx)
	}
	enc.EncodeTerminate(1)
	data := enc.Flush()

	dec, err := NewDecoder(data)
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	decCtx := ctx
	for i, want := range bins {
		got := dec.DecodeDecision(&decCtx)
		if got != want {
			t.Errorf("bin[%d]: got %d, want %d", i, got, want)
		}
	}
	term := dec.DecodeTerminate()
	if term != 1 {
		t.Errorf("terminate: got %d, want 1", term)
	}
}
