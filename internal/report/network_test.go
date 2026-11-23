package report

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var networkMutations = []porttest.Mutation{
	{"an open run lists runtime isolation as not proven", "drop the open-network line from unproven", "red: an open run's notProven.declared is empty", "TestAnOpenRunListsRuntimeIsolationAsNotProvenAndKeepsItsVerdict"},
	{"the summary carries the level", "drop Network from Summary", "red: summary.json has no network key", "TestAnOpenRunListsRuntimeIsolationAsNotProvenAndKeepsItsVerdict"},
}

func TestNetworkMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "network_test.go", networkMutations)
}

func TestAnOpenRunListsRuntimeIsolationAsNotProvenAndKeepsItsVerdict(t *testing.T) {
	sealed := Judge(with(func(in *Inputs) { in.Network = contract.NetworkSealed }))
	if sealed.Verdict != contract.VerdictPassed || len(sealed.Declared) != 1 || slices.Contains(sealed.Declared, OpenNetworkNotProven) {
		t.Fatalf("canary: a sealed passing run %+v", sealed)
	}
	open := Judge(with(func(in *Inputs) { in.Network = contract.NetworkOpen }))
	if open.Verdict != contract.VerdictPassed || !slices.Equal(open.Declared, append([]string{OpenNetworkNotProven}, sealed.Declared...)) {
		t.Fatalf("an open passing run %+v", open)
	}
	_, sealedDoc := assembled(t, func(f *fixture) { f.bundle.Inputs.Network = contract.NetworkSealed })
	out, doc := assembled(t, func(f *fixture) { f.bundle.Inputs.Network = contract.NetworkOpen })
	if doc["network"] != "open" || sealedDoc["network"] != "sealed" || doc["verdict"] != sealedDoc["verdict"] {
		t.Fatalf("summary network %v and %v, verdict %v and %v", doc["network"], sealedDoc["network"], doc["verdict"], sealedDoc["verdict"])
	}
	text, err := os.ReadFile(filepath.Join(out.Dir, FileNotProven))
	if err != nil || !strings.Contains(string(text), "network: open") {
		t.Fatalf("not-proven.md %q %v", text, err)
	}
}
