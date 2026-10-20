package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/hub/lock"
	"github.com/d0lim/cockpit/internal/hub/store"
)

const (
	EnvRegistry = "COCKPIT_REGISTRY"
	xdgState    = "XDG_STATE_HOME"
	dirName     = "cockpit-hubs"
	idLength    = 12
	entrySuffix = ".json"
)

type Hub struct {
	ID   string
	Name string
	Home string
	Self bool
	Up   bool
	Port int
}

func (h Hub) URL() string {
	if !h.Up {
		return ""
	}
	return "http://" + lock.ListenHost + ":" + strconv.Itoa(h.Port)
}

type entry struct {
	Home string `json:"home"`
	Name string `json:"name"`
}

type Registry struct {
	dir  string
	self string
}

func Dir(getenv func(string) string) (string, error) {
	if v := getenv(EnvRegistry); v != "" {
		return v, nil
	}
	if v := getenv(xdgState); v != "" {
		return filepath.Join(v, dirName), nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userHome, ".local", "state", dirName), nil
}

func absolute(home string) string {
	abs, err := filepath.Abs(home)
	if err != nil {
		return filepath.Clean(home)
	}
	return abs
}

func ID(home string) string {
	sum := sha256.Sum256([]byte(absolute(home)))
	return hex.EncodeToString(sum[:])[:idLength]
}

func Open(dir, home string) *Registry {
	return &Registry{dir: dir, self: absolute(home)}
}

func (r *Registry) Register() error {
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(entry{Home: r.self, Name: filepath.Base(r.self)})
	if err != nil {
		return err
	}
	return store.WriteAtomic(filepath.Join(r.dir, ID(r.self)+entrySuffix), append(data, '\n'))
}

func (r *Registry) Hubs() []Hub {
	names, err := os.ReadDir(r.dir)
	if err != nil {
		return nil
	}
	var out []Hub
	for _, n := range names {
		id, ok := strings.CutSuffix(n.Name(), entrySuffix)
		if !ok || n.IsDir() {
			continue
		}
		if h, ok := r.read(id, filepath.Join(r.dir, n.Name())); ok {
			out = append(out, h)
		}
	}
	slices.SortFunc(out, func(a, b Hub) int {
		if a.Self != b.Self {
			if a.Self {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name+a.ID, b.Name+b.ID)
	})
	return out
}

func (r *Registry) read(id, path string) (Hub, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Hub{}, false
	}
	var e entry
	if json.Unmarshal(data, &e) != nil || e.Home == "" || !filepath.IsAbs(e.Home) || ID(e.Home) != id {
		return Hub{}, false
	}
	if info, err := os.Stat(e.Home); err != nil || !info.IsDir() {
		return Hub{}, false
	}
	h := Hub{ID: id, Name: e.Name, Home: e.Home, Self: e.Home == r.self}
	if l, ok := lock.Holder(e.Home); ok {
		h.Up, h.Port = true, l.Port
	}
	return h, true
}

func Owner(hubs []Hub, runID string) string {
	if !store.IsRunID(runID) {
		return ""
	}
	for _, h := range hubs {
		if h.Self {
			continue
		}
		if m, err := store.Open(h.Home).ReadMeta(runID); err == nil && m != nil {
			return h.ID
		}
	}
	return ""
}
