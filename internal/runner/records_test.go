package runner

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/tracectx"
)

const recordSuite = `import { type Ctx, flow, json, stage, step, suite } from "cockpit:flow";

const base = (ctx: Ctx) => String(ctx.env.baseUrl);

export default suite({
	name: "fake",
	stages: [stage({ id: "use", title: "Use" })],
	flows: [
		flow({ id: "alpha", title: "alpha", stage: "use", once: false, requires: [], steps: [
			step({ id: "make", title: "make", kind: "request", run: async (ctx) => {
				const id = json<{ id: string }>(await ctx.http.send("POST", base(ctx) + "/items?x=1", { json: { a: 1 } })).id;
				ctx.capture("itemId", id);
				ctx.ref("item", id);
				ctx.capture("hidden", "hidden-" + id);
				await ctx.http.send("GET", base(ctx) + "/items/" + id);
				await ctx.http.send("GET", base(ctx) + "/other/hidden-" + id);
				await ctx.http.send("GET", base(ctx) + "/secret/" + ctx.priv.credential);
			} }),
			step({ id: "quiet", title: "quiet", kind: "wait", run: async () => {} }),
			step({ id: "refuse", title: "refuse", kind: "capture", run: async (ctx) => {
				let refused = 0;
				for (const v of [ctx.priv.credential, "x-" + ctx.priv.credential + "-y", "Bearer abcdefghijklmnop", "eyJhbGciOi.eyJzdWIiOi.c2ln"]) {
					try { ctx.ref("bad", v); } catch { refused++; }
				}
				ctx.ref("item", "id-5432-ab");
				if (refused !== 4) throw new Error("refused " + refused);
			} }),
		] }),
	],
});
`

func recordSteps(t *testing.T) []contract.ResultStep {
	t.Helper()
	steps, _ := tracedRecordSteps(t, nil)
	return steps
}

func tracedRecordSteps(t *testing.T, extra map[string]string) ([]contract.ResultStep, *world) {
	t.Helper()
	w := newWorld(t, core, nil)
	if err := os.WriteFile(filepath.Join(w.scripts, "suites", "fake.ts"), []byte(recordSuite), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := w.run(extra); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	raw, _ := os.ReadFile(filepath.Join(w.dir, "result.json"))
	if strings.Contains(string(raw), privSecret) || strings.Contains(string(raw), "hidden-id-1") {
		t.Fatalf("a secret reached result.json: %s", raw)
	}
	var result contract.Result
	w.read("result.json", &result)
	return result.Steps, w
}

func TestEachStepRecordsItsRequestsWithMaskedPathsAndItsRefs(t *testing.T) {
	steps := recordSteps(t)
	var paths []string
	for _, c := range steps[0].Calls {
		if c.Host != "fake.local" || c.Status == nil || *c.Status != 200 || c.StartedAt == "" {
			t.Fatalf("call %+v", c)
		}
		paths = append(paths, c.Method+" "+c.Path)
	}
	want := []string{"POST /items", "GET /items/id-1", "GET /other/****", "GET /secret/****"}
	if !slices.Equal(paths, want) {
		t.Fatalf("paths %v, want %v", paths, want)
	}
	if !slices.Equal(steps[0].Refs, []contract.StepRef{{Name: "item", Value: "id-1"}}) {
		t.Fatalf("refs %v", steps[0].Refs)
	}
	if len(steps[1].Calls) != 0 || len(steps[1].Refs) != 0 {
		t.Fatalf("the next step inherited records: %+v", steps[1])
	}
}

func TestARefOfAPrivateValueOrACredentialShapeIsRefusedAndAShortPrivateValueBlocksNothing(t *testing.T) {
	refuse := recordSteps(t)[2]
	if refuse.Status != contract.StepStatusPassed || !slices.Equal(refuse.Refs, []contract.StepRef{{Name: "item", Value: "id-5432-ab"}}) {
		t.Fatalf("refuse step %+v", refuse)
	}
}

func TestTheStepCallCarriesTheSpanIDThatWasSent(t *testing.T) {
	runTrace := tracectx.TraceID("run-20261012-153001-997a37e")
	steps, w := tracedRecordSteps(t, map[string]string{"COCKPIT_TRACE_ID": runTrace})
	calls := steps[0].Calls
	if len(calls) != 4 || len(w.parents) != 4 {
		t.Fatalf("%d calls recorded, %d requests received", len(calls), len(w.parents))
	}
	for i, c := range calls {
		trace, span, ok := tracectx.Parse(w.parents[i])
		if !ok || trace != runTrace || c.SpanID != span {
			t.Fatalf("call %d recorded span id %q, the server received %q", i, c.SpanID, w.parents[i])
		}
	}
}

func TestARunnerWithoutATraceIDSendsNoTraceparentAndAMalformedTraceIDRefusesToStart(t *testing.T) {
	steps, w := tracedRecordSteps(t, nil)
	if len(steps[0].Calls) != 4 || len(w.parents) != 4 {
		t.Fatalf("%d calls recorded, %d requests received", len(steps[0].Calls), len(w.parents))
	}
	for i, c := range steps[0].Calls {
		if c.SpanID != "" || w.parents[i] != "" {
			t.Fatalf("call %d without a trace id recorded %q and sent %q", i, c.SpanID, w.parents[i])
		}
	}
	bad := newWorld(t, core, nil)
	got := bad.run(map[string]string{"COCKPIT_TRACE_ID": strings.ToUpper(tracectx.TraceID("run-x"))})
	if got.code != 2 || !strings.Contains(strings.Join(got.stderr, "\n"), "COCKPIT_TRACE_ID") || len(bad.calls) != 0 {
		t.Fatalf("malformed trace id: exit %d stderr %v calls %v", got.code, got.stderr, bad.calls)
	}
}
