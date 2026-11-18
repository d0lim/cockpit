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

func TestTheSubmitFormRecordsOnlyWhenItsWireBoxIsTicked(t *testing.T) {
	f := newFixture(t)
	_, _, page := f.get("/")
	if !strings.Contains(page, `name="wire"`) || !strings.Contains(page, `type="checkbox"`) {
		t.Fatalf("the form has no wire box: %s", page)
	}
	cases := map[string]struct {
		form url.Values
		want bool
	}{
		"ticked":   {url.Values{"worktree": {toyRepo}, "commit": {"c1"}, "suite": {"joint"}, "wire": {"1"}}, true},
		"unticked": {url.Values{"worktree": {toyRepo}, "commit": {"c2"}, "suite": {"joint"}}, false},
	}
	for name, c := range cases {
		status, body := f.post("/runs/submit", c.form)
		if status != http.StatusAccepted {
			t.Fatalf("%s: %d %s", name, status, body)
		}
		id := body[strings.Index(body, "/runs/run-")+len("/runs/"):]
		id = id[:strings.IndexByte(id, '"')]
		m, err := f.st.MustRead(id)
		if err != nil || m.Wire != c.want {
			t.Fatalf("%s: wire %v, want %v: %v", name, m.Wire, c.want, err)
		}
	}
}
