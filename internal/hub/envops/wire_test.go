package envops

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/wire"
)

const (
	runValue     = "PLANTED-run-value-8e21"
	shortCapture = "qz81xk"
	shortRun     = "ab12cd"
	publicToken  = "tok-1f2e3d4c5b6a7980"
)

var wireMaskerMutations = []porttest.Mutation{
	{"the run's sources join the record masker", "skip mask.Refresh of the run sources in recordMasker", "red: the run value stays", "TestTheRecordMaskerCoversRunAndCapturedValuesWithABareMarkerAndKeepsPublicFields"},
	{"values under eight characters stay covered", "read the sources in mask.Hub mode", "red: the six-character value stays", "TestTheRecordMaskerCoversRunAndCapturedValuesWithABareMarkerAndKeepsPublicFields"},
	{"the marker is bare", "build the masker in mask.Hub mode", "red: the marker carries the last four characters", "TestTheRecordMaskerCoversRunAndCapturedValuesWithABareMarkerAndKeepsPublicFields"},
	{"a public name cannot hide a captured value", "read the captured file with the public names of the run", "red: the captured value under the name values stays", "TestAPublicNameOfTheRunDoesNotHideACapturedValue"},
	{"a missing values file beside a manifest refuses", "return no values whenever the file is missing", "red: no error is returned", "TestAMissingValuesFileRefusesOnlyWhenTheRunnerStarted"},
	{"the captured values have their own limit", "read the file under the credential-source limit", "red: a file of 2 MiB is refused", "TestACapturedValuesFileBetweenTheTwoLimitsStillMasksTheRecord"},
	{"a file over its limit is withheld under its own detail", "drop the size check from capturedValues", "red: the detail is the generic one", "TestACapturedValuesFileOverItsLimitWithholdsTheTakeAndSaysSo"},
}

func TestWireMaskerMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "wire_test.go", wireMaskerMutations)
}

func wireRun(t *testing.T, publicNames ...string) (string, *contract.Declaration) {
	t.Helper()
	dir := t.TempDir()
	put := func(rel, data string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	put("src/.env", "APP_TOKEN="+runValue+"\nSHORT="+shortRun+"\n")
	put("work/e2e/manifest.json", "{}")
	put("work/e2e/"+contract.WireValuesFile, `{"values":["`+shortCapture+`"]}`)
	decl := &contract.Declaration{}
	decl.Env.Outputs = []contract.RelPath{".env"}
	decl.Secrets.PublicNames = publicNames
	decl.Wire.PublicFields = []contract.WirePublicField{{Key: "token", Value: "tok-[0-9a-f]{8,64}"}}
	return dir, decl
}

func TestTheRecordMaskerCoversRunAndCapturedValuesWithABareMarkerAndKeepsPublicFields(t *testing.T) {
	dir, decl := wireRun(t)
	m, err := recordMasker(dir, decl)
	if err != nil {
		t.Fatal(err)
	}
	got := m.Mask("run " + runValue + " short " + shortRun + " captured " + shortCapture)
	if got != "run **** short **** captured ****" {
		t.Fatalf("masked %q", got)
	}
	clean := wire.Clean(m, wire.Call{Response: wire.Message{Body: wire.Body{Kind: wire.BodyJSON, JSON: map[string]any{"token": publicToken, "password": "hunter2-long"}}}})
	body := clean.Response.Body.JSON.(map[string]any)
	if body["token"] != publicToken || body["password"] != "****" {
		t.Fatalf("body %v", body)
	}
}

func TestAPublicNameOfTheRunDoesNotHideACapturedValue(t *testing.T) {
	dir, decl := wireRun(t, "values")
	m, err := recordMasker(dir, decl)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Mask("captured " + shortCapture); got != "captured ****" {
		t.Fatalf("masked %q", got)
	}
}

func TestAMissingValuesFileRefusesOnlyWhenTheRunnerStarted(t *testing.T) {
	dir, decl := wireRun(t)
	values := filepath.Join(dir, "work", "e2e", contract.WireValuesFile)
	if err := os.Remove(values); err != nil {
		t.Fatal(err)
	}
	if _, err := recordMasker(dir, decl); err == nil || !strings.Contains(err.Error(), "captured values") {
		t.Fatalf("beside a manifest: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "work", "e2e", "manifest.json")); err != nil {
		t.Fatal(err)
	}
	m, err := recordMasker(dir, decl)
	if err != nil {
		t.Fatalf("a runner that never started: %v", err)
	}
	if got := m.Mask("run " + runValue); got != "run ****" {
		t.Fatalf("canary: masked %q", got)
	}
	if err := os.WriteFile(values, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := recordMasker(dir, decl); err == nil {
		t.Fatal("a broken values file was accepted")
	}
	if _, err := recordMasker(dir, nil); err == nil {
		t.Fatal("a run without a declaration was accepted")
	}
}

func writeCapturedValues(t *testing.T, dir string, size int, values ...string) {
	t.Helper()
	filler := strings.Repeat("v", size)
	raw, err := json.Marshal(contract.WireValues{Values: append(values, filler)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "work", "e2e", contract.WireValuesFile), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestACapturedValuesFileBetweenTheTwoLimitsStillMasksTheRecord(t *testing.T) {
	dir, decl := wireRun(t)
	writeCapturedValues(t, dir, 2<<20, shortCapture)
	m, err := recordMasker(dir, decl)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Mask("captured " + shortCapture); got != "captured ****" {
		t.Fatalf("masked %q", got)
	}
}

func TestACapturedValuesFileOverItsLimitWithholdsTheTakeAndSaysSo(t *testing.T) {
	take := func(dir string, decl *contract.Declaration) wire.Status {
		return wire.Take(context.Background(), wire.TakeInput{
			Store: wire.OpenStore(dir), Name: wire.SuiteTake,
			Record: func() (*mask.Masker, error) { return recordMasker(dir, decl) },
			Stream: func(context.Context, io.Writer) error {
				t.Error("a withheld take read the recorder")
				return wire.ErrRecorderGone
			},
		})
	}
	dir, decl := wireRun(t)
	writeCapturedValues(t, dir, contract.WireValuesLimit, runValue)
	over := take(dir, decl)
	if over.State != wire.StateWithheld || !strings.Contains(over.Detail, "16 MiB") || strings.Contains(over.Detail, runValue) {
		t.Fatalf("over the limit: %+v", over)
	}
	broken, decl := wireRun(t)
	if err := os.WriteFile(filepath.Join(broken, "work", "e2e", contract.WireValuesFile), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	other := take(broken, decl)
	if other.State != wire.StateWithheld || other.Detail == over.Detail {
		t.Fatalf("canary: a broken file reads %+v, the file over the limit %+v", other, over)
	}
}
