package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

const (
	surfaceRecord   = "report/wire"
	surfaceSummary  = "summary.json"
	surfaceView     = "wire view"
	surfaceCall     = "wire call"
	surfaceList     = "screen list"
	surfacePicked   = "screen call"
	surfaceRecorder = "recorder logs"
	wantedValues    = 7
)

type wireSurface struct {
	kind, name, text string
}

type wireSurfaces []wireSurface

func dotenvValue(t *testing.T, path, name string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), name+"="); ok && v != "" {
			return v
		}
	}
	t.Fatalf("%s holds no %s", path, name)
	return ""
}

func derived(appToken, label string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(appToken+"|"+label)))[:24]
}

func (w wireRun) credentialValues() []string {
	t := w.e.t
	t.Helper()
	env := filepath.Join(w.dir(), "src", ".env")
	password := dotenvValue(t, env, "POSTGRES_PASSWORD")
	appToken := dotenvValue(t, env, "APP_TOKEN")
	var private struct {
		Local map[string]string `json:"local"`
	}
	readJSONFile(t, filepath.Join(w.dir(), "src", "e2e.private.env.json"), &private)
	var captured contract.WireValues
	readJSONFile(t, filepath.Join(w.dir(), "work", "e2e", contract.WireValuesFile), &captured)
	claims := mustJSON(t, map[string]string{"sub": "user-7", "aud": "ledger", "note": password, "apiToken": derived(appToken, "claim")})
	values := []string{password, appToken, private.Local["partnerKey"], derived(appToken, "shape"), derived(appToken, "claim"), base64.RawURLEncoding.EncodeToString([]byte(claims))}
	values = append(values, captured.Values...)
	slices.Sort(values)
	values = slices.Compact(values)
	if len(values) != wantedValues || slices.Contains(values, "") {
		t.Fatalf("%d credential values were read, want %d", len(values), wantedValues)
	}
	return values
}

func (w wireRun) surfaces(takes ...string) wireSurfaces {
	t := w.e.t
	t.Helper()
	var out wireSurfaces
	root := filepath.Join(w.dir(), "report", "wire")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err == nil {
			out = append(out, wireSurface{surfaceRecord, path, string(data)})
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := os.ReadFile(filepath.Join(w.dir(), "report", "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	out = append(out, wireSurface{surfaceSummary, "summary.json", string(summary)})
	if logs := w.e.logs(w.id, "cockpit-wire"); logs != "" {
		out = append(out, wireSurface{surfaceRecorder, "docker logs", logs})
	}
	for _, take := range takes {
		out = append(out, w.takeSurfaces(take)...)
	}
	return out
}

func (w wireRun) takeSurfaces(take string) wireSurfaces {
	t := w.e.t
	t.Helper()
	var out wireSurfaces
	api := "/api/runs/" + w.id + "/wire"
	page := "/runs/" + w.id + "/wire"
	queries := []string{"take=" + take, "take=" + take + "&scope=outside"}
	for _, s := range w.view("?take=" + take).Steps {
		queries = append(queries, "take="+take+"&flow="+s.Flow+"&step="+s.Step)
	}
	seqs := map[int64]bool{}
	for _, q := range queries {
		var view struct {
			Calls []struct {
				Seq int64 `json:"seq"`
			} `json:"calls"`
		}
		body := w.getJSON(api+"?limit=500&"+q, &view)
		out = append(out, wireSurface{surfaceView, q, body})
		for _, c := range view.Calls {
			seqs[c.Seq] = true
		}
		out = append(out, wireSurface{surfaceList, q, w.screen(page + "?" + q)})
	}
	if len(seqs) == 0 {
		t.Fatalf("take %s lists no call", take)
	}
	for seq := range seqs {
		var raw json.RawMessage
		out = append(out, wireSurface{surfaceCall, itoa(seq), w.getJSON(api+"/"+itoa(seq)+"?take="+take, &raw)})
		out = append(out, wireSurface{surfacePicked, itoa(seq), w.screen(page + "?take=" + take + "&call=" + itoa(seq))})
	}
	return out
}

func (w wireRun) screen(path string) string {
	w.e.t.Helper()
	status, body := w.get(path)
	if status != 200 {
		w.e.t.Fatalf("GET %s: %d", path, status)
	}
	return body
}

func (s wireSurfaces) assertClean(t *testing.T, when string, values []string) {
	t.Helper()
	checks := 0
	markers := map[string]int{}
	counts := map[string]int{}
	for _, surface := range s {
		counts[surface.kind]++
		markers[surface.kind] += strings.Count(surface.text, hidden)
		for i, value := range values {
			checks++
			if strings.Contains(surface.text, value) {
				t.Errorf("%s: credential value %d is in %s %s", when, i, surface.kind, surface.name)
			}
		}
	}
	if checks == 0 || len(values) == 0 {
		t.Fatalf("%s: no value was checked", when)
	}
	for _, kind := range []string{surfaceRecord, surfaceView, surfaceCall, surfaceList, surfacePicked} {
		if counts[kind] == 0 || markers[kind] == 0 {
			t.Errorf("%s: %s has %d items and %d markers", when, kind, counts[kind], markers[kind])
		}
	}
	if counts[surfaceSummary] != 1 {
		t.Errorf("%s: %d summaries", when, counts[surfaceSummary])
	}
	t.Logf("%s: %d values against %d surfaces, markers %v", when, len(values), len(s), markers)
}

func (s wireSurfaces) assertVisible(t *testing.T, public ...string) {
	t.Helper()
	for _, want := range public {
		seen := map[string]bool{}
		for _, surface := range s {
			if strings.Contains(surface.text, want) {
				seen[surface.kind] = true
			}
		}
		for _, kind := range []string{surfaceRecord, surfaceCall, surfacePicked} {
			if !seen[kind] {
				t.Errorf("the public value %q is not visible in %s", want, kind)
			}
		}
	}
}
