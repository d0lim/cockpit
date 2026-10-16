package mask

import (
	"cmp"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

var mutations = []struct {
	target   string
	method   string
	mutate   func(engine) engine
	red      int
	rootless bool
}{
	{"value replacement stage", "empty the masker's value replacer", dropValues, 22, false},
	{"encoded variants of values", "register only the raw known values", plainValues, 2, false},
	{"core patterns", "empty the PEM rule and the core rules", dropCore, 29, false},
	{"declared patterns", "build the masker without the declared patterns", dropDeclared, 7, false},
	{"attribute key name rule", "empty the key rule", dropKeys, 3, false},
	{"mask before truncate", "truncate before masking", cutFirst, 2, false},
	{"key pattern value length", "revert the value slot +? to {1,1000}?", boundedValue, 1, false},
	{"fixed-point iteration", "run a single pass", singlePass, 1, false},
	{"rescan refuses links", "swap in a rescan that follows links", followRescan, 1, false},
	{"rescan read failure handling", "count unreadable files as clean", lenientRescan, 1, true},
	{"value source reads refuse links, oversize files and escaping paths", "read with os.ReadFile", followRead, 6, false},
	{"whole masker (positive control)", "swap in an identity masker with every stage emptied", identity, 54, false},
}

func TestMutationRecordEachMutationTurnsTheRecordedNumberOfCasesAndScenariosRed(t *testing.T) {
	c := loadCorpus(t)
	if red := redCases(c, realEngine); len(red) != 0 {
		t.Fatalf("the unmutated engine is red:\n%s", strings.Join(red, "\n"))
	}
	for _, mu := range mutations {
		t.Run(mu.target, func(t *testing.T) {
			if mu.rootless && os.Geteuid() == 0 {
				t.Skip("root reads mode 000 files too, so no read failure can be produced")
			}
			red := redCases(c, mu.mutate(realEngine))
			if len(red) == 0 || len(red) != mu.red {
				t.Errorf("%s: %d red cases, recorded %d\n%s", mu.method, len(red), mu.red, strings.Join(red, "\n"))
			}
		})
	}
}

func redCases(c corpusFile, e engine) []string {
	var red []string
	for _, k := range c.Cases {
		if len(runCase(c, k, e)) > 0 {
			red = append(red, "corpus: "+k.Name)
		}
	}
	for _, s := range scenarios {
		if len(s.run(e)) > 0 {
			red = append(red, "scenario: "+s.name)
		}
	}
	return red
}

func reshape(e engine, edit func(m *Masker, o Options)) engine {
	build := e.build
	e.build = func(o Options) (*Masker, error) {
		m, err := build(o)
		if err == nil {
			edit(m, o)
		}
		return m, err
	}
	e.withKnown = func(m *Masker, known []string) *Masker {
		next := m.WithKnown(known)
		edit(next, Options{Mode: m.mode, Known: known})
		return next
	}
	return e
}

func dropValues(e engine) engine {
	return reshape(e, func(m *Masker, _ Options) { m.values = nil })
}

func plainValues(e engine) engine {
	return reshape(e, func(m *Masker, o Options) {
		var pairs []string
		for _, v := range slices.SortedFunc(slices.Values(o.Known), func(a, b string) int { return cmp.Compare(len(b), len(a)) }) {
			if utf8.RuneCountInString(v) >= o.Mode.min() {
				pairs = append(pairs, v, o.Mode.cover(v))
			}
		}
		m.values = nil
		if len(pairs) > 0 {
			m.values = strings.NewReplacer(pairs...)
		}
	})
}

func dropCore(e engine) engine {
	return reshape(e, func(m *Masker, _ Options) { m.pem, m.rules = nil, nil })
}

func dropKeys(e engine) engine {
	return reshape(e, func(m *Masker, _ Options) { m.keys = nil })
}

func identity(e engine) engine {
	return reshape(e, func(m *Masker, _ Options) { *m = Masker{mode: m.mode} })
}

func dropDeclared(e engine) engine {
	build := e.build
	e.build = func(o Options) (*Masker, error) {
		o.Patterns = nil
		return build(o)
	}
	return e
}

func cutFirst(e engine) engine {
	e.summarize = func(m *Masker, s string, max int) string { return m.Mask(truncate(s, max)) }
	return e
}

func boundedValue(e engine) engine {
	e.build = func(o Options) (*Masker, error) {
		return newMasker(o, config{quotes: quoteForms, value: `[^"\r\n<>]{1,1000}?`})
	}
	return e
}

func singlePass(e engine) engine {
	e.mask = func(m *Masker, s string) string { return m.pass(s) }
	return e
}

func followRescan(e engine) engine {
	e.rescan = naiveRescan(true, false)
	return e
}

func lenientRescan(e engine) engine {
	e.rescan = naiveRescan(false, true)
	return e
}

func naiveRescan(follow, lenient bool) func(string, func(string) string) (Report, error) {
	return func(dir string, mask func(string) string) (Report, error) {
		var r Report
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(dir, path)
			r.Scanned++
			if !follow && !d.Type().IsRegular() {
				r.Left = append(r.Left, rel)
				return nil
			}
			raw, err := os.ReadFile(path)
			switch {
			case err != nil && !lenient:
				r.Left = append(r.Left, rel)
			case err == nil && mask(string(raw)) != string(raw):
				r.Left = append(r.Left, rel)
			}
			return nil
		})
		slices.Sort(r.Left)
		return r, err
	}
}

func followRead(e engine) engine {
	e.refresh = func(src Sources) (Values, error) {
		return refresh(src, func(root *os.Root, rel string, _ int64) (string, error) {
			raw, err := os.ReadFile(filepath.Join(root.Name(), rel))
			return string(raw), err
		})
	}
	return e
}
