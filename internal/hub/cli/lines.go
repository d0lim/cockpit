package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/labels"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

func progress(run api.RunItem) string {
	if run.Phase != lifecycle.Queued {
		return run.ID + " · " + run.PhaseLabel
	}
	line := run.ID + " · " + labels.Text(labels.MsgQueued, map[string]string{"position": fmt.Sprint(run.Position)})
	if reason, ok := lifecycle.WaitLabels[run.WaitReason]; ok {
		line += " · " + reason
	}
	return line
}

func joinNonEmpty(sep string, parts ...string) string {
	kept := parts[:0:0]
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

func listLines(hub api.HubList) []string {
	var lines []string
	for _, m := range hub.Runs {
		verdict, keep, queued := "-", "", ""
		if m.VerdictLabel != nil {
			verdict = *m.VerdictLabel
		}
		if m.Keep {
			keep = labels.Text(labels.MsgKeep, nil)
		}
		if m.Position > 0 {
			queued = labels.Text(labels.MsgQueued, map[string]string{"position": fmt.Sprint(m.Position)})
		}
		lines = append(lines, joinNonEmpty("  ", m.ID, m.PhaseLabel, verdict, m.Suite, m.Submitter, keep, queued))
	}
	if len(lines) == 0 {
		lines = append(lines, labels.Text(labels.MsgNoRuns, nil))
	}
	footer := labels.Text(labels.MsgListFooter, map[string]string{
		"pool": fmt.Sprint(hub.Pool.Size), "poolMax": fmt.Sprint(hub.Pool.Max), "up": fmt.Sprint(hub.Up.Count), "upCap": fmt.Sprint(hub.Up.Cap),
	})
	notes := []string{footer}
	if hub.DockerDown {
		notes = append(notes, labels.Text(labels.MsgDockerDown, nil))
	}
	if len(hub.Orphans) > 0 {
		notes = append(notes, labels.Text(labels.MsgOrphans, map[string]string{"count": fmt.Sprint(len(hub.Orphans))}))
	}
	if hub.TickError != nil {
		notes = append(notes, labels.Text(labels.MsgTickError, map[string]string{"error": *hub.TickError}))
	}
	return append(lines, strings.Join(notes, " · "))
}

func showLines(run api.RunDetail, kit *api.KitView) []string {
	head := run.ID + " · " + run.PhaseLabel
	if run.VerdictLabel != nil {
		head += " · verdict " + *run.VerdictLabel
	}
	if run.Keep {
		head += " · " + labels.Text(labels.MsgKeep, nil)
	}
	lines := []string{head, fmt.Sprintf("commit %s · suite %s · submitter %s · host API %d · cockpit %s", run.SHA, orDash(run.Suite), run.Submitter, run.HostAPI, run.CockpitVersion)}
	if run.Phase == lifecycle.Queued {
		lines = append(lines, progress(run.RunItem))
	}
	if run.Notice != "" {
		lines = append(lines, labels.Text(labels.MsgReason, map[string]string{"reason": run.Notice}))
	}
	notes := [][2]string{
		{"stop reason", lifecycle.StopLabels[run.StopReason]},
		{"destroy reason", lifecycle.DestroyLabels[run.DestroyReason]},
	}
	if run.FailedStage != "" {
		notes = append(notes, [2]string{"stopped at stage", lifecycle.StageLabel(run.FailedStage)})
	}
	for _, n := range notes {
		if n[1] != "" {
			lines = append(lines, n[0]+": "+n[1])
		}
	}
	if len(run.AllowedEvents) > 0 {
		events := make([]string, len(run.AllowedEvents))
		for i, e := range run.AllowedEvents {
			events[i] = string(e)
		}
		lines = append(lines, labels.Text(labels.MsgAllowedEvents, map[string]string{"events": strings.Join(events, " · ")}))
	}
	for _, s := range run.Services {
		lines = append(lines, fmt.Sprintf("  127.0.0.1:%d  %s:%d  %s", s.HostPort, s.Service, s.Port, s.Label))
	}
	if kit == nil {
		return lines
	}
	if kit.Kit != nil {
		lines = append(lines, labels.Text(labels.MsgKit, map[string]string{"dir": kit.Kit.Dir}))
	}
	if kit.Exec != nil {
		lines = append(lines, labels.Text(labels.MsgExecCommand, map[string]string{"command": kit.Exec.Command}))
		if len(kit.Exec.Flows) > 0 {
			lines = append(lines, labels.Text(labels.MsgExecFlows, map[string]string{"flows": strings.Join(kit.Exec.Flows, " · ")}))
		} else {
			lines = append(lines, labels.Text(labels.MsgExecFlowsUnknown, nil))
		}
	}
	return append(lines, labels.Text(labels.MsgBundle, map[string]string{"dir": kit.ReportDir}))
}

func reportLines(rep api.ReportView) []string {
	head := rep.ID + " · " + rep.PhaseLabel
	if rep.VerdictLabel != nil {
		head += " · verdict " + *rep.VerdictLabel
	}
	lines := []string{head}
	switch rep.Status {
	case api.ReportPending:
		lines = append(lines, labels.Text(labels.MsgReportPending, nil))
	case api.ReportRedactionFailed:
		lines = append(lines, labels.Text(labels.MsgRedactionFailed, nil))
		for _, f := range rep.Files {
			lines = append(lines, "  - "+f)
		}
	case api.ReportMetaOnly:
		stage := lifecycle.Phase("")
		if rep.FailedStage != nil {
			stage = *rep.FailedStage
		}
		lines = append(lines, labels.Text(labels.MsgMetaOnly, map[string]string{"stage": lifecycle.StageLabel(stage)}))
		if rep.Notice != nil && *rep.Notice != "" {
			lines = append(lines, labels.Text(labels.MsgReason, map[string]string{"reason": *rep.Notice}))
		}
		tail := rep.Tail[max(0, len(rep.Tail)-10):]
		for _, t := range tail {
			lines = append(lines, "  | "+t)
		}
	case api.ReportBundle:
		lines = append(lines, bundleLines(rep)...)
	}
	return lines
}

func bundleLines(rep api.ReportView) []string {
	var lines []string
	if s := rep.Steps; s != nil {
		num := func(n *int) string {
			if n == nil {
				return "-"
			}
			return fmt.Sprint(*n)
		}
		lines = append(lines, fmt.Sprintf("steps %s/%s · failed %d · skipped %d · requests %d", num(s.Actual), num(s.Expected), s.Failed, s.Skipped, s.Requests))
	}
	if st := rep.StoppedAt; st != nil {
		where := st.Flow + " › " + st.Step
		if st.Message != "" {
			where += " — " + st.Message
		}
		lines = append(lines, labels.Text(labels.MsgStopPoint, map[string]string{"where": where}))
	}
	var summary struct {
		Reasons []string `json:"reasons"`
	}
	if json.Unmarshal(rep.Summary, &summary) != nil {
		summary.Reasons = nil
	}
	for _, r := range summary.Reasons {
		lines = append(lines, "  - "+r)
	}
	if len(rep.Flows) > 0 {
		lines = append(lines, labels.Text(labels.MsgFlowResults, nil))
	}
	for _, f := range rep.Flows {
		lines = append(lines, fmt.Sprintf("  %s › %s  %s  %d/%d  %s", f.Stage, f.ID, f.Status, f.Passed, f.Steps, orDash(f.StoppedAt)))
	}
	if np := rep.NotProven; np != nil {
		for _, item := range append(append(append([]string{}, np.Skipped...), np.Declared...), np.Unread...) {
			lines = append(lines, labels.Text(labels.MsgNotProven, map[string]string{"item": item}))
		}
	}
	return append(lines, labels.Text(labels.MsgBundle, map[string]string{"dir": rep.ReportDir}))
}

func execLines(view api.ExecView) []string {
	var lines []string
	for _, f := range view.Flows {
		for _, s := range f.Steps {
			line := fmt.Sprintf("%s › %s › %s", f.ID, s.ID, s.Status)
			if s.HTTPStatus != nil {
				line += fmt.Sprintf(" [%d]", *s.HTTPStatus)
			}
			if s.Message != "" {
				line += " — " + s.Message
			}
			lines = append(lines, line)
		}
	}
	if len(view.Flows) == 0 {
		lines = append(lines, view.Lines...)
	}
	if view.Reason != "" {
		lines = append(lines, labels.Text(labels.MsgReason, map[string]string{"reason": view.Reason}))
	}
	code := "none"
	if view.ExitCode != nil {
		code = fmt.Sprint(*view.ExitCode)
	}
	exit := labels.Text(labels.MsgExecExit, map[string]string{"code": code})
	if view.TimedOut {
		exit += " · time limit exceeded"
	}
	if view.OOMKilled {
		exit += " · out of memory (OOM)"
	}
	return append(lines, exit)
}
