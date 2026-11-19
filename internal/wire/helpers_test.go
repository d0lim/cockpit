package wire

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/mask"
)

const (
	planted      = "PLANTED-wire-value-9d31"
	plantedDigit = "424242424"
	publicToken  = "tok-1f2e3d4c5b6a7980"
)

var callStart = time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)

func newMasker(t *testing.T, known ...string) *mask.Masker {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Runner, Known: known})
	if err != nil {
		t.Fatal(err)
	}
	m, err = m.WithPublicFields([]mask.PublicField{{Key: "token", Value: "tok-[0-9a-f]{8,64}"}})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func recordMasker(t *testing.T) *mask.Masker {
	t.Helper()
	return newMasker(t, planted, plantedDigit)
}

func baseExchange() Exchange {
	return Exchange{
		Seq:      1,
		Caller:   "orders",
		Target:   "ledger",
		Port:     8081,
		Method:   "POST",
		Path:     "/v1/entries",
		Started:  callStart,
		Finished: callStart.Add(10 * time.Millisecond),
		Status:   200,
		Outcome:  OutcomeAnswered,
	}
}

func encoded(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func lineOf(t *testing.T, c Call) string {
	t.Helper()
	return encoded(t, Line{Call: &c})
}

func assertMasked(t *testing.T, label, text, plain string) {
	t.Helper()
	if strings.Contains(text, plain) || !strings.Contains(text, marker) {
		t.Errorf("%s: want %q gone and %s present in %s", label, plain, marker, text)
	}
}

func jsonRequest(body string) RawMessage {
	return RawMessage{Headers: map[string][]string{"Content-Type": {"application/json"}}, Body: []byte(body)}
}

func typed(media, body string) RawMessage {
	return RawMessage{Headers: map[string][]string{"Content-Type": {media}}, Body: []byte(body)}
}

func compactJWT(header, claims, signature string) string {
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(header)) + "." + enc.EncodeToString([]byte(claims)) + "." + enc.EncodeToString([]byte(signature))
}
