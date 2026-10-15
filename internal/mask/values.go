package mask

import (
	"bytes"
	"cmp"
	"encoding/base64"
	"encoding/json"
	"html"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

func replacer(mode Mode, known []string) *strings.Replacer {
	seen := map[string]bool{}
	var olds []string
	for _, v := range known {
		if utf8.RuneCountInString(v) < mode.min() {
			continue
		}
		for _, x := range variants(v) {
			if !seen[x] {
				seen[x] = true
				olds = append(olds, x)
			}
		}
	}
	if len(olds) == 0 {
		return nil
	}
	slices.SortFunc(olds, func(a, b string) int {
		return cmp.Or(cmp.Compare(len(b), len(a)), strings.Compare(a, b))
	})
	pairs := make([]string, 0, 2*len(olds))
	for _, x := range olds {
		pairs = append(pairs, x, mode.cover(x))
	}
	return strings.NewReplacer(pairs...)
}

func variants(v string) []string {
	escaped := jsonText(v, false)
	out := []string{v, escaped, jsonText(v, true), jsonText(escaped, false), url.QueryEscape(v), url.PathEscape(v)}
	for _, s := range []string{v, escaped} {
		h := html.EscapeString(s)
		out = append(out, h, strings.ReplaceAll(h, "&#34;", "&quot;"))
	}
	for _, q := range []string{`\"`, "&quot;", "&#34;", "&#x22;", "%22"} {
		out = append(out, strings.ReplaceAll(v, `"`, q))
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		out = append(out, enc.EncodeToString([]byte(v)))
	}
	return out
}

func jsonText(s string, escapeHTML bool) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(escapeHTML)
	_ = enc.Encode(s)
	quoted := strings.TrimSuffix(b.String(), "\n")
	return quoted[1 : len(quoted)-1]
}
