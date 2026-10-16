package engine

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

var confineMutations = []mutation{
	{"global allowlist", "make setup skip confineGlobals", "red: GoError, require, setInterval, setImmediate and clear* remain", "TestGlobalObjectNamesAndTheAllowListMatchBothWays"},
	{"core prototype freeze", "make setup skip freezePrototypes", "red: unfrozen prototypes from Object to URLSearchParams are listed", "TestEveryGlobalConstructorPrototypeIsFrozen"},
	{"require loader that never reads the disk", "pass require.DefaultSourceLoader to WithLoader", "red: absolute, relative and extensionless paths all return PLANTED_REGISTRY", "TestRegistryDoesNotReadPlantedFilesFromDisk"},
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

func TestGlobalObjectNamesAndTheAllowListMatchBothWays(t *testing.T) {
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
		t.Fatalf("missing globals %q, extra globals %q", missing, extra)
	}
	table := append(slices.Clone(languageGlobals), hostGlobals...)
	if missing, extra := difference(wantGlobals, table), difference(table, wantGlobals); len(missing)+len(extra) > 0 {
		t.Fatalf("engine allow table differs: missing %q, extra %q", missing, extra)
	}
}

func TestEveryGlobalConstructorPrototypeIsFrozen(t *testing.T) {
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
		t.Fatalf("checked prototypes %q", p.records[0])
	}
	if p.records[1] != "" || p.records[2] != "true,true,true,true,true,true,true,true,true" {
		t.Fatalf("unfrozen prototypes %q, core prototypes frozen %q", p.records[1], p.records[2])
	}
}

func TestRegistryDoesNotReadPlantedFilesFromDisk(t *testing.T) {
	abs, rel := plant(t, "planted.json", `{"secret":"PLANTED_REGISTRY"}`)
	reg := newRegistry()
	reg.RegisterNativeModule("canary", func(rt *goja.Runtime, m *goja.Object) {
		_ = m.Get("exports").(*goja.Object).Set("ok", true)
	})
	rm := reg.Enable(goja.New())
	canary, err := rm.Require("canary")
	if err != nil || canary.ToObject(nil).Get("ok").ToBoolean() != true {
		t.Fatalf("native module did not resolve (canary): %v", err)
	}
	specs := []string{abs, rel, strings.TrimSuffix(rel, ".json")}
	for _, spec := range specs {
		v, err := rm.Require(spec)
		if err == nil {
			t.Errorf("%s resolved: %v", spec, v.Export())
		} else if strings.Contains(err.Error(), "PLANTED") || !strings.Contains(err.Error(), "Invalid module") {
			t.Errorf("%s: %v", spec, err)
		}
	}
}

func TestConfineMutationRecordAndTestsInThisFileMatchBothWays(t *testing.T) {
	assertMutationRecord(t, "confine_test.go", confineMutations)
}
