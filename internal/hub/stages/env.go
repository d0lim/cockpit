package stages

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/seal"
)

type Limits struct {
	Build time.Duration
	Boot  time.Duration
	Suite time.Duration
	Call  time.Duration
	Poll  time.Duration
}

func DefaultLimits() Limits {
	return Limits{Build: 60 * time.Minute, Boot: 10 * time.Minute, Suite: 20 * time.Minute, Call: 2 * time.Minute, Poll: 2 * time.Second}
}

type Failed struct {
	Message   string
	Violation bool
}

func (f *Failed) Error() string { return f.Message }

type Env struct {
	RunID        string
	Repo         compose.Git
	Loaded       compose.Loaded
	Layout       compose.Layout
	Root         *os.Root
	Base         int
	Image        string
	RunnerUser   string
	Docker       compose.Docker
	Masker       *mask.Masker
	Project      compose.Project
	Keys         map[string]string
	Static       []seal.Result
	Probes       []seal.Result
	Limits       Limits
	ListenCanary CanaryListener
	Now          func() time.Time
	Log          func(line string)
}

func (e *Env) Mask(s string) string {
	if e.Masker == nil {
		return s
	}
	return e.Masker.Mask(s)
}

func (e *Env) logLine(line string) {
	if e.Log != nil {
		e.Log(e.Mask(line))
	}
}

func (e *Env) RefreshMasker() error {
	m, err := RunMasker(e.Loaded.Declaration.Secrets.Patterns, compose.MaskSources(e.Loaded.Declaration, e.Layout))
	if err != nil {
		return err
	}
	e.Masker = m
	return nil
}

func RunMasker(patterns []string, src mask.Sources) (*mask.Masker, error) {
	values, err := mask.Refresh(src)
	if err != nil {
		values = mask.Values{}
	}
	m, merr := mask.New(mask.Options{Mode: mask.Hub, Known: values.Known, Patterns: patterns})
	if merr != nil {
		return nil, merr
	}
	return m, err
}

func (e *Env) Extract(ctx context.Context) error {
	return compose.Extract(ctx, e.Repo, e.Loaded.Tree, e.Root)
}

func (e *Env) GenerateEnv(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, e.Limits.Call)
	defer cancel()
	in := compose.EnvInput{RunID: e.RunID, Layout: e.Layout, Env: e.Loaded.Declaration.Env, User: compose.EnvUser(os.Getuid(), os.Getgid()), Mask: e.Mask}
	if err := compose.GenerateEnv(ctx, e.Docker, e.Root, in); err != nil {
		return err
	}
	return e.RefreshMasker()
}

func (e *Env) WriteRunFiles() error {
	decl := e.Loaded.Declaration
	if err := compose.MakeWorkTree(e.Root, decl); err != nil {
		return err
	}
	keys, err := compose.ImageKeys(decl, e.Loaded.Tree)
	if err != nil {
		return err
	}
	e.Keys = keys
	if err := compose.WriteRunEnv(e.Root, compose.RunEnvInput{RunID: e.RunID, Layout: e.Layout, PortBase: e.Base, Ports: decl.Ports, Keys: keys}); err != nil {
		return err
	}
	if err := compose.WriteKit(e.Root, compose.KitInput{RunID: e.RunID, PortBase: e.Base, Ports: decl.Ports, Notes: decl.Kit.Notes}); err != nil {
		return err
	}
	harness := compose.HarnessInput{RunID: e.RunID, RunnerImage: e.Image, PortBase: e.Base, Layout: e.Layout, Loaded: e.Loaded, RunnerUser: e.RunnerUser}
	if err := compose.WriteHarness(e.Root, harness); err != nil {
		return err
	}
	p, err := OpenProject(e.RunID, e.Layout, decl, e.Docker, e.Mask, e.logLine)
	if err != nil {
		return err
	}
	e.Project = p
	return nil
}

func failures(results []seal.Result, inconclusive bool) string {
	var out []string
	for _, r := range results {
		if !r.OK && r.Inconclusive == inconclusive {
			out = append(out, r.Name+": "+r.Detail)
		}
	}
	return strings.Join(out, " | ")
}

func sealFailure(what string, results []seal.Result) error {
	switch seal.Verdict(results) {
	case "failed":
		return &Failed{Message: what + " violated: " + failures(results, false), Violation: true}
	case "error":
		return &Failed{Message: "the seal could not be checked: " + failures(results, true)}
	}
	return nil
}

func (e *Env) WriteSeal() error {
	return seal.Write(e.Root, seal.Report{Static: e.Static, Probes: e.Probes}, e.Now(), e.Mask)
}

func (e *Env) SealStatic(ctx context.Context) error {
	config, err := e.Project.Config(ctx)
	if err != nil {
		return err
	}
	decl := e.Loaded.Declaration
	files, err := compose.Files(e.Layout, decl)
	if err != nil {
		return err
	}
	e.Static = seal.Static(seal.Input{
		Config: config, Files: files, Forbidden: decl.Compose.Forbidden, RunID: e.RunID, RunDir: e.Layout.Root,
		Layout: e.Layout, PortBase: e.Base, Ports: decl.Ports, Builder: decl.Project.Builder, RunnerImage: e.Image,
	})
	if err := e.WriteSeal(); err != nil {
		return err
	}
	return sealFailure("the static seal", e.Static)
}

func (e *Env) Build(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, e.Limits.Build)
	defer cancel()
	if err := e.Project.Build(ctx, e.Loaded.Declaration.Project.Builder); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("the build passed its limit of %s", e.Limits.Build)
		}
		return err
	}
	return nil
}

func HostRunnerUser() string {
	return compose.RunnerUser(runtime.GOOS, os.Getuid(), os.Getgid())
}
