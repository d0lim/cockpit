package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const serviceName = "app"

var (
	traceparent     = regexp.MustCompile(`^[0-9a-f]{2}-([0-9a-f]{32})-([0-9a-f]{16})-[0-9a-f]{2}$`)
	durationBoundMs = []float64{5, 10, 25, 50, 100, 250, 500, 1000}
)

type attr map[string]any

type telemetry struct {
	endpoint string
	client   *http.Client
	start    time.Time
	mu       sync.Mutex
	requests int64
	sumMs    float64
	buckets  []int64
}

func newTelemetry(endpoint string) *telemetry {
	if endpoint == "" {
		return nil
	}
	return &telemetry{
		endpoint: strings.TrimRight(endpoint, "/"),
		client:   &http.Client{Timeout: 2 * time.Second},
		start:    time.Now(),
		buckets:  make([]int64, len(durationBoundMs)+1),
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (t *telemetry) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		end := time.Now()
		t.observe(end.Sub(begin))
		go t.post("/v1/traces", t.spanExport(r, sw.status, begin, end))
	})
}

func (t *telemetry) observe(d time.Duration) {
	ms := float64(d.Microseconds()) / 1000
	i := 0
	for i < len(durationBoundMs) && ms > durationBoundMs[i] {
		i++
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.requests++
	t.sumMs += ms
	t.buckets[i]++
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func nanos(at time.Time) string {
	return strconv.FormatInt(at.UnixNano(), 10)
}

func str(key, value string) attr {
	return attr{"key": key, "value": attr{"stringValue": value}}
}

func resource() attr {
	return attr{"attributes": []attr{str("service.name", serviceName)}}
}

func (t *telemetry) spanExport(r *http.Request, status int, begin, end time.Time) attr {
	name := r.Pattern
	if name == "" {
		name = r.Method + " " + r.URL.Path
	}
	span := attr{
		"traceId":           randomHex(16),
		"spanId":            randomHex(8),
		"name":              name,
		"kind":              2,
		"startTimeUnixNano": nanos(begin),
		"endTimeUnixNano":   nanos(end),
		"attributes": []attr{
			str("http.request.method", r.Method),
			str("url.path", r.URL.Path),
			{"key": "http.response.status_code", "value": attr{"intValue": strconv.Itoa(status)}},
		},
		"status": attr{"code": 0},
	}
	if m := traceparent.FindStringSubmatch(r.Header.Get("traceparent")); m != nil {
		span["traceId"], span["parentSpanId"] = m[1], m[2]
	}
	if status >= 500 {
		span["status"] = attr{"code": 2}
	}
	return attr{"resourceSpans": []attr{{
		"resource":   resource(),
		"scopeSpans": []attr{{"scope": attr{"name": "toy/app"}, "spans": []attr{span}}},
	}}}
}

func (t *telemetry) metricExport(at time.Time) attr {
	t.mu.Lock()
	requests, sumMs := t.requests, t.sumMs
	counts := make([]string, len(t.buckets))
	for i, c := range t.buckets {
		counts[i] = strconv.FormatInt(c, 10)
	}
	t.mu.Unlock()
	point := attr{"startTimeUnixNano": nanos(t.start), "timeUnixNano": nanos(at)}
	counter := attr{"asInt": strconv.FormatInt(requests, 10)}
	histogram := attr{"count": strconv.FormatInt(requests, 10), "sum": sumMs, "bucketCounts": counts, "explicitBounds": durationBoundMs}
	for k, v := range point {
		counter[k], histogram[k] = v, v
	}
	metrics := []attr{
		{"name": "toy.requests", "unit": "1", "sum": attr{"aggregationTemporality": 2, "isMonotonic": true, "dataPoints": []attr{counter}}},
		{"name": "toy.request.duration", "unit": "ms", "histogram": attr{"aggregationTemporality": 2, "dataPoints": []attr{histogram}}},
	}
	return attr{"resourceMetrics": []attr{{
		"resource":     resource(),
		"scopeMetrics": []attr{{"scope": attr{"name": "toy/app"}, "metrics": metrics}},
	}}}
}

func (t *telemetry) export(interval time.Duration) {
	for at := range time.Tick(interval) {
		t.post("/v1/metrics", t.metricExport(at))
	}
}

func (t *telemetry) post(path string, body attr) {
	data, err := json.Marshal(body)
	if err != nil {
		slog.Warn("otlp export not encoded", "path", path, "err", err)
		return
	}
	res, err := t.client.Post(t.endpoint+path, "application/json", bytes.NewReader(data))
	if err != nil {
		slog.Warn("otlp export failed", "path", path, "err", err)
		return
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		slog.Warn("otlp export refused", "path", path, "status", res.StatusCode)
	}
}
