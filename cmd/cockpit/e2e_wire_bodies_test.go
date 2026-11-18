package main

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/wire"
)

const hidden = "****"

func field(t *testing.T, body wire.Body, path ...string) any {
	t.Helper()
	var at any = body.JSON
	for _, key := range path {
		object, ok := at.(map[string]any)
		if !ok {
			t.Fatalf("%v: %v is not an object in %+v", path, at, body)
		}
		at = object[key]
	}
	return at
}

func assertBodies(t *testing.T, w wireRun) {
	t.Helper()
	assertCredentialPlaces(t, w.picked("credentials", "env-token", 0))
	assertCredentialPlaces(t, w.picked("credentials", "private-key", 0))
	assertCredentialPlaces(t, w.picked("credentials", "service-secret", 1))
	if p := w.headOf(wire.SuiteTake, "credentials", "path-secret", 0).Path; p != "/accounts/"+hidden {
		t.Error("the credential in the path is not masked")
	}

	shapes := w.picked("credentials", "shapes", 0).Response.Body
	if got := field(t, shapes, "numeric", "password"); got != hidden {
		t.Error("the numeric password is not masked")
	}
	if got := field(t, shapes, "private", "token"); got != hidden {
		t.Error("the private token is not masked")
	}
	if got := field(t, shapes, "public", "token"); got != "tok-0a1b2c3d4e5f" {
		t.Errorf("the public token reads %v", got)
	}

	issued := w.picked("learned", "issue", 0).Response.Body
	if got := field(t, issued, "reference"); got != hidden {
		t.Error("the captured reference is not masked in the response")
	}
	redeem := w.picked("learned", "redeem", 0).Request.Body
	if got := field(t, redeem, "reference"); got != hidden {
		t.Error("the captured reference is not masked in the request")
	}
	if field(t, redeem, "id") != field(t, issued, "id") {
		t.Error("the public id does not read the same in both calls")
	}

	if got := field(t, w.picked("recall", "latest", 0).Response.Body, "latest"); got != hidden {
		t.Error("the repeated reference is not masked")
	}

	token := w.picked("bodies", "jwt", 0).Response.Body
	if token.Kind != wire.BodyJWT || token.JWT == nil {
		t.Fatalf("the token body reads %+v", token)
	}
	for key, want := range map[string]string{"note": hidden, "apiToken": hidden, "sub": "user-7", "aud": "ledger"} {
		if token.JWT.Claims[key] != want {
			t.Errorf("claim %s does not read %s", key, want)
		}
	}

	begin, end := w.picked("bodies", "pem-begin", 0).Response.Body, w.picked("bodies", "pem-end", 0).Response.Body
	if !strings.Contains(begin.Text, "BEGIN PRIVATE KEY") || !strings.Contains(end.Text, "END PRIVATE KEY") {
		t.Errorf("the PEM halves read %q and %q", begin.Text, end.Text)
	}
	var meta map[string]any
	readJSONFile(t, filepath.Join(w.dir(), "report", "meta.json"), &meta)
	if _, discarded := meta["redaction"]; discarded {
		t.Errorf("the bundle was discarded: %v", meta["redaction"])
	}

	big := w.picked("bodies", "big", 0).Response
	if big.Body.Kind != wire.BodyOmitted || big.Body.Omitted != wire.OmitTooLarge || big.Size <= wire.MaxBody {
		t.Errorf("the big response reads %+v of size %d", big.Body, big.Size)
	}
	binary := w.picked("bodies", "binary", 0).Response
	if binary.Body.Kind != wire.BodyOmitted || binary.Body.Omitted != wire.OmitBinary || binary.Size != 12 {
		t.Errorf("the binary response reads %+v of size %d", binary.Body, binary.Size)
	}
}

func assertCredentialPlaces(t *testing.T, c wire.Call) {
	t.Helper()
	if got := c.Query.Form["note"]; !slices.Equal(got, []string{hidden}) {
		t.Errorf("call %d query is not masked", c.Seq)
	}
	if got := field(t, c.Request.Body, "note"); got != hidden {
		t.Errorf("call %d body is not masked", c.Seq)
	}
	if got := field(t, c.Response.Body, "note"); got != hidden {
		t.Errorf("call %d response is not masked", c.Seq)
	}
	headers := 0
	for _, name := range []string{"authorization", "x-partner-key", "x-ledger-key"} {
		if values, ok := c.Request.Headers[name]; ok {
			headers++
			if !slices.Equal(values, []string{hidden}) {
				t.Errorf("call %d header %s is not masked", c.Seq, name)
			}
		}
	}
	if headers != 2 {
		t.Errorf("call %d holds %d of the credential headers", c.Seq, headers)
	}
}

func (w wireRun) assertSealPassed() {
	t := w.e.t
	t.Helper()
	var seal struct {
		Verdict string `json:"verdict"`
		OK      bool   `json:"ok"`
		Static  []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"static"`
		Probes []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"probes"`
	}
	readJSONFile(t, filepath.Join(w.dir(), "report", "seal.json"), &seal)
	if seal.Verdict != "passed" || !seal.OK {
		t.Errorf("seal verdict %q ok %v", seal.Verdict, seal.OK)
	}
	passed := 0
	for _, s := range seal.Static {
		if !s.OK {
			t.Errorf("static assertion %s failed", s.Name)
		} else {
			passed++
		}
	}
	if passed != 25 || len(seal.Static) != 25 {
		t.Errorf("static seal %d of %d", passed, len(seal.Static))
	}
	if len(seal.Probes) == 0 {
		t.Error("the seal file holds no probes")
	}
	for _, p := range seal.Probes {
		if !p.OK {
			t.Errorf("probe %s failed", p.Name)
		}
	}
	if w.summary().Seal != contract.SealPassed {
		t.Errorf("summary seal %s", w.summary().Seal)
	}
}

func (w wireRun) assertScreenShowsWire() {
	t := w.e.t
	t.Helper()
	_, page := w.get("/runs/" + w.id)
	if !strings.Contains(page, "/runs/"+w.id+"/wire") {
		t.Error("the overview does not link the Wire screen")
	}
	if !strings.Contains(page, "passed through the wire recorder") {
		t.Error("the overview does not say the run was recorded")
	}
	_, screen := w.get("/runs/" + w.id + "/wire")
	for _, want := range []string{"child-span", "POST", "/relay"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the Wire screen lacks %q", want)
		}
	}
	jwt := w.headOf(wire.SuiteTake, "bodies", "jwt", 0)
	_, picked := w.get("/runs/" + w.id + "/wire?flow=bodies&step=jwt&call=" + itoa(jwt.Seq))
	for _, want := range []string{"Decoded JWT", "apiToken", "user-7", hidden} {
		if !strings.Contains(picked, want) {
			t.Errorf("the picked JWT call lacks %q on the screen", want)
		}
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
