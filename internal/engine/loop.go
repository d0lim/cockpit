package engine

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
)

type Options struct {
	Site    Site
	Budget  Budget
	Modules map[string]ModuleBuilder
	Log     *slog.Logger
}

type Engine struct {
	opts Options
}

type Host struct {
	ctx        context.Context
	rt         *goja.Runtime
	loop       *eventloop.EventLoop
	reg        *require.Registry
	rm         *require.RequireModule
	log        *slog.Logger
	in         intrinsics
	allowed    map[string]bool
	rejections []*goja.Promise
	failure    *Failure
	pending    int
	timers     int
	settled    bool
	done       bool
	finished   bool
}

func New(opts Options) (*Engine, error) {
	if opts.Site.module() == "" {
		return nil, fmt.Errorf("unknown %s", opts.Site)
	}
	budget, err := opts.Budget.normalize()
	if err != nil {
		return nil, err
	}
	opts.Budget = budget
	for name, build := range opts.Modules {
		if !opts.Site.allows(name) {
			return nil, fmt.Errorf("module %q is not available at the %s site", name, opts.Site)
		}
		if build == nil {
			return nil, fmt.Errorf("module %q has a nil builder", name)
		}
	}
	opts.Modules = maps.Clone(opts.Modules)
	if opts.Log == nil {
		opts.Log = slog.New(slog.DiscardHandler)
	}
	return &Engine{opts: opts}, nil
}

func (e *Engine) Run(ctx context.Context, b Bundle, entry string) error {
	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	reg := newRegistry()
	loop := eventloop.NewEventLoop(eventloop.EnableConsole(false), eventloop.WithRegistry(reg))
	h := &Host{ctx: runCtx, loop: loop, reg: reg, log: e.opts.Log, allowed: map[string]bool{}}
	loop.Run(func(rt *goja.Runtime) { h.rt = rt })
	h.registerModules(reg, e.opts.Modules)
	unwatch := h.watch(runCtx, cancel, e.opts.Budget.Deadline)
	loop.RunOnLoop(func(*goja.Runtime) { h.start(b, entry, e.opts.Budget) })
	loop.StartInForeground()
	unwatch()
	h.finished = true
	loop.Terminate()
	return h.result(context.Cause(runCtx))
}

func (h *Host) start(b Bundle, entry string, budget Budget) {
	if err := h.setup(budget); err != nil {
		h.fail(&Failure{Kind: FailLoad, Message: err.Error()})
		return
	}
	run, err := h.evaluate(b, entry)
	if err != nil {
		h.fail(err)
		return
	}
	result, err := run(goja.Undefined())
	if err != nil {
		h.fail(err)
		return
	}
	h.await(result)
}

func (h *Host) setup(budget Budget) error {
	h.rt.SetParserOptions(parser.WithSourceMapLoader(noDiskSourceMap))
	h.rt.SetMaxCallStackSize(budget.MaxCallStack)
	h.rt.SetPromiseRejectionTracker(h.track)
	h.rm = h.reg.Enable(h.rt)
	in, err := captureIntrinsics(h.rt)
	if err != nil {
		return err
	}
	h.in = in
	if err := h.installGlobals(); err != nil {
		return err
	}
	if err := confineGlobals(h.rt); err != nil {
		return err
	}
	return freezePrototypes(h.rt)
}

func (h *Host) evaluate(b Bundle, entry string) (goja.Callable, error) {
	prg, err := compileBundle(b.Code)
	if err != nil {
		return nil, &Failure{Kind: FailLoad, Message: err.Error()}
	}
	wrapper, err := h.rt.RunProgram(prg)
	if err != nil {
		return nil, err
	}
	call, ok := goja.AssertFunction(wrapper)
	if !ok {
		return nil, &Failure{Kind: FailLoad, Message: "the bundle does not evaluate to a module function"}
	}
	exports, err := call(goja.Undefined(), h.rt.ToValue(h.require))
	if err != nil {
		return nil, err
	}
	module, isObject := exports.(*goja.Object)
	var run goja.Callable
	if isObject {
		if ex := h.rt.Try(func() { run, ok = goja.AssertFunction(module.Get(entry)) }); ex != nil {
			return nil, ex
		}
	}
	if !isObject || !ok {
		return nil, &Failure{Kind: FailLoad, Message: fmt.Sprintf("entry export %q is not a function", entry)}
	}
	return run, nil
}

func (h *Host) await(result goja.Value) {
	promise, resolve, _ := h.rt.NewPromise()
	if err := resolve(result); err != nil {
		h.fail(err)
		return
	}
	onSettled := h.rt.ToValue(func(goja.FunctionCall) goja.Value {
		h.settle(nil)
		return goja.Undefined()
	})
	onRejected := h.rt.ToValue(func(call goja.FunctionCall) goja.Value {
		h.settle(h.describe(FailThrown, call.Argument(0)))
		return goja.Undefined()
	})
	_, err := h.in.then(h.rt.ToValue(promise), onSettled, onRejected)
	h.after(err)
}

func (h *Host) settle(failure *Failure) {
	if h.done {
		return
	}
	h.done, h.settled, h.failure = true, true, failure
	h.loop.StopNoWait()
}

func (h *Host) callback(run func() error) {
	if h.done || h.finished {
		return
	}
	h.after(run())
}

func (h *Host) after(err error) {
	switch {
	case err != nil:
		h.fail(err)
	case !h.done && h.pending == 0:
		h.fail(&Failure{Kind: FailUnsettled, Message: "the entry promise is pending with no timer or port call left to settle it"})
	}
}

func (h *Host) fail(err error) {
	if h.done {
		return
	}
	h.done, h.failure = true, h.classify(err)
	h.loop.StopNoWait()
}

func (h *Host) track(p *goja.Promise, op goja.PromiseRejectionOperation) {
	if op == goja.PromiseRejectionReject {
		h.rejections = append(h.rejections, p)
		return
	}
	h.rejections = slices.DeleteFunc(h.rejections, func(q *goja.Promise) bool { return q == p })
}

func (h *Host) result(cause error) error {
	switch {
	case h.failure != nil && h.failure.Kind != failInterrupt:
		return h.failure
	case h.failure != nil || !h.settled:
		return interruption(cause)
	case len(h.rejections) > 0:
		return h.describe(FailUnhandled, h.rejections[0].Result())
	}
	return nil
}

func (h *Host) setTimeout(call goja.FunctionCall) goja.Value {
	fn, ok := goja.AssertFunction(call.Argument(0))
	if !ok {
		panic(h.rt.NewTypeError("setTimeout callback is not a function"))
	}
	ms := call.Argument(1).ToInteger()
	if ms < 0 || ms > math.MaxInt32 {
		ms = 0
	}
	args := slices.Clone(call.Arguments[min(2, len(call.Arguments)):])
	h.pending++
	h.loop.SetTimeout(func(*goja.Runtime) {
		h.pending--
		h.callback(func() error {
			_, err := fn(goja.Undefined(), args...)
			return err
		})
	}, time.Duration(ms)*time.Millisecond)
	h.timers++
	return h.rt.ToValue(h.timers)
}
