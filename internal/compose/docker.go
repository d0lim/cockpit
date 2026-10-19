package compose

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	dockerWaitDelay   = 10 * time.Second
	dockerStderrBytes = 64 << 10
)

var dockerPassEnv = []string{"PATH", "HOME", "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CONFIG", "DOCKER_CERT_PATH", "DOCKER_TLS_VERIFY", "TMPDIR"}

type Command struct {
	Args   []string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
}

type Result struct {
	Stdout []byte
	Stderr []byte
	Code   int
}

type Docker interface {
	Run(ctx context.Context, cmd Command) (Result, error)
}

type CLI struct {
	Bin string
}

func dockerEnv(extra []string) []string {
	env := []string{"DOCKER_CLI_HINTS=false"}
	for _, name := range dockerPassEnv {
		if v, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+v)
		}
	}
	return append(env, extra...)
}

func (c CLI) Run(ctx context.Context, in Command) (Result, error) {
	bin := c.Bin
	if bin == "" {
		bin = "docker"
	}
	cmd := exec.CommandContext(ctx, bin, in.Args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = dockerWaitDelay
	cmd.Env = dockerEnv(in.Env)
	cmd.Stdin = in.Stdin
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if in.Stdout != nil {
		cmd.Stdout = in.Stdout
	}
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{buf: &stderr, max: dockerStderrBytes}
	err := cmd.Run()
	res := Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	var exit *exec.ExitError
	switch {
	case err == nil:
		return res, nil
	case ctx.Err() != nil:
		return res, ctx.Err()
	case errors.As(err, &exit):
		res.Code = exit.ExitCode()
		return res, nil
	}
	return res, err
}
