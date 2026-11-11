package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose/composetest"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var includeMutations = []porttest.Mutation{
	{"included services, volumes and networks join the model, the declaration checks and the harness", "skip the include items in walk.visit", "red: queue is missing and the declaration is rejected at project.expectedServices[4]", "TestServicesVolumesAndNetworksOfANestedIncludeChainJoinTheModelTheChecksAndTheHarness"},
	{"a top-level include path resolves against compose.projectDirectory", "resolve the includes of a declared file against the directory of that file", "red: stack/inc.yaml beside the declared file is read and the root inc.yaml is not", "TestATopLevelIncludePathResolvesAgainstTheProjectDirectoryAndNotAgainstTheFileDirectory"},
	{"an include cycle is rejected", "drop the chain check in walk.visit", "red: a.yaml and b.yaml include each other until the 64-read cap, not a cycle rejection", "TestAnIncludeCycleIsRejectedAndTheSameFileReachedThroughTwoChainsPasses"},
	{"the model reads at most 64 compose files", "drop the reads check in walk.visit", "red: 65 reads pass", "TestMoreThan64ComposeFileReadsAreRejectedAtTheReadThatCrossesTheCapAnd64Pass"},
	{"a submodule root is a project directory of an include", "make Tree.IsDir true only for tree entries", "red: project_directory deps/queue is not a directory in the commit", "TestAnIncludeIntoASubmoduleReadsItsPinnedFilesAndAValueClimbingOutOfTheCommitFromThereIsRejected"},
}

func TestIncludeMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "include_test.go", includeMutations)
}

const queueStack = "services:\n  queue:\n    image: docker.io/library/alpine:3.22\n"

type includeCase struct {
	name    string
	include string
	files   map[string]string
	links   map[string]string
	field   string
	says    string
}

func (c includeCase) mutate(t *testing.T) func(string) {
	return func(d string) {
		replaceIn(t, filepath.Join(d, ".gitignore"), ".env\n", "/.env\n")
		if c.include != "" {
			top := filepath.Join(d, "compose.yaml")
			writeFile(t, top, c.include+readFile(t, top))
		}
		for name, body := range c.files {
			writeFile(t, filepath.Join(d, filepath.FromSlash(name)), body)
		}
		for name, target := range c.links {
			link := filepath.Join(d, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func declare(t *testing.T, d, old, new string) {
	t.Helper()
	replaceIn(t, filepath.Join(d, DeclarationPath), old, new)
}

func includeRejected(t *testing.T, c includeCase, err error) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: accepted", c.name)
		return
	}
	if r := rejection(t, err); r.Kind != RejectCompose || r.Field != c.field || !strings.Contains(r.Message, c.says) {
		t.Errorf("%s: %v, want a compose rejection at %s saying %q", c.name, r, c.field, c.says)
	}
}

func TestServicesVolumesAndNetworksOfANestedIncludeChainJoinTheModelTheChecksAndTheHarness(t *testing.T) {
	chain := includeCase{include: "include:\n  - stack/compose.yaml\n", files: map[string]string{
		"stack/compose.yaml":       "include:\n  - extra/compose.yaml\nservices:\n  queue:\n    image: docker.io/library/alpine:3.22\n    ports: [\"5672:5672\"]\n    networks: [backplane]\nnetworks:\n  backplane: {}\n",
		"stack/extra/compose.yaml": "services:\n  worker:\n    image: docker.io/library/alpine:3.22\n    volumes: [\"worker-data:/data\"]\nvolumes:\n  worker-data: {}\n",
		"extra/compose.yaml":       "services:\n  decoy:\n    image: docker.io/library/alpine:3.22\n",
	}}
	names := func(d string) {
		declare(t, d, "expectedServices: [app, db, cache, seed]", "expectedServices: [app, db, cache, seed, queue, worker]")
		declare(t, d, "display: Toy Valkey, open: copy }\n", "display: Toy Valkey, open: copy }\n  - { name: queue, offset: 3, service: queue, port: 5672, display: Queue, open: copy }\n")
	}
	bare := chain
	bare.include = ""
	_, err := loadToy(t, func(d string) { bare.mutate(t)(d); names(d) })
	if r := rejection(t, err); r.Kind != RejectCompose || r.Field != "project.expectedServices[4]" {
		t.Fatalf("canary: without the include the queue service is not missing: %v", r)
	}
	loaded, err := loadToy(t, func(d string) { chain.mutate(t)(d); names(d) })
	if err != nil {
		t.Fatal(err)
	}
	m := loaded.Model
	if got := strings.Join(m.ServiceNames(), ","); got != "app,builder,cache,db,queue,seed,worker" {
		t.Fatalf("services %s", got)
	}
	if _, ok := m.Volumes["worker-data"]; !ok || !slices.Contains(sortedKeys(m.Networks), "backplane") {
		t.Fatalf("volumes %v networks %v", m.Volumes, m.Networks)
	}
	if want := []string{"compose.yaml", "compose.sealed-app.yaml", "stack/compose.yaml", "stack/extra/compose.yaml"}; !slices.Equal(m.Files, want) {
		t.Fatalf("files %q, want %q", m.Files, want)
	}
	doc, node, _ := harnessDocFrom(t, HarnessInput{RunID: runID, RunnerImage: "cockpit-runner:0123456789ab", PortBase: 22100, Layout: testLayout(t), Loaded: loaded})
	if ports := serviceField(t, node, "queue", "ports"); ports.Tag != "!reset" || len(ports.Content) != 0 {
		t.Errorf("queue ports %q %d", ports.Tag, len(ports.Content))
	}
	for _, name := range []string{"queue", "worker"} {
		if labels := service(t, doc, name)["labels"].(map[string]any); labels[LabelManaged] != "true" || labels[LabelRunID] != runID {
			t.Errorf("%s labels %v", name, labels)
		}
	}
	if doc["networks"].(map[string]any)["backplane"].(map[string]any)["internal"] != true {
		t.Error("the included network backplane is not internal")
	}
	if doc["volumes"].(map[string]any)["worker-data"].(map[string]any)["labels"].(map[string]any)[LabelRunID] != runID {
		t.Error("the included volume worker-data carries no run label")
	}
}

func TestATopLevelIncludePathResolvesAgainstTheProjectDirectoryAndNotAgainstTheFileDirectory(t *testing.T) {
	third := func(projectDirectory string, files map[string]string) (Loaded, error) {
		files["stack/compose.include.yaml"] = "include:\n  - inc.yaml\n"
		c := includeCase{files: files}
		return loadToy(t, func(d string) {
			c.mutate(t)(d)
			declare(t, d, "files: [compose.yaml, compose.sealed-app.yaml]", "files: [compose.yaml, compose.sealed-app.yaml, stack/compose.include.yaml]"+projectDirectory)
		})
	}
	decoy := "services:\n  decoy:\n    image: docker.io/library/alpine:3.22\n"
	_, err := third("", map[string]string{"stack/inc.yaml": decoy})
	includeRejected(t, includeCase{name: "a file only beside the declared file", field: "compose.files[2] include[0]", says: `"inc.yaml" resolves to "inc.yaml", which is not a file`}, err)
	rooted, err := third("", map[string]string{"stack/inc.yaml": decoy, "inc.yaml": queueStack})
	if err != nil || !rooted.Model.has("queue") || rooted.Model.has("decoy") {
		t.Fatalf("without projectDirectory: services %v, %v", rooted.Model.ServiceNames(), err)
	}
	nested, err := third("\n  projectDirectory: stack", map[string]string{"stack/inc.yaml": decoy, "inc.yaml": queueStack})
	if err != nil || nested.Model.has("queue") || !nested.Model.has("decoy") || !slices.Contains(nested.Model.Files, "stack/inc.yaml") {
		t.Fatalf("with projectDirectory stack: services %v files %v, %v", nested.Model.ServiceNames(), nested.Model.Files, err)
	}
}

func TestAnIncludeCycleIsRejectedAndTheSameFileReachedThroughTwoChainsPasses(t *testing.T) {
	cycles := []includeCase{
		{name: "a.yaml and b.yaml include each other", include: "include:\n  - a.yaml\n", files: map[string]string{"a.yaml": "include:\n  - b.yaml\n", "b.yaml": "include:\n  - a.yaml\n"},
			field: "compose.files[0] include[0] include[0] include[0]", says: "include cycle at a.yaml"},
		{name: "a.yaml includes the declared file", include: "include:\n  - a.yaml\n", files: map[string]string{"a.yaml": "include:\n  - compose.yaml\n"},
			field: "compose.files[0] include[0] include[0]", says: "include cycle at compose.yaml"},
	}
	for _, c := range cycles {
		_, err := loadToy(t, c.mutate(t))
		includeRejected(t, c, err)
	}
	diamond := includeCase{include: "include:\n  - a.yaml\n  - b.yaml\n", files: map[string]string{"a.yaml": "include:\n  - c.yaml\n", "b.yaml": "include:\n  - c.yaml\n", "c.yaml": queueStack}}
	loaded, err := loadToy(t, diamond.mutate(t))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"compose.yaml", "compose.sealed-app.yaml", "a.yaml", "c.yaml", "b.yaml"}; !slices.Equal(loaded.Model.Files, want) || !loaded.Model.has("queue") {
		t.Fatalf("diamond files %q services %v", loaded.Model.Files, loaded.Model.ServiceNames())
	}
}

func fanOut(t *testing.T, n int) func(string) {
	return func(d string) {
		var list strings.Builder
		list.WriteString("include:\n")
		for i := range n {
			fmt.Fprintf(&list, "  - inc/f%02d.yaml\n", i)
			writeFile(t, filepath.Join(d, "inc", fmt.Sprintf("f%02d.yaml", i)), "services: {}\n")
		}
		sealed := filepath.Join(d, "compose.sealed-app.yaml")
		writeFile(t, sealed, list.String()+readFile(t, sealed))
	}
}

func TestMoreThan64ComposeFileReadsAreRejectedAtTheReadThatCrossesTheCapAnd64Pass(t *testing.T) {
	at64, err := loadToy(t, fanOut(t, maxComposeReads-2))
	if err != nil || len(at64.Model.Files) != maxComposeReads {
		t.Fatalf("64 reads: %d files, %v", len(at64.Model.Files), err)
	}
	_, err = loadToy(t, fanOut(t, maxComposeReads-1))
	includeRejected(t, includeCase{name: "65 reads", field: "compose.files[1] include[62]", says: "more than 64 files"}, err)
}

func TestAnIncludeIntoASubmoduleReadsItsPinnedFilesAndAValueClimbingOutOfTheCommitFromThereIsRejected(t *testing.T) {
	c := includeCase{include: "include:\n  - path: deps/queue/compose.yaml\n    project_directory: deps/queue\n", files: map[string]string{
		"deps/queue/compose.yaml": "include:\n  - extra.yaml\n" + queueStack,
		"deps/queue/extra.yaml":   "services:\n  worker:\n    image: docker.io/library/alpine:3.22\n",
	}}
	dir, _ := toyRepo(t, c.mutate(t))
	composetest.Submodule(t, dir, "deps/queue", "queue")
	loaded, err := ReadDeclaration(context.Background(), Git{Dir: dir}, gitT(t, dir, "rev-parse", "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	if e := loaded.Tree.Entries["deps/queue"]; e.Type != submoduleType || !loaded.Model.has("queue") || !loaded.Model.has("worker") || !slices.Contains(loaded.Model.Files, "deps/queue/extra.yaml") {
		t.Fatalf("submodule entry %+v services %v files %v", e, loaded.Model.ServiceNames(), loaded.Model.Files)
	}
	sub := filepath.Join(dir, "deps", "queue")
	writeFile(t, filepath.Join(sub, "extra.yaml"), "include:\n  - ../../../outside.yaml\n")
	composetest.Git(t, sub, "commit", "-q", "-a", "-m", "climb")
	_, err = ReadDeclaration(context.Background(), Git{Dir: dir}, commit(t, dir, func() {}))
	includeRejected(t, includeCase{name: "a climb out of the submodule", field: "compose.files[0] include[0].path include[0] include[0]", says: `resolves to "../outside.yaml", outside the commit`}, err)
}
