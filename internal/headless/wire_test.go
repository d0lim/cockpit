package headless

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/cli"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/wire"
)

const wireSection = `wire:
  routes:
    - { name: app, service: app, port: 8080 }
  publicFields:
    - { key: token, value: "tok-[0-9a-f]{8,64}" }
`

var wireMutations = []porttest.Mutation{
	{"a commit without routes refuses --wire before the run", "pass false for wire to WireRefusal in the declaration step", "red: the run claims an id, starts and closes as error with exit 2", "TestWireOnACommitWithoutRoutesIsRefusedBeforeTheRunClaimsAnID"},
	{"the take masks with this run's declaration", "pass an empty declaration to the RunWire of recorder", "red: the planted value shows in the copied calls.jsonl", "TestARunWithWireCarriesItsMaskedSuiteRecordInItsBundleCopyAndArchive"},
	{"the report takes the suite record of a recording run", "return nil from recorder", "red: the bundle has no wire/suite and summary.wire is absent", "TestARunWithWireCarriesItsMaskedSuiteRecordInItsBundleCopyAndArchive"},
	{"a recording run renders the recorder", "stop passing Wire to the stages.Env in Run", "red: the harness has no recorder, so the runner leaves no captured values and the record is withheld", "TestARunWithWireCarriesItsMaskedSuiteRecordInItsBundleCopyAndArchive"},
	{"a run without --wire records nothing", "set Wire on every stages.Env in Run", "red: the kept harness of the plain run holds the recorder", "TestARunWithoutWireRendersNoRecorderEvenWhenTheCommitDeclaresRoutes"},
}

func TestWireMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "wire_test.go", wireMutations)
}

func declareWire(t *testing.T) func(dir string) {
	return func(dir string) {
		t.Helper()
		path := filepath.Join(dir, ".cockpit", "project.yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(data, wireSection...), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func recordedCalls() []wire.Call {
	start := time.Date(2026, 10, 19, 1, 0, 3, 100_000_000, time.UTC)
	call := func(seq int64, request, response string) wire.Call {
		at := start.Add(time.Duration(seq) * time.Millisecond)
		return wire.Call{
			Seq: seq, Caller: "cockpit-runner", Target: "app", Port: 8080, Method: "POST", Path: "/orders",
			Started: at, Finished: at.Add(5 * time.Millisecond), Status: 200, Outcome: wire.OutcomeAnswered,
			Request:  wire.Message{Size: int64(len(request)), Body: wire.Body{Kind: wire.BodyText, Text: request}},
			Response: wire.Message{Size: int64(len(response)), Body: wire.Body{Kind: wire.BodyText, Text: response}},
		}
	}
	return []wire.Call{call(1, "token "+planted, "created"), call(2, "plain request", "plain response")}
}

func TestARunWithWireCarriesItsMaskedSuiteRecordInItsBundleCopyAndArchive(t *testing.T) {
	dir := t.TempDir()
	out, archive := filepath.Join(dir, "bundle"), filepath.Join(dir, "run.tar.gz")
	f := newFake(func(f *fakedocker.Fake) { f.Steps, f.Wire.Calls = timedSteps(), recordedCalls() })
	code, text := runFake(t, toyProject(t, declareWire(t)), f, Options{Wire: true, Out: out, Archive: archive})
	if code != cli.ExitOK {
		t.Fatalf("exit %d\n%s", code, text)
	}
	meta, summary := readJSON(t, filepath.Join(out, "meta.json")), readJSON(t, filepath.Join(out, "summary.json"))
	status, _ := summary["wire"].(map[string]any)
	if meta["wire"] != true || status["state"] != "taken" || status["calls"] != float64(2) || !slices.Equal(f.Wire.Taken(), []string{wire.SuiteTake}) {
		t.Fatalf("meta wire %v, summary wire %v, takes %v\n%s", meta["wire"], status, f.Wire.Taken(), text)
	}
	calls, err := os.ReadFile(filepath.Join(out, "wire", wire.SuiteTake, wire.CallsFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "plain request") {
		t.Fatalf("canary: the copied calls lack the plain call:\n%s", calls)
	}
	if strings.Contains(string(calls), planted) || !strings.Contains(string(calls), "token ****") {
		t.Fatalf("the copied calls must hold **** in place of the planted value:\n%s", calls)
	}
	if _, err := os.Stat(filepath.Join(out, "wire", wire.SuiteTake, wire.IndexFile)); err != nil {
		t.Fatal(err)
	}
	id, _ := summary["runId"].(string)
	entries := archiveEntries(t, archive)
	for _, name := range []string{wire.CallsFile, wire.IndexFile} {
		if _, ok := entries[id+"/report/wire/"+wire.SuiteTake+"/"+name]; !ok {
			t.Errorf("the archive lacks report/wire/%s/%s", wire.SuiteTake, name)
		}
	}
}

func TestWireOnACommitWithoutRoutesIsRefusedBeforeTheRunClaimsAnID(t *testing.T) {
	f := newFake(nil)
	code, text := runFake(t, toyProject(t), f, Options{Wire: true})
	if code != cli.ExitRejected || !strings.Contains(text, "declares no wire.routes, so this run cannot record") {
		t.Fatalf("exit %d\n%s", code, text)
	}
	verbs := 0
	for _, c := range f.Calls() {
		verbs++
		if verb := fakedocker.Verb(c.Args); verb == "create" || strings.HasPrefix(verb, "compose") {
			t.Fatalf("the refused run ran %v", c.Args)
		}
	}
	if !strings.Contains(text, "ok   project") {
		t.Fatalf("canary: the run did not reach the declaration step (%d docker calls)\n%s", verbs, text)
	}
}

var keptDir = regexp.MustCompile(`kept: run directory (\S+);`)

func TestARunWithoutWireRendersNoRecorderEvenWhenTheCommitDeclaresRoutes(t *testing.T) {
	code, text := runFake(t, toyProject(t, declareWire(t)), newFake(nil), Options{Keep: true})
	m := keptDir.FindStringSubmatch(text)
	if code != cli.ExitOK || m == nil {
		t.Fatalf("exit %d\n%s", code, text)
	}
	harness, err := os.ReadFile(filepath.Join(m[1], compose.HarnessFile))
	if err != nil {
		t.Fatal(err)
	}
	runEnv, err := os.ReadFile(filepath.Join(m[1], compose.RunEnvFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(harness), "  "+compose.RoleRunner+":\n") || !strings.Contains(string(runEnv), "RUN_ID=") {
		t.Fatalf("canary: the harness lacks the runner or the run env is empty:\n%s\n%s", harness, runEnv)
	}
	if strings.Contains(string(harness), compose.RoleWire) || strings.Contains(string(runEnv), "COCKPIT_WIRE_") {
		t.Fatalf("the plain run renders the recorder:\n%s\n%s", harness, runEnv)
	}
}
