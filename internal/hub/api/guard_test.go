package api

import (
	"net/http"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var guardMutations = []porttest.Mutation{
	{"the Host header must be a loopback name with the hub port", "skip the hosts check in Guard", "red: a request with Host evil.example:port is served", "TestRequestsWithAnotherHostAreRefusedOnEveryMethod"},
	{"writes need a loopback Origin", "skip the origins check in Guard", "red: a POST without Origin changes keep", "TestWritesWithoutOriginOrWithoutTheCockpitHeaderAreRefusedAndChangeNothing"},
	{"writes need X-Cockpit: 1", "skip the header check in Guard", "red: a POST without the header changes keep", "TestWritesWithoutOriginOrWithoutTheCockpitHeaderAreRefusedAndChangeNothing"},
	{"browser cross-site writes are refused by CrossOriginProtection", "serve next instead of protected for writes", "red: a cross-site fetch with a forged Origin changes keep", "TestACrossSiteBrowserWriteIsRefusedEvenWithALoopbackOrigin"},
}

func TestGuardMutationRecordNamesEveryTestInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "guard_test.go", guardMutations)
}

func guardFixture(t *testing.T) *fixture {
	f := newFixture(t, nil)
	f.seed(runMeta(1, lifecycle.State{Phase: lifecycle.Done, Env: lifecycle.EnvUp, Verdict: lifecycle.VerdictPassed}, doneUp))
	return f
}

func (f *fixture) kept() bool {
	return f.sched.Get(rid(1)).Keep
}

func TestRequestsWithAnotherHostAreRefusedOnEveryMethod(t *testing.T) {
	f := guardFixture(t)
	for _, host := range []string{"evil.example:" + itoa(f.port), "127.0.0.1:1", "[::1]:" + itoa(f.port)} {
		req, _ := http.NewRequest(http.MethodGet, f.srv.URL+"/api/runs", nil)
		req.Host = host
		res, err := f.srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("%s: %d", host, res.StatusCode)
		}
		post, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/api/runs/"+rid(1)+"/keep", nil)
		post.Host, post.Header = host, f.cli()
		res, err = f.srv.Client().Do(post)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden || f.kept() {
			t.Errorf("%s POST: %d, kept %v", host, res.StatusCode, f.kept())
		}
	}
	for _, host := range LoopbackHosts(f.port) {
		req, _ := http.NewRequest(http.MethodGet, f.srv.URL+"/api/runs", nil)
		req.Host = host
		res, err := f.srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("allowed host %s: %d", host, res.StatusCode)
		}
	}
}

func TestWritesWithoutOriginOrWithoutTheCockpitHeaderAreRefusedAndChangeNothing(t *testing.T) {
	f := guardFixture(t)
	path := "/api/runs/" + rid(1) + "/keep"
	denied := map[string]http.Header{
		"no origin":      {HeaderCockpit: {"1"}},
		"foreign origin": {"Origin": {"http://evil.example"}, HeaderCockpit: {"1"}},
		"no header":      {"Origin": {f.origin}},
		"header not 1":   {"Origin": {f.origin}, HeaderCockpit: {"true"}},
	}
	for name, h := range denied {
		status, raw := f.do(http.MethodPost, path, map[string]any{"submitter": "cli"}, h)
		if status != http.StatusForbidden || f.kept() {
			t.Errorf("%s: %d %s, kept %v", name, status, raw, f.kept())
		}
	}
	status, raw := f.post(path, map[string]any{"submitter": "cli"})
	if status != http.StatusOK || !f.kept() {
		t.Fatalf("CLI headers: %d %s", status, raw)
	}
	if len(denied) != 4 {
		t.Fatal("case count")
	}
}

func TestACrossSiteBrowserWriteIsRefusedEvenWithALoopbackOrigin(t *testing.T) {
	f := guardFixture(t)
	path := "/api/runs/" + rid(1) + "/keep"
	h := f.cli()
	h.Set("Sec-Fetch-Site", "cross-site")
	status, raw := f.do(http.MethodPost, path, map[string]any{"submitter": "cli"}, h)
	if status != http.StatusForbidden || f.kept() {
		t.Fatalf("cross-site: %d %s, kept %v", status, raw, f.kept())
	}
	h.Set("Sec-Fetch-Site", "same-origin")
	if status, raw := f.do(http.MethodPost, path, map[string]any{"submitter": "cli"}, h); status != http.StatusOK || !f.kept() {
		t.Fatalf("same-origin: %d %s", status, raw)
	}
}
