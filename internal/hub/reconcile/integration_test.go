package reconcile

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func runnerImage(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("docker", "image", "ls", "--filter", "reference=cockpit-runner", "--format", "{{.Repository}}:{{.Tag}}").Output()
	if err != nil {
		t.Skipf("docker image ls: %v", err)
	}
	images := strings.Fields(string(out))
	if len(images) == 0 {
		t.Skip("no cockpit-runner image on this host; run cockpit runner-image first")
	}
	return images[0]
}

func composeUp(t *testing.T, project, image string, labels map[string]string) {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "services:\n  edge:\n    image: %s\n    pull_policy: never\n    command: [\"edge\", \"19999=127.0.0.1:1\"]\n", image)
	if len(labels) > 0 {
		b.WriteString("    labels:\n")
		for k, v := range labels {
			fmt.Fprintf(&b, "      %s: %q\n", k, v)
		}
	}
	file := filepath.Join(t.TempDir(), "compose.yaml")
	if err := os.WriteFile(file, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	down := func() { exec.Command("docker", "compose", "-p", project, "-f", file, "down", "--remove-orphans").Run() }
	t.Cleanup(down)
	if out, err := exec.Command("docker", "compose", "-p", project, "-f", file, "up", "-d").CombinedOutput(); err != nil {
		t.Fatalf("compose up %s: %v: %s", project, err, out)
	}
}

func TestRealDaemonCountsAnUnrelatedRunningProjectAndMarksALabelledOneOrphan(t *testing.T) {
	if os.Getenv("COCKPIT_INTEGRATION") != "1" {
		t.Skip("set COCKPIT_INTEGRATION=1 to run against the local Docker daemon")
	}
	image := runnerImage(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cli := NewCLI()
	if !cli.Info(ctx) {
		t.Fatal("docker info failed")
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000)
	foreign, orphan := "cockpit-it-foreign-"+suffix, "cockpit-it-orphan-"+suffix
	baseline, err := ScanProjects(ctx, cli, map[string]bool{}, NewManaged(), nil)
	if err != nil {
		t.Fatal(err)
	}
	composeUp(t, foreign, image, nil)
	composeUp(t, orphan, image, map[string]string{LabelRunID: orphan, LabelManaged: "true"})
	projects, err := cli.ComposeLs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{foreign, orphan} {
		i := slices.IndexFunc(projects, func(p Project) bool { return p.Name == name })
		if i < 0 || !Running(projects[i].Status) {
			t.Fatalf("%s missing or not running in %+v", name, projects)
		}
	}
	if l, err := cli.Labelled(ctx, orphan); err != nil || l.Containers != 1 {
		t.Fatalf("orphan labelled %+v %v", l, err)
	}
	if l, err := cli.Labelled(ctx, foreign); err != nil || l.Containers != 0 {
		t.Fatalf("foreign labelled %+v %v", l, err)
	}
	scan, err := ScanProjects(ctx, cli, map[string]bool{}, NewManaged(), projects)
	if err != nil {
		t.Fatal(err)
	}
	if scan.ForeignUp != baseline.ForeignUp+1 {
		t.Fatalf("foreign up %d, baseline %d", scan.ForeignUp, baseline.ForeignUp)
	}
	if !slices.Contains(scan.Orphans, Orphan{Project: orphan, Running: true}) || slices.ContainsFunc(scan.Orphans, func(o Orphan) bool { return o.Project == foreign }) {
		t.Fatalf("orphans %+v", scan.Orphans)
	}
}
