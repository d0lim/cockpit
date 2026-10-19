package lifecycle

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

func docRows(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "lifecycle.md"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	inTable := false
	for line := range strings.SplitSeq(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "| State | Event |"):
			inTable = true
		case inTable && strings.HasPrefix(line, "|---"):
		case inTable && strings.HasPrefix(line, "| "):
			var cells []string
			for _, c := range strings.Split(strings.Trim(line, "|"), "|") {
				cells = append(cells, strings.TrimSpace(c))
			}
			rows = append(rows, strings.Join(cells, " "))
		case inTable:
			inTable = false
		}
	}
	return rows
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func codeRows() []string {
	var rows []string
	add := func(k Key, event string, c Change) {
		rows = append(rows, strings.Join([]string{string(k), event, dash(string(c.Phase)), dash(string(c.Env)), dash(string(c.Verdict)), dash(string(c.Mark))}, " "))
	}
	for k, row := range Transitions {
		for e, c := range row.On {
			add(k, string(e), c)
		}
		for o, c := range row.Observed {
			add(k, "observed:"+string(o), c)
		}
	}
	return rows
}

func TestLifecycleDocumentTableMatchesTheTransitionTableBothWays(t *testing.T) {
	doc, code := docRows(t), codeRows()
	sort.Strings(doc)
	sort.Strings(code)
	if len(doc) != 60 || !slices.Equal(doc, code) {
		t.Fatalf("document rows %d differ from code rows %d\ndoc  %v\ncode %v", len(doc), len(code), doc, code)
	}
}
