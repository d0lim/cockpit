package compose

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var collectDefMutations = []porttest.Mutation{
	{"a collector's runtime id must match its file name", "skip the id comparison in checkShape", "red: a file b.ts exporting id a is accepted", "TestCollectorDefsReadIdsIntervalsAndPortsAndRefuseAMismatchedIdOrABadInterval"},
	{"intervals are whole milliseconds from one second to one hour", "skip the range check in checkShape", "red: intervalMs 500 and 1.5 are accepted", "TestCollectorDefsReadIdsIntervalsAndPortsAndRefuseAMismatchedIdOrABadInterval"},
	{"sql is allowed only with declared databases", "always allow every hub port", "red: a declaration without databases allows sql", "TestCollectorDefsReadIdsIntervalsAndPortsAndRefuseAMismatchedIdOrABadInterval"},
}

func TestCollectDefMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "collectdefs_test.go", collectDefMutations)
}

func toyScriptCopy(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "scripts")
	if err := os.CopyFS(dir, os.DirFS(filepath.Join("..", "..", "testdata", "toy-project", "scripts"))); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCollectorDefsReadIdsIntervalsAndPortsAndRefuseAMismatchedIdOrABadInterval(t *testing.T) {
	decl := toyDeclaration(t)
	dir := toyScriptCopy(t)
	defs, err := CollectorDefs(t.Context(), decl, dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]time.Duration{}
	for _, d := range defs {
		got[d.ID] = d.Interval
		if d.Code == "" || !slices.Equal(d.Allowed, []string{"docker", "http", "signal", "sql", "state"}) {
			t.Errorf("%s: code %d bytes, allowed %v", d.ID, len(d.Code), d.Allowed)
		}
	}
	if len(defs) != 4 || got["pulse"] != time.Second || got["items"] != 5*time.Second || got["cache"] != 5*time.Second || got["facts"] != 0 {
		t.Fatalf("defs %v", got)
	}
	bad := map[string]string{
		"mismatch": `export default collector({ id: "other", run: async () => {} });`,
		"too-fast": `export default collector({ id: "too-fast", intervalMs: 500, run: async () => {} });`,
		"fraction": `export default collector({ id: "fraction", intervalMs: 1500.5, run: async () => {} });`,
		"no-def":   `export default 42;`,
	}
	refused := 0
	for id, body := range bad {
		d := toyScriptCopy(t)
		writeFile(t, filepath.Join(d, "collectors", id+".ts"), "import { collector } from \"cockpit:collect\";\n"+body+"\n")
		if _, err := CollectorDefs(t.Context(), decl, d); err == nil || !strings.Contains(err.Error(), id) {
			t.Errorf("%s: %v", id, err)
		}
		refused++
	}
	if refused != 4 {
		t.Fatal(refused)
	}
	decl.Databases = nil
	if slices.Contains(AllowedPorts(decl), "sql") || !slices.Contains(AllowedPorts(decl), "signal") {
		t.Fatalf("allowed without databases: %v", AllowedPorts(decl))
	}
}
