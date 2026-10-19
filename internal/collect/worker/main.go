package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/dop251/goja"

	"github.com/d0lim/cockpit/internal/engine"
)

const (
	setupMax   = 64 << 20
	moduleName = "cockpit:collect"
	entryName  = "default"
)

const collectModuleJS = `(function (call, callSync, expected) {
	"use strict";
	const ID = /^[a-z0-9][a-z0-9-]{0,62}$/;
	const freeze = (o) => {
		for (const v of Object.values(o)) if (v !== null && typeof v === "object") freeze(v);
		return Object.freeze(o);
	};
	const async_ = (port, method) => (...args) => call(port, method, args);
	const sync_ = (port, method) => (...args) => {
		callSync(port, method, args);
	};
	const ctx = freeze({
		sql: { query: async_("sql", "query") },
		docker: { ps: async_("docker", "ps"), logs: async_("docker", "logs"), stats: async_("docker", "stats"), file: async_("docker", "file"), exec: async_("docker", "exec") },
		http: { send: async_("http", "send") },
		signal: { metric: sync_("signal", "metric"), event: sync_("signal", "event"), span: sync_("signal", "span") },
		state: {
			get: (...args) => call("state", "get", args).then((v) => (v === null ? undefined : v)),
			set: async (...args) => {
				await call("state", "set", args);
			},
		},
	});
	const collector = (def) => {
		if (!def || typeof def !== "object" || typeof def.run !== "function") throw new Error("collector needs { id, run }");
		if (!ID.test(def.id)) throw new Error("collector id \"" + def.id + "\" does not match " + ID);
		return def;
	};
	const __run = async (mod) => {
		const def = mod && typeof mod === "object" ? (mod.default ?? mod.collector) : undefined;
		if (!def || typeof def.run !== "function") throw new Error("the collector module does not export collector({ id, run })");
		if (def.id !== expected) throw new Error("collector id \"" + def.id + "\" does not match the declared id \"" + expected + "\"");
		await def.run(ctx);
	};
	return { collector, __run };
})`

func Driver(entry string) string {
	return "import * as mod from \"./" + entry + "\";\nimport { __run } from \"cockpit:collect\";\nexport default () => __run(mod);\n"
}

func Bundle(ctx context.Context, root, entry string) (string, error) {
	b, err := engine.BuildBundle(ctx, engine.BundleInput{Driver: Driver(entry), Root: root, Site: engine.SiteHub})
	return b.Code, err
}

func Main(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) int {
	in := bufio.NewReaderSize(stdin, 1<<16)
	line, err := readLine(in, setupMax)
	if err != nil {
		fmt.Fprintln(stderr, "script host: cannot read the setup:", err)
		return 2
	}
	var setup Setup
	if err := json.Unmarshal(line, &setup); err != nil || setup.DeadlineMs <= 0 {
		fmt.Fprintln(stderr, "script host: the setup is not valid")
		return 2
	}
	c := newClient(stdout)
	go c.serve(in)
	done := run(ctx, setup, c, slog.New(slog.NewTextHandler(stderr, nil)))
	if err := c.out.send(Frame{Done: &done}); err != nil {
		return 2
	}
	if done.OK {
		return 0
	}
	return 1
}

func run(ctx context.Context, setup Setup, c *client, log *slog.Logger) Done {
	eng, err := engine.New(engine.Options{
		Site:    engine.SiteHub,
		Budget:  engine.Budget{Deadline: time.Duration(setup.DeadlineMs) * time.Millisecond, MaxCallStack: setup.MaxCallStack},
		Modules: map[string]engine.ModuleBuilder{moduleName: module(c, setup.Collector)},
		Log:     log,
	})
	if err != nil {
		return Done{Kind: string(engine.FailLoad), Message: err.Error()}
	}
	if err := eng.Run(ctx, engine.Bundle{Code: setup.Code}, entryName); err != nil {
		var f *engine.Failure
		if errors.As(err, &f) {
			return Done{Kind: string(f.Kind), Message: f.Message}
		}
		return Done{Kind: string(engine.FailLoad), Message: err.Error()}
	}
	return Done{OK: true}
}

func module(c *client, expected string) engine.ModuleBuilder {
	return func(h *engine.Host) (goja.Value, error) {
		rt := h.Runtime()
		factory, err := rt.RunScript(moduleName, collectModuleJS)
		if err != nil {
			return nil, err
		}
		build, ok := goja.AssertFunction(factory)
		if !ok {
			return nil, errors.New("the cockpit:collect module script did not evaluate to a function")
		}
		b := binder{h: h, c: c}
		return build(goja.Undefined(), rt.ToValue(b.async), rt.ToValue(b.sync), rt.ToValue(expected))
	}
}

type binder struct {
	h *engine.Host
	c *client
}

func (b binder) args(call goja.FunctionCall) (string, string, []json.RawMessage) {
	port, method := call.Argument(0).String(), call.Argument(1).String()
	var list []any
	if err := b.h.DecodeArgs(call.Argument(2), &list); err != nil {
		panic(b.h.Error(err))
	}
	out := make([]json.RawMessage, len(list))
	for i, v := range list {
		raw, err := json.Marshal(v)
		if err != nil {
			panic(b.h.Error(&engine.PortError{Code: engine.PortBadArgs, Message: "port arguments are not serializable"}))
		}
		out[i] = raw
	}
	return port, method, out
}

func (b binder) async(call goja.FunctionCall) goja.Value {
	port, method, args := b.args(call)
	return b.h.Async(func(ctx context.Context) (any, error) {
		raw, err := b.c.call(ctx, port, method, args)
		if err != nil {
			return nil, err
		}
		return raw, nil
	})
}

func (b binder) sync(call goja.FunctionCall) goja.Value {
	port, method, args := b.args(call)
	if _, err := b.c.call(context.Background(), port, method, args); err != nil {
		panic(b.h.Error(err))
	}
	return goja.Undefined()
}

func readLine(r *bufio.Reader, max int) ([]byte, error) {
	var out []byte
	for {
		chunk, err := r.ReadSlice('\n')
		out = append(out, chunk...)
		if len(out) > max {
			return nil, fmt.Errorf("line exceeds %d bytes", max)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return out, err
	}
}
