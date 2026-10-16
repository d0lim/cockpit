package totp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/clock"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const rfcSeed = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
const plantedSeed = "PLANTEDSEEDAAAAABBBBBCCCCCDDDDD2"

var totpMutations = []porttest.Mutation{
	{"RFC 6238 코드", "HMAC 의 동적 절단 오프셋을 0 으로 고정한다", "빨강: 59초 벡터가 287082 가 아니다", "TestRFC_6238_벡터와_base32_디코드가_맞는다"},
	{"같은 창의 둘째 발급은 다음 창을 기다림", "ChooseStep 이 last 를 무시한다", "빨강: 둘째 발급이 기다리지 않고 같은 스텝을 낸다", "Test같은_계정의_같은_30초_창에서_둘째_발급은_다음_창을_기다리고_스텝을_먼저_적는다"},
	{"스텝을 코드보다 먼저 적음", "Issue 가 코드를 만든 뒤 CommitStep 을 부른다", "빨강: 적기가 실패해도 코드가 나간다", "Test같은_계정의_같은_30초_창에서_둘째_발급은_다음_창을_기다리고_스텝을_먼저_적는다"},
	{"시드는 오류 문장에 없음", "모르는 계정 오류에 시드 값을 싣는다", "빨강: 오류 문장에 PLANTED 시드가 보인다", "Test모르는_계정과_빈_시드는_거부되고_오류_문장에_시드가_없다"},
	{"제자리 저장", "SeedFile.Save 가 rename 으로 쓴다", "빨강: inode 가 바뀌고 mode 가 0600 이 아니다", "Test시드_저장은_private_env_를_제자리에서_다시_쓰고_다른_절을_보존한다"},
}

type fakeSteps struct {
	last    map[string]int64
	commits []int64
	fail    bool
}

func (f *fakeSteps) LastStep(id string) (int64, bool) { v, ok := f.last[id]; return v, ok }
func (f *fakeSteps) CommitStep(id string, step int64) error {
	if f.fail {
		return errors.New("commit failed")
	}
	f.last[id] = step
	f.commits = append(f.commits, step)
	return nil
}

func issuer(now *int64, steps *fakeSteps, slept *[]int64) Issuer {
	return Issuer{
		Seeds: func(id string) (string, bool) {
			if id == "admin-1" {
				return rfcSeed, true
			}
			if id == "empty" {
				return "", true
			}
			return "", false
		},
		Clock: clock.Clock{Now: func() time.Time { return time.UnixMilli(*now) }},
		Sleep: func(_ context.Context, ms int64) error { *slept = append(*slept, ms); *now += ms; return nil },
		Steps: steps,
		Mask:  ports.Identity,
	}
}

func TestRFC_6238_벡터와_base32_디코드가_맞는다(t *testing.T) {
	raw, err := DecodeBase32("MZXW6===")
	if err != nil || string(raw) != "foo" {
		t.Fatalf("%q %v", raw, err)
	}
	if _, err := DecodeBase32("MZXW6=1="); err == nil {
		t.Fatal("알파벳 밖 문자")
	}
	code, err := At(rfcSeed, 1)
	if err != nil || code != "287082" {
		t.Fatalf("%q %v", code, err)
	}
	code, err = At(rfcSeed, 1111111109/30)
	if err != nil || code != "081804" {
		t.Fatalf("%q %v", code, err)
	}
}

func Test같은_계정의_같은_30초_창에서_둘째_발급은_다음_창을_기다리고_스텝을_먼저_적는다(t *testing.T) {
	now := int64(90_000 + 100)
	steps := &fakeSteps{last: map[string]int64{}}
	var slept []int64
	iss := issuer(&now, steps, &slept)
	first, err := iss.Issue(context.Background(), "admin-1")
	if err != nil || first == "" || len(slept) != 0 || steps.commits[0] != 3 {
		t.Fatalf("첫 발급: %q %v slept=%v commits=%v", first, err, slept, steps.commits)
	}
	second, err := iss.Issue(context.Background(), "admin-1")
	if err != nil || second == "" || len(slept) != 0 || steps.commits[1] != 4 {
		t.Fatalf("둘째 발급(다음 창은 검증 창 안이라 기다리지 않는다): %q %v slept=%v commits=%v", second, err, slept, steps.commits)
	}
	third, err := iss.Issue(context.Background(), "admin-1")
	if err != nil || third == "" || len(slept) != 1 || slept[0] != 4*30_000-(90_000+100)+300 || steps.commits[2] != 5 {
		t.Fatalf("셋째 발급은 창 밖이라 기다린다: %q %v slept=%v commits=%v", third, err, slept, steps.commits)
	}
	steps.fail = true
	if code, err := iss.Issue(context.Background(), "admin-1"); err == nil || code != "" {
		t.Fatalf("적기 실패면 코드가 나가지 않는다: %q %v", code, err)
	}
}

func Test모르는_계정과_빈_시드는_거부되고_오류_문장에_시드가_없다(t *testing.T) {
	now := int64(0)
	steps := &fakeSteps{last: map[string]int64{}}
	var slept []int64
	iss := issuer(&now, steps, &slept)
	iss.Seeds = func(id string) (string, bool) {
		if id == "planted" {
			return plantedSeed, true
		}
		return "", id == "empty"
	}
	var pe *engine.PortError
	_, err := iss.Issue(context.Background(), "nobody")
	if !errors.As(err, &pe) || pe.Code != engine.PortBadArgs || !strings.Contains(pe.Message, "nobody") {
		t.Fatalf("%v", err)
	}
	_, err = iss.Issue(context.Background(), "empty")
	if !errors.As(err, &pe) || pe.Code != engine.PortUnavailable {
		t.Fatalf("%v", err)
	}
	steps.fail = true
	_, err = iss.Issue(context.Background(), "planted")
	if err == nil || strings.Contains(err.Error(), plantedSeed) || strings.Contains(err.Error(), plantedSeed[:8]) {
		t.Fatalf("%v", err)
	}
}

func Test시드_저장은_private_env_를_제자리에서_다시_쓰고_다른_절을_보존한다(t *testing.T) {
	path := filepath.Join(t.TempDir(), "e2e.private.env.json")
	if err := os.WriteFile(path, []byte(`{"local":{"adminPassword":"PLANTED-pw-123456"},"meta":{"generatedAt":"t0"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)
	local, err := SeedFile{Path: path}.Save("totpSecret1", plantedSeed)
	if err != nil || local["totpSecret1"] != plantedSeed || local["adminPassword"] != "PLANTED-pw-123456" {
		t.Fatalf("%v %v", local, err)
	}
	after, _ := os.Stat(path)
	if after.Mode().Perm() != 0o600 || before.Sys().(*syscall.Stat_t).Ino != after.Sys().(*syscall.Stat_t).Ino {
		t.Fatalf("mode %v inode %v→%v", after.Mode(), before.Sys().(*syscall.Stat_t).Ino, after.Sys().(*syscall.Stat_t).Ino)
	}
	var doc map[string]any
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &doc); err != nil || doc["meta"].(map[string]any)["generatedAt"] != "t0" {
		t.Fatalf("%s %v", raw, err)
	}
	if _, err := (SeedFile{Path: filepath.Join(t.TempDir(), "missing.json")}).Save("k", "v"); err == nil {
		t.Fatal("없는 파일은 만들지 않는다")
	}
	got, err := ReadPrivateEnv(path)
	if err != nil || got["totpSecret1"] != plantedSeed {
		t.Fatalf("%v %v", got, err)
	}
}

func Test2팩터_변이_기록표는_이_파일의_시험과_양방향으로_같다(t *testing.T) {
	porttest.AssertMutationRecord(t, "totp_test.go", totpMutations)
}
