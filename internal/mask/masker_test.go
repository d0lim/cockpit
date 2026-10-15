package mask

import (
	"fmt"
	"strings"
	"testing"
)

const planted = "planted-cred-7f3a9c"

type scenario struct {
	name string
	run  func(engine) []string
}

var scenarios = []scenario{
	{"가린 뒤 자르면 경계에 걸친 심은 값의 조각이 남지 않고 자른 뒤 가리면 조각이 남는다", summarizeOrderScenario},
	{"허브 모드는 8자 미만 값을 등록하지 않고 러너 모드는 6자부터 끝자리 없이 가린다", modeBoundsScenario},
	{"선언 패턴이 RE2 로 읽히지 않으면 마스커를 만들지 않고 몇 번째 패턴인지 알린다", invalidPatternScenario},
	{"값 목록을 바꾼 마스커는 모드와 선언 패턴을 잇고 원래 마스커의 값 목록은 그대로다", withKnownScenario},
	{"선언 원천에 적지 않은 env 생성 산출물의 값도 가리고 공개 이름의 값은 가리지 않는다", outputsAndPublicScenario},
	{"빈 값과 ${VAR} 리터럴은 등록하지 않고 원천과 이름을 누락으로 보고한다", missingScenario},
	{"값 원천을 읽거나 해석하지 못하면 반쯤 만든 목록 없이 오류만 돌려준다", refreshFailureScenario},
	{"재스캔은 읽지 못한 파일과 값이 남은 파일을 남은 것으로 세고 살핀 건수를 낸다", rescanUnreadableScenario},
	{"재스캔은 심볼릭 링크를 따르지 않는다 — 링크는 남은 것이고 링크된 디렉토리 안은 보지 않는다", rescanLinkScenario},
	{"재스캔할 디렉토리를 열지 못하면 오류를 돌려준다", rescanMissingDirScenario},
	{"구조화 신호의 문자열 잎에 심은 값은 가려지고 구조와 숫자는 남는다", leavesPlantedScenario},
	{"자격 이름 키의 배열 값은 원소를 전부 가리고 다른 키의 배열은 둔다", leavesArrayScenario},
	{"자격 이름 키의 숫자와 참거짓 값도 가리고 다른 키의 숫자는 둔다", leavesScalarScenario},
	{"모르는 타입의 값은 JSON 으로 풀어 잎과 키 규칙을 적용한다", leavesUnknownTypeScenario},
}

func Test마스커_시나리오는_실제_엔진에서_전부_통과한다(t *testing.T) {
	ran := 0
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			ran++
			for _, f := range s.run(realEngine) {
				t.Error(f)
			}
		})
	}
	if ran == 0 || ran != len(scenarios) {
		t.Fatalf("시나리오 실행 %d건, 등록 %d건", ran, len(scenarios))
	}
}

func expectEqual(label, got, want string) []string {
	if got != want {
		return []string{fmt.Sprintf("%s: 결과 %q, 기대 %q", label, got, want)}
	}
	return nil
}

func truncate(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

func summarizeOrderScenario(e engine) []string {
	var f []string
	text := strings.Repeat("가", 10) + planted + " 뒤 문장"
	fragment := planted[:5]
	for _, mode := range []Mode{Hub, Runner} {
		m, err := e.build(Options{Mode: mode, Known: []string{planted}})
		if err != nil {
			return []string{err.Error()}
		}
		got := e.summarize(m, text, 15)
		if strings.Contains(got, fragment) || !strings.Contains(got, marker) {
			f = append(f, fmt.Sprintf("모드 %d: 요약 %q 에 원문 조각이 남았거나 표식이 없다", mode, got))
		}
		if cut := m.Mask(truncate(text, 15)); !strings.Contains(cut, fragment) {
			f = append(f, fmt.Sprintf("모드 %d: 자른 뒤 가린 대조군 %q 에 조각이 없다", mode, cut))
		}
	}
	return f
}

func modeBoundsScenario(e engine) []string {
	hub, err := e.build(Options{Mode: Hub, Known: []string{"abc1234", "abcd1234"}})
	if err != nil {
		return []string{err.Error()}
	}
	runner, err := e.build(Options{Mode: Runner, Known: []string{"ab123", "ab1234"}})
	if err != nil {
		return []string{err.Error()}
	}
	return append(append(append(
		expectEqual("허브 7자", e.mask(hub, "x abc1234 y"), "x abc1234 y"),
		expectEqual("허브 8자", e.mask(hub, "x abcd1234 y"), "x ****1234 y")...),
		expectEqual("러너 5자", e.mask(runner, "x ab123 y"), "x ab123 y")...),
		expectEqual("러너 6자", e.mask(runner, "x ab1234 y"), "x **** y")...)
}

func invalidPatternScenario(e engine) []string {
	_, err := e.build(Options{Mode: Hub, Patterns: []string{`ok_[a-z]{8,}`, `(?<![A-Za-z])sk_`}})
	if err == nil || !strings.Contains(err.Error(), "선언 패턴 1 ") {
		return []string{fmt.Sprintf("오류 %v 가 둘째 패턴을 짚지 않는다", err)}
	}
	return nil
}

func withKnownScenario(e engine) []string {
	base, err := e.build(Options{Mode: Hub, Patterns: []string{`(?:^|[^A-Za-z])pk_(?P<secret>[A-Za-z0-9]{8,})`}})
	if err != nil {
		return []string{err.Error()}
	}
	next := e.withKnown(base, []string{planted})
	line := "v " + planted + " k pk_abcdefgh12"
	return append(
		expectEqual("바꾼 마스커", e.mask(next, line), "v ****3a9c k pk_****gh12"),
		expectEqual("원래 마스커", e.mask(base, line), "v "+planted+" k pk_****gh12")...)
}
