package compose

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var reservedMutations = []porttest.Mutation{
	{"every harness service name is reserved for the hub", "drop the HarnessServices loop in Model.check", "red: a consumer cockpit-edge, cockpit-runner, cockpit-probe, cockpit-probe-control or cockpit-otlp service passes", "TestAConsumerServiceNamedAfterAPrefixedHarnessServiceIsRefusedAtItsField"},
	{"the harness names carry the cockpit- prefix", "set RoleRunner back to runner", "red: a consumer service named runner is refused as reserved", "TestConsumerServicesNamedEdgeRunnerProbeProbeControlOrOTLPPass"},
	{"the reserved list is the prefixed names and nothing else", "drop RoleOTLP from HarnessServices", "red: the reserved list lacks cockpit-otlp", "TestTheReservedHarnessNamesAreExactlyTheFivePrefixedServices"},
}

var prefixedHarnessNames = []string{"cockpit-edge", "cockpit-runner", "cockpit-probe", "cockpit-probe-control", "cockpit-otlp"}

var bareHarnessNames = []string{"edge", "runner", "probe", "probe-control", "otlp"}

func TestReservedMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "reserved_test.go", reservedMutations)
}

func withConsumerService(t *testing.T, name string) (Loaded, error) {
	return loadToy(t, func(d string) {
		replaceIn(t, filepath.Join(d, "compose.yaml"), "\n  cache:\n    image:", "\n  "+name+":\n    image: docker.io/library/alpine:3.22\n  cache:\n    image:")
	})
}

func TestTheReservedHarnessNamesAreExactlyTheFivePrefixedServices(t *testing.T) {
	got := slices.Sorted(slices.Values(HarnessServices))
	want := slices.Sorted(slices.Values(prefixedHarnessNames))
	if !slices.Equal(got, want) {
		t.Fatalf("HarnessServices %v, want %v", got, want)
	}
	for _, name := range HarnessServices {
		if !strings.HasPrefix(name, "cockpit-") {
			t.Errorf("harness service %q lacks the cockpit- prefix", name)
		}
	}
}

func TestAConsumerServiceNamedAfterAPrefixedHarnessServiceIsRefusedAtItsField(t *testing.T) {
	refused := 0
	for _, name := range prefixedHarnessNames {
		_, err := withConsumerService(t, name)
		r := rejection(t, err)
		if r.Kind != RejectCompose || r.Field != "services."+name || !strings.Contains(r.Error(), "reserved for the hub harness") {
			t.Errorf("%s: %v, want compose at services.%s", name, r, name)
			continue
		}
		refused++
	}
	if refused != 5 {
		t.Fatalf("refused %d of %d names", refused, len(prefixedHarnessNames))
	}
}

func TestConsumerServicesNamedEdgeRunnerProbeProbeControlOrOTLPPass(t *testing.T) {
	passed := 0
	for _, name := range bareHarnessNames {
		loaded, err := withConsumerService(t, name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !slices.Contains(loaded.Model.ServiceNames(), name) {
			t.Errorf("%s: the model lacks the service: %v", name, loaded.Model.ServiceNames())
			continue
		}
		passed++
	}
	if passed != 5 {
		t.Fatalf("passed %d of %d names", passed, len(bareHarnessNames))
	}
}
