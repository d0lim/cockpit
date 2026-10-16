package source

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var sourceMutations = []porttest.Mutation{
	{"선언 밖 이름 거부", "Read 의 roots 조회 실패를 빈 루트로 대신한다", "빨강: 모르는 이름이 PORT_DENIED 가 아니다", "Test선언_안_파일은_읽히고_밖_이름과_탈출_경로와_링크는_거부된다"},
	{"경로 탈출 거부", "IsLocal 검사를 지운다", "빨강: ../ 경로가 루트 밖 파일을 읽는다", "Test선언_안_파일은_읽히고_밖_이름과_탈출_경로와_링크는_거부된다"},
	{"심볼릭 링크 거부", "Lstat 을 Stat 으로 바꾼다", "빨강: 링크가 가리키는 파일 내용이 돌아온다", "Test선언_안_파일은_읽히고_밖_이름과_탈출_경로와_링크는_거부된다"},
	{"크기 상한", "maxBytes 검사를 지운다", "빨강: 상한을 넘는 파일이 잘리지 않고 읽힌다", "Test크기_상한을_넘는_파일은_PORT_LIMIT_이고_상한_안은_그대로다"},
	{"나열은 일반 파일만", "ReadDir 결과를 거르지 않는다", "빨강: 하위 디렉토리와 링크 이름이 목록에 든다", "Test나열은_바로_아래_일반_파일_이름만_정렬해_돌려주고_없는_디렉토리는_빈_목록이다"},
}

func fixture(t *testing.T) (*Reader, string) {
	t.Helper()
	root := t.TempDir()
	outside := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "modules"), 0o755))
	must(os.WriteFile(filepath.Join(root, "modules", "b.json"), []byte(`{"b":1}`), 0o644))
	must(os.WriteFile(filepath.Join(root, "modules", "a.json"), []byte(`{"a":1}`), 0o644))
	must(os.WriteFile(filepath.Join(root, "modules", ".hidden.json"), []byte(`{}`), 0o644))
	must(os.MkdirAll(filepath.Join(root, "modules", "sub"), 0o755))
	must(os.WriteFile(filepath.Join(outside, "secret.json"), []byte(`PLANTED_OUTSIDE`), 0o644))
	must(os.Symlink(filepath.Join(outside, "secret.json"), filepath.Join(root, "modules", "link.json")))
	must(os.WriteFile(filepath.Join(root, "big.json"), []byte(strings.Repeat("x", 65)), 0o644))
	r, err := Open(Roots{"fixtures": root}, 64, ports.Identity)
	must(err)
	t.Cleanup(r.Close)
	return r, outside
}

func codeOf(err error) engine.PortCode {
	var pe *engine.PortError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func Test선언_안_파일은_읽히고_밖_이름과_탈출_경로와_링크는_거부된다(t *testing.T) {
	r, _ := fixture(t)
	got, err := r.Read("fixtures", "modules/a.json")
	if err != nil || got != `{"a":1}` {
		t.Fatalf("허용 읽기: %q %v", got, err)
	}
	denied := []struct{ name, rel string }{
		{"other", "modules/a.json"},
		{"fixtures", "../secret.json"},
		{"fixtures", "/etc/hosts"},
		{"fixtures", "modules/link.json"},
	}
	for _, d := range denied {
		got, err := r.Read(d.name, d.rel)
		if codeOf(err) != engine.PortDenied || got != "" || strings.Contains(err.Error(), "PLANTED_OUTSIDE") {
			t.Fatalf("%s %s: %q %v", d.name, d.rel, got, err)
		}
	}
	if len(denied) != 4 {
		t.Fatal("거부 사례 수")
	}
}

func Test크기_상한을_넘는_파일은_PORT_LIMIT_이고_상한_안은_그대로다(t *testing.T) {
	r, _ := fixture(t)
	if _, err := r.Read("fixtures", "big.json"); codeOf(err) != engine.PortLimit {
		t.Fatalf("상한 초과: %v", err)
	}
	got, err := r.Read("fixtures", "modules/b.json")
	if err != nil || got != `{"b":1}` {
		t.Fatalf("상한 안: %q %v", got, err)
	}
}

func Test나열은_바로_아래_일반_파일_이름만_정렬해_돌려주고_없는_디렉토리는_빈_목록이다(t *testing.T) {
	r, _ := fixture(t)
	names, err := r.List("fixtures", "modules")
	if err != nil || !slices.Equal(names, []string{".hidden.json", "a.json", "b.json"}) {
		t.Fatalf("나열: %q %v", names, err)
	}
	empty, err := r.List("fixtures", "vectors")
	if err != nil || len(empty) != 0 {
		t.Fatalf("없는 디렉토리: %q %v", empty, err)
	}
	if _, err := r.List("fixtures", "../"); codeOf(err) != engine.PortDenied {
		t.Fatalf("탈출 나열: %v", err)
	}
	if _, err := r.List("other", "modules"); codeOf(err) != engine.PortDenied {
		t.Fatalf("모르는 이름 나열: %v", err)
	}
}

func Test소스_변이_기록표는_이_파일의_시험과_양방향으로_같다(t *testing.T) {
	porttest.AssertMutationRecord(t, "source_test.go", sourceMutations)
}
