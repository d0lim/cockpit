package pipeline

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgproto3"
)

type livePostgres struct {
	mu       sync.Mutex
	conns    map[net.Conn]bool
	live     atomic.Int32
	executes atomic.Int32
}

func startLivePostgres(t *testing.T, port int) *livePostgres {
	t.Helper()
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("the live fake database cannot listen on %d: %v", port, err)
	}
	f := &livePostgres{conns: map[net.Conn]bool{}}
	t.Cleanup(func() {
		l.Close()
		f.mu.Lock()
		defer f.mu.Unlock()
		for c := range f.conns {
			c.Close()
		}
	})
	go func() {
		for {
			c, err := l.Accept()
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if err == nil {
				f.track(c, true)
				go f.serve(c)
			}
		}
	}()
	return f
}

func (f *livePostgres) track(c net.Conn, open bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if open {
		f.conns[c] = true
		f.live.Add(1)
		return
	}
	delete(f.conns, c)
	f.live.Add(-1)
}

func (f *livePostgres) idleWithin(wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for f.live.Load() != 0 {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	return true
}

func (f *livePostgres) serve(c net.Conn) {
	defer f.track(c, false)
	defer c.Close()
	b := pgproto3.NewBackend(c, c)
	if _, err := b.ReceiveStartupMessage(); err != nil {
		return
	}
	b.Send(&pgproto3.AuthenticationCleartextPassword{})
	if b.Flush() != nil || b.SetAuthType(pgproto3.AuthTypeCleartextPassword) != nil {
		return
	}
	if msg, err := b.Receive(); err != nil {
		return
	} else if _, ok := msg.(*pgproto3.PasswordMessage); !ok {
		return
	}
	b.Send(&pgproto3.AuthenticationOk{})
	b.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	if b.Flush() != nil {
		return
	}
	status := byte('I')
	row := &pgproto3.RowDescription{Fields: []pgproto3.FieldDescription{{Name: []byte("n"), DataTypeOID: 25, DataTypeSize: -1, TypeModifier: -1}}}
	for {
		msg, err := b.Receive()
		if err != nil {
			return
		}
		flush := false
		switch m := msg.(type) {
		case *pgproto3.Query:
			text := strings.ToLower(strings.TrimSpace(m.String))
			switch {
			case strings.HasPrefix(text, "begin"):
				status = 'T'
			case strings.HasPrefix(text, "rollback"), strings.HasPrefix(text, "commit"):
				status = 'I'
			}
			b.Send(&pgproto3.CommandComplete{CommandTag: []byte("DO")})
			b.Send(&pgproto3.ReadyForQuery{TxStatus: status})
			flush = true
		case *pgproto3.Parse:
			b.Send(&pgproto3.ParseComplete{})
		case *pgproto3.Describe:
			if m.ObjectType == 'S' {
				b.Send(&pgproto3.ParameterDescription{})
			}
			b.Send(row)
		case *pgproto3.Bind:
			b.Send(&pgproto3.BindComplete{})
		case *pgproto3.Execute:
			f.executes.Add(1)
			b.Send(&pgproto3.DataRow{Values: [][]byte{[]byte("1")}})
			b.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})
		case *pgproto3.Close:
			b.Send(&pgproto3.CloseComplete{})
		case *pgproto3.Flush:
			flush = true
		case *pgproto3.Sync:
			b.Send(&pgproto3.ReadyForQuery{TxStatus: status})
			flush = true
		default:
			return
		}
		if flush && b.Flush() != nil {
			return
		}
	}
}
