package compose

import (
	"path/filepath"
	"strings"
	"testing"
)

func toyKit(t *testing.T, flows []string) (string, KitInput) {
	t.Helper()
	decl := toyDeclaration(t)
	in := KitInput{RunID: runID, PortBase: 22300, Ports: decl.Ports, Notes: decl.Kit.Notes, Flows: flows}
	text, err := KitReadme(in)
	if err != nil {
		t.Fatal(err)
	}
	return text, in
}

func TestKitReadmeCarriesTheDeclarationNotesAndHubCommandsSpelledCockpitRuns(t *testing.T) {
	text, in := toyKit(t, []string{"health", "orders"})
	for _, line := range strings.Split(strings.TrimSpace(in.Notes), "\n") {
		if !strings.Contains(text, line) {
			t.Errorf("notes line %q is missing", line)
		}
	}
	commands := []string{"show", "exec", "stop", "resume", "destroy"}
	for _, c := range commands {
		if !strings.Contains(text, "cockpit runs "+c+" "+runID) {
			t.Errorf("command cockpit runs %s is missing", c)
		}
	}
	if !strings.Contains(text, "cockpit runs exec "+runID+" -- <flow>") {
		t.Fatal("the exec command lacks the flow placeholder")
	}
	if strings.Contains(text, "bun run") || strings.Contains(text, "{{") {
		t.Fatalf("the readme holds old tool names or template residue:\n%s", text)
	}
	noNotes := in
	noNotes.Notes = ""
	bare, err := KitReadme(noNotes)
	if err != nil || strings.Contains(bare, "## Project notes") {
		t.Fatalf("a kit without notes still has the notes section: %v", err)
	}
}

func TestKitReadmeListsEveryPortWithTheAddressOfItsOpenMode(t *testing.T) {
	text, _ := toyKit(t, nil)
	rows := []string{
		"| 22300 | `app` | Toy app API (`app`) | http://" + runID + ".localhost:22300 |",
		"| 22301 | `db` | Toy PostgreSQL (`db`) | `127.0.0.1:22301` |",
		"| 22302 | `cache` | Toy Valkey (`cache`) | `127.0.0.1:22302` |",
	}
	for _, r := range rows {
		if !strings.Contains(text, r) {
			t.Errorf("row %q is missing from\n%s", r, text)
		}
	}
	if !strings.Contains(text, "Port 22399 is reserved") {
		t.Fatal("the canary port is not named")
	}
	if got := openText(runID, 22350, "link"); got != "http://127.0.0.1:22350" {
		t.Fatalf("link address %s", got)
	}
}

func TestKitReadmeSaysTheManifestIsUnreadUntilFlowsAreKnownAndThenListsThem(t *testing.T) {
	unread, _ := toyKit(t, nil)
	if !strings.Contains(unread, "has not been read yet") || strings.Contains(unread, "- `health`") {
		t.Fatalf("unread manifest section:\n%s", unread)
	}
	known, _ := toyKit(t, []string{"health", "orders", "facts"})
	for _, id := range []string{"health", "orders", "facts"} {
		if !strings.Contains(known, "- `"+id+"`") {
			t.Errorf("flow %s is missing", id)
		}
	}
	if strings.Contains(known, "has not been read yet") || !strings.HasSuffix(known, "- `facts`\n") {
		t.Fatalf("known flows section:\n%s", known)
	}
}

func TestWriteKitWritesTheReadmeAt600InA700Directory(t *testing.T) {
	l := testLayout(t)
	root := runRoot(t, l)
	_, in := toyKit(t, nil)
	if err := WriteKit(root, in); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(l.Root, KitReadmeFile)
	if mode(t, readme) != 0o600 || mode(t, filepath.Dir(readme)) != 0o700 {
		t.Fatalf("modes %v %v", mode(t, readme), mode(t, filepath.Dir(readme)))
	}
	if !strings.HasPrefix(readFile(t, readme), "# Run "+runID) {
		t.Fatal("the readme does not start with its title")
	}
}
