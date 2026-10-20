package api

import (
	"log/slog"
	"net/http"
)

func screen(h http.Handler, log *slog.Logger) http.Handler {
	if h == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeProblem(log, w, notFound("not found")) })
	}
	return h
}
