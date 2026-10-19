package server

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

var screenAsset = regexp.MustCompile(`(?:src|href)="\./([^"]+)"`)

func fetch(t *testing.T, url string) (int, string, string) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header.Get("Content-Type"), string(raw)
}

func TestTheHubServesTheEmbeddedScreenAndEveryAssetItReferences(t *testing.T) {
	r := start(t, t.TempDir(), "")
	status, kind, index := fetch(t, r.url+"/")
	if status != http.StatusOK || !strings.HasPrefix(kind, "text/html") || !strings.Contains(index, "<script") {
		t.Fatalf("index %d %s %q", status, kind, index)
	}
	assets := screenAsset.FindAllStringSubmatch(index, -1)
	for _, m := range assets {
		status, kind, body := fetch(t, r.url+"/"+m[1])
		if status != http.StatusOK || body == "" || !(strings.Contains(kind, "javascript") || strings.HasPrefix(kind, "text/css")) {
			t.Fatalf("%s: %d %s", m[1], status, kind)
		}
	}
	if len(assets) != 2 {
		t.Fatalf("index references %d assets: %q", len(assets), index)
	}
	if status, _, _ := fetch(t, r.url+"/index-missing.js"); status != http.StatusNotFound {
		t.Fatalf("missing asset %d", status)
	}
}
