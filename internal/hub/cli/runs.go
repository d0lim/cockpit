package cli

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/labels"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
)

func withID(a args, run func(id string) (int, error)) (int, error) {
	if len(a.positional) == 0 {
		return 0, usageError{a.sub + " needs a run id"}
	}
	id := a.positional[0]
	if !store.IsRunID(id) {
		return 0, usageError{"not a run id: " + id}
	}
	if len(a.positional) > 1 {
		return 0, usageError{"extra arguments: " + strings.Join(a.positional[1:], " ")}
	}
	return run(id)
}

func gitToplevel(c context.Context, cwd string) (string, error) {
	out, err := exec.CommandContext(c, "git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (x *ctx) submit(a args) (int, error) {
	if len(a.positional) > 0 {
		return 0, usageError{"extra arguments: " + strings.Join(a.positional, " ")}
	}
	timeout, err := timeoutOf(a)
	if err != nil {
		return 0, err
	}
	worktree, err := x.io.Toplevel(x, x.io.Cwd)
	if err != nil || worktree == "" {
		return x.usage("run this inside a git worktree; the submitted worktree is the git top level of the current directory: " + x.io.Cwd), nil
	}
	commit := a.flags["commit"]
	if commit == "" {
		commit = "HEAD"
	}
	body := map[string]any{"worktree": worktree, "commit": commit, "submitter": x.submitter(a), "new": a.flags["new"] == "true"}
	if s := a.flags["suite"]; s != "" {
		body["suite"] = s
	}
	r, err := x.call(http.MethodPost, "/api/runs", body, requestLimit)
	if err != nil {
		return 0, err
	}
	if r.status != http.StatusAccepted {
		return x.refused(r), nil
	}
	sub, err := decode[api.Submitted](r.body)
	if err != nil || !store.IsRunID(sub.ID) {
		return x.emit(ExitError, CodeHubError, r.body, []string{"the hub reply has no run id"}, true), nil
	}
	vars := map[string]string{"id": sub.ID, "position": fmt.Sprint(sub.Position), "cmd": Cmd}
	head := labels.Text(labels.MsgSubmitted, vars)
	if sub.Merged {
		head = labels.Text(labels.MsgMerged, vars)
	}
	data := map[string]any{"id": sub.ID, "phase": sub.Phase, "position": sub.Position, "merged": sub.Merged}
	if a.flags["wait"] != "true" {
		return x.emit(ExitOK, CodeSubmitted, data, []string{head, labels.Text(labels.MsgWaitHint, vars)}, false), nil
	}
	if !x.json {
		fmt.Fprintf(x.io.Stdout, "%s · %s\n", head, labels.Text(labels.MsgWaitResume, vars))
	}
	return x.wait(sub.ID, timeout, true)
}

func (x *ctx) list() (int, error) {
	r, err := x.call(http.MethodGet, "/api/runs", nil, requestLimit)
	if err != nil {
		return 0, err
	}
	if r.status != http.StatusOK {
		return x.refused(r), nil
	}
	hub, err := decode[api.HubList](r.body)
	if err != nil {
		return 0, err
	}
	return x.emit(ExitOK, CodeOK, r.body, listLines(hub), false), nil
}

func (x *ctx) show(id string) (int, error) {
	r, err := x.call(http.MethodGet, "/api/runs/"+id, nil, requestLimit)
	if err != nil {
		return 0, err
	}
	if r.status != http.StatusOK {
		return x.refused(r), nil
	}
	run, err := decode[api.RunDetail](r.body)
	if err != nil {
		return 0, err
	}
	var kit *api.KitView
	data := map[string]any{"run": r.body, "kit": nil}
	if k, err := x.call(http.MethodGet, "/api/runs/"+id+"/kit", nil, requestLimit); err == nil && k.status == http.StatusOK {
		if v, err := decode[api.KitView](k.body); err == nil {
			kit, data["kit"] = &v, k.body
		}
	}
	return x.emit(ExitOK, CodeOK, data, showLines(run, kit), false), nil
}

func (x *ctx) report(id string) (int, error) {
	r, err := x.call(http.MethodGet, "/api/runs/"+id+"/report", nil, requestLimit)
	if err != nil {
		return 0, err
	}
	if r.status != http.StatusOK {
		return x.refused(r), nil
	}
	rep, err := decode[api.ReportView](r.body)
	if err != nil {
		return 0, err
	}
	return x.emit(ExitOK, CodeOK, map[string]any{"report": r.body}, reportLines(rep), false), nil
}

func (x *ctx) operate(a args, id string) (int, error) {
	path := "/api/runs/" + id + "/" + a.sub
	if a.flags["force"] == "true" {
		path += "?force=1"
	}
	r, err := x.call(http.MethodPost, path, map[string]any{"submitter": x.submitter(a)}, requestLimit)
	if err != nil {
		return 0, err
	}
	if r.status >= 300 {
		return x.refused(r), nil
	}
	op, err := decode[api.OpReply](r.body)
	if err != nil {
		return 0, err
	}
	line := op.Run.ID + " · " + op.Run.PhaseLabel
	if op.Run.Keep {
		line += " · " + labels.Text(labels.MsgKeep, nil)
	}
	return x.emit(ExitOK, CodeOK, r.body, []string{line}, false), nil
}

func settled(run api.RunItem) bool {
	pending := run.Phase == lifecycle.Queued || (run.Mark == lifecycle.MarkNone && isActivePhase(run.Phase))
	return run.Mark != lifecycle.MarkNone || (run.Verdict != lifecycle.VerdictNone && !pending)
}

func isActivePhase(p lifecycle.Phase) bool {
	return lifecycle.State{Phase: p}.Active()
}

func stateData(run api.RunItem) map[string]any {
	return map[string]any{"id": run.ID, "phase": run.Phase, "verdict": run.Verdict, "position": run.Position, "waitReason": run.WaitReason, "notice": run.Notice}
}

func (x *ctx) verdict(run api.RunItem) int {
	v, ok := verdictExit[string(run.Verdict)]
	if !ok {
		v.exit, v.code = ExitError, CodeError
	}
	label := labels.Text(labels.MsgNoVerdict, nil)
	if run.VerdictLabel != nil {
		label = *run.VerdictLabel
	}
	lines := []string{labels.Text(labels.MsgVerdictLine, map[string]string{"id": run.ID, "phase": run.PhaseLabel, "verdict": label})}
	if run.Notice != "" {
		lines = append(lines, labels.Text(labels.MsgReason, map[string]string{"reason": run.Notice}))
	}
	lines = append(lines, labels.Text(labels.MsgReportHint, map[string]string{"cmd": Cmd, "id": run.ID}))
	return x.emit(v.exit, v.code, stateData(run), lines, false)
}

func (x *ctx) wait(id string, timeout time.Duration, connected bool) (int, error) {
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	var lostAt time.Time
	var last *api.RunItem
	shown := ""
	for {
		limit := requestLimit
		if !deadline.IsZero() {
			limit = max(time.Millisecond, min(limit, time.Until(deadline)))
		}
		r, err := x.call(http.MethodGet, "/api/runs/"+id, nil, limit)
		if _, lost := err.(unreachable); err != nil && !lost {
			return 0, err
		}
		switch {
		case err != nil && !connected:
			return x.unreachable(""), nil
		case err != nil && lostAt.IsZero():
			lostAt = time.Now()
			if !x.json {
				fmt.Fprintln(x.io.Stderr, labels.Text(labels.MsgHubLost, nil))
			}
		case err != nil && time.Since(lostAt) >= x.io.Reconnect:
			return x.unreachable(id), nil
		case err == nil && r.status != http.StatusOK:
			return x.refused(r), nil
		case err == nil:
			connected, lostAt = true, time.Time{}
			run, derr := decode[api.RunItem](r.body)
			if derr != nil {
				return 0, derr
			}
			last = &run
			if settled(run) {
				return x.verdict(run), nil
			}
			if run.Phase == lifecycle.Queued && run.WaitReason == lifecycle.WaitPoolAllKept {
				line := labels.Text(labels.MsgPoolAllKept, map[string]string{"line": progress(run), "cmd": Cmd})
				return x.emit(ExitHuman, CodeWaitingOnHuman, stateData(run), []string{line}, false), nil
			}
			if line := progress(run); line != shown && !x.json {
				fmt.Fprintln(x.io.Stdout, line)
				shown = line
			}
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			now, data := "", map[string]any{"id": id}
			if last != nil {
				now, data = " (now "+last.PhaseLabel+")", stateData(*last)
			}
			line := labels.Text(labels.MsgTimeout, map[string]string{"now": now, "cmd": Cmd, "id": id})
			return x.emit(ExitTimeout, CodeTimeout, data, []string{line}, false), nil
		}
		pause := x.io.Poll
		if !deadline.IsZero() {
			pause = max(0, min(pause, time.Until(deadline)))
		}
		select {
		case <-x.Done():
			return 0, x.Err()
		case <-time.After(pause):
		}
	}
}
