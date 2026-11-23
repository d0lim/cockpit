package headless

import (
	"bytes"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/seal"
)

const planted = "PLANTED-secret-7f3a9c"

var pipelineMutations = []porttest.Mutation{
	{"the unseal check passes only when all four probes turn red", "accept any red probe in Bites", "red: three red probes and one inconclusive count as a bite", "TestUnsealCheckBitesOnlyWhenEveryProbeIsAViolation"},
	{"stage output passes the run redactor", "print stage lines without s.mask", "red: the planted value shows in the stage lines", "TestStageLinesAreMaskedAndAFailedStageReturnsItsError"},
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

func TestStageLinesAreMaskedAndAFailedStageReturnsItsError(t *testing.T) {
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	s := &stages{out: &out, mask: m.Mask}
	if err := s.run("environment generation", func() (string, error) { return "wrote " + planted, nil }); err != nil {
		t.Fatalf("a passing stage returned %v", err)
	}
	if err := s.run("boot", func() (string, error) { return "", errStage(planted) }); err == nil {
		t.Fatal("a failed stage returned no error")
	}
	text := out.String()
	if strings.Contains(text, planted) || !strings.Contains(text, "****") {
		t.Fatalf("unmasked output:\n%s", text)
	}
	if !strings.Contains(text, "ok   environment generation") || !strings.Contains(text, "FAIL boot") {
		t.Fatalf("stage lines:\n%s", text)
	}
}

type errStage string

func (e errStage) Error() string { return "token " + string(e) + " rejected" }
