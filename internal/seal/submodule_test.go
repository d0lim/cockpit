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
	{"a dockerfile outside its build context that reaches a gitlink the run does not splice fails paths.build", "drop the outsideContext check in pathChecks", "red: the dockerfile ../vendor/extra/Dockerfile of the context build/mock passes paths.build", "TestABindBuildContextOrConfigFileThatReachesALeftOutSubmoduleFailsNamingItAndPassesOnceADeclaredPathReferencesIt"},
	{"a dockerfile inside its build context is judged by the context alone", "return true whatever the path in outsideContext", "red: the dockerfile Dockerfile.sealed inside build/mock/vendor/extra names the gitlink twice", "TestADockerfileInsideItsContextIsJudgedByTheContextAlone"},
	{"a path that contains a left-out gitlink reaches it", "drop the loop over contained gitlinks in compose Tree.LeftOut", "red: the bind of src/vendor and the additional context at the build root pass", "TestABindBuildContextOrConfigFileThatReachesALeftOutSubmoduleFailsNamingItAndPassesOnceADeclaredPathReferencesIt"},
	{"paths under build/ are judged against the tree as well as paths under src/", "map paths only under Layout.Src in leftOut", "red: the builder bind and the build context under build/ pass", "TestABindBuildContextOrConfigFileThatReachesALeftOutSubmoduleFailsNamingItAndPassesOnceADeclaredPathReferencesIt"},
	{"a path that contains a tree root reaches every left-out gitlink of that tree", "map only paths inside Layout.Src or Layout.Build in leftOut, not paths that contain one", "red: the build context at the run directory passes paths.build", "TestABindBuildContextOrConfigFileThatReachesALeftOutSubmoduleFailsNamingItAndPassesOnceADeclaredPathReferencesIt"},
	{"a gitlink reached through both trees is named once", "drop slices.Compact in leftOut", "red: the build context at the run directory names vendor/extra twice", "TestABindBuildContextOrConfigFileThatReachesALeftOutSubmoduleFailsNamingItAndPassesOnceADeclaredPathReferencesIt"},
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
		{"a build context at the run directory, around both trees", "paths.build", func(in *Input) {
			record(svc(t, *in, "app")["build"])["context"] = fixtureRun
		}},
		{"a dockerfile outside its context inside the gitlink", "paths.build", func(in *Input) {
			record(svc(t, *in, "app")["build"])["dockerfile"] = "../vendor/extra/Dockerfile"
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
		if d := detail(results, c.assertion); strings.Count(d, "the submodule vendor/extra, which no declared path references") != 1 {
			t.Errorf("%s: %s does not name the gitlink exactly once: %s", c.name, c.assertion, d)
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
	if denied != 9 || passed != 9 {
		t.Fatalf("%d of 9 copies denied and %d of 9 passed", denied, passed)
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

func TestADockerfileInsideItsContextIsJudgedByTheContextAlone(t *testing.T) {
	tree := umbrellaTree(t)
	in := toyInput(t)
	in.Tree = tree
	if got := failing(Static(in)); len(got) != 0 {
		t.Fatalf("canary: the toy dockerfile inside build/mock turned %v red", got)
	}
	in = toyInput(t)
	in.Tree = tree
	record(svc(t, in, "app")["build"])["context"] = fixtureRun + "/build/vendor/extra"
	results := Static(in)
	if got := failing(results); !slices.Equal(got, []string{"paths.build"}) {
		t.Fatalf("a context at vendor/extra with its dockerfile inside turned %v red", got)
	}
	if d := detail(results, "paths.build"); strings.Count(d, "the submodule vendor/extra") != 1 || strings.Contains(d, "dockerfile") {
		t.Fatalf("paths.build does not name the gitlink once through the context alone: %s", d)
	}
}
