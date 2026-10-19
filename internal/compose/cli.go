package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
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

func ParseJSONRows(data []byte) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if trimmed[0] == '[' {
		var rows []json.RawMessage
		if err := json.Unmarshal(trimmed, &rows); err != nil {
			return nil, fmt.Errorf("cannot parse the JSON array output: %w", err)
		}
		return rows, nil
	}
	var rows []json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	for {
		var row json.RawMessage
		err := dec.Decode(&row)
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return nil, fmt.Errorf("cannot parse the JSON Lines output: %w", err)
		}
		rows = append(rows, row)
	}
}

var minCompose = [3]int{2, 24, 0}

func parseVersion(raw string) ([3]int, error) {
	var v [3]int
	s := strings.TrimPrefix(strings.TrimSpace(raw), "v")
	s, _, _ = strings.Cut(s, "-")
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return v, fmt.Errorf("cannot read version %q", raw)
	}
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return v, fmt.Errorf("cannot read version %q", raw)
		}
		v[i] = n
	}
	return v, nil
}

func CheckVersions(ctx context.Context, d Docker) error {
	server, err := d.Run(ctx, Command{Args: []string{"version", "--format", "{{.Server.Version}}"}})
	if err != nil || server.Code != 0 {
		return fmt.Errorf("cannot reach the docker daemon (docker version exit code %d): %v", server.Code, err)
	}
	res, err := d.Run(ctx, Command{Args: []string{"compose", "version", "--short"}})
	if err != nil || res.Code != 0 {
		return fmt.Errorf("docker compose is not available (exit code %d): %v", res.Code, err)
	}
	got, err := parseVersion(string(res.Stdout))
	if err != nil {
		return err
	}
	for i := range got {
		if got[i] != minCompose[i] {
			if got[i] < minCompose[i] {
				return fmt.Errorf("docker compose %s is older than %d.%d.%d, which the harness needs for !reset and !override", strings.TrimSpace(string(res.Stdout)), minCompose[0], minCompose[1], minCompose[2])
			}
			break
		}
	}
	return nil
}
