package stages

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
)

const (
	diagServices = 3
	diagLines    = 40
)

func broken(r compose.PsRow) bool {
	return r.State == "dead" || (r.State == "exited" && r.ExitCode != 0) || r.Health == "unhealthy"
}

func notReady(r compose.PsRow) bool {
	if r.State == "exited" && r.ExitCode == 0 {
		return false
	}
	return r.State != "running" || (r.Health != "" && r.Health != "healthy")
}

func Culprits(rows []compose.PsRow, expected []string, timedOut bool) []compose.PsRow {
	var ordered []compose.PsRow
	for _, name := range expected {
		for _, r := range rows {
			if r.Service == name && !r.OneOff() {
				ordered = append(ordered, r)
			}
		}
	}
	for _, r := range rows {
		if !r.OneOff() && !slices.Contains(expected, r.Service) {
			ordered = append(ordered, r)
		}
	}
	pick := func(match func(compose.PsRow) bool) []compose.PsRow {
		var out []compose.PsRow
		for _, r := range ordered {
			if match(r) && len(out) < diagServices && !slices.ContainsFunc(out, func(o compose.PsRow) bool { return o.Service == r.Service }) {
				out = append(out, r)
			}
		}
		return out
	}
	if out := pick(broken); len(out) > 0 || !timedOut {
		return out
	}
	return pick(func(r compose.PsRow) bool { return slices.Contains(expected, r.Service) && notReady(r) })
}

func CulpritText(r compose.PsRow) string {
	switch {
	case r.State == "dead":
		return r.Service + " died"
	case r.State == "exited":
		return r.Service + " exited with code " + strconv.Itoa(r.ExitCode)
	case r.Health == "unhealthy":
		return r.Service + " is unhealthy"
	}
	return fmt.Sprintf("%s is %s %s", r.Service, r.State, r.Health)
}

func (e *Env) diagnose(ctx context.Context, cause error) error {
	var waited *Failed
	rows, err := e.Project.Ps(ctx)
	if err != nil {
		return cause
	}
	culprits := Culprits(rows, e.Loaded.Declaration.Project.ExpectedServices, errors.As(cause, &waited))
	texts := make([]string, len(culprits))
	for i, r := range culprits {
		texts[i] = CulpritText(r)
		res, err := e.Project.ByName(ctx, "compose logs", "logs", "--no-color", "--tail", strconv.Itoa(diagLines), r.Service)
		if err != nil {
			e.logLine(r.Service + " | the log could not be read: " + err.Error())
			continue
		}
		for _, line := range strings.Split(string(res.Stdout), "\n") {
			if strings.TrimSpace(line) != "" {
				e.logLine(line)
			}
		}
	}
	var up *compose.Failed
	if errors.As(cause, &up) && up.Step == compose.StepUp && len(texts) > 0 {
		return &Failed{Message: "compose up failed: " + strings.Join(texts, ", ")}
	}
	return cause
}
