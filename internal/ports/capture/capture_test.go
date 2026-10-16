package capture

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const session = "PLANTED-session-7f3a9c"

var captureMutations = []porttest.Mutation{
	{"캡처는 기본이 비밀", "Set 이 persist 와 무관하게 persistNames 에 넣는다", "빨강: persist 없는 캡처가 context.json 에 쓰인다", "TestPersist_없는_캡처는_context_json_에_쓰이지_않고_persist_true_인_캡처만_쓰인다"},
	{"비밀로 다시 적으면 남기는 목록에서 빠짐", "Set 이 persist=false 일 때 persistNames 를 지우지 않는다", "빨강: draftId 가 persisted 에 남는다", "Test이어_받은_캡처는_다시_남고_비밀로_다시_적으면_남기는_목록에서_빠진다"},
	{"없는 캡처는 이름과 함께 거부", "Get 이 빈 문자열을 돌려준다", "빨강: 오류가 없거나 이름이 빠진다", "Test없는_캡처를_읽으면_PORT_BAD_ARGS_와_이름으로_거부한다"},
	{"원자 쓰기", "Save 가 임시 파일 없이 직접 쓴다", "빨강: 쓰기 중 디렉토리에 .tmp 가 아닌 반쯤 쓴 파일이 보인다", "Test저장은_schemaVersion_2_의_context_json_을_원자적으로_쓰고_다시_읽는다"},
	{"schemaVersion 검사", "Load 가 버전을 보지 않는다", "빨강: 버전 1 파일이 이어 받아진다", "Test모르는_schemaVersion_의_context_json_은_이어_받지_않는다"},
}

func TestPersist_없는_캡처는_context_json_에_쓰이지_않고_persist_true_인_캡처만_쓰인다(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	c.Captures.Set("adminSession", session, false)
	c.Captures.Set("workspaceId", "ws-0001", true)
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "context.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, session) || strings.Contains(text, "adminSession") || !strings.Contains(text, `"workspaceId": "ws-0001"`) {
		t.Fatalf("context.json: %s", text)
	}
	if got, err := c.Captures.Get("adminSession"); err != nil || got != session {
		t.Fatalf("메모리 캡처: %q %v", got, err)
	}
	if !slices.Equal(c.Captures.Values(), []string{session, "ws-0001"}) && !slices.Equal(c.Captures.Values(), []string{"ws-0001", session}) {
		t.Fatalf("값 원천: %q", c.Captures.Values())
	}
}

func Test이어_받은_캡처는_다시_남고_비밀로_다시_적으면_남기는_목록에서_빠진다(t *testing.T) {
	s := NewStore(map[string]string{"workspaceId": "ws-0001", "draftId": "d-1"})
	if got, err := s.Get("workspaceId"); err != nil || got != "ws-0001" {
		t.Fatalf("%q %v", got, err)
	}
	s.Set("draftId", "d-2", false)
	persisted := s.Persisted()
	if len(persisted) != 1 || persisted["workspaceId"] != "ws-0001" {
		t.Fatalf("persisted: %v", persisted)
	}
}

func Test없는_캡처를_읽으면_PORT_BAD_ARGS_와_이름으로_거부한다(t *testing.T) {
	_, err := NewStore(nil).Get("workspaceId")
	var pe *engine.PortError
	if !errors.As(err, &pe) || pe.Code != engine.PortBadArgs || !strings.Contains(pe.Message, "workspaceId") {
		t.Fatalf("%v", err)
	}
}

func Test저장은_schemaVersion_2_의_context_json_을_원자적으로_쓰고_다시_읽는다(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	c.Captures.Set("workspaceId", "ws-0001", true)
	if err := c.CommitStep("admin-1", 1234); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "context.json" {
			t.Fatalf("남은 임시 파일: %s", e.Name())
		}
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "context.json"))
	parsed, err := contract.ReadRunnerFile[contract.Context](strings.NewReader(string(raw)))
	if err != nil || parsed.SchemaVersion != contract.RunnerSchemaVersion || parsed.TOTPSteps["admin-1"] != 1234 || parsed.Captures["workspaceId"] != "ws-0001" {
		t.Fatalf("%+v %v", parsed, err)
	}
	again, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if last, ok := again.LastStep("admin-1"); !ok || last != 1234 {
		t.Fatalf("이어 받은 스텝: %d %v", last, ok)
	}
	if got, err := again.Captures.Get("workspaceId"); err != nil || got != "ws-0001" {
		t.Fatalf("이어 받은 캡처: %q %v", got, err)
	}
	if _, ok := again.LastStep("admin-2"); ok {
		t.Fatal("없는 계정의 스텝")
	}
}

func Test모르는_schemaVersion_의_context_json_은_이어_받지_않는다(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "context.json"), []byte(`{"schemaVersion":1,"captures":{},"totpSteps":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !errors.Is(err, contract.ErrSchemaVersion) {
		t.Fatalf("%v", err)
	}
}

func Test캡처_변이_기록표는_이_파일의_시험과_양방향으로_같다(t *testing.T) {
	porttest.AssertMutationRecord(t, "capture_test.go", captureMutations)
}
