package fakedocker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/procgroup"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
)

const (
	RunnerTag   = "cockpit-runner:0123456789ab"
	PinnedID    = "sha256:" + "a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1"
	OtherID     = "sha256:" + "b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2b2"
	ContainerID = "0123456789abcdef0123"
	FakePgid    = 424242
)

type Call struct {
	Args []string
	Env  []string
}

type Fake struct {
	mu              sync.Mutex
	calls           []Call
	ConfigFixture   string
	CanaryPort      int
	Version         string
	FailVerb        map[string]bool
	ProbeLeak       bool
	AppExited       bool
	RunnerCode      int
	RunnerOOM       bool
	RunnerHangs     bool
	RunnerLogs      string
	ManifestImage   string
	TagID           string
	PinnedGone      bool
	EnvFiles        map[string]string
	OnSpawn         func(pgid int)
	Steps           []contract.ResultStep
	Flows           []contract.ManifestFlow
	StoppedAt       *contract.StopPoint
	OnRunner        func(runDir string)
	ContainerSecret string
	RunnerDelay     time.Duration
}

func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *Fake) For(h procgroup.Hooks) compose.Docker { return &bound{f: f, h: h} }

type bound struct {
	f *Fake
	h procgroup.Hooks
}

func (b *bound) Run(ctx context.Context, cmd compose.Command) (compose.Result, error) {
	if b.h.OnSpawn != nil {
		b.h.OnSpawn(FakePgid)
	}
	if b.f.OnSpawn != nil {
		b.f.OnSpawn(FakePgid)
	}
	res, err := b.f.Run(ctx, cmd)
	if b.h.OnExit != nil {
		b.h.OnExit()
	}
	return res, err
}

func flag(args []string, name string) string {
	if i := slices.Index(args, name); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}
	return ""
}

func Verb(args []string) string {
	if args[0] != "compose" {
		if args[0] == "image" || args[0] == "network" {
			return args[0] + " " + args[1]
		}
		return args[0]
	}
	for _, v := range []string{"config", "build", "up", "ps", "stop", "down", "ls", "version"} {
		if slices.Contains(args, v) {
			return "compose " + v
		}
	}
	if i := slices.Index(args, "run"); i >= 0 {
		rest := args[i+1:]
		for _, s := range rest {
			if !strings.HasPrefix(s, "-") {
				return "compose run " + s
			}
		}
	}
	return "compose"
}

func (f *Fake) Run(ctx context.Context, cmd compose.Command) (compose.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, Call{Args: slices.Clone(cmd.Args), Env: slices.Clone(cmd.Env)})
	f.mu.Unlock()
	verb := Verb(cmd.Args)
	if f.FailVerb[verb] {
		return compose.Result{Code: 1, Stderr: []byte(verb + " failed on purpose")}, nil
	}
	switch verb {
	case "version":
		return out("29.0.0"), nil
	case "compose version":
		return out("2.29.0"), nil
	case "run":
		return f.envgen(cmd.Args)
	case "compose config":
		return f.config(cmd.Args)
	case "compose run probe":
		rc := 1
		if f.ProbeLeak {
			rc = 0
		}
		return out(probeLines(rc)), nil
	case "compose run probe-control":
		if conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(f.CanaryPort))); err == nil {
			conn.Close()
			time.Sleep(20 * time.Millisecond)
		}
		return out(probeLines(0)), nil
	case "compose ps":
		return out(f.ps()), nil
	case "compose run runner":
		f.writeRunnerFiles(cmd.Args, cmd.Env)
		return out("Creating\n" + ContainerID), nil
	case "wait":
		if f.RunnerHangs {
			<-ctx.Done()
			return compose.Result{Code: -1}, ctx.Err()
		}
		select {
		case <-ctx.Done():
			return compose.Result{Code: -1}, ctx.Err()
		case <-time.After(f.RunnerDelay):
		}
		return out(strconv.Itoa(f.RunnerCode)), nil
	case "logs":
		if slices.Contains(cmd.Args, "--follow") {
			return f.containerLogs(ctx, cmd)
		}
		return compose.Result{Stdout: []byte(f.RunnerLogs)}, nil
	case "ps":
		if slices.Contains(cmd.Args, "--format") {
			return out(containerTable(cmd.Args)), nil
		}
	case "inspect":
		return out(strconv.FormatBool(f.RunnerOOM)), nil
	case "image inspect":
		return f.inspect(cmd.Args[len(cmd.Args)-1])
	}
	return compose.Result{}, nil
}

func out(s string) compose.Result { return compose.Result{Stdout: []byte(s + "\n")} }

func probeLines(rc int) string {
	var lines []string
	for i := 1; i <= 4; i++ {
		lines = append(lines, fmt.Sprintf("probe=%d rc=%d", i, rc))
	}
	return strings.Join(lines, "\n")
}

func (f *Fake) inspect(ref string) (compose.Result, error) {
	tagID := f.TagID
	if tagID == "" {
		tagID = PinnedID
	}
	switch {
	case ref == RunnerTag:
		return out(tagID), nil
	case ref == PinnedID && !f.PinnedGone:
		return out(PinnedID), nil
	case ref == tagID:
		return out(tagID), nil
	}
	return compose.Result{Code: 1}, nil
}

func (f *Fake) ps() string {
	appState := "running"
	if f.AppExited {
		appState = "exited"
	}
	rows := []map[string]any{
		{"Service": "app", "ID": "aaa", "State": appState, "Health": "", "ExitCode": 0},
		{"Service": "db", "ID": "bbb", "State": "running", "Health": "healthy"},
		{"Service": "cache", "ID": "ccc", "State": "running", "Health": "healthy"},
		{"Service": "edge", "ID": "ddd", "State": "running", "Health": ""},
	}
	data, _ := json.Marshal(rows)
	return string(data)
}

func (f *Fake) envgen(args []string) (compose.Result, error) {
	mount := flag(args, "--mount")
	src := strings.TrimPrefix(strings.Split(mount, ",")[1], "source=")
	for name, data := range f.EnvFiles {
		path := filepath.Join(src, name)
		os.MkdirAll(filepath.Dir(path), 0o700)
		os.WriteFile(path, []byte(data), 0o644)
	}
	return compose.Result{}, nil
}

func (f *Fake) config(args []string) (compose.Result, error) {
	data, err := os.ReadFile(f.ConfigFixture)
	if err != nil {
		return compose.Result{}, err
	}
	src := flag(args, "--project-directory")
	project := flag(args, "-p")
	text := strings.ReplaceAll(string(data), "/fixture/runs/r-toy", filepath.Dir(src))
	text = strings.ReplaceAll(text, `"r-toy`, `"`+project)
	return compose.Result{Stdout: []byte(text)}, nil
}

type Reconcile struct{}

func (Reconcile) ComposeLs(ctx context.Context) ([]reconcile.Project, error) { return nil, nil }

func (Reconcile) Info(ctx context.Context) bool { return true }

func (Reconcile) Labelled(ctx context.Context, project string) (reconcile.Labelled, error) {
	return reconcile.Labelled{Containers: 3}, nil
}
