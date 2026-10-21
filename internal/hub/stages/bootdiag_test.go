package stages

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const plantedBoot = "PLANTED-boot-secret-7f3a"

var bootDiagMutations = []porttest.Mutation{
	{"a failed boot's service log lines pass the run redactor", "write the log lines with e.Log instead of e.logLine", "red: the planted secret reaches the tail", "TestAFailedComposeUpNamesTheServiceThatExitedAndKeepsItsMaskedLogInTheTail"},
	{"only broken services are diagnosed after a failed up", "pick every service, broken or not", "red: the running db is diagnosed and its log read", "TestAFailedComposeUpNamesTheServiceThatExitedAndKeepsItsMaskedLogInTheTail"},
	{"compose progress lines leave the notice", "drop the withoutProgress call in Project.run", "red: the notice holds the Creating and Created lines", "TestAComposeUpFailureWithoutABrokenServiceKeepsComposesMessageWithoutProgressLines"},
	{"a boot out of time reads the expected services still not ready", "return no culprits when none is broken", "red: the starting app's log is not read", "TestABootThatRunsOutOfTimeReadsTheLogsOfTheExpectedServicesStillNotReady"},
	{"culprits come in declaration order and stop at three", "drop the diagServices limit", "red: d is a fourth culprit", "TestCulpritsPutExpectedServicesFirstAndStopAtThree"},
}

func TestBootDiagnosisMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "bootdiag_test.go", bootDiagMutations)
}

type bootDocker struct {
	upErr   string
	rows    []compose.PsRow
	logs    map[string]string
	read    []string
	upCalls int
}

func (d *bootDocker) Run(_ context.Context, cmd compose.Command) (compose.Result, error) {
	switch {
	case slices.Contains(cmd.Args, "up"):
		d.upCalls++
		if d.upErr != "" {
			return compose.Result{Code: 1, Stderr: []byte(d.upErr)}, nil
		}
	case slices.Contains(cmd.Args, "ps"):
		data, _ := json.Marshal(d.rows)
		return compose.Result{Stdout: data}, nil
	case slices.Contains(cmd.Args, "logs"):
		service := cmd.Args[len(cmd.Args)-1]
		d.read = append(d.read, service)
		return compose.Result{Stdout: []byte(d.logs[service])}, nil
	case slices.Contains(cmd.Args, "config"):
		return compose.Result{Stdout: []byte(`{"services":{"app":{},"db":{},"migrate":{"restart":"no"}}}`)}, nil
	}
	return compose.Result{}, nil
}

func bootEnv(t *testing.T, d *bootDocker) (*Env, *[]string) {
	t.Helper()
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: []string{plantedBoot}})
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	e := &Env{
		Masker: m, Log: func(line string) { lines = append(lines, line) },
		Limits: Limits{Boot: 30 * time.Millisecond, Poll: 5 * time.Millisecond},
		Loaded: compose.Loaded{Declaration: contract.Declaration{Project: contract.ProjectSection{ExpectedServices: []string{"db", "migrate", "app"}}}},
	}
	e.Project = compose.Project{Docker: d, Name: "run-boot", Args: []string{"compose", "-p", "run-boot"}, Mask: m.Mask}
	return e, &lines
}

func psRow(service, state, health string, code int) compose.PsRow {
	return compose.PsRow{Service: service, Name: "run-boot-" + service + "-1", State: state, Health: health, ExitCode: code}
}

func TestAFailedComposeUpNamesTheServiceThatExitedAndKeepsItsMaskedLogInTheTail(t *testing.T) {
	d := &bootDocker{
		upErr: " Container run-boot-db-1  Created\n Container run-boot-migrate-1  Started\n Container run-boot-db-1  Healthy\n" +
			"service \"migrate\" didn't complete successfully: exit 1\n",
		rows: []compose.PsRow{psRow("db", "running", "healthy", 0), psRow("migrate", "exited", "", 1), psRow("app", "created", "", 0)},
		logs: map[string]string{
			"migrate": "migrate-1  | connecting with " + plantedBoot + "\nmigrate-1  | access denied for user library\n",
			"db":      "db-1  | ready for connections\n",
		},
	}
	e, lines := bootEnv(t, d)
	err := e.Boot(context.Background())
	if err == nil || err.Error() != "compose up failed: migrate exited with code 1" {
		t.Fatalf("notice %v", err)
	}
	if !slices.Equal(d.read, []string{"migrate"}) {
		t.Fatalf("logs read for %v", d.read)
	}
	all := strings.Join(*lines, "\n")
	if strings.Contains(all, plantedBoot) || !strings.Contains(all, "migrate-1  | connecting with ****") || !strings.Contains(all, "access denied for user library") {
		t.Fatalf("tail lines:\n%s", all)
	}
}

func TestAComposeUpFailureWithoutABrokenServiceKeepsComposesMessageWithoutProgressLines(t *testing.T) {
	d := &bootDocker{
		upErr: " Container run-boot-db-1  Creating\n Network run-boot_default  Created\nError response from daemon: port is already allocated\n",
		rows:  []compose.PsRow{psRow("db", "created", "", 0)},
	}
	e, _ := bootEnv(t, d)
	err := e.Boot(context.Background())
	if err == nil || !strings.Contains(err.Error(), "port is already allocated") || strings.Contains(err.Error(), "Creating") || strings.Contains(err.Error(), "Created") {
		t.Fatalf("notice %v", err)
	}
	if len(d.read) != 0 {
		t.Fatalf("logs read for %v", d.read)
	}
}

func TestABootThatRunsOutOfTimeReadsTheLogsOfTheExpectedServicesStillNotReady(t *testing.T) {
	d := &bootDocker{
		rows: []compose.PsRow{psRow("db", "running", "healthy", 0), psRow("migrate", "exited", "", 0), psRow("app", "running", "starting", 0)},
		logs: map[string]string{"app": "app-1  | waiting for the cache\n"},
	}
	e, lines := bootEnv(t, d)
	err := e.Boot(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not ready within") {
		t.Fatalf("notice %v", err)
	}
	if !slices.Equal(d.read, []string{"app"}) || !strings.Contains(strings.Join(*lines, "\n"), "waiting for the cache") {
		t.Fatalf("logs read for %v, lines %v", d.read, *lines)
	}
}

func TestCulpritsPutExpectedServicesFirstAndStopAtThree(t *testing.T) {
	rows := []compose.PsRow{
		psRow("extra", "exited", "", 2), psRow("c", "dead", "", 0), psRow("b", "running", "unhealthy", 0),
		psRow("a", "exited", "", 137), psRow("d", "exited", "", 1), psRow("ok", "exited", "", 0),
	}
	got := Culprits(rows, []string{"a", "b", "c", "d", "ok"}, false)
	var texts []string
	for _, r := range got {
		texts = append(texts, CulpritText(r))
	}
	if !slices.Equal(texts, []string{"a exited with code 137", "b is unhealthy", "c died"}) {
		t.Fatalf("culprits %v", texts)
	}
	if got := Culprits([]compose.PsRow{psRow("ok", "exited", "", 0)}, []string{"ok"}, true); len(got) != 0 {
		t.Fatalf("a one-shot that exited 0 is a culprit: %v", got)
	}
}
