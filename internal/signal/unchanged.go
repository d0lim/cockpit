package signal

import (
	"crypto/sha256"
	"encoding/json"

	"github.com/d0lim/cockpit/internal/contract"
)

type contentSum [16]byte

func sumOf(sig contract.Signal) contentSum {
	sig.TS, sig.TSSource, sig.Seq, sig.Rev = "", "", 0, 0
	data, _ := json.Marshal(sig)
	full := sha256.Sum256(data)
	var sum contentSum
	copy(sum[:], full[:])
	return sum
}

func (s *Stream) unchangedWinner(sig contract.Signal) (int64, bool) {
	if sig.ID == "" || sig.Kind == contract.SignalKindMetric {
		return 0, false
	}
	w, ok := s.revs[sigKey(sig)]
	if !ok || sig.Rev > w.rev || w.sum != sumOf(sig) || !s.ix.alive(w.seq) {
		return 0, false
	}
	return w.seq, true
}
