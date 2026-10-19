package compose

import (
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var runnerUserMutations = []porttest.Mutation{
	{"on Linux the runner runs as the hub uid:gid so it can write the 0700 work/e2e", "make RunnerUser always return an empty string", "red: the Linux harness carries no runner user", "TestRunnerRunsAsTheHubUserOnLinuxAndKeepsTheImageUserElsewhere"},
	{"the runner never runs as root", "return uid:gid for uid 0", "red: a root hub yields user 0:0", "TestRunnerRunsAsTheHubUserOnLinuxAndKeepsTheImageUserElsewhere"},
}

func TestRunnerUserMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "runneruser_test.go", runnerUserMutations)
}

func TestRunnerRunsAsTheHubUserOnLinuxAndKeepsTheImageUserElsewhere(t *testing.T) {
	cases := []struct {
		goos     string
		uid, gid int
		want     string
	}{
		{"linux", 1000, 1000, "1000:1000"},
		{"linux", 501, 20, "501:20"},
		{"linux", 0, 0, ""},
		{"darwin", 501, 20, ""},
	}
	for _, c := range cases {
		if got := RunnerUser(c.goos, c.uid, c.gid); got != c.want {
			t.Errorf("RunnerUser(%s, %d, %d) = %q, want %q", c.goos, c.uid, c.gid, got, c.want)
		}
	}
	loaded := mustLoadToy(t)
	in := HarnessInput{RunID: runID, RunnerImage: "cockpit-runner:0123456789ab", PortBase: 22100, Layout: testLayout(t), Loaded: loaded}
	for _, user := range []string{"1000:1000", ""} {
		in.RunnerUser = user
		doc, _, _ := harnessDocFrom(t, in)
		got, ok := service(t, doc, RoleRunner)["user"]
		if user == "" && ok || user != "" && got != user {
			t.Errorf("RunnerUser %q: runner user %v %v", user, got, ok)
		}
		if _, ok := service(t, doc, RoleEdge)["user"]; ok {
			t.Error("the edge got a user")
		}
	}
}
