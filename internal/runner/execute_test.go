package runner

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

func TestAE1_전부_통과하면_매니페스트와_결과의_단계_집합이_같고_complete_이며_종료_0_원장_셋이_passed(t *testing.T) {
	w := newWorld(t, core, nil)
	got := w.run(nil)
	if got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	var manifest contract.Manifest
	var result contract.Result
	w.read("manifest.json", &manifest)
	w.read("result.json", &result)
	var manifestSteps []string
	for _, f := range manifest.Flows {
		for _, s := range f.Steps {
			manifestSteps = append(manifestSteps, f.ID+"/"+s.ID)
		}
	}
	want := []string{"alpha/a1", "beta/b1", "beta/b2", "beta/b3", "gamma/g1"}
	if !slices.Equal(manifestSteps, want) || !slices.Equal(stepIDs(result.Steps), want) {
		t.Fatalf("manifest %v result %v", manifestSteps, stepIDs(result.Steps))
	}
	if manifest.SchemaVersion != 2 || manifest.Runner == nil || manifest.Runner.Version == "" || manifest.Suite != "fake" {
		t.Fatalf("manifest %+v", manifest)
	}
	if !result.Complete || result.FinishedAt == "" || result.StoppedAt != nil || result.SchemaVersion != 2 {
		t.Fatalf("result %+v", result)
	}
	for _, s := range result.Steps {
		if s.Status != contract.StepStatusPassed || s.Requests != 1 || s.HTTPStatus == nil || *s.HTTPStatus != 200 || s.DurationMs == nil || *s.DurationMs != 1000 || s.StartedAt == "" || s.FinishedAt == "" {
			t.Fatalf("step %+v", s)
		}
	}
	for _, id := range []string{"alpha", "beta", "gamma"} {
		if w.ledgerStatus(id) != contract.FlowStatusPassed {
			t.Fatalf("ledger %s = %s", id, w.ledgerStatus(id))
		}
	}
	var ctx contract.Context
	w.read("context.json", &ctx)
	if len(ctx.Captures) != 3 || ctx.Captures["alpha.id"] != "id-1" || ctx.Captures["beta.id"] != "id-4" || ctx.Captures["gamma.id"] != "id-5" {
		t.Fatalf("captures %v", ctx.Captures)
	}
	if !slices.Equal(got.stdout, []string{"alpha › a1 passed 1000ms [200]", "beta › b1 passed 1000ms [200]", "beta › b2 passed 1000ms [200]", "beta › b3 passed 1000ms [200]", "gamma › g1 passed 1000ms [200]"}) {
		t.Fatalf("stdout %q", got.stdout)
	}
	if len(got.stderr) != 0 {
		t.Fatalf("stderr %q", got.stderr)
	}
	for _, f := range manifest.Flows {
		if f.Source == nil || f.Source.File != "flows/fake.ts" || f.Source.Line < 1 {
			t.Fatalf("flow source %+v", f.Source)
		}
		for _, s := range f.Steps {
			if s.Source == nil || s.Source.File != "flows/fake.ts" || s.Source.Line < 1 {
				t.Fatalf("step source %+v", s.Source)
			}
		}
	}
}

func TestAE2_둘째_플로우의_둘째_단계가_실패하면_남은_단계와_그것을_전제하는_플로우가_skipped_종료_1(t *testing.T) {
	w := newWorld(t, core, []string{"beta/b2"})
	got := w.run(nil)
	if got.code != 1 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	var result contract.Result
	w.read("result.json", &result)
	if !result.Complete || !slices.Equal(statuses(result.Steps), []string{"passed", "passed", "failed", "skipped", "skipped"}) {
		t.Fatalf("%v complete=%v", statuses(result.Steps), result.Complete)
	}
	if result.StoppedAt == nil || *result.StoppedAt != (contract.StopPoint{Flow: "beta", Step: "b2"}) || result.Steps[3].Requests != 0 || result.Steps[3].HTTPStatus != nil {
		t.Fatalf("%+v", result)
	}
	if !slices.Equal(w.paths(), []string{"/alpha/a1", "/beta/b1", "/beta/b2"}) {
		t.Fatalf("calls %v", w.paths())
	}
	if w.ledgerStatus("alpha") != "passed" || w.ledgerStatus("beta") != "failed" || w.ledgerStatus("gamma") != "skipped" {
		t.Fatalf("ledger")
	}
	if !slices.Equal(got.stdout[2:], []string{"beta › b2 failed 1000ms [200]", "beta › b3 skipped", "gamma › g1 skipped"}) {
		t.Fatalf("stdout %q", got.stdout)
	}
}

func Test건너뜀은_전제를_따라_이어진다_실패한_플로우의_손자도_시작하지_않는다(t *testing.T) {
	specs := append(slices.Clone(core), flowSpec{ID: "delta", Requires: []string{"gamma"}, Steps: []string{"d1"}})
	w := newWorld(t, specs, []string{"alpha/a1"})
	if got := w.run(nil); got.code != 1 {
		t.Fatalf("exit %d", got.code)
	}
	for _, id := range []string{"beta", "gamma", "delta"} {
		if w.ledgerStatus(id) != contract.FlowStatusSkipped {
			t.Fatalf("%s = %s", id, w.ledgerStatus(id))
		}
	}
	if !slices.Equal(w.paths(), []string{"/alpha/a1"}) {
		t.Fatalf("calls %v", w.paths())
	}
}

func TestAE7_단계가_도는_중의_result_json_은_온전한_JSON_이고_complete_false_에_그_단계가_running_이며_원장은_started_다(t *testing.T) {
	w := newWorld(t, core, nil)
	type snapshot struct {
		result contract.Result
		ledger contract.Ledger
		files  []string
	}
	var seen []snapshot
	w.during = func(path string) {
		if path != "/beta/b2" {
			return
		}
		var s snapshot
		w.read("result.json", &s.result)
		w.read("ledger.json", &s.ledger)
		entries, _ := os.ReadDir(w.dir)
		for _, e := range entries {
			s.files = append(s.files, e.Name())
		}
		seen = append(seen, s)
	}
	if got := w.run(nil); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	if len(seen) != 1 {
		t.Fatalf("중간 시점 %d", len(seen))
	}
	mid := seen[0]
	if mid.result.Complete || !slices.Equal(statuses(mid.result.Steps), []string{"passed", "passed", "running", "notRun", "notRun"}) {
		t.Fatalf("%+v", mid.result)
	}
	if mid.ledger.Flows["beta"].Status != contract.FlowStatusStarted {
		t.Fatalf("ledger %+v", mid.ledger)
	}
	if _, ok := mid.ledger.Flows["gamma"]; ok {
		t.Fatalf("gamma 가 원장에 먼저 적혔다")
	}
	for _, name := range mid.files {
		if strings.HasSuffix(name, ".tmp") {
			t.Fatalf("임시 파일이 남아 있다: %v", mid.files)
		}
	}
}

func Test표준_출력_결과_원장_컨텍스트_stderr_에_심은_토큰과_private_값의_원문이_없고_실패_문장은_가린_채_남는다(t *testing.T) {
	specs := append(slices.Clone(core), flowSpec{ID: "delta", Requires: []string{"alpha"}, Steps: []string{"log1"}})
	w := newWorld(t, specs, []string{"beta/b2"})
	got := w.run(nil)
	entries, _ := os.ReadDir(w.dir)
	texts := []string{strings.Join(got.stdout, "\n"), strings.Join(got.stderr, "\n")}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") && e.Name() != "e2e.private.env.json" {
			raw, _ := os.ReadFile(filepath.Join(w.dir, e.Name()))
			texts = append(texts, string(raw))
		}
	}
	for _, text := range texts {
		if strings.Contains(text, planted) || strings.Contains(text, privSecret) {
			t.Fatalf("원문이 보인다: %q", text)
		}
	}
	var result contract.Result
	w.read("result.json", &result)
	if result.Steps[2].Message != "기대와 다르다 — token=**** cred=**** 단계=b2" {
		t.Fatalf("message %q", result.Steps[2].Message)
	}
	if len(got.stderr) != 1 || !strings.Contains(got.stderr[0], "cred=****") {
		t.Fatalf("console stderr %q", got.stderr)
	}
}

func Test실패_문장은_가린_뒤에_자른다_긴_문장의_끝에_걸친_값도_원문_조각을_남기지_않는다(t *testing.T) {
	w := newWorld(t, []flowSpec{{ID: "alpha", Steps: []string{"long1"}}}, nil)
	w.run(nil)
	var result contract.Result
	w.read("result.json", &result)
	message := result.Steps[0].Message
	if strings.Contains(message, privSecret[:5]) || !strings.HasPrefix(message, strings.Repeat("x", 495)) || !strings.HasSuffix(message, "****") {
		t.Fatalf("message %q", message)
	}
}

func Test단계가_요청을_보내지_않으면_requests_0_이고_httpStatus_가_없다(t *testing.T) {
	w := newWorld(t, []flowSpec{{ID: "alpha", Steps: []string{"a1", "noop2"}}}, nil)
	if got := w.run(nil); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	var result contract.Result
	w.read("result.json", &result)
	if result.Steps[1].Requests != 0 || result.Steps[1].HTTPStatus != nil || result.Steps[0].Requests != 1 {
		t.Fatalf("%+v", result.Steps)
	}
}
