package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/runnerbin"
)

func TestIntegrationHubAndRunsSubmitWaitPassTheToyConsumerKeepCollectingLiveWithNoFalseFailures(t *testing.T) {
	if os.Getenv("COCKPIT_INTEGRATION") != "1" {
		t.Skip("needs COCKPIT_INTEGRATION=1, a Docker daemon and scripts/build-runners.sh")
	}
	if _, err := ensureImage(t.Context(), runnerbin.FilePins{Path: filepath.Join(t.TempDir(), pinsFile)}); errors.Is(err, runnerbin.ErrNotEmbedded) {
		t.Skip("the runner is not embedded; run scripts/build-runners.sh first")
	}
	e := newE2E(t)
	stop := e.startHub()
	defer stop()
	code, out, errOut := e.cockpit(e.repo, "runs", "submit", "--wait", "--json", "--timeout", "1200")
	var run struct {
		ID      string `json:"id"`
		Verdict string `json:"verdict"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &run); err != nil || code != 0 || run.Verdict != "passed" {
		t.Fatalf("submit exit %d json err %v\nstdout %s\nstderr %s", code, err, out, errOut)
	}
	defer e.cockpit(e.repo, "runs", "destroy", run.ID, "--force")
	t.Logf("passed: %s", run.ID)
	dir := filepath.Join(e.home, "runs", run.ID, "report")
	if _, err := os.Stat(filepath.Join(dir, "summary.json")); err != nil {
		t.Fatalf("no bundle: %v", err)
	}
	frozen, err := os.ReadFile(filepath.Join(dir, "signals", "stream.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"collector":"pulse"`, `"collector":"items"`, `"collector":"services"`, `"collector":"logs"`} {
		if !strings.Contains(string(frozen), want) {
			t.Errorf("the bundle stream lacks %s", want)
		}
	}
	noFailures := func(when string) collectorRead {
		r := e.signals(run.ID)
		for _, c := range r.Collectors {
			if c.Failures != 0 {
				t.Errorf("%s: collector %s failed %d times: %s", when, c.ID, c.Failures, c.Reason)
			}
		}
		if len(r.Collectors) != 4 {
			t.Errorf("%s: %d collector states", when, len(r.Collectors))
		}
		return r
	}
	if code, out, errOut := e.cockpit(e.repo, "runs", "keep", run.ID); code != 0 {
		t.Fatalf("runs keep: exit %d\n%s%s", code, out, errOut)
	}
	reported := noFailures("after the report").count("pulse")
	time.Sleep(3500 * time.Millisecond)
	kept := noFailures("on the kept run after the report").count("pulse")
	t.Logf("pulse signals through the API on the kept run: %d at the report, %d 3.5s later", reported, kept)
	if kept <= reported {
		t.Error("the kept run did not keep collecting after the report")
	}
	for _, op := range []string{"stop", "resume"} {
		if code, out, errOut := e.cockpit(e.repo, "runs", op, run.ID); code != 0 {
			t.Fatalf("runs %s: exit %d\n%s%s", op, code, out, errOut)
		}
	}
	before := e.signals(run.ID).count("pulse")
	time.Sleep(3500 * time.Millisecond)
	if code, out, errOut := e.cockpit(e.repo, "runs", "stop", run.ID); code != 0 {
		t.Fatalf("second stop: exit %d\n%s%s", code, out, errOut)
	}
	after := noFailures("around stop and resume").count("pulse")
	t.Logf("pulse signals through the API: %d after resume, %d after the second stop", before, after)
	if after <= before {
		t.Error("pulse did not run after resume")
	}
}
