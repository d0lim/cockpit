package engine

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/dop251/goja"
)

type PortCode string

const (
	PortDenied      PortCode = "PORT_DENIED"
	PortTimeout     PortCode = "PORT_TIMEOUT"
	PortLimit       PortCode = "PORT_LIMIT"
	PortUnavailable PortCode = "PORT_UNAVAILABLE"
	PortBadArgs     PortCode = "PORT_BAD_ARGS"
)

const portFailed = "port call failed"

type PortError struct {
	Code    PortCode
	Message string
}

func (e *PortError) Error() string {
	return string(e.Code) + ": " + e.Message
}

func (h *Host) Runtime() *goja.Runtime {
	return h.rt
}

func (h *Host) Error(err error) *goja.Object {
	known := &PortError{Code: PortUnavailable, Message: portFailed}
	errors.As(err, &known)
	obj := h.newError(known.Message)
	_ = obj.Set("code", string(known.Code))
	return obj
}

func (h *Host) Value(v any) (goja.Value, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, &PortError{Code: PortUnavailable, Message: "port result is not serializable"}
	}
	return h.in.parseJSON(goja.Undefined(), h.rt.ToValue(string(raw)))
}

func (h *Host) DecodeArgs(v goja.Value, target any) error {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	var exported any
	if ex := h.rt.Try(func() { exported = v.Export() }); ex != nil {
		return &PortError{Code: PortBadArgs, Message: "port arguments could not be read"}
	}
	raw, err := json.Marshal(exported)
	if err != nil {
		return &PortError{Code: PortBadArgs, Message: "port arguments are not serializable"}
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return &PortError{Code: PortBadArgs, Message: "port arguments do not match the expected shape"}
	}
	return nil
}

func (h *Host) Async(work func(context.Context) (any, error)) goja.Value {
	promise, resolve, reject := h.rt.NewPromise()
	h.pending++
	go func() {
		value, err := work(h.ctx)
		h.loop.RunOnLoop(func(*goja.Runtime) {
			h.pending--
			h.callback(func() error {
				if err != nil {
					return reject(h.Error(err))
				}
				jsValue, convErr := h.Value(value)
				if convErr != nil {
					return reject(h.Error(convErr))
				}
				return resolve(jsValue)
			})
		})
	}()
	return h.rt.ToValue(promise)
}

func (h *Host) newError(message string) *goja.Object {
	obj, err := h.rt.New(h.in.errorCtor, h.rt.ToValue(message))
	if err != nil {
		return h.rt.NewTypeError(message)
	}
	return obj
}
