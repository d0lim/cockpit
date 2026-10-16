package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

const helperSuite = `import { type Ctx, expectStatus, flow, json, poll, stage, step, suite, target } from "cockpit:flow";

let count = 0;
const base = (ctx: Ctx) => String(ctx.env.baseUrl);
const must = (ok: boolean, what: string) => {
	if (!ok) throw new Error(what);
};
const one = (id: string, run: (ctx: Ctx) => Promise<void>) =>
	flow({ id, title: id + " flow", stage: "use", once: false, requires: [], steps: [step({ id: "h", title: id + " h", kind: "request", run })] });

export default suite({
	name: "fake",
	stages: [stage({ id: "use", title: "Use" })],
	flows: [
		one("alpha", async (ctx) => {
			const res = await ctx.http.send("GET", base(ctx) + "/alpha/h?x=1");
			expectStatus(res, 200, 201);
			must(json<{ id: string }>(res).id === "id-1", "json");
			must(target("GET", res.url) === "GET /alpha/h", "target " + target("GET", res.url));
			const { value, attempts } = await poll(async () => ++count, (v) => v >= 3, { what: "count", deadlineMs: 60000, intervalMs: 1, now: ctx.clock, sleep: ctx.sleep });
			must(value === 3 && attempts === 3, "poll " + value + "/" + attempts);
		}),
		one("beta", async (ctx) => {
			const res = await ctx.http.send("GET", base(ctx) + "/beta/h?credential=" + ctx.priv.credential);
			expectStatus(res, 201);
		}),
		one("gamma", async (ctx) => {
			json({ method: "GET", url: "http://fake.local/p?q=1", status: 200, headers: {}, body: ctx.priv.credential + " is not json" });
		}),
		one("delta", async () => {
			let n = 0;
			await poll(async () => n, () => false, { what: "never", deadlineMs: 2500, intervalMs: 1, now: () => (n += 1000), sleep: async () => {} });
		}),
	],
});
`

func helperSteps(t *testing.T) []contract.ResultStep {
	t.Helper()
	w := newWorld(t, core, nil)
	if err := os.WriteFile(filepath.Join(w.scripts, "suites", "fake.ts"), []byte(helperSuite), 0o644); err != nil {
		t.Fatal(err)
	}
	got := w.run(nil)
	var result contract.Result
	w.read("result.json", &result)
	if got.code != 1 || len(result.Steps) != 4 || !result.Complete {
		t.Fatalf("exit %d steps %d complete %v stderr %v", got.code, len(result.Steps), result.Complete, got.stderr)
	}
	return result.Steps
}

func TestFlowHelpersExpectStatusJsonTargetAndPollWorkInsideAStep(t *testing.T) {
	alpha := helperSteps(t)[0]
	if alpha.Flow != "alpha" || alpha.Status != contract.StepStatusPassed || alpha.Message != "" || alpha.Requests != 1 {
		t.Fatalf("alpha %+v", alpha)
	}
}

func TestExpectStatusFailureNamesMethodAndPathOnly(t *testing.T) {
	beta := helperSteps(t)[1]
	if beta.Flow != "beta" || beta.Status != contract.StepStatusFailed || beta.Message != "GET /beta/h — expected status 201 but got 200" {
		t.Fatalf("beta %+v", beta)
	}
	if strings.Contains(beta.Message, "?") || strings.Contains(beta.Message, "credential") || strings.Contains(beta.Message, "token") {
		t.Fatalf("the query or the body leaked into %q", beta.Message)
	}
}

func TestJsonFailureOnANonJSONBodyKeepsTheBodyOut(t *testing.T) {
	gamma := helperSteps(t)[2]
	if gamma.Flow != "gamma" || gamma.Status != contract.StepStatusFailed || gamma.Message != "GET /p — the response body is not JSON (status 200)" {
		t.Fatalf("gamma %+v", gamma)
	}
}

func TestPollGivesUpAtTheDeadlineAndReportsTheAttemptCount(t *testing.T) {
	delta := helperSteps(t)[3]
	if delta.Flow != "delta" || delta.Status != contract.StepStatusFailed || delta.Message != "never — not satisfied within 2500ms (3 attempts)" || delta.Requests != 0 {
		t.Fatalf("delta %+v", delta)
	}
}
