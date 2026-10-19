package sqlport

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
)

const (
	planted     = "PLANTED-root-4c1e"
	plantedPass = "PLANTED-reader-9b2d"
)

var errFakeAuth = errors.New("fake auth failure")

type fakeState struct {
	mu         sync.Mutex
	columns    []string
	types      []string
	rows       [][]driver.Value
	queryErr   func(attempt int) error
	attempts   int
	statements []string
	args       [][]any
	readOnly   []bool
	rollbacks  int
	commits    int
	block      bool
}

type fakeBackend struct {
	state        *fakeState
	provisions   atomic.Int32
	passwords    []string
	provisionErr error
	mu           sync.Mutex
	slow         time.Duration
}

func (b *fakeBackend) provision(ctx context.Context, root, password string) error {
	b.provisions.Add(1)
	time.Sleep(b.slow)
	b.mu.Lock()
	b.passwords = append(b.passwords, password)
	b.mu.Unlock()
	if b.provisionErr != nil {
		return errors.New(b.provisionErr.Error() + " root=" + root + " password=" + password)
	}
	return nil
}

func (b *fakeBackend) open(string) *sql.DB { return sql.OpenDB(fakeConnector{b.state}) }

func (b *fakeBackend) classify(err error) (engine.PortCode, string) {
	return engine.PortUnavailable, "fake failure: " + err.Error()
}

func (b *fakeBackend) authFailure(err error) bool { return errors.Is(err, errFakeAuth) }

type fakeConnector struct{ s *fakeState }

func (c fakeConnector) Connect(context.Context) (driver.Conn, error) { return &fakeConn{c.s}, nil }

func (c fakeConnector) Driver() driver.Driver { return nil }

type fakeConn struct{ s *fakeState }

func (c *fakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare unsupported")
}

func (c *fakeConn) Close() error { return nil }

func (c *fakeConn) Begin() (driver.Tx, error) { return nil, errors.New("use BeginTx") }

func (c *fakeConn) BeginTx(_ context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	c.s.readOnly = append(c.s.readOnly, opts.ReadOnly)
	return fakeTx{c.s}, nil
}

func (c *fakeConn) QueryContext(ctx context.Context, query string, named []driver.NamedValue) (driver.Rows, error) {
	values := make([]any, len(named))
	for i, n := range named {
		values[i] = n.Value
	}
	c.s.mu.Lock()
	c.s.statements = append(c.s.statements, query)
	c.s.args = append(c.s.args, values)
	attempt := c.s.attempts
	c.s.attempts++
	block := c.s.block
	c.s.mu.Unlock()
	if block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if c.s.queryErr != nil {
		if err := c.s.queryErr(attempt); err != nil {
			return nil, err
		}
	}
	return &fakeRows{s: c.s}, nil
}

type fakeTx struct{ s *fakeState }

func (t fakeTx) Commit() error {
	t.s.mu.Lock()
	t.s.commits++
	t.s.mu.Unlock()
	return nil
}

func (t fakeTx) Rollback() error {
	t.s.mu.Lock()
	t.s.rollbacks++
	t.s.mu.Unlock()
	return nil
}

type fakeRows struct {
	s *fakeState
	i int
}

func (r *fakeRows) Columns() []string { return r.s.columns }

func (r *fakeRows) Close() error { return nil }

func (r *fakeRows) ColumnTypeDatabaseTypeName(i int) string { return r.s.types[i] }

func (r *fakeRows) Next(dest []driver.Value) error {
	if r.i >= len(r.s.rows) {
		return io.EOF
	}
	copy(dest, r.s.rows[r.i])
	r.i++
	return nil
}

func fakePort(t *testing.T, state *fakeState, opts Options) (*Port, *fakeBackend) {
	t.Helper()
	fb := &fakeBackend{state: state}
	if opts.Databases == nil {
		opts.Databases = []contract.Database{{Name: "main", Driver: contract.DriverMySQL, ReadSchemas: []string{"app"}, RootSecret: "ROOT_PASSWORD"}}
		opts.Targets = map[string]Target{"main": {Host: "127.0.0.1", Port: 3306}}
	}
	if opts.Secret == nil {
		opts.Secret = func(name string) (string, bool) { return planted, name == "ROOT_PASSWORD" }
	}
	if opts.Mask == nil {
		opts.Mask = func(s string) string { return strings.ReplaceAll(s, "PLANTED-run", "****") }
	}
	p, err := build(opts, func(contract.Database, Target, limits) backend { return fb })
	if err != nil {
		t.Fatal(err)
	}
	p.password = func() (string, error) { return plantedPass, nil }
	t.Cleanup(func() { _ = p.Close() })
	return p, fb
}

func itemRows(n int) *fakeState {
	s := &fakeState{columns: []string{"id", "name"}, types: []string{"BIGINT", "VARCHAR"}}
	for i := range n {
		s.rows = append(s.rows, []driver.Value{int64(i + 1), []byte("item")})
	}
	return s
}

func codeOf(err error) engine.PortCode {
	var pe *engine.PortError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}
