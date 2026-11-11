package stages

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/seal"
)

var sealFilesMutations = []porttest.Mutation{
	{"the files.overlays assertion reads every compose file of the model", "pass compose.Files, the declared files and the harness, as the seal input in Env.SealStatic", "red: files.overlays passes with deps/queue/compose.tunnel.yaml in the model", "TestTheStaticSealChecksEveryComposeFileTheModelReadSoAForbiddenIncludedNameFailsFilesOverlays"},
}

func TestSealFilesMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "sealfiles_test.go", sealFilesMutations)
}

type configOnly struct{}

func (configOnly) Run(ctx context.Context, cmd compose.Command) (compose.Result, error) {
	return compose.Result{Stdout: []byte(`{"services":{}}`)}, nil
}

func overlaysResult(t *testing.T, included string) seal.Result {
	t.Helper()
	l, err := compose.NewLayout(filepath.Join(t.TempDir(), "run"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := compose.OpenRun(l.Root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	decl := contract.Declaration{Compose: contract.ComposeSection{Files: []contract.RelPath{"compose.yaml"}, Forbidden: []string{"tunnel"}}}
	loaded := compose.Loaded{Declaration: decl, Model: compose.Model{Files: []string{"compose.yaml", included}}}
	p, err := OpenProject("r-1", l, decl, configOnly{}, func(s string) string { return s }, nil)
	if err != nil {
		t.Fatal(err)
	}
	e := &Env{RunID: "r-1", Loaded: loaded, Layout: l, Root: root, Project: p, Base: 22000, Now: time.Now}
	if err := e.SealStatic(context.Background()); err == nil {
		t.Fatal("canary: an empty render passed the static seal")
	}
	for _, r := range e.Static {
		if r.Name == "files.overlays" {
			return r
		}
	}
	t.Fatalf("no files.overlays result in %d results", len(e.Static))
	return seal.Result{}
}

func TestTheStaticSealChecksEveryComposeFileTheModelReadSoAForbiddenIncludedNameFailsFilesOverlays(t *testing.T) {
	if r := overlaysResult(t, "deps/queue/compose.yaml"); !r.OK {
		t.Fatalf("canary: a benign included name fails files.overlays: %s", r.Detail)
	}
	r := overlaysResult(t, "deps/queue/compose.tunnel.yaml")
	if r.OK || !strings.Contains(r.Detail, filepath.Join("src", "deps", "queue", "compose.tunnel.yaml")) {
		t.Fatalf("a forbidden included name: %+v", r)
	}
}
