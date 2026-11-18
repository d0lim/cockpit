package recorder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/wire"
)

const (
	ControlPort   = 41000
	MemoryLimit   = 256 << 20
	InitialTake   = wire.SuiteTake
	shutdownGrace = 5 * time.Second
)

type Route struct {
	Listen  int
	Service string
	Port    int
}

func (r Route) upstream() string {
	return net.JoinHostPort(r.Service, strconv.Itoa(r.Port))
}

func ParseRoute(spec string) (Route, error) {
	listen, upstream, ok := strings.Cut(spec, "=")
	if !ok {
		return Route{}, fmt.Errorf("wire route %q must have the form <listen port>=<service>:<port>", spec)
	}
	service, port, err := net.SplitHostPort(upstream)
	if err != nil || service == "" {
		return Route{}, fmt.Errorf("wire route %q must name its upstream as <service>:<port>", spec)
	}
	r := Route{Service: service}
	if r.Listen, err = portNumber(listen); err != nil {
		return Route{}, fmt.Errorf("wire route %q has a bad listen port", spec)
	}
	if r.Port, err = portNumber(port); err != nil {
		return Route{}, fmt.Errorf("wire route %q has a bad upstream port", spec)
	}
	return r, nil
}

func portNumber(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return 0, errors.New("not an integer between 1 and 65535")
	}
	return n, nil
}

type Limits struct {
	Pending    int64
	WithBodies int64
	Lines      int64
	Queue      int
	Quiet      time.Duration
	MaxWait    time.Duration
}

func DefaultLimits() Limits {
	return Limits{
		Pending:    64 << 20,
		WithBodies: 96 << 20,
		Lines:      128 << 20,
		Queue:      4096,
		Quiet:      2500 * time.Millisecond,
		MaxWait:    10 * time.Second,
	}
}

type Options struct {
	BindHost    string
	Project     string
	Routes      []Route
	Masker      *mask.Masker
	Log         *slog.Logger
	Limits      Limits
	ControlPort int
	LookupAddr  func(ctx context.Context, addr string) ([]string, error)
	Dial        func(ctx context.Context, network, addr string) (net.Conn, error)
}

type Recorder struct {
	log     *slog.Logger
	store   *store
	control net.Listener
	routes  []net.Listener
	servers []*http.Server
}

func Listen(opts Options) (*Recorder, error) {
	if opts.Limits == (Limits{}) {
		opts.Limits = DefaultLimits()
	}
	if opts.Log == nil {
		opts.Log = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	if opts.LookupAddr == nil {
		opts.LookupAddr = net.DefaultResolver.LookupAddr
	}
	r := &Recorder{log: opts.Log, store: newStore(opts.Limits, opts.Masker, newCallerNames(opts.Project, opts.LookupAddr))}
	transport := upstreamTransport(opts.Dial)
	discard := log.New(io.Discard, "", 0)
	for _, route := range opts.Routes {
		ln, err := net.Listen("tcp", net.JoinHostPort(opts.BindHost, strconv.Itoa(route.Listen)))
		if err != nil {
			r.closeListeners()
			return nil, fmt.Errorf("could not listen on wire port %d: %w", route.Listen, err)
		}
		r.routes = append(r.routes, ln)
		r.servers = append(r.servers, &http.Server{
			Handler:                      newProxy(route, r.store, transport, discard),
			ErrorLog:                     discard,
			DisableGeneralOptionsHandler: true,
		})
	}
	control, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(opts.ControlPort)))
	if err != nil {
		r.closeListeners()
		return nil, fmt.Errorf("could not listen on the control port: %w", err)
	}
	r.control = control
	return r, nil
}

func (r *Recorder) closeListeners() {
	for _, ln := range r.routes {
		ln.Close()
	}
}

func (r *Recorder) RouteAddr(i int) string { return r.routes[i].Addr().String() }

func (r *Recorder) ControlAddr() string { return r.control.Addr().String() }

func (r *Recorder) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go r.store.work(ctx)
	go r.serveControl(ctx)
	failed := make(chan error, len(r.servers))
	for i, srv := range r.servers {
		go func() { failed <- srv.Serve(r.routes[i]) }()
	}
	var err error
	select {
	case <-ctx.Done():
	case err = <-failed:
	}
	r.control.Close()
	r.shutdown()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (r *Recorder) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	done := make(chan struct{}, len(r.servers))
	for _, srv := range r.servers {
		go func() {
			if srv.Shutdown(ctx) != nil {
				srv.Close()
			}
			done <- struct{}{}
		}()
	}
	for range r.servers {
		<-done
	}
}
