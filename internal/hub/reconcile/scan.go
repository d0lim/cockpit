package reconcile

import (
	"context"
	"regexp"
	"sync"
)

const (
	LabelManaged        = "io.cockpit.managed"
	LabelRunID          = "io.cockpit.run-id"
	LabelComposeProject = "com.docker.compose.project"
)

var runningStatus = regexp.MustCompile(`running\(\d+\)`)

func Running(status string) bool { return runningStatus.MatchString(status) }

type Project struct {
	Name   string
	Status string
}

type Labelled struct {
	Containers int
	Networks   int
	Volumes    int
}

type Docker interface {
	ComposeLs(ctx context.Context) ([]Project, error)
	Info(ctx context.Context) bool
	Labelled(ctx context.Context, project string) (Labelled, error)
}

type Orphan struct {
	Project string `json:"project"`
	Running bool   `json:"running"`
}

type Scan struct {
	Orphans   []Orphan
	Foreign   []string
	ForeignUp int
}

func (s Scan) OrphansUp() int {
	n := 0
	for _, o := range s.Orphans {
		if o.Running {
			n++
		}
	}
	return n
}

type Managed struct {
	mu   sync.Mutex
	seen map[string]bool
}

func NewManaged() *Managed { return &Managed{seen: map[string]bool{}} }

func (m *Managed) is(ctx context.Context, d Docker, name string) (bool, error) {
	m.mu.Lock()
	seen, ok := m.seen[name]
	m.mu.Unlock()
	if ok {
		return seen, nil
	}
	found, err := d.Labelled(ctx, name)
	if err != nil {
		return false, err
	}
	labelled := found.Containers+found.Networks > 0
	m.mu.Lock()
	m.seen[name] = labelled
	m.mu.Unlock()
	return labelled, nil
}

func ScanProjects(ctx context.Context, d Docker, known map[string]bool, managed *Managed, projects []Project) (Scan, error) {
	if projects == nil {
		listed, err := d.ComposeLs(ctx)
		if err != nil {
			return Scan{}, err
		}
		projects = listed
	}
	scan := Scan{Orphans: []Orphan{}, Foreign: []string{}}
	for _, p := range projects {
		if known[p.Name] {
			continue
		}
		labelled, err := managed.is(ctx, d, p.Name)
		if err != nil {
			return Scan{}, err
		}
		switch {
		case labelled:
			scan.Orphans = append(scan.Orphans, Orphan{Project: p.Name, Running: Running(p.Status)})
		case Running(p.Status):
			scan.Foreign = append(scan.Foreign, p.Name)
			scan.ForeignUp++
		}
	}
	return scan, nil
}
