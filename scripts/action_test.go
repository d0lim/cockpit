package scripts

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var actionMutations = []porttest.Mutation{
	{"the action installs only an archive that matches checksums.txt", "drop the sha256sum check from action-install.sh", "red: a changed archive installs and runs", "TestTheActionInstallsAMatchingReleaseAndRefusesAChangedArchive"},
	{"the action never publishes an archive this run did not write", "emit archive whenever the file exists in action-run.sh", "red: a file left by an earlier step is published as this run's archive", "TestTheActionPublishesOnlyAnArchiveThisRunWrote"},
	{"the action records the wire only when its input says true", "add --wire to every cockpit run in action-run.sh", "red: the run with the default input passes --wire", "TestTheActionPassesWireOnlyWhenItsInputIsTrue"},
	{"the action passes a declaration only when its input names one", "add --declaration .cockpit/project.yaml to every cockpit run in action-run.sh", "red: the run with the empty input passes --declaration", "TestTheActionPassesTheDeclarationOnlyWhenItsInputNamesOneAndTheSummaryNamesIt"},
	{"the job summary names the declaration the run read", "drop the Declaration row from action-summary.jq", "red: the summary of the alt run does not name .cockpit/alt.yaml", "TestTheActionPassesTheDeclarationOnlyWhenItsInputNamesOneAndTheSummaryNamesIt"},
}

func TestActionMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "action_test.go", actionMutations)
}

const fakeCurl = `#!/usr/bin/env bash
out=""
url=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    -*) shift ;;
    *) url="$1"; shift ;;
  esac
done
cp "$FAKE_RELEASE/${url##*/}" "$out"
`

const shasumWrapper = "#!/usr/bin/env bash\nexec shasum -a 256 \"$@\"\n"

func releaseArchive(t *testing.T, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	body := []byte("#!/bin/sh\necho cockpit " + version + "\n")
	if err := tw.WriteHeader(&tar.Header{Name: "cockpit", Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

func install(t *testing.T, tamper bool) (string, string, error) {
	t.Helper()
	release, bin, temp := t.TempDir(), t.TempDir(), t.TempDir()
	name := "cockpit_9.9.9_linux_" + runtime.GOARCH + ".tar.gz"
	archive := releaseArchive(t, "9.9.9")
	sum := sha256.Sum256(archive)
	writeFile(t, filepath.Join(release, "checksums.txt"), []byte(hex.EncodeToString(sum[:])+"  "+name+"\n"), 0o644)
	if tamper {
		archive = releaseArchive(t, "6.6.6")
	}
	writeFile(t, filepath.Join(release, name), archive, 0o644)
	writeFile(t, filepath.Join(bin, "curl"), []byte(fakeCurl), 0o755)
	if _, err := exec.LookPath("sha256sum"); err != nil {
		writeFile(t, filepath.Join(bin, "sha256sum"), []byte(shasumWrapper), 0o755)
	}
	cmd := exec.Command("bash", "action-install.sh")
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "FAKE_RELEASE="+release, "RUNNER_TEMP="+temp,
		"GITHUB_PATH="+filepath.Join(temp, "path"), "COCKPIT_VERSION=9.9.9", "COCKPIT_PATH=", "ACTION_REF=")
	out, err := cmd.CombinedOutput()
	return filepath.Join(temp, "cockpit", "bin", "cockpit"), string(out), err
}

func TestTheActionInstallsAMatchingReleaseAndRefusesAChangedArchive(t *testing.T) {
	installed, out, err := install(t, false)
	if err != nil || !strings.Contains(out, "cockpit 9.9.9") {
		t.Fatalf("canary: a matching release did not install: %v\n%s", err, out)
	}
	if _, err := os.Stat(installed); err != nil {
		t.Fatalf("canary: no installed binary: %v", err)
	}
	installed, out, err = install(t, true)
	if err == nil || strings.Contains(out, "cockpit 6.6.6") {
		t.Fatalf("a changed archive installed or ran: %v\n%s", err, out)
	}
	if _, err := os.Stat(installed); !os.IsNotExist(err) {
		t.Fatalf("a changed archive left a binary: %v", err)
	}
}

func runAction(t *testing.T, cockpitScript string, leftover bool, env ...string) (string, string) {
	t.Helper()
	bin, work, project := t.TempDir(), t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(bin, "cockpit"), []byte(cockpitScript), 0o755)
	archive := filepath.Join(work, "cockpit-run.tar.gz")
	if leftover {
		writeFile(t, archive, []byte("an earlier step's archive"), 0o644)
	}
	output, summary := filepath.Join(work, "output"), filepath.Join(work, "summary.md")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "action-run.sh")
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "RUNNER_TEMP="+work, "GITHUB_WORKSPACE="+work,
		"GITHUB_OUTPUT="+output, "GITHUB_STEP_SUMMARY="+summary, "GITHUB_ACTION_PATH="+filepath.Dir(wd),
		"GITHUB_RUN_ID=1", "GITHUB_RUN_ATTEMPT=1", "COCKPIT_PROJECT="+project, "COCKPIT_ARCHIVE=cockpit-run.tar.gz", "COCKPIT_SUITE=")
	cmd.Env = append(cmd.Env, env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("action-run.sh: %v\n%s", err, out)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	written, _ := os.ReadFile(summary)
	return string(data), string(written)
}

func TestTheActionPublishesOnlyAnArchiveThisRunWrote(t *testing.T) {
	writes := "#!/usr/bin/env bash\nwhile [[ $# -gt 0 ]]; do if [[ $1 = --archive ]]; then echo new > \"$2\"; fi; shift; done\n"
	if got, _ := runAction(t, writes, false); !strings.Contains(got, "archive=") || !strings.Contains(got, "exit-code=0") {
		t.Fatalf("canary: a fresh archive is not published:\n%s", got)
	}
	refuses := "#!/usr/bin/env bash\nexit 3\n"
	got, _ := runAction(t, refuses, true)
	if strings.Contains(got, "archive=") || !strings.Contains(got, "exit-code=3") {
		t.Fatalf("a leftover archive is published as this run's:\n%s", got)
	}
}

func TestTheActionPassesWireOnlyWhenItsInputIsTrue(t *testing.T) {
	echoes := "#!/usr/bin/env bash\necho \"args=$*\" >> \"$GITHUB_OUTPUT\"\n"
	if got, _ := runAction(t, echoes, false, "COCKPIT_WIRE=true"); !strings.Contains(got, " --wire") {
		t.Fatalf("canary: the input true does not pass --wire:\n%s", got)
	}
	for _, value := range []string{"false", ""} {
		got, _ := runAction(t, echoes, false, "COCKPIT_WIRE="+value)
		if !strings.Contains(got, "args=run --project") || strings.Contains(got, "--wire") {
			t.Fatalf("the input %q passes --wire or runs nothing:\n%s", value, got)
		}
	}
}

func TestTheActionPassesTheDeclarationOnlyWhenItsInputNamesOneAndTheSummaryNamesIt(t *testing.T) {
	reports := "#!/usr/bin/env bash\necho \"args=$*\" >> \"$GITHUB_OUTPUT\"\n" +
		"while [[ $# -gt 0 ]]; do if [[ $1 = --out ]]; then mkdir -p \"$2\"; echo '{\"verdict\":\"passed\",\"suite\":\"alt\",\"commit\":\"0123456789abcdef\"}' > \"$2/summary.json\"; fi; shift; done\n"
	got, summary := runAction(t, reports, false, "COCKPIT_DECLARATION_FILE=.cockpit/alt.yaml")
	if !strings.Contains(got, " --declaration .cockpit/alt.yaml") || !strings.Contains(summary, "| Declaration | `.cockpit/alt.yaml` |") {
		t.Fatalf("canary: the input .cockpit/alt.yaml is not passed or not in the summary:\n%s\n%s", got, summary)
	}
	got, summary = runAction(t, reports, false, "COCKPIT_DECLARATION_FILE=")
	if !strings.Contains(got, "args=run --project") || strings.Contains(got, "--declaration") || !strings.Contains(summary, "| Declaration | `.cockpit/project.yaml` |") {
		t.Fatalf("the empty input passes --declaration or the summary does not name the default:\n%s\n%s", got, summary)
	}
}
