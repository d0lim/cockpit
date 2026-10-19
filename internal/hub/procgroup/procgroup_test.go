package procgroup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var procgroupMutations = []porttest.Mutation{
	{"a cancelled command takes its whole process group with it", "drop Setpgid and signal only the leader", "red: the grandchild sleep survives the cancel", "TestCancelledCommandTakesItsGrandchildrenAlongAndPlainExecDoesNot"},
	{"the pgid is reported at spawn and cleared at exit", "skip the OnSpawn or OnExit call", "red: no pgid is recorded or it is never cleared", "TestSpawnReportsTheGroupIDAndExitClearsIt"},
	{"only foreign groups above 1 are signalled", "drop the validGroup call in KillGroup", "red: KillGroup returns nil for the hub's own pid (observed before this test shape: with 0 in the list the unguarded call signalled the test's own group and killed the run)", "TestKillGroupRefusesTheHubAndInitAndEndsALeftOverGroup"},
}

func TestProcgroupMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "procgroup_test.go", procgroupMutations)
}

func grandchild(t *testing.T, dir string) (*exec.Cmd, string) {
	pidFile := filepath.Join(dir, "pid")
	return exec.Command("sh", "-c", "sleep 30 & echo $! > "+pidFile+"; wait"), pidFile
}

func readPid(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(data)) != "" {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the grandchild pid never appeared")
	return 0
}

func running(pid int) bool { return syscall.Kill(pid, 0) == nil }

func TestCancelledCommandTakesItsGrandchildrenAlongAndPlainExecDoesNot(t *testing.T) {
	plainCtx, plainCancel := context.WithCancel(context.Background())
	plain, plainPid := grandchild(t, t.TempDir())
	plainCmd := exec.CommandContext(plainCtx, plain.Path, plain.Args[1:]...)
	plainCmd.WaitDelay = 100 * time.Millisecond
	if err := plainCmd.Start(); err != nil {
		t.Fatal(err)
	}
	survivor := readPid(t, plainPid)
	plainCancel()
	plainCmd.Wait()
	if !running(survivor) {
		t.Fatal("canary: plain exec already killed the grandchild, so the test proves nothing")
	}
	syscall.Kill(survivor, syscall.SIGKILL)
	ctx, cancel := context.WithCancel(context.Background())
	base, pidFile := grandchild(t, t.TempDir())
	cmd := exec.CommandContext(ctx, base.Path, base.Args[1:]...)
	done := make(chan error, 1)
	go func() { done <- Run(cmd, Hooks{Grace: 200 * time.Millisecond}) }()
	victim := readPid(t, pidFile)
	cancel()
	<-done
	deadline := time.Now().Add(2 * time.Second)
	for running(victim) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if running(victim) {
		syscall.Kill(victim, syscall.SIGKILL)
		t.Fatal("the grandchild survived the cancelled group")
	}
}

func TestSpawnReportsTheGroupIDAndExitClearsIt(t *testing.T) {
	var spawned, exited int
	recorded := -1
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "exit 0")
	err := Run(cmd, Hooks{OnSpawn: func(pgid int) { spawned++; recorded = pgid }, OnExit: func() { exited++; recorded = 0 }})
	if err != nil || spawned != 1 || exited != 1 || recorded != 0 {
		t.Fatalf("err %v spawned %d exited %d recorded %d", err, spawned, exited, recorded)
	}
	var pgid int
	cmd = exec.CommandContext(context.Background(), "sleep", "0.1")
	Run(cmd, Hooks{OnSpawn: func(p int) { pgid = p }})
	if pgid != cmd.Process.Pid || pgid <= 1 {
		t.Fatalf("pgid %d pid %d", pgid, cmd.Process.Pid)
	}
}

func TestKillGroupRefusesTheHubAndInitAndEndsALeftOverGroup(t *testing.T) {
	own, _ := syscall.Getpgid(0)
	refused := 0
	for _, pgid := range []int{0, 1, -5, os.Getpid(), own} {
		if validGroup(pgid) == nil {
			t.Errorf("pgid %d was accepted", pgid)
		}
		refused++
	}
	if refused != 5 {
		t.Fatalf("checked %d", refused)
	}
	if err := KillGroup(context.Background(), os.Getpid(), time.Second); err == nil {
		t.Fatal("KillGroup accepted the hub's own pid without checking it")
	}
	cmd := exec.Command("sh", "-c", "sleep 30 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pgid := cmd.Process.Pid
	go cmd.Wait()
	if err := KillGroup(context.Background(), pgid, 500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for alive(pgid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if alive(pgid) {
		syscall.Kill(-pgid, syscall.SIGKILL)
		t.Fatal("the left-over group survived")
	}
	if err := KillGroup(context.Background(), pgid, time.Second); err != nil {
		t.Fatalf("killing a gone group failed: %v", err)
	}
}
