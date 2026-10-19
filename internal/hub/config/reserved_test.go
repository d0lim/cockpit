package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/config"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var reservedMutations = []porttest.Mutation{
	{"reserved ranges are non-empty half-open ranges", "drop the from >= to check in reservedPorts", "red: [21100, 21100) and [21200, 21000) load", "TestReservedRangesOutsideOneTo65535EmptyOrReversedAreRejectedWithTheirEntry"},
	{"reserved range bounds stay inside 1..65535", "drop the bounds loop in reservedPorts", "red: from 0 and to 65536 load", "TestReservedRangesOutsideOneTo65535EmptyOrReversedAreRejectedWithTheirEntry"},
	{"every reserved range is named once and has both bounds", "drop the name and nil checks", "red: an unnamed, a duplicate or a one-sided entry loads", "TestReservedRangesOutsideOneTo65535EmptyOrReversedAreRejectedWithTheirEntry"},
	{"configured reserved ranges reach the hub", "return an empty ReservedPorts from Parse", "red: the two configured ranges are missing", "TestReservedPortsLoadAsHalfOpenRangesAndDefaultToAnEmptyList"},
	{"the range end is exclusive", "make Contains use port <= To", "red: the port equal to to counts as reserved", "TestReservedPortsLoadAsHalfOpenRangesAndDefaultToAnEmptyList"},
}

func TestReservedMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "reserved_test.go", reservedMutations)
}

func TestReservedPortsLoadAsHalfOpenRangesAndDefaultToAnEmptyList(t *testing.T) {
	absent, err := config.Load(t.TempDir())
	if err != nil || absent.ReservedPorts == nil || len(absent.ReservedPorts) != 0 {
		t.Fatalf("absent file: %+v %v", absent.ReservedPorts, err)
	}
	empty, err := config.Parse([]byte("limits:\n  keptPool: 2\n"))
	if err != nil || empty.ReservedPorts == nil || len(empty.ReservedPorts) != 0 {
		t.Fatalf("file without reservedPorts: %+v %v", empty.ReservedPorts, err)
	}
	home := t.TempDir()
	data := "reservedPorts:\n  - { name: manual-stack, from: 18000, to: 18100 }\n  - { name: old-hub, from: 21000, to: 21800 }\n"
	if err := os.WriteFile(filepath.Join(home, config.FileName), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(home)
	if err != nil {
		t.Fatal(err)
	}
	want := []config.PortRange{{Name: "manual-stack", From: 18000, To: 18100}, {Name: "old-hub", From: 21000, To: 21800}}
	if !slices.Equal(cfg.ReservedPorts, want) {
		t.Fatalf("reserved %+v", cfg.ReservedPorts)
	}
	r := want[0]
	cases := map[int]bool{17999: false, 18000: true, 18099: true, 18100: false}
	for port, in := range cases {
		if r.Contains(port) != in {
			t.Errorf("Contains(%d) = %v, want %v", port, !in, in)
		}
	}
	if !r.Overlaps(18099, 18200) || r.Overlaps(18100, 18200) || r.Overlaps(17900, 18000) || !r.Overlaps(17000, 19000) {
		t.Fatal("Overlaps does not treat both ranges as half-open")
	}
}

func TestReservedRangesOutsideOneTo65535EmptyOrReversedAreRejectedWithTheirEntry(t *testing.T) {
	cases := []struct {
		entry string
		says  string
	}{
		{"{ name: a, from: 0, to: 10 }", "reservedPorts[0].from"},
		{"{ name: a, from: 10, to: 65536 }", "reservedPorts[0].to"},
		{"{ name: a, from: -5, to: 10 }", "reservedPorts[0].from"},
		{"{ name: a, from: 21100, to: 21100 }", "from < to"},
		{"{ name: a, from: 21200, to: 21000 }", "from < to"},
		{"{ name: '', from: 1, to: 2 }", "reservedPorts[0].name"},
		{"{ name: a, from: 1 }", "needs both from and to"},
		{"{ name: a, from: 1, to: 2, extra: 3 }", "extra"},
	}
	evaluated := 0
	for _, c := range cases {
		_, err := config.Parse([]byte("reservedPorts:\n  - " + c.entry + "\n"))
		if err == nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %v", c.entry, err)
		}
		evaluated++
	}
	if evaluated != len(cases) {
		t.Fatalf("evaluated %d cases", evaluated)
	}
	_, err := config.Parse([]byte("reservedPorts:\n  - { name: a, from: 1, to: 2 }\n  - { name: a, from: 3, to: 4 }\n"))
	if err == nil || !strings.Contains(err.Error(), "reservedPorts[1].name") {
		t.Fatalf("duplicate name: %v", err)
	}
	edges, err := config.Parse([]byte("reservedPorts:\n  - { name: low, from: 1, to: 2 }\n  - { name: high, from: 65534, to: 65535 }\n"))
	if err != nil || len(edges.ReservedPorts) != 2 {
		t.Fatalf("ranges at both bounds were rejected: %v", err)
	}
}
