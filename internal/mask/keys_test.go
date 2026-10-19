package mask

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var keyMutations = []porttest.Mutation{
	{"map keys are masked like values", "return the original key from maskKeys", "red: the planted key is in every map shape and in the runner result", "TestASecretUsedAsAMapKeyIsMaskedInEveryMapShapeAndOrdinaryKeysStay"},
	{"keys that mask to the same text keep every value", "write every masked key without a suffix", "red: only one of the two colliding values survives", "TestKeysThatMaskToTheSameTextKeepEveryValueWithAStableSuffixAndOrdinaryKeysKeepTheirName"},
	{"the hidden-name rule reads the original key", "pass the masked key to hiddenKey", "red: a secret key under a credential name keeps its value", "TestASecretUsedAsAMapKeyIsMaskedInEveryMapShapeAndOrdinaryKeysStay"},
}

func TestKeyMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "keys_test.go", keyMutations)
}

const otherPlanted = "SECOND-secret-abcd3a9c"

func keyMasker(t *testing.T) *Masker {
	t.Helper()
	m, err := New(Options{Mode: Hub, Known: []string{planted, otherPlanted}})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func encoded(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestASecretUsedAsAMapKeyIsMaskedInEveryMapShapeAndOrdinaryKeysStay(t *testing.T) {
	m := keyMasker(t)
	shapes := map[string]any{
		"any":     map[string]any{planted: "seen", "status": "ok", "nested": map[string]any{planted: 1.0}},
		"strings": map[string]string{planted: "seen", "status": "ok"},
		"lists":   map[string][]string{planted: {"seen"}, "status": {"ok"}},
	}
	var result any
	if err := json.Unmarshal([]byte(`{"schemaVersion":2,"complete":true,"steps":[{"flow":"health","id":"health-ok","status":"passed","requests":1}],"captures":{"`+planted+`":"x","requestId":"r-1"}}`), &result); err != nil {
		t.Fatal(err)
	}
	shapes["runner result"] = result
	for name, in := range shapes {
		out := encoded(t, m.Leaves(in))
		if strings.Contains(out, planted) || !strings.Contains(out, "****") {
			t.Errorf("%s: %s", name, out)
		}
		if !strings.Contains(out, `"status"`) && name != "runner result" || name == "runner result" && !strings.Contains(out, `"requestId":"r-1"`) {
			t.Errorf("%s: an ordinary key changed: %s", name, out)
		}
	}
	if len(shapes) != 4 {
		t.Fatal(len(shapes))
	}
	hidden := encoded(t, m.Leaves(map[string]any{"api_token": map[string]any{planted: "v"}, "note": "plain"}))
	if strings.Contains(hidden, planted) || !strings.Contains(hidden, `"note":"plain"`) || !strings.Contains(hidden, `"api_token":{"****3a9c":"****"}`) {
		t.Fatalf("hidden-name rule: %s", hidden)
	}
	named, err := New(Options{Mode: Hub, Known: []string{"svc.api_token"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := encoded(t, named.Leaves(map[string]any{"svc.api_token": "plain-value"})); got != `{"****oken":"****"}` {
		t.Fatalf("a secret key under a credential name: %s", got)
	}
}

func TestKeysThatMaskToTheSameTextKeepEveryValueWithAStableSuffixAndOrdinaryKeysKeepTheirName(t *testing.T) {
	m := keyMasker(t)
	if m.Mask(planted) != m.Mask(otherPlanted) {
		t.Fatalf("canary: %q and %q do not collide", m.Mask(planted), m.Mask(otherPlanted))
	}
	in := map[string]any{planted: "first", otherPlanted: "second", "status": "ok"}
	want := `{"****3a9c":"second","****3a9c#2":"first","status":"ok"}`
	for range 20 {
		if got := encoded(t, m.Leaves(in)); got != want {
			t.Fatalf("got %s want %s", got, want)
		}
	}
	taken := map[string]string{"****3a9c": "ordinary", planted: "secret"}
	if got := encoded(t, m.Leaves(taken)); got != `{"****3a9c":"ordinary","****3a9c#2":"secret"}` {
		t.Fatalf("an ordinary key lost its name: %s", got)
	}
}
