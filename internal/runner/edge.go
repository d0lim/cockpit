package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const upstreamDialTimeout = 5 * time.Second

type Route struct {
	Listen   string
	Upstream string
}

func ParseRoute(spec string) (Route, error) {
	port, upstream, ok := strings.Cut(spec, "=")
	if !ok {
		return Route{}, fmt.Errorf("edge entry %q must have the form <port>=<host>:<port>", spec)
	}
	if _, err := validPort(port); err != nil {
		return Route{}, fmt.Errorf("edge entry %q has a bad listen port: %w", spec, err)
	}
	host, uport, err := net.SplitHostPort(upstream)
	if err != nil || host == "" {
		return Route{}, fmt.Errorf("edge entry %q must name its upstream as <host>:<port>", spec)
	}
	if _, err := validPort(uport); err != nil {
		return Route{}, fmt.Errorf("edge entry %q has a bad upstream port: %w", spec, err)
	}
	return Route{Listen: ":" + port, Upstream: upstream}, nil
}

func validPort(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return 0, errors.New("not an integer between 1 and 65535")
	}
	return n, nil
}

type Proxy struct {
	ln       net.Listener
	upstream string
}

func (p *Proxy) Addr() net.Addr { return p.ln.Addr() }

func ListenAll(routes []Route) ([]*Proxy, error) {
	var proxies []*Proxy
	for _, r := range routes {
		ln, err := net.Listen("tcp", r.Listen)
		if err != nil {
			for _, p := range proxies {
				p.ln.Close()
			}
			return nil, fmt.Errorf("could not listen on %s: %w", r.Listen, err)
		}
		proxies = append(proxies, &Proxy{ln: ln, upstream: r.Upstream})
	}
	return proxies, nil
}

func ServeAll(ctx context.Context, proxies []*Proxy) error {
	for _, p := range proxies {
		go p.serve(ctx)
	}
	<-ctx.Done()
	for _, p := range proxies {
		p.ln.Close()
	}
	return ctx.Err()
}

func (p *Proxy) serve(ctx context.Context) {
	for {
		c, err := p.ln.Accept()
		if err != nil {
			return
		}
		go p.pipe(ctx, c)
	}
}

func (p *Proxy) pipe(ctx context.Context, c net.Conn) {
	defer c.Close()
	up, err := (&net.Dialer{Timeout: upstreamDialTimeout}).DialContext(ctx, "tcp", p.upstream)
	if err != nil {
		return
	}
	defer up.Close()
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(up, c)
		closeWrite(up)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(c, up)
		closeWrite(c)
		done <- struct{}{}
	}()
	<-done
	<-done
}

func closeWrite(c net.Conn) {
	if tc, ok := c.(*net.TCPConn); ok {
		tc.CloseWrite()
	}
}
