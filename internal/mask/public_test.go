package mask

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const (
	publicToken   = "tok-1f2e3d4c5b6a7980"
	publicPattern = "tok-[0-9a-f]{8,64}"
	plantedScan   = "PLANTED-scan-value-41c7"
)

var publicMutations = []porttest.Mutation{
	{"a public value is tied to its key", "make publicValue ignore the key", "red: a token-shaped value under session shows", "TestADeclaredValueUnderItsOwnKeyStaysReadableAndAnyOtherValueReadsTheMarker"},
	{"the pattern covers the whole value", "compile the value without the anchors", "red: a secret around a token-shaped part shows", "TestADeclaredValueUnderItsOwnKeyStaysReadableAndAnyOtherValueReadsTheMarker"},
	{"a public value still passes Mask", "return the value from leaf without Mask", "red: a known value under the public key shows", "TestADeclaredValueUnderItsOwnKeyStaysReadableAndAnyOtherValueReadsTheMarker"},
	{"a value below a hiding key stays hidden", "let cover.under take the inner key while hidden", "red: a token-shaped value below password shows", "TestADeclaredValueUnderItsOwnKeyStaysReadableAndAnyOtherValueReadsTheMarker"},
	{"non-string values stay hidden", "return scalars from leaf when the key has a public field", "red: a number under the public key shows", "TestADeclaredValueUnderItsOwnKeyStaysReadableAndAnyOtherValueReadsTheMarker"},
	{"without public fields nothing is exempt", "make publicValue true when no field is declared", "red: a credential value shows without a declaration", "TestAMaskerWithoutPublicFieldsHidesEveryValueUnderACredentialKey"},
	{"only the token, session and credential family qualifies", "drop the publicKeyFamily check", "red: the key email passes", "TestCheckPublicFieldAdmitsNarrowPatternsOnTokenFamilyKeysAndRefusesTheRest"},
	{"a key naming a stronger secret is refused", "drop the neverPublicKey check", "red: client_secret_token passes", "TestCheckPublicFieldAdmitsNarrowPatternsOnTokenFamilyKeysAndRefusesTheRest"},
	{"a pattern matching a real credential shape is refused", "drop the probe loop", "red: broad patterns pass", "TestCheckPublicFieldAdmitsNarrowPatternsOnTokenFamilyKeysAndRefusesTheRest"},
	{"a pattern cannot escape its anchors", "compile only the wrapped pattern", "red: a)|(?:b passes as an unanchored pattern", "TestCheckPublicFieldAdmitsNarrowPatternsOnTokenFamilyKeysAndRefusesTheRest"},
	{"a refused field builds no masker", "skip the field that fails to compile", "red: WithPublicFields answers without an error", "TestWithPublicFieldsRefusesABadFieldAndLeavesTheReceiverAsItWas"},
	{"the value scan drops the key-name rules", "return the masker unchanged from ValueScan", "red: a public field in an encoded line is masked", "TestValueScanLeavesKeyNamedValuesAloneAndStillMasksEveryValueRule"},
	{"the value scan keeps the value rules", "empty rules in ValueScan", "red: Bearer and JWT survive the scan", "TestValueScanLeavesKeyNamedValuesAloneAndStillMasksEveryValueRule"},
}

func TestPublicMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "public_test.go", publicMutations)
}

func publicMasker(t *testing.T, known ...string) *Masker {
	t.Helper()
	m, err := New(Options{Mode: Hub, Known: known})
	if err != nil {
		t.Fatal(err)
	}
	m, err = m.WithPublicFields([]PublicField{{Key: "token", Value: publicPattern}})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestADeclaredValueUnderItsOwnKeyStaysReadableAndAnyOtherValueReadsTheMarker(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want any
	}{
		{"the declared key", map[string]any{"token": publicToken}, map[string]any{"token": publicToken}},
		{"the key in other case", map[string]any{"TOKEN": publicToken}, map[string]any{"TOKEN": publicToken}},
		{"an array under the key", map[string]any{"token": []any{publicToken}}, map[string]any{"token": []any{publicToken}}},
		{"form values under the key", map[string][]string{"token": {publicToken}}, map[string][]string{"token": {publicToken}}},
		{"a value around a public part", map[string]any{"token": "PLANTED-" + publicToken}, map[string]any{"token": marker}},
		{"a public part before a secret", map[string]any{"token": publicToken + "\nPLANTED"}, map[string]any{"token": marker}},
		{"a value of another shape", map[string]any{"token": "PLANTED-opaque"}, map[string]any{"token": marker}},
		{"another credential key", map[string]any{"session": publicToken}, map[string]any{"session": marker}},
		{"a key that only ends in the name", map[string]any{"refresh_token": publicToken}, map[string]any{"refresh_token": marker}},
		{"a dotted key", map[string]any{"auth.token": publicToken}, map[string]any{"auth.token": marker}},
		{"below the key in an object", map[string]any{"token": map[string]any{"value": publicToken}}, map[string]any{"token": map[string]any{"value": marker}}},
		{"below a password", map[string]any{"password": map[string]any{"token": publicToken}}, map[string]any{"password": map[string]any{"token": marker}}},
		{"a number under the key", map[string]any{"token": json.Number("12345678")}, map[string]any{"token": marker}},
		{"a boolean under the key", map[string]any{"token": true}, map[string]any{"token": marker}},
	}
	m := publicMasker(t)
	for _, c := range cases {
		if got := m.Leaves(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %#v, want %#v", c.name, got, c.want)
		}
	}
	known := publicMasker(t, publicToken)
	got := known.Leaves(map[string]any{"token": publicToken})
	if want := (map[string]any{"token": marker + "7980"}); !reflect.DeepEqual(got, want) {
		t.Errorf("a known value under the public key: got %#v, want %#v", got, want)
	}
	if len(cases) != 14 {
		t.Fatalf("evaluated %d cases", len(cases))
	}
}

func TestAMaskerWithoutPublicFieldsHidesEveryValueUnderACredentialKey(t *testing.T) {
	plain, err := New(Options{Mode: Hub})
	if err != nil {
		t.Fatal(err)
	}
	emptied, err := plain.WithPublicFields(nil)
	if err != nil {
		t.Fatal(err)
	}
	in := map[string]any{"token": publicToken, "note": "readable"}
	want := map[string]any{"token": marker, "note": "readable"}
	for name, m := range map[string]*Masker{"plain": plain, "empty list": emptied} {
		if got := m.Leaves(in); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %#v, want %#v", name, got, want)
		}
	}
	if got := publicMasker(t).Leaves(in); !reflect.DeepEqual(got, in) {
		t.Fatalf("the masker with the field read %#v, so the control proves nothing", got)
	}
}

func TestCheckPublicFieldAdmitsNarrowPatternsOnTokenFamilyKeysAndRefusesTheRest(t *testing.T) {
	allowed := []PublicField{
		{"token", publicPattern},
		{"Token", publicPattern},
		{"page_token", "pg_[0-9]{4}"},
		{"session", "sess-[a-z]{6}"},
		{"credential", "cred/[0-9]+"},
	}
	for _, f := range allowed {
		if err := CheckPublicField(f); err != nil {
			t.Errorf("%+v was refused: %v", f, err)
		}
	}
	refusedKeys := []string{"", "email", "name", "password", "passwd", "secret", "client_secret", "client_secret_token", "password_token", "api_key", "apikey_token", "api-key-token", "authorization", "authorization_token", "cookie", "cookie_token", "a.token"}
	for _, key := range refusedKeys {
		err := CheckPublicField(PublicField{Key: key, Value: publicPattern})
		if err == nil || !strings.Contains(err.Error(), "family") {
			t.Errorf("key %q: %v", key, err)
		}
	}
	broad := map[string]string{
		"the empty string":        "x*",
		"16 lowercase letters":    "[a-z]+",
		"16 capitals":             "[A-Z]{16}",
		"16 digits":               "[0-9]+",
		"32 hex characters":       "[0-9a-f]{32}",
		"43 base64url characters": "[A-Za-z0-9_-]{43}",
		"a UUID":                  "[0-9a-f-]{36}",
		"a JWT":                   `[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`,
		"a mixed password":        `[A-Za-z0-9]+&[0-9]-[A-Za-z]+![0-9]`,
	}
	if len(broad) != len(publicProbes) {
		t.Fatalf("%d broad patterns for %d probes", len(broad), len(publicProbes))
	}
	for _, p := range publicProbes {
		pattern, ok := broad[p.name]
		if !ok {
			t.Errorf("probe %q has no broad pattern", p.name)
			continue
		}
		err := CheckPublicField(PublicField{Key: "token", Value: pattern})
		if err == nil || !strings.Contains(err.Error(), p.name) {
			t.Errorf("pattern %q for %s: %v", pattern, p.name, err)
		}
	}
	for _, value := range []string{".*", "(", "a)|(?:b"} {
		if err := CheckPublicField(PublicField{Key: "token", Value: value}); err == nil {
			t.Errorf("pattern %q passed", value)
		}
	}
}

func TestWithPublicFieldsRefusesABadFieldAndLeavesTheReceiverAsItWas(t *testing.T) {
	m := publicMasker(t)
	next, err := m.WithPublicFields([]PublicField{{Key: "session", Value: "sess-[a-z]{6}"}, {Key: "password", Value: publicPattern}})
	if err == nil || next != nil {
		t.Fatalf("a refused field gave %v, %v", next, err)
	}
	got := m.Leaves(map[string]any{"token": publicToken, "session": "sess-abcdef"})
	want := map[string]any{"token": publicToken, "session": marker}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestValueScanLeavesKeyNamedValuesAloneAndStillMasksEveryValueRule(t *testing.T) {
	base, err := New(Options{Mode: Hub, Known: []string{plantedScan}, Patterns: []string{"decl_[0-9]{6}"}})
	if err != nil {
		t.Fatal(err)
	}
	scan := base.ValueScan()
	keyNamed := []string{
		`{"password":"hunter2-value","token":"` + publicToken + `","session":"sess-1"}`,
		`{"password":["hunter2-value"]}`,
		"authorization: basic-value",
		"token=" + publicToken,
		"session=sess-1",
	}
	for _, line := range keyNamed {
		if base.Mask(line) == line {
			t.Errorf("the base masker left %s alone, so its key rule was not active", line)
		}
		if got := scan.Mask(line); got != line {
			t.Errorf("the value scan changed a key-named value: %s", got)
		}
	}
	cases := []struct{ name, text, gone, left string }{
		{"PEM block", "-----BEGIN KEY-----\nPLANTEDPEM\n-----END KEY-----", "PLANTEDPEM", pemMarker},
		{"known value", "value " + plantedScan, plantedScan, marker},
		{"bearer value", "Bearer PLANTEDbearer", "PLANTEDbearer", "Bearer " + marker},
		{"JWT", "eyJhbGciOiJub25lIn0.eyJzdWIiOiJQIn0.PLANTEDjwt", "PLANTEDjwt", marker},
		{"declared pattern", "id decl_123456", "decl_123456", marker},
	}
	for _, c := range cases {
		got := scan.Mask(c.text)
		if strings.Contains(got, c.gone) || !strings.Contains(got, c.left) {
			t.Errorf("%s: %q", c.name, got)
		}
	}
	if len(cases) != 5 {
		t.Fatalf("evaluated %d cases", len(cases))
	}
}
