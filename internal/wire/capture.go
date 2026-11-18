package wire

import (
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/tracectx"
)

type RawMessage struct {
	Headers map[string][]string
	Body    []byte
	Size    int64
	Omitted OmitReason
}

type Exchange struct {
	Seq      int64
	Caller   string
	Target   string
	Port     int
	Method   string
	Path     string
	RawQuery string
	Started  time.Time
	Finished time.Time
	Status   int
	Outcome  Outcome
	Request  RawMessage
	Response RawMessage
}

func Capture(m *mask.Masker, raw Exchange) Call {
	return Clean(m, Call{
		Seq:      raw.Seq,
		Caller:   raw.Caller,
		Target:   raw.Target,
		Port:     raw.Port,
		Method:   raw.Method,
		Path:     raw.Path,
		Query:    parseQuery(raw.RawQuery),
		Started:  raw.Started,
		Finished: raw.Finished,
		Status:   raw.Status,
		Outcome:  raw.Outcome,
		Span:     spanOf(raw.Request.Headers),
		Request:  captureMessage(raw.Request),
		Response: captureMessage(raw.Response),
	})
}

func captureMessage(raw RawMessage) Message {
	return Message{
		Headers: raw.Headers,
		Size:    max(raw.Size, int64(len(raw.Body))),
		Body:    parseBody(raw),
	}
}

func spanOf(headers map[string][]string) string {
	values := headerValues(headers, tracectx.Header)
	if len(values) != 1 {
		return ""
	}
	_, span, ok := tracectx.Parse(values[0])
	if !ok {
		return ""
	}
	return span
}

func headerValues(headers map[string][]string, name string) []string {
	var out []string
	for k, values := range headers {
		if strings.EqualFold(k, name) {
			out = append(out, values...)
		}
	}
	return out
}
