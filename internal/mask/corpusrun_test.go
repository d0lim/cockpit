package mask

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

type engine struct {
	build     func(Options) (*Masker, error)
	withKnown func(*Masker, []string) *Masker
	mask      func(*Masker, string) string
	summarize func(*Masker, string, int) string
	refresh   func(Sources) (Values, error)
	rescan    func(string, func(string) string) (Report, error)
}

var realEngine = engine{
	build:     New,
	withKnown: (*Masker).WithKnown,
	mask:      (*Masker).Mask,
	summarize: (*Masker).Summarize,
	refresh:   Refresh,
	rescan:    Rescan,
}

func modeOf(s string) Mode {
	if s == "runner" {
		return Runner
	}
	return Hub
}

func runCase(c corpusFile, k corpusCase, e engine) []string {
	switch k.Kind {
	case "mask", "summarize":
		return runMaskCase(k, e)
	case "values":
		return runValuesCase(k)
	case "run":
		return runRunCase(c, k, e)
	}
	return []string{"unknown kind: " + k.Kind}
}

func runMaskCase(k corpusCase, e engine) []string {
	m, err := e.build(Options{Mode: modeOf(k.Mode), Known: k.Known, Patterns: k.Patterns})
	if err != nil {
		return []string{"cannot build masker: " + err.Error()}
	}
	var fails []string
	for i, ch := range k.Checks {
		cur := m
		if ch.KnownNow != nil {
			cur = e.withKnown(m, ch.KnownNow)
		}
		var out string
		switch {
		case ch.Off:
			out = ch.Input
		case k.Kind == "summarize":
			out = e.summarize(cur, ch.Input, k.Max)
		default:
			out = e.mask(cur, ch.Input)
		}
		fails = append(fails, judge(i, ch, out, func(s string) string { return e.mask(cur, s) })...)
	}
	return fails
}

func judge(i int, ch check, out string, again func(string) string) []string {
	var f []string
	if ch.Expect != nil && out != *ch.Expect {
		f = append(f, fmt.Sprintf("check %d: got %q, want %q", i, clip(out), clip(*ch.Expect)))
	}
	for _, s := range ch.Contains {
		if !strings.Contains(out, s) {
			f = append(f, fmt.Sprintf("check %d: %q is missing", i, clip(s)))
		}
	}
	for _, s := range ch.NotContains {
		if strings.Contains(out, s) {
			f = append(f, fmt.Sprintf("check %d: %q is still present", i, clip(s)))
		}
	}
	if ch.MaxRunes > 0 && utf8.RuneCountInString(out) > ch.MaxRunes {
		f = append(f, fmt.Sprintf("check %d: length %d exceeds %d", i, utf8.RuneCountInString(out), ch.MaxRunes))
	}
	if ch.Idempotent && again(out) != out {
		f = append(f, fmt.Sprintf("check %d: masking again turns it into %q", i, clip(again(out))))
	}
	return f
}

func runValuesCase(k corpusCase) []string {
	var doc any
	if err := json.Unmarshal(k.JSON, &doc); err != nil {
		return []string{err.Error()}
	}
	got := slices.Sorted(slices.Values(JSONStrings(doc)))
	if !slices.Equal(got, k.Expect) {
		return []string{fmt.Sprintf("values %q, want %q", got, k.Expect)}
	}
	return nil
}

type runState struct {
	e      engine
	k      corpusCase
	dirs   map[string]string
	src    Sources
	cur    *Masker
	known  []string
	bundle string
}

func runRunCase(c corpusFile, k corpusCase, e engine) (fails []string) {
	fx := c.Fixtures[k.Fixture]
	st := runState{e: e, k: k, dirs: map[string]string{}}
	for _, name := range []string{"run", "outside", "bundle"} {
		dir, err := os.MkdirTemp("", "mask-corpus-*")
		if err != nil {
			return []string{err.Error()}
		}
		defer os.RemoveAll(dir)
		st.dirs[name], _ = filepath.EvalSymlinks(dir)
	}
	st.bundle = st.dirs["bundle"]
	for rel, text := range fx.Files {
		if err := put(st.dirs["run"], rel, text); err != nil {
			return []string{err.Error()}
		}
	}
	st.src = Sources{Dir: st.dirs["run"], Mode: modeOf(k.Mode), PublicNames: fx.Sources.PublicNames}
	for _, s := range fx.Sources.Outputs {
		st.src.Outputs = append(st.src.Outputs, Source{Kind: Kind(s.Kind), Path: s.Path})
	}
	for _, s := range fx.Sources.Declared {
		st.src.Declared = append(st.src.Declared, Source{Kind: Kind(s.Kind), Path: s.Path})
	}
	var err error
	if st.cur, err = e.build(Options{Mode: modeOf(k.Mode), Patterns: k.Patterns}); err != nil {
		return []string{err.Error()}
	}
	for i, s := range k.Steps {
		if msg := st.do(s); msg != "" {
			fails = append(fails, fmt.Sprintf("step %d (%s): %s", i, s.Do, msg))
		}
	}
	return fails
}

func (st *runState) do(s step) string {
	base := st.dirs[cmp.Or(s.Base, "run")]
	switch s.Do {
	case "refresh":
		v, err := st.e.refresh(st.src)
		if err == nil {
			st.known = v.Known
			st.cur = st.e.withKnown(st.cur, v.Known)
		}
		return judgeError(s, err)
	case "known":
		var want []string
		_ = json.Unmarshal(s.Expect, &want)
		if got := slices.Sorted(slices.Values(st.known)); !slices.Equal(got, slices.Sorted(slices.Values(want))) {
			return fmt.Sprintf("value list of %d entries differs from the expected %d", len(got), len(want))
		}
	case "mask":
		var want string
		_ = json.Unmarshal(s.Expect, &want)
		if got := st.e.mask(st.cur, s.Input); got != want {
			return fmt.Sprintf("got %q, want %q", clip(got), clip(want))
		}
	case "write":
		text := strings.Repeat(s.Repeat, s.Count)
		if s.Content != nil {
			text = *s.Content
		}
		return errText(put(base, s.Path, text))
	case "remove":
		return errText(os.RemoveAll(filepath.Join(st.dirs["run"], s.Path)))
	case "symlink":
		return errText(os.Symlink(filepath.Join(base, s.Target), filepath.Join(st.dirs["run"], s.Path)))
	case "bundle":
		return st.writeBundle(s)
	case "bundleSymlink":
		return errText(os.Symlink(filepath.Join(base, s.Target), filepath.Join(st.bundle, s.Path)))
	case "rescan":
		var want []string
		_ = json.Unmarshal(s.Expect, &want)
		r, err := st.e.rescan(st.bundle, func(t string) string { return st.e.mask(st.cur, t) })
		if err != nil {
			return err.Error()
		}
		if !slices.Equal(r.Left, want) {
			return fmt.Sprintf("left files %q, want %q", r.Left, want)
		}
	case "bundleFree":
		return st.bundleFree(s.Values)
	default:
		return "unknown step"
	}
	return ""
}

func (st *runState) writeBundle(s step) string {
	write := func(t string) string { return st.e.mask(st.cur, t) }
	switch s.Writer {
	case "identity":
		write = func(t string) string { return t }
	case "plainQuoteOnly":
		weak, err := newMasker(Options{Mode: modeOf(st.k.Mode), Known: st.known, Patterns: st.k.Patterns}, config{quotes: []string{`"`}, value: valueClass})
		if err != nil {
			return err.Error()
		}
		write = weak.Mask
	}
	for rel, text := range s.Files {
		if err := put(st.bundle, rel, write(text)); err != nil {
			return err.Error()
		}
	}
	return ""
}

func (st *runState) bundleFree(values []string) string {
	files := 0
	var left []string
	err := filepath.WalkDir(st.bundle, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		files++
		raw, err := os.ReadFile(path)
		for _, v := range values {
			if strings.Contains(string(raw), v) {
				left = append(left, clip(v))
			}
		}
		return err
	})
	switch {
	case err != nil:
		return err.Error()
	case files == 0:
		return "bundle has zero files"
	case len(left) > 0:
		return fmt.Sprintf("planted values left in the bundle: %q", left)
	}
	return ""
}

func judgeError(s step, err error) string {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	switch {
	case s.ErrorEquals != "" && msg != s.ErrorEquals:
		return fmt.Sprintf("error %q, want %q", msg, s.ErrorEquals)
	case s.ErrorContains != "" && !strings.Contains(msg, s.ErrorContains):
		return fmt.Sprintf("error %q does not contain %q", msg, s.ErrorContains)
	case s.ErrorEquals == "" && s.ErrorContains == "" && err != nil:
		return "unexpected error: " + msg
	}
	for _, v := range s.ErrorNotContains {
		if strings.Contains(msg, v) {
			return "error message carries a fragment of the input"
		}
	}
	return ""
}

func put(root, rel, text string) error {
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o600)
}
