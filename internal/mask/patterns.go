package mask

import (
	"regexp"
	"strings"
)

const (
	marker     = "****"
	pemMarker  = "[PEM 가림]"
	jsSpace    = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`
	secretKey  = `(?:credential|[\w-]*token|[\w-]*secret|password|session|api[_-]?key|authorization|cookie)`
	valueClass = `[^"\r\n<>]+?`
)

var quoteForms = []string{`"`, `\\{1,16}"`, `&quot;`, `&#0*34;`, `&#x0*22;`, `%22`}

type config struct {
	quotes []string
	value  string
}

type rule struct {
	re     *regexp.Regexp
	secret int
	mark   string
	tail   bool
	rewind int
	inner  *rule
}

var (
	pemRule = &rule{
		re:   regexp.MustCompile(`-----BEGIN [A-Z0-9 ]+-----[\s\S]*?-----END [A-Z0-9 ]+-----`),
		mark: pemMarker,
	}
	defaultRules = coreRules(config{quotes: quoteForms, value: valueClass})
)

func coreRules(c config) []*rule {
	q := "(?:" + strings.Join(c.quotes, "|") + ")"
	ws := "[" + jsSpace + "]"
	value := "[^" + jsSpace + `"'&<>;,\\#)}\]]+`
	head := q + secretKey + q + ws + "*(?::|%3A)" + ws + "*"
	return []*rule{
		{re: regexp.MustCompile(`(?i)(` + head + q + `)(` + c.value + `)(` + q + `|[\r\n]|$)`), secret: 2, mark: marker, rewind: 3},
		{
			re:     regexp.MustCompile(`(?i)(` + head + `\[)([^\]\r\n<>]*)(\])`),
			secret: 2,
			inner:  &rule{re: regexp.MustCompile(`(?i)(` + q + `)([^"\r\n<>]*?)(` + q + `)`), secret: 2, mark: marker},
		},
		{re: regexp.MustCompile(`(?i)(authorization` + ws + `*:` + ws + `*)(?:([a-z]+)(` + ws + `+))?([^` + jsSpace + `"'&<>\\]+)`), secret: 4, mark: marker},
		{re: regexp.MustCompile(`(?i)(secret|password|passwd|token|credential)(` + ws + `*[=:]` + ws + `*)(["']?)(` + value + `)`), secret: 4, mark: marker},
		{re: regexp.MustCompile(`(?i)(session|api[_-]?key)=(` + value + `)`), secret: 2, mark: marker},
		{re: regexp.MustCompile(`(Bearer` + ws + `+)([A-Za-z0-9._~+/=-]{6,})`), secret: 2, mark: marker},
		{re: regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*`), mark: marker},
	}
}

func declaredRule(src string) (*rule, error) {
	re, err := regexp.Compile(src)
	if err != nil {
		return nil, err
	}
	return &rule{re: re, secret: max(re.SubexpIndex("secret"), 0), tail: true}, nil
}

func (r *rule) apply(s string, mode Mode) string {
	if r.rewind > 0 {
		return r.applyRewind(s, mode)
	}
	locs := r.re.FindAllStringSubmatchIndex(s, -1)
	if locs == nil {
		return s
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		from, to := loc[2*r.secret], loc[2*r.secret+1]
		if from < 0 || from == to {
			continue
		}
		b.WriteString(s[last:from])
		b.WriteString(r.cover(s[from:to], mode))
		last = to
	}
	b.WriteString(s[last:])
	return b.String()
}

func (r *rule) applyRewind(s string, mode Mode) string {
	var b strings.Builder
	pos := 0
	for pos < len(s) {
		loc := r.re.FindStringSubmatchIndex(s[pos:])
		if loc == nil {
			break
		}
		next := pos + loc[1]
		if loc[2*r.rewind] >= 0 {
			next = pos + loc[2*r.rewind]
		}
		from, to := pos+loc[2*r.secret], pos+loc[2*r.secret+1]
		b.WriteString(s[pos:from])
		b.WriteString(r.cover(s[from:to], mode))
		b.WriteString(s[to:next])
		pos = next
	}
	b.WriteString(s[pos:])
	return b.String()
}

func (r *rule) cover(secret string, mode Mode) string {
	switch {
	case r.inner != nil:
		return r.inner.apply(secret, mode)
	case r.tail:
		return mode.cover(secret)
	}
	return r.mark
}
