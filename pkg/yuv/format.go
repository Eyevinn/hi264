package yuv

import "fmt"

// FormatText expands %-specifiers in pattern for the given frame number and fps.
//
// Supported specifiers:
//
//	%d       frame number (no padding)
//	%Nd      frame number space-padded to N digits (e.g. %3d)
//	%0Nd     frame number zero-padded to N digits (e.g. %03d)
//	%hh      hours (2 digits)
//	%mm      minutes (2 digits)
//	%ss      seconds (2 digits)
//	%ff      frame within current second (2 digits)
//	%ms      milliseconds (3 digits)
//	%%       literal %
func FormatText(pattern string, frameNum, fps int) string {
	if fps <= 0 {
		fps = 1
	}

	totalSeconds := frameNum / fps
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60
	frameInSecond := frameNum % fps
	milliseconds := (frameNum % fps) * 1000 / fps

	var out []byte
	i := 0
	for i < len(pattern) {
		if pattern[i] != '%' {
			out = append(out, pattern[i])
			i++
			continue
		}

		// We have a '%' — try to match specifiers
		rest := pattern[i+1:]

		// Try 2-char named specifiers first
		if len(rest) >= 2 {
			switch rest[:2] {
			case "hh":
				out = append(out, fmt.Sprintf("%02d", hours)...)
				i += 3
				continue
			case "mm":
				out = append(out, fmt.Sprintf("%02d", minutes)...)
				i += 3
				continue
			case "ss":
				out = append(out, fmt.Sprintf("%02d", seconds)...)
				i += 3
				continue
			case "ff":
				out = append(out, fmt.Sprintf("%02d", frameInSecond)...)
				i += 3
				continue
			case "ms":
				out = append(out, fmt.Sprintf("%03d", milliseconds)...)
				i += 3
				continue
			}
		}

		// Try %% (literal %)
		if len(rest) >= 1 && rest[0] == '%' {
			out = append(out, '%')
			i += 2
			continue
		}

		// Try %0Nd (zero-padded), %Nd (space-padded), or %d (no padding)
		if len(rest) >= 1 {
			j := 0
			zeroPad := false
			pad := 0
			if j < len(rest) && rest[j] == '0' {
				zeroPad = true
				j++
			}
			for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
				pad = pad*10 + int(rest[j]-'0')
				j++
			}
			if j < len(rest) && rest[j] == 'd' {
				if pad > 0 && zeroPad {
					out = append(out, fmt.Sprintf("%0*d", pad, frameNum)...)
				} else if pad > 0 {
					out = append(out, fmt.Sprintf("%*d", pad, frameNum)...)
				} else {
					out = append(out, fmt.Sprintf("%d", frameNum)...)
				}
				i += 1 + j + 1 // '%' + consumed chars + 'd'
				continue
			}
		}

		// Unknown specifier — pass through '%' literally
		out = append(out, '%')
		i++
	}

	return string(out)
}
