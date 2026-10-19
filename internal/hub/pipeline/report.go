package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/envops"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/runfiles"
	"github.com/d0lim/cockpit/internal/version"
)

func errorOutcome(format string, args ...any) (reconcile.ReportOutcome, string, error) {
	return reconcile.ReportOutcome{Verdict: lifecycle.VerdictError}, fmt.Sprintf(format, args...), nil
}

func SameVersion(a, b string) bool {
	if a == b {
		return true
	}
	unknown := func(x, y string) bool { return x == "dev" && strings.HasPrefix(y, "dev+") }
	return unknown(a, b) || unknown(b, a)
}

func IdentityMismatch(m *contract.Manifest, pin *contract.RunnerImage, cockpitVersion string) string {
	if m == nil || m.Runner == nil {
		return "the manifest carries no runner identity"
	}
	if pin == nil || m.Runner.Image != pin.Tag || !SameVersion(m.Runner.Version, cockpitVersion) {
		pinTag := ""
		if pin != nil {
			pinTag = pin.Tag
		}
		return fmt.Sprintf("runner identity mismatch: the manifest says image %s version %s, the run pinned image %s version %s", m.Runner.Image, m.Runner.Version, pinTag, cockpitVersion)
	}
	return ""
}

func DefaultReport(ctx context.Context, in envops.ReportInput) (reconcile.ReportOutcome, string, error) {
	r := in.Runner
	switch {
	case r != nil && r.OOMKilled:
		return errorOutcome("the runner was killed for running out of memory")
	case r != nil && r.TimedOut:
		return errorOutcome("the runner passed its time limit")
	case r != nil && r.Code == nil:
		return errorOutcome("the runner left no exit code")
	case r != nil && *r.Code != 0 && *r.Code != 1:
		return errorOutcome("the runner exit code %d is outside 0 and 1", *r.Code)
	}
	manifest, err := runfiles.Manifest(in.Root)
	if err != nil {
		return errorOutcome("the runner manifest cannot be read: %s", in.Mask(err.Error()))
	}
	cockpitVersion := in.Meta.CockpitVersion
	if cockpitVersion == "" {
		cockpitVersion = version.String()
	}
	if mismatch := IdentityMismatch(manifest, in.Meta.RunnerImage, cockpitVersion); mismatch != "" {
		return errorOutcome("%s", mismatch)
	}
	result, err := runfiles.Result(in.Root)
	if err != nil {
		return errorOutcome("the runner result cannot be read: %s", in.Mask(err.Error()))
	}
	passed, failed := 0, 0
	for _, s := range result.Steps {
		switch s.Status {
		case contract.StepStatusPassed:
			passed++
		case contract.StepStatusFailed:
			failed++
		}
	}
	switch {
	case failed > 0 || (r != nil && *r.Code == 1):
		return reconcile.ReportOutcome{Verdict: lifecycle.VerdictFailed}, fmt.Sprintf("%d steps failed", failed), nil
	case passed == 0 || passed != len(result.Steps):
		return errorOutcome("%d of %d steps passed and none failed", passed, len(result.Steps))
	}
	return reconcile.ReportOutcome{Verdict: lifecycle.VerdictPassed}, "", nil
}
