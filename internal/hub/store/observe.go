package store

import (
	"path/filepath"
	"sync"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

type Transition struct {
	Before lifecycle.State
	Event  lifecycle.Event
	Detail lifecycle.Detail
	After  Meta
}

type observer struct {
	fn func(Transition)
}

var (
	observersMu sync.Mutex
	observers   = map[string][]*observer{}
)

func homeKey(home string) string {
	abs, err := filepath.Abs(home)
	if err != nil {
		return filepath.Clean(home)
	}
	return abs
}

func Observe(home string, fn func(Transition)) (stop func()) {
	key := homeKey(home)
	o := &observer{fn: fn}
	observersMu.Lock()
	observers[key] = append(observers[key], o)
	observersMu.Unlock()
	return func() {
		observersMu.Lock()
		defer observersMu.Unlock()
		list := observers[key]
		for i, x := range list {
			if x == o {
				observers[key] = append(list[:i:i], list[i+1:]...)
				break
			}
		}
		if len(observers[key]) == 0 {
			delete(observers, key)
		}
	}
}

func notify(home string, t Transition) {
	observersMu.Lock()
	list := append([]*observer(nil), observers[homeKey(home)]...)
	observersMu.Unlock()
	for _, o := range list {
		o.fn(t)
	}
}
