package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
)

const (
	fileManifest = "manifest.json"
	fileResult   = "result.json"
	fileLedger   = "ledger.json"
	execDir      = "exec"
	isoLayout    = "2006-01-02T15:04:05.000Z"
)

func iso(t time.Time) string {
	return t.UTC().Format(isoLayout)
}

func WriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".write-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func ReadLedger(dir string) (contract.Ledger, error) {
	empty := contract.Ledger{SchemaVersion: contract.RunnerSchemaVersion, Flows: map[string]contract.LedgerEntry{}}
	raw, err := os.ReadFile(filepath.Join(dir, fileLedger))
	if errors.Is(err, fs.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return contract.Ledger{}, err
	}
	parsed, err := contract.ReadRunnerFile[contract.Ledger](bytes.NewReader(raw))
	if err != nil {
		return contract.Ledger{}, fmt.Errorf("%s: %w", fileLedger, err)
	}
	if parsed.Flows == nil {
		parsed.Flows = map[string]contract.LedgerEntry{}
	}
	return *parsed, nil
}

func ExecOutPath(dir, base string, targets []string, now time.Time) func(target string) string {
	if base == "" {
		stamp := strings.NewReplacer(":", "-", ".", "-").Replace(iso(now))
		return func(target string) string { return filepath.Join(dir, execDir, stamp+"-"+target+".json") }
	}
	if len(targets) == 1 {
		return func(string) string { return base }
	}
	stem := strings.TrimSuffix(base, ".json")
	return func(target string) string { return stem + "-" + target + ".json" }
}
