package runenv

import (
	"bytes"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/seal"
)

const planted = "PLANTED-secret-7f3a9c"

var pipelineMutations = []porttest.Mutation{
	{"the unseal check passes only when all four probes turn red", "accept any red probe in Bites", "red: three red probes and one inconclusive count as a bite", "TestUnsealCheckBitesOnlyWhenEveryProbeIsAViolation"},
	{"stage output passes the run redactor", "print stage lines without s.mask", "red: the planted value shows in the output and the summary", "TestStageLinesAndTheSummaryAreMaskedAndAnyFailedStageFailsTheRun"},
	{"boot waits for running and healthy expected services and stops on an exited one", "treat every listed row as ready", "red: a starting or exited service counts as ready", "TestReadinessWaitsForHealthAndFailsOnAStoppedService"},
}

func TestPipelineMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "pipeline_test.go", pipelineMutations)
}

func probes(sealedOK ...bool) []seal.Result {
	var out []seal.Result
	for i, ok := range sealedOK {
		out = append(out, seal.Result{Name: "probe." + []string{"tcp", "dns", "host", "udp"}[i], OK: ok})
	}
	return out
}

func TestUnsealCheckBitesOnlyWhenEveryProbeIsAViolation(t *testing.T) {
	if !Bites(probes(false, false, false, false)) {
		t.Fatal("four red probes did not bite")
	}
	partial := probes(false, false, false, false)
	partial[3].Inconclusive = true
	for name, rs := range map[string][]seal.Result{"one green": probes(false, true, false, false), "one inconclusive": partial, "none": nil} {
		if Bites(rs) {
			t.Errorf("%s bit", name)
		}
	}
}

func TestStageLinesAndTheSummaryAreMaskedAndAnyFailedStageFailsTheRun(t *testing.T) {
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	s := &stages{out: &out, mask: m.Mask}
	s.run("environment generation", func() (string, error) { return "wrote " + planted, nil })
	if !s.passed() {
		t.Fatal("a passing stage failed the run")
	}
	s.run("boot", func() (string, error) { return "", errStage(planted) })
	summary := s.summary()
	for _, text := range []string{out.String(), summary} {
		if strings.Contains(text, planted) || !strings.Contains(text, "****") {
			t.Fatalf("unmasked output:\n%s", text)
		}
	}
	if s.passed() || !strings.Contains(summary, "boot | failed") {
		t.Fatalf("summary:\n%s", summary)
	}
	if (&stages{}).passed() {
		t.Fatal("a run without stages passed")
	}
}

type errStage string

func (e errStage) Error() string { return "token " + string(e) + " rejected" }

func TestReadinessWaitsForHealthAndFailsOnAStoppedService(t *testing.T) {
	row := func(service, state, health string) compose.PsRow {
		return compose.PsRow{Service: service, State: state, Health: health}
	}
	expected := []string{"app", "db"}
	ready, _, err := readiness([]compose.PsRow{row("app", "running", ""), row("db", "running", "healthy")}, expected)
	if !ready || err != nil {
		t.Fatalf("ready services: %v %v", ready, err)
	}
	ready, waiting, err := readiness([]compose.PsRow{row("app", "running", ""), row("db", "running", "starting")}, expected)
	if ready || err != nil || !strings.Contains(waiting, "db") {
		t.Fatalf("starting db: %v %q %v", ready, waiting, err)
	}
	ready, waiting, _ = readiness([]compose.PsRow{row("app", "running", "")}, expected)
	if ready || !strings.Contains(waiting, "db (no container)") {
		t.Fatalf("missing db: %v %q", ready, waiting)
	}
	oneOff := compose.PsRow{Service: "db", State: "running", Labels: "com.docker.compose.oneoff=True"}
	if ready, _, _ := readiness([]compose.PsRow{row("app", "running", ""), oneOff}, expected); ready {
		t.Fatal("a one-off container counted as the db service")
	}
	if _, _, err := readiness([]compose.PsRow{row("app", "exited", ""), row("db", "running", "healthy")}, expected); err == nil {
		t.Fatal("an exited app did not fail the boot")
	}
}
