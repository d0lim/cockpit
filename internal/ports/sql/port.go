package sqlport

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports"
)

const (
	defaultMaxRows  = 1000
	defaultMaxBytes = 1 << 20
	defaultTimeout  = 10 * time.Second
	poolConns       = 3
)

type Target struct {
	Host     string
	Port     int
	RootUser string
	Database string
}

type Options struct {
	Databases []contract.Database
	Targets   map[string]Target
	Secret    func(name string) (string, bool)
	Mask      ports.Mask
	MaxRows   int
	MaxBytes  int
	Timeout   time.Duration
}

type Request struct {
	DB        string `json:"db"`
	Statement string `json:"statement"`
	MaxRows   int    `json:"maxRows,omitempty"`
	MaxBytes  int    `json:"maxBytes,omitempty"`
	TimeoutMs int64  `json:"timeoutMs,omitempty"`
}

type Result struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Truncated bool     `json:"truncated"`
}

type limits struct {
	rows    int
	bytes   int
	timeout time.Duration
}

type backend interface {
	provision(ctx context.Context, rootPassword, password string) error
	open(password string) *sql.DB
	classify(err error) (engine.PortCode, string)
	authFailure(err error) bool
}

type entry struct {
	decl    contract.Database
	backend backend
	mu      sync.Mutex
	pool    *sql.DB
	secrets []string
}

type Port struct {
	dbs      map[string]*entry
	secret   func(string) (string, bool)
	mask     ports.Mask
	max      limits
	password func() (string, error)
}

func New(opts Options) (*Port, error) {
	clearPGEnv()
	return build(opts, func(d contract.Database, t Target, l limits) backend {
		if d.Driver == contract.DriverPostgres {
			return pgBackend{decl: d, target: t, max: l}
		}
		return mysqlBackend{decl: d, target: t, max: l}
	})
}

func build(opts Options, newBackend func(contract.Database, Target, limits) backend) (*Port, error) {
	p := &Port{dbs: map[string]*entry{}, secret: opts.Secret, mask: opts.Mask, password: newPassword,
		max: limits{rows: opts.MaxRows, bytes: opts.MaxBytes, timeout: opts.Timeout}}
	if p.mask == nil {
		p.mask = ports.Identity
	}
	if p.secret == nil {
		p.secret = func(string) (string, bool) { return "", false }
	}
	if p.max.rows <= 0 {
		p.max.rows = defaultMaxRows
	}
	if p.max.bytes <= 0 {
		p.max.bytes = defaultMaxBytes
	}
	if p.max.timeout <= 0 {
		p.max.timeout = defaultTimeout
	}
	for _, d := range opts.Databases {
		t, ok := opts.Targets[string(d.Name)]
		if !ok {
			return nil, fmt.Errorf("database %q has no target in this run", d.Name)
		}
		if err := validate(d, &t); err != nil {
			return nil, err
		}
		p.dbs[string(d.Name)] = &entry{decl: d, backend: newBackend(d, t, p.max)}
	}
	return p, nil
}

func validate(d contract.Database, t *Target) error {
	if d.Driver != contract.DriverMySQL && d.Driver != contract.DriverPostgres {
		return fmt.Errorf("database %q has an unsupported driver %q", d.Name, d.Driver)
	}
	if len(d.ReadSchemas) == 0 || d.RootSecret == "" || t.Host == "" || t.Port <= 0 {
		return fmt.Errorf("database %q needs read schemas, a root secret name, and a target host and port", d.Name)
	}
	if err := checkIdentifiers("read schema", d.ReadSchemas...); err != nil {
		return err
	}
	if d.OwnerRole != "" {
		if err := checkIdentifiers("owner role", d.OwnerRole); err != nil {
			return err
		}
	}
	defaults := map[contract.DBDriver][2]string{contract.DriverMySQL: {"root", ""}, contract.DriverPostgres: {"postgres", "postgres"}}[d.Driver]
	if t.RootUser == "" {
		t.RootUser = defaults[0]
	}
	if t.Database == "" {
		t.Database = defaults[1]
	}
	return nil
}

func (p *Port) Query(ctx context.Context, req Request) (Result, error) {
	e := p.dbs[req.DB]
	if e == nil {
		return Result{}, ports.Denied(p.mask, fmt.Sprintf("database %q is not declared for this run", req.DB))
	}
	stmt, serr := checkStatement(e.decl.Driver, req.Statement)
	if serr != nil {
		if serr.denied {
			return Result{}, ports.Denied(p.mask, serr.reason+" — db "+req.DB)
		}
		return Result{}, ports.BadArgs(p.mask, serr.reason+" — db "+req.DB)
	}
	l := p.limitsFor(req)
	qctx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()
	for attempt := 0; ; attempt++ {
		pool, err := p.ensure(qctx, e)
		if err != nil {
			return Result{}, err
		}
		res, err := run(qctx, pool, stmt, l, e.scrub(p.mask))
		if err == nil {
			return res, nil
		}
		if attempt == 0 && e.backend.authFailure(err) {
			e.reset(pool)
			continue
		}
		return Result{}, p.failure(qctx, e, e.scrub(p.mask), err, l)
	}
}

func (p *Port) limitsFor(req Request) limits {
	l := p.max
	if req.MaxRows > 0 && req.MaxRows < l.rows {
		l.rows = req.MaxRows
	}
	if req.MaxBytes > 0 && req.MaxBytes < l.bytes {
		l.bytes = req.MaxBytes
	}
	if t := time.Duration(req.TimeoutMs) * time.Millisecond; t > 0 && t < l.timeout {
		l.timeout = t
	}
	return l
}

func (p *Port) ensure(ctx context.Context, e *entry) (*sql.DB, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pool != nil {
		return e.pool, nil
	}
	name := string(e.decl.Name)
	root, ok := p.secret(e.decl.RootSecret)
	if !ok || root == "" {
		return nil, ports.Unavailable(p.mask, fmt.Sprintf("root secret %q for db %s is not available", e.decl.RootSecret, name))
	}
	password, err := p.password()
	if err != nil {
		return nil, ports.Unavailable(p.mask, "failed to create a reader password — db "+name)
	}
	e.secrets = []string{root, password}
	if err := e.backend.provision(ctx, root, password); err != nil {
		return nil, p.failure(ctx, e, scrubber(e.secrets, p.mask), err, p.max)
	}
	e.pool = e.backend.open(password)
	e.pool.SetMaxOpenConns(poolConns)
	e.pool.SetMaxIdleConns(poolConns)
	return e.pool, nil
}

func (e *entry) reset(pool *sql.DB) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pool == pool {
		e.pool = nil
	}
	_ = pool.Close()
}

func (e *entry) scrub(mask ports.Mask) ports.Mask {
	e.mu.Lock()
	defer e.mu.Unlock()
	return scrubber(e.secrets, mask)
}

func scrubber(values []string, mask ports.Mask) ports.Mask {
	secrets := append([]string(nil), values...)
	return func(s string) string {
		for _, v := range secrets {
			if v != "" {
				s = strings.ReplaceAll(s, v, "****")
			}
		}
		return mask(s)
	}
}

func (p *Port) failure(ctx context.Context, e *entry, mask ports.Mask, err error, l limits) error {
	name := string(e.decl.Name)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ports.Timeout(mask, fmt.Sprintf("query exceeded the %dms time limit — db %s", l.timeout.Milliseconds(), name))
	}
	code, reason := e.backend.classify(err)
	return &engine.PortError{Code: code, Message: mask(reason + " — db " + name)}
}

func (p *Port) Close() error {
	var errs []error
	for _, e := range p.dbs {
		e.mu.Lock()
		if e.pool != nil {
			errs = append(errs, e.pool.Close())
			e.pool = nil
		}
		e.mu.Unlock()
	}
	return errors.Join(errs...)
}
