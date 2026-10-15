package engine

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
)

type Site int

const (
	SiteRunner Site = iota + 1
	SiteHub
)

const (
	ModuleFlow    = "cockpit:flow"
	ModuleCollect = "cockpit:collect"
	modulePrefix  = "cockpit:"
)

type ModuleBuilder func(h *Host) (goja.Value, error)

func (s Site) String() string {
	switch s {
	case SiteRunner:
		return "runner"
	case SiteHub:
		return "hub"
	}
	return fmt.Sprintf("site(%d)", int(s))
}

func (s Site) module() string {
	switch s {
	case SiteRunner:
		return ModuleFlow
	case SiteHub:
		return ModuleCollect
	}
	return ""
}

func (s Site) allows(name string) bool {
	return name != "" && name == s.module()
}

func newRegistry() *require.Registry {
	return require.NewRegistry(require.WithLoader(func(string) ([]byte, error) {
		return nil, require.ModuleFileDoesNotExistError
	}))
}

func (h *Host) registerModules(reg *require.Registry, modules map[string]ModuleBuilder) {
	for name, build := range modules {
		reg.RegisterNativeModule(name, h.loadModule(name, build))
		h.allowed[name] = true
	}
}

func (h *Host) loadModule(name string, build ModuleBuilder) require.ModuleLoader {
	return func(_ *goja.Runtime, module *goja.Object) {
		exports, err := build(h)
		if err == nil {
			err = h.freezeDeep(exports)
		}
		if err == nil {
			err = module.Set("exports", exports)
		}
		if err != nil {
			panic(h.Error(&PortError{Code: PortUnavailable, Message: fmt.Sprintf("module %q failed to load", name)}))
		}
	}
}

func (h *Host) require(call goja.FunctionCall) goja.Value {
	name := call.Argument(0).String()
	if !h.allowed[name] {
		panic(h.newError(fmt.Sprintf("Invalid module %q", name)))
	}
	exports, err := h.rm.Require(name)
	if err != nil {
		panic(h.Error(err))
	}
	return exports
}
