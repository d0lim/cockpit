package compose

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var renderMutations = []porttest.Mutation{
	{"every role is present exactly once before any assertion runs", "make AssertRoles return nil", "red: configs without a builder, with two runners or with an unlabeled service pass", "TestRenderedConfigWithEveryRolePassesAndMissingDuplicatedOrUnlabeledRolesFail"},
	{"compose config failures are masked", "build the failure message from raw stderr", "red: the planted value appears in the error", "TestComposeConfigFailureIsMaskedAndKeepsTheExitCode"},
}

func TestRenderMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "render_test.go", renderMutations)
}

func rendered(builder string, roles map[string]string) Rendered {
	r := Rendered{Services: map[string]RenderedService{}}
	for name, role := range roles {
		l := map[string]string{LabelManaged: "true", LabelRunID: runID}
		if role != "" {
			l[LabelRole] = role
		}
		r.Services[name] = RenderedService{Labels: l}
	}
	return r
}

func allRoles() map[string]string {
	return map[string]string{"edge": RoleEdge, "runner": RoleRunner, "probe": RoleProbe, "probe-control": RoleProbeControl, "builder": RoleBuilder, "app": ""}
}

func TestRenderedConfigWithEveryRolePassesAndMissingDuplicatedOrUnlabeledRolesFail(t *testing.T) {
	if err := AssertRoles(rendered("builder", allRoles()), runID, "builder"); err != nil {
		t.Fatalf("complete config failed: %v", err)
	}
	cases := map[string]func(m map[string]string) (Rendered, string){
		"builder missing": func(m map[string]string) (Rendered, string) {
			delete(m, "builder")
			return rendered("builder", m), "builder"
		},
		"edge missing": func(m map[string]string) (Rendered, string) { delete(m, "edge"); return rendered("builder", m), "edge" },
		"two runners": func(m map[string]string) (Rendered, string) {
			m["runner-2"] = RoleRunner
			return rendered("builder", m), "runner (found 2)"
		},
		"builder unlabeled": func(m map[string]string) (Rendered, string) {
			m["builder"] = ""
			return rendered("builder", m), "builder"
		},
		"foreign run label": func(m map[string]string) (Rendered, string) {
			r := rendered("builder", m)
			r.Services["app"] = RenderedService{Labels: map[string]string{LabelManaged: "true", LabelRunID: "r-other"}}
			return r, "app"
		},
	}
	for name, build := range cases {
		r, says := build(allRoles())
		err := AssertRoles(r, runID, "builder")
		if err == nil || !strings.Contains(err.Error(), says) {
			t.Errorf("%s: %v", name, err)
		}
	}
	noBuilder := allRoles()
	delete(noBuilder, "builder")
	if err := AssertRoles(rendered("", noBuilder), runID, ""); err != nil {
		t.Fatalf("a project without a builder failed: %v", err)
	}
}

type fakeDocker struct {
	calls [][]string
	run   func(cmd Command) (Result, error)
}

func (f *fakeDocker) Run(ctx context.Context, cmd Command) (Result, error) {
	f.calls = append(f.calls, cmd.Args)
	return f.run(cmd)
}

func TestComposeConfigFailureIsMaskedAndKeepsTheExitCode(t *testing.T) {
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{planted}})
	if err != nil {
		t.Fatal(err)
	}
	d := &fakeDocker{run: func(Command) (Result, error) {
		return Result{Code: 15, Stderr: []byte("invalid interpolation\nPOSTGRES_PASSWORD=" + planted + " is not allowed\n")}, nil
	}}
	_, err = Render(context.Background(), d, []string{"compose", "-p", runID}, m.Mask)
	if err == nil || strings.Contains(err.Error(), planted) || !strings.Contains(err.Error(), "****") || !strings.Contains(err.Error(), "exit code 15") {
		t.Fatalf("error %v", err)
	}
	ok := &fakeDocker{run: func(Command) (Result, error) {
		body, _ := json.Marshal(rendered("builder", allRoles()))
		return Result{Stdout: body}, nil
	}}
	r, err := Render(context.Background(), ok, []string{"compose"}, m.Mask)
	if err != nil || len(r.Services) != len(allRoles()) {
		t.Fatalf("render %v %v", r, err)
	}
	if got := strings.Join(ok.calls[0], " "); got != "compose --profile * config --format json" {
		t.Fatalf("args %s", got)
	}
}
