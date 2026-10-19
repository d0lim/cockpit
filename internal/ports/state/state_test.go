package stateport

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/signal"
)

var stateMutations = []porttest.Mutation{
	{"state is scoped to the port's collector", "use a fixed collector id in KVGet and KVPut", "red: acme-queue reads the toy-db cursor", "TestStateIsScopedToTheCollectorAndSurvivesARestart"},
	{"keys are checked", "drop validKey", "red: an empty key and a control-character key are stored", "TestBadKeysOversizedValuesAndUnknownMethodsAreRefused"},
	{"keys and values pass the run redactor before they are stored", "store key and value without mask", "red: the planted value is in kv/toy-db.json", "TestCredentialsInStateKeysAndValuesAreMaskedBeforeTheyReachTheKVFileAndOtherValuesRoundTrip"},
	{"ordinary state values are not rewritten by the run redactor", "mask values with a rule that rewrites every 22-character id", "red: the base64url id reads back as ****", "TestOrdinaryStateValuesRoundTripByteIdenticalThroughTheHubRedactorAndARestartWhileAPlantedSecretIsMasked"},
	{"values have a size limit", "drop the MaxValue check", "red: a 100 KiB value is stored", "TestBadKeysOversizedValuesAndUnknownMethodsAreRefused"},
}

func open(t *testing.T, dir string) *signal.Stream {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Hub})
	if err != nil {
		t.Fatal(err)
	}
	s, err := signal.Open(dir, signal.Options{Masker: func() *mask.Masker { return m }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func args(values ...string) []json.RawMessage {
	out := make([]json.RawMessage, len(values))
	for i, v := range values {
		out[i], _ = json.Marshal(v)
	}
	return out
}

func TestStateIsScopedToTheCollectorAndSurvivesARestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	s := open(t, dir)
	toy := &Port{Collector: "toy-db", Store: s}
	acme := &Port{Collector: "acme-queue", Store: s}
	if v, err := toy.Call(context.Background(), "get", args("cursor")); err != nil || v != nil {
		t.Fatalf("missing key: %v %v", v, err)
	}
	if _, err := toy.Call(context.Background(), "set", args("cursor", "42")); err != nil {
		t.Fatal(err)
	}
	if v, _ := acme.Call(context.Background(), "get", args("cursor")); v != nil {
		t.Fatalf("acme-queue sees %v", v)
	}
	s.Close()
	again := &Port{Collector: "toy-db", Store: open(t, dir)}
	if v, err := again.Call(context.Background(), "get", args("cursor")); err != nil || v != "42" {
		t.Fatalf("%v %v", v, err)
	}
}

func TestBadKeysOversizedValuesAndUnknownMethodsAreRefused(t *testing.T) {
	p := &Port{Collector: "toy-db", Store: open(t, filepath.Join(t.TempDir(), "run"))}
	codeOf := func(err error) engine.PortCode {
		var pe *engine.PortError
		if errors.As(err, &pe) {
			return pe.Code
		}
		return ""
	}
	for _, key := range []string{"", "a\nb", strings.Repeat("k", 257)} {
		if _, err := p.Call(context.Background(), "set", args(key, "v")); codeOf(err) != engine.PortBadArgs {
			t.Errorf("key %q: %v", key, err)
		}
	}
	if _, err := p.Call(context.Background(), "set", args("big", strings.Repeat("x", 100<<10))); codeOf(err) != engine.PortLimit {
		t.Fatalf("%v", err)
	}
	if _, err := p.Call(context.Background(), "delete", args("k")); codeOf(err) != engine.PortBadArgs {
		t.Fatalf("%v", err)
	}
	if _, err := p.Call(context.Background(), "set", args(strings.Repeat("k", 256), strings.Repeat("x", 1000))); err != nil {
		t.Fatalf("canary: %v", err)
	}
}

func TestCredentialsInStateKeysAndValuesAreMaskedBeforeTheyReachTheKVFileAndOtherValuesRoundTrip(t *testing.T) {
	const planted = "PLANTED-state-4d2a"
	dir := filepath.Join(t.TempDir(), "run")
	p := &Port{Collector: "toy-db", Store: open(t, dir), Mask: func(s string) string { return strings.ReplaceAll(s, planted, "****") }}
	if _, err := p.Call(context.Background(), "set", args("tok-"+planted, "seen "+planted)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Call(context.Background(), "set", args("cursor", "42")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "signals", "kv", "toy-db.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), planted) || !strings.Contains(string(data), `"tok-****":"seen ****"`) {
		t.Fatalf("kv file %s", data)
	}
	if v, err := p.Call(context.Background(), "get", args("tok-"+planted)); err != nil || v != "seen ****" {
		t.Fatalf("masked key read back %v %v", v, err)
	}
	if v, err := p.Call(context.Background(), "get", args("cursor")); err != nil || v != "42" {
		t.Fatalf("plain value read back %v %v", v, err)
	}
}

func TestOrdinaryStateValuesRoundTripByteIdenticalThroughTheHubRedactorAndARestartWhileAPlantedSecretIsMasked(t *testing.T) {
	const planted = "PLANTED-state-secret-7c1e"
	digest := sha256.Sum256([]byte("toy rows"))
	id := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef"))
	ordinary := map[string]string{
		"digest": hex.EncodeToString(digest[:]),
		"id":     id,
		"ts":     "2026-10-19T05:44:12.123456789Z",
		"seq":    "184467",
		"uuid":   "3f2b8c1e-9a4d-4e6f-8b7a-1c2d3e4f5a6b",
	}
	object, err := json.Marshal(ordinary)
	if err != nil {
		t.Fatal(err)
	}
	ordinary["object"] = string(object)
	if len(ordinary["digest"]) != 64 || len(id) != 22 {
		t.Fatalf("digest %d and id %d characters", len(ordinary["digest"]), len(id))
	}
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}, Patterns: []string{"toy_[A-Za-z0-9]{24}"}})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "run")
	p := &Port{Collector: "toy-db", Store: open(t, dir), Mask: m.Mask}
	for key, value := range ordinary {
		if _, err := p.Call(context.Background(), "set", args(key, value)); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
	}
	if _, err := p.Call(context.Background(), "set", args("secret", planted)); err != nil {
		t.Fatal(err)
	}
	again := &Port{Collector: "toy-db", Store: open(t, dir), Mask: m.Mask}
	checked := 0
	for key, want := range ordinary {
		if got, err := again.Call(context.Background(), "get", args(key)); err != nil || got != want {
			t.Errorf("%s: read back %q %v, want %q", key, got, err, want)
		}
		checked++
	}
	if checked != 6 {
		t.Fatalf("checked %d values", checked)
	}
	got, err := again.Call(context.Background(), "get", args("secret"))
	if s, _ := got.(string); err != nil || strings.Contains(s, planted) || !strings.HasPrefix(s, "****") {
		t.Fatalf("planted secret read back %q %v", got, err)
	}
}

func TestStateMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "state_test.go", stateMutations)
}
