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

const adversarialRun = "Test적대적_스크립트는_표에_적힌_단계와_사유로_모두_거부된다"

var adversarialMutations = []mutation{
	{"적대적 표와 디렉토리의 양방향 대조", "표에서 busy-loop.ts 행을 지운다", "빨강: 디렉토리의 busy-loop.ts 가 표에 없다", "Test적대적_코퍼스의_파일과_표가_양방향으로_같다"},
	{"번들 경계 어휘 검사", "boundary.resolve 의 within 검사를 false 로 바꾼다", "빨강: escape-commit.ts 가 경계 거부 대신 Could not resolve 로 떨어진다", adversarialRun},
	{"bare specifier 거부", "boundary.resolve 의 bare 분기가 빈 결과를 돌려준다", "빨강: node-builtin.ts·bare-specifier.ts 가 bare 거부 없이 Could not resolve 로 떨어진다", adversarialRun},
	{"동적 import 거부", "ResolveJSDynamicImport 분기를 false 로 바꾼다", "빨강: dynamic-import.ts:2 거부가 사라지고 node-builtin.ts:1 거부로 옮겨 간다", adversarialRun},
	{"엔진 파싱 검사", "BuildBundle 이 parseProblems 결과를 버린다", "빨강: dynamic-import-variable.ts 가 번들을 통과한다", adversarialRun},
	{"자리별 cockpit 모듈 번들 검사", "boundary.resolve 의 site.allows 검사를 false 로 바꾼다", "빨강: 러너 자리 foreign-module.ts 가 번들을 통과한다", adversarialRun},
	{"require 허용목록 관문", "Host.require 의 allowed 검사를 false 로 바꾼다", "빨강: buffer·node:buffer 가 풀리고 eval-require.ts 의 문장이 바뀐다", adversarialRun},
	{"eval 소스맵 디스크 읽기 차단", "런타임 파서 옵션의 로더를 nil 로 바꾼다", "빨강: eval 스택에 PLANTED_MAP_SOURCE.ts 가 나타난다", adversarialRun},
	{"주요 프로토타입 동결", "setup 이 freezePrototypes 를 건너뛴다", "빨강: pollute true·inherited 1000000000 이 기록된다", adversarialRun},
	{"포트 객체 깊은 동결", "loadModule 이 freezeDeep 을 건너뛴다", "빨강: overwrite true 이고 7 이 포트에 닿지 않는다", adversarialRun},
	{"포트 옵션 own property 읽기", "DecodeArgs 가 프로토타입 체인을 타는 Get 으로 maxRows 를 읽는다", "빨강: 포트가 읽은 maxRows 가 [0 1000000000 5 7] 이다", adversarialRun},
	{"기한의 인터럽트", "기한 타이머를 1시간으로 늘린다", "빨강: busy-loop.ts 가 끝나지 않아 20초 시험 기한에 걸린다", adversarialRun},
	{"호출 스택 상한", "기본 상한을 1<<30 으로 바꾼다", "빨강: deep-recursion.ts 가 stack_overflow 대신 deadline 으로 끝난다", adversarialRun},
	{"기다리지 않은 거부 추적", "SetPromiseRejectionTracker 에 nil 을 준다", "빨강: unhandled-rejection.ts 가 성공으로 끝난다", adversarialRun},
	{"settle 되지 않은 플로우 판정", "after 의 pending==0 분기를 false 로 바꾼다", "빨강: never-settles.ts 가 unsettled 대신 deadline 으로 끝난다", adversarialRun},
	{"심볼릭 링크 실경로 경계", "boundary.load 의 within 검사를 지운다", "빨강: 밖을 가리키는 링크가 번들을 통과한다", "Test경계_안의_심볼릭_링크가_경계_밖을_가리키면_번들이_거부되고_안을_가리키면_통과한다"},
	{"경계 안 패키지의 bare specifier 거부", "boundary.resolve 의 bare 분기가 빈 결과를 돌려준다", "빨강: node_modules/left-pad 가 bare import 로 번들에 들어간다", "Test경계_안에_패키지가_있어도_bare_specifier는_거부되고_같은_파일의_상대_경로는_통과한다"},
	{"경계 밖 tsconfig 차단", "TsconfigRaw 를 비운다", "빨강: 상위 tsconfig 가 읽혀 setter 1·assign 이 기록된다", "Test번들은_경계_밖_상위_디렉토리의_tsconfig를_읽지_않는다"},
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
	{file: "bare-specifier.ts", site: SiteHub, root: commitRoot, stage: rejectAtBundle, line: 1, text: `bare specifier "left-pad" is not allowed`},
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
			t.Fatalf("기록 %q, 기대 %q", p.records, want)
		}
	}
}

func everyRequireDenied(t *testing.T, p *probe) {
	targets := []string{p.planted["absolute"], p.planted["relativeJson"], "buffer", "node:buffer", "left-pad", "process", p.foreign()}
	if len(p.records) != len(targets) {
		t.Fatalf("시도 %d 건, 기대 %d 건: %q", len(p.records), len(targets), p.records)
	}
	for i, r := range p.records {
		if !strings.HasPrefix(r, `denied Invalid module "`+targets[i]+`"`) || strings.Contains(r, "PLANTED") {
			t.Errorf("%s 이 막히지 않았다: %q", targets[i], r)
		}
	}
}

func pollutionIgnored(t *testing.T, p *probe) {
	recordsAre("pollute false", "inherited undefined", "overwrite false", "frozen true true true true true")(t, p)
	if !slices.Equal(p.rows, []int{0, 0, 5, 7}) {
		t.Fatalf("포트가 읽은 maxRows %v, 기대 [0 0 5 7]", p.rows)
	}
}

func diskMapIgnored(t *testing.T, p *probe) {
	if len(p.records) != 2 {
		t.Fatalf("기록 %q", p.records)
	}
	if !strings.Contains(p.records[0], "Error: disk") || strings.Contains(p.records[0], "PLANTED_MAP_SOURCE") {
		t.Errorf("eval 이 디스크의 소스맵을 읽었다: %q", p.records[0])
	}
	if !strings.Contains(p.records[1], "INLINE_CANARY.ts") {
		t.Errorf("eval 안의 인라인 소스맵이 적용되지 않았다(카나리): %q", p.records[1])
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

func Test적대적_코퍼스의_파일과_표가_양방향으로_같다(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(scriptRoot, "*.ts"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("코퍼스 파일 %d 개, %v", len(paths), err)
	}
	inDir := map[string]bool{}
	for _, path := range paths {
		inDir[filepath.Base(path)] = true
	}
	inTable := map[string]bool{}
	for _, c := range adversarialCases {
		inTable[c.file] = true
		if !inDir[c.file] {
			t.Errorf("표의 %s 가 디렉토리에 없다", c.file)
		}
	}
	for name := range inDir {
		if !inTable[name] {
			t.Errorf("디렉토리의 %s 가 표에 없다", name)
		}
	}
}

func Test적대적_스크립트는_표에_적힌_단계와_사유로_모두_거부된다(t *testing.T) {
	ran := 0
	for _, c := range adversarialCases {
		t.Run(c.site.String()+"/"+c.file, func(t *testing.T) {
			b, err := bundleAt(t, c.root, c.entry(), c.site)
			switch c.stage {
			case rejectAtBundle:
				assertProblem(t, err, filepath.ToSlash(c.entry()), c.line, c.text)
			case passAtBundle:
				if err != nil || !strings.Contains(b.Code, "sourceMappingURL=data:") {
					t.Fatalf("번들이 통과해야 한다: %v", err)
				}
			case execute:
				if err != nil {
					t.Fatalf("번들: %v", err)
				}
				assertExecution(t, c, b)
			}
			ran++
		})
	}
	if ran != len(adversarialCases) || ran == 0 {
		t.Fatalf("실행 %d 건, 표 %d 건", ran, len(adversarialCases))
	}
}

func assertProblem(t *testing.T, err error, file string, line int, text string) {
	t.Helper()
	var be *BundleError
	if !errors.As(err, &be) || len(be.Problems) == 0 {
		t.Fatalf("번들 거부를 기대했는데 %v", err)
	}
	for _, p := range be.Problems {
		if p.File == file && p.Line == line && strings.Contains(p.Text, text) {
			return
		}
	}
	t.Fatalf("%s:%d 의 %q 거부가 없다: %+v", file, line, text, be.Problems)
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
			t.Fatalf("실행이 끝까지 가야 한다: %v", err)
		}
	} else if f := failureOf(t, err, c.kind); !strings.Contains(f.Message, c.text) {
		t.Fatalf("실패 문장 %q 에 %q 가 없다", f.Message, c.text)
	} else if c.stack != "" && (!strings.Contains(f.Stack, c.stack) || strings.Count(f.Stack, "\n") >= overflowFrames) {
		t.Fatalf("스택이 %q 를 가리키지 않거나 %d 프레임을 넘는다: %q", c.stack, overflowFrames, f.Stack)
	}
	if c.check != nil {
		c.check(t, p)
	}
}

func Test경계_안의_심볼릭_링크가_경계_밖을_가리키면_번들이_거부되고_안을_가리키면_통과한다(t *testing.T) {
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
		t.Fatalf("밖을 가리키는 링크가 거부되지 않았다: %v", err)
	}
	stay, err := bundleAt(t, root, "stay.ts", SiteHub)
	if err != nil || !strings.Contains(stay.Code, "INSIDE_VALUE") {
		t.Fatalf("안을 가리키는 링크는 통과해야 한다: %v", err)
	}
}

func Test경계_안에_패키지가_있어도_bare_specifier는_거부되고_같은_파일의_상대_경로는_통과한다(t *testing.T) {
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
		t.Fatalf("경계 안의 상대 경로는 통과해야 한다: %v", err)
	}
}

func Test번들은_경계_밖_상위_디렉토리의_tsconfig를_읽지_않는다(t *testing.T) {
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

func Test적대적_코퍼스_변이_기록표는_이_파일의_시험과_양방향으로_같다(t *testing.T) {
	assertMutationRecord(t, "adversarial_test.go", adversarialMutations)
}
