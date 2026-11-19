package recorder

import (
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/d0lim/cockpit/internal/wire"
)

type side struct {
	body     []byte
	size     int64
	declared int64
	ended    bool
	tooLarge bool
}

type call struct {
	store    *store
	gen      uint64
	seq      int64
	addr     string
	route    Route
	method   string
	path     string
	query    string
	started  time.Time
	headers  map[string][]string
	callerUp func() error

	mu              sync.Mutex
	request         side
	response        side
	responseHeaders map[string][]string
	status          int
	busy            bool
	closed          bool
	reserved        int64
	failure         wire.Outcome
	callerGone      bool
	served          bool
	lastWrite       time.Time
}

type tee struct {
	io.ReadCloser
	call *call
	side *side
}

func (t *tee) Read(p []byte) (int, error) {
	n, err := t.ReadCloser.Read(p)
	if n > 0 {
		t.call.add(t.side, p[:n])
	}
	if err == io.EOF {
		t.call.ended(t.side)
	}
	return n, err
}

func (c *call) add(s *side, p []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s.size += int64(len(p))
	switch {
	case c.closed || c.busy || s.tooLarge:
	case s.size > wire.MaxBody:
		s.tooLarge = true
		c.drop(s)
	case !c.store.reserve(int64(len(p))):
		c.busy = true
		c.drop(&c.request)
		c.drop(&c.response)
	default:
		c.reserved += int64(len(p))
		s.body = append(s.body, p...)
	}
}

func (c *call) drop(s *side) {
	c.store.release(int64(len(s.body)))
	c.reserved -= int64(len(s.body))
	s.body = nil
}

func (c *call) ended(s *side) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s.ended = true
}

func (c *call) responded(res *http.Response) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = res.StatusCode
	c.responseHeaders = res.Header.Clone()
	c.response.declared = res.ContentLength
}

func (c *call) failed(outcome wire.Outcome) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failure = outcome
}

func (c *call) markServed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.served = true
}

func (c *call) markCallerGone() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callerGone = true
}

func (c *call) writing() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastWrite = time.Now()
}

func (c *call) outcome() wire.Outcome {
	switch {
	case c.failure != "":
		return c.failure
	case c.served:
		return wire.OutcomeAnswered
	case c.callerGone || c.callerUp() != nil:
		return wire.OutcomeCallerGone
	}
	return wire.OutcomeIncomplete
}

func (c *call) message(s *side, headers map[string][]string) wire.RawMessage {
	m := wire.RawMessage{Headers: headers, Size: s.size}
	switch {
	case s.size == 0:
	case s.tooLarge:
		m.Omitted = wire.OmitTooLarge
	case c.busy:
		m.Omitted = wire.OmitBusy
	case s.ended || s.size == s.declared:
		m.Body = s.body
	}
	return m
}

func (c *call) exchange() wire.Exchange {
	return wire.Exchange{
		Seq:      c.seq,
		Caller:   c.addr,
		Target:   c.route.Service,
		Port:     c.route.Port,
		Method:   c.method,
		Path:     c.path,
		RawQuery: c.query,
		Started:  c.started,
		Replied:  c.lastWrite,
		Status:   c.status,
		Request:  c.message(&c.request, c.headers),
		Response: c.message(&c.response, c.responseHeaders),
	}
}

func (c *call) open() wire.Exchange {
	c.mu.Lock()
	defer c.mu.Unlock()
	ex := c.exchange()
	ex.Outcome = wire.OutcomeIncomplete
	return ex
}

func (c *call) close() (wire.Exchange, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	ex := c.exchange()
	ex.Outcome = c.outcome()
	ex.Finished = time.Now()
	return ex, c.reserved
}
