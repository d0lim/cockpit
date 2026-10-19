package signal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/mask"
)

const (
	Dir           = "signals"
	streamFile    = "stream.jsonl"
	RunnerSource  = "runner"
	LogCollector  = "logs"
	ContainerAttr = "container"
)

var (
	ErrReadRefused = errors.New("signal reads are refused for this run")
	ErrNoMasker    = errors.New("no masker for this run")
	collectorName  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

type Options struct {
	Caps   Caps
	Masker func() *mask.Masker
	Now    func() time.Time
}

type Stream struct {
	mu      sync.Mutex
	dir     string
	root    *os.Root
	file    *os.File
	size    int64
	opts    Options
	ix      index
	lim     *limiter
	revs    revisions
	state   segmentState
	nextSeq int64
}

func ValidCollector(id string) bool { return collectorName.MatchString(id) }

func Open(runDir string, opts Options) (*Stream, error) {
	if opts.Masker == nil {
		return nil, ErrNoMasker
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Caps == (Caps{}) {
		opts.Caps = DefaultCaps()
	}
	dir := filepath.Join(runDir, Dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	s := &Stream{dir: dir, root: root, opts: opts}
	if err := s.load(); err != nil {
		root.Close()
		return nil, err
	}
	return s, nil
}

func OpenRun(st *store.Store, runID string, opts Options) (*Stream, error) {
	dir, err := st.RunDir(runID)
	if err != nil {
		return nil, err
	}
	return Open(dir, opts)
}

func RemoveRun(st *store.Store, runID string) error {
	dir, err := st.RunDir(runID)
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(dir, Dir))
}

func (s *Stream) masker() (*mask.Masker, error) {
	m := s.opts.Masker()
	if m == nil {
		return nil, ErrNoMasker
	}
	return m, nil
}

func maskSignal(m *mask.Masker, sig contract.Signal) ([]byte, error) {
	raw, err := json.Marshal(sig)
	if err != nil {
		return nil, err
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, err
	}
	return json.Marshal(m.Leaves(tree))
}

func validate(sig contract.Signal) error {
	if !ValidCollector(sig.Collector) {
		return fmt.Errorf("collector id %q does not match %s", sig.Collector, collectorName)
	}
	bodies := map[contract.SignalKind]bool{contract.SignalKindMetric: sig.Metric != nil, contract.SignalKindEvent: sig.Event != nil, contract.SignalKindSpan: sig.Span != nil}
	present, known := bodies[sig.Kind]
	n := 0
	for _, p := range bodies {
		if p {
			n++
		}
	}
	if !known || !present || n != 1 {
		return fmt.Errorf("signal kind %q needs exactly its own body", sig.Kind)
	}
	switch sig.TSSource {
	case contract.TimeSourceHost, contract.TimeSourceContainer, contract.TimeSourceRunner:
	default:
		return fmt.Errorf("unknown time source %q", sig.TSSource)
	}
	if sig.Span != nil {
		switch sig.Span.Status {
		case contract.SpanStatusOK, contract.SpanStatusError, contract.SpanStatusIncomplete:
		default:
			return fmt.Errorf("unknown span status %q", sig.Span.Status)
		}
	}
	return nil
}

func (s *Stream) Append(sig contract.Signal) (int64, error) {
	m, err := s.masker()
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if sig.TS == "" {
		sig.TS = s.opts.Now().UTC().Format(time.RFC3339Nano)
	}
	if sig.TSSource == "" {
		sig.TSSource = contract.TimeSourceHost
	}
	if sig.Attrs == nil {
		sig.Attrs = contract.Attrs{}
	}
	if err := validate(sig); err != nil {
		return 0, err
	}
	sig.Seq = s.nextSeq
	line, err := maskSignal(m, sig)
	if err != nil {
		return 0, err
	}
	var stored contract.Signal
	if err := json.Unmarshal(line, &stored); err != nil {
		return 0, err
	}
	line = append(line, '\n')
	if _, err := s.file.Write(line); err != nil {
		return 0, err
	}
	s.record(stored, s.size)
	s.size += int64(len(line))
	s.nextSeq++
	return sig.Seq, s.maybeCompact()
}

func (s *Stream) record(sig contract.Signal, offset int64) {
	container, _ := sig.Attrs[ContainerAttr].(string)
	b := s.lim.bucketFor(sig.Collector, container, sig.Seq)
	s.ix.put(sig.Seq, offset, b)
	if sig.ID != "" && sig.Kind != contract.SignalKindMetric {
		key := revKey(sig.Collector, string(sig.Kind), sig.ID)
		prev, had := s.revs[key]
		s.revs.note(key, sig.Seq, sig.Rev)
		if had && s.revs[key].seq != sig.Seq {
			s.ix.supersede(sig.Seq)
			return
		}
		if had {
			s.lim.release(&s.ix, prev.seq)
		}
	}
	s.lim.admit(&s.ix, sig.Seq, b)
}

func (s *Stream) Revisions(collector string, kind contract.SignalKind, id string) (latest int64, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.revs[revKey(collector, string(kind), id)]
	return r.seq, r.n
}

func (s *Stream) Dropped() map[string]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.droppedLocked()
}

func (s *Stream) droppedLocked() map[string]int64 {
	out := map[string]int64{}
	for k, v := range s.state.DroppedBase {
		out[k] += v
	}
	for k, v := range s.lim.dropped {
		out[k] += v
	}
	return out
}

func (s *Stream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var err error
	if s.file != nil {
		err = s.file.Close()
		s.file = nil
	}
	return errors.Join(err, s.root.Close())
}

func (s *Stream) Destroy() error {
	err := s.Close()
	return errors.Join(err, os.RemoveAll(s.dir))
}
