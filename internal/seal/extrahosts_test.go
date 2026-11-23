package seal

import (
	"slices"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var extraHostsMutations = []porttest.Mutation{
	{"only the probes may carry extra_hosts", "drop the probe-name check in extraHosts", "red: a consumer service with the host-gateway entry passes services.extra-hosts", "TestTheProbesHostGatewayEntryPassesAndAnyOtherExtraHostsTurnsTheAssertionRed"},
	{"the probes carry exactly the host-gateway entry", "drop the entry comparison in extraHosts", "red: a probe that maps host.docker.internal to an outside address passes", "TestTheProbesHostGatewayEntryPassesAndAnyOtherExtraHostsTurnsTheAssertionRed"},
	{"the assertion expects the entry in the form compose renders", "set probeHostRendered to compose.ProbeHostEntry, the colon form", "red: the probes of the rendered toy fixture carry an entry the assertion does not expect", "TestTheToyRenderGivesBothProbesTheHostGatewayEntryAndNoOtherService"},
}

func TestExtraHostsMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "extrahosts_test.go", extraHostsMutations)
}

func extraHostsResult(t *testing.T, in Input) Result {
	t.Helper()
	for _, r := range Static(in) {
		if r.Name == "services.extra-hosts" {
			return r
		}
	}
	t.Fatal("no services.extra-hosts result")
	return Result{}
}

func TestTheProbesHostGatewayEntryPassesAndAnyOtherExtraHostsTurnsTheAssertionRed(t *testing.T) {
	if r := extraHostsResult(t, toyInput(t)); !r.OK {
		t.Fatalf("the toy render with both probes mapped to host-gateway failed: %s", r.Detail)
	}
	cases := map[string]struct {
		service string
		value   []any
	}{
		"a consumer service with the probe entry":   {"app", []any{probeHostRendered}},
		"the runner with the probe entry":           {compose.RoleRunner, []any{probeHostRendered}},
		"the sealed probe pointed outside":          {compose.RoleProbe, []any{"host.docker.internal=1.1.1.1"}},
		"the control probe with a second entry":     {compose.RoleProbeControl, []any{probeHostRendered, "example.com=1.1.1.1"}},
		"the sealed probe mapping another name":     {compose.RoleProbe, []any{"example.com=host-gateway"}},
		"the sealed probe with an empty-name entry": {compose.RoleProbe, []any{"=host-gateway"}},
	}
	evaluated := 0
	for name, c := range cases {
		in := toyInput(t)
		svc(t, in, c.service)["extra_hosts"] = c.value
		r := extraHostsResult(t, in)
		if r.OK || r.Detail != c.service {
			t.Errorf("%s: ok=%v detail=%q, want a violation naming %s", name, r.OK, r.Detail, c.service)
		}
		evaluated++
	}
	if evaluated != 6 {
		t.Fatalf("evaluated %d cases", evaluated)
	}
}

func TestTheToyRenderGivesBothProbesTheHostGatewayEntryAndNoOtherService(t *testing.T) {
	in := toyInput(t)
	var carrying []string
	for _, s := range services(in.Config) {
		if present(s.doc["extra_hosts"]) {
			carrying = append(carrying, s.name)
			if got := list(s.doc["extra_hosts"]); len(got) != 1 || text(got[0]) != probeHostRendered {
				t.Errorf("%s carries %v", s.name, got)
			}
		}
	}
	if want := []string{compose.RoleProbe, compose.RoleProbeControl}; !slices.Equal(carrying, want) {
		t.Fatalf("services with extra_hosts %v, want %v", carrying, want)
	}
}
