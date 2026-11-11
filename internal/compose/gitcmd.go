package compose

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	gitTimeout      = 2 * time.Minute
	gitStderrBytes  = 2048
	maxBlobBytes    = 1 << 20
	symlinkMode     = "120000"
	treeType        = "tree"
	submoduleType   = "commit"
	gitEnvPrefix    = "GIT_"
	hooksDirPattern = "cockpit-nohooks-*"
)

var shaPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

var ErrCommitInput = errors.New("commit is not a valid revision")

type Git struct {
	Dir string
}

func HardenedArgs(dir, hooksDir string, args ...string) []string {
	return append([]string{
		"-c", "core.fsmonitor=false", "-c", "core.hooksPath=" + hooksDir,
		"-c", "maintenance.auto=false", "-c", "maintenance.autoDetach=false", "-c", "gc.auto=0", "-c", "gc.autoDetach=false",
		"--no-optional-locks", "-C", dir,
	}, args...)
}

func gitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, gitEnvPrefix) {
			env = append(env, kv)
		}
	}
	return append(env, "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
}

func (g Git) Run(ctx context.Context, args ...string) ([]byte, error) {
	var out bytes.Buffer
	if err := g.Stream(ctx, &out, args...); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (g Git) Stream(ctx context.Context, stdout io.Writer, args ...string) error {
	return g.stream(ctx, stdout, nil, args)
}

func (g Git) stream(ctx context.Context, stdout io.Writer, global, args []string) error {
	if !filepath.IsAbs(g.Dir) {
		return fmt.Errorf("git directory must be absolute, got %q", g.Dir)
	}
	hooks, err := os.MkdirTemp("", hooksDirPattern)
	if err != nil {
		return err
	}
	defer os.RemoveAll(hooks)
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", HardenedArgs(g.Dir, hooks, slices.Concat(global, args)...)...)
	cmd.Env = gitEnv()
	cmd.Stdout = stdout
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{buf: &stderr, max: gitStderrBytes}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

type limitedWriter struct {
	buf *bytes.Buffer
	max int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if room := w.max - w.buf.Len(); room > 0 {
		w.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

func line(out []byte) string {
	return strings.TrimRight(string(out), "\n")
}

func (g Git) CommonDir(ctx context.Context) (string, error) {
	out, err := g.Run(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(line(out))
}

func CommonDirOf(ctx context.Context, dir string) (string, error) {
	return Git{Dir: dir}.CommonDir(ctx)
}

func (g Git) TopLevel(ctx context.Context) (string, error) {
	out, err := g.Run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(line(out))
}

func (g Git) ResolveCommit(ctx context.Context, ref string) (string, error) {
	if strings.TrimSpace(ref) == "" || strings.HasPrefix(strings.TrimSpace(ref), "-") {
		return "", fmt.Errorf("%w: %q", ErrCommitInput, ref)
	}
	out, err := g.Run(ctx, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	sha := line(out)
	if err != nil || !shaPattern.MatchString(sha) {
		return "", fmt.Errorf("%w: %q does not resolve to a commit", ErrCommitInput, ref)
	}
	return sha, nil
}
