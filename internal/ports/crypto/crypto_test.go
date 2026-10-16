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
	{"RSASSA-PKCS1-v1_5 signature matches Bun", "SignPKCS1v15 signs with PSS", "red: the fixed vector differs from the Bun signature", "TestRS512SignatureOfFixedKeyAndInputMatchesBunRunner"},
	{"signing with a public or unknown key is rejected", "remove the key type check in Sign", "red: a public key reference produces a signature", "TestOnlyGeneratedPrivateKeySignsWhilePublicKeyAndUnknownRefAreRejected"},
	{"random byte length limit", "remove the limit in RandomBytes", "red: a 65537-byte request passes", "TestRandomUUIDIsV4AndRandomBytesAreFilledOnlyWithinTheLimit"},
	{"signing error messages are masked", "build the rejection message without mask", "red: the value planted in the key id shows in the message", "TestOnlyGeneratedPrivateKeySignsWhilePublicKeyAndUnknownRefAreRejected"},
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

func TestRS512SignatureOfFixedKeyAndInputMatchesBunRunner(t *testing.T) {
	sig, err := SignPKCS1v15(vectorKey(t), crypto.SHA512, []byte(vectorInput))
	if err != nil {
		t.Fatal(err)
	}
	if got := base64.StdEncoding.EncodeToString(sig); got != vectorBun {
		t.Fatalf("Go signature differs from Bun\n%s\n%s", got, vectorBun)
	}
}

func TestOnlyGeneratedPrivateKeySignsWhilePublicKeyAndUnknownRefAreRejected(t *testing.T) {
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
		t.Fatalf("verify: %v", err)
	}
	if _, err := keys.Sign(pair.PublicKey.ID, data); codeOf(err) != engine.PortDenied {
		t.Fatalf("public key sign: %v", err)
	}
	_, err = keys.Sign(planted, data)
	if codeOf(err) != engine.PortBadArgs || regexp.MustCompile(regexp.QuoteMeta(planted)).MatchString(err.Error()) {
		t.Fatalf("unknown reference: %v", err)
	}
	if _, err := keys.Generate(context.Background(), 1024, "SHA-256"); codeOf(err) != engine.PortBadArgs {
		t.Fatalf("short key: %v", err)
	}
	if _, err := keys.Generate(context.Background(), 2048, "MD5"); codeOf(err) != engine.PortBadArgs {
		t.Fatalf("unknown hash: %v", err)
	}
}

func TestRandomUUIDIsV4AndRandomBytesAreFilledOnlyWithinTheLimit(t *testing.T) {
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

func TestCryptoMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "crypto_test.go", cryptoMutations)
}
