package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/signal"
)

const (
	DirReport        = "report"
	DirPartial       = "report.partial"
	FileSummary      = "summary.json"
	FileMeta         = "meta.json"
	FileNotProven    = "not-proven.md"
	RedactionFailed  = "failed"
	StageSignals     = "signals"
	RefusedRedaction = "the report bundle failed the redaction rescan"
)

var reserved = []string{FileSummary, FileMeta, FileNotProven, signal.Dir}

type Signals interface {
	Freeze(bundleDir string) (signal.FreezeResult, error)
	RefuseReads(reason string) error
}

type Bundle struct {
	RunDir  string
	Meta    store.Meta
	Inputs  Inputs
	Summary SummaryInput
	Files   map[string][]byte
	Unread  []Unread
	Mask    *mask.Masker
	Signals Signals
	Patch   func(change func(*store.Meta)) error
}

type Outcome struct {
	Judgement Judgement
	Dir       string
	Redaction *store.Redaction
	Frozen    signal.FreezeResult
}

func Assemble(b Bundle) (Outcome, error) {
	if b.Mask == nil {
		return Outcome{}, errors.New("report: no masker")
	}
	for name := range b.Files {
		if !filepath.IsLocal(name) || slices.Contains(reserved, firstElem(name)) {
			return Outcome{}, fmt.Errorf("report: bundle path %q is not allowed", name)
		}
	}
	final := filepath.Join(b.RunDir, DirReport)
	partial := filepath.Join(b.RunDir, DirPartial)
	for _, dir := range []string{final, partial} {
		if err := os.RemoveAll(dir); err != nil {
			return Outcome{}, err
		}
	}
	if err := os.Mkdir(partial, 0o700); err != nil {
		return Outcome{}, err
	}
	out := Outcome{}
	unread, stages := slices.Clone(b.Unread), cloneStages(b.Summary.Stages)
	stages[StageSignals] = contract.CollectStage{Status: contract.CollectStageSkipped}
	if b.Signals != nil {
		frozen, err := b.Signals.Freeze(partial)
		if err != nil {
			stages[StageSignals] = contract.CollectStage{Status: contract.CollectStageError, Detail: "freeze failed"}
			unread = append(unread, Unread{Source: StageSignals, Detail: "freeze failed", Count: 1})
		} else {
			out.Frozen, stages[StageSignals] = frozen, contract.CollectStage{Status: contract.CollectStageOK}
		}
	}
	judged := Settle(Judge(b.Inputs), b.Meta.Verdict)
	out.Judgement = maskJudgement(judged, b.Mask)
	lines := UnreadLines(unread, out.Frozen.Dropped)
	s := b.Summary
	s.Stages = stages
	summary := Summary(b.Inputs, judged, s, lines)
	files := map[string][]byte{}
	for name, data := range b.Files {
		files[name] = data
	}
	var err error
	if files[FileSummary], err = encode(summary); err != nil {
		return discard(partial, out, err)
	}
	if files[FileMeta], err = encode(BundleMetaOf(b.Meta, s, out.Judgement.Verdict)); err != nil {
		return discard(partial, out, err)
	}
	files[FileNotProven] = []byte(NotProvenText(b.Meta.ID, summary.NotProven))
	if err := writeMasked(partial, files, b.Mask); err != nil {
		return discard(partial, out, err)
	}
	report, err := mask.Rescan(partial, b.Mask.Mask)
	if err != nil {
		return discard(partial, out, err)
	}
	if len(report.Left) > 0 {
		return refuse(b, partial, out, report.Left)
	}
	if err := os.Rename(partial, final); err != nil {
		return discard(partial, out, err)
	}
	out.Dir = final
	return out, nil
}

func maskJudgement(j Judgement, m *mask.Masker) Judgement {
	masked := func(in []string) []string {
		out := make([]string, len(in))
		for i, s := range in {
			out[i] = m.Mask(s)
		}
		return out
	}
	j.Reasons, j.Skipped, j.Declared = masked(j.Reasons), masked(j.Skipped), masked(j.Declared)
	return j
}

func firstElem(name string) string {
	clean := filepath.ToSlash(filepath.Clean(name))
	for i, r := range clean {
		if r == '/' {
			return clean[:i]
		}
	}
	return clean
}

func cloneStages(in map[string]contract.CollectStage) map[string]contract.CollectStage {
	out := make(map[string]contract.CollectStage, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func encode(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	return append(data, '\n'), err
}

func discard(partial string, out Outcome, cause error) (Outcome, error) {
	if err := os.RemoveAll(partial); err != nil {
		return out, errors.Join(cause, err)
	}
	return out, cause
}

func refuse(b Bundle, partial string, out Outcome, left []string) (Outcome, error) {
	if err := os.RemoveAll(partial); err != nil {
		return out, err
	}
	out.Redaction = &store.Redaction{Status: RedactionFailed, Files: left}
	var errs []error
	if b.Patch != nil {
		errs = append(errs, b.Patch(func(m *store.Meta) { m.Redaction = out.Redaction }))
	}
	if b.Signals != nil {
		errs = append(errs, b.Signals.RefuseReads(RefusedRedaction))
	}
	return out, errors.Join(errs...)
}

func writeMasked(dir string, files map[string][]byte, m *mask.Masker) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		rel := filepath.Clean(name)
		if parent := filepath.Dir(rel); parent != "." {
			if err := root.MkdirAll(parent, 0o700); err != nil {
				return err
			}
		}
		f, err := root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, werr := f.WriteString(m.Mask(string(files[name])))
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return werr
		}
	}
	return chmodTree(root)
}

func chmodTree(root *os.Root) error {
	return fsWalk(root, ".", func(rel string, dir bool) error {
		if dir {
			return root.Chmod(rel, 0o700)
		}
		return root.Chmod(rel, 0o600)
	})
}

func fsWalk(root *os.Root, rel string, visit func(rel string, dir bool) error) error {
	if err := visit(rel, true); err != nil {
		return err
	}
	f, err := root.Open(rel)
	if err != nil {
		return err
	}
	entries, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		return err
	}
	for _, e := range entries {
		child := filepath.Join(rel, e.Name())
		switch {
		case e.IsDir():
			err = fsWalk(root, child, visit)
		case e.Type().IsRegular():
			err = visit(child, false)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
