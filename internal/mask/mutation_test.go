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
	{"값 치환 단계", "마스커의 값 치환기를 비운다", dropValues, 22, false},
	{"값의 인코딩 변형", "알려진 값 원문만 등록한다", plainValues, 2, false},
	{"코어 패턴", "PEM 과 코어 규칙을 비운다", dropCore, 29, false},
	{"선언 패턴", "선언 패턴을 버리고 마스커를 만든다", dropDeclared, 7, false},
	{"속성 키 이름 규칙", "키 규칙을 비운다", dropKeys, 3, false},
	{"가린 뒤 자르기", "자른 뒤 가린다", cutFirst, 2, false},
	{"키 패턴 값 길이", "값 자리 +? 를 {1,1000}? 로 되돌린다", boundedValue, 1, false},
	{"고정점 반복", "한 번만 지난다", singlePass, 1, false},
	{"재스캔의 링크 거부", "링크를 따라 읽는 재스캔으로 바꾼다", followRescan, 1, false},
	{"재스캔의 읽기 실패 처리", "읽지 못한 파일을 깨끗한 것으로 센다", lenientRescan, 1, true},
	{"값 원천 읽기의 링크·크기·경로 거부", "os.ReadFile 로 읽는다", followRead, 6, false},
	{"마스커 전체(양성 대조)", "모든 단계를 비운 항등 마스커를 끼운다", identity, 54, false},
}

func Test변이_기록표의_변이마다_코퍼스와_시나리오가_기록한_건수만큼_빨개진다(t *testing.T) {
	c := loadCorpus(t)
	if red := redCases(c, realEngine); len(red) != 0 {
		t.Fatalf("변이 없는 엔진이 빨갛다:\n%s", strings.Join(red, "\n"))
	}
	for _, mu := range mutations {
		t.Run(mu.target, func(t *testing.T) {
			if mu.rootless && os.Geteuid() == 0 {
				t.Skip("root 는 권한 000 파일도 읽어 읽기 실패를 만들 수 없다")
			}
			red := redCases(c, mu.mutate(realEngine))
			if len(red) == 0 || len(red) != mu.red {
				t.Errorf("%s: 빨간 사례 %d건, 기록 %d건\n%s", mu.method, len(red), mu.red, strings.Join(red, "\n"))
			}
		})
	}
}

func redCases(c corpusFile, e engine) []string {
	var red []string
	for _, k := range c.Cases {
		if len(runCase(c, k, e)) > 0 {
			red = append(red, "코퍼스: "+k.Name)
		}
	}
	for _, s := range scenarios {
		if len(s.run(e)) > 0 {
			red = append(red, "시나리오: "+s.name)
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
