package wire

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/mask"
)

const marker = "****"

var readableHeaders = map[string]bool{
	"accept": true, "accept-encoding": true, "accept-language": true, "cache-control": true,
	"content-encoding": true, "content-length": true, "content-type": true, "date": true,
	"etag": true, "host": true, "location": true, "retry-after": true, "server": true,
	"traceparent": true, "tracestate": true, "user-agent": true, "vary": true,
	"x-correlation-id": true, "x-request-id": true,
}

func Clean(m *mask.Masker, c Call) Call {
	if c.Withheld {
		return stub(m, c)
	}
	return Call{
		Seq:      c.Seq,
		Caller:   m.Mask(c.Caller),
		Target:   m.Mask(c.Target),
		Port:     c.Port,
		Method:   m.Mask(c.Method),
		Path:     m.Mask(c.Path),
		Query:    cleanBody(m, c.Query),
		Started:  c.Started,
		Finished: c.Finished,
		Status:   c.Status,
		Outcome:  knownOutcome(c.Outcome),
		Span:     m.Mask(c.Span),
		Request:  cleanMessage(m, c.Request),
		Response: cleanMessage(m, c.Response),
	}
}

func stub(m *mask.Masker, c Call) Call {
	return Call{
		Seq:      c.Seq,
		Caller:   m.Mask(c.Caller),
		Target:   m.Mask(c.Target),
		Port:     c.Port,
		Method:   m.Mask(c.Method),
		Started:  c.Started,
		Finished: c.Finished,
		Status:   c.Status,
		Outcome:  knownOutcome(c.Outcome),
		Withheld: true,
	}
}

func knownOutcome(o Outcome) Outcome {
	switch o {
	case OutcomeAnswered, OutcomeNoUpstream, OutcomeCallerGone, OutcomeIncomplete:
		return o
	}
	return OutcomeIncomplete
}

func cleanMessage(m *mask.Masker, msg Message) Message {
	return Message{Headers: cleanHeaders(m, msg.Headers), Size: msg.Size, Body: cleanBody(m, msg.Body)}
}

func cleanHeaders(m *mask.Masker, headers map[string][]string) map[string][]string {
	if len(headers) == 0 {
		return nil
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	slices.Sort(names)
	shown := make(map[string][]string, len(headers))
	for _, name := range names {
		lower := strings.ToLower(name)
		for _, value := range headers[name] {
			if !readableHeaders[lower] {
				value = marker
			}
			shown[lower] = append(shown[lower], value)
		}
	}
	return m.Leaves(shown).(map[string][]string)
}

func cleanBody(m *mask.Masker, b Body) Body {
	switch b.Kind {
	case "":
		return Body{}
	case BodyJSON:
		return Body{Kind: BodyJSON, JSON: cleanValue(m, b.JSON)}
	case BodyForm:
		return Body{Kind: BodyForm, Form: m.Leaves(b.Form).(map[string][]string)}
	case BodyText:
		return Body{Kind: BodyText, Text: m.Mask(b.Text)}
	case BodyJWT:
		if b.JWT == nil {
			return omitted(OmitBinary)
		}
		return Body{Kind: BodyJWT, JWT: &JWT{Header: cleanObject(m, b.JWT.Header), Claims: cleanObject(m, b.JWT.Claims)}}
	case BodyOmitted:
		return omitted(knownReason(b.Omitted))
	}
	return omitted(OmitBinary)
}

func knownReason(r OmitReason) OmitReason {
	switch r {
	case OmitTooLarge, OmitBinary, OmitEncoded, OmitBusy, OmitRecordFull:
		return r
	}
	return OmitBinary
}

func cleanObject(m *mask.Masker, object map[string]any) map[string]any {
	cleaned, _ := cleanValue(m, object).(map[string]any)
	return cleaned
}

func cleanValue(m *mask.Masker, v any) any {
	return maskNumbers(m, m.Leaves(v))
}

func maskNumbers(m *mask.Masker, v any) any {
	switch x := v.(type) {
	case json.Number:
		if m.Mask(x.String()) != x.String() {
			return marker
		}
	case []any:
		for i, e := range x {
			x[i] = maskNumbers(m, e)
		}
	case map[string]any:
		for k, e := range x {
			x[k] = maskNumbers(m, e)
		}
	}
	return v
}
