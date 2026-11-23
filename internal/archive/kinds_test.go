package archive

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var kindMutations = []porttest.Mutation{
	{"a path is exported only when a kind names it", "match every path in Kind.takes", "red: a neighbour of a named path gets a kind", "TestKindOfNamesTheBundlePathsAndRefusesTheirNeighbours"},
	{"the kind table and the document agree", "add a kind to Kinds without a row", "red: the document lacks the kind", "TestTheKindTableInTheExportDocumentMatchesTheKindsTheArchiveNamesBothWays"},
	{"the kind table and the contract agree", "drop a kind from the contract enum", "red: a kind of the table is no contract value", "TestEveryArchiveKindOfTheContractIsInTheKindTableOrIsTheSchemaKind"},
}

func TestKindMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "kinds_test.go", kindMutations)
}

func TestKindOfNamesTheBundlePathsAndRefusesTheirNeighbours(t *testing.T) {
	cases := []struct {
		rel  string
		want contract.ArchiveKind
	}{
		{"summary.json", contract.ArchiveKindSummary},
		{"summary.json.bak", ""},
		{"meta.json", contract.ArchiveKindMeta},
		{"not-proven.md", contract.ArchiveKindNotProven},
		{"seal.json", contract.ArchiveKindSeal},
		{"e2e/manifest.json", contract.ArchiveKindManifest},
		{"e2e/result.json", contract.ArchiveKindResult},
		{"e2e/ledger.json", contract.ArchiveKindLedger},
		{"e2e/context.json", contract.ArchiveKindContext},
		{"e2e/wire.private.env.json", ""},
		{"e2e/exec/alpha-1.json", contract.ArchiveKindExec},
		{"e2e/exec/deep/alpha-1.json", ""},
		{"e2e/exec/alpha-1.txt", ""},
		{"source/source-index.json", contract.ArchiveKindSourceIndex},
		{"source/flows/alpha.ts", contract.ArchiveKindSource},
		{"sources/flows/alpha.ts", ""},
		{"signals/stream.jsonl", contract.ArchiveKindSignals},
		{"signals/segment.json", ""},
		{"wire/suite/index.json", contract.ArchiveKindWireIndex},
		{"wire/suite/calls.jsonl", contract.ArchiveKindWireCalls},
		{"wire/suite/other.json", ""},
		{"wire/calls.jsonl", ""},
		{"run.env", ""},
	}
	named := 0
	for _, c := range cases {
		kind, ok := KindOf(c.rel)
		if ok != (c.want != "") || kind.Name != c.want {
			t.Errorf("%s: kind %q, named %v", c.rel, kind.Name, ok)
		}
		if ok {
			named++
		}
	}
	if len(cases) != 23 || named != 14 {
		t.Fatalf("%d cases, %d named", len(cases), named)
	}
	for rel, want := range map[string]bool{"wire/suite/index.json": true, "wire/suite/calls.jsonl": true, "wire/.partial-suite/calls.jsonl": false, "wire/suite/other.json": false} {
		if take, ok := takeOf(rel); ok != want || (ok && take != "suite") {
			t.Errorf("%s: take %q, %v", rel, take, ok)
		}
	}
}

var kindRow = regexp.MustCompile("^\\| `([a-z-]+)` \\| [^|]+ \\| (?:`([a-z]+)`[^|]*)?\\|$")

func TestTheKindTableInTheExportDocumentMatchesTheKindsTheArchiveNamesBothWays(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "docs", "export.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	section := text[strings.Index(text, "## Layout"):strings.Index(text, "## Masking")]
	var documented, named []string
	for _, line := range strings.Split(section, "\n") {
		if m := kindRow.FindStringSubmatch(line); m != nil {
			documented = append(documented, m[1]+" "+m[2])
		}
	}
	for _, k := range Kinds {
		named = append(named, string(k.Name)+" "+k.Schema)
	}
	if !slices.Equal(documented, named) || len(named) != 14 {
		t.Fatalf("documented %q\nnamed %q", documented, named)
	}
}

func TestEveryArchiveKindOfTheContractIsInTheKindTableOrIsTheSchemaKind(t *testing.T) {
	var declared []string
	for _, e := range contract.Enums() {
		if e.Type == reflect.TypeFor[contract.ArchiveKind]() {
			for _, v := range e.Values {
				declared = append(declared, v.(string))
			}
		}
	}
	named := []string{string(contract.ArchiveKindSchema)}
	for _, k := range Kinds {
		named = append(named, string(k.Name))
	}
	slices.Sort(declared)
	slices.Sort(named)
	if !slices.Equal(declared, named) || len(declared) != 15 {
		t.Fatalf("declared %q\nnamed %q", declared, named)
	}
}
