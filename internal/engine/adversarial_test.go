package engine

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const adversarialRun = "TestAdversarialScriptsAreRejectedAtTheStageAndReasonListedInTheTable"

var adversarialMutations = []mutation{
	{"two-way match of the adversarial table and directory", "delete the busy-loop.ts row from the table", "red: busy-loop.ts is in the directory but not in the table", "TestAdversarialCorpusFilesAndCaseTableMatchBothWays"},
	{"lexical bundle boundary check", "replace the within check in boundary.resolve with false", "red: escape-commit.ts fails with Could not resolve instead of the boundary rejection", adversarialRun},
	{"bare specifier rejection", "make the bare branch in boundary.resolve return an empty result", "red: node-builtin.ts and bare-specifier.ts fail with Could not resolve instead of the bare rejection", adversarialRun},
	{"dynamic import rejection", "replace the ResolveJSDynamicImport branch with false", "red: the dynamic-import.ts:2 rejection disappears and moves to node-builtin.ts:1", adversarialRun},
	{"engine parse check", "make BuildBundle discard the parseProblems result", "red: dynamic-import-variable.ts passes the bundle step", adversarialRun},
	{"per-site cockpit module bundle check", "replace the site.allows check in boundary.resolve with false", "red: foreign-module.ts at the runner site passes the bundle step", adversarialRun},
	{"require allowlist gate", "replace the allowed check in Host.require with false", "red: buffer and node:buffer load and the eval-require.ts message changes", adversarialRun},
	{"blocking disk reads of eval source maps", "set the loader in the runtime parser options to nil", "red: PLANTED_MAP_SOURCE.ts appears in the eval stack", adversarialRun},
	{"core prototype freeze", "make setup skip freezePrototypes", "red: pollute true and inherited 1000000000 are recorded", adversarialRun},
	{"deep freeze of the port object", "make loadModule skip freezeDeep", "red: overwrite true is recorded and 7 never reaches the port", adversarialRun},
	{"own-property reads of port options", "make DecodeArgs read maxRows with a Get that walks the prototype chain", "red: the port reads maxRows as [0 1000000000 5 7]", adversarialRun},
	{"deadline interrupt", "raise the deadline timer to one hour", "red: busy-loop.ts never ends and hits the 20 second test deadline", adversarialRun},
	{"call stack limit", "change the default limit to 1<<30", "red: deep-recursion.ts ends with deadline instead of stack_overflow", adversarialRun},
	{"tracking unawaited rejections", "pass nil to SetPromiseRejectionTracker", "red: unhandled-rejection.ts ends in success", adversarialRun},
	{"verdict for a flow that never settles", "replace the pending==0 branch in after with false", "red: never-settles.ts ends with deadline instead of unsettled", adversarialRun},
	{"real-path boundary for symbolic links", "delete the within check in boundary.load", "red: a link pointing outside passes the bundle step", "TestSymlinkPointingOutsideTheBoundaryIsRejectedAndOnePointingInsidePasses"},
	{"bare specifier rejection for a package inside the boundary", "make the bare branch in boundary.resolve return an empty result", "red: node_modules/left-pad enters the bundle through a bare import", "TestBareSpecifierToAPackageInsideTheBoundaryIsRejectedButARelativePathToItPasses"},
	{"blocking tsconfig outside the boundary", "empty TsconfigRaw", "red: the parent tsconfig is read and setter 1 and assign are recorded", "TestBundleIgnoresTsconfigInAParentDirectoryOutsideTheBoundary"},
}

type stage string

const (
	rejectAtBundle stage = "bundle"
	passAtBundle   stage = "bundle-pass"
	execute        stage = "execute"
)

type adversarialCase struct {
	file     string
	site     Site
	root     string
	stage    stage
	line     int
	text     string
	kind     FailureKind
	deadline time.Duration
	stack    string
	check    func(t *testing.T, p *probe)
}

var adversarialCases = []adversarialCase{
	{file: "escape-commit.ts", site: SiteHub, root: commitRoot, stage: rejectAtBundle, line: 1, text: `"../../outside/secret" is outside the boundary`},
	{file: "node-builtin.ts", site: SiteHub, root: commitRoot, stage: rejectAtBundle, line: 1, text: `bare specifier "node:fs" is not allowed`},
	{file: "bare-specifier.ts", site: SiteHub, root: commitRoot, stage: rejectAtBundle, line: 1, text: `bare specifier "left-pad" is not allowed: import only cockpit:* modules and relative paths`},
	{file: "dynamic-import.ts", site: SiteHub, root: commitRoot, stage: rejectAtBundle, line: 2, text: `dynamic import of "./node-builtin" is not allowed`},
	{file: "dynamic-import-variable.ts", site: SiteHub, root: commitRoot, stage: rejectAtBundle, line: 4, text: "the engine cannot parse the bundle"},
	{file: "escape-script-dir.ts", site: SiteRunner, root: scriptRoot, stage: rejectAtBundle, line: 1, text: `"../conformance/json-roundtrip" is outside the boundary`},
	{file: "escape-script-dir.ts", site: SiteHub, root: commitRoot, stage: passAtBundle},
	{file: "foreign-module.ts", site: SiteRunner, root: scriptRoot, stage: rejectAtBundle, line: 1, text: `module "cockpit:collect" is not available at the runner site`},
	{file: "foreign-module.ts", site: SiteHub, root: commitRoot, stage: passAtBundle},
	{file: "eval-require.ts", site: SiteRunner, root: scriptRoot, stage: execute, kind: FailThrown, text: `Invalid module "cockpit:collect"`, check: recordsAre("eval reached cockpit:flow")},
	{file: "require-nonliteral.ts", site: SiteRunner, root: scriptRoot, stage: execute, check: everyRequireDenied},
	{file: "require-nonliteral.ts", site: SiteHub, root: commitRoot, stage: execute, check: everyRequireDenied},
	{file: "proto-pollution.ts", site: SiteRunner, root: scriptRoot, stage: execute, check: pollutionIgnored},
	{file: "sourcemap-disk.ts", site: SiteRunner, root: scriptRoot, stage: execute, check: diskMapIgnored},
	{file: "busy-loop.ts", site: SiteRunner, root: scriptRoot, stage: execute, kind: FailDeadline, deadline: 300 * time.Millisecond},
	{file: "deep-recursion.ts", site: SiteRunner, root: scriptRoot, stage: execute, kind: FailStack, deadline: 500 * time.Millisecond, stack: "at down (deep-recursion.ts:2:"},
	{file: "unhandled-rejection.ts", site: SiteRunner, root: scriptRoot, stage: execute, kind: FailUnhandled, text: "dangling rejection"},
	{file: "never-settles.ts", site: SiteRunner, root: scriptRoot, stage: execute, kind: FailUnsettled, deadline: time.Second},
}

func (c adversarialCase) entry() string {
	if c.root == commitRoot {
		return filepath.Join("adversarial", c.file)
	}
	return c.file
}

func recordsAre(want ...string) func(*testing.T, *probe) {
	return func(t *testing.T, p *probe) {
		if !slices.Equal(p.records, want) {
			t.Fatalf("records %q, want %q", p.records, want)
		}
	}
}

func everyRequireDenied(t *testing.T, p *probe) {
	targets := []string{p.planted["absolute"], p.planted["relativeJson"], "buffer", "node:buffer", "left-pad", "process", p.foreign()}
	if len(p.records) != len(targets) {
		t.Fatalf("%d attempts, want %d: %q", len(p.records), len(targets), p.records)
	}
	for i, r := range p.records {
		if !strings.HasPrefix(r, `denied Invalid module "`+targets[i]+`"`) || strings.Contains(r, "PLANTED") {
			t.Errorf("%s was not denied: %q", targets[i], r)
		}
	}
}

func pollutionIgnored(t *testing.T, p *probe) {
	recordsAre("pollute false", "inherited undefined", "overwrite false", "frozen true true true true true")(t, p)
	if !slices.Equal(p.rows, []int{0, 0, 5, 7}) {
		t.Fatalf("port read maxRows %v, want [0 0 5 7]", p.rows)
	}
}

func diskMapIgnored(t *testing.T, p *probe) {
	if len(p.records) != 2 {
		t.Fatalf("records %q", p.records)
	}
	if !strings.Contains(p.records[0], "Error: disk") || strings.Contains(p.records[0], "PLANTED_MAP_SOURCE") {
		t.Errorf("eval read the source map from disk: %q", p.records[0])
	}
	if !strings.Contains(p.records[1], "INLINE_CANARY.ts") {
		t.Errorf("inline source map inside eval was not applied (canary): %q", p.records[1])
	}
}

func plantAll(t *testing.T, p *probe) {
	abs, rel := plant(t, "planted.json", `{"secret":"PLANTED_JSON_CONTENT"}`)
	_, mapRel := plant(t, "planted.map", `{"version":3,"sources":["PLANTED_MAP_SOURCE.ts"],"names":[],"mappings":"AAAA;AACA"}`)
	inline := `{"version":3,"sources":["INLINE_CANARY.ts"],"names":[],"mappings":"AAAA;AACA"}`
	p.planted["absolute"] = abs
	p.planted["relativeJson"] = rel
	p.planted["map"] = mapRel
	p.planted["inlineMap"] = "data:application/json;base64," + base64.StdEncoding.EncodeToString([]byte(inline))
}

func TestAdversarialCorpusFilesAndCaseTableMatchBothWays(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(scriptRoot, "*.ts"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("%d corpus files, %v", len(paths), err)
	}
	inDir := map[string]bool{}
	for _, path := range paths {
		inDir[filepath.Base(path)] = true
	}
	inTable := map[string]bool{}
	for _, c := range adversarialCases {
		inTable[c.file] = true
		if !inDir[c.file] {
			t.Errorf("%s is in the table but not in the directory", c.file)
		}
	}
	for name := range inDir {
		if !inTable[name] {
			t.Errorf("%s is in the directory but not in the table", name)
		}
	}
}

func TestAdversarialScriptsAreRejectedAtTheStageAndReasonListedInTheTable(t *testing.T) {
	ran := 0
	for _, c := range adversarialCases {
		t.Run(c.site.String()+"/"+c.file, func(t *testing.T) {
			b, err := bundleAt(t, c.root, c.entry(), c.site)
			switch c.stage {
			case rejectAtBundle:
				assertProblem(t, err, filepath.ToSlash(c.entry()), c.line, c.text)
			case passAtBundle:
				if err != nil || !strings.Contains(b.Code, "sourceMappingURL=data:") {
					t.Fatalf("bundle should pass: %v", err)
				}
			case execute:
				if err != nil {
					t.Fatalf("bundle: %v", err)
				}
				assertExecution(t, c, b)
			}
			ran++
		})
	}
	if ran != len(adversarialCases) || ran == 0 {
		t.Fatalf("ran %d cases, table has %d", ran, len(adversarialCases))
	}
}

func assertProblem(t *testing.T, err error, file string, line int, text string) {
	t.Helper()
	var be *BundleError
	if !errors.As(err, &be) || len(be.Problems) == 0 {
		t.Fatalf("want a bundle rejection, got %v", err)
	}
	for _, p := range be.Problems {
		if p.File == file && p.Line == line && strings.Contains(p.Text, text) {
			return
		}
	}
	t.Fatalf("no rejection at %s:%d containing %q: %+v", file, line, text, be.Problems)
}

func assertExecution(t *testing.T, c adversarialCase, b Bundle) {
	t.Helper()
	p := newProbe(c.site)
	plantAll(t, p)
	deadline := c.deadline
	if deadline == 0 {
		deadline = 5 * time.Second
	}
	err := newEngine(t, c.site, p, deadline).Run(context.Background(), b, "default")
	if c.kind == "" {
		if err != nil {
			t.Fatalf("run should complete: %v", err)
		}
	} else if f := failureOf(t, err, c.kind); !strings.Contains(f.Message, c.text) {
		t.Fatalf("failure message %q does not contain %q", f.Message, c.text)
	} else if c.stack != "" && (!strings.Contains(f.Stack, c.stack) || strings.Count(f.Stack, "\n") >= overflowFrames) {
		t.Fatalf("stack does not point at %q or exceeds %d frames: %q", c.stack, overflowFrames, f.Stack)
	}
	if c.check != nil {
		c.check(t, p)
	}
}

func TestSymlinkPointingOutsideTheBoundaryIsRejectedAndOnePointingInsidePasses(t *testing.T) {
	outside := writeTree(t, map[string]string{"secret.ts": "export const secret = \"PLANTED_OUTSIDE\";\n"})
	root := writeTree(t, map[string]string{
		"inside.ts": "export const value = \"INSIDE_VALUE\";\n",
		"escape.ts": "import { secret } from \"./link\";\n\nexport default () => secret;\n",
		"stay.ts":   "import { value } from \"./alias\";\n\nexport default () => value;\n",
	})
	if err := os.Symlink(filepath.Join(outside, "secret.ts"), filepath.Join(root, "link.ts")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "inside.ts"), filepath.Join(root, "alias.ts")); err != nil {
		t.Fatal(err)
	}
	b, err := bundleAt(t, root, "escape.ts", SiteHub)
	var be *BundleError
	if !errors.As(err, &be) || !strings.Contains(be.Error(), "outside the boundary") || strings.Contains(b.Code+be.Error(), "PLANTED_OUTSIDE") {
		t.Fatalf("link pointing outside was not rejected: %v", err)
	}
	stay, err := bundleAt(t, root, "stay.ts", SiteHub)
	if err != nil || !strings.Contains(stay.Code, "INSIDE_VALUE") {
		t.Fatalf("link pointing inside should pass: %v", err)
	}
}

func TestBareSpecifierToAPackageInsideTheBoundaryIsRejectedButARelativePathToItPasses(t *testing.T) {
	root := writeTree(t, map[string]string{
		"node_modules/left-pad/package.json": `{"name":"left-pad","main":"index.js"}`,
		"node_modules/left-pad/index.js":     "module.exports = () => \"PADDED_BY_PACKAGE\";\n",
		"bare.ts":                            "import pad from \"left-pad\";\n\nexport default () => pad();\n",
		"relative.ts":                        "import pad from \"./node_modules/left-pad/index.js\";\n\nexport default () => pad();\n",
	})
	_, err := bundleAt(t, root, "bare.ts", SiteHub)
	assertProblem(t, err, "bare.ts", 1, `bare specifier "left-pad" is not allowed`)
	b, err := bundleAt(t, root, "relative.ts", SiteHub)
	if err != nil || !strings.Contains(b.Code, "PADDED_BY_PACKAGE") {
		t.Fatalf("relative path inside the boundary should pass: %v", err)
	}
}

func TestBundleIgnoresTsconfigInAParentDirectoryOutsideTheBoundary(t *testing.T) {
	parent := writeTree(t, map[string]string{
		"tsconfig.json": `{"compilerOptions":{"useDefineForClassFields":false}}`,
		"scripts/entry.ts": `import { record } from "cockpit:flow";

class Base {
	set x(value: number) {
		record("setter " + value);
	}
}

class Child extends Base {
	x = 1;
}

export default function (): void {
	record(Object.hasOwn(new Child(), "x") ? "define" : "assign");
}
`,
	})
	p := newProbe(SiteRunner)
	b := mustBundle(t, filepath.Join(parent, "scripts"), "entry.ts", SiteRunner)
	if err := newEngine(t, SiteRunner, p, 5*time.Second).Run(context.Background(), b, "default"); err != nil {
		t.Fatal(err)
	}
	recordsAre("define")(t, p)
}

func TestAdversarialMutationRecordAndTestsInThisFileMatchBothWays(t *testing.T) {
	assertMutationRecord(t, "adversarial_test.go", adversarialMutations)
}
