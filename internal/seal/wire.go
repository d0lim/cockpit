package seal

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
)

const wireExpected = "without wire no service holds the wire role; with wire the recorder runs the runner image in wire mode, with no entrypoint override, volumes or tmpfs, and proxies exactly the declared routes to project services"

type wireCommand struct {
	project string
	routes  []compose.WireRoute
}

func parseWireCommand(command []any) (wireCommand, []string) {
	if len(command) == 0 || text(command[0]) != "wire" {
		return wireCommand{}, []string{"the recorder command does not start with the wire mode"}
	}
	var parsed wireCommand
	var problems []string
	args := command[1:]
	for i := 0; i < len(args); i++ {
		arg := text(args[i])
		switch {
		case arg == "--project" && i+1 < len(args):
			i++
			parsed.project = text(args[i])
		case arg == "--public-field" && i+1 < len(args):
			i++
		default:
			route, ok := parseWireRoute(arg)
			if !ok {
				problems = append(problems, fmt.Sprintf("wire route %q is not <port>=<service>:<port>", arg))
				continue
			}
			parsed.routes = append(parsed.routes, route)
		}
	}
	return parsed, problems
}

func parseWireRoute(spec string) (compose.WireRoute, bool) {
	listen, upstream, ok := strings.Cut(spec, "=")
	service, port, found := strings.Cut(upstream, ":")
	l, lerr := strconv.Atoi(listen)
	p, perr := strconv.Atoi(port)
	if !ok || !found || service == "" || lerr != nil || perr != nil {
		return compose.WireRoute{}, false
	}
	return compose.WireRoute{Service: service, Port: p, Listen: l}, true
}

func holdsWireRole(s service) bool {
	return text(record(s.doc["labels"])[compose.LabelRole]) == compose.RoleWire
}

func wireCheck(in Input, svcs []service) Result {
	var wrong []string
	if !in.Wire {
		for _, s := range svcs {
			if s.name == compose.RoleWire || holdsWireRole(s) {
				wrong = append(wrong, s.name+" is a wire recorder in a run that does not record")
			}
		}
		return result("wire.routes", wireExpected, wrong)
	}
	at := slices.IndexFunc(svcs, func(s service) bool { return s.name == compose.RoleWire })
	if at < 0 {
		return result("wire.routes", wireExpected, []string{"the run records and has no " + compose.RoleWire + " service"})
	}
	recorder := svcs[at].doc
	if image := text(recorder["image"]); image != in.RunnerImage {
		wrong = append(wrong, fmt.Sprintf("the recorder runs %q instead of the runner image", image))
	}
	for _, field := range []string{"entrypoint", "volumes", "tmpfs"} {
		if present(recorder[field]) {
			wrong = append(wrong, "the recorder sets "+field)
		}
	}
	parsed, problems := parseWireCommand(list(recorder["command"]))
	wrong = append(wrong, problems...)
	if parsed.project != in.RunID {
		wrong = append(wrong, fmt.Sprintf("the recorder names project %q", parsed.project))
	}
	return result("wire.routes", wireExpected, append(wrong, wireRouteProblems(in, svcs, parsed.routes)...))
}

func wireRouteProblems(in Input, svcs []service, got []compose.WireRoute) []string {
	var wrong []string
	key := func(r compose.WireRoute) string { return fmt.Sprintf("%d=%s", r.Listen, r.Upstream()) }
	seen := map[string]bool{}
	for _, r := range got {
		seen[key(r)] = true
		switch {
		case !slices.ContainsFunc(svcs, func(s service) bool { return s.name == r.Service }):
			wrong = append(wrong, key(r)+" is not a service of this project")
		case slices.Contains(compose.HarnessServices, r.Service):
			wrong = append(wrong, key(r)+" targets a harness service")
		}
		if !slices.ContainsFunc(in.WireRoutes, func(w compose.WireRoute) bool { return key(w) == key(r) }) {
			wrong = append(wrong, key(r)+" is not a declared route")
		}
	}
	for _, w := range in.WireRoutes {
		if !seen[key(w)] {
			wrong = append(wrong, key(w)+" is declared and not proxied")
		}
	}
	return wrong
}
