package seal

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var staticMutations = []porttest.Mutation{
	{"the harness roles are present before any other assertion", "drop roles() from Static", "red: the count is 23 and a render without the edge role passes", "TestToyRenderPassesAll24AssertionsInTheDocumentedOrder"},
	{"each assertion catches its own violation", "make any one assertion function return no violation", "red: that assertion's violating copy stays green", "TestEachViolatingCopyTurnsOnlyItsOwnAssertionRed"},
	{"bind sources are judged by real path", "use the bind source string instead of Realpath in mountChecks", "red: a runner bind through a link out of work passes", "TestRunnerBindThroughALinkOutOfTheWorkTreeIsRejectedAndAPlainWorkBindPasses"},
	{"unresolvable paths are violations", "return the path unchanged on errors other than not-exist, or drop the dangling-link check", "red: a path under a regular file or through a dangling link resolves", "TestRealPathResolvesThroughTheNearestExistingAncestorAndRefusesDanglingLinksAndRelativePaths"},
}

func TestStaticMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "static_test.go", staticMutations)
}

var assertionNames = []string{
	"roles.present",
	"files.overlays",
	"networks.set",
	"networks.internal",
	"services.networks",
	"services.ports",
	"services.network-mode",
	"services.extra-hosts",
	"edge.upstreams",
	"edge.proxy-pass",
	"edge.listen",
	"edge.host-ip",
	"edge.published",
	"mounts.binds",
	"mounts.sockets",
	"volumes.named",
	"privileges.privileged",
	"privileges.devices",
	"privileges.namespaces",
	"privileges.security-opt",
	"privileges.cap-add",
	"paths.configs-secrets",
	"paths.env-file",
	"paths.build",
}

func TestToyRenderPassesAll24AssertionsInTheDocumentedOrder(t *testing.T) {
	results := Static(toyInput(t))
	var names []string
	passed := 0
	for _, r := range results {
		names = append(names, r.Name)
		if r.OK {
			passed++
		} else {
			t.Errorf("%s: %s", r.Name, r.Detail)
		}
	}
	if !slices.Equal(names, assertionNames) {
		t.Fatalf("assertions\n%q\nwant\n%q", names, assertionNames)
	}
	if passed != 24 || Verdict(results) != "passed" {
		t.Fatalf("%d of %d passed", passed, len(results))
	}
}

func violations(t *testing.T) map[string]func(in *Input) {
	app := func(in *Input) map[string]any { return svc(t, *in, "app") }
	return map[string]func(in *Input){
		"roles.present":  func(in *Input) { delete(record(svc(t, *in, "edge")["labels"]), "io.cockpit.role") },
		"files.overlays": func(in *Input) { in.Files = append(in.Files, fixtureRun+"/src/compose.tunnel.yaml") },
		"networks.set": func(in *Input) {
			record(in.Config["networks"])["shared"] = map[string]any{"external": true, "internal": true}
		},
		"networks.internal": func(in *Input) { record(in.Config["networks"])["side"] = map[string]any{} },
		"services.networks": func(in *Input) {
			record(svc(t, *in, "runner")["networks"])["egress"] = nil
		},
		"services.ports": func(in *Input) {
			app(in)["ports"] = []any{map[string]any{"host_ip": "127.0.0.1", "published": "8080", "target": 8080.0}}
		},
		"services.network-mode": func(in *Input) { app(in)["network_mode"] = "host" },
		"services.extra-hosts":  func(in *Input) { app(in)["extra_hosts"] = []any{"host.docker.internal=host-gateway"} },
		"edge.upstreams":        func(in *Input) { setRoute(t, *in, 0, "30000=example.com:443") },
		"edge.proxy-pass":       func(in *Input) { edgePort(t, *in, 0)["target"] = 30050.0 },
		"edge.listen": func(in *Input) {
			setRoute(t, *in, 0, "30007=app:8080")
			edgePort(t, *in, 0)["target"] = 30007.0
		},
		"edge.host-ip":   func(in *Input) { edgePort(t, *in, 0)["host_ip"] = "0.0.0.0" },
		"edge.published": func(in *Input) { edgePort(t, *in, 0)["published"] = "23000" },
		"mounts.binds": func(in *Input) {
			for _, raw := range list(svc(t, *in, "runner")["volumes"]) {
				if strings.HasSuffix(text(record(raw)["target"]), "/scripts") {
					record(raw)["source"] = "/home/someone/.ssh"
				}
			}
		},
		"mounts.sockets": func(in *Input) {
			app(in)["volumes"] = []any{map[string]any{"type": "bind", "source": fixtureRun + "/src/docker.sock", "target": "/var/run/docker.sock"}}
		},
		"volumes.named": func(in *Input) {
			record(record(in.Config["volumes"])["db-data"])["driver_opts"] = map[string]any{"o": "bind", "device": "/etc"}
		},
		"privileges.privileged":   func(in *Input) { app(in)["privileged"] = true },
		"privileges.devices":      func(in *Input) { app(in)["devices"] = []any{"/dev/kvm"} },
		"privileges.namespaces":   func(in *Input) { app(in)["pid"] = "host" },
		"privileges.security-opt": func(in *Input) { app(in)["security_opt"] = []any{"seccomp=unconfined"} },
		"privileges.cap-add":      func(in *Input) { app(in)["cap_add"] = []any{"NET_ADMIN"} },
		"paths.configs-secrets": func(in *Input) {
			in.Config["configs"] = map[string]any{"leak": map[string]any{"file": "/etc/hosts"}}
		},
		"paths.env-file": func(in *Input) { app(in)["env_file"] = []any{map[string]any{"path": fixtureRun + "/src/.env"}} },
		"paths.build":    func(in *Input) { record(app(in)["build"])["context"] = "/elsewhere/app" },
	}
}

func TestEachViolatingCopyTurnsOnlyItsOwnAssertionRed(t *testing.T) {
	cases := violations(t)
	evaluated := 0
	for _, name := range assertionNames {
		mutate, ok := cases[name]
		if !ok {
			t.Errorf("assertion %s has no violating copy", name)
			continue
		}
		in := toyInput(t)
		mutate(&in)
		results := Static(in)
		if got := failing(results); !slices.Equal(got, []string{name}) {
			t.Errorf("violating copy for %s turned %v red", name, got)
		}
		if Verdict(results) != "failed" {
			t.Errorf("violating copy for %s: verdict %s", name, Verdict(results))
		}
		evaluated++
	}
	if evaluated != len(assertionNames) || len(cases) != len(assertionNames) {
		t.Fatalf("evaluated %d copies for %d assertions (%d copies defined)", evaluated, len(assertionNames), len(cases))
	}
}

func TestRunnerBindThroughALinkOutOfTheWorkTreeIsRejectedAndAPlainWorkBindPasses(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run := filepath.Join(base, "run")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{filepath.Join(run, "work", "scripts"), outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(run, "work", "escape")); err != nil {
		t.Fatal(err)
	}
	in := toyInput(t)
	in.Realpath = RealPath
	in.Layout.Work = filepath.Join(run, "work")
	binds := list(svc(t, in, "runner")["volumes"])
	in.Config["services"].(map[string]any)["runner"].(map[string]any)["volumes"] = []any{
		map[string]any{"type": "bind", "source": filepath.Join(run, "work", "scripts"), "target": "/work/scripts"},
	}
	byName := func() []string {
		var out []string
		for _, r := range Static(in) {
			if r.Name == "mounts.binds" && !r.OK {
				out = append(out, r.Detail)
			}
		}
		return out
	}
	for _, f := range byName() {
		if strings.Contains(f, "runner") {
			t.Fatalf("a plain work bind was rejected: %s", f)
		}
	}
	record(binds[0])["source"] = filepath.Join(run, "work", "escape", "x")
	in.Config["services"].(map[string]any)["runner"].(map[string]any)["volumes"] = binds[:1]
	found := byName()
	if len(found) != 1 || !strings.Contains(found[0], "runner") || !strings.Contains(found[0], "outside work") {
		t.Fatalf("a bind through a link out of work: %v", found)
	}
}

func TestRealPathResolvesThroughTheNearestExistingAncestorAndRefusesDanglingLinksAndRelativePaths(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "target"), filepath.Join(base, "dangling")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "real"), filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}
	ok := map[string]string{
		filepath.Join(base, "alias", "not", "yet"): filepath.Join(base, "real", "not", "yet"),
		filepath.Join(base, "real"):                filepath.Join(base, "real"),
	}
	for in, want := range ok {
		if got, resolved := RealPath(in); !resolved || got != want {
			t.Errorf("RealPath(%s) = %s %v, want %s", in, got, resolved, want)
		}
	}
	if err := os.WriteFile(filepath.Join(base, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, in := range []string{filepath.Join(base, "dangling", "x"), filepath.Join(base, "file", "x"), "relative/path", ""} {
		if got, resolved := RealPath(in); resolved {
			t.Errorf("RealPath(%q) resolved to %s", in, got)
		}
	}
}
