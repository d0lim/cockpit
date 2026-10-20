package api

import (
	"slices"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/labels"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
	"github.com/d0lim/cockpit/internal/hub/pool"
	"github.com/d0lim/cockpit/internal/hub/reconcile"
	"github.com/d0lim/cockpit/internal/hub/registry"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
	"github.com/d0lim/cockpit/internal/hub/store"
)

type RunItem struct {
	ID             string                  `json:"id"`
	Phase          lifecycle.Phase         `json:"phase"`
	Env            lifecycle.Env           `json:"env"`
	Verdict        lifecycle.Verdict       `json:"verdict"`
	Mark           lifecycle.Mark          `json:"mark,omitempty"`
	SHA            string                  `json:"sha"`
	Suite          string                  `json:"suite"`
	HostAPI        int                     `json:"hostApi"`
	CockpitVersion string                  `json:"cockpitVersion"`
	Submitter      string                  `json:"submitter"`
	Keep           bool                    `json:"keep"`
	CreatedAt      string                  `json:"createdAt"`
	StartedAt      string                  `json:"startedAt,omitempty"`
	FinishedAt     string                  `json:"finishedAt,omitempty"`
	LastUpAt       string                  `json:"lastUpAt,omitempty"`
	Slot           *int                    `json:"slot,omitempty"`
	Position       int                     `json:"position,omitempty"`
	WaitReason     lifecycle.WaitReason    `json:"waitReason,omitempty"`
	StopReason     lifecycle.StopReason    `json:"stopReason,omitempty"`
	DestroyReason  lifecycle.DestroyReason `json:"destroyReason,omitempty"`
	FailedStage    lifecycle.Phase         `json:"failedStage,omitempty"`
	Notice         string                  `json:"notice,omitempty"`
	Recreate       bool                    `json:"recreate,omitempty"`
	Redaction      *store.Redaction        `json:"redaction,omitempty"`
	PhaseLabel     string                  `json:"phaseLabel"`
	VerdictLabel   *string                 `json:"verdictLabel"`
}

type ServicePort struct {
	Name     string            `json:"name"`
	Service  string            `json:"service"`
	Port     int               `json:"port"`
	HostPort int               `json:"hostPort"`
	Label    string            `json:"label"`
	Open     contract.OpenMode `json:"open,omitempty"`
}

type RunDetail struct {
	RunItem
	ImageKeys     map[string]string     `json:"imageKeys,omitempty"`
	ImageDigests  map[string]string     `json:"imageDigests,omitempty"`
	RunnerImage   *contract.RunnerImage `json:"runnerImage,omitempty"`
	Tail          []string              `json:"tail,omitempty"`
	AllowedEvents []lifecycle.Event     `json:"allowedEvents"`
	Services      []ServicePort         `json:"services"`
}

type OrphanItem struct {
	reconcile.Orphan
	Hub string `json:"hub,omitempty"`
}

type HubItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Home string `json:"home"`
	Self bool   `json:"self"`
	Up   bool   `json:"up"`
	URL  string `json:"url,omitempty"`
}

type HubList struct {
	Runs       []RunItem       `json:"runs"`
	Orphans    []OrphanItem    `json:"orphans"`
	Foreign    []string        `json:"foreign"`
	Hubs       []HubItem       `json:"hubs"`
	Pool       scheduler.Size  `json:"pool"`
	Up         scheduler.Count `json:"up"`
	DockerDown bool            `json:"dockerDown"`
	TickError  *string         `json:"tickError"`
	Served     []int           `json:"servedHostApis"`
}

type Submitted struct {
	ID       string          `json:"id"`
	Phase    lifecycle.Phase `json:"phase"`
	Position int             `json:"position"`
	Merged   bool            `json:"merged"`
}

type OpReply struct {
	Run RunItem `json:"run"`
}

var userEvents = []lifecycle.Event{lifecycle.Stop, lifecycle.Resume, lifecycle.Destroy, lifecycle.Abort}

func Item(v scheduler.RunView) RunItem {
	m := v.Meta
	return RunItem{
		ID: m.ID, Phase: m.Phase, Env: m.Env, Verdict: m.Verdict, Mark: m.Mark, SHA: m.SHA, Suite: m.Suite,
		HostAPI: m.HostAPI, CockpitVersion: m.CockpitVersion, Submitter: m.Submitter, Keep: m.Keep,
		CreatedAt: m.CreatedAt, StartedAt: m.StartedAt, FinishedAt: m.FinishedAt, LastUpAt: m.LastUpAt,
		Slot: m.Slot, Position: v.Position, WaitReason: m.WaitReason, StopReason: m.StopReason,
		DestroyReason: m.DestroyReason, FailedStage: m.FailedStage, Notice: m.Notice, Recreate: m.Recreate,
		Redaction: m.Redaction, PhaseLabel: lifecycle.Label(m.State), VerdictLabel: labels.Verdict(m.Verdict),
	}
}

func MetaItem(m store.Meta) RunItem { return Item(scheduler.RunView{Meta: m}) }

func Detail(v scheduler.RunView, allowed []lifecycle.Event, services []ServicePort) RunDetail {
	events := []lifecycle.Event{}
	for _, e := range allowed {
		if slices.Contains(userEvents, e) {
			events = append(events, e)
		}
	}
	if services == nil {
		services = []ServicePort{}
	}
	return RunDetail{
		RunItem: Item(v), ImageKeys: v.ImageKeys, ImageDigests: v.ImageDigests, RunnerImage: v.RunnerImage,
		Tail: v.Tail, AllowedEvents: events, Services: services,
	}
}

func PublicList(l scheduler.HubList, hubs []registry.Hub) HubList {
	out := HubList{
		Runs: []RunItem{}, Orphans: []OrphanItem{}, Foreign: append([]string{}, l.Foreign...), Hubs: []HubItem{},
		Pool: l.Pool, Up: l.Up, DockerDown: l.DockerDown, Served: append([]int{}, l.Served...),
	}
	for _, o := range l.Orphans {
		out.Orphans = append(out.Orphans, OrphanItem{Orphan: o, Hub: registry.Owner(hubs, o.Project)})
	}
	for _, h := range hubs {
		out.Hubs = append(out.Hubs, HubItem{ID: h.ID, Name: h.Name, Home: h.Home, Self: h.Self, Up: h.Up, URL: h.URL()})
	}
	if l.TickError != "" {
		out.TickError = &l.TickError
	}
	for _, v := range l.Runs {
		out.Runs = append(out.Runs, Item(v))
	}
	return out
}

func holdsPorts(m store.Meta) bool { return m.Slot != nil && store.HoldsLease(m) }

func execAccepts(m store.Meta) bool { return pool.Stoppable(m) && m.Slot != nil }
