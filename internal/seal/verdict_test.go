package seal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var verdictMutations = []porttest.Mutation{
	{"a violation outranks an inconclusive result and no result is an error", "return passed when nothing failed outright", "red: an inconclusive set or an empty set passes", "TestVerdictIsFailedOnAnyViolationErrorWhenInconclusiveOrEmptyAndPassedOtherwise"},
	{"seal.json passes every detail through the run redactor", "write Detail without mask", "red: the planted value appears in seal.json", "TestSealJSONMasksDetailsAndCommandsAndWritesAt600"},
}

func TestVerdictMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "verdict_test.go", verdictMutations)
}

func TestVerdictIsFailedOnAnyViolationErrorWhenInconclusiveOrEmptyAndPassedOtherwise(t *testing.T) {
	pass := Result{Name: "a", OK: true}
	fail := Result{Name: "b"}
	unsure := Result{Name: "c", Inconclusive: true}
	cases := map[string]struct {
		results []Result
		want    string
	}{
		"all pass":                   {[]Result{pass, pass}, "passed"},
		"one violation":              {[]Result{pass, fail}, "failed"},
		"violation and inconclusive": {[]Result{unsure, fail}, "failed"},
		"inconclusive only":          {[]Result{pass, unsure}, "error"},
		"empty":                      {nil, "error"},
	}
	for name, c := range cases {
		if got := string(Verdict(c.results)); got != c.want {
			t.Errorf("%s: %s, want %s", name, got, c.want)
		}
	}
}

func TestSealJSONMasksDetailsAndCommandsAndWritesAt600(t *testing.T) {
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	report := Report{
		Static: []Result{{Name: "mounts.binds", Expected: "x", Detail: "runner: /home/" + planted + " is outside work"}},
		Probes: []Result{{Name: "probe.tcp", OK: true, Command: "cockpit-runner probe " + planted, Expected: "y", Detail: "rc=1"}},
	}
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	at := time.Date(2026, 10, 19, 1, 2, 3, 0, time.UTC)
	if err := Write(root, report, at, m.Mask); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, planted) || strings.Count(text, "****") != 2 {
		t.Fatalf("seal.json:\n%s", text)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
	var doc struct {
		CheckedAt string `json:"checkedAt"`
		Verdict   string `json:"verdict"`
		OK        bool   `json:"ok"`
		Static    []map[string]any
		Probes    []map[string]any
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Verdict != "failed" || doc.OK || doc.CheckedAt != "2026-10-19T01:02:03Z" || len(doc.Static) != 1 || len(doc.Probes) != 1 {
		t.Fatalf("seal.json fields %+v", doc)
	}
	if doc.Static[0]["actual"] == nil || doc.Static[0]["expected"] != "x" {
		t.Fatalf("entry %v", doc.Static[0])
	}
}
