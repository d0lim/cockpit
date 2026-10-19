package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"go.yaml.in/yaml/v3"

	httpport "github.com/d0lim/cockpit/internal/ports/http"
	"github.com/d0lim/cockpit/internal/ports/source"
)

type settings struct {
	dir, privPath, envPath, execOut, scripts, sources, declaration, image string
}

func loadSettings(getenv func(string) string) settings {
	or := func(key, fallback string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return fallback
	}
	s := settings{}
	s.scripts = or("COCKPIT_SCRIPTS", "/work/scripts")
	s.dir = or("E2E_DIR", "/work/e2e")
	s.privPath = or("E2E_PRIVATE_ENV", filepath.Join(s.dir, "e2e.private.env.json"))
	s.envPath = or("E2E_ENV", filepath.Join(s.scripts, "env.json"))
	s.execOut = getenv("E2E_EXEC_OUT")
	s.sources = or("COCKPIT_SOURCES", "/work/source")
	s.declaration = or("COCKPIT_DECLARATION", "/work/.cockpit/project.yaml")
	s.image = getenv("COCKPIT_RUNNER_IMAGE")
	return s
}

func declaredHostAPI(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("could not read declaration %s: %w", path, err)
	}
	var doc struct {
		HostAPI *int `yaml:"hostApi"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return 0, fmt.Errorf("could not parse declaration %s as YAML: %w", path, err)
	}
	if doc.HostAPI == nil {
		return 0, fmt.Errorf("declaration %s has no hostApi", path)
	}
	return *doc.HostAPI, nil
}

func loadPublicEnv(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read public env %s: %w", path, err)
	}
	var env map[string]any
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("public env %s is not a JSON object: %w", path, err)
	}
	if env == nil {
		env = map[string]any{}
	}
	return env, nil
}

func allowedOrigins(env map[string]any) []string {
	var origins []string
	for _, v := range env {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if origin, ok := httpport.Origin(s); ok {
			origins = append(origins, origin)
		}
	}
	slices.Sort(origins)
	return slices.Compact(origins)
}

func seedKeys(env map[string]any) map[string]string {
	keys := map[string]string{}
	table, _ := env["totpSeedKeys"].(map[string]any)
	for login, v := range table {
		if key, ok := v.(string); ok && key != "" {
			keys[login] = key
		}
	}
	return keys
}

func sourceRoots(dir string) (source.Roots, error) {
	roots := source.Roots{}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return roots, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not read source directory %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			roots[e.Name()] = filepath.Join(dir, e.Name())
		}
	}
	return roots, nil
}
