package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/wire"
	"github.com/d0lim/cockpit/internal/wire/wiretest"
)

var wireMutations = []porttest.Mutation{
	{"list heads pass the run masker", "return the stored strings from wireHead", "red: the planted path is in the view", "TestBothWireRoutesMaskAValueLearnedAfterTheTake"},
	{"a picked call passes Clean at read", "answer the stored call instead of the result of Clean in WireCall", "red: the planted body is in the call", "TestBothWireRoutesMaskAValueLearnedAfterTheTake"},
	{"step and route names pass the run masker", "return the stored names from wireSteps and wireRoutes", "red: the planted step id and title are in the view", "TestStepNamesTitlesAndRouteNamesPassTheRunMasker"},
	{"the masker is asked on every read", "keep the first masker of a run in wireRun", "red: a value learned after the first read stays visible", "TestAValueLearnedAfterTheFirstReadIsMaskedOnTheNextRead"},
	{"stored public fields apply at read", "clean with the run masker without WithPublicFields", "red: the public token reads ****", "TestAPublicFieldStaysVisibleAndItsNeighboursStayMasked"},
	{"no masker serves nothing", "drop the Mask nil check from wireRun", "red: the read panics instead of answering 409", "TestWithoutARunMaskerBothRoutesAnswerNotAvailable"},
	{"a run without wire has no record", "drop the Meta.Wire check from wireRun", "red: the read answers 200", "TestARunWithoutWireAnswersNotFoundOnBothRoutes"},
}

func TestWireMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "wire_test.go", wireMutations)
}

var maskedPlanted = "****" + planted[len(planted)-4:]

func wireMeta(m *store.Meta) { m.Wire = true }

var wireDone = lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictFailed}

func wireFixture(t *testing.T) *fixture {
	t.Helper()
	return newFixture(t, func(d *Deps) { d.Mask = func(string) *mask.Masker { return hubMasker(t, planted) } })
}

func seededWire(t *testing.T) (*fixture, string) {
	t.Helper()
	f := wireFixture(t)
	f.seed(runMeta(1, wireDone, doneUp, wireMeta))
	wiretest.Seed(t, f.runDir(rid(1)), planted)
	return f, "/api/runs/" + rid(1) + "/wire"
}

func getWire(t *testing.T, f *fixture, path string) WireView {
	t.Helper()
	status, raw := f.get(path)
	if status != http.StatusOK {
		t.Fatalf("%s: %d %s", path, status, raw)
	}
	validate(t, reflect.TypeFor[WireView](), raw)
	var v WireView
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func getWireCall(t *testing.T, f *fixture, path string) (WireCallView, string) {
	t.Helper()
	status, raw := f.get(path)
	if status != http.StatusOK {
		t.Fatalf("%s: %d %s", path, status, raw)
	}
	validate(t, reflect.TypeFor[WireCallView](), raw)
	var v WireCallView
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v, string(raw)
}

func seqs(calls []WireHead) []int64 {
	out := []int64{}
	for _, c := range calls {
		out = append(out, c.Seq)
	}
	return out
}

func TestBothWireRoutesMaskAValueLearnedAfterTheTake(t *testing.T) {
	f, base := seededWire(t)
	raw, err := os.ReadFile(filepath.Join(f.runDir(rid(1)), wire.StagingDir, wire.SuiteTake, wire.CallsFile))
	if err != nil || !strings.Contains(string(raw), planted) {
		t.Fatalf("canary: the stored record must still hold the value the take did not know: %v", err)
	}
	for _, path := range []string{base + "?flow=alpha&step=a2", base + "/4"} {
		status, body := f.get(path)
		if text := string(body); status != http.StatusOK || strings.Contains(text, planted) || !strings.Contains(text, "****") {
			t.Fatalf("%s: %d planted %v masked %v", path, status, strings.Contains(text, planted), strings.Contains(text, "****"))
		}
	}
	view := getWire(t, f, base+"?flow=alpha&step=a2")
	if view.Calls[0].Path != "/orders/"+maskedPlanted {
		t.Fatalf("canary: the masked path is %q", view.Calls[0].Path)
	}
	call, _ := getWireCall(t, f, base+"/4")
	if call.Call.Response.Body.Text != "failed for "+maskedPlanted || call.Call.Request.Headers["x-request-id"][0] != maskedPlanted || call.Call.Query.Form["ref"][0] != maskedPlanted {
		t.Fatalf("canary: the call is %+v", call.Call)
	}
}

func TestStepNamesTitlesAndRouteNamesPassTheRunMasker(t *testing.T) {
	f := wireFixture(t)
	f.seed(runMeta(1, wireDone, doneUp, wireMeta))
	dir := f.runDir(rid(1))
	writeRunnerFiles(t, dir, "message")
	manifest := filepath.Join(dir, RunnerDir, report.RunnerManifest)
	raw, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var m contract.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m.Flows[0].Steps[0].Title = "title " + planted
	raw, _ = json.Marshal(m)
	if err := os.WriteFile(manifest, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	step := m.Flows[0].Steps[0].ID
	result := &contract.Result{Steps: []contract.ResultStep{{Flow: m.Flows[0].ID, ID: step, Status: contract.StepStatusPassed}, {Flow: "flow-" + planted, ID: "x", Status: contract.StepStatusPassed}}}
	routes := []contract.WireRoute{{Name: "orders", Service: "svc-" + planted, Port: 80}}
	wiretest.Store(t, dir, wiretest.Take{Result: result, Routes: routes})
	status, body := f.get("/api/runs/" + rid(1) + "/wire")
	if text := string(body); status != http.StatusOK || strings.Contains(text, planted) || !strings.Contains(text, "title ****") || !strings.Contains(text, "flow-****") || !strings.Contains(text, "svc-****") {
		t.Fatalf("%d %s", status, body)
	}
}

func TestAValueLearnedAfterTheFirstReadIsMaskedOnTheNextRead(t *testing.T) {
	known := []string{}
	f := newFixture(t, func(d *Deps) { d.Mask = func(string) *mask.Masker { return hubMasker(t, known...) } })
	f.seed(runMeta(1, wireDone, doneUp, wireMeta))
	wiretest.Store(t, f.runDir(rid(1)), wiretest.Take{Calls: wiretest.Calls(planted), Result: &contract.Result{Steps: []contract.ResultStep{{Flow: "alpha", ID: planted, Status: contract.StepStatusFailed}}}})
	path := "/api/runs/" + rid(1) + "/wire"
	if _, body := f.get(path); !strings.Contains(string(body), planted) {
		t.Fatal("canary: the first read does not know the value yet")
	}
	known = []string{planted}
	if _, body := f.get(path); strings.Contains(string(body), planted) || !strings.Contains(string(body), "****") {
		t.Fatalf("the next read: %s", body)
	}
}

func TestAPublicFieldStaysVisibleAndItsNeighboursStayMasked(t *testing.T) {
	f, base := seededWire(t)
	call, text := getWireCall(t, f, base+"/4")
	body := call.Call.Request.Body.JSON.(map[string]any)
	if body["token"] != wiretest.PublicToken || body["password"] != "****" || body["note"] != maskedPlanted {
		t.Fatalf("body %+v", body)
	}
	if strings.Contains(text, "hunter2") || strings.Contains(text, planted) {
		t.Fatal("a masked value is in the answer")
	}
}

func TestWithoutARunMaskerBothRoutesAnswerNotAvailable(t *testing.T) {
	f := newFixture(t, nil)
	f.seed(runMeta(1, wireDone, doneUp, wireMeta))
	wiretest.Seed(t, f.runDir(rid(1)), planted)
	for _, path := range []string{"/wire", "/wire/1"} {
		status, body := f.get("/api/runs/" + rid(1) + path)
		if status != http.StatusConflict || !strings.Contains(string(body), CodeNotAvailable) || strings.Contains(string(body), planted) {
			t.Fatalf("%s: %d %s", path, status, body)
		}
	}
}

func TestARunWithoutWireAnswersNotFoundOnBothRoutes(t *testing.T) {
	f := wireFixture(t)
	f.seed(runMeta(1, wireDone, doneUp))
	wiretest.Seed(t, f.runDir(rid(1)), planted)
	f.seed(runMeta(2, wireDone, doneUp, wireMeta))
	wiretest.Seed(t, f.runDir(rid(2)), planted)
	for _, path := range []string{"/wire", "/wire/1"} {
		if status, body := f.get("/api/runs/" + rid(1) + path); status != http.StatusNotFound || !strings.Contains(string(body), CodeNotFound) {
			t.Fatalf("%s: %d %s", path, status, body)
		}
		if status, body := f.get("/api/runs/" + rid(2) + path); status != http.StatusOK {
			t.Fatalf("canary %s: %d %s", path, status, body)
		}
	}
}
