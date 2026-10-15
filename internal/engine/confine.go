package engine

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/buffer"
	nodeconsole "github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/url"
)

var languageGlobals = []string{
	"AggregateError", "Array", "ArrayBuffer", "BigInt", "BigInt64Array", "BigUint64Array", "Boolean", "DataView",
	"Date", "Error", "EvalError", "Float32Array", "Float64Array", "Function", "Infinity", "Int16Array", "Int32Array",
	"Int8Array", "Iterator", "JSON", "Map", "Math", "NaN", "Number", "Object", "Promise", "Proxy", "RangeError",
	"ReferenceError", "Reflect", "RegExp", "Set", "String", "Symbol", "SyntaxError", "TypeError", "URIError",
	"Uint16Array", "Uint32Array", "Uint8Array", "Uint8ClampedArray", "WeakMap", "WeakSet", "decodeURI",
	"decodeURIComponent", "encodeURI", "encodeURIComponent", "escape", "eval", "globalThis", "isFinite", "isNaN",
	"parseFloat", "parseInt", "undefined", "unescape",
}

var hostGlobals = []string{"Buffer", "TextEncoder", "URL", "URLSearchParams", "atob", "btoa", "console", "setTimeout"}

type printer struct{ log *slog.Logger }

func (p printer) Log(s string)   { p.log.Info("console", slog.String("text", s)) }
func (p printer) Warn(s string)  { p.log.Warn("console", slog.String("text", s)) }
func (p printer) Error(s string) { p.log.Error("console", slog.String("text", s)) }

func (h *Host) installGlobals() error {
	rt := h.rt
	module := rt.NewObject()
	if err := module.Set("exports", rt.NewObject()); err != nil {
		return err
	}
	if ex := rt.Try(func() {
		nodeconsole.RequireWithPrinter(printer{h.log})(rt, module)
		buffer.Enable(rt)
		url.Enable(rt)
	}); ex != nil {
		return ex
	}
	encoder, err := h.textEncoder()
	if err != nil {
		return err
	}
	for name, value := range map[string]any{
		"console":     module.Get("exports"),
		"TextEncoder": encoder,
		"atob":        h.atob,
		"btoa":        h.btoa,
		"setTimeout":  h.setTimeout,
	} {
		if err := rt.Set(name, value); err != nil {
			return err
		}
	}
	return nil
}

func confineGlobals(rt *goja.Runtime) error {
	keep := map[string]bool{}
	for _, name := range append(append([]string{}, languageGlobals...), hostGlobals...) {
		keep[name] = true
	}
	global := rt.GlobalObject()
	for _, name := range global.GetOwnPropertyNames() {
		if keep[name] {
			continue
		}
		if err := global.Delete(name); err != nil {
			return fmt.Errorf("remove global %s: %w", name, err)
		}
	}
	return nil
}

func (h *Host) textEncoder() (*goja.Object, error) {
	rt := h.rt
	ctor := rt.ToValue(func(goja.ConstructorCall) *goja.Object { return nil }).ToObject(rt)
	proto := ctor.Get("prototype").ToObject(rt)
	encode := func(call goja.FunctionCall) goja.Value {
		text := ""
		if arg := call.Argument(0); !goja.IsUndefined(arg) {
			text = arg.String()
		}
		bytes, err := rt.New(h.in.uint8Ctor, rt.ToValue(rt.NewArrayBuffer([]byte(text))))
		if err != nil {
			panic(err)
		}
		return bytes
	}
	if err := proto.Set("encode", encode); err != nil {
		return nil, err
	}
	return ctor, proto.Set("encoding", "utf-8")
}

func (h *Host) btoa(call goja.FunctionCall) goja.Value {
	text := call.Argument(0).String()
	raw := make([]byte, 0, len(text))
	for _, r := range text {
		if r > 0xff {
			panic(h.newError("btoa: the string has characters outside of the Latin1 range"))
		}
		raw = append(raw, byte(r))
	}
	return h.rt.ToValue(base64.StdEncoding.EncodeToString(raw))
}

func (h *Host) atob(call goja.FunctionCall) goja.Value {
	text := strings.Map(func(r rune) rune {
		if strings.ContainsRune("\t\n\f\r ", r) {
			return -1
		}
		return r
	}, call.Argument(0).String())
	if len(text)%4 == 0 {
		text = strings.TrimSuffix(strings.TrimSuffix(text, "="), "=")
	}
	raw, err := base64.RawStdEncoding.DecodeString(text)
	if err != nil {
		panic(h.newError("atob: the string is not correctly encoded"))
	}
	latin := make([]rune, len(raw))
	for i, b := range raw {
		latin[i] = rune(b)
	}
	return h.rt.ToValue(string(latin))
}
