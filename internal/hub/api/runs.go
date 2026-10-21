package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/hub/scheduler"
)

const (
	UISubmitter = "ui"
	maxBody     = 64 << 10
	maxText     = 4096
	OpExec      = "exec"
)

var (
	labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@-]{0,63}$`)
	Ops          = []string{"keep", "unkeep", "stop", "resume", "destroy", "abort"}
)

func bodyOf(r *http.Request) (map[string]any, error) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(raw) > maxBody {
		return nil, badRequest("the body cannot be read or is over 64 KiB")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil || body == nil {
		return nil, badRequest("the body must be a JSON object")
	}
	return body, nil
}

func submitterOf(r *http.Request, body map[string]any) (string, error) {
	var label any = UISubmitter
	if v, ok := body["submitter"]; ok {
		label = v
	} else if h := r.Header.Get(HeaderSubmitter); h != "" {
		label = h
	}
	s, ok := label.(string)
	if !ok || !labelPattern.MatchString(s) {
		return "", badRequest("the submitter label must be letters, digits and ._@-, up to 64 characters")
	}
	return s, nil
}

func actorOf(r *http.Request, body map[string]any) (scheduler.Actor, error) {
	by, err := submitterOf(r, body)
	return scheduler.Actor{By: by, Force: r.URL.Query().Get("force") == "1"}, err
}

func text(body map[string]any, key string, required bool) (string, error) {
	v, ok := body[key]
	if !ok && !required {
		return "", nil
	}
	s, isText := v.(string)
	if !isText || strings.TrimSpace(s) == "" || len(s) > maxText {
		return "", badRequest(key + " is empty or not a string")
	}
	return s, nil
}

func (a *api) list(w http.ResponseWriter, _ *http.Request) error {
	l, err := a.reader().List()
	if err != nil {
		return err
	}
	writeJSON(a.log(), w, http.StatusOK, l)
	return nil
}

func (a *api) submit(w http.ResponseWriter, r *http.Request) error {
	body, err := bodyOf(r)
	if err != nil {
		return err
	}
	in := scheduler.SubmitInput{}
	if in.Worktree, err = text(body, "worktree", true); err != nil {
		return err
	}
	if in.Commit, err = text(body, "commit", true); err != nil {
		return err
	}
	if in.Suite, err = text(body, "suite", false); err != nil {
		return err
	}
	if v, ok := body["new"]; ok {
		b, isBool := v.(bool)
		if !isBool {
			return badRequest("new is true or false")
		}
		in.New = b
	}
	if in.Submitter, err = submitterOf(r, body); err != nil {
		return err
	}
	submitted, err := a.reader().Submit(r.Context(), in)
	if err != nil {
		return err
	}
	writeJSON(a.log(), w, http.StatusAccepted, submitted)
	return nil
}

func (a *api) get(w http.ResponseWriter, r *http.Request) error {
	d, err := a.reader().Run(r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(a.log(), w, http.StatusOK, d)
	return nil
}

func (a *api) operate(w http.ResponseWriter, r *http.Request) error {
	op := r.PathValue("op")
	if op != OpExec && !slices.Contains(Ops, op) {
		return notFound("not found")
	}
	v, err := a.runOf(r)
	if err != nil {
		return err
	}
	body, err := bodyOf(r)
	if err != nil {
		return err
	}
	actor, err := actorOf(r, body)
	if err != nil {
		return err
	}
	if op == OpExec {
		flow, ok := body["flow"].(string)
		if !ok || flow == "" {
			return badRequest("flow is a flow id string")
		}
		view, err := a.reader().Exec(v.ID, flow, actor)
		if err != nil {
			return err
		}
		writeJSON(a.log(), w, http.StatusOK, view)
		return nil
	}
	reply, status, err := a.reader().Operate(v.ID, op, actor)
	if err != nil {
		return err
	}
	writeJSON(a.log(), w, status, reply)
	return nil
}

func (a *api) report(w http.ResponseWriter, r *http.Request) error {
	view, err := a.reader().Report(r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(a.log(), w, http.StatusOK, view)
	return nil
}
