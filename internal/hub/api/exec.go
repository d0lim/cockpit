package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/report"
)

const (
	RunnerDir   = "work/e2e"
	KitDir      = "env"
	KitReadme   = "env/README.md"
	ExecCommand = "cockpit runs exec "
)

type ExecStep struct {
	ID         string              `json:"id"`
	Status     contract.StepStatus `json:"status"`
	HTTPStatus *int                `json:"httpStatus,omitempty"`
	Message    string              `json:"message,omitempty"`
}

type ExecFlow struct {
	ID    string     `json:"id"`
	Steps []ExecStep `json:"steps"`
}

type ExecView struct {
	ExitCode  *int       `json:"exitCode"`
	OOMKilled bool       `json:"oomKilled"`
	TimedOut  bool       `json:"timedOut"`
	Flows     []ExecFlow `json:"flows"`
	Lines     []string   `json:"lines"`
	Reason    string     `json:"reason,omitempty"`
}

func (a *api) exec(w http.ResponseWriter, id string, body map[string]any, actor scheduler.Actor) error {
	flow, ok := body["flow"].(string)
	if !ok || flow == "" {
		return badRequest("flow is a flow id string")
	}
	res := a.d.Scheduler.Exec(id, flow, actor)
	a.changed()
	if res.Fail != nil {
		return fromFailure(res.Fail)
	}
	raw, err := json.Marshal(res.Result)
	if err != nil {
		return err
	}
	var view ExecView
	if err := json.Unmarshal(raw, &view); err != nil {
		return err
	}
	if view.Flows == nil {
		view.Flows = []ExecFlow{}
	}
	if view.Lines == nil {
		view.Lines = []string{}
	}
	writeJSON(w, http.StatusOK, view)
	return nil
}

type KitFiles struct {
	Dir    string `json:"dir"`
	Readme string `json:"readme"`
}

type KitExec struct {
	Command string   `json:"command"`
	Flows   []string `json:"flows"`
}

type KitView struct {
	ID        string        `json:"id"`
	Env       lifecycle.Env `json:"env"`
	Services  []ServicePort `json:"services"`
	Kit       *KitFiles     `json:"kit"`
	Exec      *KitExec      `json:"exec"`
	ReportDir string        `json:"reportDir"`
}

func repeatableFlows(runDir string) []string {
	c := report.ReadRunnerFiles(filepath.Join(runDir, RunnerDir))
	flows := []string{}
	if c.Manifest.Value == nil {
		return flows
	}
	for _, f := range c.Manifest.Value.Flows {
		if !f.Once {
			flows = append(flows, f.ID)
		}
	}
	return flows
}

func (a *api) kit(w http.ResponseWriter, r *http.Request) error {
	v, err := a.runOf(r)
	if err != nil {
		return err
	}
	dir, err := a.runDir(v.ID)
	if err != nil {
		return err
	}
	view := KitView{ID: v.ID, Env: v.Env, Services: []ServicePort{}, ReportDir: filepath.Join(dir, report.DirReport)}
	if holdsPorts(v.Meta) {
		if a.d.Services != nil {
			view.Services = a.d.Services(v.Meta)
		}
		if _, f := report.ReadConfined(dir, KitReadme); f == report.ReadOK {
			view.Kit = &KitFiles{Dir: filepath.Join(dir, KitDir), Readme: filepath.Join(dir, KitReadme)}
		}
		if execAccepts(v.Meta) {
			view.Exec = &KitExec{Command: ExecCommand + v.ID + " -- <flow id>", Flows: repeatableFlows(dir)}
		}
	}
	writeJSON(w, http.StatusOK, view)
	return nil
}
