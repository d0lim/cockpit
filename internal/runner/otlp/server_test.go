package otlp

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

func TestABodyOverTheLimitIsRefusedWith413(t *testing.T) {
	r := newReceiver(t)
	big := bytes.Repeat([]byte{0}, MaxBody+1)
	if got := r.post(t, "/v1/traces", big, "Content-Type", contentProtobuf); got.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("raw body: status %d, want 413", got.status)
	}
	var zipped bytes.Buffer
	zw := gzip.NewWriter(&zipped)
	zw.Write(big)
	zw.Close()
	if zipped.Len() > MaxBody {
		t.Fatalf("the gzip body is %d bytes, so it does not test the decompressed limit", zipped.Len())
	}
	if got := r.post(t, "/v1/metrics", zipped.Bytes(), "Content-Type", contentProtobuf, "Content-Encoding", "gzip"); got.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("gzip body that inflates past the limit: status %d, want 413", got.status)
	}
	noise := make([]byte, MaxBody-64)
	rand.Read(noise)
	var grown bytes.Buffer
	zw = gzip.NewWriter(&grown)
	zw.Write(noise)
	zw.Close()
	if grown.Len() <= MaxBody {
		t.Fatalf("the gzip body of random bytes is %d bytes, so it does not test the compressed limit", grown.Len())
	}
	if got := r.post(t, "/v1/traces", grown.Bytes(), "Content-Type", contentProtobuf, "Content-Encoding", "gzip"); got.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("gzip body over the limit that inflates under it: status %d, want 413", got.status)
	}
	if got := r.postProto(t, "/v1/traces", spanPair(t)); got.status != 200 {
		t.Fatalf("after the refusals: status %d, want 200", got.status)
	}
	if n := len(r.lines(t)); n != 2 {
		t.Fatalf("%d lines, want only the 2 spans of the accepted export", n)
	}
}

func TestABodyAtTheLimitIsAccepted(t *testing.T) {
	r := newReceiver(t)
	raw, err := proto.Marshal(spanPair(t))
	if err != nil {
		t.Fatal(err)
	}
	padded := append(raw, bytes.Repeat([]byte{0x0a, 0x00}, (MaxBody-len(raw))/2)...)
	if len(padded) > MaxBody || len(padded) < MaxBody-1 {
		t.Fatalf("padded body is %d bytes, want the limit", len(padded))
	}
	if got := r.post(t, "/v1/traces", padded, "Content-Type", contentProtobuf); got.status != 200 {
		t.Fatalf("status %d, want 200", got.status)
	}
	if n := len(r.lines(t)); n != 2 {
		t.Fatalf("%d lines, want 2", n)
	}
}

func TestOnlyPostToTheTraceAndMetricPathsWithAKnownEncodingIsServed(t *testing.T) {
	r := newReceiver(t)
	empty := []byte{}
	cases := []struct {
		name   string
		method string
		path   string
		header []string
		want   int
	}{
		{"logs are out of scope", http.MethodPost, "/v1/logs", []string{"Content-Type", contentProtobuf}, http.StatusNotFound},
		{"root path", http.MethodPost, "/", []string{"Content-Type", contentProtobuf}, http.StatusNotFound},
		{"unknown path", http.MethodPost, "/PLANTED-PATH", []string{"Content-Type", contentProtobuf}, http.StatusNotFound},
		{"get on traces", http.MethodGet, "/v1/traces", nil, http.StatusMethodNotAllowed},
		{"text body", http.MethodPost, "/v1/traces", []string{"Content-Type", "text/plain"}, http.StatusUnsupportedMediaType},
		{"no content type", http.MethodPost, "/v1/metrics", nil, http.StatusUnsupportedMediaType},
		{"brotli body", http.MethodPost, "/v1/metrics", []string{"Content-Type", contentProtobuf, "Content-Encoding", "br"}, http.StatusUnsupportedMediaType},
		{"broken protobuf", http.MethodPost, "/v1/traces", []string{"Content-Type", contentProtobuf}, http.StatusBadRequest},
		{"empty protobuf traces", http.MethodPost, "/v1/traces", []string{"Content-Type", contentProtobuf}, http.StatusOK},
		{"empty protobuf metrics with identity", http.MethodPost, "/v1/metrics", []string{"Content-Type", contentProtobuf, "Content-Encoding", "identity"}, http.StatusOK},
	}
	for _, c := range cases {
		body := empty
		if c.name == "broken protobuf" {
			body = []byte{0x0a, 0x05, 0x01}
		}
		req, err := http.NewRequest(c.method, r.srv.URL+c.path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i+1 < len(c.header); i += 2 {
			req.Header.Set(c.header[i], c.header[i+1])
		}
		res, err := r.srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != c.want {
			t.Errorf("%s: status %d, want %d", c.name, res.StatusCode, c.want)
		}
		if c.want == http.StatusMethodNotAllowed && res.Header.Get("Allow") != http.MethodPost {
			t.Errorf("%s: Allow %q, want POST", c.name, res.Header.Get("Allow"))
		}
	}
	if r.out.Len() != 0 {
		t.Fatalf("refused or empty exports wrote %q", r.out.String())
	}
	if strings.Contains(r.log.String(), "PLANTED") || !strings.Contains(r.log.String(), `"status":404`) {
		t.Fatalf("the log names a received path or misses the refusal: %q", r.log.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, net.ErrClosed }

func TestAnExportWhoseLinesCannotBeWrittenIsAnsweredWith503(t *testing.T) {
	r := newReceiver(t)
	w, err := NewWriter(failingWriter{})
	if err != nil {
		t.Fatal(err)
	}
	r.srv.Config.Handler.(*Handler).Out = w
	if got := r.postProto(t, "/v1/traces", spanPair(t)); got.status != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", got.status)
	}
}

func TestServeReturnsNilWhenTheContextEndsAndStopsListening(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	r := newReceiver(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, r.srv.Config.Handler.(*Handler)) }()
	url := "http://" + ln.Addr().String() + "/v1/traces"
	res, err := http.Post(url, contentProtobuf, bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d before the context ended, want 200", res.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v, want nil", err)
		}
	case <-time.After(shutdownGrace + time.Second):
		t.Fatal("Serve did not return after the context ended")
	}
	if _, err := http.Post(url, contentProtobuf, bytes.NewReader(nil)); err == nil {
		t.Fatal("the receiver still answers after Serve returned")
	}
}
