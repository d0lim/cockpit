package pipeline

import (
	"errors"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"
)

var readerPassword = regexp.MustCompile(`PASSWORD '((?:[^']|'')*)'`)

type fakePostgres struct {
	mu      sync.Mutex
	roots   []string
	readers []string
}

func startFakePostgres(t *testing.T, port int) *fakePostgres {
	t.Helper()
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("the fake database cannot listen on %d: %v", port, err)
	}
	t.Cleanup(func() { l.Close() })
	f := &fakePostgres{}
	go func() {
		for {
			c, err := l.Accept()
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if err == nil {
				go f.serve(c)
			}
		}
	}()
	return f
}

func (f *fakePostgres) seen() ([]string, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.roots), slices.Clone(f.readers)
}

func (f *fakePostgres) serve(c net.Conn) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(10 * time.Second))
	b := pgproto3.NewBackend(c, c)
	if _, err := b.ReceiveStartupMessage(); err != nil {
		return
	}
	b.Send(&pgproto3.AuthenticationCleartextPassword{})
	if b.Flush() != nil || b.SetAuthType(pgproto3.AuthTypeCleartextPassword) != nil {
		return
	}
	msg, err := b.Receive()
	pw, ok := msg.(*pgproto3.PasswordMessage)
	if err != nil || !ok {
		return
	}
	root := pw.Password
	f.mu.Lock()
	f.roots = append(f.roots, root)
	f.mu.Unlock()
	b.Send(&pgproto3.AuthenticationOk{})
	b.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	if b.Flush() != nil {
		return
	}
	for {
		msg, err := b.Receive()
		if err != nil {
			return
		}
		q, ok := msg.(*pgproto3.Query)
		if !ok {
			return
		}
		if m := readerPassword.FindStringSubmatch(q.String); m != nil {
			f.mu.Lock()
			f.readers = append(f.readers, strings.ReplaceAll(m[1], "''", "'"))
			f.mu.Unlock()
			b.Send(&pgproto3.ErrorResponse{Severity: "ERROR", Code: "42501", Message: "refused " + q.String + " for root " + root})
		} else {
			b.Send(&pgproto3.CommandComplete{CommandTag: []byte("DO")})
		}
		b.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
		if b.Flush() != nil {
			return
		}
	}
}
