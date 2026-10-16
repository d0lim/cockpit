package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var runnerMutations = []porttest.Mutation{
	{"hostApi 불일치는 매니페스트 전에 2", "Main 의 hostApi 비교를 지운다", "빨강: 종료 0 이고 manifest.json 이 생긴다", "Test선언의_hostApi_가_러너와_다르면_매니페스트를_쓰기_전에_종료_2_이고_사유에_두_값이_적힌다"},
	{"선언 없음은 거부", "선언 파일이 없을 때 hostApi 를 러너 값으로 둔다", "빨강: 선언 없이 종료 0", "Test선언이_없거나_읽히지_않으면_종료_2_다"},
	{"once 이미 돎 거부", "PlanExec 의 ran 검사를 지운다", "빨강: alpha 가 다시 돌고 종료 0", "TestAE3_원장에_돈_기록이_있는_once_플로우는_돌지_않고_종료_2_와_이유_줄을_낸다"},
	{"once 전제 미통과 거부", "PlanExec 의 passed 검사를 지운다", "빨강: 종료 2 대신 요청이 나간다", "TestAE4_once_전제가_원장에_passed_가_아니면_종료_2_이고_아무것도_보내지_않는다"},
	{"실패 문장 마스킹", "Finish 가 Summarize 대신 원문을 적는다", "빨강: result.json 에 planted 토큰이 보인다", "Test표준_출력_결과_원장_컨텍스트_stderr_에_심은_토큰과_private_값의_원문이_없고_실패_문장은_가린_채_남는다"},
	{"console 출력 마스킹", "consoleHandler 가 가리지 않고 쓴다", "빨강: stderr 에 priv 값이 보인다", "Test표준_출력_결과_원장_컨텍스트_stderr_에_심은_토큰과_private_값의_원문이_없고_실패_문장은_가린_채_남는다"},
	{"가린 뒤 자름", "Summarize 전에 500자로 자른다", "빨강: 잘린 조각에 시드 앞부분이 남는다", "Test실패_문장은_가린_뒤에_자른다_긴_문장의_끝에_걸친_값도_원문_조각을_남기지_않는다"},
	{"건너뜀은 앞 기록을 덮지 않음", "setLedger 의 skipped 보호를 지운다", "빨강: two 가 skipped 로 덮인다", "Test건너뜀은_앞서_돈_기록을_덮지_않는다_시작하지_않았다는_기록일_뿐이다"},
	{"전제 실패는 뒤 플로우를 막음", "Next 의 blocked 판정을 지운다", "빨강: beta 가 돌고 요청이 나간다", "Test건너뜀은_전제를_따라_이어진다_실패한_플로우의_손자도_시작하지_않는다"},
	{"원자 쓰기", "WriteJSON 이 직접 쓴다", "빨강: 중간 시점 디렉토리에 .tmp 가 보이거나 JSON 이 깨진다", "TestAE7_단계가_도는_중의_result_json_은_온전한_JSON_이고_complete_false_에_그_단계가_running_이며_원장은_started_다"},
	{"exec 는 전체 실행 파일을 건드리지 않음", "exec 플랜도 result.json 에 쓴다", "빨강: result.json 이 생긴다", "TestAE4_반복_가능_전제는_다시_돌고_once_전제는_원장_passed_와_컨텍스트_캡처로_만족한다_두_번_불러도_같다"},
	{"뒤 대상은 앞 대상 실패 뒤 skipped", "skipAll 을 항상 false 로 둔다", "빨강: three 가 돈다", "Test대상_셋은_앞에서_뒤로_돌고_둘째가_실패하면_셋째는_skipped_exec_파일이_셋_생긴다"},
	{"전부 통과만 0", "ExitCode 가 항상 0", "빨강: 실패 런이 0", "TestAE2_둘째_플로우의_둘째_단계가_실패하면_남은_단계와_그것을_전제하는_플로우가_skipped_종료_1"},
	{"계약 모양", "manifest 에 runner 를 빼거나 result 의 schemaVersion 을 1 로", "빨강: 단정과 스키마 검증이 깨진다", "TestAE1_전부_통과하면_매니페스트와_결과의_단계_집합이_같고_complete_이며_종료_0_원장_셋이_passed"},
	{"요청 수는 Go 포트가 센다", "Finish 가 requests 를 1 로 고정", "빨강: noop 단계의 requests 가 0 이 아니다", "Test단계가_요청을_보내지_않으면_requests_0_이고_httpStatus_가_없다"},
	{"정족 전제 캡처 없으면 실패", "ctx.get 이 빈 문자열을 돌려준다", "빨강: 종료 1 이 아니다", "TestOnce_전제의_캡처가_컨텍스트에_없으면_이어_받지_못해_그_단계가_실패한다"},
	{"once 전제만이면 대상만", "PlanExec 가 once 전제도 다시 돌린다", "빨강: alpha 요청이 나간다", "Test전제가_once_passed_뿐인_대상은_대상_하나만_돈다"},
	{"exec 기본 경로", "ExecOutPath 가 빈 base 에 result.json 을 돌려준다", "빨강: exec/ 에 파일이 없다", "TestE2E_EXEC_OUT_이_없으면_exec_시각_플로우_json_에_쓴다"},
	{"once 미시작은 허용", "PlanExec 가 기록 없는 once 도 거부", "빨강: 종료 2", "Test허용_시작한_적_없는_once_플로우는_exec_로_돌고_원장이_passed_가_된다"},
	{"인자 검사", "parseArgs 가 꼴을 보지 않는다", "빨강: 잘못된 id 가 통과한다", "Test인자가_틀리면_종료_2_와_사유다"},
	{"스위트 적재 실패는 2", "번들 오류를 0 으로 삼킨다", "빨강: 종료 0", "Test스위트_파일이_없거나_잘못되면_종료_2_이고_매니페스트가_없다"},
	{"이름 불일치 거부", "Begin 의 이름 검사를 지운다", "빨강: 이름이 다른 스위트가 돈다", "Test스위트_파일이_없거나_잘못되면_종료_2_이고_매니페스트가_없다"},
}

func Test선언의_hostApi_가_러너와_다르면_매니페스트를_쓰기_전에_종료_2_이고_사유에_두_값이_적힌다(t *testing.T) {
	w := newWorld(t, core, nil)
	if err := os.WriteFile(w.env["COCKPIT_DECLARATION"], []byte("hostApi: 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := w.run(nil)
	joined := strings.Join(got.stderr, "\n")
	if got.code != 2 || !strings.Contains(joined, "7") || !strings.Contains(joined, "1") || w.exists("manifest.json") || len(w.paths()) != 0 {
		t.Fatalf("exit %d stderr %q manifest=%v", got.code, joined, w.exists("manifest.json"))
	}
	if err := os.WriteFile(w.env["COCKPIT_DECLARATION"], []byte("hostApi: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := w.run(nil); got.code != 0 {
		t.Fatalf("같은 값: exit %d stderr %v", got.code, got.stderr)
	}
}

func Test선언이_없거나_읽히지_않으면_종료_2_다(t *testing.T) {
	w := newWorld(t, core, nil)
	os.Remove(w.env["COCKPIT_DECLARATION"])
	if got := w.run(nil); got.code != 2 || w.exists("manifest.json") {
		t.Fatalf("없음: exit %d stderr %v", got.code, got.stderr)
	}
	for _, body := range []string{"hostApi: one\n", "scripts: {}\n", ":::\n"} {
		if err := os.WriteFile(w.env["COCKPIT_DECLARATION"], []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := w.run(nil); got.code != 2 {
			t.Fatalf("%q: exit %d stderr %v", body, got.code, got.stderr)
		}
	}
}

func Test인자가_틀리면_종료_2_와_사유다(t *testing.T) {
	w := newWorld(t, core, nil)
	for _, args := range [][]string{{"beta"}, {"--flow"}, {"--what"}, {"--flow", "Bad Id"}} {
		got := w.run(nil, args...)
		if got.code != 2 || len(got.stderr) == 0 || w.exists("manifest.json") {
			t.Fatalf("%v: exit %d stderr %v", args, got.code, got.stderr)
		}
	}
	var out, errOut strings.Builder
	if code := Main(t.Context(), []string{"Bad/name"}, func(string) string { return "" }, Deps{Stdout: &out, Stderr: &errOut}); code != 2 || !strings.Contains(errOut.String(), "꼴") {
		t.Fatalf("exit %d stderr %q", code, errOut.String())
	}
}

func Test스위트_파일이_없거나_잘못되면_종료_2_이고_매니페스트가_없다(t *testing.T) {
	w := newWorld(t, core, nil)
	suite := filepath.Join(w.scripts, "suites", "fake.ts")
	for name, body := range map[string]string{
		"구문 오류":       "export default {\n",
		"꼴 아님":        "export default { name: \"fake\" };\n",
		"이름 불일치":      "import { suite } from \"cockpit:flow\";\nexport default suite({ name: \"other\", stages: [], flows: [] });\n",
		"루트 밖 import": "import { x } from \"../../outside.ts\";\nexport default x;\n",
	} {
		if err := os.WriteFile(suite, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		got := w.run(nil)
		if got.code != 2 || len(got.stderr) == 0 || w.exists("manifest.json") {
			t.Fatalf("%s: exit %d stderr %v", name, got.code, got.stderr)
		}
	}
	os.Remove(suite)
	if got := w.run(nil); got.code != 2 {
		t.Fatalf("없음: exit %d", got.code)
	}
}

func Test러너_변이_기록표는_시험_파일들과_양방향으로_같다(t *testing.T) {
	porttest.AssertMutationRecordAcross(t, []string{"main_test.go", "execute_test.go", "exec_test.go"}, runnerMutations)
}
