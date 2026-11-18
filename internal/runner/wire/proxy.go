package recorder

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/d0lim/cockpit/internal/wire"
)

const upstreamDialTimeout = 30 * time.Second

var forwardingHeaders = []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto"}

type callKey struct{}

func upstreamTransport(dial func(ctx context.Context, network, addr string) (net.Conn, error)) *http.Transport {
	if dial == nil {
		dial = (&net.Dialer{Timeout: upstreamDialTimeout}).DialContext
	}
	return &http.Transport{Proxy: nil, DialContext: dial, DisableKeepAlives: true, DisableCompression: true}
}

type proxy struct {
	route   Route
	store   *store
	forward *httputil.ReverseProxy
}

func newProxy(route Route, s *store, transport http.RoundTripper, errors *log.Logger) *proxy {
	target := &url.URL{Scheme: "http", Host: route.upstream()}
	return &proxy{route: route, store: s, forward: &httputil.ReverseProxy{
		Rewrite:        func(pr *httputil.ProxyRequest) { rewrite(pr, target) },
		Transport:      transport,
		FlushInterval:  -1,
		ErrorLog:       errors,
		ModifyResponse: observeResponse,
		ErrorHandler:   abortCall,
	}}
}

func rewrite(pr *httputil.ProxyRequest, target *url.URL) {
	pr.SetURL(target)
	pr.Out.URL.Path, pr.Out.URL.RawPath = pr.In.URL.Path, pr.In.URL.RawPath
	pr.Out.URL.RawQuery, pr.Out.URL.ForceQuery = pr.In.URL.RawQuery, pr.In.URL.ForceQuery
	for _, name := range forwardingHeaders {
		if values, ok := pr.In.Header[name]; ok {
			pr.Out.Header[name] = values
		}
	}
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := p.store.begin(p.route, r)
	out := &callerWriter{ResponseWriter: w, call: c}
	if c == nil {
		p.forward.ServeHTTP(out, r)
		return
	}
	defer p.store.finish(c)
	if r.Body != nil && r.Body != http.NoBody {
		r.Body = &tee{ReadCloser: r.Body, call: c, side: &c.request}
	}
	p.forward.ServeHTTP(out, r.WithContext(context.WithValue(r.Context(), callKey{}, c)))
	c.markServed()
}

func callOf(r *http.Request) *call {
	c, _ := r.Context().Value(callKey{}).(*call)
	return c
}

func observeResponse(res *http.Response) error {
	c := callOf(res.Request)
	if c == nil {
		return nil
	}
	c.responded(res)
	if res.StatusCode != http.StatusSwitchingProtocols {
		res.Body = &tee{ReadCloser: res.Body, call: c, side: &c.response}
	}
	return nil
}

func abortCall(_ http.ResponseWriter, r *http.Request, err error) {
	if c := callOf(r); c != nil {
		c.failed(failureOutcome(r, err))
	}
	panic(http.ErrAbortHandler)
}

func failureOutcome(r *http.Request, err error) wire.Outcome {
	var dial *net.OpError
	switch {
	case r.Context().Err() != nil:
		return wire.OutcomeCallerGone
	case errors.As(err, &dial) && dial.Op == "dial":
		return wire.OutcomeNoUpstream
	}
	return wire.OutcomeIncomplete
}

type callerWriter struct {
	http.ResponseWriter
	call *call
}

func (w *callerWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *callerWriter) WriteHeader(code int) {
	if code < http.StatusOK {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	h := w.Header()
	for _, name := range []string{"Content-Type", "Date"} {
		if _, ok := h[name]; !ok {
			h[name] = nil
		}
	}
	w.ResponseWriter.WriteHeader(code)
	http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *callerWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if err != nil && w.call != nil {
		w.call.markCallerGone()
	}
	return n, err
}
