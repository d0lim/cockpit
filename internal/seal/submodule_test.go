package seal

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/compose/composetest"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var submoduleMutations = []porttest.Mutation{
	{"a bind source that reaches a gitlink the run does not splice fails mounts.binds", "drop the leftOut call in mountChecks", "red: the binds at, inside and around vendor/extra pass mounts.binds", "TestABindBuildContextOrConfigFileThatReachesALeftOutSubmoduleFailsNamingItAndPassesOnceADeclaredPathReferencesIt"},
	{"a build or additional context that reaches a gitlink the run does not splice fails paths.build", "drop the leftOut call on contexts in pathChecks", "red: the build context inside vendor/extra and the additional context at the build root pass paths.build", "TestABindBuildContextOrConfigFileThatReachesALeftOutSubmoduleFailsNamingItAndPassesOnceADeclaredPathReferencesIt"},
	{"a configs or secrets file that reaches a gitlink the run does not splice fails paths.configs-secrets", "drop the leftOut call on configs and secrets files in pathChecks", "red: the config file inside vendor/extra passes paths.configs-secrets", "TestABindBuildContextOrConfigFileThatReachesALeftOutSubmoduleFailsNamingItAndPassesOnceADeclaredPathReferencesIt"},
	{"a path that contains a left-out gitlink reaches it", "drop the loop over contained gitlinks in compose Tree.LeftOut", "red: the bind of src/vendor and the additional context at the build root pass", "TestABindBuildContextOrConfigFileThatReachesALeftOutSubmoduleFailsNamingItAndPassesOnceADeclaredPathReferencesIt"},
	{"paths under build/ are judged against the tree as well as paths under src/", "map paths only under Layout.Src in leftOut", "red: the builder bind and the build context under build/ pass", "TestABindBuildContextOrConfigFileThatReachesALeftOutSubmoduleFailsNamingItAndPassesOnceADeclaredPathReferencesIt"},
	{"a gitlink a declared path references is not left out", "test only the gitlink type, not whether it is spliced, in compose Tree.LeftOut", "red: the copies whose tree splices vendor/extra still fail", "TestABindBuildContextOrConfigFileThatReachesALeftOutSubmoduleFailsNamingItAndPassesOnceADeclaredPathReferencesIt"},
	{"a path reaches a gitlink segment by segment, not by string prefix", "compare with strings.HasPrefix(at, path) in place of covers in compose Tree.LeftOut", "red: the bind of src/vendor/ext fails mounts.binds", "TestASiblingThatSharesAStringPrefixWithALeftOutSubmoduleDoesNotReachIt"},
}

func TestSubmoduleMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "submodule_test.go", submoduleMutations)
}

func umbrellaTree(t *testing.T, paths ...string) compose.Tree {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo")
	for _, f := range []string{"vendor/extra/readme.txt", "vendor/extra-other/readme.txt", "keep.txt"} {
		path := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(f+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	composetest.Git(t, dir, "init", "-q", "-b", "main")
	composetest.Git(t, dir, "add", "-A")
	composetest.Git(t, dir, "commit", "-q", "-m", "umbrella")
	composetest.Submodule(t, dir, "vendor/extra", "extra")
	tree, err := compose.Git{Dir: dir}.ListTree(context.Background(), composetest.Git(t, dir, "rev-parse", "HEAD"), paths...)
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func addBind(t *testing.T, in *Input, service, source string) {
	t.Helper()
	s := svc(t, *in, service)
	s["volumes"] = append(list(s["volumes"]), map[string]any{"type": "bind", "source": source, "target": "/mnt/extra"})
}

func reachCases(t *testing.T) []struct {
	name, assertion string
	mutate          func(in *Input)
} {
	return []struct {
		name, assertion string
		mutate          func(in *Input)
	}{
		{"a project bind at the gitlink", "mounts.binds", func(in *Input) { addBind(t, in, "db", fixtureRun+"/src/vendor/extra") }},
		{"a project bind inside the gitlink", "mounts.binds", func(in *Input) { addBind(t, in, "db", fixtureRun+"/src/vendor/extra/readme.txt") }},
		{"a project bind around the gitlink", "mounts.binds", func(in *Input) { addBind(t, in, "db", fixtureRun+"/src/vendor") }},
		{"a builder bind at the gitlink under build", "mounts.binds", func(in *Input) { addBind(t, in, "builder", fixtureRun+"/build/vendor/extra") }},
		{"a build context inside the gitlink", "paths.build", func(in *Input) {
			record(svc(t, *in, "app")["build"])["context"] = fixtureRun + "/build/vendor/extra/app"
		}},
		{"an additional context around the gitlink at the build root", "paths.build", func(in *Input) {
			record(svc(t, *in, "app")["build"])["additional_contexts"] = map[string]any{"tree": fixtureRun + "/build"}
		}},
		{"a config file inside the gitlink", "paths.configs-secrets", func(in *Input) {
			in.Config["configs"] = map[string]any{"extra": map[string]any{"file": fixtureRun + "/src/vendor/extra/extra.conf"}}
		}},
	}
}

func TestABindBuildContextOrConfigFileThatReachesALeftOutSubmoduleFailsNamingItAndPassesOnceADeclaredPathReferencesIt(t *testing.T) {
	leftOut, referenced := umbrellaTree(t), umbrellaTree(t, "vendor/extra")
	if leftOut.Entries["vendor/extra"].Type != "commit" || leftOut.Has("vendor/extra/readme.txt") || !referenced.Has("vendor/extra/readme.txt") {
		t.Fatalf("canary: vendor/extra left out %v, spliced when referenced %v", !leftOut.Has("vendor/extra/readme.txt"), referenced.Has("vendor/extra/readme.txt"))
	}
	denied, passed := 0, 0
	for _, c := range reachCases(t) {
		in := toyInput(t)
		in.Tree = leftOut
		c.mutate(&in)
		results := Static(in)
		if got := failing(results); !slices.Equal(got, []string{c.assertion}) {
			t.Errorf("%s with vendor/extra left out turned %v red, want %s", c.name, got, c.assertion)
			continue
		}
		if d := detail(results, c.assertion); !strings.Contains(d, "the submodule vendor/extra, which no declared path references") {
			t.Errorf("%s: %s does not name the gitlink: %s", c.name, c.assertion, d)
			continue
		}
		denied++
		in = toyInput(t)
		in.Tree = referenced
		c.mutate(&in)
		if got := failing(Static(in)); len(got) != 0 {
			t.Errorf("%s with vendor/extra referenced turned %v red", c.name, got)
			continue
		}
		passed++
	}
	if denied != 7 || passed != 7 {
		t.Fatalf("%d of 7 copies denied and %d of 7 passed", denied, passed)
	}
}

func TestASiblingThatSharesAStringPrefixWithALeftOutSubmoduleDoesNotReachIt(t *testing.T) {
	tree := umbrellaTree(t)
	evaluated := 0
	for _, sibling := range []string{"vendor/extra-other", "vendor/ext", "vendor/extra-other/readme.txt"} {
		in := toyInput(t)
		in.Tree = tree
		addBind(t, &in, "db", fixtureRun+"/src/"+sibling)
		record(svc(t, in, "app")["build"])["context"] = fixtureRun + "/build/" + sibling
		if got := failing(Static(in)); len(got) != 0 {
			t.Errorf("a bind and a build context at %s turned %v red", sibling, got)
		}
		evaluated++
	}
	in := toyInput(t)
	in.Tree = tree
	addBind(t, &in, "db", fixtureRun+"/src/vendor/extra")
	if got := failing(Static(in)); !slices.Equal(got, []string{"mounts.binds"}) {
		t.Fatalf("canary: a bind at vendor/extra itself turned %v red", got)
	}
	if evaluated != 3 {
		t.Fatalf("evaluated %d of 3 siblings", evaluated)
	}
}

func detail(results []Result, name string) string {
	for _, r := range results {
		if r.Name == name {
			return r.Detail
		}
	}
	return ""
}
