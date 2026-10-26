package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var compatMutations = []porttest.Mutation{
	{"a declaration change that breaks a script written for the current host API fails the build", "delete the ref member of the flow module in registry.go and run go generate", "red: TestEveryUseTheCurrentHostApiPromisedStillTypeChecks reports TS2339 on ctx.ref in flow.ts", "TestEveryUseTheCurrentHostApiPromisedStillTypeChecks"},
	{"narrowing a value the host hands to scripts fails the build", "make HttpResponse.body optional in registry.go and run go generate", "red: TestEveryUseTheCurrentHostApiPromisedStillTypeChecks reports TS2322 where flow.ts and collect.ts read res.body as a string", "TestEveryUseTheCurrentHostApiPromisedStillTypeChecks"},
	{"the fixture grows with every declared name", "add a member to the collect module in registry.go and run go generate without touching the fixture", "red: TestTheHostApiFixtureUsesEveryNameTheDeclarationsDeclare names the new member", "TestTheHostApiFixtureUsesEveryNameTheDeclarationsDeclare"},
}

var (
	declaredName = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:declare\s+)?(?:interface|type|function|class|var)\s+([A-Za-z_]\w*)`)
	memberName   = regexp.MustCompile(`(?m)^\t+(?:readonly\s+)?([A-Za-z_]\w*)\??\s*(?:<[^>]*>)?\s*[:(]`)
)

func compatDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("testdata", "hostapi", fmt.Sprint(HostAPI))
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("no compatibility fixture for host API %d: when a declaration change breaks the previous fixture, raise HostAPI and start %s from a copy of it: %v", HostAPI, dir, err)
	}
	return dir
}

func compatSamples(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".ts") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		t.Fatalf("%s has no fixture scripts", dir)
	}
	return names
}

func TestEveryUseTheCurrentHostApiPromisedStillTypeChecks(t *testing.T) {
	dir := compatDir(t)
	samples := compatSamples(t, dir)
	run := runTscFrom(t, dir, samples...)
	if run.err != nil || len(run.errors) > 0 {
		t.Fatalf("a script written for host API %d no longer type-checks; keep the declarations compatible, or raise HostAPI and start a new fixture: %v\n%s", HostAPI, run.err, run.output)
	}
	if len(run.samples) != len(samples) || len(run.sdk) != 3 {
		t.Fatalf("checked samples %v of %v, declarations %v\n%s", run.samples, samples, run.sdk, run.output)
	}
}

func TestTheHostApiFixtureUsesEveryNameTheDeclarationsDeclare(t *testing.T) {
	dir := compatDir(t)
	var fixture strings.Builder
	for _, name := range compatSamples(t, dir) {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		fixture.Write(data)
	}
	files, err := filepath.Glob(filepath.Join(repoRoot(t), "sdk", "*.d.ts"))
	if err != nil || len(files) != 3 {
		t.Fatalf("declarations %v: %v", files, err)
	}
	var missing []string
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, re := range []*regexp.Regexp{declaredName, memberName} {
			for _, m := range re.FindAllStringSubmatch(string(data), -1) {
				names = append(names, m[1])
			}
		}
		if len(names) == 0 {
			t.Fatalf("no names read from %s", file)
		}
		for _, name := range names {
			if name == "constructor" {
				continue
			}
			if !regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`).MatchString(fixture.String()) {
				missing = append(missing, filepath.Base(file)+": "+name)
			}
		}
	}
	slices.Sort(missing)
	missing = slices.Compact(missing)
	if len(missing) > 0 {
		t.Fatalf("the fixture for host API %d does not use %d declared names; add a use of each to %s:\n%s", HostAPI, len(missing), dir, strings.Join(missing, "\n"))
	}
}

func TestCompatMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "compat_test.go", compatMutations)
}
