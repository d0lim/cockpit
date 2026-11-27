package compose

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose/composetest"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var includeGitlinkMutations = []porttest.Mutation{
	{"an include value at or inside a gitlink the run does not splice is rejected and names the gitlink", "drop the unspliced gitlink check in walk.confine", "red: the path and env file are rejected without naming deps/queue and the project directory passes", "TestAnIncludeValueAtOrInsideASubmoduleNoDeclaredPathReferencesIsRejectedNamingItAndPassesOnceReferenced"},
	{"an extends file inside a gitlink the run does not splice is rejected and names the gitlink", "drop the walk.extends call in walk.visit", "red: both extends files are accepted and fail later in the render", "TestAnExtendsFileInsideASubmoduleNoDeclaredPathReferencesIsRejectedNamingItAndPassesOnceReferenced"},
	{"an extends file resolves against the project directory of the file that holds it", "resolve the extends file against the directory of the compose file in walk.extends", "red: the extends file of the include whose project directory is the root resolves to stack/deps/queue and passes", "TestAnExtendsFileInsideASubmoduleNoDeclaredPathReferencesIsRejectedNamingItAndPassesOnceReferenced"},
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

func TestAnExtendsFileInsideASubmoduleNoDeclaredPathReferencesIsRejectedNamingItAndPassesOnceReferenced(t *testing.T) {
	queue := map[string]string{"deps/queue/compose.yaml": queueStack}
	extend := func(file string) string {
		return "services:\n  extra:\n    extends: {file: " + file + ", service: queue}\n"
	}
	top := func(file string) func(string) {
		return func(d string) {
			replaceIn(t, filepath.Join(d, "compose.yaml"), "services:\n", extend(file))
		}
	}
	cases := []struct {
		name, field string
		mutate      func(string)
	}{
		{"a top-level file", "compose.files[0] services.extra.extends.file", top("deps/queue/compose.yaml")},
		{"an included file resolved against its project directory", "compose.files[0] include[0].path services.extra.extends.file", includeCase{include: "include:\n  - {path: stack/compose.yaml, project_directory: .}\n", files: map[string]string{"stack/compose.yaml": extend("deps/queue/compose.yaml")}}.mutate(t)},
	}
	denied, passed := 0, 0
	for _, c := range cases {
		dir, _ := toyRepo(t, func(d string) {
			for name, body := range queue {
				writeFile(t, filepath.Join(d, filepath.FromSlash(name)), body)
			}
			c.mutate(d)
		})
		composetest.Submodule(t, dir, "deps/queue", "queue")
		_, err := readHead(t, dir)
		if r := rejection(t, err); r.Kind != RejectCompose || r.Field != c.field || !strings.Contains(r.Message, `resolves to "deps/queue/compose.yaml" in the submodule deps/queue, which no declared path references`) {
			t.Errorf("%s: %v, want a compose rejection at %s naming deps/queue", c.name, r, c.field)
			continue
		}
		denied++
		reference(t, dir, "deps/queue")
		if loaded, err := readHead(t, dir); err != nil || !loaded.Model.has("extra") {
			t.Errorf("%s: referenced: extra in the model %v, %v", c.name, loaded.Model.has("extra"), err)
			continue
		}
		passed++
	}
	if denied != 2 || passed != 2 {
		t.Fatalf("%d of 2 denied and %d of 2 passed", denied, passed)
	}
}
