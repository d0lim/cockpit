package runenv

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/config"
	hubstages "github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/runnerbin"
)

const (
	ExitPassed      = 0
	ExitFailed      = 1
	ExitRefused     = 2
	ExitInterrupted = 130
	logFile         = "run-env.log"
)

type Deps struct {
	Docker      compose.Docker
	EnsureImage func(ctx context.Context) (runnerbin.Image, error)
	Home        string
	Out         io.Writer
	Free        func(port int) bool
	TempDir     string
	Now         func() time.Time
}

type run struct {
	opts   Options
	deps   Deps
	st     *stages
	sha    string
	runID  string
	loaded compose.Loaded
	image  runnerbin.Image
	base   int
	env    *hubstages.Env
	log    *os.File
}

func (r *run) mask(s string) string { return r.env.Mask(s) }

func Run(ctx context.Context, opts Options, deps Deps) int {
	if deps.Free == nil {
		deps.Free = loopbackFree
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	m, err := mask.New(mask.Options{Mode: mask.Hub})
	if err != nil {
		fmt.Fprintln(deps.Out, err)
		return ExitRefused
	}
	r := &run{opts: opts, deps: deps, env: &hubstages.Env{Masker: m, RunnerUser: hubstages.HostRunnerUser(), Docker: deps.Docker, Limits: hubstages.DefaultLimits(), Now: deps.Now}}
	r.st = &stages{out: deps.Out, mask: r.mask}
	if !r.prepare(ctx) {
		r.st.say("\n%s", r.st.summary())
		return ExitRefused
	}
	r.pipeline(ctx)
	r.cleanup(ctx)
	r.st.say("\n%s", r.st.summary())
	switch {
	case ctx.Err() != nil:
		return ExitInterrupted
	case r.st.passed():
		return ExitPassed
	}
	return ExitFailed
}

func (r *run) prepare(ctx context.Context) bool {
	git := compose.Git{Dir: r.opts.Project}
	return r.st.run("project", func() (string, error) {
		top, err := git.TopLevel(ctx)
		if err != nil {
			return "", err
		}
		if top != realOrSelf(r.opts.Project) {
			return "", fmt.Errorf("--project must be the top of a git worktree (%s)", top)
		}
		if r.sha, err = git.ResolveCommit(ctx, "HEAD"); err != nil {
			return "", err
		}
		r.runID = fmt.Sprintf("run-env-%s-%s", r.deps.Now().UTC().Format("20060102-150405"), r.sha[:7])
		return fmt.Sprintf("%s at %s, run %s", top, r.sha[:12], r.runID), nil
	}) && r.st.run("declaration", func() (string, error) {
		loaded, err := compose.ReadDeclaration(ctx, git, r.sha)
		if err != nil {
			return "", err
		}
		r.loaded = loaded
		dirty, err := git.Dirty(ctx, relPaths(loaded.Declaration.Inputs.Dirty))
		if err != nil {
			return "", err
		}
		note := ""
		if len(dirty) > 0 {
			note = fmt.Sprintf("; warning: %d uncommitted changes, the run uses HEAD", len(dirty))
		}
		suite, err := pickSuite(loaded, r.opts.Suite)
		r.opts.Suite = suite
		return fmt.Sprintf("hostApi %d, suite %s%s", loaded.Declaration.HostAPI, suite, note), err
	}) && r.st.run("docker", func() (string, error) {
		if err := compose.CheckVersions(ctx, r.deps.Docker); err != nil {
			return "", err
		}
		img, err := r.deps.EnsureImage(ctx)
		r.image = img
		return "runner image " + img.Tag, err
	}) && r.st.run("port block", func() (string, error) {
		cfg, err := config.Load(r.deps.Home)
		if err != nil {
			return "", err
		}
		r.base, err = FreeBlock(cfg, r.deps.Free)
		return fmt.Sprintf("%d-%d, canary %d", r.base, r.base+compose.PortWidth-1, compose.CanaryPort(r.base)), err
	})
}

func realOrSelf(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return p
}

func relPaths[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, p := range in {
		out[i] = string(p)
	}
	return out
}

func pickSuite(loaded compose.Loaded, want string) (string, error) {
	dir := string(loaded.Declaration.Scripts.Suites)
	var names []string
	for p, e := range loaded.Tree.Entries {
		if e.Type == "blob" && path.Dir(p) == dir && strings.HasSuffix(p, ".ts") {
			names = append(names, strings.TrimSuffix(path.Base(p), ".ts"))
		}
	}
	sort.Strings(names)
	switch {
	case want != "" && contains(names, want):
		return want, nil
	case want != "":
		return "", fmt.Errorf("suite %q is not in %s; the commit has %s", want, dir, strings.Join(names, ", "))
	case len(names) == 1:
		return names[0], nil
	}
	return "", fmt.Errorf("pass --suite; %s holds %d suites: %s", dir, len(names), strings.Join(names, ", "))
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
