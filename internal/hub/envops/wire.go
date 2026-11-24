package envops

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/procgroup"
	"github.com/d0lim/cockpit/internal/hub/runfiles"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/signal"
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
	ID     string
	Dir    string
	Decl   *contract.Declaration
	Docker compose.Docker
	Mask   func(string) string
	Stream func() *signal.Stream
	Log    *slog.Logger
}

func (o *Ops) RunWire(id, dir string, decl *contract.Declaration) *RunWire {
	return &RunWire{
		ID: id, Dir: dir, Decl: decl, Docker: o.DockerFor(procgroup.Hooks{}), Log: o.log(),
		Mask:   func(s string) string { return o.Mask(id, s) },
		Stream: func() *signal.Stream { return o.Stream(id) },
	}
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
		Store: wire.OpenStore(w.Dir), Name: take, Result: result,
		Record: func() (*mask.Masker, error) { return recordMasker(w.Dir, w.Decl) },
		Stream: func(ctx context.Context, out io.Writer) error { return w.control(ctx, wireTake, take, out) },
		Spans:  func() []wire.Span { return w.spans(result) },
	}
	if w.Decl != nil {
		in.PublicFields, in.Routes = w.Decl.Wire.PublicFields, w.Decl.Wire.Routes
	}
	return wire.Take(ctx, in)
}

func (w *RunWire) TakeSuite(ctx context.Context, result *contract.Result) wire.Status {
	if err := wire.OpenStore(w.Dir).Detach(); err != nil {
		w.warn("the wire record stays in the bundle, and the report carries it over", err)
	}
	return w.Take(ctx, wire.SuiteTake, result)
}

func (w *RunWire) Attach() {
	if err := wire.OpenStore(w.Dir).Attach(); err != nil {
		w.warn("the wire record was not moved into the bundle", err)
	}
}

func (w *RunWire) warn(message string, err error) {
	w.Log.Warn(message, "run", w.ID, "err", w.Mask(err.Error()))
}

func (w *RunWire) control(ctx context.Context, verb, take string, out io.Writer) error {
	container, err := w.container(ctx)
	if err != nil {
		return err
	}
	res, err := w.Docker.Run(ctx, compose.Command{Args: compose.WireCtlArgs(container, verb, take), Stdout: out})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		w.warn("wire-ctl "+verb+" was refused", errors.New(strings.TrimSpace(string(res.Stderr))))
		return wire.ErrRecorderGone
	}
	return nil
}

func (w *RunWire) container(ctx context.Context) (string, error) {
	project := compose.Project{Docker: w.Docker, Name: w.ID, Mask: w.Mask}
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
