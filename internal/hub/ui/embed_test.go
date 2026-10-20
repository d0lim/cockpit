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

func TestTheEmbeddedAssetsAreTheStylesheetTheScriptAndTheTwoHtmxFiles(t *testing.T) {
	var names []string
	err := fs.WalkDir(Files(), ".", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"app.css", "app.js", "htmx.min.js", "hx-ws.min.js"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("embedded %v, want %v", names, want)
	}
	app, _ := fs.ReadFile(Files(), "app.js")
	if strings.Contains(string(app), "export{") || strings.Contains(string(app), "export {") {
		t.Fatal("app.js is a module, but the page loads it as a classic script")
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
