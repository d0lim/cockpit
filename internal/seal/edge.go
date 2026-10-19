package seal

import (
	"fmt"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
)

type route struct {
	listen int
	host   string
	port   int
}

func edgeRoutes(edge doc) ([]route, []string) {
	var problems []string
	if present(edge["entrypoint"]) {
		problems = append(problems, "the edge overrides its entrypoint")
	}
	command := list(edge["command"])
	if len(command) == 0 || text(command[0]) != "edge" {
		return nil, append(problems, "the edge command does not start with the edge mode")
	}
	var routes []route
	for _, raw := range command[1:] {
		spec := text(raw)
		l, upstream, ok := strings.Cut(spec, "=")
		listen, lerr := strconv.Atoi(l)
		host, p, herr := net.SplitHostPort(upstream)
		port, perr := strconv.Atoi(p)
		if !ok || lerr != nil || herr != nil || perr != nil || host == "" {
			problems = append(problems, fmt.Sprintf("edge route %q is not <port>=<host>:<port>", spec))
			continue
		}
		routes = append(routes, route{listen: listen, host: host, port: port})
	}
	return routes, problems
}

func setDiff(aName string, a []int, bName string, b []int) []string {
	only := func(from, other []int) []string {
		var out []string
		seen := map[int]bool{}
		for _, v := range from {
			if !slices.Contains(other, v) && !seen[v] {
				seen[v] = true
				out = append(out, strconv.Itoa(v))
			}
		}
		sort.Strings(out)
		return out
	}
	var out []string
	if x := only(a, b); len(x) > 0 {
		out = append(out, fmt.Sprintf("%s %s not in %s", aName, strings.Join(x, ","), bName))
	}
	if x := only(b, a); len(x) > 0 {
		out = append(out, fmt.Sprintf("%s %s not in %s", bName, strings.Join(x, ","), aName))
	}
	return out
}

func number(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case string:
		n, err := strconv.Atoi(x)
		if err != nil {
			return -1
		}
		return n
	}
	return -1
}

func edgeChecks(in Input, svcs []service) []Result {
	edge := record(record(in.Config["services"])[compose.RoleEdge])
	ports := list(edge["ports"])
	var targets []int
	for _, p := range ports {
		targets = append(targets, number(record(p)["target"]))
	}
	routes, problems := edgeRoutes(edge)
	names := map[string]bool{}
	for _, s := range svcs {
		names[s.name] = true
	}
	var upstreams []string
	upstreams = append(upstreams, problems...)
	var listens []int
	for _, r := range routes {
		listens = append(listens, r.listen)
		switch {
		case !names[r.host]:
			upstreams = append(upstreams, fmt.Sprintf("%d -> %s:%d is not a service of this project", r.listen, r.host, r.port))
		case slices.Contains(compose.HarnessServices, r.host):
			upstreams = append(upstreams, fmt.Sprintf("%d -> %s:%d targets a harness service", r.listen, r.host, r.port))
		}
	}
	if image := text(edge["image"]); image != in.RunnerImage {
		upstreams = append(upstreams, fmt.Sprintf("the edge runs %q instead of the runner image", image))
	}
	var expected []int
	for _, p := range in.Ports {
		expected = append(expected, compose.EdgeListen(p))
	}
	var hostIP []string
	for _, p := range ports {
		if ip := text(record(p)["host_ip"]); ip != "127.0.0.1" {
			if ip == "" {
				ip = "(none, every interface)"
			}
			hostIP = append(hostIP, fmt.Sprintf("%d: %s", number(record(p)["target"]), ip))
		}
	}
	return []Result{
		result("edge.upstreams", "the edge runs the runner image in edge mode and every route targets a service of this project", upstreams),
		result("edge.proxy-pass", "the routes of the offset map are exactly the targets of the edge ports", setDiff("route", listens, "ports.target", targets)),
		result("edge.listen", "the edge listens only on the ports of the declared offsets", setDiff("route", listens, "offset port", expected)),
		result("edge.host-ip", "every edge port binds 127.0.0.1", hostIP),
		published(in, ports, targets),
	}
}

func published(in Input, ports []any, targets []int) Result {
	start, end := in.PortBase, in.PortBase+compose.PortWidth
	var leased []int
	for _, p := range in.Ports {
		leased = append(leased, compose.HostPort(in.PortBase, p))
	}
	seen := map[int]bool{}
	var wrong []string
	for _, raw := range ports {
		v := number(record(raw)["published"])
		switch {
		case v < start || v >= end:
			wrong = append(wrong, fmt.Sprintf("%v: outside the leased block %d-%d", record(raw)["published"], start, end-1))
		case !slices.Contains(leased, v):
			wrong = append(wrong, fmt.Sprintf("%d: inside the block but not a declared offset", v))
		case seen[v]:
			wrong = append(wrong, fmt.Sprintf("%d: published twice", v))
		}
		seen[v] = true
	}
	distinct := map[int]bool{}
	for _, t := range targets {
		distinct[t] = true
	}
	if len(seen) != len(distinct) {
		wrong = append(wrong, fmt.Sprintf("%d published ports for %d targets", len(seen), len(distinct)))
	}
	return result("edge.published", "published ports are declared offsets inside the leased block, distinct, and as many as the targets", wrong)
}
