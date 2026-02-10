package context

import "testing"

// TestInitModelsPSlice verifies P-slice context initialization for a few known
// entries to catch transcription errors in the PB init tables.
func TestInitModelsPSlice(t *testing.T) {
	// P-slice (type=0), QP=26, cabac_init_idc=0
	models := InitModels(26, 0, 0)

	// Verify context index 11 (mb_skip_flag base for P-slices).
	// PB table idc=0, index 11: {m=23, n=33}
	// preCtxState = clip3(1, 126, ((23*26)>>4) + 33) = clip3(1, 126, 37+33) = 70
	// Since 70 > 63: pStateIdx = 70 - 64 = 6, valMPS = 1
	if models[11].PStateIdx != 6 || models[11].ValMPS != 1 {
		t.Errorf("ctx[11]: got pStateIdx=%d valMPS=%d, want pStateIdx=6 valMPS=1",
			models[11].PStateIdx, models[11].ValMPS)
	}

	// Verify context index 0 (shared with I-slice table, should match).
	// PB table idc=0, index 0: {m=20, n=-15}
	// preCtxState = clip3(1, 126, ((20*26)>>4) + (-15)) = clip3(1, 126, 32-15) = 17
	// Since 17 <= 63: pStateIdx = 63 - 17 = 46, valMPS = 0
	if models[0].PStateIdx != 46 || models[0].ValMPS != 0 {
		t.Errorf("ctx[0]: got pStateIdx=%d valMPS=%d, want pStateIdx=46 valMPS=0",
			models[0].PStateIdx, models[0].ValMPS)
	}

	// Verify with cabac_init_idc=1
	models1 := InitModels(26, 0, 1)

	// PB table idc=1, index 11: {m=22, n=25}
	// preCtxState = clip3(1, 126, ((22*26)>>4) + 25) = clip3(1, 126, 35+25) = 60
	// Since 60 <= 63: pStateIdx = 63 - 60 = 3, valMPS = 0
	if models1[11].PStateIdx != 3 || models1[11].ValMPS != 0 {
		t.Errorf("idc=1 ctx[11]: got pStateIdx=%d valMPS=%d, want pStateIdx=3 valMPS=0",
			models1[11].PStateIdx, models1[11].ValMPS)
	}

	// Verify with cabac_init_idc=2
	models2 := InitModels(26, 0, 2)

	// PB table idc=2, index 11: {m=29, n=16}
	// preCtxState = clip3(1, 126, ((29*26)>>4) + 16) = clip3(1, 126, 47+16) = 63
	// Since 63 <= 63: pStateIdx = 63 - 63 = 0, valMPS = 0
	if models2[11].PStateIdx != 0 || models2[11].ValMPS != 0 {
		t.Errorf("idc=2 ctx[11]: got pStateIdx=%d valMPS=%d, want pStateIdx=0 valMPS=0",
			models2[11].PStateIdx, models2[11].ValMPS)
	}
}

// TestInitModelsISlice verifies I-slice context initialization still works.
func TestInitModelsISlice(t *testing.T) {
	models := InitModels(26, 2, 0) // I-slice

	// I-slice table, index 0: {m=20, n=-15} (same as PB table idc=0)
	// preCtxState = clip3(1, 126, ((20*26)>>4) + (-15)) = 17
	// pStateIdx = 63 - 17 = 46, valMPS = 0
	if models[0].PStateIdx != 46 || models[0].ValMPS != 0 {
		t.Errorf("I-slice ctx[0]: got pStateIdx=%d valMPS=%d, want pStateIdx=46 valMPS=0",
			models[0].PStateIdx, models[0].ValMPS)
	}
}
