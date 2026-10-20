package labels

import (
	"regexp"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

func TestEveryLifecycleStateStageAndReasonHasALabel(t *testing.T) {
	table := Load()
	keys := 0
	for key := range lifecycle.Transitions {
		if key == lifecycle.KeyNew {
			continue
		}
		if table.States[string(key)] == "" {
			t.Errorf("state %s has no label", key)
		}
		keys++
	}
	for _, reason := range []lifecycle.WaitReason{lifecycle.WaitActiveRun, lifecycle.WaitPoolAllKept, lifecycle.WaitPoolOver, lifecycle.WaitInProgress, lifecycle.WaitDockerUnavailable, lifecycle.WaitNoPortBlock} {
		if table.Wait[string(reason)] == "" {
			t.Errorf("wait reason %s has no label", reason)
		}
	}
	for _, v := range []lifecycle.Verdict{lifecycle.VerdictPassed, lifecycle.VerdictFailed, lifecycle.VerdictError, lifecycle.VerdictAborted} {
		if table.Verdicts[string(v)] == "" || Verdict(v) == nil {
			t.Errorf("verdict %s has no label", v)
		}
	}
	if Verdict(lifecycle.VerdictNone) != nil || keys < 15 {
		t.Fatalf("no verdict has a label, or only %d states", keys)
	}
}

func TestMessagesFillTheirPlaceholdersAndLeaveNoneBehind(t *testing.T) {
	placeholder := regexp.MustCompile(`\{[a-zA-Z]+\}`)
	vars := map[string]string{}
	for _, text := range messages {
		for _, p := range placeholder.FindAllString(text, -1) {
			vars[p[1:len(p)-1]] = "x"
		}
	}
	for m := range messages {
		if out := Text(m, vars); placeholder.MatchString(out) || out == "" {
			t.Errorf("%s: %q", m, out)
		}
	}
	if got := Text(MsgWaitHint, map[string]string{"cmd": "cockpit runs", "id": "r1"}); got != "to wait: cockpit runs wait r1" {
		t.Fatalf("wait hint %q", got)
	}
	if len(vars) < 10 {
		t.Fatalf("only %d placeholders", len(vars))
	}
}
