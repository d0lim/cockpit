package screen

import (
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/sse"
)

func standing(r api.RunItem) string {
	switch {
	case r.Mark != "":
		return r.PhaseLabel + ". The hub keeps the record and the report; nothing else of the environment remains."
	case r.Phase == lifecycle.Queued:
		reason := "It starts soon."
		if r.WaitReason != "" {
			reason = "Waiting: " + label(labelTable.Wait, string(r.WaitReason)) + "."
		}
		return "Queued at position " + itoa(r.Position) + ". " + reason
	case isActive(r):
		return "Running the " + lifecycle.StageLabel(r.Phase) + " stage since " + stamp(orElse(r.StartedAt, r.CreatedAt)) + "."
	case r.Phase == lifecycle.Stopping:
		return "Stopping the environment."
	case r.Phase == lifecycle.Resuming:
		return "Resuming the environment."
	case r.Phase == lifecycle.Destroying:
		return "Destroying the environment."
	}
	verdict := "without a verdict"
	if r.VerdictLabel != nil {
		verdict = "with the verdict " + *r.VerdictLabel
	}
	return "Finished " + verdict + ". " + envSentence(r)
}

func envSentence(r api.RunItem) string {
	switch r.Env {
	case lifecycle.EnvUp:
		return "The environment is up."
	case lifecycle.EnvStopped:
		if r.StopReason != "" {
			return "The environment is stopped: " + label(labelTable.Stop, string(r.StopReason)) + "."
		}
		return "The environment is stopped."
	case lifecycle.EnvDestroyed:
		if r.DestroyReason != "" {
			return "The environment was destroyed (" + label(labelTable.Destroy, string(r.DestroyReason)) + "); only the report remains."
		}
		return "The environment was destroyed; only the report remains."
	}
	return "No environment remains."
}

func notices(r api.RunItem, l api.HubList) []string {
	var out []string
	if !served(r, l) {
		out = append(out, "This hub does not serve host API "+itoa(r.HostAPI)+". Signals and the report are hidden; the run can only be listed and destroyed.")
	}
	if r.WaitReason == lifecycle.WaitPoolAllKept {
		out = append(out, "Every kept environment has keep on. Release keep on one kept run to start.")
	}
	if r.Recreate {
		out = append(out, "The containers are gone; resume recreates them from the volumes.")
	}
	if r.Redaction != nil && r.Redaction.Status == "failed" {
		out = append(out, "The redaction rescan discarded the report bundle.")
	}
	if r.FailedStage != "" {
		out = append(out, "Stopped at the "+lifecycle.StageLabel(r.FailedStage)+" stage.")
	}
	if r.Notice != "" {
		out = append(out, "Last failure: "+r.Notice)
	}
	return out
}

type stageStep struct {
	label string
	state string
}

var pipelinePhases = []lifecycle.Phase{lifecycle.Preparing, lifecycle.Building, lifecycle.Sealing, lifecycle.Booting, lifecycle.Testing, lifecycle.Reporting}

func stageSteps(r api.RunItem) []stageStep {
	steps := make([]stageStep, len(pipelinePhases))
	at := slices.Index(pipelinePhases, r.Phase)
	failed := slices.Index(pipelinePhases, r.FailedStage)
	for i, p := range pipelinePhases {
		steps[i] = stageStep{label: lifecycle.StageLabel(p), state: stageState(r, i, at, failed)}
	}
	return steps
}

func stageState(r api.RunItem, i, at, failed int) string {
	switch {
	case r.Phase == lifecycle.Queued:
		return "todo"
	case at >= 0:
		return order(i, at, "now", "todo")
	case failed >= 0:
		return order(i, failed, "failed", "skip")
	case r.StartedAt == "":
		return "skip"
	}
	return "done"
}

func order(i, pivot int, here, after string) string {
	switch {
	case i < pivot:
		return "done"
	case i == pivot:
		return here
	}
	return after
}

func keepFrozen(r api.RunItem) bool {
	return r.Mark != "" || r.Phase == lifecycle.Destroying || r.Phase == lifecycle.Done && (r.Env == lifecycle.EnvAbsent || r.Env == lifecycle.EnvDestroyed)
}

func allowed(d *api.RunDetail, e lifecycle.Event) bool {
	return d != nil && slices.Contains(d.AllowedEvents, e)
}

func needsForce(op string, r api.RunItem) bool {
	return (op == "stop" || op == "destroy" || op == "unkeep") && r.Submitter != api.UISubmitter
}

func confirmText(op string, r api.RunItem) string {
	owner := ""
	if r.Submitter != api.UISubmitter {
		owner = " This run was submitted by " + r.Submitter + "."
	}
	var what string
	switch {
	case op == "stop":
		what = "In-memory state of the stand-ins is lost, and resume works only while the running environments are under the cap."
	case op == "abort":
		what = "Stops the running suite. The environment is stopped like a finished run and its data stays."
	case op == "unkeep":
		if r.Submitter == api.UISubmitter {
			return ""
		}
		what = "Releases keep. The run becomes a candidate for the kept pool cleanup."
	case r.Phase == lifecycle.Queued:
		what = "Removes it from the queue."
	case isActive(r):
		what = "Stops the run and removes the environment. An aborted report remains."
	default:
		what = "Removes the volumes too. Only the report remains."
		if r.Keep {
			what += " This environment has keep on."
		}
	}
	return r.ID + " · " + short(r.SHA) + " · " + r.Suite + " — " + what + owner
}

var doneTexts = map[string]string{
	"keep": "keep is on", "unkeep": "keep is released", "stop": "stopped", "resume": "resumed", "destroy": "destroyed", "abort": "abort signal sent",
}

func doneText(op string) string { return doneTexts[op] }

func (st *state) detail() *api.RunDetail {
	d, err := st.s.d.Reader.Run(st.run.ID)
	if err != nil {
		return nil
	}
	return &d
}

type job struct {
	id    string
	title string
	lines []string
	code  *int
}

func (j job) text() string { return joinLines(j.lines) }

func joinLines(lines []string) string { return strings.Join(lines, "\n") }

func (st *state) jobs() []job {
	if st.s.d.Bus == nil {
		return nil
	}
	var out []job
	at := map[string]int{}
	for _, e := range st.s.d.Bus.Recent(st.run.ID) {
		ev, ok := e.Body.(sse.JobEvent)
		if e.Kind != sse.KindJob || !ok || e.Target != st.run.ID {
			continue
		}
		i, seen := at[ev.JobID]
		if !seen {
			i = len(out)
			at[ev.JobID] = i
			out = append(out, job{id: ev.JobID, title: ev.Title})
		}
		switch ev.Phase {
		case sse.JobStart:
			if ev.Text != "" {
				out[i].lines = append(out[i].lines, "$ "+ev.Text)
			}
		case sse.JobOut:
			out[i].lines = append(out[i].lines, ev.Text)
		case sse.JobExit:
			out[i].code = ev.Code
		}
	}
	slices.Reverse(out)
	return out
}
