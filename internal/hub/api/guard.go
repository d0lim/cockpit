package api

import (
	"log/slog"
	"net/http"
	"slices"
	"strconv"
)

const (
	HeaderCockpit   = "X-Cockpit"
	HeaderSubmitter = "X-Cockpit-Submitter"
)

type denial struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

func forbidden(log *slog.Logger, w http.ResponseWriter, reason string) {
	writeJSON(log, w, http.StatusForbidden, denial{Code: CodeForbidden, Message: "forbidden:" + reason, Error: "forbidden:" + reason})
}

func LoopbackHosts(port int) []string {
	p := strconv.Itoa(port)
	return []string{"127.0.0.1:" + p, "localhost:" + p}
}

func Guard(port int, log *slog.Logger, next http.Handler) http.Handler {
	hosts := LoopbackHosts(port)
	origins := make([]string, len(hosts))
	for i, h := range hosts {
		origins[i] = "http://" + h
	}
	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { forbidden(log, w, "cross-origin") }))
	protected := cop.Handler(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !slices.Contains(hosts, r.Host) {
			forbidden(log, w, "host")
			return
		}
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		if !slices.Contains(origins, r.Header.Get("Origin")) {
			forbidden(log, w, "origin")
			return
		}
		if r.Header.Get(HeaderCockpit) != "1" {
			forbidden(log, w, "header")
			return
		}
		protected.ServeHTTP(w, r)
	})
}
