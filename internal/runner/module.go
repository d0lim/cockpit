package runner

import (
	"errors"

	"github.com/dop251/goja"

	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/runnerctx"
)

const moduleScript = "cockpit:flow"

const flowModuleJS = `(function (frames, runner, ctx) {
	"use strict";
	const FLOW_ID = /^[a-z][a-z0-9-]{1,40}$/;
	const KINDS = ["request", "wait", "capture"];
	const here = () => frames().filter((f) => f.file !== "cockpit:flow");
	const stepFrames = new WeakMap();
	const stepSource = (list, flowFile) => {
		for (let i = list.length - 1; i >= 0; i--) if (list[i].file === flowFile) return list[i];
		return list[0];
	};
	const suite = (def) => def;
	const stage = (def) => def;
	const step = (def) => {
		if (!KINDS.includes(def.kind)) throw new Error("step " + def.id + " has kind \"" + def.kind + "\", which is not one of " + KINDS.join(", "));
		stepFrames.set(def, here());
		return def;
	};
	const flow = (def) => {
		if (!FLOW_ID.test(def.id)) throw new Error("flow id \"" + def.id + "\" does not match " + FLOW_ID);
		const seen = new Set();
		for (const s of def.steps) {
			if (seen.has(s.id)) throw new Error("flow " + def.id + " declares step id \"" + s.id + "\" twice");
			seen.add(s.id);
		}
		const source = here()[0];
		const steps = def.steps.map((s) => {
			const at = stepSource(stepFrames.get(s) ?? [], source ? source.file : undefined);
			return at ? { ...s, source: at } : s;
		});
		return { ...def, ...(source ? { source } : {}), steps };
	};
	const describe = (def, runs) => ({
		name: def.name,
		stages: def.stages.map((s) => ({ id: s.id, title: s.title, ...(s.notProven ? { notProven: s.notProven } : {}) })),
		flows: def.flows.map((f) => {
			for (const s of f.steps) runs.set(f.id + "\u0000" + s.id, s.run);
			return {
				id: f.id,
				title: f.title,
				stage: f.stage,
				once: f.once === true,
				requires: [...(f.requires ?? [])],
				steps: f.steps.map((s) => ({ id: s.id, title: s.title, kind: s.kind, ...(s.source ? { source: s.source } : {}) })),
				...(f.source ? { source: f.source } : {}),
			};
		}),
	});
	const __drive = async (mod) => {
		const def = mod && typeof mod === "object" ? (mod.default ?? mod.suite) : undefined;
		if (!def || typeof def !== "object" || !Array.isArray(def.stages) || !Array.isArray(def.flows)) {
			throw new Error("the suite module does not export { name, stages, flows }");
		}
		const runs = new Map();
		runner.begin(describe(def, runs));
		for (;;) {
			const next = runner.next();
			if (next === null) return;
			const run = runs.get(next.flow + "\u0000" + next.step);
			let failure = null;
			try {
				if (typeof run !== "function") throw new Error("step " + next.flow + " › " + next.step + " has no run function");
				await run(ctx);
			} catch (e) {
				failure = e instanceof Error ? e.message : String(e);
			}
			runner.finish(next.flow, next.step, failure);
		}
	};
	return { suite, stage, flow, step, __drive };
})`

func Module(d *Driver, ctxDeps *runnerctx.Deps) engine.ModuleBuilder {
	return func(h *engine.Host) (goja.Value, error) {
		ctx, err := runnerctx.New(h, ctxDeps)
		if err != nil {
			return nil, err
		}
		rt := h.Runtime()
		factory, err := rt.RunScript(moduleScript, flowModuleJS)
		if err != nil {
			return nil, err
		}
		call, ok := goja.AssertFunction(factory)
		if !ok {
			return nil, errors.New("the cockpit:flow module script did not evaluate to a function")
		}
		b := &binder{h: h, d: d}
		runnerObj := rt.NewObject()
		for name, fn := range map[string]func(goja.FunctionCall) goja.Value{"begin": b.begin, "next": b.next, "finish": b.finish} {
			if err := runnerObj.Set(name, fn); err != nil {
				return nil, err
			}
		}
		return call(goja.Undefined(), rt.ToValue(b.frames), runnerObj, ctx)
	}
}

type binder struct {
	h *engine.Host
	d *Driver
}

func (b *binder) frames(goja.FunctionCall) goja.Value {
	frames := b.h.Frames()
	if frames == nil {
		frames = []engine.Frame{}
	}
	v, err := b.h.Value(frames)
	if err != nil {
		panic(b.h.Error(err))
	}
	return v
}

func (b *binder) begin(call goja.FunctionCall) goja.Value {
	var decl SuiteDecl
	if err := b.h.DecodeArgs(call.Argument(0), &decl); err != nil {
		panic(b.h.Error(err))
	}
	if err := b.d.Begin(decl); err != nil {
		panic(b.h.Error(portError(err)))
	}
	return goja.Undefined()
}

func (b *binder) next(goja.FunctionCall) goja.Value {
	ref, err := b.d.Next()
	if err != nil {
		panic(b.h.Error(portError(err)))
	}
	if ref == nil {
		return goja.Null()
	}
	v, err := b.h.Value(ref)
	if err != nil {
		panic(b.h.Error(err))
	}
	return v
}

func (b *binder) finish(call goja.FunctionCall) goja.Value {
	ref := StepRef{Flow: call.Argument(0).String(), Step: call.Argument(1).String()}
	var failure *string
	if raw := call.Argument(2); !goja.IsNull(raw) && !goja.IsUndefined(raw) {
		s := raw.String()
		failure = &s
	}
	if err := b.d.Finish(ref, failure); err != nil {
		panic(b.h.Error(portError(err)))
	}
	return goja.Undefined()
}

func portError(err error) error {
	var rej *Rejection
	if errors.As(err, &rej) {
		return &engine.PortError{Code: engine.PortDenied, Message: rejected + rej.Reason}
	}
	var pe *engine.PortError
	if errors.As(err, &pe) {
		return pe
	}
	return &engine.PortError{Code: engine.PortUnavailable, Message: err.Error()}
}
