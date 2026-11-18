package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const holdLimit = 5 * time.Minute

type wireServer struct {
	ledger     string
	appToken   string
	dbPassword string
	client     *http.Client
	mu         sync.Mutex
	tickets    map[string]string
	latest     string
}

func init() {
	if len(os.Args) == 3 && os.Args[1] == "-probe" {
		res, err := http.Get(os.Args[2])
		if err != nil || res.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}
	role := os.Getenv("WIRE_ROLE")
	if role == "" {
		return
	}
	s := &wireServer{
		ledger: os.Getenv("LEDGER_URL"), appToken: os.Getenv("APP_TOKEN"), dbPassword: os.Getenv("POSTGRES_PASSWORD"),
		client: &http.Client{Timeout: holdLimit}, tickets: map[string]string{},
	}
	if role == "app" {
		go s.boot()
	}
	logged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("request", "role", role, "method", r.Method, "path", r.URL.Path)
		s.routes().ServeHTTP(w, r)
	})
	if err := http.ListenAndServe(os.Getenv("WIRE_LISTEN"), logged); err != nil {
		slog.Error("wire service stopped", "err", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func (s *wireServer) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", ok)
	mux.HandleFunc("GET /boot", ok)
	mux.HandleFunc("GET /accounts/{key}", ok)
	mux.HandleFunc("POST /entries", ok)
	mux.HandleFunc("POST /hooks/{name}", ok)
	mux.HandleFunc("GET /facts", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]int{"created": 1, "rejected": 1})
	})
	mux.HandleFunc("POST /relay", s.relay)
	mux.HandleFunc("POST /later", s.later)
	mux.HandleFunc("POST /echo", echo)
	mux.HandleFunc("POST /secret-relay", s.secretRelay)
	mux.HandleFunc("POST /register", s.register)
	mux.HandleFunc("POST /tickets", s.issue)
	mux.HandleFunc("POST /tickets/redeem", s.redeem)
	mux.HandleFunc("GET /tickets/latest", s.latestTicket)
	mux.HandleFunc("POST /slow", s.slow)
	mux.HandleFunc("POST /hold", hold)
	mux.HandleFunc("GET /shapes", s.shapes)
	mux.HandleFunc("GET /jwt", s.token)
	mux.HandleFunc("GET /pem/begin", pemPart("-----BEGIN PRIVATE KEY-----\nTk9UQUtFWQ==\n"))
	mux.HandleFunc("GET /pem/end", pemPart("Tk9UQUtFWQ==\n-----END PRIVATE KEY-----\n"))
	mux.HandleFunc("GET /big", big)
	mux.HandleFunc("GET /binary", binary)
	return mux
}

func ok(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *wireServer) boot() {
	for range 60 {
		if status, _, err := s.send(context.Background(), http.MethodGet, s.ledger+"/boot", nil, nil); err == nil && status == http.StatusOK {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	slog.Error("the ledger never answered the boot call")
}

func (s *wireServer) send(ctx context.Context, method, url string, body any, header http.Header) (int, []byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return 0, nil, err
	}
	for name, values := range header {
		req.Header[name] = values
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	return res.StatusCode, data, err
}

func childSpan(parent string) string {
	parts := strings.Split(parent, "-")
	if len(parts) != 4 {
		return ""
	}
	id := make([]byte, 8)
	rand.Read(id)
	parts[2] = hex.EncodeToString(id)
	return strings.Join(parts, "-")
}

func (s *wireServer) relay(w http.ResponseWriter, r *http.Request) {
	header := http.Header{}
	switch r.URL.Query().Get("trace") {
	case "same":
		header.Set("Traceparent", r.Header.Get("Traceparent"))
	case "child":
		header.Set("Traceparent", childSpan(r.Header.Get("Traceparent")))
	}
	status, data, err := s.send(r.Context(), http.MethodPost, s.ledger+"/entries", map[string]string{"from": "app"}, header)
	if err != nil || status != http.StatusOK {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "the ledger did not answer"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]json.RawMessage{"ledger": data})
}

func (s *wireServer) later(w http.ResponseWriter, r *http.Request) {
	delay, err := strconv.Atoi(r.URL.Query().Get("delayMs"))
	name := r.URL.Query().Get("hook")
	if err != nil || name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "delayMs and hook are required"})
		return
	}
	time.AfterFunc(time.Duration(delay)*time.Millisecond, func() {
		if _, _, err := s.send(context.Background(), http.MethodPost, s.ledger+"/hooks/"+name, map[string]string{"from": "timer"}, nil); err != nil {
			slog.Error("the delayed call failed", "hook", name)
		}
	})
	writeJSON(w, http.StatusAccepted, map[string]bool{"scheduled": true})
}

func echo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"note": in.Note, "seen": r.URL.Query().Get("note")})
}

func (s *wireServer) secretRelay(w http.ResponseWriter, r *http.Request) {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+s.dbPassword)
	header.Set("X-Ledger-Key", s.dbPassword)
	status, _, err := s.send(r.Context(), http.MethodPost, s.ledger+"/echo?note="+s.dbPassword, map[string]string{"note": s.dbPassword}, header)
	if err != nil || status != http.StatusOK {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "the ledger did not answer"})
		return
	}
	ok(w, r)
}

func (s *wireServer) register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Callback string `json:"callback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Callback == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "callback is required"})
		return
	}
	status, _, err := s.send(r.Context(), http.MethodPost, in.Callback, map[string]string{"from": "callback"}, nil)
	if err != nil || status != http.StatusOK {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "the callback failed"})
		return
	}
	ok(w, r)
}

func randomText(n int) string {
	const letters = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	raw := make([]byte, n)
	rand.Read(raw)
	for i, b := range raw {
		raw[i] = letters[int(b)%len(letters)]
	}
	return string(raw)
}

func (s *wireServer) issue(w http.ResponseWriter, r *http.Request) {
	id := "ord-" + strings.ToLower(randomText(8))
	ticket := randomText(18)
	s.mu.Lock()
	s.tickets[ticket] = id
	s.latest = ticket
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "reference": ticket})
}

func (s *wireServer) redeem(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID        string `json:"id"`
		Reference string `json:"reference"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON"})
		return
	}
	s.mu.Lock()
	id := s.tickets[in.Reference]
	s.mu.Unlock()
	if id == "" || id != in.ID {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no such ticket"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "state": "redeemed"})
}

func (s *wireServer) latestTicket(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]string{"latest": s.latest})
}

func (s *wireServer) slow(w http.ResponseWriter, r *http.Request) {
	status, _, err := s.send(context.Background(), http.MethodPost, s.ledger+"/hold", map[string]string{"from": "app"}, nil)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "the hold broke off"})
		return
	}
	writeJSON(w, status, map[string]bool{"held": true})
}

func hold(w http.ResponseWriter, r *http.Request) {
	time.Sleep(holdLimit)
	ok(w, r)
}

func derive(token, label string) string {
	sum := sha256.Sum256([]byte(token + "|" + label))
	return fmt.Sprintf("%x", sum)[:24]
}
