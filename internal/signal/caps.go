package signal

const (
	DefaultTail         = 50_000
	DefaultPerCollector = 20_000
	DefaultPerContainer = 10_000
	defaultCompactMin   = 4096
	maxBuckets          = 1<<15 - 1
)

const (
	ReasonTailCap        = "tail-cap"
	ReasonCollectorQuota = "collector-quota"
	ReasonContainerQuota = "container-quota"
)

type Caps struct {
	Tail         int
	PerCollector int
	PerContainer int
	CompactMin   int
}

func DefaultCaps() Caps {
	return Caps{Tail: DefaultTail, PerCollector: DefaultPerCollector, PerContainer: DefaultPerContainer, CompactMin: defaultCompactMin}
}

type bucket struct {
	quota  int
	log    bool
	live   int
	cursor int64
}

type limiter struct {
	caps       Caps
	byName     map[string]uint16
	buckets    []*bucket
	tailCursor int64
	tailN      int
	frozen     bool
	dropped    map[string]int64
}

func newLimiter(c Caps) *limiter {
	return &limiter{caps: c, byName: map[string]uint16{}, dropped: map[string]int64{}}
}

func (l *limiter) bucketFor(collector, container string, from int64) uint16 {
	name, quota, log := "c:"+collector, l.caps.PerCollector, false
	if collector == LogCollector {
		name, quota, log = "l:"+container, l.caps.PerContainer, true
	}
	if i, ok := l.byName[name]; ok {
		return i
	}
	if len(l.buckets) >= maxBuckets {
		return maxBuckets - 1
	}
	i := uint16(len(l.buckets))
	l.byName[name] = i
	l.buckets = append(l.buckets, &bucket{quota: quota, log: log, cursor: from})
	return i
}

func (l *limiter) freeze(from int64) {
	l.frozen = true
	l.tailCursor, l.tailN = from, 0
	for _, b := range l.buckets {
		b.live, b.cursor = 0, from
	}
}

func (l *limiter) oldest(ix *index, cursor *int64, b uint16, any bool) (int64, bool) {
	for ; *cursor <= ix.last(); *cursor++ {
		if got, ok := ix.bucketOf(*cursor); ok && (any || got == b) {
			return *cursor, true
		}
	}
	return 0, false
}

func (l *limiter) admit(ix *index, seq int64, b uint16) {
	bk := l.buckets[b]
	bk.live++
	if l.frozen {
		l.tailN++
	}
	reason := ReasonCollectorQuota
	if bk.log {
		reason = ReasonContainerQuota
	}
	for bk.quota > 0 && bk.live > bk.quota {
		s, ok := l.oldest(ix, &bk.cursor, b, false)
		if !ok {
			break
		}
		l.drop(ix, s, reason)
	}
	for l.frozen && l.caps.Tail > 0 && l.tailN > l.caps.Tail {
		s, ok := l.oldest(ix, &l.tailCursor, 0, true)
		if !ok {
			break
		}
		l.drop(ix, s, ReasonTailCap)
	}
}

func (l *limiter) drop(ix *index, seq int64, reason string) {
	b, ok := ix.bucketOf(seq)
	if !ok {
		return
	}
	ix.kill(seq)
	l.buckets[b].live--
	if l.frozen {
		l.tailN--
	}
	l.dropped[reason]++
}
