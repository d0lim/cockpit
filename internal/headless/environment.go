package headless

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/store"
)

type step struct {
	name  string
	phase lifecycle.Phase
	fn    func(context.Context) (string, error)
}

func (r *run) steps() []step {
	steps := []step{
		{"run directory", lifecycle.Preparing, r.runDirectory},
		{"environment generation", lifecycle.Preparing, r.generate},
		{"run files", lifecycle.Preparing, r.runFiles},
		{"seal static", lifecycle.Preparing, r.sealStatic},
		{"build", lifecycle.Building, r.build},
		{"seal probes", lifecycle.Sealing, r.sealProbes},
	}
	if r.opts.UnsealCheck {
		steps = append(steps, step{"unseal check", lifecycle.Sealing, r.unsealCheck})
	}
	return append(steps, step{"boot", lifecycle.Booting, r.boot}, step{"runner", lifecycle.Testing, r.runner})
}

func (r *run) pipeline(ctx context.Context) {
	r.walk(lifecycle.Submit, lifecycle.Detail{})
	r.walk(lifecycle.TickStart, lifecycle.Detail{})
	r.meta.StartedAt = store.Timestamp(r.deps.Now())
	steps := r.steps()
	for i, s := range steps {
		err := r.st.run(s.name, func() (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			detail, err := s.fn(ctx)
			if err == nil {
				err = ctx.Err()
			}
			return detail, err
		})
		if err != nil {
			r.fail(ctx, s.phase, err)
			break
		}
		if i == len(steps)-1 || steps[i+1].phase != s.phase {
			r.walk(passEvents[s.phase], lifecycle.Detail{})
		}
	}
	if r.meta.Phase == lifecycle.Reporting {
		r.st.run("report", func() (string, error) { return r.report(ctx) })
	}
}

func (r *run) runDirectory(ctx context.Context) (string, error) {
	dir, err := os.MkdirTemp(r.deps.TempDir, "cockpit-"+r.meta.ID+"-")
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
	e.RunID, e.Loaded, e.Base, e.Image = r.meta.ID, r.loaded, r.base, r.meta.RunnerImage.Tag
	e.Log = r.remember
	if err := e.Extract(ctx); err != nil {
		return dir, err
	}
	return fmt.Sprintf("%s, %d files extracted twice, child output in %s", dir, r.loaded.Tree.Files(), logFile), nil
}

func (r *run) remember(line string) {
	fmt.Fprintln(r.log, line)
	r.lines = append(r.lines, line)
	if len(r.lines) > tailLines {
		r.lines = r.lines[len(r.lines)-tailLines:]
	}
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
	r.meta.ImageKeys, r.meta.HarnessVersion = r.env.Keys, compose.HarnessVersion
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
