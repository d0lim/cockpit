package wire

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

const MaxBody = 1 << 20

var bracketedName = strings.NewReplacer("[", ".", "]", "")

func omitted(reason OmitReason) Body {
	return Body{Kind: BodyOmitted, Omitted: reason}
}

func parseBody(raw RawMessage) Body {
	switch {
	case raw.Omitted != "":
		return omitted(raw.Omitted)
	case len(raw.Body) == 0:
		return Body{}
	case len(raw.Body) > MaxBody:
		return omitted(OmitTooLarge)
	}
	plain, reason := decodeContent(raw.Body, headerValues(raw.Headers, "content-encoding"))
	if reason != "" {
		return omitted(reason)
	}
	return classify(plain, mediaType(headerValues(raw.Headers, "content-type")))
}

func decodeContent(body []byte, encodings []string) ([]byte, OmitReason) {
	var applied []string
	for _, value := range encodings {
		for name := range strings.SplitSeq(value, ",") {
			name = strings.ToLower(strings.TrimSpace(name))
			if name != "" && name != "identity" {
				applied = append(applied, name)
			}
		}
	}
	switch {
	case len(applied) == 0:
		return body, ""
	case len(applied) > 1 || (applied[0] != "gzip" && applied[0] != "x-gzip"):
		return nil, OmitEncoded
	}
	r, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, OmitEncoded
	}
	plain, err := io.ReadAll(io.LimitReader(r, MaxBody+1))
	switch {
	case err != nil:
		return nil, OmitEncoded
	case len(plain) > MaxBody:
		return nil, OmitTooLarge
	}
	return plain, ""
}

func mediaType(values []string) string {
	if len(values) == 0 {
		return ""
	}
	media, _, _ := strings.Cut(values[0], ";")
	return strings.ToLower(strings.TrimSpace(media))
}

func classify(data []byte, media string) Body {
	if strings.HasPrefix(media, "multipart/") {
		return omitted(OmitBinary)
	}
	if media == "application/x-www-form-urlencoded" {
		if form, ok := parseForm(string(data)); ok {
			return Body{Kind: BodyForm, Form: form}
		}
	}
	if declaresJSON(media) || startsLikeJSON(data) {
		if v, err := decodeJSON(data); err == nil {
			return Body{Kind: BodyJSON, JSON: v}
		}
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return omitted(OmitBinary)
	}
	if jwt, ok := decodeJWT(strings.TrimSpace(string(data))); ok {
		return Body{Kind: BodyJWT, JWT: jwt}
	}
	return Body{Kind: BodyText, Text: string(data)}
}

func declaresJSON(media string) bool {
	return media == "application/json" || strings.HasSuffix(media, "+json")
}

func startsLikeJSON(data []byte) bool {
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
}

func decodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("more than one JSON value")
	}
	return v, nil
}

func parseQuery(raw string) Body {
	if raw == "" {
		return Body{}
	}
	if form, ok := parseForm(raw); ok {
		return Body{Kind: BodyForm, Form: form}
	}
	return Body{Kind: BodyText, Text: raw}
}

func parseForm(raw string) (map[string][]string, bool) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, false
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)
	form := make(map[string][]string, len(values))
	for _, name := range names {
		if !utf8.ValidString(name) || !allValidUTF8(values[name]) {
			return nil, false
		}
		field := strings.TrimRight(bracketedName.Replace(name), ".")
		form[field] = append(form[field], values[name]...)
	}
	return form, true
}

func allValidUTF8(values []string) bool {
	return !slices.ContainsFunc(values, func(v string) bool { return !utf8.ValidString(v) })
}
