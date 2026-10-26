package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type releaseRepo struct {
	t   *testing.T
	dir string
}

func newReleaseRepo(t *testing.T) *releaseRepo {
	t.Helper()
	r := &releaseRepo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	r.git("config", "user.email", "release@example.test")
	r.git("config", "user.name", "release")
	r.git("config", "commit.gpgsign", "false")
	r.git("config", "tag.gpgsign", "false")
	return r
}

func (r *releaseRepo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (r *releaseRepo) commit(hostAPI, declarations, tag string) {
	r.t.Helper()
	files := map[string]string{
		"internal/contract/hostapi.go": "package contract\n\nconst HostAPI = " + hostAPI + "\n",
		"sdk/cockpit-flow.d.ts":        declarations,
		"README.md":                    "tag " + tag + "\n",
	}
	for path, data := range files {
		full := filepath.Join(r.dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "release "+tag)
	r.git("tag", tag)
}

func (r *releaseRepo) notes(body string) (string, string, string, error) {
	r.t.Helper()
	bin := r.t.TempDir()
	calls := filepath.Join(bin, "calls")
	edited := filepath.Join(bin, "edited")
	current := filepath.Join(bin, "current")
	if err := os.WriteFile(current, []byte(body+"\n"), 0o644); err != nil {
		r.t.Fatal(err)
	}
	fake := "#!/usr/bin/env bash\necho \"$*\" >> " + calls + "\n" +
		"if [ \"$2\" = view ]; then cat " + current + "; fi\n" +
		"if [ \"$2\" = edit ]; then cp \"$5\" " + edited + "; fi\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(fake), 0o755); err != nil {
		r.t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.Join(repoRoot(r.t), "scripts", "release-notes.sh"), "v0.2.0")
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	callLog, _ := os.ReadFile(calls)
	editedNotes, _ := os.ReadFile(edited)
	return string(out), string(callLog), string(editedNotes), err
}

func TestReleaseNotesGainTheDeclarationDiffWhenDeclarationsOnlyGrew(t *testing.T) {
	r := newReleaseRepo(t)
	r.commit("1", "get(name: string): string;\n", "v0.1.0")
	r.commit("1", "get(name: string): string;\nref(name: string, value: string): void;\n", "v0.2.0")
	out, calls, notes, err := r.notes("## Changelog\n* abc feat: x")
	if err != nil || !strings.Contains(calls, "release edit v0.2.0 --notes-file") {
		t.Fatalf("err %v\nout %s\ncalls %s", err, out, calls)
	}
	for _, want := range []string{"## Changelog\n* abc feat: x\n", "\n## Declarations changed\n", "Host API 1, unchanged since v0.1.0", "+ref(name: string, value: string): void;", "cockpit sdk write <dir>"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("notes lack %q:\n%s", want, notes)
		}
	}
}

func TestReleaseNotesNameTheRaisedHostApiWhenItChanged(t *testing.T) {
	r := newReleaseRepo(t)
	r.commit("1", "get(name: string): string;\n", "v0.1.0")
	r.commit("2", "get(name: string): number;\n", "v0.2.0")
	out, _, notes, err := r.notes("## Changelog")
	if err != nil || !strings.Contains(notes, "Host API raised from 1 to 2 since v0.1.0") || !strings.Contains(notes, "-get(name: string): string;") || !strings.Contains(notes, "`hostApi: 2`") {
		t.Fatalf("err %v\nout %s\nnotes %s", err, out, notes)
	}
}

func TestReleaseNotesStayAsTheyAreWhenTheDeclarationsDidNotChange(t *testing.T) {
	r := newReleaseRepo(t)
	r.commit("1", "get(name: string): string;\n", "v0.1.0")
	r.commit("1", "get(name: string): string;\n", "v0.2.0")
	out, calls, _, err := r.notes("## Changelog")
	if err != nil || !strings.Contains(out, "declarations unchanged since v0.1.0") || calls != "" {
		t.Fatalf("err %v\nout %s\ncalls %s", err, out, calls)
	}
}

func TestReleaseNotesOfTheFirstTagGetNoDeclarationsSection(t *testing.T) {
	r := newReleaseRepo(t)
	r.commit("1", "get(name: string): string;\n", "v0.2.0")
	out, calls, _, err := r.notes("## Changelog")
	if err != nil || !strings.Contains(out, "no tag before v0.2.0") || calls != "" {
		t.Fatalf("err %v\nout %s\ncalls %s", err, out, calls)
	}
}

func TestReleaseNotesAreNotEditedTwice(t *testing.T) {
	r := newReleaseRepo(t)
	r.commit("1", "get(name: string): string;\n", "v0.1.0")
	r.commit("1", "get(name: string): string;\nref(name: string, value: string): void;\n", "v0.2.0")
	out, calls, _, err := r.notes("## Changelog\n\n## Declarations changed\n\nHost API 1")
	if err != nil || !strings.Contains(out, "already have a declarations section") || strings.Contains(calls, "edit") || !strings.Contains(calls, "release view v0.2.0") {
		t.Fatalf("err %v\nout %s\ncalls %s", err, out, calls)
	}
}
