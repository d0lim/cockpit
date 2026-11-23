package headless

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
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/cli"
	"github.com/d0lim/cockpit/internal/hub/config"
	hubstages "github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/runnerbin"
	"github.com/d0lim/cockpit/internal/version"
)

const (
	ExitInterrupted = 130
	logFile         = "run.log"
)

type Deps struct {
	Docker       compose.Docker
	EnsureImage  func(ctx context.Context) (runnerbin.Image, error)
	Home         string
	Out          io.Writer
	Free         func(port int) bool
	TempDir      string
	Now          func() time.Time
	Submitter    string
	ListenCanary hubstages.CanaryListener
}

type run struct {
	opts     Options
	deps     Deps
	st       *stages
	loaded   compose.Loaded
	base     int
	env      *hubstages.Env
	log      *os.File
	meta     store.Meta
	lines    []string
	suiteRun *compose.RunnerResult
	booted   *contract.BootVerdict
	reasons  []string
	bundle   string
	claim    string
	broken   bool
}

type check struct {
	name string
	fn   func() (string, error)
}

func (r *run) mask(s string) string { return r.env.Mask(s) }

func Run(ctx context.Context, opts Options, deps Deps) int {
	if deps.Free == nil {
		deps.Free = loopbackFree
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.TempDir == "" {
		deps.TempDir = os.TempDir()
	}
	m, err := mask.New(mask.Options{Mode: mask.Hub})
	if err != nil {
		fmt.Fprintln(deps.Out, err)
		return cli.ExitRejected
	}
	r := &run{opts: opts, deps: deps, env: &hubstages.Env{
		Masker: m, RunnerUser: hubstages.HostRunnerUser(), Docker: deps.Docker, Limits: hubstages.DefaultLimits(), Now: deps.Now, ListenCanary: deps.ListenCanary,
	}}
	r.st = &stages{out: deps.Out, mask: r.mask}
	r.meta = store.Meta{CockpitVersion: version.String(), Submitter: deps.Submitter, Keep: opts.Keep, CreatedAt: store.Timestamp(deps.Now())}
	refused := !r.prepare(ctx)
	if refused && r.claim != "" {
		os.Remove(r.claim)
	}
	if !refused {
		r.pipeline(ctx)
		r.deliver()
		r.cleanup(ctx)
		r.verdict()
	}
	switch {
	case ctx.Err() != nil:
		return ExitInterrupted
	case refused:
		return cli.ExitRejected
	case r.broken:
		return cli.ExitError
	}
	return cli.VerdictExit(string(r.meta.Verdict))
}

func (r *run) prepare(ctx context.Context) bool {
	git := compose.Git{Dir: r.opts.Project}
	checks := []check{{"outputs", r.outputs}, {"project", func() (string, error) {
		top, err := git.TopLevel(ctx)
		if err != nil {
			return "", err
		}
		if top != realOrSelf(r.opts.Project) {
			return "", fmt.Errorf("--project must be the top of a git worktree (%s)", top)
		}
		if r.meta.SHA, err = git.ResolveCommit(ctx, "HEAD"); err != nil {
			return "", err
		}
		if r.meta.ID, r.claim, err = claimRunID(ctx, r.deps.Docker, r.deps.TempDir, r.deps.Now(), r.meta.SHA); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s at %s, run %s", top, r.meta.SHA[:12], r.meta.ID), nil
	}}, {"declaration", func() (string, error) {
		loaded, err := compose.ReadDeclaration(ctx, git, r.meta.SHA)
		if err != nil {
			return "", err
		}
		r.loaded, r.meta.HostAPI = loaded, loaded.Declaration.HostAPI
		dirty, err := git.Dirty(ctx, loaded.Tree, relPaths(loaded.Declaration.Inputs.Dirty))
		if err != nil {
			return "", err
		}
		note := ""
		if len(dirty) > 0 {
			note = fmt.Sprintf("; warning: %d uncommitted changes, the run uses HEAD", len(dirty))
		}
		if r.opts.UnsealCheck && loaded.Declaration.NetworkLevel() == contract.NetworkOpen {
			return "", fmt.Errorf("--unseal-check needs network level sealed; this commit declares network: open")
		}
		suite, err := pickSuite(loaded, r.opts.Suite)
		r.meta.Suite = suite
		return fmt.Sprintf("hostApi %d, suite %s%s", loaded.Declaration.HostAPI, suite, note), err
	}}, {"docker", func() (string, error) {
		if err := compose.CheckVersions(ctx, r.deps.Docker); err != nil {
			return "", err
		}
		img, err := r.deps.EnsureImage(ctx)
		r.meta.RunnerImage = &contract.RunnerImage{Tag: img.Tag, ID: img.ID}
		return "runner image " + img.Tag, err
	}}, {"port block", func() (string, error) {
		cfg, err := config.Load(r.deps.Home)
		if err != nil {
			return "", err
		}
		r.base, err = FreeBlock(cfg, r.deps.Free)
		slot := (r.base - cfg.Limits.PortBase) / compose.PortWidth
		r.meta.Slot = &slot
		return fmt.Sprintf("%d-%d, canary %d", r.base, r.base+compose.PortWidth-1, compose.CanaryPort(r.base)), err
	}}}
	for _, c := range checks {
		if r.st.run(c.name, c.fn) != nil {
			return false
		}
	}
	return true
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
