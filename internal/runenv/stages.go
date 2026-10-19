package runenv

import (
	"fmt"
	"io"
	"strings"
	"time"
)

type row struct {
	name   string
	ok     bool
	detail string
	took   time.Duration
}

type stages struct {
	out  io.Writer
	mask func(string) string
	rows []row
}

func (s *stages) say(format string, args ...any) {
	fmt.Fprintln(s.out, s.mask(fmt.Sprintf(format, args...)))
}

func (s *stages) run(name string, fn func() (string, error)) bool {
	start := time.Now()
	detail, err := fn()
	r := row{name: name, ok: err == nil, detail: detail, took: time.Since(start).Round(time.Millisecond)}
	if err != nil {
		r.detail = strings.TrimSpace(strings.Join([]string{detail, err.Error()}, " "))
	}
	s.rows = append(s.rows, r)
	mark := "ok  "
	if !r.ok {
		mark = "FAIL"
	}
	s.say("%s %s (%s): %s", mark, name, r.took, r.detail)
	return r.ok
}

func (s *stages) passed() bool {
	if len(s.rows) == 0 {
		return false
	}
	for _, r := range s.rows {
		if !r.ok {
			return false
		}
	}
	return true
}

func (s *stages) summary() string {
	var b strings.Builder
	b.WriteString("stage | result | detail\n")
	for _, r := range s.rows {
		result := "passed"
		if !r.ok {
			result = "failed"
		}
		fmt.Fprintf(&b, "%s | %s | %s\n", r.name, result, r.detail)
	}
	return s.mask(b.String())
}
