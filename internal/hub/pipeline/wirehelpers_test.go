package pipeline

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/fakedocker"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/report"
	"github.com/d0lim/cockpit/internal/wire"
)

const (
	capturedSix = "zq7xk2"
	publicToken = "tok-1f2e3d4c5b6a7980"
	pemBegin    = "-----BEGIN KEY-----"
	pemEnd      = "-----END KEY-----"
)

var wireStart = time.Date(2026, 10, 19, 1, 0, 2, 100_000_000, time.UTC)

func wireCall(seq int64, request, response wire.Body) wire.Call {
	started := wireStart.Add(time.Duration(seq) * time.Millisecond)
	return wire.Call{
		Seq: seq, Caller: "cockpit-runner", Target: "app", Port: 8080, Method: "POST", Path: "/orders",
		Started: started, Finished: started.Add(5 * time.Millisecond), Status: 200, Outcome: wire.OutcomeAnswered,
		Request:  wire.Message{Headers: map[string][]string{"content-type": {"text/plain"}}, Size: int64(len(request.Text)), Body: request},
		Response: wire.Message{Size: int64(len(response.Text)), Body: response},
	}
}

func textBody(s string) wire.Body { return wire.Body{Kind: wire.BodyText, Text: s} }

func secretCalls() []wire.Call {
	return []wire.Call{
		wireCall(1, textBody("token "+planted), textBody("captured "+capturedSix)),
		wireCall(2, textBody("plain request"), textBody("plain response")),
	}
}

func wireWorld(t *testing.T, edit func(*fakedocker.Fake)) *world {
	t.Helper()
	return newWorld(t, func(f *fakedocker.Fake) {
		f.Steps = tracedSteps()
		f.Wire.Calls, f.Wire.Values = secretCalls(), []string{capturedSix}
		if edit != nil {
			edit(f)
		}
	}, declareWire(t))
}

func (w *world) reportedWire() (store.Meta, string) {
	w.t.Helper()
	id, fail := w.submitWire()
	if fail != nil {
		w.t.Fatalf("submit: %+v", fail)
	}
	m := w.meta(id)
	if m.Phase != lifecycle.Done {
		w.t.Fatalf("state %+v notice %q\n%s", m.State, m.Notice, w.eventText())
	}
	dir, err := w.hub.Store().RunDir(id)
	if err != nil {
		w.t.Fatal(err)
	}
	return m, dir
}

func summaryWire(t *testing.T, dir string) contract.WireStatus {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, report.DirReport, report.FileSummary))
	if err != nil {
		t.Fatal(err)
	}
	var doc contract.Summary
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Wire == nil {
		t.Fatalf("the summary has no wire block: %s", data)
	}
	return *doc.Wire
}

func unreadLines(t *testing.T, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, report.DirReport, report.FileSummary))
	if err != nil {
		t.Fatal(err)
	}
	var doc contract.Summary
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.NotProven.Unread
}

func wireFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		files[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func bundleRecord(dir string) string { return filepath.Join(dir, report.DirReport, wire.StagingDir) }

func stagedRecord(dir string) string { return filepath.Join(dir, wire.StagingDir) }

func absent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists: %v", path, err)
	}
}

func present(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("%s is missing: %v", path, err)
	}
}

func oneUnreadWireLine(t *testing.T, dir string, state wire.State) {
	t.Helper()
	var found []string
	for _, line := range unreadLines(t, dir) {
		if strings.HasPrefix(line, stageWire) {
			found = append(found, line)
		}
	}
	if len(found) != 1 || !strings.Contains(found[0], string(state)+":") {
		t.Fatalf("unread wire lines %q, want one naming %s", found, state)
	}
}
