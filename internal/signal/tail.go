package signal

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
)

const defaultReadLimit = 1000

type ReadRequest struct {
	After int64
	Limit int
}

type Resync struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

type ReadResponse struct {
	Signals   []contract.Signal `json:"signals"`
	Frozen    bool              `json:"frozen"`
	FrozenSeq int64             `json:"frozenSeq"`
	Dropped   map[string]int64  `json:"dropped"`
	Resync    *Resync           `json:"resync,omitempty"`
	Next      int64             `json:"next"`
}

func (s *Stream) lineAt(seq int64) ([]byte, bool, error) {
	off, ok := s.ix.offset(seq)
	if !ok {
		return nil, false, nil
	}
	r := bufio.NewReader(io.NewSectionReader(s.file, off, s.size-off))
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, false, fmt.Errorf("cannot read signal %d: %w", seq, err)
	}
	return bytes.TrimSuffix(line, []byte{'\n'}), true, nil
}

func remask(m *mask.Masker, raw []byte) ([]byte, error) {
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, err
	}
	return json.Marshal(m.Leaves(tree))
}

func (s *Stream) superseded(sig contract.Signal) bool {
	if sig.ID == "" || sig.Kind == contract.SignalKindMetric {
		return false
	}
	r, ok := s.revs[revKey(sig.Collector, string(sig.Kind), sig.ID)]
	return ok && r.seq != sig.Seq
}

func (s *Stream) Read(req ReadRequest) (ReadResponse, error) {
	m, err := s.masker()
	if err != nil {
		return ReadResponse{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.ReadRefused != "" {
		return ReadResponse{}, fmt.Errorf("%w: %s", ErrReadRefused, s.state.ReadRefused)
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultReadLimit
	}
	resp := ReadResponse{Signals: []contract.Signal{}, Frozen: s.state.Frozen, FrozenSeq: s.state.FrozenSeq, Dropped: s.droppedLocked(), Next: req.After}
	seq := max(req.After+1, s.ix.base)
	gapEnd := int64(0)
	for ; seq <= s.ix.last() && !s.ix.alive(seq); seq++ {
		if !s.ix.superseded(seq) {
			gapEnd = seq
		}
	}
	if gapEnd > 0 && gapEnd <= s.ix.last() {
		resp.Resync = &Resync{From: max(req.After+1, s.ix.base), To: gapEnd}
	}
	resp.Next = max(resp.Next, seq-1)
	for ; seq <= s.ix.last() && len(resp.Signals) < limit; seq++ {
		raw, ok, err := s.lineAt(seq)
		if err != nil {
			return ReadResponse{}, err
		}
		resp.Next = seq
		if !ok {
			continue
		}
		var stored contract.Signal
		if err := json.Unmarshal(raw, &stored); err != nil {
			return ReadResponse{}, err
		}
		if s.superseded(stored) {
			continue
		}
		masked, err := remask(m, raw)
		if err != nil {
			return ReadResponse{}, err
		}
		var sig contract.Signal
		if err := json.Unmarshal(masked, &sig); err != nil {
			return ReadResponse{}, err
		}
		resp.Signals = append(resp.Signals, sig)
	}
	return resp, nil
}

func instant(ts string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, ts)
	return t, err == nil
}

func ByTime(a, b contract.Signal) int {
	ta, okA := instant(a.TS)
	tb, okB := instant(b.TS)
	order := cmp.Compare(a.TS, b.TS)
	if okA && okB {
		order = ta.Compare(tb)
	}
	return cmp.Or(order, cmp.Compare(a.Seq, b.Seq))
}
