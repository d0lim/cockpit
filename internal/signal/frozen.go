package signal

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
)

const maxFrozenLine = 4 << 20

var ErrNoFrozenCopy = errors.New("the report bundle has no frozen signal copy")

func ReadFrozen(bundleDir string, m *mask.Masker, req ReadRequest) (ReadResponse, error) {
	if m == nil {
		return ReadResponse{}, errors.New("no masker for the frozen copy")
	}
	root, err := os.OpenRoot(filepath.Join(bundleDir, Dir))
	if err != nil {
		return ReadResponse{}, ErrNoFrozenCopy
	}
	defer root.Close()
	f, err := root.Open(streamFile)
	if err != nil {
		return ReadResponse{}, ErrNoFrozenCopy
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		return ReadResponse{}, ErrNoFrozenCopy
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultReadLimit
	}
	resp := ReadResponse{Signals: []contract.Signal{}, Frozen: true, Dropped: map[string]int64{}, Next: req.After}
	r := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := readLine(r)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ReadResponse{}, err
		}
		if len(line) == 0 {
			continue
		}
		var stored contract.Signal
		if err := json.Unmarshal(line, &stored); err != nil {
			return ReadResponse{}, fmt.Errorf("the frozen copy has a broken line: %w", err)
		}
		resp.FrozenSeq = max(resp.FrozenSeq, stored.Seq)
		if stored.Seq <= req.After || len(resp.Signals) >= limit {
			continue
		}
		masked, err := remask(m, line)
		if err != nil {
			return ReadResponse{}, err
		}
		var sig contract.Signal
		if err := json.Unmarshal(masked, &sig); err != nil {
			return ReadResponse{}, err
		}
		resp.Signals = append(resp.Signals, sig)
		resp.Next = sig.Seq
	}
	if len(resp.Signals) < limit {
		resp.Next = max(resp.Next, resp.FrozenSeq)
	}
	return resp, nil
}

func readLine(r *bufio.Reader) ([]byte, error) {
	var out []byte
	for {
		chunk, more, err := r.ReadLine()
		if err != nil {
			return nil, err
		}
		out = append(out, chunk...)
		if len(out) > maxFrozenLine {
			return nil, errors.New("a frozen signal line exceeds the limit")
		}
		if !more {
			return out, nil
		}
	}
}
