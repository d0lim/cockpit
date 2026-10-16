package capture

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

const session = "PLANTED-session-7f3a9c"

var captureMutations = []porttest.Mutation{
	{"captures are secret by default", "Set adds the name to persistNames whatever persist says", "red: a capture without persist is written to context.json", "TestCaptureWithoutPersistStaysOutOfContextJSONWhilePersistTrueIsWritten"},
	{"rewriting a capture as secret drops it from the persisted list", "Set does not clear persistNames when persist=false", "red: draftId stays in persisted", "TestCarriedOverCapturesPersistAgainAndRewritingAsSecretDropsThem"},
	{"a missing capture is rejected with its name", "Get returns an empty string", "red: no error, or the name is missing", "TestReadingMissingCaptureFailsWithPortBadArgsAndItsName"},
	{"atomic write", "Save writes the file directly without a temp file", "red: a half-written file other than .tmp shows up in the directory during the write", "TestSaveWritesSchemaVersion2ContextJSONAtomicallyAndLoadReadsItBack"},
	{"schemaVersion check", "Load ignores the version", "red: a version 1 file is carried over", "TestContextJSONWithUnknownSchemaVersionIsNotCarriedOver"},
}

func TestCaptureWithoutPersistStaysOutOfContextJSONWhilePersistTrueIsWritten(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	c.Captures.Set("adminSession", session, false)
	c.Captures.Set("workspaceId", "ws-0001", true)
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "context.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, session) || strings.Contains(text, "adminSession") || !strings.Contains(text, `"workspaceId": "ws-0001"`) {
		t.Fatalf("context.json: %s", text)
	}
	if got, err := c.Captures.Get("adminSession"); err != nil || got != session {
		t.Fatalf("in-memory capture: %q %v", got, err)
	}
	if !slices.Equal(c.Captures.Values(), []string{session, "ws-0001"}) && !slices.Equal(c.Captures.Values(), []string{"ws-0001", session}) {
		t.Fatalf("masker values: %q", c.Captures.Values())
	}
}

func TestCarriedOverCapturesPersistAgainAndRewritingAsSecretDropsThem(t *testing.T) {
	s := NewStore(map[string]string{"workspaceId": "ws-0001", "draftId": "d-1"})
	if got, err := s.Get("workspaceId"); err != nil || got != "ws-0001" {
		t.Fatalf("%q %v", got, err)
	}
	s.Set("draftId", "d-2", false)
	persisted := s.Persisted()
	if len(persisted) != 1 || persisted["workspaceId"] != "ws-0001" {
		t.Fatalf("persisted: %v", persisted)
	}
}

func TestReadingMissingCaptureFailsWithPortBadArgsAndItsName(t *testing.T) {
	_, err := NewStore(nil).Get("workspaceId")
	var pe *engine.PortError
	if !errors.As(err, &pe) || pe.Code != engine.PortBadArgs || !strings.Contains(pe.Message, "workspaceId") {
		t.Fatalf("%v", err)
	}
}

func TestSaveWritesSchemaVersion2ContextJSONAtomicallyAndLoadReadsItBack(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	c.Captures.Set("workspaceId", "ws-0001", true)
	if err := c.CommitStep("admin-1", 1234); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "context.json" {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "context.json"))
	parsed, err := contract.ReadRunnerFile[contract.Context](strings.NewReader(string(raw)))
	if err != nil || parsed.SchemaVersion != contract.RunnerSchemaVersion || parsed.TOTPSteps["admin-1"] != 1234 || parsed.Captures["workspaceId"] != "ws-0001" {
		t.Fatalf("%+v %v", parsed, err)
	}
	again, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if last, ok := again.LastStep("admin-1"); !ok || last != 1234 {
		t.Fatalf("carried-over step: %d %v", last, ok)
	}
	if got, err := again.Captures.Get("workspaceId"); err != nil || got != "ws-0001" {
		t.Fatalf("carried-over capture: %q %v", got, err)
	}
	if _, ok := again.LastStep("admin-2"); ok {
		t.Fatal("step found for an unknown account")
	}
}

func TestContextJSONWithUnknownSchemaVersionIsNotCarriedOver(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "context.json"), []byte(`{"schemaVersion":1,"captures":{},"totpSteps":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !errors.Is(err, contract.ErrSchemaVersion) {
		t.Fatalf("%v", err)
	}
}

func TestCaptureMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "capture_test.go", captureMutations)
}
