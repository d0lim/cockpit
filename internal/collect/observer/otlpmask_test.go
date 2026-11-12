package observer

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/tracectx"
)

type failingOTLPStream struct {
	appendErr, getErr, putErr error
	appended                  int
}

func (f *failingOTLPStream) Append(contract.Signal) (int64, error) {
	f.appended++
	return 0, f.appendErr
}

func (f *failingOTLPStream) KVGet(string) (map[string]any, error) { return nil, f.getErr }

func (f *failingOTLPStream) KVPut(string, map[string]any) error { return f.putErr }

func plantedErr(what string) error {
	return errors.New(what + " failed near token " + planted)
}

func maskedOTLP(t *testing.T, s *failingOTLPStream) (*OTLP, *bytes.Buffer) {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := &OTLP{RunID: runID, Stream: s, Mask: m.Mask, Log: slog.New(slog.NewTextHandler(&out, nil))}
	o.defaults()
	return o, &out
}

func assertMaskedWarn(t *testing.T, out *bytes.Buffer, message string) {
	t.Helper()
	text := out.String()
	if !strings.Contains(text, message) {
		t.Fatalf("the warn line %q was not written: %q", message, text)
	}
	if strings.Contains(text, planted) || !strings.Contains(text, "****") {
		t.Fatalf("the warn line must hold **** in place of the planted value: %q", text)
	}
}

func TestARefusedOTLPSignalLogsAWarnWithThePlantedValueMasked(t *testing.T) {
	s := &failingOTLPStream{appendErr: plantedErr("append")}
	o, out := maskedOTLP(t, s)
	line := serviceSpan(t, "w3c:"+tracectx.TraceID(runID), "w3c:0000000000000001", "", map[string]any{"service": "app"})
	o.line(stamp(1) + " " + line)
	if s.appended != 1 || o.state.dropped[DropRefused] != 1 {
		t.Fatalf("appended %d, dropped %v", s.appended, o.state.dropped)
	}
	assertMaskedWarn(t, out, "an OTLP signal was refused")
}

func TestAGapEventTheStreamRefusesLogsAWarnWithThePlantedValueMasked(t *testing.T) {
	s := &failingOTLPStream{appendErr: plantedErr("append")}
	o, out := maskedOTLP(t, s)
	o.gap()
	if s.appended != 1 {
		t.Fatalf("appended %d", s.appended)
	}
	assertMaskedWarn(t, out, "cannot record the end of the otlp receiver output")
}

func TestACursorThatCannotBeReadLogsAWarnWithThePlantedValueMasked(t *testing.T) {
	o, out := maskedOTLP(t, &failingOTLPStream{getErr: plantedErr("kv get")})
	o.load()
	assertMaskedWarn(t, out, "cannot read the otlp cursor")
}

func TestACursorThatCannotBeSavedLogsAWarnWithThePlantedValueMasked(t *testing.T) {
	o, out := maskedOTLP(t, &failingOTLPStream{putErr: plantedErr("kv put")})
	o.save()
	assertMaskedWarn(t, out, "cannot save the otlp cursor")
}
