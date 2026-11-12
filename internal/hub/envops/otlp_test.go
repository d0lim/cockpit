package envops

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/signal"
)

var otlpOpsMutations = []porttest.Mutation{
	{"the warn for a failed reparent passes the run masker", "log err.Error() in place of o.Mask(id, err.Error()) in ReparentServiceSpans", "red: the warn line holds the planted value", "TestAReparentThatFailsLogsAWarnWithThePlantedValueMasked"},
}

func TestOTLPOpsMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "otlp_test.go", otlpOpsMutations)
}

func TestAReparentThatFailsLogsAWarnWithThePlantedValueMasked(t *testing.T) {
	const planted, id = "PLANTED-envops-5e1d", "run-20261019-010000-abcdef0"
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	st := store.Open(t.TempDir())
	var out bytes.Buffer
	o := &Ops{Store: st, Streams: signal.NewRegistry(st, func(string) *mask.Masker { return m }), Log: slog.New(slog.NewTextHandler(&out, nil))}
	o.maskers.Store(id, m)
	t.Cleanup(o.Streams.CloseAll)
	s, err := o.Streams.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RefuseReads("the bundle was discarded near token " + planted); err != nil {
		t.Fatal(err)
	}
	res := contract.Result{Steps: []contract.ResultStep{{Flow: "orders", ID: "create", Calls: []contract.StepCall{{Method: "POST", Host: "app", Path: "/orders", SpanID: "00f067aa0ba902b7"}}}}}
	o.ReparentServiceSpans(id, id, res)
	text := out.String()
	if !strings.Contains(text, "service spans were not moved under their calls") {
		t.Fatalf("the warn line was not written: %q", text)
	}
	if strings.Contains(text, planted) || !strings.Contains(text, "****") {
		t.Fatalf("the warn line must hold **** in place of the planted value: %q", text)
	}
}
