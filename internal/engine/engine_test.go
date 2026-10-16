package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

var engineMutations = []mutation{
	{"hiding Go error text", "make Host.Error use err.Error() as the message", "red: PLANTED_GO_ERROR is visible to the script", "TestPortAsyncResolvesPlainJSValuesAndRejectsGoErrorsWithStableCodes"},
	{"plain JS value conversion of port results", "make Async use rt.ToValue instead of h.Value", "red: Go field names and the Credential method are visible to the script", "TestPortAsyncResolvesPlainJSValuesAndRejectsGoErrorsWithStableCodes"},
	{"completion guard after the result is settled", "delete the done and finished checks in callback", "red: late completion woke the script is recorded", "TestPortCompletionArrivingAfterTheResultIsSettledDoesNotWakeTheScript"},
	{"the deadline stops a waiting loop", "delete StopNoWait from the deadline watcher", "red: a run waiting on a port never ends and hits the 20 second test deadline", "TestCompletionOfAPortJobThatFinishesAfterTheDeadlineDoesNothing"},
	{"inline source map", "change Sourcemap to SourceMapNone", "red: the stack points at bundle.js:29", "TestStackOfAnErrorThrownInAStepPointsToTheTSFileAndLine"},
	{"load failure kind for the entry export", "classify a non-function entry as thrown", "red: the kind is thrown instead of load", "TestEntryExportThatIsNotAFunctionIsALoadFailure"},
	{"watching the parent context for cancellation", "make Run derive from context.Background instead of the parent", "red: the run ends with deadline after 10 seconds instead of canceled", "TestCanceledParentContextEndsTheRunAsCanceled"},
	{"rejecting modules configured for another site", "replace the site.allows check in New with false", "red: cockpit:collect on the runner and cockpit:flow on the hub are accepted", "TestNewRejectsModulesForAnotherSiteAndAnEmptyDeadlineAndAcceptsAValidConfig"},
	{"structured log path for console", "replace printer.Log with an empty function", "red: the INFO line disappears", "TestScriptConsoleGoesToTheStructuredLogAndKeepsItsLevel"},
}

func TestPortAsyncResolvesPlainJSValuesAndRejectsGoErrorsWithStableCodes(t *testing.T) {
	p := newProbe(SiteRunner)
	err := runSource(t, SiteRunner, p, `import { echo, deny, leak, badArgs, record } from "cockpit:flow";

const describe = (e: any): string => [e instanceof Error, e.code, e.message].join("|");

export default async function (): Promise<void> {
	const v = await echo();
	record([JSON.stringify(v), typeof v.Credential, Object.getPrototypeOf(v) === Object.prototype, Array.isArray(v.items)].join("|"));
	await deny().catch((e: unknown) => record(describe(e)));
	await leak().catch((e: unknown) => record(describe(e)));
	try {
		badArgs();
	} catch (e) {
		record(describe(e));
	}
}
`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`{"status":201,"items":["a","b"]}|undefined|true|true`,
		"true|PORT_DENIED|denied by policy ****",
		"true|PORT_UNAVAILABLE|port call failed",
		"true|PORT_BAD_ARGS|maxRows must be a number",
	}
	if !slices.Equal(p.records, want) || strings.Contains(strings.Join(p.records, "\n"), "PLANTED_GO") {
		t.Fatalf("records %q, want %q", p.records, want)
	}
}

func TestCompletionOfAPortJobThatFinishesAfterTheDeadlineDoesNothing(t *testing.T) {
	p := newProbe(SiteRunner)
	root := writeTree(t, map[string]string{"entry.ts": `import { wait, record } from "cockpit:flow";

export default async function (): Promise<void> {
	record("started");
	record(String(await wait()));
}
`})
	b := mustBundle(t, root, "entry.ts", SiteRunner)
	eng := newEngine(t, SiteRunner, p, 200*time.Millisecond)
	before := runtime.NumGoroutine()
	failureOf(t, eng.Run(context.Background(), b, "default"), FailDeadline)
	close(p.release)
	until := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(until) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Fatalf("port goroutines leaked: %d > %d", n, before)
	}
	if !slices.Equal(p.records, []string{"started"}) {
		t.Fatalf("late completion woke the script: %q", p.records)
	}
}

func TestPortCompletionArrivingAfterTheResultIsSettledDoesNotWakeTheScript(t *testing.T) {
	p := newProbe(SiteRunner)
	err := runSource(t, SiteRunner, p, `import { hold, releaseInOrder, record } from "cockpit:flow";

export default async function (): Promise<void> {
	const first = hold("a");
	const second = hold("b");
	second.then(() => record("late completion woke the script"));
	releaseInOrder();
	const until = Date.now() + 60;
	while (Date.now() < until) {}
	record(String(await first));
}
`)
	if err != nil || !slices.Equal(p.records, []string{"released"}) {
		t.Fatalf("records %q, run %v", p.records, err)
	}
}

func TestStackOfAnErrorThrownInAStepPointsToTheTSFileAndLine(t *testing.T) {
	root := writeTree(t, map[string]string{
		"flows/step.ts":  "export function run(): void {\n\tthrow new Error(\"step failed\");\n}\n",
		"suites/main.ts": "import { run } from \"../flows/step\";\n\nexport default function (): void {\n\trun();\n}\n",
	})
	err := newEngine(t, SiteRunner, nil, 5*time.Second).Run(context.Background(), mustBundle(t, root, "suites/main.ts", SiteRunner), "default")
	f := failureOf(t, err, FailThrown)
	if f.Message != "step failed" || !strings.Contains(f.Stack, "at run (flows/step.ts:2:") || !strings.Contains(f.Stack, "suites/main.ts:4:") {
		t.Fatalf("message %q stack %q", f.Message, f.Stack)
	}
}

func TestEntryExportThatIsNotAFunctionIsALoadFailure(t *testing.T) {
	f := failureOf(t, runSource(t, SiteRunner, nil, "export const value = 1;\n"), FailLoad)
	if !strings.Contains(f.Message, `"default"`) {
		t.Fatalf("message %q", f.Message)
	}
}

func TestCanceledParentContextEndsTheRunAsCanceled(t *testing.T) {
	root := writeTree(t, map[string]string{"entry.ts": "export default function (): void {\n\tfor (;;) {}\n}\n"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(100*time.Millisecond, cancel)
	err := newEngine(t, SiteRunner, nil, 10*time.Second).Run(ctx, mustBundle(t, root, "entry.ts", SiteRunner), "default")
	failureOf(t, err, FailCanceled)
}

func TestNewRejectsModulesForAnotherSiteAndAnEmptyDeadlineAndAcceptsAValidConfig(t *testing.T) {
	builder := func(*Host) (goja.Value, error) { return goja.Undefined(), nil }
	cases := []struct {
		opts Options
		want string
	}{
		{Options{Site: SiteRunner, Budget: Budget{Deadline: time.Second}, Modules: map[string]ModuleBuilder{ModuleCollect: builder}}, `"cockpit:collect"`},
		{Options{Site: SiteHub, Budget: Budget{Deadline: time.Second}, Modules: map[string]ModuleBuilder{ModuleFlow: builder}}, `"cockpit:flow"`},
		{Options{Site: SiteRunner, Budget: Budget{Deadline: time.Second}, Modules: map[string]ModuleBuilder{ModuleFlow: nil}}, "nil"},
		{Options{Site: SiteRunner}, "deadline"},
		{Options{Site: SiteRunner, Budget: Budget{Deadline: time.Second, MaxCallStack: -1}}, "call stack"},
		{Options{Budget: Budget{Deadline: time.Second}}, "site"},
		{Options{Site: SiteRunner, Budget: Budget{Deadline: time.Second}, Modules: map[string]ModuleBuilder{ModuleFlow: builder}}, ""},
		{Options{Site: SiteHub, Budget: Budget{Deadline: time.Second}, Modules: map[string]ModuleBuilder{ModuleCollect: builder}}, ""},
	}
	for i, c := range cases {
		_, err := New(c.opts)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%d: valid config was rejected: %v", i, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%d: want a rejection mentioning %q, got %v", i, c.want, err)
		}
	}
}

func TestScriptConsoleGoesToTheStructuredLogAndKeepsItsLevel(t *testing.T) {
	var out bytes.Buffer
	root := writeTree(t, map[string]string{"entry.ts": "export default function (): void {\n\tconsole.log(\"seen\", 1);\n\tconsole.error(\"broken %s\", \"x\");\n}\n"})
	eng, err := New(Options{Site: SiteRunner, Budget: Budget{Deadline: 5 * time.Second}, Log: slog.New(slog.NewJSONHandler(&out, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Run(context.Background(), mustBundle(t, root, "entry.ts", SiteRunner), "default"); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var entry struct{ Level, Msg, Text string }
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("not a structured log line: %q", line)
		}
		lines = append(lines, entry.Level+"|"+entry.Msg+"|"+entry.Text)
	}
	if want := []string{"INFO|console|seen 1", "ERROR|console|broken x"}; !slices.Equal(lines, want) {
		t.Fatalf("logs %q, want %q", lines, want)
	}
}

func TestEngineMutationRecordAndTestsInThisFileMatchBothWays(t *testing.T) {
	assertMutationRecord(t, "engine_test.go", engineMutations)
}
