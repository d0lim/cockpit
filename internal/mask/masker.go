package mask

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Mode int

const (
	Hub Mode = iota
	Runner
)

const (
	hubMin    = 8
	runnerMin = 6
	maxPasses = 8
)

type Options struct {
	Mode     Mode
	Known    []string
	Patterns []string
}

type Masker struct {
	mode     Mode
	pem      *rule
	values   *strings.Replacer
	rules    []*rule
	declared []*rule
	keys     *regexp.Regexp
}

func New(opts Options) (*Masker, error) {
	return build(opts, defaultRules)
}

func newMasker(opts Options, c config) (*Masker, error) {
	return build(opts, coreRules(c))
}

func build(opts Options, rules []*rule) (*Masker, error) {
	m := &Masker{mode: opts.Mode, pem: pemRule, rules: rules, keys: secretKeyName}
	for i, p := range opts.Patterns {
		r, err := declaredRule(p)
		if err != nil {
			return nil, fmt.Errorf("cannot compile declared pattern %d as RE2: %w", i, err)
		}
		m.declared = append(m.declared, r)
	}
	m.values = replacer(opts.Mode, opts.Known)
	return m, nil
}

func (m *Masker) WithKnown(known []string) *Masker {
	next := *m
	next.values = replacer(m.mode, known)
	return &next
}

func (m *Masker) Mask(text string) string {
	for range maxPasses {
		next := m.pass(text)
		if next == text {
			return text
		}
		text = next
	}
	return text
}

func (m *Masker) pass(s string) string {
	if m.pem != nil {
		s = m.pem.apply(s, m.mode)
	}
	if m.values != nil {
		s = m.values.Replace(s)
	}
	for _, r := range m.rules {
		s = r.apply(s, m.mode)
	}
	for _, r := range m.declared {
		s = r.apply(s, m.mode)
	}
	return s
}

func (m *Masker) Summarize(text string, max int) string {
	masked := m.Mask(text)
	n := 0
	for i := range masked {
		if n == max {
			return masked[:i] + "…"
		}
		n++
	}
	return masked
}

func (md Mode) min() int {
	if md == Runner {
		return runnerMin
	}
	return hubMin
}

func (md Mode) cover(secret string) string {
	if md != Hub || utf8.RuneCountInString(secret) < hubMin {
		return marker
	}
	runes := []rune(secret)
	return marker + string(runes[len(runes)-4:])
}
