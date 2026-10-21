package runner

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
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
	w := newWorld(t, core, nil)
	if err := os.WriteFile(filepath.Join(w.scripts, "suites", "fake.ts"), []byte(recordSuite), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := w.run(nil); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	raw, _ := os.ReadFile(filepath.Join(w.dir, "result.json"))
	if strings.Contains(string(raw), privSecret) || strings.Contains(string(raw), "hidden-id-1") {
		t.Fatalf("a secret reached result.json: %s", raw)
	}
	var result contract.Result
	w.read("result.json", &result)
	return result.Steps
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
