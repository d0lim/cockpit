package collect_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/collect/collecttest"
	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var adversarialMutations = []porttest.Mutation{
	{"the flow module is refused at the hub site", "let Site.allows accept cockpit:flow", "red: the bundle builds and the runtime require returns exports", "TestAdversarialRequireOfTheFlowModuleIsRefusedAtBundleTimeAndAtRuntime"},
	{"memory over the limit kills only the child", "skip watchRSS", "red: the huge string child is not killed", "TestAdversarialHugeStringKillsOnlyTheChildWhileTheHubKeepsAnswering"},
	{"the hub deadline kills a child stuck in a builtin", "drop the AfterFunc deadline timer", "red: the backtracking child runs past the test timeout", "TestAdversarialRegexBacktrackingIsKilledAtTheDeadlineWhileTheHubKeepsAnswering"},
	{"child output is capped", "copy stderr without the cap", "red: Stderr is larger than the cap and not truncated", "TestAdversarialHugeConsoleOutputIsTruncated"},
	{"errors sent to the child pass the hub masker", "send known.Message without Mask in reply", "red: the canary is in the frame sent to the child", "TestAdversarialCanarySecretIsAbsentFromErrorsTheScriptSees"},
	{"no global or module exposes the process environment", "set globalThis.process = { env: os.Environ() } in installGlobals", "red: the walker finds the planted value and process is a global", "TestAdversarialVariablePlantedInTheHubEnvironmentIsInvisibleInEveryGlobalAndModule"},
	{"a dead child never touches the run lifecycle", "close the active run as error on a killed outcome", "red: the run meta leaves testing", "TestAdversarialDeadChildDoesNotCloseTheActiveRunAsError"},
}

const canary = "PLANTED-canary-secret-7f3a9c"

func runChild(t *testing.T, spec worker.Spec) worker.Outcome {
	t.Helper()
	out, err := worker.Run(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAdversarialRequireOfTheFlowModuleIsRefusedAtBundleTimeAndAtRuntime(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "collector.ts"), `import { suite } from "cockpit:flow";
import { collector } from "cockpit:collect";
export default collector({ id: "toy-db", run: async () => { suite; } });`)
	if _, err := worker.Bundle(context.Background(), dir, "collector.ts"); err == nil || !strings.Contains(err.Error(), "not available at the hub site") {
		t.Fatalf("bundle: %v", err)
	}
	code := collecttest.Bundle(t, "toy-db", `
		const req = require;
		let got;
		try { got = req("cockpit" + ":flow"); } catch (e) { if (!String(e.message).includes("Invalid module")) throw e; return; }
		throw new Error("cockpit:flow loaded: " + typeof got);
	`)
	if out := runChild(t, collecttest.Spec(code, nil, nil)); out.Status != worker.StatusOK {
		t.Fatalf("%+v", out)
	}
}

func hubAnswers(t *testing.T, during func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) }))
	defer srv.Close()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	answered, failed := 0, 0
	wg.Go(func() {
		for {
			c := http.Client{Timeout: time.Second}
			if res, err := c.Get(srv.URL); err == nil && res.StatusCode == 200 {
				res.Body.Close()
				answered++
			} else {
				failed++
			}
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	})
	during()
	close(stop)
	wg.Wait()
	if answered == 0 || failed != 0 {
		t.Fatalf("hub answered %d times and failed %d times while the child ran", answered, failed)
	}
}

func TestAdversarialHugeStringKillsOnlyTheChildWhileTheHubKeepsAnswering(t *testing.T) {
	bad := collecttest.Spec(collecttest.Bundle(t, "toy-db", `
		const parts = [];
		for (let i = 0; i < 16; i++) parts.push("x".repeat(2 ** 29) + i);
		throw new Error("survived " + parts.length);
	`), nil, nil)
	bad.Limits.MaxRSS = 256 << 20
	bad.Limits.Deadline = 20 * time.Second
	good := collecttest.Spec(collecttest.Bundle(t, "toy-db", `await new Promise((r) => setTimeout(r, 200));`), nil, nil)
	var badOut, goodOut worker.Outcome
	hubAnswers(t, func() {
		var wg sync.WaitGroup
		wg.Go(func() { badOut = runChild(t, bad) })
		wg.Go(func() { goodOut = runChild(t, good) })
		wg.Wait()
	})
	if badOut.Status != worker.StatusKilled || badOut.Kill != worker.KillMemory {
		t.Fatalf("huge string child: %+v", badOut)
	}
	if goodOut.Status != worker.StatusOK {
		t.Fatalf("canary: healthy child %+v", goodOut)
	}
}

func TestAdversarialRegexBacktrackingIsKilledAtTheDeadlineWhileTheHubKeepsAnswering(t *testing.T) {
	spec := collecttest.Spec(collecttest.Bundle(t, "toy-db", `/^(a+)+(?!x)$/.test("a".repeat(40) + "b");`), nil, nil)
	spec.Limits.Deadline = 500 * time.Millisecond
	var out worker.Outcome
	start := time.Now()
	hubAnswers(t, func() { out = runChild(t, spec) })
	if out.Status != worker.StatusKilled || out.Kill != worker.KillDeadline {
		t.Fatalf("%+v", out)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("took %v", took)
	}
}

func TestAdversarialHugeConsoleOutputIsTruncated(t *testing.T) {
	spec := collecttest.Spec(collecttest.Bundle(t, "toy-db", `for (let i = 0; i < 20; i++) console.log("y".repeat(100000));`), nil, nil)
	spec.Limits.MaxStderr = 32 << 10
	out := runChild(t, spec)
	if out.Status != worker.StatusOK || !out.StderrTruncated || len(out.Stderr) > 32<<10 || !strings.Contains(out.Stderr, "yyyy") {
		t.Fatalf("status %s truncated %v len %d", out.Status, out.StderrTruncated, len(out.Stderr))
	}
	small := collecttest.Spec(collecttest.Bundle(t, "toy-db", `console.log("short line");`), nil, nil)
	if out := runChild(t, small); out.StderrTruncated || !strings.Contains(out.Stderr, "short line") {
		t.Fatalf("canary: %+v", out)
	}
}

func TestAdversarialCanarySecretIsAbsentFromErrorsTheScriptSees(t *testing.T) {
	leaky := &collecttest.Recorder{Err: ports.Unavailable(nil, "row 7 holds "+canary)}
	frames := &collecttest.Frames{}
	spec := collecttest.Spec(collecttest.Bundle(t, "toy-db", `
		try { await ctx.sql.query("app", "select secret from t"); } catch (e) { throw new Error("seen: " + e.message); }
	`), []string{ports.PortSQL}, map[string]ports.Handler{ports.PortSQL: leaky})
	spec.Mask = collecttest.MaskOf(canary)
	spec.Tap = frames.Tap
	out := runChild(t, spec)
	sent := strings.Join(frames.Lines(worker.TapToChild)[1:], "")
	if len(leaky.Calls()) != 1 {
		t.Fatal("canary: the port was not called")
	}
	if strings.Contains(sent, canary) || !strings.Contains(sent, "row 7 holds ****") || strings.Contains(out.Failure, canary) || !strings.Contains(out.Failure, "seen: ") {
		t.Fatalf("sent %q failure %q", sent, out.Failure)
	}
}

func TestAdversarialVariablePlantedInTheHubEnvironmentIsInvisibleInEveryGlobalAndModule(t *testing.T) {
	t.Setenv("COCKPIT_PLANTED_TOKEN", canary)
	code := collecttest.Bundle(t, "toy-db", `
		const needle = "`+reverse(canary)+`".split("").reverse().join("");
		const mod = require("cockpit:collect");
		const seen = new Set();
		let strings = 0;
		const walk = (v, depth) => {
			if (depth > 6 || v === null || v === undefined) return;
			if (typeof v === "string") { strings++; if (v.includes(needle)) throw new Error("planted value visible"); return; }
			if (typeof v !== "object" && typeof v !== "function") return;
			if (seen.has(v)) return;
			seen.add(v);
			for (const key of Reflect.ownKeys(v)) {
				if (typeof key === "string") { strings++; if (key.includes(needle)) throw new Error("planted key visible"); }
				let child;
				try { child = v[key]; } catch (e) { continue; }
				walk(child, depth + 1);
			}
		};
		walk(globalThis, 0);
		walk(mod, 0);
		walk(ctx, 0);
		for (const name of ["process", "require", "Deno", "Bun", "os", "fs", "env"]) if (typeof globalThis[name] !== "undefined") throw new Error(name + " is a global");
		if (strings < 200) throw new Error("walked only " + strings + " strings");
	`)
	spec := collecttest.Spec(code, nil, nil)
	if out := runChild(t, spec); out.Status != worker.StatusOK {
		t.Fatalf("%+v", out)
	}
}

func TestAdversarialDeadChildDoesNotCloseTheActiveRunAsError(t *testing.T) {
	st := store.Open(filepath.Join(t.TempDir(), "home"))
	id := "run-20261012-153001-997a37e"
	slot := 0
	m := store.Meta{State: lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}, ID: id, HostAPI: 1, Submitter: "cli", CreatedAt: "2026-10-12T06:00:00.000Z", Slot: &slot}
	if err := st.WriteMeta(m); err != nil {
		t.Fatal(err)
	}
	spec := collecttest.Spec(collecttest.Bundle(t, "toy-db", `while (true) {}`), nil, nil)
	spec.Limits.Deadline = 300 * time.Millisecond
	if out := runChild(t, spec); out.Status == worker.StatusOK {
		t.Fatalf("%+v", out)
	}
	after, err := st.MustRead(id)
	if err != nil || after.Phase != lifecycle.Testing || after.Verdict != lifecycle.VerdictNone || after.Env != lifecycle.EnvUp {
		t.Fatalf("%+v %v", after.State, err)
	}
}

func TestAdversarialMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "adversarial_test.go", adversarialMutations)
}
