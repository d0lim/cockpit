package api

import (
	"net/http"
	"path/filepath"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
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
	view, err := a.reader().Kit(r.PathValue("id"))
	if err != nil {
		return err
	}
	writeJSON(a.log(), w, http.StatusOK, view)
	return nil
}
