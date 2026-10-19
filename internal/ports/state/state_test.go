package stateport

import (
	"context"
	"encoding/json"
	"errors"
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

func TestStateMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "state_test.go", stateMutations)
}
