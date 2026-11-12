package otlp

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

type receiver struct {
	srv *httptest.Server
	out *bytes.Buffer
	log *bytes.Buffer
}

func newReceiver(t *testing.T) *receiver {
	t.Helper()
	var out, log bytes.Buffer
	w, err := NewWriter(&out)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(&Handler{Out: w, Log: slog.New(slog.NewJSONHandler(&log, nil))})
	t.Cleanup(srv.Close)
	return &receiver{srv: srv, out: &out, log: &log}
}

type answer struct {
	status int
	ctype  string
	body   []byte
}

func (r *receiver) post(t *testing.T, path string, body []byte, header ...string) answer {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, r.srv.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	res, err := r.srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return answer{status: res.StatusCode, ctype: res.Header.Get("Content-Type"), body: raw}
}

func (r *receiver) postProto(t *testing.T, path string, m proto.Message) answer {
	t.Helper()
	raw, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return r.post(t, path, raw, "Content-Type", contentProtobuf)
}

func (r *receiver) lines(t *testing.T) []Line {
	t.Helper()
	var out []Line
	sc := bufio.NewScanner(bytes.NewReader(r.out.Bytes()))
	sc.Buffer(nil, MaxLine)
	for sc.Scan() {
		var l Line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatalf("output line %q is not a JSON line: %v", sc.Text(), err)
		}
		out = append(out, l)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func str(k, v string) *commonv1.KeyValue {
	return &commonv1.KeyValue{Key: k, Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: v}}}
}

func resource(service string) *resourcev1.Resource {
	return &resourcev1.Resource{Attributes: []*commonv1.KeyValue{str("service.name", service)}}
}

func traces(service string, spans ...*tracev1.Span) *tracev1.TracesData {
	return &tracev1.TracesData{ResourceSpans: []*tracev1.ResourceSpans{{
		Resource:   resource(service),
		ScopeSpans: []*tracev1.ScopeSpans{{Spans: spans}},
	}}}
}

func bytesOf(t *testing.T, hexText string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(hexText)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
