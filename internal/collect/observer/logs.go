package observer

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports"
	dockerport "github.com/d0lim/cockpit/internal/ports/docker"
	"github.com/d0lim/cockpit/internal/signal"
)

const (
	DefaultPoll    = 2 * time.Second
	DefaultMaxLine = 16 << 10
)

type Emitter interface {
	Append(sig contract.Signal) (int64, error)
}

type Lister interface {
	Containers(ctx context.Context) ([]dockerport.Container, error)
}

type Logs struct {
	Lister  Lister
	Docker  compose.Docker
	Emit    Emitter
	Mask    ports.Mask
	Poll    time.Duration
	MaxLine int
	Now     func() time.Time
}

func (l *Logs) Name() string { return signal.LogCollector }

func (l *Logs) defaults() {
	if l.Mask == nil {
		l.Mask = ports.Identity
	}
	if l.Poll <= 0 {
		l.Poll = DefaultPoll
	}
	if l.MaxLine <= 0 {
		l.MaxLine = DefaultMaxLine
	}
	if l.Now == nil {
		l.Now = time.Now
	}
}

func (l *Logs) Run(ctx context.Context) error {
	l.defaults()
	since := l.Now().UTC()
	following := map[string]context.CancelFunc{}
	var wg sync.WaitGroup
	defer func() {
		for _, stop := range following {
			stop()
		}
		wg.Wait()
	}()
	for {
		cs, err := l.Lister.Containers(ctx)
		if err != nil && ctx.Err() == nil {
			return err
		}
		seen := map[string]bool{}
		for _, c := range cs {
			seen[c.ID] = true
			if _, ok := following[c.ID]; ok {
				continue
			}
			fctx, stop := context.WithCancel(ctx)
			following[c.ID] = stop
			wg.Go(func() { l.follow(fctx, c, since) })
		}
		for id, stop := range following {
			if !seen[id] {
				stop()
				delete(following, id)
			}
		}
		t := time.NewTimer(l.Poll)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil
		case <-t.C:
		}
	}
}

type cursor struct {
	mu    sync.Mutex
	last  time.Time
	floor time.Time
	again bool
}

func (l *Logs) follow(ctx context.Context, c dockerport.Container, since time.Time) {
	cur := &cursor{last: since}
	for ctx.Err() == nil {
		cur.mu.Lock()
		from := cur.last
		cur.floor = from
		cur.mu.Unlock()
		emit := func(line string) { l.line(c.Service, line, cur) }
		out, errs := &lineWriter{emit: emit, max: l.MaxLine}, &lineWriter{emit: emit, max: l.MaxLine}
		args := []string{"logs", "--follow", "--timestamps", "--since=" + from.Format(time.RFC3339Nano), c.ID}
		_, _ = l.Docker.Run(ctx, compose.Command{Args: args, Stdout: out, Stderr: errs})
		out.flush()
		errs.flush()
		cur.mu.Lock()
		cur.again = true
		cur.mu.Unlock()
		t := time.NewTimer(l.Poll)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

func (l *Logs) line(service, raw string, cur *cursor) {
	stamp, text, ok := strings.Cut(raw, " ")
	at, err := time.Parse(time.RFC3339Nano, stamp)
	if !ok || err != nil {
		at, text = l.Now().UTC(), raw
	}
	cur.mu.Lock()
	if cur.again && !at.After(cur.floor) {
		cur.mu.Unlock()
		return
	}
	if at.After(cur.last) {
		cur.last = at
	}
	cur.mu.Unlock()
	text = clip(l.Mask(text), l.MaxLine)
	_, _ = l.Emit.Append(contract.Signal{
		TS: at.UTC().Format(time.RFC3339Nano), TSSource: contract.TimeSourceContainer, Kind: contract.SignalKindEvent,
		Collector: signal.LogCollector, Attrs: contract.Attrs{signal.ContainerAttr: service}, Event: &contract.EventBody{Body: text},
	})
}

func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

type lineWriter struct {
	mu   sync.Mutex
	buf  []byte
	max  int
	emit func(string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(strings.TrimRight(string(w.buf[:i]), "\r"))
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) > 2*w.max+64 {
		w.emit(string(w.buf))
		w.buf = nil
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.emit(string(w.buf))
		w.buf = nil
	}
}
