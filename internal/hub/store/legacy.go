package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const legacySchemaVersion = 1

var ErrLegacyLayout = errors.New("COCKPIT_HOME holds the layout of the previous cockpit")

func RefuseLegacyLayout(home string) error {
	var found []string
	if data, err := os.ReadFile(filepath.Join(home, stateFile)); err == nil {
		var head struct {
			SchemaVersion int `json:"schemaVersion"`
		}
		if json.Unmarshal(data, &head) == nil && head.SchemaVersion == legacySchemaVersion {
			found = append(found, fmt.Sprintf("%s has schemaVersion %d", stateFile, legacySchemaVersion))
		}
	}
	entries, err := os.ReadDir(filepath.Join(home, runsDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	legacyRuns := 0
	for _, e := range entries {
		if e.IsDir() && hasHarnessField(filepath.Join(home, runsDir, e.Name(), metaFile)) {
			legacyRuns++
		}
	}
	if legacyRuns > 0 {
		found = append(found, fmt.Sprintf("%d run meta files carry the harness field", legacyRuns))
	}
	if len(found) == 0 {
		return nil
	}
	return fmt.Errorf("%w (%s): %s; move that directory away or point COCKPIT_HOME at an empty directory, then start the hub again", ErrLegacyLayout, home, strings.Join(found, ", "))
}

func hasHarnessField(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return false
	}
	_, ok := fields["harness"]
	return ok
}
