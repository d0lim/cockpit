package envops

import (
	"context"
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

var defaultDBPort = map[contract.DBDriver]int{contract.DriverPostgres: 5432, contract.DriverMySQL: 3306}

func DatabaseTargets(decl contract.Declaration, runPorts map[string]int) map[string]sqlport.Target {
	targets := map[string]sqlport.Target{}
	for _, db := range decl.Databases {
		host := 0
		if p, ok := runPorts[string(db.Name)]; ok {
			host = p
		} else {
			var matches []int
			for _, p := range decl.Ports {
				if p.Port == defaultDBPort[db.Driver] {
					matches = append(matches, runPorts[string(p.Name)])
				}
			}
			if len(matches) == 1 {
				host = matches[0]
			}
		}
		if host > 0 {
			targets[string(db.Name)] = sqlport.Target{Host: "127.0.0.1", Port: host, RootUser: db.RootUser, Database: db.Database}
		}
	}
	return targets
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
	if c.stream == nil {
		if c.stream, err = signal.Open(r.Layout.Root, signal.Options{Masker: func() *mask.Masker { return o.currentMasker(id) }, Now: o.Now}); err != nil {
			return err
		}
	}
	cfg, closers, err := o.runConfig(id, r, c, m)
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

func (o *Ops) runConfig(id string, r *Run, c *collected, m ports.Mask) (collect.RunConfig, []func() error, error) {
	docker := o.DockerFor(procgroup.Hooks{})
	runPorts := o.RunPorts(r)
	dport, err := dockerport.New(dockerport.Options{Project: id, RunID: id, Builder: r.Decl.Project.Builder, Docker: docker, Allow: r.Decl.Exec.Allow, Mask: m})
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
		targets := DatabaseTargets(*r.Decl, runPorts)
		var dbs []contract.Database
		for _, db := range r.Decl.Databases {
			if _, ok := targets[string(db.Name)]; ok {
				dbs = append(dbs, db)
			}
		}
		sql, err := sqlport.New(sqlport.Options{Databases: dbs, Targets: targets, Secret: secretLookup(r), Mask: m})
		if err != nil {
			return collect.RunConfig{}, nil, err
		}
		shared[ports.PortSQL], closers = sql, append(closers, sql.Close)
	}
	collectors := make([]collect.Collector, 0, len(c.defs))
	for _, d := range c.defs {
		collectors = append(collectors, collect.Collector{ID: d.ID, Code: d.Code, Interval: d.Interval, Allowed: d.Allowed})
	}
	stream := c.stream
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
