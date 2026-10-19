package worker

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

type tapped struct {
	w   io.Writer
	tap func([]byte)
}

func (t tapped) Write(p []byte) (int, error) {
	t.tap(p)
	return t.w.Write(p)
}

func (w *writer) raw(line []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := w.w.Write(append(line, '\n'))
	return err
}

type capped struct {
	mu        sync.Mutex
	b         strings.Builder
	truncated bool
}

func (c *capped) copy(r io.Reader, limit int) {
	buf := make([]byte, 32<<10)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			c.mu.Lock()
			room := limit - c.b.Len()
			if n > room {
				c.b.Write(buf[:max(room, 0)])
				c.truncated = true
			} else {
				c.b.Write(buf[:n])
			}
			c.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (h *host) outcome(done *Done, waitErr error, stderr *capped) Outcome {
	h.mu.Lock()
	defer h.mu.Unlock()
	stderr.mu.Lock()
	o := Outcome{
		Calls:           h.calls,
		Stderr:          h.spec.Mask(stderr.b.String()),
		StderrTruncated: stderr.truncated,
		Unread:          h.unread,
	}
	stderr.mu.Unlock()
	switch {
	case h.reason != "":
		o.Status, o.Kill = StatusKilled, h.reason
	case done != nil && done.OK:
		o.Status = StatusOK
	case done != nil:
		o.Status, o.Kind, o.Failure = StatusFailed, done.Kind, h.spec.Mask(done.Message)
	default:
		o.Status, o.Failure = StatusFailed, fmt.Sprintf("the script host exited without a result (%v)", waitErr)
	}
	return o
}
