package mask

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"testing"
)

const corpusPath = "../../testdata/mask/corpus.json"

var corpusCounts = map[string]int{
	"ts/mask":    24,
	"ts/redact":  16,
	"ts/secrets": 10,
	"go":         11,
}

type corpusFile struct {
	Version  int                `json:"version"`
	Fixtures map[string]fixture `json:"fixtures"`
	Cases    []corpusCase       `json:"cases"`
}

type fixture struct {
	Files   map[string]string `json:"files"`
	Sources struct {
		Outputs     []sourceJSON `json:"outputs"`
		Declared    []sourceJSON `json:"declared"`
		PublicNames []string     `json:"publicNames"`
	} `json:"sources"`
}

type sourceJSON struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type corpusCase struct {
	Name     string          `json:"name"`
	Origin   string          `json:"origin"`
	Kind     string          `json:"kind"`
	Mode     string          `json:"mode"`
	Known    []string        `json:"known"`
	Patterns []string        `json:"patterns"`
	Max      int             `json:"max"`
	Checks   []check         `json:"checks"`
	JSON     json.RawMessage `json:"json"`
	Expect   []string        `json:"expect"`
	Fixture  string          `json:"fixture"`
	Steps    []step          `json:"steps"`
}

type check struct {
	Input       string   `json:"input"`
	Expect      *string  `json:"expect"`
	Contains    []string `json:"contains"`
	NotContains []string `json:"notContains"`
	MaxRunes    int      `json:"maxRunes"`
	Idempotent  bool     `json:"idempotent"`
	Off         bool     `json:"off"`
	KnownNow    []string `json:"knownNow"`
}

type step struct {
	Do               string            `json:"do"`
	Base             string            `json:"base"`
	Path             string            `json:"path"`
	Target           string            `json:"target"`
	Content          *string           `json:"content"`
	Repeat           string            `json:"repeat"`
	Count            int               `json:"count"`
	Input            string            `json:"input"`
	Expect           json.RawMessage   `json:"expect"`
	ErrorContains    string            `json:"errorContains"`
	ErrorEquals      string            `json:"errorEquals"`
	ErrorNotContains []string          `json:"errorNotContains"`
	Writer           string            `json:"writer"`
	Files            map[string]string `json:"files"`
	Values           []string          `json:"values"`
}

func loadCorpus(t testing.TB) corpusFile {
	t.Helper()
	raw, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var c corpusFile
	if err := dec.Decode(&c); err != nil {
		t.Fatalf("cannot read corpus: %v", err)
	}
	return c
}

func TestCorpusCasesAllMeetExpectationsAndRunCountsPerOriginMatchTheTable(t *testing.T) {
	c := loadCorpus(t)
	ran := map[string]int{}
	for _, k := range c.Cases {
		t.Run(k.Name, func(t *testing.T) {
			ran[k.Origin]++
			for _, f := range runCase(c, k, realEngine) {
				t.Error(f)
			}
		})
	}
	if !maps.Equal(ran, corpusCounts) {
		t.Fatalf("runs per origin %v, table %v", ran, corpusCounts)
	}
}

func TestCorpusCaseNamesAreUniqueAndEveryCheckAndStepHasAnAssertion(t *testing.T) {
	c := loadCorpus(t)
	seen := map[string]bool{}
	asserted := 0
	for _, k := range c.Cases {
		if seen[k.Name] {
			t.Errorf("duplicate name: %s", k.Name)
		}
		seen[k.Name] = true
		for i, ch := range k.Checks {
			if ch.Expect == nil && len(ch.Contains) == 0 && len(ch.NotContains) == 0 && !ch.Idempotent {
				t.Errorf("%s: check %d asserts nothing", k.Name, i)
			}
			asserted++
		}
		if k.Kind == "values" && k.Expect != nil {
			asserted++
		}
		steps := 0
		for _, s := range k.Steps {
			if asserts(s) {
				steps++
			}
		}
		if k.Kind == "run" && steps == 0 {
			t.Errorf("%s: no step asserts anything", k.Name)
		}
		asserted += steps
	}
	if asserted == 0 {
		t.Fatal("zero assertions")
	}
}

func asserts(s step) bool {
	switch s.Do {
	case "known", "mask", "rescan", "bundleFree":
		return true
	case "refresh":
		return s.ErrorContains != "" || s.ErrorEquals != ""
	}
	return false
}

func TestEveryCorpusInputMaskedOnceIsUnchangedByMaskingAgainInBothModes(t *testing.T) {
	c := loadCorpus(t)
	want, got := 0, 0
	for _, k := range c.Cases {
		for _, ch := range k.Checks {
			for _, mode := range []Mode{Hub, Runner} {
				want++
				m, err := New(Options{Mode: mode, Known: k.Known, Patterns: k.Patterns})
				if err != nil {
					t.Fatalf("%s: %v", k.Name, err)
				}
				once := m.Mask(ch.Input)
				if twice := m.Mask(once); twice != once {
					t.Errorf("%s (mode %d): masking twice changes the result\nonce %q\ntwice %q", k.Name, mode, clip(once), clip(twice))
				}
				got++
			}
		}
	}
	if got == 0 || got != want {
		t.Fatalf("ran %d, want %d", got, want)
	}
}

func errText(err error) string {
	if err != nil {
		return err.Error()
	}
	return ""
}

func clip(s string) string {
	if r := []rune(s); len(r) > 160 {
		return string(r[:160]) + "…"
	}
	return s
}
