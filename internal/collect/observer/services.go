package observer

import (
	"context"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	dockerport "github.com/d0lim/cockpit/internal/ports/docker"
)

const ServicesCollector = "services"

type PS interface {
	PS(ctx context.Context) ([]dockerport.ServiceState, error)
}

type Services struct {
	Source PS
	Emit   Emitter
	Poll   time.Duration
}

type serviceSeen struct {
	state, health string
	exit          int
	rev           int
}

func (s *Services) Name() string { return ServicesCollector }

func tone(state, health string, exit int) contract.Tone {
	switch {
	case state == "running" && (health == "" || health == "healthy"):
		return contract.ToneOK
	case state == "running":
		return contract.ToneWarn
	case state == "absent", state == "exited" && exit == 0:
		return contract.ToneIdle
	}
	return contract.ToneBad
}

func exitOf(st dockerport.ServiceState) int {
	if st.ExitCode == nil {
		return -1
	}
	return *st.ExitCode
}

func (s *Services) Run(ctx context.Context) error {
	poll := s.Poll
	if poll <= 0 {
		poll = DefaultPoll
	}
	seen := map[string]*serviceSeen{}
	for {
		list, err := s.Source.PS(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		current := map[string]bool{}
		for _, st := range list {
			current[st.Service] = true
			s.note(seen, st.Service, st.State, st.Health, exitOf(st))
		}
		for name, prev := range seen {
			if !current[name] && prev.state != "absent" {
				s.note(seen, name, "absent", "", -1)
			}
		}
		t := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil
		case <-t.C:
		}
	}
}

func (s *Services) note(seen map[string]*serviceSeen, name, state, health string, exit int) {
	prev := seen[name]
	if prev != nil && prev.state == state && prev.health == health && prev.exit == exit {
		return
	}
	next := &serviceSeen{state: state, health: health, exit: exit, rev: 1}
	if prev != nil {
		next.rev = prev.rev + 1
	}
	seen[name] = next
	attrs := contract.Attrs{"table": ServicesCollector, "label": name, "status": state, "tone": string(tone(state, health, exit))}
	if health != "" {
		attrs["health"] = health
	}
	if exit >= 0 {
		attrs["exitCode"] = exit
	}
	_, _ = s.Emit.Append(contract.Signal{
		Kind: contract.SignalKindEvent, Collector: ServicesCollector, TSSource: contract.TimeSourceHost,
		ID: "service-" + name, Rev: next.rev, Attrs: attrs, Event: &contract.EventBody{Body: name + " is " + state},
	})
}
