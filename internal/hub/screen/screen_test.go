package screen

import (
	"context"
	"io/fs"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/sse"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/hub/ui"
)

var assetRef = regexp.MustCompile(`(?:src|href)="/assets/([^"]+)"`)

func TestEveryHubScreenAnswersAWholePageWithItsLiveRegionsUnderTheContentSecurityPolicy(t *testing.T) {
	f := newFixture(t)
	cases := map[string][]string{"/": {"rail", "runs"}, "/hub": {"rail", "hub"}, "/stacks": {"rail", "stacks"}}
	for path, regions := range cases {
		status, header, body := f.get(path)
		if status != http.StatusOK || header.Get("Content-Security-Policy") != csp || !strings.HasPrefix(header.Get("Content-Type"), "text/html") {
			t.Fatalf("%s: %d %v", path, status, header)
		}
		for _, id := range regions {
			if !strings.Contains(body, `id="`+id+`"`) {
				t.Errorf("%s has no region %s", path, id)
			}
		}
		want := `hx-ws:connect="/live?screen=` + strings.ReplaceAll(path, "/", "%2F") + `"`
		if !strings.Contains(body, want) {
			t.Errorf("%s does not connect to its live screen: want %s", path, want)
		}
	}
	if len(cases) != 3 {
		t.Fatal("case count")
	}
}

func TestTheLayoutLoadsOnlyEmbeddedAssets(t *testing.T) {
	f := newFixture(t)
	_, _, body := f.get("/")
	refs := assetRef.FindAllStringSubmatch(body, -1)
	for _, m := range refs {
		if _, err := fs.Stat(ui.Files(), m[1]); err != nil {
			t.Errorf("the layout loads %s, which is not embedded: %v", m[1], err)
		}
		status, header, _ := f.get("/assets/" + m[1])
		if status != http.StatusOK || header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("/assets/%s: %d %v", m[1], status, header)
		}
	}
	if len(refs) != 4 {
		t.Fatalf("the layout loads %d assets", len(refs))
	}
	if strings.Contains(body, "://") && !strings.Contains(body, "http://127.0.0.1:") {
		t.Fatalf("the layout names another origin:\n%s", body)
	}
}

func TestAnUnknownPathAnswersANotFoundScreenAndHasNoLiveScreen(t *testing.T) {
	f := newFixture(t)
	status, _, body := f.get("/nowhere")
	if status != http.StatusNotFound || !strings.Contains(body, "There is no screen at /nowhere.") {
		t.Fatalf("%d %q", status, body)
	}
	for _, path := range []string{"/nowhere", "", "/runs", "/hub/extra"} {
		if sc, err := f.sc.Resolve(context.Background(), path); err == nil || sc != nil {
			t.Errorf("%q resolved to %+v", path, sc)
		}
	}
	if sc, err := f.sc.Resolve(context.Background(), "/"); err != nil || sc == nil {
		t.Fatalf("canary: / does not resolve: %v", err)
	}
}

func TestTheRunsScreenResolvesToItsRegionsAndRendersOneAsAMorphingPartial(t *testing.T) {
	f := newFixture(t)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictPassed}, doneUp))
	sc, err := f.sc.Resolve(context.Background(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(sc.Regions, []string{"rail", "runs"}) || !slices.Equal(sc.Touches[sse.KindRuns], []string{"rail", "runs"}) || sc.Run != "" || len(sc.Signals) != 0 {
		t.Fatalf("%+v", sc)
	}
	out, err := sc.Render(context.Background(), []string{"runs"})
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.HasPrefix(text, `<hx-partial hx-target="#runs" hx-swap="outerMorph"><section id="runs"`) || strings.Contains(text, `id="rail"`) || !strings.Contains(text, rid(1)) {
		t.Fatalf("%s", text)
	}
}

func TestRunsAreGroupedIntoQueueKeptAndEndedAndLinkToTheirDefaultScreen(t *testing.T) {
	f := newFixture(t)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Queued, Env: lifecycle.EnvAbsent}))
	f.seed(runMeta(2, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictFailed}, doneUp))
	f.seed(runMeta(3, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvDestroyed, Verdict: lifecycle.VerdictPassed}, func(m *store.Meta) {
		m.FinishedAt = "2026-10-19T01:00:09.000Z"
	}))
	_, _, body := f.get("/")
	queue, kept, ended := strings.Index(body, "Queue and active run"), strings.Index(body, ">Kept<"), strings.Index(body, ">Ended<")
	if queue < 0 || kept < queue || ended < kept {
		t.Fatalf("group order %d %d %d", queue, kept, ended)
	}
	for _, want := range []string{
		`href="/runs/` + rid(1) + `"`,
		`href="/runs/` + rid(2) + `/report"`,
		`href="/runs/` + rid(3) + `/report"`,
		"next to destroy", "next to stop", "only the report remains",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if !(strings.Index(body, rid(1)) > queue && strings.Index(body, rid(1)) < kept) || !(strings.Index(body, rid(2)) > kept && strings.Index(body, rid(2)) < ended) || strings.Index(body, rid(3)) < ended {
		t.Fatal("a run sits in the wrong group")
	}
}

func TestOtherStacksListsForeignAndUnownedProjectsAndLinksOwnedOnesToTheirHub(t *testing.T) {
	st := &state{list: api.HubList{
		Foreign: []string{"toy-shop"},
		Orphans: []api.OrphanItem{
			{Orphan: reconcile.Orphan{Project: rid(7), Running: true}},
			{Orphan: reconcile.Orphan{Project: rid(8)}, Hub: "abc123abc123"},
		},
		Hubs: []api.HubItem{{ID: "self00000000", Name: "here", Self: true, Up: true}, {ID: "abc123abc123", Name: "other", Up: true, URL: "http://127.0.0.1:7799"}},
	}}
	out := render(t, stacksRegion(st))
	for _, want := range []string{
		`data-copy="docker compose -p toy-shop down"`,
		`data-copy="docker compose -p ` + rid(7) + ` down -v --remove-orphans"`,
		`href="http://127.0.0.1:7799/"`, ">other<",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "docker compose -p "+rid(8)) {
		t.Fatal("a run another hub owns gets a removal command")
	}
	if got := stackCount(st.list); got != 2 {
		t.Fatalf("stack count %d", got)
	}
}

func TestInitialsTakeTwoLettersFromOneWordAndFirstLettersFromSeveral(t *testing.T) {
	cases := map[string]string{"cockpit": "CO", "hubA": "HA", "hub-b": "HB", "team 2": "T2", "x": "X", "": "?", "my hub here": "MH"}
	for name, want := range cases {
		if got := initials(name); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
	if len(cases) != 7 {
		t.Fatal("case count")
	}
}
