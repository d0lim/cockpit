package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/collect/worker"
)

func TestScriptHostModeRunsTheBundleFromStdinAndReportsDone(t *testing.T) {
	dir := t.TempDir()
	src := "import { collector } from \"cockpit:collect\";\nexport default collector({ id: \"toy-db\", run: async () => {} });\n"
	if err := os.WriteFile(filepath.Join(dir, "collector.ts"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	code, err := worker.Bundle(context.Background(), dir, "collector.ts")
	if err != nil {
		t.Fatal(err)
	}
	setup, _ := json.Marshal(worker.Setup{Collector: "toy-db", Code: code, DeadlineMs: 2000})
	scriptHostStdin = strings.NewReader(string(setup) + "\n")
	t.Cleanup(func() { scriptHostStdin = os.Stdin })
	exit, out, errOut := call(nil, worker.SubCommand)
	if exit != 0 || strings.TrimSpace(out) != `{"done":{"ok":true}}` {
		t.Fatalf("exit %d stdout %q stderr %q", exit, out, errOut)
	}
	if exit, _, _ := call(nil, worker.SubCommand, "extra"); exit != 2 {
		t.Fatalf("extra argument: exit %d", exit)
	}
}
