package envops

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/procgroup"
	"github.com/d0lim/cockpit/internal/hub/runfiles"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/wire"
)

const (
	wireTakeLimit = 3 * time.Minute
	wireArmLimit  = 30 * time.Second
	wireArm       = "arm"
	wireTake      = "take"

	detailNoCaptured       = "the runner started and left no captured values"
	detailCapturedTooLarge = "the captured values are over their limit of 16 MiB"
)

var (
	wireValuesPath = path.Join(runfiles.E2EDir, contract.WireValuesFile)
	manifestPath   = path.Join(runfiles.E2EDir, runfiles.ManifestFile)
)

type RunWire struct {
	ops  *Ops
	id   string
	dir  string
	decl *contract.Declaration
}

func (o *Ops) RunWire(id, dir string, decl *contract.Declaration) *RunWire {
	return &RunWire{ops: o, id: id, dir: dir, decl: decl}
}

func (w *RunWire) Arm(ctx context.Context, take string) error {
	ctx, cancel := context.WithTimeout(ctx, wireArmLimit)
	defer cancel()
	return w.control(ctx, wireArm, take, nil)
}

func (w *RunWire) Take(ctx context.Context, take string, result *contract.Result) wire.Status {
	ctx, cancel := context.WithTimeout(ctx, wireTakeLimit)
	defer cancel()
	in := wire.TakeInput{
		Store: wire.OpenStore(w.dir), Name: take, Result: result,
		Record: func() (*mask.Masker, error) { return recordMasker(w.dir, w.decl) },
		Stream: func(ctx context.Context, out io.Writer) error { return w.control(ctx, wireTake, take, out) },
	}
	if w.decl != nil {
		in.PublicFields, in.Routes = w.decl.Wire.PublicFields, w.decl.Wire.Routes
	}
	return wire.Take(ctx, in)
}

func (w *RunWire) TakeSuite(ctx context.Context, result *contract.Result) wire.Status {
	if err := wire.OpenStore(w.dir).Detach(); err != nil {
		w.warn("the wire record stays in the bundle, and the report carries it over", err)
	}
	return w.Take(ctx, wire.SuiteTake, result)
}

func (w *RunWire) Attach() {
	if err := wire.OpenStore(w.dir).Attach(); err != nil {
		w.warn("the wire record was not moved into the bundle", err)
	}
}

func (w *RunWire) warn(message string, err error) {
	w.ops.log().Warn(message, "run", w.id, "err", w.ops.Mask(w.id, err.Error()))
}

func (w *RunWire) control(ctx context.Context, verb, take string, out io.Writer) error {
	docker := w.ops.DockerFor(procgroup.Hooks{})
	container, err := w.container(ctx, docker)
	if err != nil {
		return err
	}
	res, err := docker.Run(ctx, compose.Command{Args: compose.WireCtlArgs(container, verb, take), Stdout: out})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		w.warn("wire-ctl "+verb+" was refused", errors.New(strings.TrimSpace(string(res.Stderr))))
		return wire.ErrRecorderGone
	}
	return nil
}

func (w *RunWire) container(ctx context.Context, docker compose.Docker) (string, error) {
	project := compose.Project{Docker: docker, Name: w.id, Mask: func(s string) string { return w.ops.Mask(w.id, s) }}
	rows, err := project.Ps(ctx)
	if err != nil {
		w.warn("the wire recorder was not found", err)
		return "", wire.ErrRecorderGone
	}
	for _, row := range rows {
		if row.Service == compose.RoleWire && !row.OneOff() && row.State == "running" && row.ID != "" {
			return row.ID, nil
		}
	}
	return "", wire.ErrRecorderGone
}

func recordMasker(runDir string, decl *contract.Declaration) (*mask.Masker, error) {
	if decl == nil {
		return nil, errors.New("the run has no declaration copy")
	}
	layout, err := compose.NewLayout(runDir)
	if err != nil {
		return nil, err
	}
	sources := compose.MaskSources(*decl, layout)
	sources.Mode = mask.Runner
	values, err := mask.Refresh(sources)
	if err != nil {
		return nil, err
	}
	captured, err := capturedValues(runDir)
	if err != nil {
		return nil, err
	}
	m, err := mask.New(mask.Options{Mode: mask.Runner, Known: append(values.Known, captured...), Patterns: decl.Secrets.Patterns})
	if err != nil {
		return nil, err
	}
	fields := make([]mask.PublicField, 0, len(decl.Wire.PublicFields))
	for _, f := range decl.Wire.PublicFields {
		fields = append(fields, mask.PublicField(f))
	}
	return m.WithPublicFields(fields)
}

func capturedValues(runDir string) ([]string, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if _, err := root.Lstat(wireValuesPath); errors.Is(err, fs.ErrNotExist) {
		if _, merr := root.Lstat(manifestPath); errors.Is(merr, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, &wire.WithheldError{Detail: detailNoCaptured}
	}
	values, err := mask.Refresh(mask.Sources{Dir: runDir, Mode: mask.Runner, Limit: contract.WireValuesLimit, Declared: []mask.Source{{Kind: mask.JSON, Path: wireValuesPath}}})
	if errors.Is(err, mask.ErrTooLarge) {
		return nil, &wire.WithheldError{Detail: detailCapturedTooLarge}
	}
	return values.Known, err
}
