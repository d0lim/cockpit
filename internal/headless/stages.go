package headless

import (
	"fmt"
	"io"
	"strings"
	"time"
)

type stages struct {
	out  io.Writer
	mask func(string) string
}

func (s *stages) say(format string, args ...any) {
	fmt.Fprintln(s.out, s.mask(fmt.Sprintf(format, args...)))
}

func (s *stages) run(name string, fn func() (string, error)) error {
	start := time.Now()
	detail, err := fn()
	mark := "ok  "
	if err != nil {
		mark, detail = "FAIL", strings.TrimSpace(strings.Join([]string{detail, err.Error()}, " "))
	}
	s.say("%s %s (%s): %s", mark, name, time.Since(start).Round(time.Millisecond), detail)
	return err
}
