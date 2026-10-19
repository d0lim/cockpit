package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/d0lim/cockpit/internal/hub/pool"
	"github.com/d0lim/cockpit/internal/hub/ticker"
)

const (
	CodeBadRequest   = "BAD_REQUEST"
	CodeNotFound     = "NOT_FOUND"
	CodeForbidden    = "FORBIDDEN"
	CodeInternal     = "INTERNAL"
	CodeReadRefused  = "READ_REFUSED"
	CodeBadTarget    = "BAD_TARGET"
	CodeNotAvailable = "NOT_AVAILABLE"
)

var Codes = []string{
	CodeBadRequest, CodeNotFound, CodeForbidden, CodeInternal, CodeReadRefused, CodeBadTarget, CodeNotAvailable,
	string(ticker.CodeConflictState), string(ticker.CodeConflictCap), string(ticker.CodeConflictOwner),
	string(ticker.CodeConflictPort), string(ticker.CodeQueueFull), string(ticker.CodeOpFailed),
	string(ticker.CodeRejectedCommit), string(ticker.CodeRejectedWorktree), string(ticker.CodeRejectedDirty),
	string(ticker.CodeRejectedNoHarness), string(ticker.CodeRejectedHostAPI),
}

type Problem struct {
	Status     int              `json:"-"`
	Code       string           `json:"code"`
	Message    string           `json:"message"`
	Files      []string         `json:"files,omitempty"`
	Run        *RunItem         `json:"run,omitempty"`
	Candidates []pool.Candidate `json:"candidates,omitempty"`
}

func (p *Problem) Error() string { return p.Message }

func badRequest(message string) *Problem {
	return &Problem{Status: http.StatusBadRequest, Code: CodeBadRequest, Message: message}
}

func notFound(message string) *Problem {
	return &Problem{Status: http.StatusNotFound, Code: CodeNotFound, Message: message}
}

func fromFailure(f *ticker.Failure) *Problem {
	p := &Problem{Status: f.Status, Code: string(f.Code), Message: f.Reason, Files: f.Files, Candidates: f.Candidates}
	if f.Run != nil {
		item := MetaItem(*f.Run)
		p.Run = &item
	}
	return p
}

func writeJSON(log *slog.Logger, w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Debug("response write failed", "error", err)
	}
}

func writeProblem(log *slog.Logger, w http.ResponseWriter, err error) {
	var p *Problem
	if !errors.As(err, &p) {
		p = &Problem{Status: http.StatusInternalServerError, Code: CodeInternal, Message: err.Error()}
	}
	writeJSON(log, w, p.Status, p)
}
