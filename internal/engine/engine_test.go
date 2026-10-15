package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

var engineMutations = []mutation{
	{"Go 에러 문장 가리기", "Host.Error 가 err.Error() 를 문장으로 쓴다", "빨강: PLANTED_GO_ERROR 가 스크립트에 보인다", "Test포트_Async는_결과를_평범한_JS_값으로_풀고_Go_에러는_안정_코드로_거절한다"},
	{"포트 결과의 평범한 JS 값 변환", "Async 가 h.Value 대신 rt.ToValue 를 쓴다", "빨강: Go 필드 이름과 Credential 메서드가 스크립트에 보인다", "Test포트_Async는_결과를_평범한_JS_값으로_풀고_Go_에러는_안정_코드로_거절한다"},
	{"결과가 정해진 뒤의 완료 가드", "callback 의 done·finished 검사를 지운다", "빨강: late completion woke the script 가 기록된다", "Test결과가_정해진_뒤_도착한_포트_완료는_스크립트를_깨우지_않는다"},
	{"기한이 대기 중인 루프를 멈춘다", "기한 감시에서 StopNoWait 를 지운다", "빨강: 포트를 기다리는 런이 끝나지 않아 20초 시험 기한에 걸린다", "Test기한_뒤에_늦게_끝난_포트_작업의_완료는_아무것도_하지_않는다"},
	{"인라인 소스맵", "Sourcemap 을 SourceMapNone 으로 바꾼다", "빨강: 스택이 bundle.js:29 를 가리킨다", "Test단계_안에서_던진_예외의_스택은_TS_파일과_줄을_가리킨다"},
	{"진입 내보내기 적재 실패 분류", "함수 아닌 진입을 thrown 으로 분류한다", "빨강: load 대신 thrown 이다", "Test진입_내보내기가_함수가_아니면_적재_실패다"},
	{"부모 컨텍스트 취소 감시", "Run 이 부모 대신 context.Background 에서 파생한다", "빨강: canceled 대신 10초 뒤 deadline 이다", "Test부모_컨텍스트가_취소되면_취소로_끝난다"},
	{"자리에 없는 모듈 구성 거부", "New 의 site.allows 검사를 false 로 바꾼다", "빨강: 러너에 cockpit:collect·허브에 cockpit:flow 구성이 받아들여진다", "TestNew는_자리에_없는_모듈과_빈_기한을_거부하고_맞는_구성은_받는다"},
	{"console 의 구조화 로그 경로", "printer.Log 를 빈 함수로 바꾼다", "빨강: INFO 줄이 사라진다", "Test스크립트의_console은_구조화_로그로_가고_수준을_지킨다"},
}

func Test포트_Async는_결과를_평범한_JS_값으로_풀고_Go_에러는_안정_코드로_거절한다(t *testing.T) {
	p := newProbe(SiteRunner)
	err := runSource(t, SiteRunner, p, `import { echo, deny, leak, badArgs, record } from "cockpit:flow";

const describe = (e: any): string => [e instanceof Error, e.code, e.message].join("|");

export default async function (): Promise<void> {
	const v = await echo();
	record([JSON.stringify(v), typeof v.Credential, Object.getPrototypeOf(v) === Object.prototype, Array.isArray(v.items)].join("|"));
	await deny().catch((e: unknown) => record(describe(e)));
	await leak().catch((e: unknown) => record(describe(e)));
	try {
		badArgs();
	} catch (e) {
		record(describe(e));
	}
}
`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`{"status":201,"items":["a","b"]}|undefined|true|true`,
		"true|PORT_DENIED|denied by policy ****",
		"true|PORT_UNAVAILABLE|port call failed",
		"true|PORT_BAD_ARGS|maxRows must be a number",
	}
	if !slices.Equal(p.records, want) || strings.Contains(strings.Join(p.records, "\n"), "PLANTED_GO") {
		t.Fatalf("기록 %q, 기대 %q", p.records, want)
	}
}

func Test기한_뒤에_늦게_끝난_포트_작업의_완료는_아무것도_하지_않는다(t *testing.T) {
	p := newProbe(SiteRunner)
	root := writeTree(t, map[string]string{"entry.ts": `import { wait, record } from "cockpit:flow";

export default async function (): Promise<void> {
	record("started");
	record(String(await wait()));
}
`})
	b := mustBundle(t, root, "entry.ts", SiteRunner)
	eng := newEngine(t, SiteRunner, p, 200*time.Millisecond)
	before := runtime.NumGoroutine()
	failureOf(t, eng.Run(context.Background(), b, "default"), FailDeadline)
	close(p.release)
	until := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(until) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Fatalf("포트 goroutine 이 남았다: %d > %d", n, before)
	}
	if !slices.Equal(p.records, []string{"started"}) {
		t.Fatalf("늦은 완료가 스크립트를 깨웠다: %q", p.records)
	}
}

func Test결과가_정해진_뒤_도착한_포트_완료는_스크립트를_깨우지_않는다(t *testing.T) {
	p := newProbe(SiteRunner)
	err := runSource(t, SiteRunner, p, `import { hold, releaseInOrder, record } from "cockpit:flow";

export default async function (): Promise<void> {
	const first = hold("a");
	const second = hold("b");
	second.then(() => record("late completion woke the script"));
	releaseInOrder();
	const until = Date.now() + 60;
	while (Date.now() < until) {}
	record(String(await first));
}
`)
	if err != nil || !slices.Equal(p.records, []string{"released"}) {
		t.Fatalf("기록 %q, 실행 %v", p.records, err)
	}
}

func Test단계_안에서_던진_예외의_스택은_TS_파일과_줄을_가리킨다(t *testing.T) {
	root := writeTree(t, map[string]string{
		"flows/step.ts":  "export function run(): void {\n\tthrow new Error(\"step failed\");\n}\n",
		"suites/main.ts": "import { run } from \"../flows/step\";\n\nexport default function (): void {\n\trun();\n}\n",
	})
	err := newEngine(t, SiteRunner, nil, 5*time.Second).Run(context.Background(), mustBundle(t, root, "suites/main.ts", SiteRunner), "default")
	f := failureOf(t, err, FailThrown)
	if f.Message != "step failed" || !strings.Contains(f.Stack, "at run (flows/step.ts:2:") || !strings.Contains(f.Stack, "suites/main.ts:4:") {
		t.Fatalf("문장 %q 스택 %q", f.Message, f.Stack)
	}
}

func Test진입_내보내기가_함수가_아니면_적재_실패다(t *testing.T) {
	f := failureOf(t, runSource(t, SiteRunner, nil, "export const value = 1;\n"), FailLoad)
	if !strings.Contains(f.Message, `"default"`) {
		t.Fatalf("문장 %q", f.Message)
	}
}

func Test부모_컨텍스트가_취소되면_취소로_끝난다(t *testing.T) {
	root := writeTree(t, map[string]string{"entry.ts": "export default function (): void {\n\tfor (;;) {}\n}\n"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	time.AfterFunc(100*time.Millisecond, cancel)
	err := newEngine(t, SiteRunner, nil, 10*time.Second).Run(ctx, mustBundle(t, root, "entry.ts", SiteRunner), "default")
	failureOf(t, err, FailCanceled)
}

func TestNew는_자리에_없는_모듈과_빈_기한을_거부하고_맞는_구성은_받는다(t *testing.T) {
	builder := func(*Host) (goja.Value, error) { return goja.Undefined(), nil }
	cases := []struct {
		opts Options
		want string
	}{
		{Options{Site: SiteRunner, Budget: Budget{Deadline: time.Second}, Modules: map[string]ModuleBuilder{ModuleCollect: builder}}, `"cockpit:collect"`},
		{Options{Site: SiteHub, Budget: Budget{Deadline: time.Second}, Modules: map[string]ModuleBuilder{ModuleFlow: builder}}, `"cockpit:flow"`},
		{Options{Site: SiteRunner, Budget: Budget{Deadline: time.Second}, Modules: map[string]ModuleBuilder{ModuleFlow: nil}}, "nil"},
		{Options{Site: SiteRunner}, "deadline"},
		{Options{Site: SiteRunner, Budget: Budget{Deadline: time.Second, MaxCallStack: -1}}, "call stack"},
		{Options{Budget: Budget{Deadline: time.Second}}, "site"},
		{Options{Site: SiteRunner, Budget: Budget{Deadline: time.Second}, Modules: map[string]ModuleBuilder{ModuleFlow: builder}}, ""},
		{Options{Site: SiteHub, Budget: Budget{Deadline: time.Second}, Modules: map[string]ModuleBuilder{ModuleCollect: builder}}, ""},
	}
	for i, c := range cases {
		_, err := New(c.opts)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%d: 맞는 구성이 거부됐다: %v", i, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%d: %q 거부를 기대했는데 %v", i, c.want, err)
		}
	}
}

func Test스크립트의_console은_구조화_로그로_가고_수준을_지킨다(t *testing.T) {
	var out bytes.Buffer
	root := writeTree(t, map[string]string{"entry.ts": "export default function (): void {\n\tconsole.log(\"seen\", 1);\n\tconsole.error(\"broken %s\", \"x\");\n}\n"})
	eng, err := New(Options{Site: SiteRunner, Budget: Budget{Deadline: 5 * time.Second}, Log: slog.New(slog.NewJSONHandler(&out, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Run(context.Background(), mustBundle(t, root, "entry.ts", SiteRunner), "default"); err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var entry struct{ Level, Msg, Text string }
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("구조화 로그가 아니다: %q", line)
		}
		lines = append(lines, entry.Level+"|"+entry.Msg+"|"+entry.Text)
	}
	if want := []string{"INFO|console|seen 1", "ERROR|console|broken x"}; !slices.Equal(lines, want) {
		t.Fatalf("로그 %q, 기대 %q", lines, want)
	}
}

func Test엔진_변이_기록표는_이_파일의_시험과_양방향으로_같다(t *testing.T) {
	assertMutationRecord(t, "engine_test.go", engineMutations)
}
