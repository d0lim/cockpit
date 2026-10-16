package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/clock"
)

const (
	planted    = "planted-token-7f3a9c"
	privSecret = "priv-secret-v4lu3x"
	ledgerAt   = "2026-10-13T00:00:00.000Z"
)

type flowSpec struct {
	ID       string   `json:"id"`
	Stage    string   `json:"stage,omitempty"`
	Once     bool     `json:"once,omitempty"`
	Requires []string `json:"requires,omitempty"`
	Steps    []string `json:"steps"`
}

var core = []flowSpec{
	{ID: "alpha", Stage: "boot", Once: true, Steps: []string{"a1"}},
	{ID: "beta", Requires: []string{"alpha"}, Steps: []string{"b1", "b2", "b3"}},
	{ID: "gamma", Requires: []string{"beta"}, Steps: []string{"g1"}},
}

const fakeFlows = `import { type Ctx, flow, step } from "cockpit:flow";

export interface Spec { id: string; stage?: string; once?: boolean; requires?: string[]; steps: string[] }

export function makeFlow(spec: Spec, fail: string[]) {
	const requires = spec.requires ?? [];
	return flow({
		id: spec.id,
		title: spec.id + " flow",
		stage: spec.stage ?? "use",
		once: spec.once ?? false,
		requires,
		steps: spec.steps.map((id) =>
			step({
				id,
				title: spec.id + " " + id,
				kind: "request",
				run: async (ctx: Ctx) => {
					for (const r of requires) ctx.get(r + ".id");
					if (id.startsWith("noop")) return;
					if (id.startsWith("long")) throw new Error("x".repeat(495) + ctx.priv.credential);
					if (id.startsWith("log")) console.log("cred=" + ctx.priv.credential);
					const res = await ctx.http.send("GET", String(ctx.env.baseUrl) + "/" + spec.id + "/" + id);
					const body = JSON.parse(res.body) as { token: string; id: string };
					ctx.capture(spec.id + "." + id + ".token", body.token);
					ctx.capture(spec.id + ".id", body.id, { persist: true });
					if (fail.includes(spec.id + "/" + id)) throw new Error("unexpected response: token=" + body.token + " cred=" + ctx.priv.credential + " step=" + id);
				},
			}),
		),
	});
}
`

type world struct {
	t       *testing.T
	root    string
	dir     string
	scripts string
	env     map[string]string
	mu      sync.Mutex
	calls   []string
	during  func(path string)
	addr    string
}

func newWorld(t *testing.T, specs []flowSpec, fail []string) *world {
	t.Helper()
	w := &world{t: t, root: t.TempDir()}
	w.dir = filepath.Join(w.root, "e2e")
	w.scripts = filepath.Join(w.root, "scripts")
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(w.dir, 0o755))
	must(os.MkdirAll(filepath.Join(w.scripts, "suites"), 0o755))
	must(os.MkdirAll(filepath.Join(w.scripts, "flows"), 0o755))
	must(os.MkdirAll(filepath.Join(w.root, ".cockpit"), 0o755))
	must(os.WriteFile(filepath.Join(w.root, ".cockpit", "project.yaml"), []byte("hostApi: 1\nscripts:\n  dir: scripts\n"), 0o644))
	must(os.WriteFile(filepath.Join(w.dir, "e2e.private.env.json"), []byte(`{"local":{"credential":"`+privSecret+`"}}`), 0o600))
	must(os.WriteFile(filepath.Join(w.scripts, "env.json"), []byte(`{"baseUrl":"http://fake.local"}`), 0o644))
	must(os.WriteFile(filepath.Join(w.scripts, "flows", "fake.ts"), []byte(fakeFlows), 0o644))
	specJSON, _ := json.Marshal(specs)
	failJSON, _ := json.Marshal(append([]string{}, fail...))
	suite := "import { stage, suite } from \"cockpit:flow\";\nimport { makeFlow, type Spec } from \"../flows/fake.ts\";\n" +
		"const FAIL: string[] = " + string(failJSON) + ";\nconst SPECS: Spec[] = " + string(specJSON) + ";\n" +
		"export default suite({\n\tname: \"fake\",\n\tstages: [stage({ id: \"boot\", title: \"Bootstrap\" }), stage({ id: \"inbound\", title: \"Inbound\", notProven: \"no stand-in yet\" }), stage({ id: \"use\", title: \"Use\" })],\n\tflows: SPECS.map((s) => makeFlow(s, FAIL)),\n});\n"
	must(os.WriteFile(filepath.Join(w.scripts, "suites", "fake.ts"), []byte(suite), 0o644))
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		w.calls = append(w.calls, r.URL.Path)
		n++
		id := n
		during := w.during
		w.mu.Unlock()
		if during != nil {
			during(r.URL.Path)
		}
		rw.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(rw, `{"token":"%s","id":"id-%d"}`, planted, id)
	}))
	t.Cleanup(srv.Close)
	w.addr = srv.Listener.Addr().String()
	w.env = map[string]string{
		"E2E_DIR":             w.dir,
		"E2E_ENV":             filepath.Join(w.scripts, "env.json"),
		"COCKPIT_SCRIPTS":     w.scripts,
		"COCKPIT_SOURCES":     filepath.Join(w.root, "source"),
		"COCKPIT_DECLARATION": filepath.Join(w.root, ".cockpit", "project.yaml"),
	}
	return w
}

func fakeClock() clock.Clock {
	t := time.Date(2026, 10, 13, 1, 0, 0, 0, time.UTC)
	return clock.Clock{Now: func() time.Time { t = t.Add(time.Second); return t }}
}

type outcome struct {
	code   int
	stdout []string
	stderr []string
}

func (w *world) run(extra map[string]string, args ...string) outcome {
	w.t.Helper()
	env := map[string]string{}
	for k, v := range w.env {
		env[k] = v
	}
	for k, v := range extra {
		env[k] = v
	}
	var out, errOut strings.Builder
	code := Main(context.Background(), append([]string{"fake"}, args...), func(k string) string { return env[k] }, Deps{
		Stdout: &out,
		Stderr: &errOut,
		Clock:  fakeClock(),
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, w.addr)
		},
	})
	return outcome{code: code, stdout: lines(out.String()), stderr: lines(errOut.String())}
}

func lines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return []string{}
	}
	return strings.Split(s, "\n")
}

func (w *world) read(name string, v any) {
	w.t.Helper()
	raw, err := os.ReadFile(filepath.Join(w.dir, name))
	if err != nil {
		w.t.Fatal(err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		w.t.Fatalf("%s: %v\n%s", name, err, raw)
	}
}

func (w *world) exists(name string) bool {
	_, err := os.Stat(filepath.Join(w.dir, name))
	return err == nil
}

func (w *world) seed(statuses map[string]contract.FlowStatus, captures map[string]string) {
	w.t.Helper()
	ledger := contract.Ledger{SchemaVersion: contract.RunnerSchemaVersion, Flows: map[string]contract.LedgerEntry{}}
	for id, status := range statuses {
		ledger.Flows[id] = contract.LedgerEntry{Status: status, At: ledgerAt}
	}
	if captures == nil {
		captures = map[string]string{}
	}
	ctx := contract.Context{SchemaVersion: contract.RunnerSchemaVersion, Captures: captures, TOTPSteps: map[string]int64{}}
	for name, v := range map[string]any{"ledger.json": ledger, "context.json": ctx} {
		if err := WriteJSON(filepath.Join(w.dir, name), v); err != nil {
			w.t.Fatal(err)
		}
	}
}

func (w *world) ledgerStatus(id string) contract.FlowStatus {
	var ledger contract.Ledger
	w.read("ledger.json", &ledger)
	return ledger.Flows[id].Status
}

func (w *world) paths() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string{}, w.calls...)
}

func statuses(steps []contract.ResultStep) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = string(s.Status)
	}
	return out
}

func stepIDs(steps []contract.ResultStep) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = s.Flow + "/" + s.ID
	}
	return out
}
