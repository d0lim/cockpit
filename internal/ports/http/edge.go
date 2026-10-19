package httpport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/d0lim/cockpit/internal/ports"
)

const edgeHost = "127.0.0.1"

var edgeName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

type EdgeOptions struct {
	Ports   map[string]int
	Timeout time.Duration
	MaxBody int64
	Mask    ports.Mask
}

type EdgeRequest struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	TimeoutMs int64             `json:"timeoutMs,omitempty"`
}

type EdgeClient struct {
	ports   map[string]int
	allowed map[string]bool
	timeout time.Duration
	maxBody int64
	mask    ports.Mask
	client  *http.Client
}

func NewEdge(opts EdgeOptions) (*EdgeClient, error) {
	c := &EdgeClient{ports: map[string]int{}, allowed: map[string]bool{}, timeout: opts.Timeout, maxBody: opts.MaxBody, mask: opts.Mask}
	if c.mask == nil {
		c.mask = ports.Identity
	}
	if c.timeout <= 0 {
		c.timeout = 10 * time.Second
	}
	if c.maxBody <= 0 {
		c.maxBody = 8 << 20
	}
	for name, port := range opts.Ports {
		if !edgeName.MatchString(name) || port < 1 || port > 65535 {
			return nil, fmt.Errorf("edge %q with port %d is not a valid edge port", name, port)
		}
		c.ports[name] = port
		c.allowed[net.JoinHostPort(edgeHost, strconv.Itoa(port))] = true
	}
	dialer := &net.Dialer{Timeout: c.timeout, Control: c.control}
	c.client = &http.Client{
		Transport:     &http.Transport{Proxy: nil, DialContext: dialer.DialContext, DisableCompression: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return c, nil
}

func (c *EdgeClient) control(_, address string, _ syscall.RawConn) error {
	if !c.allowed[address] {
		return fmt.Errorf("dial to %s is outside the run's edge ports", address)
	}
	return nil
}

func (c *EdgeClient) resolve(method, raw string) (*url.URL, *url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, nil, ports.Denied(c.mask, "not an absolute edge URL — "+method)
	}
	at := target(method, u)
	if method != http.MethodGet && method != http.MethodHead {
		return nil, nil, ports.Denied(c.mask, "only GET and HEAD reach edge ports — "+at)
	}
	name := strings.ToLower(u.Hostname())
	port, ok := c.ports[name]
	if u.Scheme != "http" || u.User != nil || u.Port() != "" || !ok {
		return nil, nil, ports.Denied(c.mask, "not an edge of this run — "+at+": use http://<edge name>/path with an edge name from the project declaration")
	}
	dial := *u
	dial.Host = net.JoinHostPort(edgeHost, strconv.Itoa(port))
	return u, &dial, nil
}

func (c *EdgeClient) Send(ctx context.Context, req EdgeRequest) (Response, error) {
	u, dial, err := c.resolve(req.Method, req.URL)
	if err != nil {
		return Response{}, err
	}
	at := target(req.Method, u)
	timeout := c.timeout
	if req.TimeoutMs > 0 {
		timeout = min(time.Duration(req.TimeoutMs)*time.Millisecond, c.timeout)
	}
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	hreq, err := http.NewRequestWithContext(hctx, req.Method, dial.String(), nil)
	if err != nil {
		return Response{}, ports.BadArgs(c.mask, "failed to build the request — "+at)
	}
	for k, v := range req.Headers {
		hreq.Header.Set(k, v)
	}
	res, err := c.client.Do(hreq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(hctx.Err(), context.DeadlineExceeded) {
			return Response{}, ports.Timeout(c.mask, fmt.Sprintf("request exceeded the %dms time limit — %s", timeout.Milliseconds(), at))
		}
		return Response{}, ports.Unavailable(c.mask, "failed to reach the edge — "+at)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 && res.StatusCode < 400 && res.Header.Get("Location") != "" {
		return Response{}, ports.Denied(c.mask, fmt.Sprintf("got a redirect response (%d) that is not followed — %s", res.StatusCode, at))
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, c.maxBody+1))
	if err != nil {
		return Response{}, ports.Unavailable(c.mask, "failed to read the edge response — "+at)
	}
	if int64(len(data)) > c.maxBody {
		return Response{}, ports.Limit(c.mask, "response body exceeds the limit — "+at)
	}
	hdrs := map[string]string{}
	for name, values := range res.Header {
		hdrs[strings.ToLower(name)] = strings.Join(values, ", ")
	}
	return Response{Method: req.Method, URL: req.URL, Status: res.StatusCode, Headers: hdrs, Body: string(data)}, nil
}

func (c *EdgeClient) Call(ctx context.Context, method string, args []json.RawMessage) (any, error) {
	if method != "send" {
		return nil, ports.UnknownMethod(c.mask, ports.PortHTTP, method)
	}
	var req EdgeRequest
	if err := ports.Arg(args, 0, &req.Method, c.mask, "method"); err != nil {
		return nil, err
	}
	if err := ports.Arg(args, 1, &req.URL, c.mask, "url"); err != nil {
		return nil, err
	}
	var opts struct {
		Headers   map[string]string `json:"headers"`
		TimeoutMs int64             `json:"timeoutMs"`
	}
	if err := ports.OptionalArg(args, 2, &opts, c.mask, "options"); err != nil {
		return nil, err
	}
	req.Headers, req.TimeoutMs = opts.Headers, opts.TimeoutMs
	return c.Send(ctx, req)
}
