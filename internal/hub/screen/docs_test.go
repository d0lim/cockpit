package screen

import (
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

var ticked = regexp.MustCompile("`([^`]+)`")

func docTable(t *testing.T, header string) [][]string {
	t.Helper()
	raw, err := os.ReadFile("../../../docs/screen.md")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	start := slices.Index(lines, header)
	if start < 0 {
		t.Fatalf("docs/screen.md has no table %q", header)
	}
	var rows [][]string
	for _, line := range lines[start+2:] {
		if !strings.HasPrefix(line, "|") {
			break
		}
		var cells []string
		for _, c := range strings.Split(strings.Trim(line, "|"), "|") {
			cells = append(cells, strings.TrimSpace(c))
		}
		rows = append(rows, cells)
	}
	return rows
}

func pathOf(pattern string) string {
	if pattern == "/{$}" {
		return "/"
	}
	return pattern
}

func TestTheScreenDocumentTablesMatchTheViewsBothWays(t *testing.T) {
	documented := map[string]string{}
	for _, row := range docTable(t, "| Path | Screen | Question | Holds |") {
		documented[strings.Trim(row[0], "`")] = row[1]
	}
	registered := map[string]string{}
	for _, v := range views {
		registered[pathOf(v.pattern)] = v.title
	}
	if len(documented) != len(views) || len(registered) != len(views) {
		t.Fatalf("%d documented screens, %d views", len(documented), len(views))
	}
	for path, title := range registered {
		if documented[path] != title {
			t.Errorf("view %s is %q, documented as %q", path, title, documented[path])
		}
	}
	for path := range documented {
		if _, ok := registered[path]; !ok {
			t.Errorf("documented screen %s is not a view", path)
		}
	}

	want := map[string]string{}
	for _, v := range views {
		for _, r := range v.regions() {
			want[r.id] = regionChanges(r)
		}
	}
	got := map[string]string{}
	for _, row := range docTable(t, "| Screens | Region | Changed by |") {
		var kinds []string
		for _, m := range ticked.FindAllStringSubmatch(row[2], -1) {
			kinds = append(kinds, m[1])
		}
		if strings.Contains(row[2], "new signals") {
			kinds = append(kinds, "new signals")
		}
		sort.Strings(kinds)
		got[strings.Trim(row[1], "`")] = strings.Join(kinds, ",")
	}
	if len(got) != len(want) || len(want) < 12 {
		t.Fatalf("%d documented regions, %d regions", len(got), len(want))
	}
	for id, changes := range want {
		if got[id] != changes {
			t.Errorf("region %s changes on %q, documented %q", id, changes, got[id])
		}
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("documented region %s is not a region", id)
		}
	}
}

func regionChanges(r region) string {
	var kinds []string
	for _, k := range r.kinds {
		kinds = append(kinds, string(k))
	}
	if r.signals {
		kinds = append(kinds, "new signals")
	}
	sort.Strings(kinds)
	return strings.Join(kinds, ",")
}
