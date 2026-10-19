package runenv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
)

type step struct {
	name string
	fn   func(context.Context) (string, error)
}

func (r *run) pipeline(ctx context.Context) {
	steps := []step{
		{"run directory", r.runDirectory},
		{"environment generation", r.generate},
		{"run files", r.runFiles},
		{"seal static", r.sealStatic},
		{"build", r.build},
		{"seal probes", r.sealProbes},
	}
	if r.opts.UnsealCheck {
		steps = append(steps, step{"unseal check", r.unsealCheck})
	}
	steps = append(steps, step{"boot", r.boot}, step{"runner", r.runner})
	for _, s := range steps {
		if ctx.Err() != nil {
			r.st.run(s.name, func() (string, error) { return "", ctx.Err() })
			return
		}
		if !r.st.run(s.name, func() (string, error) { return s.fn(ctx) }) {
			return
		}
	}
}

func (r *run) runDirectory(ctx context.Context) (string, error) {
	dir, err := os.MkdirTemp(r.deps.TempDir, "cockpit-run-env-")
	if err != nil {
		return "", err
	}
	if dir, err = filepath.EvalSymlinks(dir); err != nil {
		return "", err
	}
	e := r.env
	if e.Layout, err = compose.NewLayout(dir); err != nil {
		return "", err
	}
	if e.Root, err = compose.OpenRun(dir); err != nil {
		return "", err
	}
	if r.log, err = e.Root.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600); err != nil {
		return "", err
	}
	e.RunID, e.Loaded, e.Base, e.Image = r.runID, r.loaded, r.base, r.image.Tag
	e.Repo = compose.Git{Dir: r.opts.Project}
	e.Log = func(line string) { fmt.Fprintln(r.log, line) }
	if err := e.Extract(ctx); err != nil {
		return dir, err
	}
	return fmt.Sprintf("%s, %d files extracted twice, child output in %s", dir, r.loaded.Tree.Files(), logFile), nil
}

func (r *run) generate(ctx context.Context) (string, error) {
	if err := r.env.GenerateEnv(ctx); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d outputs in a networkless container, redactor values refreshed", len(r.loaded.Declaration.Env.Outputs)), nil
}

func (r *run) runFiles(ctx context.Context) (string, error) {
	if err := r.env.WriteRunFiles(); err != nil {
		return "", err
	}
	var shown []string
	for _, k := range sortedKeys(r.env.Keys) {
		shown = append(shown, k+"="+r.env.Keys[k])
	}
	return "work tree, run.env, kit and harness written; " + strings.Join(shown, " "), nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (r *run) sealStatic(ctx context.Context) (string, error) {
	err := r.env.SealStatic(ctx)
	passed := 0
	for _, x := range r.env.Static {
		if x.OK {
			passed++
		}
	}
	return fmt.Sprintf("%d/%d passed", passed, len(r.env.Static)), err
}
