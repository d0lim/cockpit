package screen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/a-h/templ"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/hubtest"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/sse"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ui"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/signal"
)

const (
	planted = "PLANTED-screen-secret-7d21aa"
	toyRepo = "/work/toy-repo"
)

type fixture struct {
	t       *testing.T
	st      *store.Store
	sched   *scheduler.Scheduler
	sc      *Screen
	srv     *httptest.Server
	bus     *sse.Bus
	signals *fakeSignals
}

type fakeSignals struct {
	mu        sync.Mutex
	list      []contract.Signal
	destroyed bool
}

func (f *fakeSignals) add(s ...contract.Signal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range s {
		x.Seq = int64(len(f.list) + 1)
		f.list = append(f.list, x)
	}
}

func (f *fakeSignals) Read(_ string, req signal.ReadRequest) (signal.ReadResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.destroyed {
		return signal.ReadResponse{}, signal.ErrRunDestroyed
	}
	resp := signal.ReadResponse{Signals: []contract.Signal{}, Dropped: map[string]int64{}, Next: req.After}
	for _, s := range f.list {
		if s.Seq > req.After && (req.Limit == 0 || len(resp.Signals) < req.Limit) {
			resp.Signals = append(resp.Signals, s)
			resp.Next = s.Seq
		}
	}
	return resp, nil
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st := store.Open(filepath.Join(t.TempDir(), "home"))
	w := hubtest.New(st)
	sched := scheduler.New(w.Deps())
	t.Cleanup(func() { sched.Dispose(); sched.Idle() })
	f := &fixture{t: t, st: st, sched: sched, bus: sse.New("1", 0, nil), signals: &fakeSignals{}}
	masker, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	reader := api.NewReader(api.Deps{
		Store: st, Scheduler: sched, Signals: f.signals, Mask: func(string) *mask.Masker { return masker },
		Repos: []string{toyRepo},
		Suites: func(_ context.Context, worktree, commit string) ([]string, error) {
			if worktree != toyRepo {
				return nil, errors.New(worktree + " is not listed in allowedRepositories")
			}
			if commit == "old" {
				return []string{"smoke"}, nil
			}
			return []string{"joint", "canary"}, nil
		},
		Services: func(store.Meta) []api.ServicePort {
			return []api.ServicePort{{Name: "app", Service: "app", Port: 8080, HostPort: 22101, Label: "App", Open: contract.OpenBrowser}}
		},
	})
	f.sc = New(Deps{Reader: reader, Bus: f.bus, Assets: ui.Files(), Identity: Identity{Home: st.Home(), Port: 7710}})
	f.srv = httptest.NewServer(f.sc.Handler())
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fixture) seed(m store.Meta) {
	f.t.Helper()
	if err := f.st.WriteMeta(m); err != nil {
		f.t.Fatal(err)
	}
	list, _ := f.st.ListRuns()
	f.st.WriteState(store.RebuildState(list.Runs))
}

func (f *fixture) runDir(id string) string {
	f.t.Helper()
	dir, err := f.st.RunDir(id)
	if err != nil {
		f.t.Fatal(err)
	}
	return dir
}

func (f *fixture) get(path string) (int, http.Header, string) {
	f.t.Helper()
	res, err := http.Get(f.srv.URL + path)
	if err != nil {
		f.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, string(raw)
}

func (f *fixture) post(path string, form url.Values) (int, string) {
	f.t.Helper()
	res, err := http.Post(f.srv.URL+path, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		f.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(raw)
}

func render(t *testing.T, c templ.Component) string {
	t.Helper()
	var out bytes.Buffer
	if err := c.Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func rid(n int) string { return "run-20261019-0100" + strconv.Itoa(10+n) + "-abc1234" }

func runMeta(n int, state lifecycle.State, edit ...func(*store.Meta)) store.Meta {
	m := store.Meta{State: state, ID: rid(n), SHA: hubtest.ToySHA, Suite: "toy", HostAPI: 1, CockpitVersion: "0.1.0", Submitter: "cli", CreatedAt: "2026-10-19T01:00:0" + strconv.Itoa(n) + ".000Z"}
	for _, e := range edit {
		e(&m)
	}
	return m
}

func doneUp(m *store.Meta) {
	slot := 1
	m.Slot, m.StartedAt, m.FinishedAt, m.LastUpAt = &slot, "2026-10-19T01:00:01.000Z", "2026-10-19T01:00:09.000Z", "2026-10-19T01:00:09.000Z"
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, _ := json.Marshal(v)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeRunner(t *testing.T, dir, message string) {
	t.Helper()
	manifest := contract.Manifest{SchemaVersion: contract.RunnerSchemaVersion, Suite: "toy", Stages: []contract.ManifestStage{{ID: "basic", Title: "Basic"}},
		Flows: []contract.ManifestFlow{{ID: "alpha", Title: "Alpha", Stage: "basic", Requires: []string{}, Source: &contract.ManifestSource{File: "flows/alpha.ts", Line: 1},
			Steps: []contract.ManifestStep{{ID: "a1", Title: "A1", Kind: contract.StepKindRequest, Source: &contract.ManifestSource{File: "flows/alpha.ts", Line: 2}}}}}}
	result := contract.Result{SchemaVersion: contract.RunnerSchemaVersion, Suite: "toy", Complete: true, StartedAt: "2026-10-19T01:00:00.000Z",
		Steps: []contract.ResultStep{{Flow: "alpha", ID: "a1", Status: contract.StepStatusFailed, Requests: 1, Message: message}}}
	writeJSON(t, filepath.Join(dir, api.RunnerDir, "manifest.json"), manifest)
	writeJSON(t, filepath.Join(dir, api.RunnerDir, "result.json"), result)
	src := filepath.Join(dir, api.ScriptsDir, "flows", "alpha.ts")
	if err := os.MkdirAll(filepath.Dir(src), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("const token = \""+planted+"\";\nexport const a1 = 1;\nexport default 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeBundle(t *testing.T, dir string) {
	t.Helper()
	six := 6
	summary := contract.Summary{Format: contract.SummaryFormat3, RunID: "x", Suite: "toy", Verdict: contract.VerdictFailed, Reasons: []string{"failed steps: 1, the first at alpha › a2"},
		Steps:     contract.StepCounts{Expected: &six, Actual: &six, Failed: 1, Skipped: 3, Requests: 3},
		Flows:     []contract.FlowSummary{{ID: "alpha", Stage: "basic", Title: "Alpha", Requires: []string{}, StepList: []contract.FlowStep{}, Status: contract.FlowOutcomeFailed, Steps: 3, Passed: 1, Failed: 1, Skipped: 1, StoppedAt: "a2"}},
		StoppedAt: &contract.SummaryStop{Flow: "alpha", Step: "a2", Message: "status 500"}, NotProven: contract.NotProven{Skipped: []string{"alpha › a3"}, Declared: []string{}, Unread: []string{}},
		Seal: contract.SealPassed, Stages: map[string]contract.CollectStage{"pulse": {Status: contract.CollectStageError, Detail: "deadline"}}, Ports: map[string]int{},
		Images: contract.SummaryImages{Keys: map[string]string{}, Digests: map[string]string{}}, Versions: map[string]string{}}
	bundle := filepath.Join(dir, report.DirReport)
	writeJSON(t, filepath.Join(bundle, report.FileSummary), summary)
	writeJSON(t, filepath.Join(bundle, report.FileMeta), map[string]string{"id": "x"})
	if err := os.WriteFile(filepath.Join(bundle, report.FileNotProven), []byte(report.NotProvenText("x", summary.NotProven)), 0o600); err != nil {
		t.Fatal(err)
	}
}
