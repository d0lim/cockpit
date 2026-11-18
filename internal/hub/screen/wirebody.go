package screen

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/wire"
)

const wireBodyLimit = 256 << 10

type wireField struct {
	key    string
	values []string
}

type wireBlock struct {
	label string
	text  string
}

type wireBodyView struct {
	note   string
	blocks []wireBlock
	fields []wireField
	cut    bool
}

func (st *state) wireCall() (*api.WireCallView, error) {
	seq, err := strconv.ParseInt(st.query.Get("call"), 10, 64)
	if err != nil {
		return nil, nil
	}
	view, err := st.s.d.Reader.WireCall(st.run.ID, st.query.Get("take"), seq)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func wireSize(n int64) string {
	const unit = 1024
	switch {
	case n < unit:
		return strconv.FormatInt(n, 10) + " B"
	case n < unit*unit:
		return strconv.FormatFloat(float64(n)/unit, 'f', 1, 64) + " KiB"
	}
	return strconv.FormatFloat(float64(n)/(unit*unit), 'f', 1, 64) + " MiB"
}

func wireOmitted(reason wire.OmitReason, size int64) string {
	text := "Body not kept: " + wireSize(size)
	switch reason {
	case wire.OmitTooLarge:
		return text + ", over the 1 MiB limit"
	case wire.OmitBinary:
		return text + " of binary or multipart data"
	case wire.OmitEncoded:
		return text + " in a content encoding the recorder does not read"
	case wire.OmitBusy:
		return text + ", while the recorder held too many bodies waiting to be cleaned"
	case wire.OmitRecordFull:
		return text + ", after the record reached its limit for bodies"
	case wire.OmitUnparsed:
		return text + ", because its form fields do not parse"
	}
	return text
}

func wireFields(form map[string][]string) ([]wireField, bool) {
	keys := make([]string, 0, len(form))
	for k := range form {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]wireField, 0, len(keys))
	used := 0
	for _, k := range keys {
		used += len(k)
		for _, v := range form[k] {
			used += len(v)
		}
		if used > wireBodyLimit {
			return out, true
		}
		out = append(out, wireField{key: k, values: form[k]})
	}
	return out, false
}

func wireIndented(v any) string {
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return ""
	}
	return strings.TrimRight(out.String(), "\n")
}

func cutText(s string) (string, bool) {
	if len(s) <= wireBodyLimit {
		return s, false
	}
	end := wireBodyLimit
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end], true
}

func (b *wireBodyView) add(label, text string) {
	shown, cut := cutText(text)
	b.blocks = append(b.blocks, wireBlock{label: label, text: shown})
	b.cut = b.cut || cut
}

func wireBody(m wire.Message) wireBodyView {
	var out wireBodyView
	body := m.Body
	switch body.Kind {
	case wire.BodyJSON:
		out.add("", wireIndented(body.JSON))
	case wire.BodyText:
		out.add("", body.Text)
	case wire.BodyForm:
		out.fields, out.cut = wireFields(body.Form)
	case wire.BodyJWT:
		out.note = "Decoded JWT, signature not checked and not stored"
		if body.JWT != nil {
			out.add("Header", wireIndented(body.JWT.Header))
			out.add("Claims", wireIndented(body.JWT.Claims))
		}
	case wire.BodyOmitted:
		out.note = wireOmitted(body.Omitted, m.Size)
	default:
		if m.Size > 0 {
			out.note = "Body not kept: " + wireSize(m.Size) + " passed, but its stream did not run to its end"
		} else {
			out.note = "No body"
		}
	}
	return out
}

func wireHeaders(headers map[string][]string) []wireField {
	fields, _ := wireFields(headers)
	return fields
}

func wireQueryFields(q wire.Body) ([]wireField, string) {
	switch q.Kind {
	case wire.BodyForm:
		fields, _ := wireFields(q.Form)
		return fields, ""
	case wire.BodyText:
		text, _ := cutText(q.Text)
		return nil, text
	case wire.BodyOmitted:
		return nil, "Not kept: its fields do not parse"
	}
	return nil, ""
}

func wireLinkTone(kind wire.LinkKind) Tone {
	switch kind {
	case wire.LinkExact, wire.LinkNested:
		return ToneOK
	case wire.LinkTime:
		return ToneWarn
	}
	return ToneIdle
}
