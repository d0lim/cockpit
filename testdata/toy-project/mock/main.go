package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
)

type order struct {
	Item     string `json:"item"`
	Quantity int    `json:"quantity"`
}

type server struct {
	token    string
	cache    cache
	created  atomic.Int64
	rejected atomic.Int64
}

func main() {
	s := &server{token: os.Getenv("APP_TOKEN"), cache: cache{addr: os.Getenv("CACHE_ADDR")}}
	if s.token == "" || s.cache.addr == "" {
		slog.Error("APP_TOKEN and CACHE_ADDR are required")
		os.Exit(2)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /orders", s.createOrder)
	mux.HandleFunc("GET /orders/{id}", s.getOrder)
	mux.HandleFunc("GET /facts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]int64{"created": s.created.Load(), "rejected": s.rejected.Load()})
	})
	slog.Info("toy app listening", "addr", ":8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		slog.Error("toy app stopped", "err", err)
		os.Exit(1)
	}
}

func (s *server) authorized(r *http.Request) bool {
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) == 1
}

func (s *server) createOrder(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		s.rejected.Add(1)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing or wrong bearer token"})
		return
	}
	var o order
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&o); err != nil || o.Item == "" || o.Quantity < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be {item, quantity >= 1}"})
		return
	}
	id, err := s.cache.do("INCR", "order:seq")
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cache unavailable"})
		return
	}
	body, _ := json.Marshal(o)
	if _, err := s.cache.do("SET", "order:"+id, string(body)); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cache unavailable"})
		return
	}
	s.created.Add(1)
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *server) getOrder(w http.ResponseWriter, r *http.Request) {
	raw, err := s.cache.do("GET", "order:"+r.PathValue("id"))
	switch {
	case errors.Is(err, errNil):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such order"})
	case err != nil:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cache unavailable"})
	default:
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(raw))
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
