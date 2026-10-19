package signal

import "github.com/d0lim/cockpit/internal/contract"

const (
	offsetBits      = 48
	offsetMask      = int64(1)<<offsetBits - 1
	deadEntry       = int64(-1)
	noEntry         = int64(-2)
	supersededEntry = int64(-3)
)

type index struct {
	base    int64
	entries []int64
	killed  int
	live    int
}

func (ix *index) slot(seq int64) (int, bool) {
	i := seq - ix.base
	if seq < ix.base || i >= int64(len(ix.entries)) {
		return 0, false
	}
	return int(i), true
}

func (ix *index) put(seq, offset int64, b uint16) {
	for ix.base+int64(len(ix.entries)) < seq {
		ix.entries = append(ix.entries, noEntry)
	}
	ix.entries = append(ix.entries, offset|int64(b)<<offsetBits)
	ix.live++
}

func (ix *index) alive(seq int64) bool {
	i, ok := ix.slot(seq)
	return ok && ix.entries[i] >= 0
}

func (ix *index) dropped(seq int64) bool {
	i, ok := ix.slot(seq)
	return ok && ix.entries[i] == deadEntry
}

func (ix *index) offset(seq int64) (int64, bool) {
	i, ok := ix.slot(seq)
	if !ok || ix.entries[i] < 0 {
		return 0, false
	}
	return ix.entries[i] & offsetMask, true
}

func (ix *index) bucketOf(seq int64) (uint16, bool) {
	i, ok := ix.slot(seq)
	if !ok || ix.entries[i] < 0 {
		return 0, false
	}
	return uint16(ix.entries[i] >> offsetBits), true
}

func (ix *index) kill(seq int64) {
	if i, ok := ix.slot(seq); ok && ix.entries[i] >= 0 {
		ix.entries[i] = deadEntry
		ix.killed++
		ix.live--
	}
}

func (ix *index) superseded(seq int64) bool {
	i, ok := ix.slot(seq)
	return ok && ix.entries[i] == supersededEntry
}

func (ix *index) supersede(seq int64) bool {
	i, ok := ix.slot(seq)
	if !ok || ix.entries[i] < 0 {
		return false
	}
	ix.entries[i] = supersededEntry
	ix.killed++
	ix.live--
	return true
}

func (ix *index) markSuperseded(seq int64) {
	for ix.base+int64(len(ix.entries)) <= seq {
		ix.entries = append(ix.entries, noEntry)
	}
	if i, ok := ix.slot(seq); ok {
		ix.entries[i] = supersededEntry
	}
}

func (ix *index) last() int64 { return ix.base + int64(len(ix.entries)) - 1 }

type revision struct {
	seq int64
	rev int
	n   int
}

type revisions map[string]revision

func revKey(collector, kind, id string) string { return collector + "\x00" + kind + "\x00" + id }

func sigKey(sig contract.Signal) string {
	key := revKey(sig.Collector, string(sig.Kind), sig.ID)
	if sig.Span != nil {
		key += "\x00" + sig.Span.TraceID
	}
	return key
}

func (r revisions) note(key string, seq int64, rev int) {
	cur, ok := r[key]
	if !ok {
		r[key] = revision{seq: seq, rev: rev, n: 1}
		return
	}
	cur.n++
	if rev >= cur.rev {
		cur.seq, cur.rev = seq, rev
	}
	r[key] = cur
}
