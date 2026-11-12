package otlp

import (
	"encoding/hex"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
)

const (
	LineVersion = 1
	ServiceAttr = "service"
	idPrefix    = "w3c:"
)

type Line struct {
	V      int    `json:"v"`
	Signal Signal `json:"signal"`
}

type Signal struct {
	Kind     contract.SignalKind  `json:"kind"`
	ID       string               `json:"id,omitempty"`
	Rev      int                  `json:"rev,omitempty"`
	TS       string               `json:"ts"`
	TSSource contract.TimeSource  `json:"tsSource"`
	Attrs    contract.Attrs       `json:"attrs"`
	Metric   *contract.MetricBody `json:"metric,omitempty"`
	Span     *contract.SpanBody   `json:"span,omitempty"`
}

func w3c(id []byte) string {
	if len(id) == 0 {
		return ""
	}
	return idPrefix + hex.EncodeToString(id)
}

func stamp(unixNano uint64) string {
	if unixNano == 0 {
		return ""
	}
	return time.Unix(0, int64(unixNano)).UTC().Format(time.RFC3339Nano)
}
