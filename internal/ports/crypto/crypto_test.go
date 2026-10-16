package cryptoport

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"os"
	"regexp"
	"testing"

	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const (
	vectorInput = "eyJhbGciOiJSUzUxMiIsImtpZCI6ImtpZC1maXhlZCJ9.eyJpZCI6ImZpeGVkLXZlY3RvciIsImFtb3VudCI6IjUifQ"
	vectorBun   = "FCvsr6fQKkepp/+BZtFt+oxX1lh8wcVW4VJ/65hOLBquwU7pIY5yNesFG5f7dV05EPV4nmQyvgjfMV1q7DiSYTJi45rq0di1SCIK73FFQinA5sxJWBRRpkpadYht+2jgZauslEcTSa5AcFkuXGVb+uWak/bNIM8/bXhOlUsNZqlW3DNdp5mrCvedEYkWsoJDzChx4B+GpvucQe+Tv2jEHJkRPyKrKyV/PxJPSJdfvUsMjqsqt3PuNM4seUPutMn/1CBNemhJXc+y1AZs4tpoZCgEQe5X8mf0IGHmnEk7N5bqUyF29I+jNE8NIJiVH/yddgZ1iFesby3GEXzkIFW30w=="
)

var cryptoMutations = []porttest.Mutation{
	{"RSASSA-PKCS1-v1_5 서명이 Bun 과 같음", "SignPKCS1v15 가 PSS 로 서명한다", "빨강: 고정 벡터가 Bun 서명과 다르다", "Test고정_키와_입력의_RS512_서명이_Bun_러너의_결과와_같다"},
	{"공개키·모르는 키로 서명 거부", "Sign 의 키 종류 검사를 지운다", "빨강: 공개키 참조로 서명이 만들어진다", "Test생성한_키쌍의_개인키로만_서명되고_공개키와_모르는_참조는_거부된다"},
	{"난수 길이 상한", "RandomBytes 의 상한을 지운다", "빨강: 65537 바이트 요청이 통과한다", "Test난수는_v4_UUID_꼴이고_바이트_요청은_상한_안에서만_채워진다"},
	{"서명 오류 문장 마스킹", "거부 문장을 mask 없이 만든다", "빨강: 키 id 에 심은 값이 문장에 보인다", "Test생성한_키쌍의_개인키로만_서명되고_공개키와_모르는_참조는_거부된다"},
}

func vectorKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	raw, err := os.ReadFile("testdata/rsa-2048.pem")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(raw)
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return key.(*rsa.PrivateKey)
}

func codeOf(err error) engine.PortCode {
	var pe *engine.PortError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func Test고정_키와_입력의_RS512_서명이_Bun_러너의_결과와_같다(t *testing.T) {
	sig, err := SignPKCS1v15(vectorKey(t), crypto.SHA512, []byte(vectorInput))
	if err != nil {
		t.Fatal(err)
	}
	if got := base64.StdEncoding.EncodeToString(sig); got != vectorBun {
		t.Fatalf("Go 서명이 Bun 과 다르다\n%s\n%s", got, vectorBun)
	}
}

func Test생성한_키쌍의_개인키로만_서명되고_공개키와_모르는_참조는_거부된다(t *testing.T) {
	planted := "PLANTED-key-ref-7f3a9c"
	keys := NewKeys(func(s string) string {
		if s == "" {
			return s
		}
		return regexp.MustCompile(regexp.QuoteMeta(planted)).ReplaceAllString(s, "****")
	})
	pair, err := keys.Generate(context.Background(), 2048, "SHA-512")
	if err != nil || pair.PrivateKey.Type != "private" || pair.PublicKey.Type != "public" || pair.PrivateKey.ID == pair.PublicKey.ID {
		t.Fatalf("%+v %v", pair, err)
	}
	data := []byte("header.payload")
	sig, err := keys.Sign(pair.PrivateKey.ID, data)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := keys.PublicKey(pair.PublicKey.ID)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha512.Sum512(data)
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA512, digest[:], sig); err != nil {
		t.Fatalf("검증: %v", err)
	}
	if _, err := keys.Sign(pair.PublicKey.ID, data); codeOf(err) != engine.PortDenied {
		t.Fatalf("공개키 서명: %v", err)
	}
	_, err = keys.Sign(planted, data)
	if codeOf(err) != engine.PortBadArgs || regexp.MustCompile(regexp.QuoteMeta(planted)).MatchString(err.Error()) {
		t.Fatalf("모르는 참조: %v", err)
	}
	if _, err := keys.Generate(context.Background(), 1024, "SHA-256"); codeOf(err) != engine.PortBadArgs {
		t.Fatalf("짧은 키: %v", err)
	}
	if _, err := keys.Generate(context.Background(), 2048, "MD5"); codeOf(err) != engine.PortBadArgs {
		t.Fatalf("모르는 해시: %v", err)
	}
}

func Test난수는_v4_UUID_꼴이고_바이트_요청은_상한_안에서만_채워진다(t *testing.T) {
	v4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := map[string]bool{}
	for range 64 {
		id := RandomUUID()
		if !v4.MatchString(id) || seen[id] {
			t.Fatalf("%s", id)
		}
		seen[id] = true
	}
	full, err := RandomBytes(ports.Identity, 65536)
	if err != nil || len(full) != 65536 {
		t.Fatalf("%d %v", len(full), err)
	}
	if _, err := RandomBytes(ports.Identity, 65537); codeOf(err) != engine.PortLimit {
		t.Fatalf("%v", err)
	}
	if _, err := RandomBytes(ports.Identity, -1); codeOf(err) != engine.PortBadArgs {
		t.Fatalf("%v", err)
	}
}

func Test암호_변이_기록표는_이_파일의_시험과_양방향으로_같다(t *testing.T) {
	porttest.AssertMutationRecord(t, "crypto_test.go", cryptoMutations)
}
