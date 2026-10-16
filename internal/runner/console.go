package runner

import (
	"context"
	"fmt"
	"io"
	"log/slog"
)

type consoleHandler struct {
	w    io.Writer
	mask func(string) string
}

func (consoleHandler) Enabled(context.Context, slog.Level) bool { return true }

func (c consoleHandler) Handle(_ context.Context, r slog.Record) error {
	text := r.Message
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "text" {
			text = a.Value.String()
		}
		return true
	})
	_, err := fmt.Fprintln(c.w, c.mask("console: "+text))
	return err
}

func (c consoleHandler) WithAttrs([]slog.Attr) slog.Handler { return c }

func (c consoleHandler) WithGroup(string) slog.Handler { return c }
