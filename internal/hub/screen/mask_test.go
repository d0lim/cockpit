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
	panes := map[string][]string{
		"":             {"status 500 with ****", "run tree", "recorded no request"},
		"&pane=source": {"status 500 with ****", `data-scroll-here="flows/alpha.ts:2"`, `const token = &#34;****`},
	}
	for pane, canaries := range panes {
		status, _, body := f.get("/runs/" + rid(1) + "/report/alpha?step=a1" + pane)
		if status != http.StatusOK || strings.Contains(body, planted) || !strings.Contains(body, "****") {
			t.Fatalf("pane %q: %d planted %v masked %v", pane, status, strings.Contains(body, planted), strings.Contains(body, "****"))
		}
		for _, want := range canaries {
			if !strings.Contains(body, want) {
				t.Errorf("canary: pane %q lacks %q", pane, want)
			}
		}
	}
	if len(panes) != 2 {
		t.Fatal("pane count")
	}
}
