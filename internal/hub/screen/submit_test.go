package screen

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestTheRunsScreenOffersTheAllowedRepositoriesAndTheirSuites(t *testing.T) {
	f := newFixture(t)
	_, _, page := f.get("/")
	if !strings.Contains(page, "Submit a run") || !strings.Contains(page, `<option value="`+toyRepo+`"`) {
		t.Fatalf("runs page: %s", page)
	}
	_, _, options := f.get("/runs/suites?worktree=" + url.QueryEscape(toyRepo))
	if !strings.Contains(options, `value="joint"`) || !strings.Contains(options, `value="canary"`) {
		t.Fatalf("suite options: %s", options)
	}
	_, _, older := f.get("/runs/suites?worktree=" + url.QueryEscape(toyRepo) + "&commit=old")
	if !strings.Contains(older, `value="smoke"`) || strings.Contains(older, `value="joint"`) {
		t.Fatalf("suite options at another commit: %s", older)
	}
	_, _, refused := f.get("/runs/suites?worktree=" + url.QueryEscape("/elsewhere"))
	if strings.Contains(refused, `value="joint"`) || !strings.Contains(refused, "not listed in allowedRepositories") {
		t.Fatalf("suite options of an unlisted repository: %s", refused)
	}
}

func TestASubmitFromTheScreenQueuesARunOrSaysWhyNot(t *testing.T) {
	f := newFixture(t)
	if status, body := f.post("/runs/submit", url.Values{"commit": {"HEAD"}}); status != http.StatusBadRequest || !strings.Contains(body, "pick a repository") {
		t.Fatalf("without a repository: %d %s", status, body)
	}
	if status, body := f.post("/runs/submit", url.Values{"worktree": {toyRepo}, "commit": {"bad"}}); status == http.StatusAccepted || !strings.Contains(body, "tone-bad") {
		t.Fatalf("a bad commit: %d %s", status, body)
	}
	status, body := f.post("/runs/submit", url.Values{"worktree": {toyRepo}, "commit": {""}, "suite": {"joint"}})
	if status != http.StatusAccepted || !strings.Contains(body, "Submitted run-") || !strings.Contains(body, `href="/runs/run-`) {
		t.Fatalf("a good submit: %d %s", status, body)
	}
	list, err := f.st.ListRuns()
	if err != nil || len(list.Runs) != 1 || list.Runs[0].Suite != "joint" || list.Runs[0].Submitter != "ui" {
		t.Fatalf("runs after submit: %+v %v", list.Runs, err)
	}
}
