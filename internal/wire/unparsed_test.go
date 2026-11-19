package wire

import (
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var unparsedMutations = []porttest.Mutation{
	{"a query that does not parse keeps nothing", "return the raw query as text from parseQuery", "red: the value stays in the query text", "TestAQueryOrFormThatDoesNotParseKeepsNothing"},
	{"a form body that does not parse keeps nothing", "fall through to the text path in classify", "red: the value stays in the body text", "TestAQueryOrFormThatDoesNotParseKeepsNothing"},
}

func TestUnparsedMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "unparsed_test.go", unparsedMutations)
}

func TestAQueryOrFormThatDoesNotParseKeepsNothing(t *testing.T) {
	m := newMasker(t)
	broken := "pwd=" + plantedCredential + ";x=1"
	x := baseExchange()
	x.RawQuery = broken
	x.Request = typed("application/x-www-form-urlencoded", broken)
	got, ok := Prove(m, Capture(m, x))
	line := lineOf(t, got)
	if !ok || strings.Contains(line, plantedCredential) || got.Query.Omitted != OmitUnparsed || got.Request.Body.Omitted != OmitUnparsed {
		t.Fatalf("proved %v, line %s", ok, line)
	}
	canary := baseExchange()
	canary.RawQuery = "note=plain-value"
	canary.Request = typed("application/x-www-form-urlencoded", "note=plain-value")
	kept := Capture(m, canary)
	if kept.Query.Kind != BodyForm || kept.Request.Body.Kind != BodyForm || !strings.Contains(lineOf(t, kept), "plain-value") {
		t.Fatalf("a form that parses lost its fields: %s", lineOf(t, kept))
	}
}
