package screen

import (
	"net/http"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var maskMutations = []porttest.Mutation{
	{"live flow messages reach the screen masked", "skip the Mask loop in Reader.Flows", "red: the planted value is on the flow screen", "TestTheFlowScreenShowsTheLiveStepMessageAndSourceOnlyMasked"},
	{"live source text reaches the screen masked", "return data without the run masker in Reader.Source", "red: the planted value is in the code view", "TestTheFlowScreenShowsTheLiveStepMessageAndSourceOnlyMasked"},
}

func TestMaskMutationRecordMatchesTheTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "mask_test.go", maskMutations)
}

func TestTheFlowScreenShowsTheLiveStepMessageAndSourceOnlyMasked(t *testing.T) {
	f := newFixture(t)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Testing, Env: lifecycle.EnvUp}, doneUp))
	writeRunner(t, f.runDir(rid(1)), "status 500 with "+planted)
	status, _, body := f.get("/runs/" + rid(1) + "/report/alpha?step=a1")
	if status != http.StatusOK || strings.Contains(body, planted) || !strings.Contains(body, "****") {
		t.Fatalf("%d planted %v masked %v", status, strings.Contains(body, planted), strings.Contains(body, "****"))
	}
	if !strings.Contains(body, "status 500 with") || !strings.Contains(body, `data-scroll-here="flows/alpha.ts:2"`) || !strings.Contains(body, "run tree") {
		t.Fatal("canary: the live step, its line and its source badge are not on the screen")
	}
}
