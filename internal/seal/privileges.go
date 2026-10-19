package seal

import (
	"fmt"
	"strings"
)

func privilegeChecks(svcs []service) []Result {
	var namespaces, unconfined, caps []string
	for _, s := range svcs {
		for _, field := range []string{"pid", "ipc", "userns_mode"} {
			if present(s.doc[field]) {
				namespaces = append(namespaces, fmt.Sprintf("%s: %s %v", s.name, field, s.doc[field]))
			}
		}
		for _, opt := range list(s.doc["security_opt"]) {
			if strings.Contains(strings.ToLower(fmt.Sprint(opt)), "unconfined") {
				unconfined = append(unconfined, fmt.Sprintf("%s: %v", s.name, opt))
			}
		}
		if added := list(s.doc["cap_add"]); len(added) > 0 {
			caps = append(caps, fmt.Sprintf("%s: %v", s.name, added))
		}
	}
	return []Result{
		result("privileges.privileged", "privileged appears nowhere", using(svcs, "privileged")),
		result("privileges.devices", "devices appears nowhere", using(svcs, "devices")),
		result("privileges.namespaces", "pid, ipc and userns_mode appear nowhere", namespaces),
		result("privileges.security-opt", "no unconfined security_opt", unconfined),
		result("privileges.cap-add", "cap_add appears nowhere", caps),
	}
}

func pathChecks(in Input, svcs []service) []Result {
	run, runOK := in.Realpath(in.RunDir)
	inRun := func(path string) bool {
		resolved, ok := in.Realpath(path)
		return ok && runOK && inside(run, resolved)
	}
	var files []string
	for _, kind := range []string{"configs", "secrets"} {
		table := record(in.Config[kind])
		for _, name := range sortedNames(table) {
			if file := text(record(table[name])["file"]); file != "" && !inRun(file) {
				files = append(files, fmt.Sprintf("%s.%s: %s", kind, name, file))
			}
		}
	}
	var builds []string
	for _, s := range svcs {
		if !present(s.doc["build"]) {
			continue
		}
		b := record(s.doc["build"])
		contexts := []string{text(b["context"])}
		for _, c := range record(b["additional_contexts"]) {
			contexts = append(contexts, text(c))
		}
		for _, c := range list(b["additional_contexts"]) {
			contexts = append(contexts, text(c))
		}
		for _, c := range contexts {
			if !inRun(c) {
				builds = append(builds, fmt.Sprintf("%s: build context %q is outside the run directory", s.name, c))
			}
		}
		for _, field := range []string{"ssh", "secrets"} {
			if present(b[field]) {
				builds = append(builds, fmt.Sprintf("%s: build.%s", s.name, field))
			}
		}
	}
	return []Result{
		result("paths.configs-secrets", "top-level configs and secrets read files inside the run directory", files),
		result("paths.env-file", "env_file appears nowhere", using(svcs, "env_file")),
		result("paths.build", "build contexts are inside the run directory and builds use no ssh or secrets", builds),
	}
}
