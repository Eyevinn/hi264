package encode

// levelEntry defines the limits for one H.264 level (Table A-1).
type levelEntry struct {
	levelIDC int // level_idc value written to SPS
	maxMBPS  int // max macroblock processing rate (MBs/sec)
	maxFS    int // max frame size in macroblocks
	maxBR    int // max bitrate in kbit/s for Main profile
	maxBRBL  int // max bitrate in kbit/s for Baseline profile
}

// levelTable lists H.264 levels in ascending order.
var levelTable = []levelEntry{
	{10, 1485, 99, 64, 64},
	{11, 3000, 396, 192, 192},
	{12, 6000, 396, 384, 384},
	{13, 11880, 396, 768, 768},
	{20, 11880, 396, 2000, 2000},
	{21, 19800, 792, 4000, 4000},
	{22, 20250, 1620, 4000, 4000},
	{30, 40500, 1620, 10000, 10000},
	{31, 108000, 3600, 14000, 14000},
	{32, 216000, 5120, 20000, 20000},
	{40, 245760, 8192, 20000, 20000},
	{41, 245760, 8192, 50000, 50000},
	{42, 522240, 8704, 50000, 50000},
	{50, 589824, 22080, 135000, 135000},
	{51, 983040, 36864, 240000, 240000},
}

// ChooseLevel returns the lowest H.264 level_idc that satisfies the given
// resolution, frame rate, and bitrate constraints.
//
// fps==0 means MBPS is not checked (static image or unknown rate).
// kbps==0 means bitrate is not checked.
// cabac selects Main profile bitrate limits (true) vs Baseline (false).
func ChooseLevel(width, height, fps, kbps int, cabac bool) int {
	mbW := (width + 15) / 16
	mbH := (height + 15) / 16
	fs := mbW * mbH
	mbps := 0
	if fps > 0 {
		mbps = fs * fps
	}

	for _, lv := range levelTable {
		if fs > lv.maxFS {
			continue
		}
		if mbps > 0 && mbps > lv.maxMBPS {
			continue
		}
		if kbps > 0 {
			maxBR := lv.maxBR
			if !cabac {
				maxBR = lv.maxBRBL
			}
			if kbps > maxBR {
				continue
			}
		}
		return lv.levelIDC
	}
	return 51 // fallback to highest
}
