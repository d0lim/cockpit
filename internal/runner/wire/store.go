package recorder

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/wire"
)

type finished struct {
	gen      uint64
	exchange wire.Exchange
	addr     string
	reserved int64
}

type line struct {
	seq  int64
	text []byte
}

type record struct {
	lines [][]byte
	end   wire.End
}

type store struct {
	limits  Limits
	started time.Time
	masker  *mask.Masker
	names   *callerNames
	pending atomic.Int64
	queue   chan finished

	mu        sync.Mutex
	take      string
	gen       uint64
	recording bool
	seq       int64
	open      map[int64]*call
	queued    int
	lastStart time.Time
	lines     []line
	lineBytes int64
	omitted   int
	dropped   int
	frozen    *record
}

func newStore(limits Limits, masker *mask.Masker, names *callerNames) *store {
	s := &store{limits: limits, started: time.Now(), masker: masker, names: names, queue: make(chan finished, limits.Queue)}
	s.reset(InitialTake)
	return s
}

func (s *store) reset(take string) {
	s.take = take
	s.gen++
	s.recording = true
	s.seq = 0
	s.open = map[int64]*call{}
	s.queued = 0
	s.lastStart = time.Time{}
	s.lines = nil
	s.lineBytes = 0
	s.omitted = 0
	s.dropped = 0
	s.frozen = nil
}

func (s *store) arm(take string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reset(take)
}

func (s *store) reserve(n int64) bool {
	if s.pending.Add(n) > s.limits.Pending {
		s.pending.Add(-n)
		return false
	}
	return true
}

func (s *store) release(n int64) { s.pending.Add(-n) }

func remoteIP(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return remote
	}
	return host
}

func (s *store) begin(route Route, r *http.Request) *call {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.recording {
		return nil
	}
	headers := r.Header.Clone()
	headers["Host"] = []string{r.Host}
	s.seq++
	c := &call{
		store: s, gen: s.gen, seq: s.seq, addr: remoteIP(r.RemoteAddr), route: route,
		method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, started: time.Now(),
		headers: headers, callerUp: r.Context().Err,
		request: side{declared: r.ContentLength}, response: side{declared: -1},
	}
	s.open[c.seq] = c
	s.lastStart = c.started
	s.names.note(c.addr)
	return c
}

func (s *store) finish(c *call) {
	ex, reserved := c.close()
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.gen != s.gen || !s.recording || s.open[c.seq] != c {
		s.release(reserved)
		return
	}
	delete(s.open, c.seq)
	select {
	case s.queue <- finished{gen: c.gen, exchange: ex, addr: c.addr, reserved: reserved}:
		s.queued++
	default:
		s.dropped++
		s.release(reserved)
	}
}

func (s *store) work(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case f := <-s.queue:
			s.clean(ctx, f)
		}
	}
}

func (s *store) clean(ctx context.Context, f finished) {
	if s.current(f.gen) {
		s.add(ctx, f.gen, f.exchange, f.addr)
	}
	s.release(f.reserved)
	s.mu.Lock()
	defer s.mu.Unlock()
	if f.gen == s.gen {
		s.queued--
	}
}

func (s *store) current(gen uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return gen == s.gen
}

func (s *store) usage() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lineBytes
}

func withoutBodies(ex wire.Exchange) wire.Exchange {
	for _, m := range []*wire.RawMessage{&ex.Request, &ex.Response} {
		if len(m.Body) > 0 {
			m.Body, m.Omitted = nil, wire.OmitRecordFull
		}
	}
	return ex
}

func (s *store) encode(ex wire.Exchange) ([]byte, wire.Call, error) {
	c := wire.Capture(s.masker, ex)
	text, err := json.Marshal(wire.Line{Call: &c})
	return text, c, err
}

func (s *store) add(ctx context.Context, gen uint64, ex wire.Exchange, addr string) {
	ex.Caller = s.names.name(ctx, addr)
	used := s.usage()
	full := used >= s.limits.WithBodies
	if full {
		ex = withoutBodies(ex)
	}
	text, c, err := s.encode(ex)
	if err == nil && !full && used+int64(len(text)) > s.limits.WithBodies {
		text, c, err = s.encode(withoutBodies(ex))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case gen != s.gen:
	case err != nil || s.lineBytes+int64(len(text)) > s.limits.Lines:
		s.dropped++
	default:
		s.lines = append(s.lines, line{seq: c.Seq, text: text})
		s.lineBytes += int64(len(text))
		s.omitted += omittedBodies(c)
	}
}

func omittedBodies(c wire.Call) int {
	n := 0
	for _, b := range []wire.Body{c.Request.Body, c.Response.Body} {
		if b.Kind == wire.BodyOmitted {
			n++
		}
	}
	return n
}
