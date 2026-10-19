package envops

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"

	"github.com/d0lim/cockpit/internal/collect"
	"github.com/d0lim/cockpit/internal/collect/observer"
	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/procgroup"
	"github.com/d0lim/cockpit/internal/hub/runfiles"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports"
	dockerport "github.com/d0lim/cockpit/internal/ports/docker"
	httpport "github.com/d0lim/cockpit/internal/ports/http"
	signalport "github.com/d0lim/cockpit/internal/ports/signal"
	sqlport "github.com/d0lim/cockpit/internal/ports/sql"
	stateport "github.com/d0lim/cockpit/internal/ports/state"
	"github.com/d0lim/cockpit/internal/signal"
)

func DatabaseTargets(decl contract.Declaration, runPorts map[string]int) (map[string]sqlport.Target, error) {
	targets := map[string]sqlport.Target{}
	for i, db := range decl.Databases {
		host, ok := runPorts[string(db.Port)]
		if !ok || host <= 0 {
			return nil, fmt.Errorf("databases[%d].port names %q, which has no published port in this run", i, db.Port)
		}
		targets[string(db.Name)] = sqlport.Target{Host: "127.0.0.1", Port: host, RootUser: db.RootUser, Database: db.Database}
	}
	return targets, nil
}

func secretLookup(r *Run) func(string) (string, bool) {
	values := map[string]string{}
	for _, s := range r.Decl.Secrets.Sources {
		if s.Format != contract.SecretFormatDotenv || r.Root == nil {
			continue
		}
		data, err := runfiles.Read(r.Root, path.Join("src", string(s.Path)), runfiles.MaxBytes)
		if err != nil {
			continue
		}
		for k, v := range mask.DotenvValues(string(data)) {
			values[k] = v
		}
	}
	return func(name string) (string, bool) {
		v, ok := values[name]
		return v, ok && v != ""
	}
}

func (o *Ops) register(ctx context.Context, id string) error {
	if o.Collect == nil {
		return ErrCollectionOff
	}
	c := o.entry(id)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.live {
		return nil
	}
	r, err := o.Open(id)
	if err != nil {
		return err
	}
	defer r.Close()
	if r.Decl == nil {
		return fmt.Errorf("run %s has no declaration copy", id)
	}
	if c.defs == nil {
		if c.defs, err = compose.CollectorDefs(ctx, *r.Decl, filepath.Join(r.Layout.Root, "work", "scripts")); err != nil {
			return err
		}
	}
	m := func(s string) string { return o.Mask(id, s) }
	if o.Streams == nil {
		return errors.New("the hub has no signal stream registry")
	}
	stream, err := o.Streams.Get(id)
	if err != nil {
		return err
	}
	cfg, closers, err := o.runConfig(id, r, c, stream, m)
	if err != nil {
		return err
	}
	if err := o.Collect.Register(cfg); err != nil {
		for _, close := range closers {
			close()
		}
		return err
	}
	c.closers, c.live = closers, true
	return nil
}

func (o *Ops) runConfig(id string, r *Run, c *collected, stream *signal.Stream, m ports.Mask) (collect.RunConfig, []func() error, error) {
	docker := o.DockerFor(procgroup.Hooks{})
	runPorts := o.RunPorts(r)
	dport, err := dockerport.New(dockerport.Options{Project: id, RunID: id, Builder: r.Decl.Project.Builder, Docker: docker, Allow: r.Decl.Exec.Allow, Mask: m, Log: o.log()})
	if err != nil {
		return collect.RunConfig{}, nil, err
	}
	edge, err := httpport.NewEdge(httpport.EdgeOptions{Ports: runPorts, Mask: m})
	if err != nil {
		return collect.RunConfig{}, nil, err
	}
	shared := map[string]ports.Handler{ports.PortDocker: dport, ports.PortHTTP: edge}
	var closers []func() error
	if len(r.Decl.Databases) > 0 {
		targets, err := DatabaseTargets(*r.Decl, runPorts)
		if err != nil {
			return collect.RunConfig{}, nil, err
		}
		sql, err := sqlport.New(sqlport.Options{Databases: r.Decl.Databases, Targets: targets, Secret: secretLookup(r), Mask: m})
		if err != nil {
			return collect.RunConfig{}, nil, err
		}
		shared[ports.PortSQL], closers = sql, append(closers, sql.Close)
	}
	collectors := make([]collect.Collector, 0, len(c.defs))
	for _, d := range c.defs {
		collectors = append(collectors, collect.Collector{ID: d.ID, Code: d.Code, Interval: d.Interval, Allowed: d.Allowed})
	}
	cfg := collect.RunConfig{
		RunID: id, Collectors: collectors, Signals: stream, Alive: collect.FileAlive{Dir: r.Layout.Root}, Mask: m,
		Ports: func(collector string) map[string]ports.Handler {
			out := map[string]ports.Handler{
				ports.PortSignal: &signalport.Port{Collector: collector, Store: stream, Mask: m},
				ports.PortState:  &stateport.Port{Collector: collector, Store: stream, Mask: m},
			}
			for name, h := range shared {
				out[name] = h
			}
			return out
		},
		Observers: []collect.Observer{
			&observer.Logs{Lister: dport, Docker: docker, Emit: stream, Mask: m, Now: o.Now},
			&observer.Services{Source: dport, Emit: stream},
		},
	}
	return cfg, closers, nil
}
