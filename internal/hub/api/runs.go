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
	"github.com/d0lim/cockpit/internal/hub/ticker"
)

const (
	UISubmitter = "ui"
	maxBody     = 64 << 10
	maxText     = 4096
	OpExec      = "exec"
)

var (
	labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@-]{0,63}$`)
	suitePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
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
	l, err := a.d.Scheduler.List()
	if err != nil {
		return err
	}
	writeJSON(a.log(), w, http.StatusOK, PublicList(l, a.hubs()))
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
	if in.Suite != "" && !suitePattern.MatchString(in.Suite) {
		return badRequest("the suite name must be lowercase letters, digits and -, up to 63 characters")
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
	res := a.d.Scheduler.Submit(r.Context(), in)
	a.changed()
	if res.Fail != nil {
		return fromFailure(res.Fail)
	}
	writeJSON(a.log(), w, http.StatusAccepted, Submitted{ID: res.ID, Phase: res.Phase, Position: res.Position, Merged: res.Merged})
	return nil
}

func (a *api) get(w http.ResponseWriter, r *http.Request) error {
	v, err := a.runOf(r)
	if err != nil {
		return err
	}
	var services []ServicePort
	if a.d.Services != nil && holdsPorts(v.Meta) {
		services = a.d.Services(v.Meta)
	}
	writeJSON(a.log(), w, http.StatusOK, Detail(*v, a.d.Scheduler.AllowedEvents(v.Meta), services))
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
		return a.exec(w, v.ID, body, actor)
	}
	s := a.d.Scheduler
	ops := map[string]func(string, scheduler.Actor) ticker.Result{
		"keep": s.Keep, "unkeep": s.Unkeep, "stop": s.Stop, "resume": s.Resume, "destroy": s.Destroy, "abort": s.Abort,
	}
	res := ops[op](v.ID, actor)
	a.changed()
	if res.Fail != nil {
		return fromFailure(res.Fail)
	}
	status := http.StatusOK
	if op == "abort" {
		status = http.StatusAccepted
	}
	writeJSON(a.log(), w, status, OpReply{Run: MetaItem(res.Run)})
	return nil
}

func (a *api) report(w http.ResponseWriter, r *http.Request) error {
	v, err := a.runOf(r)
	if err != nil {
		return err
	}
	dir, err := a.runDir(v.ID)
	if err != nil {
		return err
	}
	writeJSON(a.log(), w, http.StatusOK, reportView(dir, v.Meta))
	return nil
}
