package signal

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/d0lim/cockpit/internal/contract"
)

const (
	segmentFile   = "segment.json"
	segmentSchema = 1
)

var ErrAlreadyFrozen = errors.New("the signal stream is already frozen")

type segmentState struct {
	SchemaVersion int              `json:"schemaVersion"`
	Frozen        bool             `json:"frozen"`
	FrozenSeq     int64            `json:"frozenSeq"`
	NextSeq       int64            `json:"nextSeq"`
	DroppedBase   map[string]int64 `json:"droppedBase,omitempty"`
	ReadRefused   string           `json:"readRefused,omitempty"`
}

func writeRootFile(root *os.Root, name string, data []byte) error {
	tmp := name + ".tmp"
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return root.Rename(tmp, name)
}

func (s *Stream) saveState() error {
	s.state.SchemaVersion = segmentSchema
	s.state.NextSeq = s.nextSeq
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return writeRootFile(s.root, segmentFile, append(data, '\n'))
}

func (s *Stream) load() error {
	data, err := s.root.ReadFile(segmentFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(data, &s.state); err != nil {
			return fmt.Errorf("cannot read %s: %w", segmentFile, err)
		}
	}
	s.file, err = s.root.OpenFile(streamFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	s.ix = index{base: 1}
	s.lim = newLimiter(s.opts.Caps)
	s.revs = revisions{}
	last, err := s.replay()
	if err != nil {
		return err
	}
	s.nextSeq = max(s.state.NextSeq, last+1, 1)
	if s.state.Frozen && !s.lim.frozen {
		s.lim.freeze(s.state.FrozenSeq + 1)
	}
	return nil
}

func (s *Stream) replay() (int64, error) {
	info, err := s.file.Stat()
	if err != nil {
		return 0, err
	}
	r := bufio.NewReaderSize(io.NewSectionReader(s.file, 0, info.Size()), 1<<16)
	var offset, last int64
	for {
		line, err := r.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			if len(line) > 0 {
				if err := s.file.Truncate(offset); err != nil {
					return 0, err
				}
			}
			break
		}
		if err != nil {
			return 0, err
		}
		if from, to, ok := parseTombstone(line); ok && from > last && to >= from {
			for seq := from; seq <= to; seq++ {
				s.ix.markSuperseded(seq)
			}
			offset += int64(len(line))
			last = to
			continue
		}
		var sig contract.Signal
		if json.Unmarshal(line, &sig) != nil || sig.Seq <= last {
			if _, peek := r.Peek(1); peek == io.EOF {
				if err := s.file.Truncate(offset); err != nil {
					return 0, err
				}
				break
			}
			return 0, fmt.Errorf("signal stream %s is corrupt at offset %d", filepath.Join(s.dir, streamFile), offset)
		}
		if s.state.Frozen && sig.Seq > s.state.FrozenSeq && !s.lim.frozen {
			s.lim.freeze(s.state.FrozenSeq + 1)
		}
		s.record(sig, offset)
		offset += int64(len(line))
		last = sig.Seq
	}
	s.size = offset
	return last, nil
}

type FreezeResult struct {
	FrozenSeq int64
	Records   int
	Dropped   map[string]int64
}

func (s *Stream) Freeze(bundleDir string) (FreezeResult, error) {
	m, err := s.masker()
	if err != nil {
		return FreezeResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Frozen {
		return FreezeResult{}, ErrAlreadyFrozen
	}
	dest := filepath.Join(bundleDir, Dir)
	if err := os.MkdirAll(dest, 0o700); err != nil {
		return FreezeResult{}, err
	}
	root, err := os.OpenRoot(dest)
	if err != nil {
		return FreezeResult{}, err
	}
	defer root.Close()
	var out []byte
	n := 0
	for seq := s.ix.base; seq <= s.ix.last(); seq++ {
		raw, ok, err := s.lineAt(seq)
		if err != nil {
			return FreezeResult{}, err
		}
		if !ok {
			continue
		}
		masked, err := remask(m, raw)
		if err != nil {
			return FreezeResult{}, err
		}
		out = append(append(out, masked...), '\n')
		n++
	}
	if err := writeRootFile(root, streamFile, out); err != nil {
		return FreezeResult{}, err
	}
	s.state.Frozen, s.state.FrozenSeq = true, s.nextSeq-1
	if err := s.saveState(); err != nil {
		return FreezeResult{}, err
	}
	s.lim.freeze(s.state.FrozenSeq + 1)
	return FreezeResult{FrozenSeq: s.state.FrozenSeq, Records: n, Dropped: s.droppedLocked()}, nil
}

func (s *Stream) RefuseReads(reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if reason == "" {
		reason = "refused"
	}
	s.state.ReadRefused = reason
	return s.saveState()
}

func (s *Stream) maybeCompact() error {
	if s.ix.killed < max(s.opts.Caps.CompactMin, 1) || s.ix.killed <= s.ix.live {
		return nil
	}
	var out []byte
	next := make([]int64, len(s.ix.entries))
	run := int64(0)
	flush := func(end int64) {
		if run > 0 {
			out = append(out, tombstone(run, end)...)
			run = 0
		}
	}
	for i, e := range s.ix.entries {
		seq := s.ix.base + int64(i)
		next[i] = noEntry
		if e == supersededEntry {
			next[i] = supersededEntry
			if run == 0 {
				run = seq
			}
			continue
		}
		flush(seq - 1)
		if e < 0 {
			continue
		}
		raw, _, err := s.lineAt(seq)
		if err != nil {
			return err
		}
		next[i] = int64(len(out)) | e&^offsetMask
		out = append(append(out, raw...), '\n')
	}
	flush(s.ix.last())
	if err := writeRootFile(s.root, streamFile, out); err != nil {
		return err
	}
	f, err := s.root.OpenFile(streamFile, os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	s.file.Close()
	s.file, s.size = f, int64(len(out))
	s.ix.entries, s.ix.killed = next, 0
	s.state.DroppedBase = s.droppedLocked()
	s.lim.dropped = map[string]int64{}
	return s.saveState()
}

const tombstonePrefix = `{"superseded":[`

func tombstone(from, to int64) []byte {
	return fmt.Appendf(nil, "%s%d,%d]}\n", tombstonePrefix, from, to)
}

func parseTombstone(line []byte) (int64, int64, bool) {
	if !bytes.HasPrefix(line, []byte(tombstonePrefix)) {
		return 0, 0, false
	}
	var t struct {
		Superseded [2]int64 `json:"superseded"`
	}
	if json.Unmarshal(line, &t) != nil {
		return 0, 0, false
	}
	return t.Superseded[0], t.Superseded[1], true
}
