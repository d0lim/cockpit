package seal

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
)

var socketPath = regexp.MustCompile(`\.sock$|docker\.sock`)

func bindRoot(in Input, name string) (string, string) {
	switch {
	case name == in.Builder && in.Builder != "":
		return "build", in.Layout.Build
	case name == compose.RoleRunner:
		return "work", in.Layout.Work
	case slices.Contains(compose.HarnessServices, name):
		return "", ""
	}
	return "src", in.Layout.Src
}

func mountChecks(in Input, svcs []service) []Result {
	var outside, sockets []string
	for _, s := range svcs {
		label, root := bindRoot(in, s.name)
		realRoot, rootOK := in.Realpath(root)
		for _, raw := range list(s.doc["volumes"]) {
			m := record(raw)
			if text(m["type"]) != "bind" {
				continue
			}
			source := text(m["source"])
			resolved, ok := in.Realpath(source)
			if socketPath.MatchString(source) || (ok && socketPath.MatchString(resolved)) {
				sockets = append(sockets, s.name+": "+source)
			}
			switch {
			case label == "":
				outside = append(outside, fmt.Sprintf("%s: binds nothing by design, found %s", s.name, source))
			case !ok || !rootOK:
				outside = append(outside, fmt.Sprintf("%s: cannot resolve the real path of %s", s.name, source))
			case !inside(realRoot, resolved):
				outside = append(outside, fmt.Sprintf("%s: %s is outside %s", s.name, source, label))
			}
		}
	}
	return []Result{
		result("mounts.binds", "bind sources resolve under build for the builder, under work for the runner, under src for project services, and the edge and probes bind nothing", outside),
		result("mounts.sockets", "no socket is bound", sockets),
	}
}

func volumeUsers(svcs []service, volume string) []string {
	var users []string
	for _, s := range svcs {
		for _, raw := range list(s.doc["volumes"]) {
			m := record(raw)
			if text(m["type"]) == "volume" && text(m["source"]) == volume {
				users = append(users, s.name)
				break
			}
		}
	}
	return users
}

func volumeCheck(config doc, svcs []service, builder string) Result {
	vols := record(config["volumes"])
	var wrong []string
	for _, name := range sortedNames(vols) {
		v := record(vols[name])
		if v["external"] == true {
			users := volumeUsers(svcs, name)
			if builder == "" || slices.ContainsFunc(users, func(u string) bool { return u != builder }) {
				wrong = append(wrong, fmt.Sprintf("%s: external and used by %s", name, strings.Join(users, ",")))
			}
			continue
		}
		if present(v["driver_opts"]) {
			wrong = append(wrong, name+": has driver_opts")
		}
		if d := text(v["driver"]); d != "" && d != "local" {
			wrong = append(wrong, name+": driver "+d)
		}
	}
	return result("volumes.named", "named volumes are project-owned (local, no driver_opts) and external volumes serve only the builder", wrong)
}
