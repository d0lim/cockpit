package screen

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

const (
	logCollector  = "logs"
	servicesTable = "services"
	containerAttr = "container"
	eventWindow   = 500
	tableWindow   = 500
	logWindow     = 2000
	bodyLimit     = 4000
	noStatus      = "(no status)"
)

func attrText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool, int, int64:
		return fmt.Sprint(x)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(raw)
}

func revKey(s contract.Signal) string {
	key := s.Collector + "\x00" + string(s.Kind) + "\x00" + s.ID
	if s.Span != nil {
		key += "\x00" + s.Span.TraceID
	}
	return key
}

func latestRevisions(signals []contract.Signal) []contract.Signal {
	versioned := func(s contract.Signal) bool { return s.ID != "" && s.Kind != contract.SignalKindMetric }
	winners := map[string]contract.Signal{}
	for _, s := range signals {
		if !versioned(s) {
			continue
		}
		cur, ok := winners[revKey(s)]
		if !ok || s.Rev > cur.Rev || s.Rev == cur.Rev && s.Seq > cur.Seq {
			winners[revKey(s)] = s
		}
	}
	out := make([]contract.Signal, 0, len(signals))
	for _, s := range signals {
		if !versioned(s) || winners[revKey(s)].Seq == s.Seq {
			out = append(out, s)
		}
	}
	return out
}

func placement(seq int64, snap signalSnapshot) string {
	if !snap.frozen {
		return ""
	}
	if seq <= snap.frozenSeq {
		return "in bundle"
	}
	return "after the bundle"
}

func droppedLine(dropped map[string]int64) string {
	var keys []string
	var total int64
	for k, n := range dropped {
		if n > 0 {
			keys = append(keys, k)
			total += n
		}
	}
	if total == 0 {
		return ""
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + " " + strconv.FormatInt(dropped[k], 10)
	}
	return strconv.FormatInt(total, 10) + " older records dropped (" + strings.Join(parts, ", ") + ")"
}

var emptySentences = map[string]string{
	"before-boot":        "The environment has not booted yet. Signals start once it is up.",
	"env-off":            "The environment is off. Only the stored signals are shown.",
	"no-collectors":      "This run declares no collectors, so only hub signals appear here.",
	"none-of-kind":       "This run has signals, but none of the kind this view shows.",
	"collectors-stopped": "Collectors are stopped, so nothing new arrives until they are armed again.",
	"no-signals":         "No signals yet. Collectors are armed and nothing has arrived.",
}

var preBoot = []lifecycle.Phase{lifecycle.Queued, lifecycle.Preparing, lifecycle.Building, lifecycle.Sealing, lifecycle.Booting}

func emptyKind(r api.RunItem, collectors []api.CollectorState, count, total int) string {
	booting := slices.Contains(preBoot, r.Phase)
	switch {
	case count > 0 && (r.Env == lifecycle.EnvUp || booting):
		return ""
	case count > 0:
		return "env-off"
	case booting:
		return "before-boot"
	case r.Env != lifecycle.EnvUp:
		return "env-off"
	case len(collectors) == 0:
		return "no-collectors"
	case total > 0:
		return "none-of-kind"
	}
	for _, c := range collectors {
		if c.State != "stopped" {
			return "no-signals"
		}
	}
	return "collectors-stopped"
}

func lastN[T any](rows []T, n int) []T {
	if len(rows) > n {
		return rows[len(rows)-n:]
	}
	return rows
}

func eventRows(signals []contract.Signal) []contract.Signal {
	var out []contract.Signal
	for _, s := range signals {
		if s.Kind == contract.SignalKindEvent && s.Collector != logCollector {
			out = append(out, s)
		}
	}
	return out
}

func statusOf(s contract.Signal) string { return orElse(attrText(s.Attrs["status"]), noStatus) }

func toneOfAttr(v any) Tone {
	switch t := Tone(attrText(v)); t {
	case ToneOK, ToneWarn, ToneBad, ToneInfo, ToneIdle:
		return t
	}
	return ToneIdle
}

type chip struct{ key, value string }

func (c chip) String() string { return c.key + ":" + c.value }

func chipsOf(rows []contract.Signal) []chip {
	seen := map[string]bool{}
	var out []chip
	add := func(c chip) {
		if !seen[c.String()] {
			seen[c.String()] = true
			out = append(out, c)
		}
	}
	for _, s := range rows {
		add(chip{"collector", s.Collector})
		for _, key := range []string{"status", "tone", "table"} {
			if v := attrText(s.Attrs[key]); v != "" {
				add(chip{key, v})
			}
		}
	}
	return out
}

func parseChips(values []string) []chip {
	var out []chip
	for _, v := range values {
		if key, value, ok := strings.Cut(v, ":"); ok {
			out = append(out, chip{key, value})
		}
	}
	return out
}

func matches(s contract.Signal, chips []chip) bool {
	for _, c := range chips {
		got := attrText(s.Attrs[c.key])
		if c.key == "collector" {
			got = s.Collector
		}
		if got != c.value {
			return false
		}
	}
	return true
}

type group struct {
	status string
	count  int
	tone   Tone
}

func groups(rows []contract.Signal) []group {
	var out []group
	at := map[string]int{}
	for _, s := range rows {
		st := statusOf(s)
		i, ok := at[st]
		if !ok {
			i = len(out)
			at[st] = i
			out = append(out, group{status: st, tone: toneOfAttr(s.Attrs["tone"])})
		}
		out[i].count++
	}
	return out
}

func selectionKey(s contract.Signal) string {
	if s.ID != "" {
		return s.Collector + "/" + s.ID
	}
	return "seq:" + strconv.FormatInt(s.Seq, 10)
}

func bodyText(s contract.Signal) (string, bool) {
	if s.Event == nil {
		return "", false
	}
	if len(s.Event.Body) > bodyLimit {
		return s.Event.Body[:bodyLimit], true
	}
	return s.Event.Body, false
}
