package contract

import (
	"path/filepath"
	"strings"
	"testing"
)

func declarationSample(t *testing.T) map[string]any {
	t.Helper()
	return readDoc(t, filepath.Join("testdata", "declaration.json"))
}

func Test선언_샘플은_선언_스키마를_통과한다(t *testing.T) {
	if err := loadSchema(t, "declaration").Validate(declarationSample(t)); err != nil {
		t.Fatal(err)
	}
}

func Test선언에서_secrets_섹션을_지우면_스키마_검증이_실패한다(t *testing.T) {
	schema := loadSchema(t, "declaration")
	doc := declarationSample(t)
	if err := schema.Validate(doc); err != nil {
		t.Fatalf("원본이 통과하지 못했다: %v", err)
	}
	delete(doc, "secrets")
	err := schema.Validate(doc)
	if err == nil || !strings.Contains(err.Error(), "secrets") {
		t.Fatalf("secrets 없는 선언이 거부되지 않았다: %v", err)
	}
}

func Test선언의_hostApi가_문자열이면_스키마_검증이_실패한다(t *testing.T) {
	schema := loadSchema(t, "declaration")
	doc := declarationSample(t)
	doc["hostApi"] = "1"
	if err := schema.Validate(doc); err == nil {
		t.Fatal("문자열 hostApi 가 통과했다")
	}
	doc["hostApi"] = 2
	if err := schema.Validate(doc); err != nil {
		t.Fatalf("정수 hostApi 가 거부됐다: %v", err)
	}
}

type fieldCase struct {
	path   []any
	accept []any
	reject []any
}

func checkFields(t *testing.T, cases []fieldCase) (accepted, rejected int) {
	t.Helper()
	schema := loadSchema(t, "declaration")
	for _, c := range cases {
		for _, value := range c.accept {
			doc := declarationSample(t)
			set(t, doc, value, c.path...)
			if err := schema.Validate(doc); err != nil {
				t.Errorf("%v = %q 가 거부됐다: %v", c.path, value, err)
			}
			accepted++
		}
		for _, value := range c.reject {
			doc := declarationSample(t)
			set(t, doc, value, c.path...)
			if err := schema.Validate(doc); err == nil {
				t.Errorf("%v = %q 가 통과했다", c.path, value)
			}
			rejected++
		}
	}
	return accepted, rejected
}

func Test선언의_경로_필드는_커밋_루트_밖과_점_세그먼트와_빈_세그먼트를_거부한다(t *testing.T) {
	accept := []any{"app", ".env", "a/b.c", "a/.hidden/c", "...", "x..y", "app/.secrets.json"}
	reject := []any{"", "/abs", "../x", "./x", "..", ".", "a/..", "a/../b", "a/./b", "a//b", "a/"}
	paths := [][]any{
		{"compose", "files", 0},
		{"inputs", "dirty", 0},
		{"inputs", "runnerSources", 0, "path"},
		{"images", 0, "inputs", 0},
		{"env", "outputs", 0},
		{"secrets", "sources", 0, "path"},
		{"scripts", "dir"},
		{"scripts", "suites"},
		{"scripts", "collectors"},
	}
	var cases []fieldCase
	for _, p := range paths {
		cases = append(cases, fieldCase{path: p, accept: accept, reject: reject})
	}
	accepted, rejected := checkFields(t, cases)
	if accepted != len(paths)*len(accept) || rejected != len(paths)*len(reject) {
		t.Fatalf("평가한 수 허용 %d 거부 %d", accepted, rejected)
	}
}

func Test선언의_이름_필드는_소문자_숫자_하이픈_63자까지만_받는다(t *testing.T) {
	longest := "a" + strings.Repeat("b", 62)
	accept := []any{"app", "a-1", "0x", longest}
	reject := []any{"", "App", "-a", "a_b", "a/b", "..", "a b", longest + "c"}
	paths := [][]any{
		{"ports", 0, "name"},
		{"inputs", "runnerSources", 0, "name"},
		{"databases", 0, "name"},
	}
	var cases []fieldCase
	for _, p := range paths {
		cases = append(cases, fieldCase{path: p, accept: accept, reject: reject})
	}
	accepted, rejected := checkFields(t, cases)
	if accepted != len(paths)*len(accept) || rejected != len(paths)*len(reject) {
		t.Fatalf("평가한 수 허용 %d 거부 %d", accepted, rejected)
	}
}

func Test선언의_env_이미지는_digest로_핀한_참조만_받는다(t *testing.T) {
	digest := "sha256:" + strings.Repeat("0123456789abcdef", 4)
	accepted, rejected := checkFields(t, []fieldCase{{
		path:   []any{"env", "image"},
		accept: []any{"alpine@" + digest, "registry.example:5000/tools/env:1@" + digest},
		reject: []any{"alpine:3.20", "alpine@sha256:abc", "alpine@" + strings.ToUpper(digest), "@" + digest},
	}})
	if accepted != 2 || rejected != 4 {
		t.Fatalf("평가한 수 허용 %d 거부 %d", accepted, rejected)
	}
}

func Test선언의_열거_필드는_정해진_값만_받는다(t *testing.T) {
	accepted, rejected := checkFields(t, []fieldCase{
		{path: []any{"ports", 0, "open"}, accept: []any{"browser", "link", "copy"}, reject: []any{"window", ""}},
		{path: []any{"databases", 0, "driver"}, accept: []any{"mysql", "postgres"}, reject: []any{"sqlite"}},
		{path: []any{"secrets", "sources", 0, "format"}, accept: []any{"dotenv", "json", "pem"}, reject: []any{"xml"}},
		{path: []any{"images", 0, "key"}, accept: []any{"IMG_APP_2"}, reject: []any{"IMG_app", "APP"}},
	})
	if accepted != 9 || rejected != 6 {
		t.Fatalf("평가한 수 허용 %d 거부 %d", accepted, rejected)
	}
}
