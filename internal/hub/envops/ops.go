package envops

import (
	"context"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/lease"
	"github.com/d0lim/cockpit/internal/hub/procgroup"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/stages"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/runnerbin"
)

var EnvDirs = []string{"src", "work", "env", "build"}

type ReportInput struct {
	Meta   store.Meta
	Store  *store.Store
	RunDir string
	Decl   *contract.Declaration
	Ports  map[string]int
	Runner *compose.RunnerResult
	Boot   *contract.BootVerdict
	Masker *mask.Masker
	Now    time.Time
}

type Reporter func(ctx context.Context, in ReportInput) (reconcile.ReportOutcome, string, error)

type Ops struct {
	Store     *store.Store
	DockerFor func(h procgroup.Hooks) compose.Docker
	Ensure    func(ctx context.Context) (runnerbin.Image, error)
	Layout    lease.Layout
	Probe     lease.Probe
	Limits    stages.Limits
	Reporter  Reporter
	Now       func() time.Time
	Grace     time.Duration
	maskers   sync.Map
}

type Run struct {
	Meta    store.Meta
	Store   *store.Store
	Layout  compose.Layout
	Root    *os.Root
	Decl    *contract.Declaration
	Project compose.Project
	Docker  compose.Docker
	Masker  *mask.Masker
}

func (r *Run) Close() {
	if r.Root != nil {
		r.Root.Close()
	}
}

func (r *Run) Mask(s string) string { return r.Masker.Mask(s) }

func (o *Ops) PgidHooks(st *store.Store, id string) procgroup.Hooks {
	note := func(pgid *int) {
		st.PatchMeta(id, func(m *store.Meta) { m.Pgid = pgid })
	}
	return procgroup.Hooks{
		Grace:   o.Grace,
		OnSpawn: func(pgid int) { note(&pgid) },
		OnExit:  func() { note(nil) },
	}
}

func (o *Ops) Open(id string) (*Run, error) {
	m, err := o.Store.MustRead(id)
	if err != nil {
		return nil, err
	}
	dir, err := o.Store.RunDir(id)
	if err != nil {
		return nil, err
	}
	layout, err := compose.NewLayout(dir)
	if err != nil {
		return nil, err
	}
	r := &Run{Meta: m, Store: o.Store, Layout: layout}
	if r.Root, err = os.OpenRoot(dir); err != nil {
		r.Root = nil
	}
	if r.Root != nil {
		if raw, err := r.Root.ReadFile(path.Join("src", compose.DeclarationPath)); err == nil {
			if decl, err := compose.ParseDeclaration(raw); err == nil {
				r.Decl = &decl
				r.Store = store.Open(o.Store.Home(), compose.SecretFilePatterns(decl)...)
			}
		}
	}
	r.Masker = o.masker(id, r)
	r.Docker = o.DockerFor(o.PgidHooks(r.Store, id))
	r.Project = compose.Project{Docker: r.Docker, Name: id, Args: []string{"compose", "--ansi", "never", "-p", id}, Mask: r.Mask}
	if r.Decl != nil {
		if p, err := stages.OpenProject(id, layout, *r.Decl, r.Docker, r.Mask, nil); err == nil {
			r.Project = p
		}
	}
	return r, nil
}

func (o *Ops) masker(id string, r *Run) *mask.Masker {
	var patterns []string
	src := mask.Sources{Dir: r.Layout.Root, Mode: mask.Hub}
	if r.Decl != nil {
		patterns = r.Decl.Secrets.Patterns
		src = compose.MaskSources(*r.Decl, r.Layout)
	}
	m, err := stages.RunMasker(patterns, src)
	if err != nil {
		if cached, ok := o.maskers.Load(id); ok {
			return cached.(*mask.Masker)
		}
	}
	if m == nil {
		m, _ = mask.New(mask.Options{Mode: mask.Hub})
	}
	o.maskers.Store(id, m)
	return m
}

func (o *Ops) Mask(id, text string) string {
	if cached, ok := o.maskers.Load(id); ok {
		return cached.(*mask.Masker).Mask(text)
	}
	r, err := o.Open(id)
	if err != nil {
		m, _ := mask.New(mask.Options{Mode: mask.Hub})
		return m.Mask(text)
	}
	defer r.Close()
	return r.Mask(text)
}

func (o *Ops) KillGroup(pgid int) error {
	return procgroup.KillGroup(context.Background(), pgid, o.grace())
}

func (o *Ops) grace() time.Duration {
	if o.Grace <= 0 {
		return procgroup.DefaultGrace
	}
	return o.Grace
}

func (o *Ops) RemoveOneOffs(ctx context.Context, id string) error {
	r, err := o.Open(id)
	if err != nil {
		return err
	}
	defer r.Close()
	_, err = r.Project.RemoveOneOffs(ctx, "")
	return err
}

func removeEnvDirs(dir string) {
	for _, name := range EnvDirs {
		os.RemoveAll(path.Join(dir, name))
	}
}

func (o *Ops) releaseLease(id string) error {
	_, err := o.Store.UpdateState(func(s store.HubState) store.HubState {
		s.Leases = lease.Release(s.Leases, id)
		return s
	})
	return err
}

func appendNotice(st *store.Store, id, text string) {
	st.PatchMeta(id, func(m *store.Meta) {
		parts := []string{}
		if m.Notice != "" {
			parts = append(parts, m.Notice)
		}
		m.Notice = strings.Join(append(parts, text), "; ")
	})
}

func (o *Ops) CloseWithoutEnv(ctx context.Context, id string, down bool) error {
	r, err := o.Open(id)
	if err != nil {
		return err
	}
	defer r.Close()
	if down {
		if err := r.Project.Down(ctx); err != nil {
			appendNotice(r.Store, id, "could not remove the remaining resources: "+r.Mask(err.Error()))
		}
	}
	removeEnvDirs(r.Layout.Root)
	o.maskers.Delete(id)
	return o.releaseLease(id)
}

func (o *Ops) StopQuietly(ctx context.Context, id string) {
	r, err := o.Open(id)
	if err != nil {
		return
	}
	defer r.Close()
	if err := r.Project.Stop(ctx); err != nil {
		appendNotice(r.Store, id, "could not stop: "+r.Mask(err.Error()))
	}
}
