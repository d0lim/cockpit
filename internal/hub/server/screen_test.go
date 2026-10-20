package server

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

var screenAsset = regexp.MustCompile(`(?:src|href)="/assets/([^"]+)"`)

func fetch(t *testing.T, url string) (int, http.Header, string) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, res.Header, string(raw)
}

func TestTheHubServesTheScreenAndEveryAssetItsPagesLoad(t *testing.T) {
	r := start(t, t.TempDir(), "")
	status, header, page := fetch(t, r.url+"/")
	if status != http.StatusOK || !strings.HasPrefix(header.Get("Content-Type"), "text/html") || header.Get("Content-Security-Policy") == "" || !strings.Contains(page, `id="runs"`) {
		t.Fatalf("runs page %d %v %q", status, header, page)
	}
	assets := screenAsset.FindAllStringSubmatch(page, -1)
	for _, m := range assets {
		status, header, body := fetch(t, r.url+"/assets/"+m[1])
		kind := header.Get("Content-Type")
		if status != http.StatusOK || body == "" || !(strings.Contains(kind, "javascript") || strings.HasPrefix(kind, "text/css")) {
			t.Fatalf("%s: %d %s", m[1], status, kind)
		}
	}
	if len(assets) != 4 {
		t.Fatalf("the page loads %d assets: %q", len(assets), page)
	}
	if status, _, _ := fetch(t, r.url+"/assets/missing.js"); status != http.StatusNotFound {
		t.Fatalf("missing asset %d", status)
	}
}
