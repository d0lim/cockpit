package tracectx

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const (
	Header     = "traceparent"
	traceIDLen = 32
	spanIDLen  = 16
	baseLen    = 55
)

func TraceID(runID string) string {
	sum := sha256.Sum256([]byte("cockpit/" + runID))
	return hex.EncodeToString(sum[:traceIDLen/2])
}

func ExecTraceID(runID, name string) string {
	return TraceID(runID + "/exec/" + name)
}

func Format(traceID, spanID string) string {
	return "00-" + traceID + "-" + spanID + "-01"
}

func Parse(value string) (traceID, spanID string, ok bool) {
	if len(value) < baseLen || value[2] != '-' || value[35] != '-' || value[52] != '-' {
		return "", "", false
	}
	version, traceID, spanID, flags := value[:2], value[3:35], value[36:52], value[53:55]
	if !lowerHex(version) || version == "ff" || !lowerHex(flags) || !IsTraceID(traceID) || !isID(spanID, spanIDLen) {
		return "", "", false
	}
	if len(value) > baseLen && (version == "00" || value[baseLen] != '-') {
		return "", "", false
	}
	return traceID, spanID, true
}

func IsTraceID(s string) bool {
	return isID(s, traceIDLen)
}

func isID(s string, n int) bool {
	return len(s) == n && lowerHex(s) && strings.Trim(s, "0") != ""
}

func lowerHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
