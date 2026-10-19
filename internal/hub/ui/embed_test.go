package ui

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

var (
	htmlRef     = regexp.MustCompile(`(?i)\b(?:src|href)\s*=\s*"([^"]*)"`)
	cssRef      = regexp.MustCompile(`(?i)(?:url\(\s*['"]?|@import\s+['"])([a-z][a-z0-9+.-]*:|//)`)
	jsRemoteRef = regexp.MustCompile(`(?:import\(|from\s*|fetch\(|EventSource\(|WebSocket\(|importScripts\(|Worker\()\s*['"\x60](?:https?:|wss?:|//)`)
)

func TestEmbeddedScreenServesIndexAndEveryAssetItReferences(t *testing.T) {
	files := Files()
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		t.Fatal(err)
	}
	refs := htmlRef.FindAllStringSubmatch(string(index), -1)
	if len(refs) < 2 {
		t.Fatalf("index.html references %d assets", len(refs))
	}
	for _, m := range refs {
		ref := m[1]
		if !strings.HasPrefix(ref, "./") {
			t.Fatalf("index.html references %q, which is not a relative asset", ref)
		}
		if _, err := fs.Stat(files, strings.TrimPrefix(ref, "./")); err != nil {
			t.Fatalf("index.html references %q, which is not embedded: %v", ref, err)
		}
	}
}

func TestBuildOutputLoadsNothingFromAnExternalURL(t *testing.T) {
	scanned := 0
	err := fs.WalkDir(Files(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(Files(), path)
		if err != nil {
			return err
		}
		text := string(raw)
		scanned++
		switch {
		case strings.HasSuffix(path, ".html"):
			for _, m := range htmlRef.FindAllStringSubmatch(text, -1) {
				if strings.Contains(m[1], "://") || strings.HasPrefix(m[1], "//") {
					t.Errorf("%s loads %q", path, m[1])
				}
			}
		case strings.HasSuffix(path, ".css"):
			if m := cssRef.FindString(text); m != "" {
				t.Errorf("%s loads %q", path, m)
			}
		case strings.HasSuffix(path, ".js"):
			if m := jsRemoteRef.FindString(text); m != "" {
				t.Errorf("%s loads %q", path, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 3 {
		t.Fatalf("scanned %d files", scanned)
	}
	for _, planted := range []string{`<script src="https://cdn.example/x.js">`, `@import "https://fonts.example/a.css";`, `import("https://cdn.example/m.js")`} {
		hit := htmlRef.MatchString(planted) && strings.Contains(planted, "://") || cssRef.MatchString(planted) || jsRemoteRef.MatchString(planted)
		if !hit {
			t.Fatalf("the check does not catch %q", planted)
		}
	}
}

func TestTheSharedWorkerTheScreenStartsIsEmbeddedAndTheScreenNamesIt(t *testing.T) {
	files := Files()
	worker, err := fs.ReadFile(files, "live/worker.js")
	if err != nil || !strings.Contains(string(worker), "onconnect") {
		t.Fatalf("live/worker.js is missing or does not accept connections: %v", err)
	}
	named := 0
	entries, err := fs.Glob(files, "index-*.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range entries {
		raw, err := fs.ReadFile(files, path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), `"/live/worker.js"`) {
			named++
		}
	}
	if len(entries) != 1 || named != 1 {
		t.Fatalf("%d screen bundles, %d name the worker", len(entries), named)
	}
	if !jsRemoteRef.MatchString(`new SharedWorker("https://cdn.example/w.js")`) {
		t.Fatal("the external URL check does not cover workers")
	}
}
