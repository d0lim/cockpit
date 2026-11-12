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
	"time"

	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	MaxBody         = 4 << 20
	contentProtobuf = "application/x-protobuf"
	contentJSON     = "application/json"
	shutdownGrace   = 5 * time.Second
)

var errTooLarge = errors.New("body over the limit")

type route func(body []byte, unmarshal func([]byte, proto.Message) error) ([]Signal, int, error)

var routes = map[string]route{
	"/v1/traces": func(body []byte, unmarshal func([]byte, proto.Message) error) ([]Signal, int, error) {
		var td tracev1.TracesData
		if err := unmarshal(body, &td); err != nil {
			return nil, 0, err
		}
		return spanSignals(&td), 0, nil
	},
	"/v1/metrics": func(body []byte, unmarshal func([]byte, proto.Message) error) ([]Signal, int, error) {
		var md metricsv1.MetricsData
		if err := unmarshal(body, &md); err != nil {
			return nil, 0, err
		}
		sigs, dropped := metricSignals(&md)
		return sigs, dropped, nil
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
	Out *Writer
	Log *slog.Logger
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
	sigs, dropped, err := convert(body, unmarshal)
	if err != nil {
		h.refuse(w, path, http.StatusBadRequest)
		return
	}
	if dropped > 0 {
		h.Log.Warn("dropped data points that are not finite numbers", "path", path, "dropped", dropped)
	}
	if err := h.Out.Write(sigs); err != nil {
		h.Log.Error("could not write output lines", "path", path, "lines", len(sigs), "err", err)
		h.refuse(w, path, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.WriteHeader(http.StatusOK)
	w.Write(emptyResponses[ctype])
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
