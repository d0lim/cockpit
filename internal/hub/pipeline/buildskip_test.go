package pipeline

import (
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var buildSkipMutations = []porttest.Mutation{
	{"a run whose keyed images are all present skips the builder and compose build", "drop the CachedImages check in the build stage", "red: the builder and compose build run although the image is present", "TestARunWhoseKeyedImagesAreAllPresentSkipsTheBuilderAndComposeBuildAndSaysSo"},
	{"a missing image builds as before", "skip the build for a keyed tag without asking whether the image exists", "red: the build is skipped with the image missing", "TestARunWithAMissingKeyedImageRunsTheBuilderAndComposeBuild"},
}

func TestBuildSkipMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "buildskip_test.go", buildSkipMutations)
}

func TestARunWhoseKeyedImagesAreAllPresentSkipsTheBuilderAndComposeBuildAndSaysSo(t *testing.T) {
	var asked []string
	w := newWorld(t, func(f *fakedocker.Fake) {
		f.ImagePresent = func(ref string) bool {
			asked = append(asked, ref)
			return strings.HasPrefix(ref, "toy/app:")
		}
	}, nil)
	m := w.submit()
	if m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	if indexOf(w.verbs(), "compose run builder") >= 0 || indexOf(w.verbs(), "compose build") >= 0 {
		t.Fatalf("the build ran with every keyed image present: %v", w.verbs())
	}
	if len(asked) != 1 || asked[0] != "toy/app:"+m.ImageKeys["IMG_APP"] {
		t.Fatalf("asked for %v, want the app image tagged with its key %s", asked, m.ImageKeys["IMG_APP"])
	}
	if text := w.eventText(); !strings.Contains(text, "build skipped: all 1 images present") {
		t.Fatalf("no build-skipped line in the events:\n%s", text)
	}
}

func TestARunWithAMissingKeyedImageRunsTheBuilderAndComposeBuild(t *testing.T) {
	w := newWorld(t, func(f *fakedocker.Fake) {
		f.ImagePresent = func(ref string) bool { return !strings.HasPrefix(ref, "toy/app:") }
	}, nil)
	m := w.submit()
	if m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("state %+v notice %q", m.State, m.Notice)
	}
	builder, build := indexOf(w.verbs(), "compose run builder"), indexOf(w.verbs(), "compose build")
	if builder < 0 || build < builder {
		t.Fatalf("builder at %d, build at %d: %v", builder, build, w.verbs())
	}
	if strings.Contains(w.eventText(), "build skipped") {
		t.Fatal("the events say the build was skipped")
	}
}
