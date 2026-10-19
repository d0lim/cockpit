package runenv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/seal"
)

func (r *run) pipeline(ctx context.Context) {
	steps := []struct {
		name string
		fn   func(context.Context) (string, error)
	}{
		{"run directory", r.runDirectory},
		{"environment generation", r.generate},
		{"run files", r.runFiles},
		{"seal static", r.sealStatic},
		{"build", r.build},
		{"seal probes", r.sealProbes},
	}
	if r.opts.UnsealCheck {
		steps = append(steps, struct {
			name string
			fn   func(context.Context) (string, error)
		}{"unseal check", r.unsealCheck})
	}
	steps = append(steps, []struct {
		name string
		fn   func(context.Context) (string, error)
	}{{"boot", r.boot}, {"runner", r.runner}}...)
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
	if r.layout, err = compose.NewLayout(dir); err != nil {
		return "", err
	}
	if r.root, err = compose.OpenRun(dir); err != nil {
		return "", err
	}
	if r.log, err = r.root.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600); err != nil {
		return "", err
	}
	if err := compose.Extract(ctx, compose.Git{Dir: r.opts.Project}, r.loaded.Tree, r.root); err != nil {
		return dir, err
	}
	return fmt.Sprintf("%s, %d files extracted twice, child output in %s", dir, r.loaded.Tree.Files(), logFile), nil
}

func (r *run) logLine(line string) {
	if r.log != nil {
		fmt.Fprintln(r.log, r.mask(line))
	}
}

func (r *run) refreshMasker() error {
	values, err := mask.Refresh(compose.MaskSources(r.loaded.Declaration, r.layout))
	if err != nil {
		return err
	}
	m, err := mask.New(mask.Options{Mode: mask.Hub, Known: values.Known, Patterns: r.loaded.Declaration.Secrets.Patterns})
	if err != nil {
		return err
	}
	r.masker = m
	return nil
}

func (r *run) generate(ctx context.Context) (string, error) {
	decl := r.loaded.Declaration
	in := compose.EnvInput{RunID: r.runID, Layout: r.layout, Env: decl.Env, User: compose.EnvUser(os.Getuid(), os.Getgid()), Mask: r.mask}
	if err := compose.GenerateEnv(ctx, r.deps.Docker, r.root, in); err != nil {
		return "", err
	}
	if err := r.refreshMasker(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d outputs in a networkless container, redactor values refreshed", len(decl.Env.Outputs)), nil
}

func (r *run) runFiles(ctx context.Context) (string, error) {
	decl := r.loaded.Declaration
	if err := compose.MakeWorkTree(r.root, decl); err != nil {
		return "", err
	}
	keys, err := compose.ImageKeys(decl, r.loaded.Tree)
	if err != nil {
		return "", err
	}
	if err := compose.WriteRunEnv(r.root, compose.RunEnvInput{RunID: r.runID, Layout: r.layout, PortBase: r.base, Ports: decl.Ports, Keys: keys}); err != nil {
		return "", err
	}
	if err := compose.WriteKit(r.root, compose.KitInput{RunID: r.runID, PortBase: r.base, Ports: decl.Ports, Notes: decl.Kit.Notes}); err != nil {
		return "", err
	}
	harness := compose.HarnessInput{RunID: r.runID, RunnerImage: r.image.Tag, PortBase: r.base, Layout: r.layout, Loaded: r.loaded}
	if err := compose.WriteHarness(r.root, harness); err != nil {
		return "", err
	}
	args, err := compose.ComposeArgs(r.runID, r.layout, decl)
	if err != nil {
		return "", err
	}
	r.project = compose.Project{Docker: r.deps.Docker, Name: r.runID, Args: args, Mask: r.mask, Log: r.logLine}
	var shown []string
	for _, k := range sortedKeys(keys) {
		shown = append(shown, k+"="+keys[k])
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
	config, err := r.project.Config(ctx)
	if err != nil {
		return "", err
	}
	files, err := compose.Files(r.layout, r.loaded.Declaration)
	if err != nil {
		return "", err
	}
	decl := r.loaded.Declaration
	r.static = seal.Static(seal.Input{
		Config: config, Files: files, Forbidden: decl.Compose.Forbidden, RunID: r.runID, RunDir: r.layout.Root,
		Layout: r.layout, PortBase: r.base, Ports: decl.Ports, Builder: decl.Project.Builder, RunnerImage: r.image.Tag,
	})
	if err := r.writeSeal(); err != nil {
		return "", err
	}
	var failed []string
	for _, x := range r.static {
		if !x.OK {
			failed = append(failed, x.Name+": "+x.Detail)
		}
	}
	detail := fmt.Sprintf("%d/%d passed", len(r.static)-len(failed), len(r.static))
	if len(failed) > 0 {
		return detail, fmt.Errorf("failed %s", strings.Join(failed, " | "))
	}
	return detail, nil
}

func (r *run) writeSeal() error {
	return seal.Write(r.root, seal.Report{Static: r.static, Probes: r.probes}, r.deps.Now(), r.mask)
}
