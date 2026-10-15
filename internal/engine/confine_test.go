package engine

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

var confineMutations = []mutation{
	{"전역 허용 목록", "setup 이 confineGlobals 를 건너뛴다", "빨강: GoError·require·setInterval·setImmediate·clear* 가 남는다", "Test전역_객체의_이름은_허용_목록과_양방향으로_같다"},
	{"주요 프로토타입 동결", "setup 이 freezePrototypes 를 건너뛴다", "빨강: Object 부터 URLSearchParams 까지 얼지 않은 프로토타입이 나열된다", "Test전역_생성자의_프로토타입은_모두_얼어_있다"},
	{"디스크를 읽지 않는 require 로더", "WithLoader 에 require.DefaultSourceLoader 를 준다", "빨강: 절대·상대·확장자 없는 경로가 모두 PLANTED_REGISTRY 를 돌려준다", "Test레지스트리는_심은_파일을_디스크에서_읽지_않는다"},
}

var wantGlobals = []string{
	"AggregateError", "Array", "ArrayBuffer", "BigInt", "BigInt64Array", "BigUint64Array", "Boolean", "DataView",
	"Date", "Error", "EvalError", "Float32Array", "Float64Array", "Function", "Infinity", "Int16Array", "Int32Array",
	"Int8Array", "Iterator", "JSON", "Map", "Math", "NaN", "Number", "Object", "Promise", "Proxy", "RangeError",
	"ReferenceError", "Reflect", "RegExp", "Set", "String", "Symbol", "SyntaxError", "TypeError", "URIError",
	"Uint16Array", "Uint32Array", "Uint8Array", "Uint8ClampedArray", "WeakMap", "WeakSet", "decodeURI",
	"decodeURIComponent", "encodeURI", "encodeURIComponent", "escape", "eval", "globalThis", "isFinite", "isNaN",
	"parseFloat", "parseInt", "undefined", "unescape",
	"Buffer", "TextEncoder", "URL", "URLSearchParams", "atob", "btoa", "console", "setTimeout",
}

func difference(a, b []string) []string {
	var out []string
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

func Test전역_객체의_이름은_허용_목록과_양방향으로_같다(t *testing.T) {
	p := newProbe(SiteRunner)
	if err := runSource(t, SiteRunner, p, `import { record } from "cockpit:flow";

export default function (): void {
	record(Object.getOwnPropertyNames(globalThis).join(","));
}
`); err != nil {
		t.Fatal(err)
	}
	got := strings.Split(p.records[0], ",")
	if missing, extra := difference(wantGlobals, got), difference(got, wantGlobals); len(missing)+len(extra) > 0 || len(got) == 0 {
		t.Fatalf("빠진 전역 %q, 남은 전역 %q", missing, extra)
	}
	table := append(slices.Clone(languageGlobals), hostGlobals...)
	if missing, extra := difference(wantGlobals, table), difference(table, wantGlobals); len(missing)+len(extra) > 0 {
		t.Fatalf("엔진의 허용 표와 어긋난다: 빠짐 %q, 더함 %q", missing, extra)
	}
}

func Test전역_생성자의_프로토타입은_모두_얼어_있다(t *testing.T) {
	p := newProbe(SiteRunner)
	if err := runSource(t, SiteRunner, p, `import { record } from "cockpit:flow";

export default function (): void {
	const g = globalThis as Record<string, unknown>;
	const withProto = Object.getOwnPropertyNames(g).filter((n) => typeof g[n] === "function" && (g[n] as { prototype?: unknown }).prototype);
	record(String(withProto.length));
	record(withProto.filter((n) => !Object.isFrozen((g[n] as { prototype: object }).prototype)).join(","));
	record([Object, Array, Function, Promise, String, Error, Map, Buffer, URL].map((c) => Object.isFrozen(c.prototype)).join(","));
}
`); err != nil {
		t.Fatal(err)
	}
	checked, err := strconv.Atoi(p.records[0])
	if err != nil || checked < 9 {
		t.Fatalf("검사한 프로토타입 %q", p.records[0])
	}
	if p.records[1] != "" || p.records[2] != "true,true,true,true,true,true,true,true,true" {
		t.Fatalf("얼지 않은 프로토타입 %q / 주요 %q", p.records[1], p.records[2])
	}
}

func Test레지스트리는_심은_파일을_디스크에서_읽지_않는다(t *testing.T) {
	abs, rel := plant(t, "planted.json", `{"secret":"PLANTED_REGISTRY"}`)
	reg := newRegistry()
	reg.RegisterNativeModule("canary", func(rt *goja.Runtime, m *goja.Object) {
		_ = m.Get("exports").(*goja.Object).Set("ok", true)
	})
	rm := reg.Enable(goja.New())
	canary, err := rm.Require("canary")
	if err != nil || canary.ToObject(nil).Get("ok").ToBoolean() != true {
		t.Fatalf("네이티브 모듈이 풀리지 않았다(카나리): %v", err)
	}
	specs := []string{abs, rel, strings.TrimSuffix(rel, ".json")}
	for _, spec := range specs {
		v, err := rm.Require(spec)
		if err == nil {
			t.Errorf("%s 가 풀렸다: %v", spec, v.Export())
		} else if strings.Contains(err.Error(), "PLANTED") || !strings.Contains(err.Error(), "Invalid module") {
			t.Errorf("%s: %v", spec, err)
		}
	}
}

func Test가두기_변이_기록표는_이_파일의_시험과_양방향으로_같다(t *testing.T) {
	assertMutationRecord(t, "confine_test.go", confineMutations)
}
