package internal

import (
	"fmt"
	"strconv"
	"time"
)

var (
	commitVersion string = "v0.8.0"     // May be updated using build flags
	commitDate    string = "1739577600" // commitDate in Epoch seconds (may be overridden using build flags)
)

// GetVersion returns version and commitDate if available.
func GetVersion() string {
	seconds, _ := strconv.Atoi(commitDate)
	if commitDate != "" {
		t := time.Unix(int64(seconds), 0)
		return fmt.Sprintf("%s, date: %s", commitVersion, t.Format("2006-01-02"))
	}
	return commitVersion
}
