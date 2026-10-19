package procgroup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	DefaultGrace = 5 * time.Second
	pollEvery    = 25 * time.Millisecond
)

type Hooks struct {
	OnSpawn func(pgid int)
	OnExit  func()
	Grace   time.Duration
}

func (h Hooks) grace() time.Duration {
	if h.Grace <= 0 {
		return DefaultGrace
	}
	return h.Grace
}

func Run(cmd *exec.Cmd, h Hooks) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return signal(cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = h.grace()
	if err := cmd.Start(); err != nil {
		return err
	}
	pgid := cmd.Process.Pid
	if h.OnSpawn != nil {
		h.OnSpawn(pgid)
	}
	err := cmd.Wait()
	signal(pgid, syscall.SIGKILL)
	if h.OnExit != nil {
		h.OnExit()
	}
	return err
}

func signal(pgid int, sig syscall.Signal) error {
	err := syscall.Kill(-pgid, sig)
	if errors.Is(err, syscall.ESRCH) || errors.Is(err, syscall.EPERM) {
		return nil
	}
	return err
}

func alive(pgid int) bool {
	err := syscall.Kill(-pgid, 0)
	return err == nil
}

func validGroup(pgid int) error {
	own, _ := syscall.Getpgid(0)
	if pgid <= 1 || pgid == os.Getpid() || pgid == own {
		return fmt.Errorf("refusing to signal process group %d", pgid)
	}
	return nil
}

func KillGroup(ctx context.Context, pgid int, grace time.Duration) error {
	if err := validGroup(pgid); err != nil {
		return err
	}
	if !alive(pgid) {
		return nil
	}
	if err := signal(pgid, syscall.SIGTERM); err != nil {
		return err
	}
	deadline := time.Now().Add(grace)
	for alive(pgid) && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return signal(pgid, syscall.SIGKILL)
		case <-time.After(pollEvery):
		}
	}
	if alive(pgid) {
		return signal(pgid, syscall.SIGKILL)
	}
	return nil
}
