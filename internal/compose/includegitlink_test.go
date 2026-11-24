package compose

import (
	"slices"
	"testing"

	"github.com/d0lim/cockpit/internal/compose/composetest"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var includeGitlinkMutations = []porttest.Mutation{
	{"an include value at or inside a gitlink the run does not splice is rejected and names the gitlink", "drop the unspliced gitlink check in walk.confine", "red: the path and env file are rejected without naming deps/queue and the project directory passes", "TestAnIncludeValueAtOrInsideASubmoduleNoDeclaredPathReferencesIsRejectedNamingItAndPassesOnceReferenced"},
}

func TestIncludeGitlinkMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "includegitlink_test.go", includeGitlinkMutations)
}

func TestAnIncludeValueAtOrInsideASubmoduleNoDeclaredPathReferencesIsRejectedNamingItAndPassesOnceReferenced(t *testing.T) {
	queue := map[string]string{"deps/queue/compose.yaml": queueStack, "deps/queue/run.env": "QUEUE=1\n"}
	cases := []includeCase{
		{name: "a path", include: "include:\n  - deps/queue/compose.yaml\n", files: queue, field: "compose.files[0] include[0]"},
		{name: "a project directory", include: "include:\n  - {path: stack/compose.yaml, project_directory: deps/queue}\n", files: stacked("", queue), field: "compose.files[0] include[0].project_directory"},
		{name: "an env file", include: "include:\n  - {path: stack/compose.yaml, env_file: deps/queue/run.env}\n", files: stacked("", queue), field: "compose.files[0] include[0].env_file"},
	}
	denied, passed := 0, 0
	for _, c := range cases {
		dir, _ := toyRepo(t, c.mutate(t))
		composetest.Submodule(t, dir, "deps/queue", "queue")
		_, err := readHead(t, dir)
		c.says = "in the submodule deps/queue, which no declared path references"
		includeRejected(t, c, err)
		denied++
		reference(t, dir, "deps/queue")
		loaded, err := readHead(t, dir)
		if err != nil || !loaded.Model.has("queue") || !loaded.Tree.Has("deps/queue/run.env") {
			t.Errorf("%s: referenced: queue in the model %v, %v", c.name, slices.Contains(loaded.Model.ServiceNames(), "queue"), err)
			continue
		}
		passed++
	}
	if denied != 3 || passed != 3 {
		t.Fatalf("%d of 3 denied and %d of 3 passed", denied, passed)
	}
}
