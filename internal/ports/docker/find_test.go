package dockerport

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var findMutations = []porttest.Mutation{
	{"containers outside the run are refused", "drop the project and run-id comparison in list", "red: logs of intruder or stale come back", "TestLogsOfAServiceInThisRunReturnAndServicesOutsideTheRunOrInTheHarnessArePortDenied"},
	{"harness roles are refused by name and by label", "remove the harness map and the role check in target", "red: logs of edge, probe, toolbox or sneaky come back", "TestLogsOfAServiceInThisRunReturnAndServicesOutsideTheRunOrInTheHarnessArePortDenied"},
	{"the listing filters by the run labels", "drop the label filters from the ps arguments", "red: the ps call lacks the project or run-id filter", "TestPSListsOnlyThisRunsTargetServicesWithTheirHealth"},
	{"ps hides the harness and other runs", "return every listed container from PS", "red: edge, intruder or stale appear in ps", "TestPSListsOnlyThisRunsTargetServicesWithTheirHealth"},
	{"the last listed container keeps its empty role column", "TrimSpace the ps output before splitting lines", "red: the last target service is missing from ps and Containers", "TestLastListedTargetContainerWithAnEmptyRoleColumnIsKept"},
	{"service names are compose names", "remove the serviceName check", "red: a name with a slash or a flag reaches docker", "TestServiceNamesThatAreNotComposeNamesArePortBadArgs"},
}

func TestLogsOfAServiceInThisRunReturnAndServicesOutsideTheRunOrInTheHarnessArePortDenied(t *testing.T) {
	f := &fakeDocker{handle: func(_ context.Context, cmd compose.Command) (compose.Result, error) {
		write(cmd, "line from "+cmd.Args[len(cmd.Args)-1]+"\n", "")
		return compose.Result{}, nil
	}}
	p, _ := newPort(t, f, Options{})
	out, err := p.Logs(context.Background(), LogsRequest{Service: "app"})
	if err != nil || out.Text != "line from c-app\n" || out.Truncated {
		t.Fatalf("%+v %v", out, err)
	}
	denied := []string{"intruder", "stale", "edge", "probe", "runner", "probe-control", "builder", "toolbox", "sneaky", "missing"}
	ran := 0
	for _, svc := range denied {
		_, err := p.Logs(context.Background(), LogsRequest{Service: svc})
		if codeOf(err) != engine.PortDenied || !strings.Contains(err.Error(), `"`+svc+`"`) {
			t.Errorf("%s: %v", svc, err)
		}
		ran++
	}
	if ran != 10 || ran != len(denied) {
		t.Fatalf("ran %d cases", ran)
	}
	if logs := f.commands("logs"); len(logs) != 1 || logs[0][len(logs[0])-1] != "c-app" {
		t.Fatalf("docker logs calls: %v", logs)
	}
}

func TestPSListsOnlyThisRunsTargetServicesWithTheirHealth(t *testing.T) {
	f := &fakeDocker{}
	p, _ := newPort(t, f, Options{})
	got, err := p.PS(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	three := 3
	want := []ServiceState{{"app", "running", "healthy", nil}, {"db", "running", "starting", nil}, {"worker", "exited", "", &three}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
	ps := f.commands("ps")
	if len(ps) != 1 || !slices.Contains(ps[0], "label=com.docker.compose.project="+project) || !slices.Contains(ps[0], "label="+compose.LabelRunID+"="+runID) {
		t.Fatalf("ps arguments: %v", ps)
	}
}

func TestServiceNamesThatAreNotComposeNamesArePortBadArgs(t *testing.T) {
	f := &fakeDocker{handle: func(_ context.Context, cmd compose.Command) (compose.Result, error) { return compose.Result{}, nil }}
	p, _ := newPort(t, f, Options{})
	bad := []string{"", "../app", "app/x", "-f", "app name", strings.Repeat("a", 64)}
	for _, svc := range bad {
		if _, err := p.Stats(context.Background(), svc); codeOf(err) != engine.PortBadArgs {
			t.Errorf("%q: %v", svc, err)
		}
	}
	if len(f.calls) != 0 {
		t.Fatalf("docker was called: %v (%d cases)", f.calls, len(bad))
	}
	if _, err := p.Logs(context.Background(), LogsRequest{Service: "app.v2-x_1"}); codeOf(err) != engine.PortDenied {
		t.Fatalf("a valid name reaches the lookup: %v", err)
	}
}

func TestLastListedTargetContainerWithAnEmptyRoleColumnIsKept(t *testing.T) {
	f := &fakeDocker{ps: []string{
		psLine("c-edge", "edge", "running", "Up", project, runID, compose.RoleEdge),
		psLine("c-app", "app", "running", "Up", project, runID, ""),
	}}
	p, _ := newPort(t, f, Options{})
	states, err := p.PS(context.Background())
	if err != nil || len(states) != 1 || states[0].Service != "app" {
		t.Fatalf("%+v %v", states, err)
	}
	cs, err := p.Containers(context.Background())
	if err != nil || len(cs) != 1 || cs[0] != (Container{ID: "c-app", Service: "app", State: "running"}) {
		t.Fatalf("%+v %v", cs, err)
	}
}

func TestFindMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "find_test.go", findMutations)
}
