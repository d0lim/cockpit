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
		t.Fatalf("코퍼스를 읽지 못했다: %v", err)
	}
	return c
}

func Test코퍼스_사례는_전부_기대와_같고_출처별_실행_건수가_표와_같다(t *testing.T) {
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
		t.Fatalf("출처별 실행 건수 %v, 표 %v", ran, corpusCounts)
	}
}

func Test코퍼스_사례는_이름이_겹치지_않고_검사와_단계마다_단정이_하나_이상_있다(t *testing.T) {
	c := loadCorpus(t)
	seen := map[string]bool{}
	asserted := 0
	for _, k := range c.Cases {
		if seen[k.Name] {
			t.Errorf("이름이 겹친다: %s", k.Name)
		}
		seen[k.Name] = true
		for i, ch := range k.Checks {
			if ch.Expect == nil && len(ch.Contains) == 0 && len(ch.NotContains) == 0 && !ch.Idempotent {
				t.Errorf("%s: 검사 %d 에 단정이 없다", k.Name, i)
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
			t.Errorf("%s: 단정하는 단계가 없다", k.Name)
		}
		asserted += steps
	}
	if asserted == 0 {
		t.Fatal("단정 0건")
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

func Test코퍼스의_모든_입력은_두_모드에서_한_번_가린_결과를_다시_가려도_같다(t *testing.T) {
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
					t.Errorf("%s (모드 %d): 두 번 가린 결과가 다르다\n한 번 %q\n두 번 %q", k.Name, mode, clip(once), clip(twice))
				}
				got++
			}
		}
	}
	if got == 0 || got != want {
		t.Fatalf("실행 %d건, 기대 %d건", got, want)
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
