package recorder

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/d0lim/cockpit/internal/wire"
)

const pollInterval = 10 * time.Millisecond

var errRearmed = errors.New("the take was armed again while it was being written")

func (s *store) takeRecord(ctx context.Context, name string) (*record, error) {
	deadline := time.Now().Add(s.limits.MaxWait)
	for {
		rec, open, gen, err := s.tryFreeze(name, deadline)
		switch {
		case err != nil || rec != nil:
			return rec, err
		case open != nil:
			return s.seal(ctx, gen, *open)
		}
		if err := pause(ctx); err != nil {
			return nil, err
		}
	}
}

func (s *store) tryFreeze(name string, deadline time.Time) (*record, *[]*call, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.take != name {
		return nil, nil, 0, fmt.Errorf("take %q is not the current take %q", name, s.take)
	}
	if s.frozen != nil {
		return s.frozen, nil, s.gen, nil
	}
	if !s.recording {
		return nil, nil, s.gen, nil
	}
	now := time.Now()
	quiet := len(s.open) == 0 && (s.lastStart.IsZero() || now.Sub(s.lastStart) >= s.limits.Quiet)
	if !quiet && now.Before(deadline) {
		return nil, nil, s.gen, nil
	}
	s.recording = false
	open := make([]*call, 0, len(s.open))
	for _, c := range s.open {
		open = append(open, c)
	}
	s.open = map[int64]*call{}
	return nil, &open, s.gen, nil
}

func (s *store) seal(ctx context.Context, gen uint64, open []*call) (*record, error) {
	for s.waitingFor(gen) {
		if err := pause(ctx); err != nil {
			return nil, err
		}
	}
	for _, c := range open {
		s.add(ctx, gen, c.open(), c.addr)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if gen != s.gen {
		return nil, errRearmed
	}
	slices.SortFunc(s.lines, func(a, b line) int { return cmp.Compare(a.seq, b.seq) })
	texts := make([][]byte, len(s.lines))
	for i, l := range s.lines {
		texts[i] = l.text
	}
	s.frozen = &record{lines: texts, end: wire.End{Calls: len(texts), BodiesOmitted: s.omitted, CallsDropped: s.dropped, Since: s.started}}
	return s.frozen, nil
}

func (s *store) waitingFor(gen uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return gen == s.gen && s.queued > 0
}

func pause(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(pollInterval):
		return nil
	}
}
