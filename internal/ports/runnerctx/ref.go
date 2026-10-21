package runnerctx

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/dop251/goja"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports"
)

const (
	maxRefs     = 50
	maxRefValue = 200
)

var refName = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,31}$`)

func (d *Deps) secretLike(value string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, v := range d.Priv {
		if v != "" && strings.Contains(value, v) {
			return true
		}
	}
	return d.Patterns != nil && d.Patterns(value) != value
}

func (d *Deps) addRef(name, value string) error {
	if !refName.MatchString(name) {
		return ports.BadArgs(d.Mask, "ref name must match ^[a-z][A-Za-z0-9]{0,31}$")
	}
	if value == "" || len(value) > maxRefValue || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return ports.BadArgs(d.Mask, "ref "+name+" must be 1 to 200 characters without control characters")
	}
	if d.secretLike(value) {
		return ports.BadArgs(d.Mask, "ref "+name+" looks like a credential and stays masked")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.refs) >= maxRefs {
		return ports.Limit(d.Mask, "a step keeps at most 50 refs")
	}
	if d.public == nil {
		d.public = map[string]bool{}
	}
	d.public[value] = true
	d.refs = append(d.refs, contract.StepRef{Name: name, Value: value})
	return nil
}

func (b *binder) ref(call goja.FunctionCall) goja.Value {
	if err := b.d.addRef(b.str(call, 0, "name"), b.str(call, 1, "value")); err != nil {
		return b.throw(err)
	}
	b.d.changed()
	return goja.Undefined()
}
