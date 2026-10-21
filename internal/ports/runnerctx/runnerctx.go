package runnerctx

import (
	"maps"
	"slices"
	"sync"

	"github.com/dop251/goja"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports"
	"github.com/d0lim/cockpit/internal/ports/capture"
	"github.com/d0lim/cockpit/internal/ports/clock"
	cryptoport "github.com/d0lim/cockpit/internal/ports/crypto"
	httpport "github.com/d0lim/cockpit/internal/ports/http"
	"github.com/d0lim/cockpit/internal/ports/source"
)

type Deps struct {
	Env      map[string]any
	Priv     map[string]string
	PrivPath string
	SeedKeys map[string]string
	HTTP     *httpport.Client
	Context  *capture.Context
	Source   *source.Reader
	Clock    clock.Clock
	Keys     *cryptoport.Keys
	Mask     ports.Mask
	Patterns ports.Mask
	Changed  func()

	mu     sync.Mutex
	public map[string]bool
	refs   []contract.StepRef
}

func (d *Deps) KnownValues() []string {
	d.mu.Lock()
	values := slices.Collect(maps.Values(d.Priv))
	for _, v := range d.Context.Captures.Values() {
		if !d.public[v] {
			values = append(values, v)
		}
	}
	d.mu.Unlock()
	slices.Sort(values)
	return slices.Compact(values)
}

func (d *Deps) TakeRefs() []contract.StepRef {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.refs
	d.refs = nil
	return out
}

func (d *Deps) seed(loginID string) (string, bool) {
	key, ok := d.SeedKeys[loginID]
	if !ok {
		return "", false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.Priv[key], true
}

func (d *Deps) replacePriv(local map[string]string) {
	d.mu.Lock()
	d.Priv = local
	d.mu.Unlock()
}

func (d *Deps) changed() {
	if d.Changed != nil {
		d.Changed()
	}
}

const helpersJS = `(function () {
	const deepFreeze = (o) => {
		if (o !== null && typeof o === "object") {
			for (const k of Object.keys(o)) deepFreeze(o[k]);
			Object.freeze(o);
		}
		return o;
	};
	const fill = (arr, ab) => {
		new Uint8Array(arr.buffer, arr.byteOffset, arr.byteLength).set(new Uint8Array(ab));
		return arr;
	};
	const bytesOf = (v) => {
		if (v instanceof ArrayBuffer) return v;
		if (ArrayBuffer.isView(v)) return v.buffer.slice(v.byteOffset, v.byteOffset + v.byteLength);
		return null;
	};
	return { deepFreeze, fill, bytesOf };
})()`

type binder struct {
	h          *engine.Host
	rt         *goja.Runtime
	d          *Deps
	deepFreeze goja.Callable
	fill       goja.Callable
	bytesOf    goja.Callable
}

func New(h *engine.Host, d *Deps) (goja.Value, error) {
	rt := h.Runtime()
	if d.Mask == nil {
		d.Mask = ports.Identity
	}
	if d.Env == nil {
		d.Env = map[string]any{}
	}
	if d.Priv == nil {
		d.Priv = map[string]string{}
	}
	if d.Keys == nil {
		d.Keys = cryptoport.NewKeys(d.Mask)
	}
	if d.Clock.Now == nil {
		d.Clock = clock.System()
	}
	helpers, err := rt.RunString(helpersJS)
	if err != nil {
		return nil, err
	}
	b := &binder{h: h, rt: rt, d: d}
	obj := helpers.ToObject(rt)
	for name, target := range map[string]*goja.Callable{"deepFreeze": &b.deepFreeze, "fill": &b.fill, "bytesOf": &b.bytesOf} {
		fn, ok := goja.AssertFunction(obj.Get(name))
		if !ok {
			return nil, &engine.Failure{Kind: engine.FailLoad, Message: "runner ctx helper " + name + " is missing"}
		}
		*target = fn
	}
	ctx, err := b.build()
	if err != nil {
		return nil, err
	}
	if _, err := b.deepFreeze(goja.Undefined(), ctx); err != nil {
		return nil, err
	}
	return ctx, nil
}

func (b *binder) build() (*goja.Object, error) {
	env, err := b.h.Value(b.d.Env)
	if err != nil {
		return nil, err
	}
	ctx := b.rt.NewObject()
	http := b.rt.NewObject()
	crypto := b.rt.NewObject()
	subtle := b.rt.NewObject()
	sets := []struct {
		obj  *goja.Object
		name string
		v    any
	}{
		{ctx, "env", env},
		{http, "send", b.send}, {ctx, "http", http},
		{ctx, "capture", b.capture}, {ctx, "get", b.get}, {ctx, "ref", b.ref},
		{ctx, "totp", b.totp}, {ctx, "saveSeed", b.saveSeed},
		{crypto, "randomUUID", b.randomUUID}, {crypto, "getRandomValues", b.getRandomValues},
		{subtle, "generateKey", b.generateKey}, {subtle, "sign", b.sign},
		{crypto, "subtle", subtle}, {ctx, "crypto", crypto},
		{ctx, "readSource", b.readSource}, {ctx, "listSource", b.listSource},
		{ctx, "clock", b.clock}, {ctx, "sleep", b.sleep},
	}
	for _, s := range sets {
		if err := s.obj.Set(s.name, s.v); err != nil {
			return nil, err
		}
	}
	if err := ctx.DefineAccessorProperty("priv", b.rt.ToValue(b.priv), nil, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return nil, err
	}
	return ctx, nil
}

func (b *binder) priv(goja.FunctionCall) goja.Value {
	b.d.mu.Lock()
	snapshot, err := b.h.Value(b.d.Priv)
	b.d.mu.Unlock()
	if err != nil {
		return b.throw(err)
	}
	frozen, err := b.deepFreeze(goja.Undefined(), snapshot)
	if err != nil {
		return b.throw(ports.Unavailable(b.d.Mask, "failed to snapshot private env"))
	}
	return frozen
}

func (b *binder) str(call goja.FunctionCall, i int, name string) string {
	s, ok := call.Argument(i).Export().(string)
	if !ok {
		panic(b.h.Error(ports.BadArgs(b.d.Mask, name+" must be a string")))
	}
	return s
}

func (b *binder) throw(err error) goja.Value {
	panic(b.h.Error(err))
}
