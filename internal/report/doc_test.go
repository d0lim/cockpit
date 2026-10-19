package report

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var tableRow = regexp.MustCompile(`^\| (\d+) \| (.+) \| (.+) \|$`)

func TestVerdictDocumentTableMatchesTheRulesBothWays(t *testing.T) {
	text := fileText(t, filepath.Join(repoRoot(t), "docs", "verdict.md"))
	section := text[strings.Index(text, "## Rules"):strings.Index(text, "## Report bundle")]
	var rows [][3]string
	for _, line := range strings.Split(section, "\n") {
		if m := tableRow.FindStringSubmatch(line); m != nil {
			rows = append(rows, [3]string{m[1], m[2], m[3]})
		}
	}
	if len(rows) != len(rules) {
		t.Fatalf("document has %d rows, code has %d rules", len(rows), len(rules))
	}
	for i, r := range rules {
		want := [3]string{strconv.Itoa(r.Number), r.Condition, r.Verdicts}
		if rows[i] != want {
			t.Errorf("row %d: document %q, code %q", i+1, rows[i], want)
		}
	}
	for _, c := range ruleCases() {
		if !strings.Contains(rules[c.rule-1].Verdicts, string(c.verdict)) {
			t.Errorf("%s: verdict %s is not listed for rule %d", c.name, c.verdict, c.rule)
		}
	}
}
