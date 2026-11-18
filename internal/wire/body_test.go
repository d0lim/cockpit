package wire

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const fragment = "FRAGMENT-of-a-secret"

var bodyMutations = []porttest.Mutation{
	{"a body over the limit is omitted whole", "drop the size case in parseBody", "red: the oversized body is stored with its fragment", "TestBodiesThatCannotBeKeptWholeAreOmittedAndLeaveNoFragment"},
	{"a decoded body stays within the limit", "drop the length check after gunzip", "red: a body expanding past the limit is stored", "TestBodiesThatCannotBeKeptWholeAreOmittedAndLeaveNoFragment"},
	{"an unknown encoding is not stored", "treat every encoding as identity", "red: br and double-encoded bodies are stored", "TestBodiesThatCannotBeKeptWholeAreOmittedAndLeaveNoFragment"},
	{"binary bytes are not stored", "drop the UTF-8 and NUL check in classify", "red: binary bodies are stored as text", "TestBodiesThatCannotBeKeptWholeAreOmittedAndLeaveNoFragment"},
	{"multipart bodies are not stored", "drop the multipart check in classify", "red: a multipart body is stored as text", "TestBodiesThatCannotBeKeptWholeAreOmittedAndLeaveNoFragment"},
	{"a recorder omission survives", "ignore RawMessage.Omitted in parseBody", "red: an omitted body loses its reason", "TestBodiesThatCannotBeKeptWholeAreOmittedAndLeaveNoFragment"},
	{"a gzip body within the limit is read and masked", "treat gzip as an unknown encoding", "red: the gzip JSON body is omitted", "TestAGzipBodyWithinTheLimitIsDecodedAndMasked"},
	{"the signature is dropped", "copy the signature into the claims in decodeJWT", "red: the signature reaches the line", "TestACompactJWTThatIsTheWholeBodyIsDecodedWithoutItsSignature"},
	{"only three segments of JSON objects are a JWT", "skip the object check in jwtObject", "red: a token with array claims is decoded", "TestABodyThatIsNotExactlyOneCompactJWTIsNotDecoded"},
}

func TestBodyMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "body_test.go", bodyMutations)
}

func gzipped(t *testing.T, plain string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write([]byte(plain)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encodedAs(encoding string, body []byte) RawMessage {
	return RawMessage{Headers: map[string][]string{"Content-Encoding": {encoding}, "Content-Type": {"text/plain"}}, Body: body}
}

func TestBodiesThatCannotBeKeptWholeAreOmittedAndLeaveNoFragment(t *testing.T) {
	half := strings.Repeat("a", maxBody/2)
	cases := []struct {
		name string
		raw  RawMessage
		want OmitReason
	}{
		{"one byte over the limit", typed("text/plain", half+fragment+half), OmitTooLarge},
		{"JSON over the limit", typed("application/json", `{"note":"`+half+fragment+half+`"}`), OmitTooLarge},
		{"gzip expanding past the limit", encodedAs("gzip", gzipped(t, half+fragment+half)), OmitTooLarge},
		{"corrupt gzip", encodedAs("gzip", []byte(fragment)), OmitEncoded},
		{"truncated gzip", encodedAs("gzip", gzipped(t, fragment)[:12]), OmitEncoded},
		{"another encoding", encodedAs("br", []byte(fragment)), OmitEncoded},
		{"two encodings", encodedAs("gzip, br", gzipped(t, fragment)), OmitEncoded},
		{"a NUL byte", typed("text/plain", fragment+"\x00"), OmitBinary},
		{"invalid UTF-8", typed("application/octet-stream", fragment+"\xff\xfe"), OmitBinary},
		{"multipart", typed("multipart/form-data; boundary=x", "--x\r\n\r\n"+fragment+"\r\n--x--"), OmitBinary},
		{"recorder said too large", RawMessage{Omitted: OmitTooLarge, Size: 5 << 20}, OmitTooLarge},
		{"recorder said busy", RawMessage{Omitted: OmitBusy, Size: 9}, OmitBusy},
		{"recorder said full", RawMessage{Omitted: OmitRecordFull, Size: 9, Body: []byte(fragment)}, OmitRecordFull},
	}
	m := recordMasker(t)
	for _, c := range cases {
		x := baseExchange()
		x.Response = c.raw
		got := Capture(m, x)
		line := lineOf(t, got)
		if got.Response.Body.Kind != BodyOmitted || got.Response.Body.Omitted != c.want {
			t.Errorf("%s: body %+v, want omitted %s", c.name, got.Response.Body, c.want)
		}
		if strings.Contains(line, fragment) {
			t.Errorf("%s: a fragment reached the line", c.name)
		}
		if want := max(c.raw.Size, int64(len(c.raw.Body))); got.Response.Size != want {
			t.Errorf("%s: size %d, want %d", c.name, got.Response.Size, want)
		}
	}
	if len(cases) != 13 {
		t.Fatalf("evaluated %d cases", len(cases))
	}
	x := baseExchange()
	x.Response = typed("text/plain", strings.Repeat("a", maxBody))
	if got := Capture(m, x).Response.Body; got.Kind != BodyText || len(got.Text) != maxBody {
		t.Fatalf("a body of exactly the limit: kind %q, %d bytes", got.Kind, len(got.Text))
	}
}

func TestAGzipBodyWithinTheLimitIsDecodedAndMasked(t *testing.T) {
	x := baseExchange()
	x.Response = RawMessage{
		Headers: map[string][]string{"Content-Encoding": {"GZIP"}, "Content-Type": {"application/json"}},
		Body:    gzipped(t, `{"note":"readable","value":"`+planted+`"}`),
	}
	got := Capture(recordMasker(t), x)
	if got.Response.Body.Kind != BodyJSON {
		t.Fatalf("body %+v", got.Response.Body)
	}
	body := encoded(t, got.Response.Body)
	assertMasked(t, "gzip JSON", body, planted)
	if !strings.Contains(body, `"note":"readable"`) {
		t.Fatalf("the readable field is gone: %s", body)
	}
	if got.Response.Size != int64(len(x.Response.Body)) {
		t.Fatalf("size %d is not the size on the wire", got.Response.Size)
	}
}

func TestACompactJWTThatIsTheWholeBodyIsDecodedWithoutItsSignature(t *testing.T) {
	const signature = "PLANTED-signature-bytes"
	token := compactJWT(`{"alg":"HS256"}`, `{"sub":"user-1","exp":1893456000,"password":"PLANTED-claim"}`, signature)
	x := baseExchange()
	x.Response = typed("text/plain", " "+token+"\n")
	got := Capture(recordMasker(t), x)
	body := got.Response.Body
	if body.Kind != BodyJWT || body.JWT == nil {
		t.Fatalf("body %+v", body)
	}
	line := lineOf(t, got)
	assertMasked(t, "JWT line", line, "PLANTED-claim")
	for _, gone := range []string{token, signature, "UExBTlRFRC1zaWduYXR1cmUtYnl0ZXM"} {
		if strings.Contains(line, gone) {
			t.Errorf("%q reached the line", gone)
		}
	}
	for _, kept := range []string{`"alg":"HS256"`, `"sub":"user-1"`, `"exp":1893456000`} {
		if !strings.Contains(line, kept) {
			t.Errorf("%s is missing from %s", kept, line)
		}
	}
}

func TestABodyThatIsNotExactlyOneCompactJWTIsNotDecoded(t *testing.T) {
	good := compactJWT(`{"alg":"none"}`, `{"sub":"a"}`, "sig")
	cases := []struct {
		name string
		body string
	}{
		{"two segments", good[:strings.LastIndexByte(good, '.')]},
		{"four segments", good + ".extra"},
		{"array header", compactJWT(`["alg"]`, `{"sub":"a"}`, "sig")},
		{"array claims", compactJWT(`{"alg":"none"}`, `["a"]`, "sig")},
		{"scalar claims", compactJWT(`{"alg":"none"}`, `1`, "sig")},
		{"segments that are not base64url", "a!.b!.c!"},
	}
	m := recordMasker(t)
	for _, c := range cases {
		x := baseExchange()
		x.Response = typed("text/plain", c.body)
		if got := Capture(m, x).Response.Body; got.Kind != BodyText || got.JWT != nil {
			t.Errorf("%s: body %+v", c.name, got)
		}
	}
	x := baseExchange()
	x.Response = typed("text/plain", "token follows "+good)
	got := Capture(m, x).Response.Body
	if got.Kind != BodyText {
		t.Fatalf("a JWT inside text: %+v", got)
	}
	assertMasked(t, "JWT inside text", got.Text, good)
	if !strings.Contains(got.Text, "token follows") {
		t.Fatalf("the text around the JWT is gone: %q", got.Text)
	}
	if len(cases) != 6 {
		t.Fatalf("evaluated %d cases", len(cases))
	}
}
