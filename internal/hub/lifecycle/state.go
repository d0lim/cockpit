package lifecycle

import (
	"encoding/json"

	"github.com/d0lim/cockpit/internal/contract"
)

type Phase string

const (
	Queued     Phase = "queued"
	Preparing  Phase = "preparing"
	Building   Phase = "building"
	Sealing    Phase = "sealing"
	Booting    Phase = "booting"
	Testing    Phase = "testing"
	Reporting  Phase = "reporting"
	Stopping   Phase = "stopping"
	Resuming   Phase = "resuming"
	Destroying Phase = "destroying"
	Done       Phase = "done"
)

type Env string

const (
	EnvAbsent    Env = "absent"
	EnvUp        Env = "up"
	EnvStopped   Env = "stopped"
	EnvDestroyed Env = "destroyed"
)

type Verdict string

const (
	VerdictNone    Verdict = ""
	VerdictPassed  Verdict = Verdict(contract.VerdictPassed)
	VerdictFailed  Verdict = Verdict(contract.VerdictFailed)
	VerdictError   Verdict = Verdict(contract.VerdictError)
	VerdictAborted Verdict = Verdict(contract.VerdictAborted)
)

func (v Verdict) MarshalJSON() ([]byte, error) {
	if v == VerdictNone {
		return []byte("null"), nil
	}
	return json.Marshal(string(v))
}

func (v *Verdict) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*v = VerdictNone
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*v = Verdict(s)
	return nil
}

type Mark string

const (
	MarkNone Mark = ""
	Lost     Mark = "lost"
	Orphan   Mark = "orphan"
)

type State struct {
	Phase   Phase   `json:"phase"`
	Env     Env     `json:"env"`
	Verdict Verdict `json:"verdict"`
	Mark    Mark    `json:"mark,omitempty"`
}

type Key string

const KeyNew Key = "new"

func (s State) Key() Key {
	if s.Mark != MarkNone {
		return Key(s.Mark)
	}
	if s.Phase == Done {
		return Key("done/" + string(s.Env))
	}
	return Key(s.Phase)
}

var stateLabels = map[Key]string{
	"queued":         "Queued",
	"preparing":      "Preparing",
	"building":       "Building",
	"sealing":        "Checking the seal",
	"booting":        "Booting",
	"testing":        "Running",
	"reporting":      "Reporting",
	"done/up":        "Done (up)",
	"done/stopped":   "Stopped",
	"done/absent":    "Closed (no environment)",
	"done/destroyed": "Destroyed",
	"stopping":       "Stopping",
	"resuming":       "Resuming",
	"destroying":     "Destroying",
	"lost":           "Lost",
	"orphan":         "Orphan",
}

func Label(s State) string {
	return stateLabels[s.Key()]
}

var stageLabels = map[Phase]string{
	Preparing: "prepare",
	Building:  "build",
	Sealing:   "seal check",
	Booting:   "boot",
	Testing:   "run",
	Reporting: "report",
}

func StageLabel(p Phase) string {
	if p == "" {
		return "-"
	}
	if label, ok := stageLabels[p]; ok {
		return label
	}
	return string(p)
}

var active = map[Phase]bool{Preparing: true, Building: true, Sealing: true, Booting: true, Testing: true, Reporting: true}

var inProgress = map[Phase]bool{Stopping: true, Resuming: true, Destroying: true}

func (s State) Active() bool { return s.Mark == MarkNone && active[s.Phase] }

func (s State) InProgress() bool { return s.Mark == MarkNone && inProgress[s.Phase] }
