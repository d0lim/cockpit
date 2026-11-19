package wire

import (
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const plantedCredential = "PLANTEDVALUE"

var credentialMutations = []porttest.Mutation{
	{"every word of the table hides its key", "remove pwd from credentialWords", "red: pwd and pwd_hint keep their values", "TestEveryKeyThatNamesACredentialHidesItsValueOnEveryStructuredSurface"},
	{"every dotted segment is read", "read only the last segment in namesCredential", "red: password.0 keeps its value", "TestABracketedNameWithAnIndexOrSubkeyHidesItsValueInFormsAndQueries"},
	{"a key rule name inside a longer key hides", "drop the mask.SecretKeyName check from namesCredential", "red: token.0 keeps its value", "TestABracketedNameWithAnIndexOrSubkeyHidesItsValueInFormsAndQueries"},
	{"a form hides before Leaves", "pass the form straight to Leaves in cleanBody", "red: new_password keeps its value in the form", "TestEveryKeyThatNamesACredentialHidesItsValueOnEveryStructuredSurface"},
	{"JSON hides before Leaves", "pass the value straight to Leaves in cleanValue", "red: new_password keeps its value in the JSON body", "TestEveryKeyThatNamesACredentialHidesItsValueOnEveryStructuredSurface"},
	{"otp is read as the end of a segment", "match otp anywhere in the segment", "red: otp_count reads the marker", "TestKeysThatOnlyResembleACredentialKeepTheirValues"},
	{"otp is read as a word", "match otp at the end of any segment", "red: spotp reads the marker", "TestKeysThatOnlyResembleACredentialKeepTheirValues"},
	{"a value of any type hides whole", "recurse into the value instead of replacing it", "red: the array keeps its shape and the object its keys", "TestAValueOfAnyTypeUnderACredentialKeyHidesWhole"},
	{"a single token, session or credential key stays with Leaves", "drop the mask.PublicFieldKey check from namesCredential", "red: the declared public value reads the marker", "TestDeclaredPublicFieldsStayVisibleUnderTheKeysLeftToLeaves"},
}

func TestCredentialMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "credential_test.go", credentialMutations)
}

var hiddenKeys = []string{
	"password", "new_password", "passwordHash", "password_hint", "has_password",
	"passwd", "user_passwd",
	"passphrase",
	"pwd", "pwd_hint", "login-pwd",
	"secret", "client_secret", "clientSecret", "secretary",
	"credentials", "aws_credential",
	"api_key", "x_api_key", "api-key", "x-api-key", "apikey", "apiKey",
	"private_key", "private-key", "privatekey", "privateKey",
	"authorization", "proxy_authorization", "authorisation",
	"cookie", "set_cookie", "cookie_consent",
	"bearer", "bearer_value",
	"session_id", "session-id", "sessionId", "user_session_id",
	"otp", "totp", "sms_otp", "login-totp", "smsOtp", "userTotp", "smsOTP", "USER_OTP",
}

func structuredSurfaces(key, value string) map[string]func(*Exchange) {
	form := url.Values{key: {value}}.Encode()
	object := encodedObject(key, value)
	return map[string]func(*Exchange){
		"JSON":       func(x *Exchange) { x.Request = jsonRequest(object) },
		"form":       func(x *Exchange) { x.Request = typed("application/x-www-form-urlencoded", form) },
		"query":      func(x *Exchange) { x.RawQuery = form },
		"JWT claims": func(x *Exchange) { x.Request = typed("application/jwt", compactJWT(`{"alg":"none"}`, object, "sig")) },
	}
}

func encodedObject(key, value string) string {
	raw, err := json.Marshal(map[string]string{key: value})
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func TestEveryKeyThatNamesACredentialHidesItsValueOnEveryStructuredSurface(t *testing.T) {
	m := newMasker(t)
	executed := 0
	for _, key := range hiddenKeys {
		for name, edit := range structuredSurfaces(key, plantedCredential) {
			x := baseExchange()
			edit(&x)
			got, ok := Prove(m, Capture(m, x))
			if !ok {
				t.Errorf("%s in %s: not proven: %+v", key, name, got)
			}
			assertMasked(t, key+" in "+name, lineOf(t, got), plantedCredential)
			executed++
		}
	}
	if executed != len(hiddenKeys)*4 || executed == 0 {
		t.Fatalf("executed %d cases", executed)
	}
	for _, word := range credentialWords {
		if !strings.Contains(strings.ToLower(strings.Join(hiddenKeys, " ")), word) {
			t.Errorf("no key in the test exercises %q", word)
		}
	}
}

func TestABracketedNameWithAnIndexOrSubkeyHidesItsValueInFormsAndQueries(t *testing.T) {
	m := newMasker(t)
	names := []string{"password[0]", "token[0]", "user[password][value]", "session[id]", "token[a][b]", "[password]", "api_key[]", "user[token][0]"}
	executed := 0
	for _, name := range names {
		raw := url.Values{name: {plantedCredential}}.Encode()
		for label, edit := range map[string]func(*Exchange){
			"form":  func(x *Exchange) { x.Request = typed("application/x-www-form-urlencoded", raw) },
			"query": func(x *Exchange) { x.RawQuery = raw },
		} {
			x := baseExchange()
			edit(&x)
			got, ok := Prove(m, Capture(m, x))
			if !ok {
				t.Errorf("%s in %s: not proven", name, label)
			}
			assertMasked(t, name+" in "+label, lineOf(t, got), plantedCredential)
			executed++
		}
	}
	if executed != 16 {
		t.Fatalf("executed %d cases", executed)
	}
	x := baseExchange()
	x.Request = jsonRequest(`{"token.0":"` + plantedCredential + `","user.password.value":"` + plantedCredential + `"}`)
	assertMasked(t, "dotted JSON keys", lineOf(t, Capture(m, x)), plantedCredential)
}

func TestKeysThatOnlyResembleACredentialKeepTheirValues(t *testing.T) {
	m := newMasker(t)
	visible := []string{"token_id", "tokens_total", "sessions", "laptop", "desktop", "otp_count", "spotp", "top", "note", "pass", "keyboard"}
	executed := 0
	for _, key := range visible {
		for name, edit := range structuredSurfaces(key, "plain-value") {
			x := baseExchange()
			edit(&x)
			got, ok := Prove(m, Capture(m, x))
			if !ok || !strings.Contains(lineOf(t, got), "plain-value") {
				t.Errorf("%s in %s lost its value: %v %s", key, name, ok, lineOf(t, got))
			}
			executed++
		}
	}
	if executed != len(visible)*4 || executed == 0 {
		t.Fatalf("executed %d cases", executed)
	}
	x := baseExchange()
	x.RawQuery = url.Values{"token_id[0]": {"plain-value"}}.Encode()
	if line := lineOf(t, Capture(m, x)); !strings.Contains(line, `"token_id.0":["plain-value"]`) {
		t.Fatalf("an indexed public name lost its value: %s", line)
	}
}

func TestAValueOfAnyTypeUnderACredentialKeyHidesWhole(t *testing.T) {
	m := newMasker(t)
	x := baseExchange()
	x.Request = jsonRequest(`{"pwd":123456,"passwd":true,"new_password":["PLANTED-a","PLANTED-b"],"client_secret":{"inner":"PLANTED-c"},"otp":null,"nested":[{"apiKey":"PLANTED-d","kept":1}]}`)
	c := Capture(m, x)
	want := map[string]any{
		"pwd": marker, "passwd": marker, "new_password": marker, "client_secret": marker, "otp": nil,
		"nested": []any{map[string]any{"apiKey": marker, "kept": json.Number("1")}},
	}
	if !reflect.DeepEqual(c.Request.Body.JSON, want) {
		t.Fatalf("body %#v", c.Request.Body.JSON)
	}
	assertMasked(t, "the line", lineOf(t, c), "PLANTED")
	x = baseExchange()
	x.Request = typed("application/x-www-form-urlencoded", "new_password=PLANTED-a&new_password=PLANTED-b&note=kept")
	form := Capture(m, x).Request.Body.Form
	if !reflect.DeepEqual(form, map[string][]string{"new_password": {marker}, "note": {"kept"}}) {
		t.Fatalf("form %v", form)
	}
}

func TestDeclaredPublicFieldsStayVisibleUnderTheKeysLeftToLeaves(t *testing.T) {
	m, err := newMasker(t).WithPublicFields([]mask.PublicField{
		{Key: "token", Value: "tok-[0-9a-f]{8,64}"},
		{Key: "session", Value: "tok-[0-9a-f]{8,64}"},
		{Key: "credential", Value: "tok-[0-9a-f]{8,64}"},
		{Key: "bearer_token", Value: "tok-[0-9a-f]{8,64}"},
	})
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{"token", "session", "credential", "bearer_token"}
	for _, key := range keys {
		x := baseExchange()
		x.Response = jsonRequest(encodedObject(key, publicToken))
		got, ok := Prove(m, Capture(m, x))
		if !ok || !strings.Contains(lineOf(t, got), publicToken) {
			t.Errorf("%s: the public value is gone: %v %s", key, ok, lineOf(t, got))
		}
		x.Response = jsonRequest(encodedObject(key, plantedCredential))
		assertMasked(t, key+" with a value the field does not allow", lineOf(t, Capture(m, x)), plantedCredential)
	}
	if len(keys) != 4 {
		t.Fatalf("evaluated %d keys", len(keys))
	}
}
