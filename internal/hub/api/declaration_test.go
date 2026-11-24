package api

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var declarationMutations = []porttest.Mutation{
	{"the submit body names only a declaration file of the allowed shape", "pass the declaration text to SubmitInput without ParseDeclarationFile in api.submit", "red: ../x.yaml, /etc/x.yaml and scripts/x.yaml create runs", "TestSubmitCarriesTheDeclarationToTheRunItemAndRefusesAnotherShapeBeforeARunExists"},
	{"a run item always names the declaration it read", "write m.Declaration as it is in Item", "red: the run item of a default run has an empty declaration", "TestSubmitCarriesTheDeclarationToTheRunItemAndRefusesAnotherShapeBeforeARunExists"},
}

func TestDeclarationMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "declaration_test.go", declarationMutations)
}

func TestSubmitCarriesTheDeclarationToTheRunItemAndRefusesAnotherShapeBeforeARunExists(t *testing.T) {
	f := newFixture(t, nil)
	runItem := func(body map[string]any) map[string]any {
		t.Helper()
		status, raw := f.post("/api/runs", body)
		if status != http.StatusAccepted {
			t.Fatalf("submit %v: %d %s", body, status, raw)
		}
		id := validate(t, reflect.TypeFor[Submitted](), raw)["id"].(string)
		until(t, func() bool { v := f.sched.Get(id); return v != nil && v.Phase == lifecycle.Done })
		_, detail := f.get("/api/runs/" + id)
		return validate(t, reflect.TypeFor[RunDetail](), detail)
	}
	if got := runItem(map[string]any{"worktree": "/w", "commit": "c1", "suite": "toy", "declaration": ".cockpit/alt.yaml"}); got["declaration"] != ".cockpit/alt.yaml" {
		t.Fatalf("alt run item declaration %v", got["declaration"])
	}
	if got := runItem(map[string]any{"worktree": "/w", "commit": "c2", "suite": "toy"}); got["declaration"] != ".cockpit/project.yaml" {
		t.Fatalf("default run item declaration %v", got["declaration"])
	}
	runs := func() int {
		list, err := f.st.ListRuns()
		if err != nil {
			t.Fatal(err)
		}
		return len(list.Runs)
	}
	before := runs()
	refused := 0
	for _, value := range []any{"../x.yaml", "/etc/x.yaml", "scripts/x.yaml", true} {
		status, raw := f.post("/api/runs", map[string]any{"worktree": "/w", "commit": "c3", "suite": "toy", "declaration": value})
		if status != http.StatusBadRequest || !strings.Contains(string(raw), "declaration") {
			t.Errorf("declaration %v: %d %s", value, status, raw)
			continue
		}
		refused++
	}
	if refused != 4 || before != 2 || runs() != before {
		t.Fatalf("refused %d of 4; runs %d before and %d after", refused, before, runs())
	}
}
