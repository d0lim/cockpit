package wire

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var captureMutations = []porttest.Mutation{
	{"header values are hidden unless listed", "treat every header as readable in cleanHeaders", "red: x-custom and authorization values show", "TestOnlyListedHeadersKeepTheirValuesAndEveryOtherHeaderKeepsItsNameAndReadsTheMarker"},
	{"every text surface passes the masker", "return the path unmasked from Clean", "red: a planted value in the path shows", "TestCaptureMasksAPlantedValueInEverySurfaceOfACall"},
	{"structured values pass Leaves", "store JSON bodies without Leaves", "red: a planted JSON leaf and a number under password show", "TestCaptureMasksAPlantedValueInEverySurfaceOfACall"},
	{"a known number is masked", "drop maskNumbers from cleanValue", "red: a known value written as a JSON number shows", "TestCaptureMasksAPlantedValueInEverySurfaceOfACall"},
	{"a bracketed form name reaches the key rules", "keep form names as they arrive", "red: user[password] shows its value", "TestCaptureMasksAPlantedValueInEverySurfaceOfACall"},
	{"a JSON-shaped body of another type is parsed", "parse JSON only under a JSON media type", "red: a number under password in a text body shows", "TestCaptureMasksAPlantedValueInEverySurfaceOfACall"},
	{"masking leaves ordinary values alone", "replace text bodies with the marker in Clean", "red: the readable text body disappears", "TestCaptureKeepsOrdinaryValuesReadable"},
	{"a span comes from one valid traceparent", "skip tracectx.Parse in spanOf", "red: a malformed traceparent becomes a span", "TestASpanComesOnlyFromASingleValidRequestTraceparent"},
}

func TestCaptureMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "capture_test.go", captureMutations)
}

type surface struct {
	name  string
	plain string
	edit  func(*Exchange)
	pick  func(Call) any
}

func requestBody(c Call) any  { return c.Request.Body }
func requestHeads(c Call) any { return c.Request.Headers }

func withRequest(msg RawMessage) func(*Exchange) {
	return func(x *Exchange) { x.Request = msg }
}

func withHeader(name, value string) func(*Exchange) {
	return func(x *Exchange) { x.Request.Headers = map[string][]string{name: {value}} }
}

func surfaces() []surface {
	return []surface{
		{"JSON leaf", planted, withRequest(jsonRequest(`{"note":"sent ` + planted + `"}`)), requestBody},
		{"JSON number under a credential key", "987654321", withRequest(jsonRequest(`{"password":987654321}`)), requestBody},
		{"known value written as a JSON number", plantedDigit, withRequest(jsonRequest(`{"count":` + plantedDigit + `}`)), requestBody},
		{"JSON string under a credential key", "PLANTED-json-pass", withRequest(jsonRequest(`{"nested":{"client_secret":"PLANTED-json-pass"}}`)), requestBody},
		{"JSON-shaped body of another type", "987654322", withRequest(typed("text/plain", `{"password":987654322}`)), requestBody},
		{"form value", planted, withRequest(typed("application/x-www-form-urlencoded", "note="+planted)), requestBody},
		{"form credential key", "PLANTED-form-pass", withRequest(typed("application/x-www-form-urlencoded", "password=PLANTED-form-pass")), requestBody},
		{"bracketed form key", "PLANTED-nested-pass", withRequest(typed("application/x-www-form-urlencoded", url.Values{"user[password]": {"PLANTED-nested-pass"}}.Encode())), requestBody},
		{"query value", planted, func(x *Exchange) { x.RawQuery = "q=" + planted }, func(c Call) any { return c.Query }},
		{"query credential key", "PLANTED-query-key", func(x *Exchange) { x.RawQuery = "api_key=PLANTED-query-key" }, func(c Call) any { return c.Query }},
		{"text body", planted, withRequest(typed("text/plain", "value "+planted)), requestBody},
		{"response text body", planted, func(x *Exchange) { x.Response = typed("text/plain", "value "+planted) }, func(c Call) any { return c.Response.Body }},
		{"allowed header", planted, withHeader("X-Request-Id", planted), requestHeads},
		{"header outside the list", "PLANTED-custom", withHeader("X-Custom", "PLANTED-custom"), requestHeads},
		{"authorization header", "PLANTED-auth", withHeader("Authorization", "Bearer PLANTED-auth"), requestHeads},
		{"response cookie", "PLANTED-cookie", func(x *Exchange) {
			x.Response.Headers = map[string][]string{"Set-Cookie": {"id=PLANTED-cookie"}}
		}, func(c Call) any { return c.Response.Headers }},
		{"JWT claim", planted, withRequest(typed("application/jwt", compactJWT(`{"alg":"none"}`, `{"sub":"`+planted+`"}`, "sig"))), requestBody},
		{"JWT credential claim", "PLANTED-claim-pass", withRequest(typed("application/jwt", compactJWT(`{"alg":"none"}`, `{"password":"PLANTED-claim-pass"}`, "sig"))), requestBody},
		{"path", planted, func(x *Exchange) { x.Path = "/v1/" + planted }, func(c Call) any { return c.Path }},
		{"caller", planted, func(x *Exchange) { x.Caller = planted }, func(c Call) any { return c.Caller }},
		{"target", planted, func(x *Exchange) { x.Target = planted }, func(c Call) any { return c.Target }},
	}
}

func TestCaptureMasksAPlantedValueInEverySurfaceOfACall(t *testing.T) {
	m := recordMasker(t)
	cases := surfaces()
	for _, s := range cases {
		x := baseExchange()
		s.edit(&x)
		assertMasked(t, s.name, encoded(t, s.pick(Capture(m, x))), s.plain)
	}
	if len(cases) != 21 {
		t.Fatalf("evaluated %d surfaces", len(cases))
	}
}

func TestCaptureKeepsOrdinaryValuesReadable(t *testing.T) {
	x := baseExchange()
	x.RawQuery = "page=2&sort=asc"
	x.Request = jsonRequest(`{"amount":1.10,"id":12345678901234567890,"note":"plain","token":"` + publicToken + `"}`)
	x.Request.Headers["X-Request-Id"] = []string{"req-1"}
	x.Response = typed("text/plain", "accepted")
	c := Capture(recordMasker(t), x)
	line := lineOf(t, c)
	want := []string{`"amount":1.10`, `"id":12345678901234567890`, `"note":"plain"`, `"token":"` + publicToken + `"`, `"content-type":["application/json"]`, `"x-request-id":["req-1"]`, `"text":"accepted"`, `"page":["2"]`, `"path":"/v1/entries"`, `"caller":"orders"`, `"target":"ledger"`, `"method":"POST"`, `"outcome":"answered"`}
	for _, s := range want {
		if !strings.Contains(line, s) {
			t.Errorf("%s is missing from %s", s, line)
		}
	}
	if c.Request.Body.Kind != BodyJSON || c.Response.Body.Kind != BodyText || c.Query.Kind != BodyForm {
		t.Fatalf("kinds %q %q %q", c.Request.Body.Kind, c.Response.Body.Kind, c.Query.Kind)
	}
	if c.Request.Size != int64(len(x.Request.Body)) {
		t.Fatalf("size %d", c.Request.Size)
	}
}

func documentedHeaders(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "wire.md"))
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if strings.HasPrefix(line, "Header values are hidden by default.") {
			sentence, _, _ := strings.Cut(line, " keep theirs")
			var names []string
			for _, m := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(sentence, -1) {
				names = append(names, m[1])
			}
			return names
		}
	}
	t.Fatal("docs/wire.md has no header list")
	return nil
}

func TestOnlyListedHeadersKeepTheirValuesAndEveryOtherHeaderKeepsItsNameAndReadsTheMarker(t *testing.T) {
	listed := documentedHeaders(t)
	if len(listed) != 19 || len(listed) != len(readableHeaders) {
		t.Fatalf("docs list %d headers, the code %d", len(listed), len(readableHeaders))
	}
	m := recordMasker(t)
	for _, name := range listed {
		x := baseExchange()
		x.Request.Headers = map[string][]string{strings.ToUpper(name): {"visible-value"}}
		got := Capture(m, x).Request.Headers[name]
		if !slices.Equal(got, []string{"visible-value"}) {
			t.Errorf("%s: %v", name, got)
		}
	}
	hidden := []string{"authorization", "cookie", "set-cookie", "proxy-authorization", "x-api-key", "x-custom", "x-forwarded-for", "x-trace"}
	for _, name := range hidden {
		x := baseExchange()
		x.Response.Headers = map[string][]string{name: {"PLANTED-header-value"}}
		got := Capture(m, x).Response.Headers
		assertMasked(t, name, encoded(t, got[name]), "PLANTED-header-value")
		if _, ok := got[name]; !ok {
			t.Errorf("%s lost its name: %v", name, got)
		}
	}
	if len(hidden) != 8 {
		t.Fatalf("evaluated %d hidden headers", len(hidden))
	}
}

func TestASpanComesOnlyFromASingleValidRequestTraceparent(t *testing.T) {
	const valid = "00-529345c60c6eb3439cf5567c6e3753c0-00f067aa0ba902b7-01"
	cases := []struct {
		name    string
		headers map[string][]string
		want    string
	}{
		{"valid", map[string][]string{"traceparent": {valid}}, "00f067aa0ba902b7"},
		{"canonical name", map[string][]string{"Traceparent": {valid}}, "00f067aa0ba902b7"},
		{"malformed", map[string][]string{"traceparent": {"00-short-01"}}, ""},
		{"zero span", map[string][]string{"traceparent": {"00-529345c60c6eb3439cf5567c6e3753c0-0000000000000000-01"}}, ""},
		{"two values", map[string][]string{"traceparent": {valid, valid}}, ""},
		{"absent", nil, ""},
	}
	m := recordMasker(t)
	for _, c := range cases {
		x := baseExchange()
		x.Request.Headers = c.headers
		if got := Capture(m, x).Span; got != c.want {
			t.Errorf("%s: span %q, want %q", c.name, got, c.want)
		}
	}
	x := baseExchange()
	x.Response.Headers = map[string][]string{"traceparent": {valid}}
	if got := Capture(m, x).Span; got != "" {
		t.Errorf("a response traceparent gave span %q", got)
	}
}
