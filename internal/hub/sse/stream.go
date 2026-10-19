package sse

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/signal"
)

const (
	DefaultPing  = 15 * time.Second
	DefaultPoll  = 500 * time.Millisecond
	signalBatch  = 500
	LastEventID  = "Last-Event-ID"
	idSeparator  = "."
	pingFrame    = ": ping\n\n"
	contentType  = "text/event-stream"
	cacheControl = "no-cache"
)

type Signals interface {
	Read(runID string, req signal.ReadRequest) (signal.ReadResponse, error)
}

type Stream struct {
	Bus     *Bus
	Signals Signals
	Ping    time.Duration
	Poll    time.Duration
}

type Cursor struct {
	Epoch  string
	Bus    int64
	Signal int64
}

func (c Cursor) String() string {
	return c.Epoch + idSeparator + strconv.FormatInt(c.Bus, 10) + idSeparator + strconv.FormatInt(c.Signal, 10)
}

func ParseCursor(raw string) (Cursor, bool) {
	parts := strings.Split(raw, idSeparator)
	if len(parts) != 3 || parts[0] == "" {
		return Cursor{}, false
	}
	bus, err1 := strconv.ParseInt(parts[1], 10, 64)
	sig, err2 := strconv.ParseInt(parts[2], 10, 64)
	if err1 != nil || err2 != nil || bus < 0 || sig < 0 {
		return Cursor{}, false
	}
	return Cursor{Epoch: parts[0], Bus: bus, Signal: sig}, true
}

func (s *Stream) start(r *http.Request) Cursor {
	c, ok := ParseCursor(r.Header.Get(LastEventID))
	if !ok || c.Epoch != s.Bus.Epoch() {
		return Cursor{Epoch: s.Bus.Epoch()}
	}
	return c
}

type writer struct {
	w   io.Writer
	rc  *http.ResponseController
	cur Cursor
	err error
}

func (w *writer) frame(kind Kind, data []byte) {
	if w.err != nil {
		return
	}
	_, w.err = fmt.Fprintf(w.w, "id: %s\nevent: %s\ndata: %s\n\n", w.cur, kind, data)
}

func (w *writer) raw(text string) {
	if w.err == nil {
		_, w.err = io.WriteString(w.w, text)
	}
}

func (w *writer) flush() {
	if w.err == nil {
		w.err = w.rc.Flush()
	}
}

func (w *writer) event(e Event) {
	data, err := e.Data()
	if err != nil {
		w.err = err
		return
	}
	w.cur.Bus = e.Seq
	w.frame(e.Kind, data)
}

func (s *Stream) Serve(w http.ResponseWriter, r *http.Request, target string, runTarget bool) {
	cur := s.start(r)
	replay, events, cancel := s.Bus.Subscribe(target, cur.Bus)
	defer cancel()
	h := w.Header()
	h.Set("Content-Type", contentType)
	h.Set("Cache-Control", cacheControl)
	w.WriteHeader(http.StatusOK)
	out := &writer{w: w, rc: http.NewResponseController(w), cur: cur}
	for _, e := range replay {
		out.event(e)
	}
	polling := runTarget && s.Signals != nil
	if polling {
		polling = s.pollSignals(out, target)
	}
	out.flush()
	ping := time.NewTicker(orDefault(s.Ping, DefaultPing))
	defer ping.Stop()
	poll := time.NewTicker(orDefault(s.Poll, DefaultPoll))
	defer poll.Stop()
	for out.err == nil {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-events:
			if !ok {
				return
			}
			out.event(e)
		case <-ping.C:
			out.raw(pingFrame)
		case <-poll.C:
			if polling {
				polling = s.pollSignals(out, target)
			}
		}
		out.flush()
	}
}

func orDefault(d, fallback time.Duration) time.Duration {
	if d <= 0 {
		return fallback
	}
	return d
}

func (s *Stream) pollSignals(out *writer, target string) bool {
	for out.err == nil {
		resp, err := s.Signals.Read(target, signal.ReadRequest{After: out.cur.Signal, Limit: signalBatch})
		if err != nil {
			reason := "signal reads failed"
			if errors.Is(err, signal.ErrReadRefused) {
				reason = "signal reads are refused for this run"
			}
			data, _ := json.Marshal(map[string]string{"target": target, "reason": reason})
			out.frame(KindRefused, data)
			return false
		}
		if resp.Resync != nil {
			data, _ := json.Marshal(map[string]any{"target": target, "from": resp.Resync.From, "to": resp.Resync.To})
			out.cur.Signal = resp.Resync.To
			out.frame(KindResync, data)
		}
		for _, sig := range resp.Signals {
			data, err := json.Marshal(sig)
			if err != nil {
				out.err = err
				return false
			}
			out.cur.Signal = sig.Seq
			out.frame(KindSignal, data)
		}
		out.cur.Signal = max(out.cur.Signal, resp.Next)
		if len(resp.Signals) < signalBatch {
			return true
		}
	}
	return false
}
