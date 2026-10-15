package engine

import (
	"errors"

	"github.com/dop251/goja"
)

const freezePrelude = `(function () {
	"use strict";
	for (const name of Object.getOwnPropertyNames(globalThis)) {
		const value = globalThis[name];
		const proto = typeof value === "function" ? value.prototype : undefined;
		if (proto !== null && (typeof proto === "object" || typeof proto === "function")) Object.freeze(proto);
	}
})();`

type intrinsics struct {
	freeze    goja.Callable
	then      goja.Callable
	parseJSON goja.Callable
	errorCtor *goja.Object
	uint8Ctor *goja.Object
}

var errIntrinsics = errors.New("the runtime is missing a required built-in")

func captureIntrinsics(rt *goja.Runtime) (intrinsics, error) {
	var in intrinsics
	ex := rt.Try(func() {
		object := rt.Get("Object").ToObject(rt)
		promiseProto := rt.Get("Promise").ToObject(rt).Get("prototype").ToObject(rt)
		in.freeze, _ = goja.AssertFunction(object.Get("freeze"))
		in.then, _ = goja.AssertFunction(promiseProto.Get("then"))
		in.parseJSON, _ = goja.AssertFunction(rt.Get("JSON").ToObject(rt).Get("parse"))
		in.errorCtor = rt.Get("Error").ToObject(rt)
		in.uint8Ctor = rt.Get("Uint8Array").ToObject(rt)
	})
	if ex != nil || in.freeze == nil || in.then == nil || in.parseJSON == nil {
		return intrinsics{}, errIntrinsics
	}
	return in, nil
}

func freezePrototypes(rt *goja.Runtime) error {
	_, err := rt.RunString(freezePrelude)
	return err
}

func (h *Host) freezeDeep(v goja.Value) error {
	return h.freezeObject(v, map[*goja.Object]bool{})
}

func (h *Host) freezeObject(v goja.Value, seen map[*goja.Object]bool) error {
	obj, ok := v.(*goja.Object)
	if !ok || seen[obj] {
		return nil
	}
	seen[obj] = true
	for _, key := range obj.GetOwnPropertyNames() {
		var child goja.Value
		if ex := h.rt.Try(func() { child = obj.Get(key) }); ex != nil {
			return ex
		}
		if err := h.freezeObject(child, seen); err != nil {
			return err
		}
	}
	_, err := h.in.freeze(goja.Undefined(), obj)
	return err
}
