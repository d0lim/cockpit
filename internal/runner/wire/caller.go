package recorder

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	callerLookupLimit = 300 * time.Millisecond
	callerNameTTL     = 30 * time.Second
)

var (
	oneOffContainer  = regexp.MustCompile(`^(.+)-run-[0-9a-f]{12}$`)
	serviceContainer = regexp.MustCompile(`^(.+)-[0-9]+$`)
)

func serviceOf(project, ptr string) (string, bool) {
	name := strings.TrimSuffix(ptr, ".")
	host, _, _ := strings.Cut(name, ".")
	if at := strings.LastIndex(name, "."+project+"_"); at >= 0 {
		host = name[:at]
	}
	container, ok := strings.CutPrefix(host, project+"-")
	if !ok {
		return "", false
	}
	for _, shape := range []*regexp.Regexp{oneOffContainer, serviceContainer} {
		if m := shape.FindStringSubmatch(container); m != nil {
			return m[1], true
		}
	}
	return "", false
}

type callerLookup struct {
	done chan struct{}
	name string
	at   time.Time
}

type callerNames struct {
	project string
	lookup  func(ctx context.Context, addr string) ([]string, error)

	mu    sync.Mutex
	known map[string]*callerLookup
}

func newCallerNames(project string, lookup func(ctx context.Context, addr string) ([]string, error)) *callerNames {
	return &callerNames{project: project, lookup: lookup, known: map[string]*callerLookup{}}
}

func (n *callerNames) note(addr string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if e, ok := n.known[addr]; ok && (e.at.IsZero() || (e.name != "" && time.Since(e.at) < callerNameTTL)) {
		return
	}
	e := &callerLookup{done: make(chan struct{})}
	n.known[addr] = e
	go n.resolve(addr, e)
}

func (n *callerNames) resolve(addr string, e *callerLookup) {
	ctx, cancel := context.WithTimeout(context.Background(), callerLookupLimit)
	defer cancel()
	defer close(e.done)
	names, err := n.lookup(ctx, addr)
	name := ""
	if err == nil {
		for _, ptr := range names {
			if service, ok := serviceOf(n.project, ptr); ok {
				name = service
				break
			}
		}
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	e.name, e.at = name, time.Now()
}

func (n *callerNames) name(ctx context.Context, addr string) string {
	n.mu.Lock()
	e := n.known[addr]
	n.mu.Unlock()
	if e == nil {
		return addr
	}
	select {
	case <-e.done:
	case <-ctx.Done():
		return addr
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if e.name == "" {
		return addr
	}
	return e.name
}
