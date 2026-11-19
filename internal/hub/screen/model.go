package screen

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/labels"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

type Tone string

const (
	ToneOK   Tone = "ok"
	ToneWarn Tone = "warn"
	ToneBad  Tone = "bad"
	ToneInfo Tone = "info"
	ToneIdle Tone = "idle"
)

var activePhases = []lifecycle.Phase{lifecycle.Preparing, lifecycle.Building, lifecycle.Sealing, lifecycle.Booting, lifecycle.Testing, lifecycle.Reporting}

func isActive(r api.RunItem) bool { return r.Mark == "" && slices.Contains(activePhases, r.Phase) }

func isPending(r api.RunItem) bool { return r.Phase == lifecycle.Queued || isActive(r) }

func inPool(r api.RunItem) bool {
	settled := r.Phase == lifecycle.Done || r.Phase == lifecycle.Stopping || r.Phase == lifecycle.Resuming
	return r.Mark == "" && settled && (r.Env == lifecycle.EnvUp || r.Env == lifecycle.EnvStopped)
}

func stoppable(r api.RunItem) bool {
	return r.Mark == "" && r.Phase == lifecycle.Done && r.Env == lifecycle.EnvUp
}

func phaseTone(r api.RunItem) Tone {
	switch {
	case r.Mark != "":
		return ToneBad
	case r.Phase == lifecycle.Queued:
		return ToneIdle
	case isPending(r), r.Phase != lifecycle.Done:
		return ToneWarn
	case r.Env == lifecycle.EnvUp:
		return ToneOK
	case r.Env == lifecycle.EnvStopped:
		return ToneInfo
	}
	return ToneIdle
}

func verdictTone(v lifecycle.Verdict) Tone {
	switch v {
	case lifecycle.VerdictPassed:
		return ToneOK
	case lifecycle.VerdictFailed, lifecycle.VerdictError:
		return ToneBad
	case lifecycle.VerdictAborted:
		return ToneWarn
	}
	return ToneIdle
}

type RunGroups struct {
	Pending []api.RunItem
	Pool    []api.RunItem
	Ended   []api.RunItem
}

func finishedKey(r api.RunItem) string {
	if r.FinishedAt != "" {
		return r.FinishedAt
	}
	return r.CreatedAt
}

func groupRuns(l api.HubList) RunGroups {
	var g RunGroups
	for _, r := range l.Runs {
		switch {
		case isPending(r):
			g.Pending = append(g.Pending, r)
		case inPool(r):
			g.Pool = append(g.Pool, r)
		default:
			g.Ended = append(g.Ended, r)
		}
	}
	slices.SortStableFunc(g.Pending, func(a, b api.RunItem) int { return a.Position - b.Position })
	slices.SortStableFunc(g.Pool, func(a, b api.RunItem) int { return strings.Compare(finishedKey(b), finishedKey(a)) })
	slices.SortStableFunc(g.Ended, func(a, b api.RunItem) int { return strings.Compare(b.CreatedAt, a.CreatedAt) })
	return g
}

const endedShown = 30

type shownGroups struct {
	RunGroups
	more int
}

var verdictOrder = []lifecycle.Verdict{lifecycle.VerdictFailed, lifecycle.VerdictError, lifecycle.VerdictAborted, lifecycle.VerdictPassed}

func (st *state) verdictPick() lifecycle.Verdict {
	v := lifecycle.Verdict(st.query.Get("verdict"))
	if slices.Contains(verdictOrder, v) {
		return v
	}
	return lifecycle.VerdictNone
}

func (st *state) runGroups() shownGroups {
	l := st.list
	if pick := st.verdictPick(); pick != lifecycle.VerdictNone {
		l.Runs = slices.DeleteFunc(slices.Clone(l.Runs), func(r api.RunItem) bool { return r.Verdict != pick })
	}
	g := shownGroups{RunGroups: groupRuns(l)}
	if st.query.Get("all") == "" && len(g.Ended) > endedShown {
		g.more = len(g.Ended) - endedShown
		g.Ended = g.Ended[:endedShown]
	}
	return g
}

type verdictFilter struct {
	label string
	count int
	href  string
	on    bool
}

func (st *state) verdictFilters() []verdictFilter {
	counts := map[lifecycle.Verdict]int{}
	for _, r := range st.list.Runs {
		counts[r.Verdict]++
	}
	pick := st.verdictPick()
	out := []verdictFilter{{label: "All", count: len(st.list.Runs), href: st.href(map[string][]string{"verdict": nil}), on: pick == lifecycle.VerdictNone}}
	for _, v := range verdictOrder {
		if counts[v] == 0 && v != pick {
			continue
		}
		name := string(v)
		if l := labels.Verdict(v); l != nil {
			name = *l
		}
		out = append(out, verdictFilter{label: name, count: counts[v], href: st.href(map[string][]string{"verdict": {string(v)}}), on: pick == v})
	}
	return out
}

func rowWhy(r api.RunItem) string {
	if r.Verdict != lifecycle.VerdictFailed && r.Verdict != lifecycle.VerdictError && r.Mark == "" {
		return ""
	}
	var parts []string
	if r.FailedStage != "" {
		parts = append(parts, "stopped at the "+lifecycle.StageLabel(r.FailedStage)+" stage")
	}
	if r.Notice != "" {
		parts = append(parts, r.Notice)
	}
	return strings.Join(parts, " · ")
}

func orElse(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func nextDestroy(runs []api.RunItem) string {
	var pick *api.RunItem
	for i, r := range runs {
		if inPool(r) && r.Phase == lifecycle.Done && !r.Keep {
			if pick == nil || finishedKey(r)+r.ID < finishedKey(*pick)+pick.ID {
				pick = &runs[i]
			}
		}
	}
	if pick == nil {
		return ""
	}
	return pick.ID
}

func nextStop(runs []api.RunItem) string {
	key := func(r api.RunItem) string { return orElse(r.LastUpAt, r.FinishedAt) + r.ID }
	var pick *api.RunItem
	for i, r := range runs {
		if stoppable(r) && (pick == nil || key(r) < key(*pick)) {
			pick = &runs[i]
		}
	}
	if pick == nil {
		return ""
	}
	return pick.ID
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func stamp(iso string) string {
	t, err := time.Parse(time.RFC3339Nano, iso)
	if err != nil {
		return "-"
	}
	return t.Local().Format("01-02 15:04")
}

func clock(iso string) string {
	t, err := time.Parse(time.RFC3339Nano, iso)
	if err != nil {
		return ""
	}
	return t.Local().Format("15:04:05")
}

func label(table map[string]string, key string) string {
	if v, ok := table[key]; ok {
		return v
	}
	return key
}

func rowDetail(r api.RunItem) string {
	switch {
	case r.Phase == lifecycle.Queued:
		reason := "starting soon"
		if r.WaitReason != "" {
			reason = label(labelTable.Wait, string(r.WaitReason))
		}
		return "queued #" + strconv.Itoa(r.Position) + " · " + reason
	case isActive(r):
		return "started " + stamp(r.StartedAt)
	}
	out := "by " + r.Submitter + " · finished " + stamp(r.FinishedAt)
	if took := tookOf(r); took != "" {
		out += " · took " + took
	}
	switch {
	case r.Env == lifecycle.EnvStopped && r.StopReason != "":
		out += " · " + label(labelTable.Stop, string(r.StopReason))
	case r.Env == lifecycle.EnvDestroyed:
		out += " · only the report remains"
	}
	return out
}

func served(r api.RunItem, l api.HubList) bool { return slices.Contains(l.Served, r.HostAPI) }

func defaultPath(r api.RunItem, l api.HubList) string {
	base := "/runs/" + r.ID
	switch {
	case r.Phase == lifecycle.Queued || !served(r, l):
		return base
	case isActive(r):
		return base + "/signals/trace"
	}
	return base + "/report"
}
