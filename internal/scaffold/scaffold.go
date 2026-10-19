package scaffold

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	httpport "github.com/d0lim/cockpit/internal/ports/http"
)

//go:embed template/smoke.ts
var smokeSuite string

const DefaultBaseURL = "http://app"

const composeOverlay = `services:
  runner:
    image: ${COCKPIT_RUNNER_IMAGE:?run cockpit runner-image first}
    pull_policy: never
    profiles: ["runner"]
    volumes:
      - ./e2e:/work/scripts:ro
      - ./.cockpit/project.yaml:/work/.cockpit/project.yaml:ro
      - ./.e2e:/work/e2e
`

const tsconfig = `{
  "extends": "@d0lim/cockpit-sdk/tsconfig.base.json",
  "include": ["**/*.ts"]
}
`

type file struct {
	path string
	data string
	mode os.FileMode
}

func files(baseURL string) ([]file, error) {
	env, err := json.MarshalIndent(map[string]string{"baseUrl": baseURL}, "", "  ")
	if err != nil {
		return nil, err
	}
	return []file{
		{".cockpit/project.yaml", fmt.Sprintf("hostApi: %d\n", contract.HostAPI), 0o644},
		{"compose.cockpit.yaml", composeOverlay, 0o644},
		{"e2e/env.json", string(env) + "\n", 0o644},
		{"e2e/suites/smoke.ts", smokeSuite, 0o644},
		{"e2e/tsconfig.json", tsconfig, 0o644},
		{".e2e/.gitignore", "*\n", 0o644},
		{".e2e/e2e.private.env.json", "{\n  \"local\": {}\n}\n", 0o600},
	}, nil
}

func Write(dir, baseURL string) ([]string, error) {
	origin, ok := httpport.Origin(baseURL)
	if !ok {
		return nil, fmt.Errorf("base URL %q is not an origin: use scheme://service[:port], such as http://web:8080, with no path and no IP address", baseURL)
	}
	fs, err := files(origin)
	if err != nil {
		return nil, err
	}
	var existing []string
	for _, f := range fs {
		if _, err := os.Lstat(filepath.Join(dir, f.path)); err == nil {
			existing = append(existing, f.path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if len(existing) > 0 {
		return nil, fmt.Errorf("refusing to overwrite: %s", strings.Join(existing, ", "))
	}
	written := make([]string, 0, len(fs))
	for _, f := range fs {
		path := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return written, err
		}
		if err := os.WriteFile(path, []byte(f.data), f.mode); err != nil {
			return written, err
		}
		if err := os.Chmod(path, f.mode); err != nil {
			return written, err
		}
		written = append(written, f.path)
	}
	if err := os.Chmod(filepath.Join(dir, ".e2e"), 0o777); err != nil {
		return written, err
	}
	return written, nil
}
