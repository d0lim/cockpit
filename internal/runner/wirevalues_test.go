package runner

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const wireTestRoutes = "orders=orders.local:8080=cockpit-wire:41001"

var wireValuesMutations = []porttest.Mutation{
	{"captured values reach the file and ref values do not", "write every capture without the public filter", "red: the ref value is in the file", "TestTheValuesFileHoldsWhatTheRunnerMasksAndNotWhatARefMarkedPublic"},
	{"the file is private", "write it with mode 644", "red: the file mode is 644", "TestTheValuesFileHoldsWhatTheRunnerMasksAndNotWhatARefMarkedPublic"},
	{"no file without recording", "write the file whatever the route list says", "red: the file exists in a run that does not record", "TestARunThatDoesNotRecordWritesNoValuesFile"},
	{"a failed update removes the stale file", "return the error without removing the file", "red: the stale path stays", "TestAFailedValuesUpdateRemovesTheStaleFileAndNamesNoValue"},
	{"a later runner keeps what an earlier one captured", "start from an empty list in loadWireValues", "red: the earlier value is gone from the file", "TestALaterRunnerKeepsTheValuesAnEarlierRunnerOfTheRunWrote"},
	{"an unreadable values file withholds the record", "ignore the parse error in loadWireValues", "red: the runner starts and the unreadable file stays", "TestAnUnreadableValuesFileStopsTheRunnerAndIsRemoved"},
	{"a later runner does not recreate a removed file", "treat a missing file as a clean start in loadWireValues", "red: the exec writes the file again", "TestAnExecAfterTheValuesFileWasRemovedLeavesItAbsentAndStillRuns"},
	{"a values file stays within its limit", "drop the size check from update", "red: the file over the limit stays", "TestAValuesFileOverItsLimitIsRemovedAndTheExecStillRuns"},
	{"a file over the limit does not stop the runner", "treat the overflow as a start failure in Main", "red: the exec exits 2", "TestAValuesFileOverItsLimitIsRemovedAndTheExecStillRuns"},
	{"the limit counts the bytes of the file", "compare with one byte more than the limit", "red: a file one byte over is written", "TestAnUpdateWritesUpToTheLimitAndStaysWithheldAfterPassingIt"},
	{"a file over the limit does not come back", "forget the withholding after the removal", "red: a later small update writes the file again", "TestAnUpdateWritesUpToTheLimitAndStaysWithheldAfterPassingIt"},
	{"a first runner still creates the file", "withhold whenever the file is missing", "red: the first runner leaves no file", "TestAnExecAfterTheValuesFileWasRemovedLeavesItAbsentAndStillRuns"},
}

func TestWireValuesMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "wirevalues_test.go", wireValuesMutations)
}

func (w *world) valuesFile() []string {
	w.t.Helper()
	raw, err := os.ReadFile(filepath.Join(w.dir, contract.WireValuesFile))
	if err != nil {
		w.t.Fatal(err)
	}
	var doc contract.WireValues
	if err := json.Unmarshal(raw, &doc); err != nil {
		w.t.Fatalf("%v\n%s", err, raw)
	}
	return doc.Values
}

func TestTheValuesFileHoldsWhatTheRunnerMasksAndNotWhatARefMarkedPublic(t *testing.T) {
	w := newWorld(t, core, nil)
	if err := os.WriteFile(filepath.Join(w.scripts, "suites", "fake.ts"), []byte(recordSuite), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.scripts, "env.json"), []byte(`{"baseUrl":"http://fake.local:80"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := w.run(recording("fake=fake.local:80=cockpit-wire:41001")); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	values := w.valuesFile()
	if !slices.Contains(values, "hidden-id-1") || !slices.Contains(values, privSecret) {
		t.Fatalf("masked values are missing from %v", values)
	}
	if slices.Contains(values, "id-1") || slices.Contains(values, "") {
		t.Fatalf("a public or empty value is in %v", values)
	}
	info, err := os.Stat(filepath.Join(w.dir, contract.WireValuesFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode())
	}
}

func TestARunThatDoesNotRecordWritesNoValuesFile(t *testing.T) {
	w := wireWorld(t, `{"target":"http://orders.local:8080"}`)
	if got := w.run(nil); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	if w.exists(contract.WireValuesFile) || !w.exists("result.json") {
		t.Fatalf("values file %v, result %v", w.exists(contract.WireValuesFile), w.exists("result.json"))
	}
}

func TestAFailedValuesUpdateRemovesTheStaleFileAndNamesNoValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, contract.WireValuesFile)
	if err := os.MkdirAll(filepath.Join(path, "blocked"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := (&wireValues{dir: dir, recording: true, limit: contract.WireValuesLimit}).update([]string{planted})
	if err == nil || strings.Contains(err.Error(), planted) || !strings.Contains(err.Error(), "withheld") {
		t.Fatalf("update: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("the stale path stays: %v", statErr)
	}
	if err := (&wireValues{dir: dir, recording: true, limit: contract.WireValuesLimit}).update([]string{planted}); err != nil {
		t.Fatalf("the next update: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), planted) {
		t.Fatalf("the file after the next update: %q %v", raw, err)
	}
}

func TestALaterRunnerKeepsTheValuesAnEarlierRunnerOfTheRunWrote(t *testing.T) {
	w := wireWorld(t, `{"target":"http://orders.local:8080"}`)
	earlier, err := json.Marshal(contract.WireValues{Values: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.dir, contract.WireValuesFile), earlier, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := w.run(recording("orders=orders.local:8080=cockpit-wire:41001")); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	values := w.valuesFile()
	if !slices.Contains(values, planted) || !slices.Contains(values, privSecret) {
		t.Fatalf("the file holds %d values and lacks the earlier or the current one", len(values))
	}
}

func TestAnUnreadableValuesFileStopsTheRunnerAndIsRemoved(t *testing.T) {
	w := wireWorld(t, `{"target":"http://orders.local:8080"}`)
	path := filepath.Join(w.dir, contract.WireValuesFile)
	if err := os.WriteFile(path, []byte(planted), 0o600); err != nil {
		t.Fatal(err)
	}
	got := w.run(recording("orders=orders.local:8080=cockpit-wire:41001"))
	if got.code == 0 || w.exists(contract.WireValuesFile) {
		t.Fatalf("exit %d, file still there: %v", got.code, w.exists(contract.WireValuesFile))
	}
	if text := strings.Join(got.stderr, "\n"); strings.Contains(text, planted) || !strings.Contains(text, "withheld") {
		t.Fatalf("stderr %q", text)
	}
}

func TestAnExecAfterTheValuesFileWasRemovedLeavesItAbsentAndStillRuns(t *testing.T) {
	w := wireWorld(t, `{"target":"http://orders.local:8080"}`)
	if got := w.run(recording(wireTestRoutes)); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	if !w.exists("manifest.json") || !slices.Contains(w.valuesFile(), privSecret) {
		t.Fatal("the first runner left no manifest or no values")
	}
	exec := func(name string) outcome {
		env := recording(wireTestRoutes)
		env["E2E_EXEC_OUT"] = w.execOut(name)
		return w.run(env, execArgs("alpha")...)
	}
	if got := exec("kept.json"); got.code != 0 || !slices.Contains(w.valuesFile(), privSecret) {
		t.Fatalf("canary: an exec beside the file: exit %d stderr %v", got.code, got.stderr)
	}
	if err := os.Remove(filepath.Join(w.dir, contract.WireValuesFile)); err != nil {
		t.Fatal(err)
	}
	if got := exec("removed.json"); got.code != 0 {
		t.Fatalf("exit %d stderr %v", got.code, got.stderr)
	}
	if w.exists(contract.WireValuesFile) {
		t.Fatal("the exec wrote the removed file again, so the hub would store a take without the earlier values")
	}
	if result := w.readExec("removed.json"); !result.Complete || !slices.Equal(statuses(result.Steps), []string{"passed"}) {
		t.Fatalf("the exec did not run: %+v", result)
	}
}

func TestAValuesFileOverItsLimitIsRemovedAndTheExecStillRuns(t *testing.T) {
	w := wireWorld(t, `{"target":"http://orders.local:8080"}`)
	filler := strings.Repeat("v", contract.WireValuesLimit)
	earlier, err := json.Marshal(contract.WireValues{Values: []string{planted, filler}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.dir, contract.WireValuesFile), earlier, 0o600); err != nil {
		t.Fatal(err)
	}
	env := recording(wireTestRoutes)
	env["E2E_EXEC_OUT"] = w.execOut("over.json")
	got := w.run(env, execArgs("alpha")...)
	if got.code != 0 || w.exists(contract.WireValuesFile) {
		t.Fatalf("exit %d, file still there: %v", got.code, w.exists(contract.WireValuesFile))
	}
	if result := w.readExec("over.json"); !result.Complete {
		t.Fatalf("the exec did not run: %+v", result)
	}
	text := strings.Join(got.stderr, "\n")
	if !strings.Contains(text, "withheld") || !strings.Contains(text, "bytes") || strings.Contains(text, planted) || strings.Contains(text, "vvvv") {
		t.Fatalf("stderr must state counts and no value: %.200q", text)
	}
}

func TestAnUpdateWritesUpToTheLimitAndStaysWithheldAfterPassingIt(t *testing.T) {
	data, err := encodeJSONFile(contract.WireValues{Values: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, contract.WireValuesFile)
	fits := &wireValues{dir: dir, recording: true, limit: len(data)}
	if err := fits.update([]string{planted}); err != nil {
		t.Fatalf("a file of exactly the limit: %v", err)
	}
	if raw, err := os.ReadFile(path); err != nil || !strings.Contains(string(raw), planted) {
		t.Fatalf("canary: the file at the limit: %q %v", raw, err)
	}
	over := &wireValues{dir: dir, recording: true, limit: len(data) - 1}
	err = over.update([]string{planted})
	if !errors.Is(err, errWireValuesOverLimit) || strings.Contains(err.Error(), planted) {
		t.Fatalf("one byte over: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("the file over the limit stays: %v", statErr)
	}
	if err := over.update([]string{"s"}); err != nil {
		t.Fatalf("a later update: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("a later small update wrote the file again: %v", statErr)
	}
}
