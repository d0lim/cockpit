package store

import (
	"fmt"
	"regexp"
	"time"
)

var runIDPattern = regexp.MustCompile(`^run-[0-9]{8}-[0-9]{6}-[0-9a-f]{7}(-[0-9]+)?$`)

const runIDMaxLength = 63

func IsRunID(value string) bool {
	return len(value) <= runIDMaxLength && runIDPattern.MatchString(value)
}

func NewRunID(now time.Time, sha string, existing []string) string {
	short := sha
	if len(short) > 7 {
		short = short[:7]
	}
	base := "run-" + now.Format("20060102-150405") + "-" + short
	taken := make(map[string]bool, len(existing))
	for _, id := range existing {
		taken[id] = true
	}
	if !taken[base] {
		return base
	}
	for n := 2; ; n++ {
		if id := fmt.Sprintf("%s-%d", base, n); !taken[id] {
			return id
		}
	}
}
