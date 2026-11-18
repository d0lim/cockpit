package screen

import (
	"net/http"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/wire"
	"github.com/d0lim/cockpit/internal/wire/wiretest"
)

var wireMaskMutations = []porttest.Mutation{
	{"the list reaches the screen masked", "return the stored strings from wireHead in the API", "red: the planted path is in the table", "TestTheWireScreenShowsTheListAndThePickedCallOnlyMasked"},
	{"the picked call reaches the screen cleaned", "answer the stored call from WireCall", "red: the planted body is in the page", "TestTheWireScreenShowsTheListAndThePickedCallOnlyMasked"},
	{"a long body is cut after it was masked", "cut the stored text to 256 KiB before Clean runs in WireCall", "red: the start of the planted value is in the shown part", "TestALongBodyIsCutAtARuneBoundaryAfterItWasMasked"},
}

func TestWireMaskMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "wire_test.go", wireMaskMutations)
}

var maskedPlanted = "****" + planted[len(planted)-4:]

func wireRun(m *store.Meta) { m.Wire = true }

func doneWire(f *fixture, n int) {
	f.seed(runMeta(n, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictFailed}, doneUp, wireRun))
}

func seededWire(t *testing.T) (*fixture, string) {
	t.Helper()
	f := newFixture(t)
	doneWire(f, 1)
	wiretest.Seed(t, f.runDir(rid(1)), planted)
	return f, "/runs/" + rid(1) + "/wire"
}

func TestTheWireScreenShowsTheListAndThePickedCallOnlyMasked(t *testing.T) {
	f, base := seededWire(t)
	status, _, body := f.get(base + "?flow=alpha&step=a2&call=4")
	if status != http.StatusOK || strings.Contains(body, planted) || !strings.Contains(body, "****") {
		t.Fatalf("%d planted %v masked %v", status, strings.Contains(body, planted), strings.Contains(body, "****"))
	}
	for _, want := range []string{"POST /orders/" + maskedPlanted, "failed for " + maskedPlanted, `id="wire-call"`, "ref = " + maskedPlanted} {
		if !strings.Contains(body, want) {
			t.Errorf("canary: the page lacks %q", want)
		}
	}
}

func TestALongBodyIsCutAtARuneBoundaryAfterItWasMasked(t *testing.T) {
	f := newFixture(t)
	doneWire(f, 1)
	call := wiretest.Calls("x")[7]
	text := strings.Repeat("a", wireBodyLimit-3) + planted + strings.Repeat("é", 200)
	call.Request = wire.Message{Size: int64(len(text)), Body: wire.Body{Kind: wire.BodyText, Text: text}}
	wiretest.Store(t, f.runDir(rid(1)), wiretest.Take{Calls: []wire.Call{call}})
	status, _, body := f.get("/runs/" + rid(1) + "/wire?scope=outside&call=8")
	if status != http.StatusOK || strings.Contains(body, planted) || strings.Contains(body, planted[:8]) || !strings.Contains(body, "aaa***</pre>") {
		t.Fatalf("%d planted %v prefix %v masked %v", status, strings.Contains(body, planted), strings.Contains(body, planted[:8]), strings.Contains(body, "aaa***</pre>"))
	}
	if !strings.Contains(body, "Shown up to 256 KiB") {
		t.Fatal("canary: the cut is not announced")
	}
	cut, was := cutText(strings.Repeat("a", wireBodyLimit-1) + "é")
	if !was || len(cut) != wireBodyLimit-1 {
		t.Fatalf("cut at %d bytes, cut %v", len(cut), was)
	}
	if whole, was := cutText("short"); was || whole != "short" {
		t.Fatal("canary: a short text was cut")
	}
}
