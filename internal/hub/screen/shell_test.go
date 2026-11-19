package screen

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
)

func TestTheSidebarNamesTheHubItsScreensTheQueueTheKeptPoolAndTheOtherHubs(t *testing.T) {
	f := newFixture(t)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Queued, Env: lifecycle.EnvAbsent}))
	f.seed(runMeta(2, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvStopped, Verdict: lifecycle.VerdictFailed}, doneUp))
	st, err := f.sc.load(t.Context(), "/runs/"+rid(2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := st.pickRun(rid(2)); status != 200 {
		t.Fatalf("pick %d", status)
	}
	st.list.Foreign = []string{"toy-shop"}
	st.list.DockerDown = true
	st.list.Hubs = []api.HubItem{
		{ID: "self00000000", Name: "here", Self: true, Up: true},
		{ID: "abc123abc123", Name: "other", Up: true, URL: "http://127.0.0.1:7799"},
		{ID: "def456def456", Name: "asleep", Home: "/state/asleep"},
	}
	out := render(t, sidebar(st))
	for _, want := range []string{
		`id="sidebar"`, ">here<", "127.0.0.1:7710", "Docker does not answer",
		`href="/" class="side-link side-on"`, `href="/stacks" class="side-link"`, `href="/hub" class="side-link"`, `class="side-count-warn">1<`,
		`aria-label="Now"`, `href="/runs/` + rid(1) + `"`, "queued #1",
		`href="/runs/` + rid(2) + `/report" class="side-run side-on"`, ">stopped<", "Running 0 / ",
		`href="http://127.0.0.1:7799/" hx-boost="false"`, ">other<", `href="/hub#hubs"`, ">asleep<", ">not running<",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the sidebar lacks %q", want)
		}
	}
	if strings.Contains(out, `>here</span><span class="side-note">`) || strings.Count(out, `class="side-run side-on"`) != 1 {
		t.Fatal("this hub is listed among the other hubs, or more than the shown run is marked")
	}
}

func TestARunWithoutAnEnvironmentOffersNoActionAndSaysSo(t *testing.T) {
	f := newFixture(t)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvDestroyed, Verdict: lifecycle.VerdictPassed}, func(m *store.Meta) {
		m.StartedAt, m.FinishedAt = "2026-10-19T01:00:01.000Z", "2026-10-19T01:03:10.000Z"
	}))
	f.seed(runMeta(2, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvStopped, Verdict: lifecycle.VerdictPassed}, doneUp))
	_, _, gone := f.get("/runs/" + rid(1))
	head := section(t, strings.Replace(gone, "<header ", "<section ", 1), "run-head")
	if strings.Contains(head, "/do/") || !strings.Contains(head, "No environment, no actions") || !strings.Contains(head, "took 3m 9s") {
		t.Fatalf("a destroyed run: %s", head)
	}
	_, _, stopped := f.get("/runs/" + rid(2))
	if !strings.Contains(stopped, `/do/resume"`) || !strings.Contains(stopped, `/do/destroy"`) || strings.Contains(stopped, "No environment, no actions") {
		t.Fatal("canary: a stopped run offers resume and destroy")
	}
}

func TestTheEnvironmentScreenRepeatsResumeBesideThePortsOfAStoppedEnvironmentOnly(t *testing.T) {
	f := newFixture(t)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvStopped, Verdict: lifecycle.VerdictPassed}, doneUp))
	f.seed(runMeta(2, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictPassed}, doneUp, func(m *store.Meta) {
		slot := 2
		m.Slot = &slot
	}))
	_, _, stopped := f.get("/runs/" + rid(1) + "/env")
	if env := section(t, stopped, "env"); !strings.Contains(env, `/do/resume"`) || !strings.Contains(env, "these ports do not answer") || !strings.Contains(env, ">App<") {
		t.Fatalf("a stopped environment: %s", env)
	}
	_, _, up := f.get("/runs/" + rid(2) + "/env")
	if env := section(t, up, "env"); strings.Contains(env, "/do/") || !strings.Contains(env, ">App<") {
		t.Fatalf("an environment that is up repeats an action or lost the canary port: %s", env)
	}
	f.seed(runMeta(3, lifecycle.State{Phase: lifecycle.Resuming, Env: lifecycle.EnvStopped, Verdict: lifecycle.VerdictPassed}, doneUp, func(m *store.Meta) {
		slot := 3
		m.Slot = &slot
	}))
	_, _, resuming := f.get("/runs/" + rid(3) + "/env")
	if env := section(t, resuming, "env"); strings.Contains(env, "/do/") || !strings.Contains(env, "these ports do not answer") {
		t.Fatalf("an environment that is already resuming offers resume again: %s", env)
	}
}

func TestTookTextReadsARunsLengthInItsTwoLargestUnits(t *testing.T) {
	cases := map[string][2]string{
		"9s":     {"2026-10-19T01:00:00.000Z", "2026-10-19T01:00:09.400Z"},
		"3m 9s":  {"2026-10-19T01:00:01.000Z", "2026-10-19T01:03:10.000Z"},
		"2h 5m":  {"2026-10-19T01:00:00.000Z", "2026-10-19T03:05:59.000Z"},
		"0s":     {"2026-10-19T01:00:00.000Z", "2026-10-19T01:00:00.000Z"},
		"":       {"2026-10-19T01:00:05.000Z", "2026-10-19T01:00:00.000Z"},
		"\x00no": {"", "2026-10-19T01:00:00.000Z"},
	}
	for want, c := range cases {
		if got := tookText(c[0], c[1]); got != strings.TrimPrefix(want, "\x00no") {
			t.Errorf("%v: %q, want %q", c, got, want)
		}
	}
	if len(cases) != 6 {
		t.Fatal("case count")
	}
}

func endedRuns(f *fixture, n int) {
	for i := range n {
		verdict := lifecycle.VerdictPassed
		if i%10 == 0 {
			verdict = lifecycle.VerdictFailed
		}
		id := "run-20261019-02" + strconv.Itoa(1000+i) + "-abc1234"
		f.seed(store.Meta{State: lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvDestroyed, Verdict: verdict}, ID: id, SHA: "abc1234", Suite: "toy", HostAPI: 1, Submitter: "cli",
			CreatedAt: "2026-10-19T02:" + strconv.Itoa(1000 + i)[:2] + ":" + strconv.Itoa(1000 + i)[2:] + ".000Z", Notice: "failed steps: 1, the first at alpha › a2"})
	}
}

func TestEndedRunsAreCutToTheLatestThirtyUntilAllIsAskedAndTheVerdictFilterCountsEveryRun(t *testing.T) {
	f := newFixture(t)
	endedRuns(f, 40)
	rows := func(query string) (int, string) {
		_, _, page := f.get("/" + query)
		region := section(t, page, "runs")
		return strings.Count(region, `class="row"`), region
	}
	n, region := rows("")
	if n != endedShown || !strings.Contains(region, "The latest 30 of 40 are listed.") || !strings.Contains(region, `href="/?all=1"`) {
		t.Fatalf("the plain list shows %d rows", n)
	}
	if !strings.Contains(region, "-021039-") || strings.Contains(region, "-021009-") {
		t.Fatal("the rows listed are not the latest ones")
	}
	for _, want := range []string{`href="/?verdict=failed" class="pill">Failed <span class="tabular-nums">4<`, `href="/?verdict=passed" class="pill">Passed <span class="tabular-nums">36<`, `class="pill pill-on">All <span class="tabular-nums">40<`} {
		if !strings.Contains(region, want) {
			t.Errorf("the filters lack %q", want)
		}
	}
	if strings.Contains(region, "verdict=error") {
		t.Fatal("a verdict no run has is offered")
	}
	if n, _ := rows("?all=1"); n != 40 {
		t.Fatalf("all=1 shows %d rows", n)
	}
	n, failed := rows("?" + url.Values{"verdict": {"failed"}}.Encode())
	if n != 4 || strings.Contains(failed, ">Passed<") || strings.Count(failed, ">failed steps: 1, the first at alpha › a2<") != 4 {
		t.Fatalf("verdict=failed shows %d rows", n)
	}
	n, passed := rows("?verdict=passed")
	if n != endedShown || strings.Contains(passed, "failed steps: 1") || !strings.Contains(passed, "The latest 30 of 36 are listed.") {
		t.Fatalf("verdict=passed shows %d rows, or a run that passed carries a reason", n)
	}
	if n, _ := rows("?verdict=nonsense"); n != endedShown {
		t.Fatalf("an unknown verdict filters the list to %d rows", n)
	}
}
