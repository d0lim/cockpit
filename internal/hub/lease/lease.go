package lease

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/hub/store"
)

const (
	DefaultBase   = 22000
	DefaultStride = 100
	DefaultSlots  = 8
	HubPort       = 7710
	lowestPort    = 1024
	ephemeralFrom = 49152
)

type Layout struct {
	Base    int
	Stride  int
	Slots   int
	Offsets map[string]int
}

func DefaultLayout(offsets map[string]int) Layout {
	return Layout{Base: DefaultBase, Stride: DefaultStride, Slots: DefaultSlots, Offsets: offsets}
}

type Reserved struct {
	Name string
	From int
	To   int
}

var ManualStackPorts = []int{3306, 4040, 5173, 5174, 7700, 8080, 8081, 8082, 9090, 9091, 9092, 9093, 9094, 9095, 13307, 16379, 18090}

func ReservedPorts() []Reserved {
	out := []Reserved{
		{Name: "previous cockpit port block", From: 21000, To: 21800},
		{Name: "hub listen port", From: HubPort, To: HubPort + 1},
	}
	for _, p := range ManualStackPorts {
		out = append(out, Reserved{Name: "manual stack port", From: p, To: p + 1})
	}
	return out
}

var ErrNoPortBlock = errors.New("no port block")

type Probe func(port int) bool

func ProbeLoopback(port int) bool {
	l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

func (l Layout) Validate() error {
	if l.Stride <= 0 || l.Slots <= 0 {
		return fmt.Errorf("port block layout needs a positive stride and slot count, got stride %d and %d slots", l.Stride, l.Slots)
	}
	end := l.Base + l.Stride*l.Slots
	if l.Base < lowestPort || end > ephemeralFrom {
		return fmt.Errorf("port blocks %d-%d leave the range %d-%d", l.Base, end-1, lowestPort, ephemeralFrom-1)
	}
	for name, off := range l.Offsets {
		if off < 0 || off >= l.Stride {
			return fmt.Errorf("port offset %s=%d is outside the block width %d", name, off, l.Stride)
		}
	}
	var hits []string
	for _, r := range ReservedPorts() {
		if r.From < end && r.To > l.Base {
			hits = append(hits, fmt.Sprintf("%s %d-%d", r.Name, r.From, r.To-1))
		}
	}
	if len(hits) > 0 {
		return fmt.Errorf("port blocks %d-%d overlap reserved ports: %s", l.Base, end-1, strings.Join(hits, ", "))
	}
	return nil
}

func (l Layout) PortsForSlot(slot int) map[string]int {
	start := l.Base + l.Stride*slot
	out := make(map[string]int, len(l.Offsets))
	for name, off := range l.Offsets {
		out[name] = start + off
	}
	return out
}

type Blocked struct {
	Name string
	Port int
}

func BlockedPorts(ports map[string]int, probe Probe) []Blocked {
	var out []Blocked
	for name, port := range ports {
		if !probe(port) {
			out = append(out, Blocked{Name: name, Port: port})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out
}

func Lease(leases store.Leases, runID string, probe Probe, l Layout) (store.Leases, int, map[string]int, error) {
	if err := l.Validate(); err != nil {
		return nil, 0, nil, err
	}
	leased := map[int]bool{}
	for _, s := range leases {
		leased[s] = true
	}
	blocked := 0
	for slot := range l.Slots {
		if leased[slot] {
			continue
		}
		ports := l.PortsForSlot(slot)
		if len(BlockedPorts(ports, probe)) == 0 {
			next := make(store.Leases, len(leases)+1)
			for k, v := range leases {
				next[k] = v
			}
			next[runID] = slot
			return next, slot, ports, nil
		}
		blocked++
	}
	return nil, 0, nil, fmt.Errorf("%w: %d slots, %d leased, %d blocked", ErrNoPortBlock, l.Slots, len(leased), blocked)
}

func Release(leases store.Leases, runID string) store.Leases {
	next := make(store.Leases, len(leases))
	for k, v := range leases {
		if k != runID {
			next[k] = v
		}
	}
	return next
}
