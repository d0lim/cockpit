package recorder

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/wire"
)

const lateReadSettle = 200 * time.Millisecond

type heldConn struct {
	net.Conn
	body    int
	head    []byte
	inBody  bool
	left    int
	held    chan struct{}
	release chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (c *heldConn) bodyWritten(p []byte) bool {
	if c.inBody {
		c.left -= len(p)
		return c.left == 0
	}
	c.head = append(c.head, p...)
	end := bytes.Index(c.head, []byte("\r\n\r\n"))
	if end < 0 {
		return false
	}
	c.inBody, c.left = true, c.body-(len(c.head)-end-4)
	return c.left == 0
}

func (c *heldConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if c.bodyWritten(p[:n]) {
		close(c.held)
		<-c.release
	}
	return n, err
}

func (c *heldConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

func TestAResponseArrivesWholeWhenTheUpstreamAnswersBeforeTheRequestWriteReturns(t *testing.T) {
	first, rest := strings.Repeat("a", 8<<10), strings.Repeat("b", 8<<10)
	conn := &heldConn{body: 16 << 10, held: make(chan struct{}), release: make(chan struct{}), closed: make(chan struct{})}
	finish := make(chan struct{})
	var release, finishOnce sync.Once
	r := startRig(t, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		io.Copy(io.Discard, req.Body)
		w.Write([]byte(first))
		http.NewResponseController(w).Flush()
		<-finish
		w.Write([]byte(rest))
	}), func(o *Options) {
		o.Dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
			up, err := (&net.Dialer{}).DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			conn.Conn = up
			return conn, nil
		}
	})
	t.Cleanup(func() {
		release.Do(func() { close(conn.release) })
		finishOnce.Do(func() { close(finish) })
	})
	res, err := r.client.Post(r.url("/orders"), "text/plain", strings.NewReader(strings.Repeat("q", conn.body)))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	select {
	case <-conn.held:
	case <-time.After(5 * time.Second):
		t.Fatal("the upstream answered, and the write of the request's last bytes was never held")
	}
	release.Do(func() { close(conn.release) })
	select {
	case <-conn.closed:
	case <-time.After(lateReadSettle):
	}
	finishOnce.Do(func() { close(finish) })
	got, err := io.ReadAll(res.Body)
	if err != nil || len(got) != len(first)+len(rest) {
		t.Fatalf("the caller read %d of %d bytes: %v", len(got), len(first)+len(rest), err)
	}
	c := r.onlyCall()
	if c.Outcome != wire.OutcomeAnswered || c.Request.Size != int64(conn.body) || c.Response.Size != int64(len(got)) {
		t.Fatalf("call outcome %q request %d response %d", c.Outcome, c.Request.Size, c.Response.Size)
	}
}
