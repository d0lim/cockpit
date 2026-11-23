package screen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/report"
)

func TestTheReportMarksARunAtTheOpenNetworkLevel(t *testing.T) {
	f := newFixture(t)
	finished(f)
	if _, _, body := f.get("/runs/" + rid(1) + "/report"); strings.Contains(body, "network open") || !strings.Contains(body, "seal passed") {
		t.Fatal("a sealed run is marked open, or the canary seal badge is missing")
	}
	path := filepath.Join(f.runDir(rid(1)), report.DirReport, report.FileSummary)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	doc["network"] = "open"
	writeJSON(t, path, doc)
	if _, _, body := f.get("/runs/" + rid(1) + "/report"); !strings.Contains(body, "network open") {
		t.Fatal("an open run has no network open mark")
	}
}
