package runnerctx

import (
	"context"

	"github.com/dop251/goja"

	"github.com/d0lim/cockpit/internal/ports"
	cryptoport "github.com/d0lim/cockpit/internal/ports/crypto"
	httpport "github.com/d0lim/cockpit/internal/ports/http"
	"github.com/d0lim/cockpit/internal/ports/totp"
)

const rsaPKCS1 = "RSASSA-PKCS1-v1_5"

func (b *binder) send(call goja.FunctionCall) goja.Value {
	var req httpport.Request
	method, url := b.str(call, 0, "method"), b.str(call, 1, "url")
	if err := b.h.DecodeArgs(call.Argument(2), &req); err != nil {
		return b.throw(err)
	}
	req.Method, req.URL = method, url
	return b.h.Async(func(ctx context.Context) (any, error) { return b.d.HTTP.Send(ctx, req) })
}

func (b *binder) capture(call goja.FunctionCall) goja.Value {
	name, value := b.str(call, 0, "name"), b.str(call, 1, "value")
	var opts struct {
		Persist bool `json:"persist"`
	}
	if err := b.h.DecodeArgs(call.Argument(2), &opts); err != nil {
		return b.throw(err)
	}
	b.d.Context.Captures.Set(name, value, opts.Persist)
	if opts.Persist {
		if err := b.d.Context.Save(); err != nil {
			return b.throw(ports.Unavailable(b.d.Mask, "failed to write capture "+name+" to context.json"))
		}
	}
	b.d.changed()
	return goja.Undefined()
}

func (b *binder) get(call goja.FunctionCall) goja.Value {
	value, err := b.d.Context.Captures.Get(b.str(call, 0, "name"))
	if err != nil {
		return b.throw(err)
	}
	return b.rt.ToValue(value)
}

func (b *binder) totp(call goja.FunctionCall) goja.Value {
	login := b.str(call, 0, "loginId")
	issuer := totp.Issuer{Seeds: b.d.seed, Clock: b.d.Clock, Sleep: b.d.Clock.Sleep, Steps: b.d.Context, Mask: b.d.Mask}
	return b.h.Async(func(ctx context.Context) (any, error) { return issuer.Issue(ctx, login) })
}

func (b *binder) saveSeed(call goja.FunctionCall) goja.Value {
	login, secret := b.str(call, 0, "loginId"), b.str(call, 1, "secret")
	key, ok := b.d.SeedKeys[login]
	if !ok {
		return b.throw(ports.BadArgs(b.d.Mask, login+" is not an account in the seed key table"))
	}
	if b.d.PrivPath == "" {
		return b.throw(ports.Unavailable(b.d.Mask, "cannot save the seed: no private env path"))
	}
	local, err := totp.SeedFile{Path: b.d.PrivPath}.Save(key, secret)
	if err != nil {
		return b.throw(ports.Unavailable(b.d.Mask, "failed to write the seed for "+login+" to private env"))
	}
	b.d.replacePriv(local)
	b.d.changed()
	return goja.Undefined()
}

func (b *binder) randomUUID(goja.FunctionCall) goja.Value {
	return b.rt.ToValue(cryptoport.RandomUUID())
}

func (b *binder) getRandomValues(call goja.FunctionCall) goja.Value {
	arr, ok := call.Argument(0).(*goja.Object)
	if !ok || goja.IsUndefined(arr.Get("byteLength")) {
		return b.throw(ports.BadArgs(b.d.Mask, "getRandomValues takes a typed array"))
	}
	bytes, err := cryptoport.RandomBytes(b.d.Mask, int(arr.Get("byteLength").ToInteger()))
	if err != nil {
		return b.throw(err)
	}
	filled, err := b.fill(goja.Undefined(), arr, b.rt.ToValue(b.rt.NewArrayBuffer(bytes)))
	if err != nil {
		return b.throw(ports.BadArgs(b.d.Mask, "getRandomValues takes a typed array"))
	}
	return filled
}

func (b *binder) generateKey(call goja.FunctionCall) goja.Value {
	var alg struct {
		Name          string `json:"name"`
		ModulusLength int    `json:"modulusLength"`
		Hash          string `json:"hash"`
	}
	decodeErr := b.h.DecodeArgs(call.Argument(0), &alg)
	return b.h.Async(func(ctx context.Context) (any, error) {
		if decodeErr != nil {
			return nil, decodeErr
		}
		if alg.Name != rsaPKCS1 {
			return nil, ports.BadArgs(b.d.Mask, "unsupported algorithm "+alg.Name)
		}
		return b.d.Keys.Generate(ctx, alg.ModulusLength, alg.Hash)
	})
}

func (b *binder) sign(call goja.FunctionCall) goja.Value {
	promise, resolve, reject := b.rt.NewPromise()
	fail := func(err error) goja.Value {
		_ = reject(b.h.Error(err))
		return b.rt.ToValue(promise)
	}
	var alg struct {
		Name string `json:"name"`
	}
	if name, ok := call.Argument(0).Export().(string); ok {
		alg.Name = name
	} else if err := b.h.DecodeArgs(call.Argument(0), &alg); err != nil {
		return fail(err)
	}
	if alg.Name != rsaPKCS1 {
		return fail(ports.BadArgs(b.d.Mask, "unsupported algorithm "+alg.Name))
	}
	var key cryptoport.KeyRef
	if err := b.h.DecodeArgs(call.Argument(1), &key); err != nil {
		return fail(err)
	}
	raw, err := b.bytesOf(goja.Undefined(), call.Argument(2))
	if err != nil {
		return fail(ports.BadArgs(b.d.Mask, "sign data must be an ArrayBuffer or a typed array"))
	}
	buf, ok := raw.Export().(goja.ArrayBuffer)
	if !ok {
		return fail(ports.BadArgs(b.d.Mask, "sign data must be an ArrayBuffer or a typed array"))
	}
	sig, err := b.d.Keys.Sign(key.ID, buf.Bytes())
	if err != nil {
		return fail(err)
	}
	_ = resolve(b.rt.ToValue(b.rt.NewArrayBuffer(sig)))
	return b.rt.ToValue(promise)
}

func (b *binder) readSource(call goja.FunctionCall) goja.Value {
	name, rel := b.str(call, 0, "source"), b.str(call, 1, "path")
	if b.d.Source == nil {
		return b.throw(ports.Denied(b.d.Mask, "no source is declared"))
	}
	text, err := b.d.Source.Read(name, rel)
	if err != nil {
		return b.throw(err)
	}
	return b.rt.ToValue(text)
}

func (b *binder) listSource(call goja.FunctionCall) goja.Value {
	name, rel := b.str(call, 0, "source"), b.str(call, 1, "dir")
	if b.d.Source == nil {
		return b.throw(ports.Denied(b.d.Mask, "no source is declared"))
	}
	names, err := b.d.Source.List(name, rel)
	if err != nil {
		return b.throw(err)
	}
	value, err := b.h.Value(names)
	if err != nil {
		return b.throw(err)
	}
	return value
}

func (b *binder) clock(goja.FunctionCall) goja.Value {
	return b.rt.ToValue(b.d.Clock.Millis())
}

func (b *binder) sleep(call goja.FunctionCall) goja.Value {
	ms := call.Argument(0).ToInteger()
	return b.h.Async(func(ctx context.Context) (any, error) { return nil, b.d.Clock.Sleep(ctx, ms) })
}
