package mask

import (
	"fmt"
	"reflect"
)

func expectDeep(label string, got, want any) []string {
	if !reflect.DeepEqual(got, want) {
		return []string{fmt.Sprintf("%s: got %#v, want %#v", label, got, want)}
	}
	return nil
}

func leavesPlantedScenario(e engine) []string {
	m, err := e.build(Options{Mode: Hub, Known: []string{planted}})
	if err != nil {
		return []string{err.Error()}
	}
	in := map[string]any{
		"name":       "step-1",
		"attributes": map[string]any{"message": "failed " + planted, "status": 401.0},
		"events":     []any{"retry " + planted},
	}
	want := map[string]any{
		"name":       "step-1",
		"attributes": map[string]any{"message": "failed ****3a9c", "status": 401.0},
		"events":     []any{"retry ****3a9c"},
	}
	return expectDeep("signal", m.Leaves(in), want)
}

func leavesArrayScenario(e engine) []string {
	m, err := e.build(Options{Mode: Hub})
	if err != nil {
		return []string{err.Error()}
	}
	decoded := map[string]any{
		"http.request.header.x-webhook-secret": []any{"whsec-aaaa1111", "whsec-bbbb2222"},
		"http.request.header.content-type":     []any{"application/json"},
	}
	typed := map[string][]string{
		"X-Webhook-Secret": {"whsec-aaaa1111"},
		"Authorization":    {"Bearer abcdef123456"},
		"Accept":           {"text/plain"},
	}
	return append(
		expectDeep("JSON-decoded headers", m.Leaves(decoded), map[string]any{
			"http.request.header.x-webhook-secret": []any{marker, marker},
			"http.request.header.content-type":     []any{"application/json"},
		}),
		expectDeep("typed headers", m.Leaves(typed), map[string][]string{
			"X-Webhook-Secret": {marker},
			"Authorization":    {marker},
			"Accept":           {"text/plain"},
		})...)
}

func leavesScalarScenario(e engine) []string {
	m, err := e.build(Options{Mode: Hub})
	if err != nil {
		return []string{err.Error()}
	}
	in := map[string]any{"pin": 1234.0, "password": 12345678.0, "otp_token": true, "retries": 3}
	return expectDeep("scalars", m.Leaves(in), map[string]any{"pin": 1234.0, "password": marker, "otp_token": marker, "retries": 3})
}

type unknownSignal struct {
	Note  string `json:"note"`
	Token string `json:"token"`
	Count int    `json:"count"`
}

func leavesUnknownTypeScenario(e engine) []string {
	m, err := e.build(Options{Mode: Hub, Known: []string{planted}})
	if err != nil {
		return []string{err.Error()}
	}
	got := m.Leaves(unknownSignal{Note: "value " + planted, Token: "t-1", Count: 2})
	return expectDeep("struct", got, map[string]any{"note": "value ****3a9c", "token": marker, "count": 2.0})
}
