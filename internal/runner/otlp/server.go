package otlp

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"sync"
	"time"

	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	MaxBody         = 4 << 20
	MemoryLimit     = 256 << 20
	contentProtobuf = "application/x-protobuf"
	contentJSON     = "application/json"
	shutdownGrace   = 5 * time.Second
)

var errTooLarge = errors.New("body over the limit")

type route func(body []byte, unmarshal func([]byte, proto.Message) error) ([]Signal, drops, error)

var routes = map[string]route{
	"/v1/traces": func(body []byte, unmarshal func([]byte, proto.Message) error) ([]Signal, drops, error) {
		var td tracev1.TracesData
		if err := unmarshal(body, &td); err != nil {
			return nil, drops{}, err
		}
		sigs, err := spanSignals(&td)
		return sigs, drops{}, err
	},
	"/v1/metrics": func(body []byte, unmarshal func([]byte, proto.Message) error) ([]Signal, drops, error) {
		var md metricsv1.MetricsData
		if err := unmarshal(body, &md); err != nil {
			return nil, drops{}, err
		}
		return metricSignals(&md)
	},
}

var decoders = map[string]func([]byte, proto.Message) error{
	contentProtobuf: proto.Unmarshal,
	contentJSON: func(body []byte, m proto.Message) error {
		fixed, err := base64IDs(body)
		if err != nil {
			return err
		}
		return protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(fixed, m)
	},
}

var emptyResponses = map[string][]byte{contentProtobuf: {}, contentJSON: []byte("{}")}

type Handler struct {
	Out     *Writer
	Log     *slog.Logger
	exports sync.Mutex
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	convert, ok := routes[path]
	if !ok {
		h.refuse(w, "", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		h.refuse(w, path, http.StatusMethodNotAllowed)
		return
	}
	ctype, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	unmarshal, ok := decoders[ctype]
	if !ok {
		h.refuse(w, path, http.StatusUnsupportedMediaType)
		return
	}
	body, status := readBody(w, r)
	if status != http.StatusOK {
		h.refuse(w, path, status)
		return
	}
	if status := h.export(path, body, convert, unmarshal); status != http.StatusOK {
		h.refuse(w, path, status)
		if status == http.StatusRequestEntityTooLarge {
			io.WriteString(w, errTooMany.Error())
		}
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.WriteHeader(http.StatusOK)
	w.Write(emptyResponses[ctype])
}

func (h *Handler) export(path string, body []byte, convert route, unmarshal func([]byte, proto.Message) error) int {
	h.exports.Lock()
	defer h.exports.Unlock()
	sigs, dropped, err := convert(body, unmarshal)
	if errors.Is(err, errTooMany) {
		return http.StatusRequestEntityTooLarge
	}
	if err != nil {
		return http.StatusBadRequest
	}
	if dropped.points > 0 {
		h.Log.Warn("dropped data points that are not finite numbers", "path", path, "dropped", dropped.points)
	}
	if dropped.buckets > 0 {
		h.Log.Warn("dropped the buckets of histogram points whose bounds are not finite numbers", "path", path, "points", dropped.buckets)
	}
	if err := h.Out.Write(sigs); err != nil {
		h.Log.Error("could not write output lines", "path", path, "lines", len(sigs), "err", err)
		return http.StatusServiceUnavailable
	}
	return http.StatusOK
}

func (h *Handler) refuse(w http.ResponseWriter, route string, status int) {
	h.Log.Warn("refused an export request", "route", route, "status", status)
	w.WriteHeader(status)
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, int) {
	var src io.Reader = http.MaxBytesReader(w, r.Body, MaxBody)
	switch r.Header.Get("Content-Encoding") {
	case "", "identity":
	case "gzip":
		zr, err := gzip.NewReader(src)
		if err != nil {
			return nil, statusOf(err)
		}
		src = zr
	default:
		return nil, http.StatusUnsupportedMediaType
	}
	body, err := io.ReadAll(io.LimitReader(src, MaxBody+1))
	if err == nil && len(body) > MaxBody {
		err = errTooLarge
	}
	if err != nil {
		return nil, statusOf(err)
	}
	return body, http.StatusOK
}

func statusOf(err error) int {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) || errors.Is(err, errTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func Serve(ctx context.Context, ln net.Listener, h *Handler) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(h.Log.Handler(), slog.LevelWarn),
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		h.Log.Warn("closed requests still in flight after the shutdown grace", "err", err)
		srv.Close()
	}
	return nil
}
