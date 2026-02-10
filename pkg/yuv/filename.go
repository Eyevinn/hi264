package yuv

import (
	"fmt"
	"path/filepath"
	"strings"
)

// AddSuffix inserts a descriptive suffix (_WxH_yuv420p) before the file extension.
func AddSuffix(filename string, width, height int) string {
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)
	return fmt.Sprintf("%s_%dx%d_yuv420p%s", base, width, height, ext)
}
