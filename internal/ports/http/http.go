package httpport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/d0lim/cockpit/internal/ports"
)

type Dialer func(ctx context.Context, network, addr string) (net.Conn, error)

type Options struct {
	Allowed []string
	Timeout time.Duration
	MaxBody int64
	Mask    ports.Mask
	Dial    Dialer
}

type Request struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	JSON      any               `json:"json,omitempty"`
	Body      *string           `json:"body,omitempty"`
	TimeoutMs int64             `json:"timeoutMs,omitempty"`
}

type Response struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type Counter struct {
	Sent       int
	LastStatus int
}

type Client struct {
	allowed map[string]bool
	timeout time.Duration
	maxBody int64
	mask    ports.Mask
	client  *http.Client
	mu      sync.Mutex
	counter Counter
}

func New(opts Options) (*Client, error) {
	c := &Client{allowed: map[string]bool{}, timeout: opts.Timeout, maxBody: opts.MaxBody, mask: opts.Mask}
	if c.mask == nil {
		c.mask = ports.Identity
	}
	if c.timeout <= 0 {
		c.timeout = 10 * time.Second
	}
	if c.maxBody <= 0 {
		c.maxBody = 4 << 20
	}
	for _, origin := range opts.Allowed {
		u, err := url.Parse(origin)
		if err != nil {
			return nil, fmt.Errorf("허용 오리진 %q 을 읽지 못했다", origin)
		}
		key, err := originKey(u)
		if err != nil {
			return nil, fmt.Errorf("허용 오리진 %q: %w", origin, err)
		}
		c.allowed[key] = true
	}
	dial := opts.Dial
	if dial == nil {
		dial = (&net.Dialer{Timeout: c.timeout}).DialContext
	}
	c.client = &http.Client{
		Transport:     &http.Transport{Proxy: nil, DialContext: dial, DisableCompression: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return c, nil
}

func originKey(u *url.URL) (string, error) {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if scheme == "" || host == "" {
		return "", errors.New("스킴과 호스트가 있는 절대 URL 이 아니다")
	}
	if net.ParseIP(host) != nil {
		return "", errors.New("IP 주소는 서비스 이름이 아니다")
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[scheme]
	}
	return scheme + "://" + host + ":" + port, nil
}

func target(method string, u *url.URL) string {
	return method + " " + u.Path
}

func (c *Client) Counter() Counter {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counter
}

func (c *Client) ResetCounter() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counter = Counter{}
}

func (c *Client) sent() {
	c.mu.Lock()
	c.counter.Sent++
	c.mu.Unlock()
}

func (c *Client) status(code int) {
	c.mu.Lock()
	c.counter.LastStatus = code
	c.mu.Unlock()
}

func (c *Client) Send(ctx context.Context, req Request) (Response, error) {
	u, err := url.Parse(req.URL)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return Response{}, ports.Denied(c.mask, "절대 URL 이 아니다 — "+req.Method)
	}
	key, err := originKey(u)
	if err != nil || !c.allowed[key] {
		return Response{}, ports.Denied(c.mask, "허용되지 않은 주소 — "+target(req.Method, u))
	}
	timeout := c.timeout
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}
	headers := http.Header{}
	for k, v := range req.Headers {
		headers.Set(k, v)
	}
	var body io.Reader
	switch {
	case req.JSON != nil:
		data, err := json.Marshal(req.JSON)
		if err != nil {
			return Response{}, ports.BadArgs(c.mask, "json 옵션을 직렬화하지 못했다 — "+target(req.Method, u))
		}
		body = bytes.NewReader(data)
		if headers.Get("Content-Type") == "" {
			headers.Set("Content-Type", "application/json")
		}
	case req.Body != nil:
		body = strings.NewReader(*req.Body)
	}
	hctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	hreq, err := http.NewRequestWithContext(hctx, req.Method, u.String(), body)
	if err != nil {
		return Response{}, ports.BadArgs(c.mask, "요청을 만들지 못했다 — "+target(req.Method, u))
	}
	hreq.Header = headers
	c.sent()
	res, err := c.client.Do(hreq)
	if err != nil {
		return Response{}, c.failure(hctx, err, timeout, target(req.Method, u))
	}
	defer res.Body.Close()
	c.status(res.StatusCode)
	if res.StatusCode >= 300 && res.StatusCode < 400 && res.Header.Get("Location") != "" {
		return Response{}, ports.Denied(c.mask, fmt.Sprintf("리다이렉트 응답(%d)을 받았다 — %s", res.StatusCode, target(req.Method, u)))
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, c.maxBody+1))
	if err != nil {
		return Response{}, c.failure(hctx, err, timeout, target(req.Method, u))
	}
	if int64(len(data)) > c.maxBody {
		return Response{}, ports.Limit(c.mask, "응답 본문이 상한을 넘는다 — "+target(req.Method, u))
	}
	hdrs := map[string]string{}
	for name, values := range res.Header {
		hdrs[strings.ToLower(name)] = strings.Join(values, ", ")
	}
	return Response{Method: req.Method, URL: req.URL, Status: res.StatusCode, Headers: hdrs, Body: string(data)}, nil
}

func (c *Client) failure(ctx context.Context, err error, timeout time.Duration, at string) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ports.Timeout(c.mask, fmt.Sprintf("요청 시간 상한 %dms 를 넘겼다 — %s", timeout.Milliseconds(), at))
	}
	return ports.Unavailable(c.mask, "요청을 보내지 못했다 — "+at)
}
