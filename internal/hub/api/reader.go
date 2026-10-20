package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/sse"
	"github.com/d0lim/cockpit/internal/hub/ticker"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/signal"
)

type Reader struct{ a *api }

func NewReader(d Deps) *Reader { return &Reader{a: &api{d: d}} }

func (r *Reader) List() (HubList, error) {
	l, err := r.a.d.Scheduler.List()
	if err != nil {
		return HubList{}, err
	}
	return PublicList(l, r.a.hubs()), nil
}

func (r *Reader) run(id string) (*scheduler.RunView, string, error) {
	v, err := r.a.runByID(id)
	if err != nil {
		return nil, "", err
	}
	dir, err := r.a.runDir(v.ID)
	return v, dir, err
}

func (r *Reader) Run(id string) (RunDetail, error) {
	v, err := r.a.runByID(id)
	if err != nil {
		return RunDetail{}, err
	}
	var services []ServicePort
	if r.a.d.Services != nil && holdsPorts(v.Meta) {
		services = r.a.d.Services(v.Meta)
	}
	return Detail(*v, r.a.d.Scheduler.AllowedEvents(v.Meta), services), nil
}

func (r *Reader) Report(id string) (ReportView, error) {
	v, dir, err := r.run(id)
	if err != nil {
		return ReportView{}, err
	}
	return reportView(dir, v.Meta), nil
}

func (r *Reader) Kit(id string) (KitView, error) {
	v, dir, err := r.run(id)
	if err != nil {
		return KitView{}, err
	}
	view := KitView{ID: v.ID, Env: v.Env, Services: []ServicePort{}, ReportDir: filepath.Join(dir, report.DirReport)}
	if holdsPorts(v.Meta) {
		if r.a.d.Services != nil {
			view.Services = r.a.d.Services(v.Meta)
		}
		if _, f := report.ReadConfined(dir, KitReadme); f == report.ReadOK {
			view.Kit = &KitFiles{Dir: filepath.Join(dir, KitDir), Readme: filepath.Join(dir, KitReadme)}
		}
		if execAccepts(v.Meta) {
			view.Exec = &KitExec{Command: ExecCommand + v.ID + " -- <flow id>", Flows: repeatableFlows(dir)}
		}
	}
	return view, nil
}

func (r *Reader) Flows(id string) (string, []contract.FlowSummary, error) {
	v, dir, err := r.run(id)
	if err != nil {
		return "", nil, err
	}
	if s := r.a.bundleSummary(dir); s != nil {
		return SourceBundle, s.Flows, nil
	}
	c := report.ReadRunnerFiles(filepath.Join(dir, RunnerDir))
	flows := report.FlowSummaries(c.Manifest, c.Result)
	if r.a.d.Mask != nil {
		m := r.a.d.Mask(v.ID)
		for i := range flows {
			for j := range flows[i].StepList {
				flows[i].StepList[j].Message = m.Mask(flows[i].StepList[j].Message)
			}
		}
	}
	return SourceLive, flows, nil
}

func (r *Reader) Flow(id, flow string) (FlowView, error) {
	source, flows, err := r.Flows(id)
	if err != nil {
		return FlowView{}, err
	}
	for _, f := range flows {
		if f.ID == flow {
			return FlowView{Source: source, Flow: f}, nil
		}
	}
	return FlowView{}, notFound("no such flow in this run: " + flow)
}

func (r *Reader) Source(id, file string) (SourceView, error) {
	v, dir, err := r.run(id)
	if err != nil {
		return SourceView{}, err
	}
	if !filepath.IsLocal(file) {
		return SourceView{}, badRequest("file is a path inside the suite")
	}
	view := SourceView{Source: SourceBundle, File: file}
	var data []byte
	failure := report.ReadMissing
	if r.a.bundleSummary(dir) != nil {
		data, failure = report.ReadConfined(filepath.Join(dir, report.DirReport, report.BundleSource), file)
	} else {
		view.Source = SourceLive
		c := report.ReadRunnerFiles(filepath.Join(dir, RunnerDir))
		if c.Manifest.Value == nil || !slices.Contains(manifestFiles(c.Manifest.Value), file) {
			return SourceView{}, notFound("the manifest does not point to " + file)
		}
		if r.a.d.Mask == nil {
			return SourceView{}, &Problem{Status: http.StatusConflict, Code: CodeNotAvailable, Message: "no run masker: live source is not served"}
		}
		data, failure = report.ReadConfined(filepath.Join(dir, ScriptsDir), file)
		data = []byte(r.a.d.Mask(v.ID).Mask(string(data)))
	}
	if failure != report.ReadOK {
		return SourceView{}, notFound("the source file cannot be read: " + string(failure))
	}
	view.Text = string(data)
	view.Lines = strings.Count(view.Text, "\n") + 1
	return view, nil
}

func (r *Reader) Signals(id string, after int64, limit int) (SignalsView, error) {
	v, err := r.a.runByID(id)
	if err != nil {
		return SignalsView{}, err
	}
	if r.a.d.Signals == nil {
		return SignalsView{}, &Problem{Status: http.StatusConflict, Code: CodeNotAvailable, Message: "this hub has no signal store"}
	}
	resp, err := r.a.d.Signals.Read(v.ID, signal.ReadRequest{After: after, Limit: min(limit, maxSignalRead)})
	if errors.Is(err, signal.ErrRunDestroyed) {
		return SignalsView{}, notFound("the run was destroyed and its live signals were removed; the report bundle keeps its frozen copy")
	}
	if errors.Is(err, signal.ErrReadRefused) {
		return SignalsView{}, &Problem{Status: http.StatusConflict, Code: CodeReadRefused, Message: "signal reads are refused for this run"}
	}
	if err != nil {
		return SignalsView{}, err
	}
	return SignalsView{ReadResponse: resp, Collectors: r.Collectors(v.ID)}, nil
}

func (r *Reader) Collectors(id string) []CollectorState {
	if r.a.d.Collectors != nil {
		if states := r.a.d.Collectors.Status(id); states != nil {
			return states
		}
	}
	return []CollectorState{}
}

func (r *Reader) Operate(id, op string, actor scheduler.Actor) (OpReply, int, error) {
	if !slices.Contains(Ops, op) {
		return OpReply{}, 0, notFound("not found")
	}
	v, err := r.a.runByID(id)
	if err != nil {
		return OpReply{}, 0, err
	}
	s := r.a.d.Scheduler
	ops := map[string]func(string, scheduler.Actor) ticker.Result{
		"keep": s.Keep, "unkeep": s.Unkeep, "stop": s.Stop, "resume": s.Resume, "destroy": s.Destroy, "abort": s.Abort,
	}
	res := ops[op](v.ID, actor)
	r.a.changed()
	if res.Fail != nil {
		return OpReply{}, 0, fromFailure(res.Fail)
	}
	status := http.StatusOK
	if op == "abort" {
		status = http.StatusAccepted
	}
	return OpReply{Run: MetaItem(res.Run)}, status, nil
}

func (r *Reader) Exec(id, flow string, actor scheduler.Actor) (ExecView, error) {
	v, err := r.a.runByID(id)
	if err != nil {
		return ExecView{}, err
	}
	if flow == "" {
		return ExecView{}, badRequest("flow is a flow id string")
	}
	res := r.a.d.Scheduler.Exec(v.ID, flow, actor)
	r.a.changed()
	if res.Fail != nil {
		return ExecView{}, fromFailure(res.Fail)
	}
	raw, err := json.Marshal(res.Result)
	if err != nil {
		return ExecView{}, err
	}
	var view ExecView
	if err := json.Unmarshal(raw, &view); err != nil {
		return ExecView{}, err
	}
	if view.Flows == nil {
		view.Flows = []ExecFlow{}
	}
	if view.Lines == nil {
		view.Lines = []string{}
	}
	return view, nil
}

func (r *Reader) SignalSource() sse.Signals { return r.a.d.Signals }
