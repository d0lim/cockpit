package lease

import (
	"errors"
	"maps"
	"net"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/store"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var leaseMutations = []porttest.Mutation{
	{"port blocks do not overlap the reserved ranges the caller passes", "skip the Reserved entries in Validate", "red: a layout over a passed range leases", "TestLayoutsOverlappingReservedRangesPassedByTheCallerAreRefusedBeforeLeasing"},
	{"the hub listen port is reserved", "drop the HubPort entry from reserved", "red: a block holding 7710 or the configured hub port leases", "TestBlocksHoldingTheHubListenPortAreRefusedAndTheConfiguredPortIsUsed"},
	{"the default layout validates without extra reserved ranges", "move DefaultBase to 7650", "red: the default layout fails validation", "TestDefaultLayoutLeasesAllEightSlotsInOrder"},
	{"offsets stay inside the block width", "drop the offset range check", "red: offset 100 with stride 100 is accepted", "TestLayoutsWithBadStrideSlotsOffsetsRangeOrReservedEntriesAreRefused"},
	{"occupied ports skip the block", "ignore the probe in Lease", "red: slot 0 is leased while one of its ports is taken", "TestOccupiedPortInSlotZeroMovesTheLeaseToSlotOne"},
	{"leased blocks are not handed out again", "ignore existing leases", "red: the stopped run's slot 0 goes to the new run", "TestLeasedBlockIsNotHandedOutEvenWhenItsPortsAreFree"},
	{"no free block is an error", "return slot 0 when nothing is free", "red: a ninth run gets a slot", "TestNoFreeBlockIsErrNoPortBlock"},
	{"the probe binds the loopback address", "bind :port instead of 127.0.0.1:port", "red: a port held on 127.0.0.1 probes as free", "TestProbeLoopbackSeesAPortHeldOnLoopbackAndAReleasedPortAsFree"},
	{"blocks start at the base and step by the stride", "drop the stride multiplication in PortsForSlot", "red: slot 7 lands on the 22000 block", "TestPortsForSlotPlacesOffsetsInBlocksOfOneHundredFrom22000"},
	{"release frees only the named run", "Release returns its input", "red: the released slot 0 is not handed out again", "TestReleasedBlockIsHandedOutAgainAndInputsAreNotModified"},
	{"taken ports are reported by name", "BlockedPorts returns nil", "red: the two taken ports are missing", "TestBlockedPortsNamesEachTakenPort"},
}

var offsets = map[string]int{
	"APP": 0, "API": 1, "APP_DB": 6, "API_DB": 7, "CONSOLE": 73, "DEMO": 74, "CACHE": 79,
	"NOTIFY": 82, "MOCK_A": 90, "MOCK_B": 91, "MOCK_C": 92, "MOCK_D": 93, "MOCK_E": 94,
}

func occupied(ports ...int) Probe {
	return func(p int) bool { return !slices.Contains(ports, p) }
}

var allFree = occupied()

func TestPortsForSlotPlacesOffsetsInBlocksOfOneHundredFrom22000(t *testing.T) {
	l := DefaultLayout(offsets)
	zero, last := l.PortsForSlot(0), l.PortsForSlot(7)
	for name, off := range offsets {
		if zero[name] != 22000+off || last[name] != 22700+off {
			t.Fatalf("%s: %d %d", name, zero[name], last[name])
		}
	}
	if len(zero) != 13 {
		t.Fatalf("ports %d", len(zero))
	}
}

func TestDefaultLayoutLeasesAllEightSlotsInOrder(t *testing.T) {
	l := DefaultLayout(offsets)
	if err := l.Validate(); err != nil {
		t.Fatal(err)
	}
	leases := store.Leases{}
	var slots []int
	for _, id := range []string{"r0", "r1", "r2", "r3", "r4", "r5", "r6", "r7"} {
		next, slot, _, err := Lease(leases, id, allFree, l)
		if err != nil {
			t.Fatal(err)
		}
		leases = next
		slots = append(slots, slot)
	}
	if !slices.Equal(slots, []int{0, 1, 2, 3, 4, 5, 6, 7}) {
		t.Fatalf("%v", slots)
	}
}

func TestOccupiedPortInSlotZeroMovesTheLeaseToSlotOne(t *testing.T) {
	l := DefaultLayout(offsets)
	ran := 0
	for _, off := range offsets {
		ran++
		_, slot, ports, err := Lease(store.Leases{}, "run-a", occupied(22000+off), l)
		if err != nil || slot != 1 || ports["APP"] != 22100 {
			t.Fatalf("offset %d: slot %d %v", off, slot, err)
		}
	}
	if ran != 13 {
		t.Fatalf("ran %d", ran)
	}
	if _, slot, _, _ := Lease(store.Leases{}, "run-a", allFree, l); slot != 0 {
		t.Fatalf("canary: free slot 0 not leased, got %d", slot)
	}
}

func TestLeasedBlockIsNotHandedOutEvenWhenItsPortsAreFree(t *testing.T) {
	l := DefaultLayout(offsets)
	next, slot, _, err := Lease(store.Leases{"run-stopped": 0}, "run-new", allFree, l)
	if err != nil || slot != 1 || !reflect.DeepEqual(next, store.Leases{"run-stopped": 0, "run-new": 1}) {
		t.Fatalf("%v %d %v", next, slot, err)
	}
}

func TestReleasedBlockIsHandedOutAgainAndInputsAreNotModified(t *testing.T) {
	before := store.Leases{"run-old": 0, "run-kept": 1}
	snapshot := maps.Clone(before)
	released := Release(before, "run-old")
	if !reflect.DeepEqual(released, store.Leases{"run-kept": 1}) {
		t.Fatalf("%v", released)
	}
	if _, slot, _, _ := Lease(released, "run-new", allFree, DefaultLayout(offsets)); slot != 0 {
		t.Fatalf("slot %d", slot)
	}
	Lease(before, "run-b", allFree, DefaultLayout(offsets))
	if !reflect.DeepEqual(before, snapshot) {
		t.Fatalf("input changed: %v", before)
	}
}

func TestNoFreeBlockIsErrNoPortBlock(t *testing.T) {
	seven := store.Leases{"run-0": 0, "run-1": 1, "run-2": 2, "run-3": 3, "run-4": 4, "run-5": 5, "run-6": 6}
	eight := maps.Clone(seven)
	eight["run-7"] = 7
	cases := []struct {
		leases store.Leases
		probe  Probe
	}{{eight, allFree}, {seven, occupied(22790)}}
	for _, c := range cases {
		if _, _, _, err := Lease(c.leases, "run-new", c.probe, DefaultLayout(offsets)); !errors.Is(err, ErrNoPortBlock) {
			t.Fatalf("err %v", err)
		}
	}
	if _, slot, _, err := Lease(seven, "run-new", allFree, DefaultLayout(offsets)); err != nil || slot != 7 {
		t.Fatalf("canary: %d %v", slot, err)
	}
}

var synthetic = []Reserved{
	{Name: "toy api", From: 9190, To: 9191},
	{Name: "acme single port", From: 9580, To: 9581},
	{Name: "toy db", From: 5432, To: 5433},
	{Name: "app block", From: 30000, To: 30800},
}

func TestLayoutsOverlappingReservedRangesPassedByTheCallerAreRefusedBeforeLeasing(t *testing.T) {
	cases := []struct {
		name string
		base int
		hit  string
	}{
		{"an offset lands on a reserved port", 9100, "toy api 9190-9190"},
		{"the whole block is checked, offset 80 holds no service", 9500, "acme single port 9580-9580"},
		{"every slot is checked, only slot 6 holds the port", 4800, "toy db 5432-5432"},
		{"a range overlapping the blocks", 30500, "app block 30000-30799"},
	}
	for _, c := range cases {
		l := DefaultLayout(offsets)
		l.Base, l.Reserved = c.base, synthetic
		_, _, _, err := Lease(store.Leases{}, "run-a", allFree, l)
		if err == nil || !strings.Contains(err.Error(), "overlap reserved ports") || !strings.Contains(err.Error(), c.hit) {
			t.Errorf("%s: err %v", c.name, err)
		}
		l.Reserved = nil
		if err := l.Validate(); err != nil {
			t.Errorf("canary %s: the same layout without the passed ranges is refused: %v", c.name, err)
		}
	}
	if len(cases) != 4 {
		t.Fatal("case count")
	}
	clear := DefaultLayout(offsets)
	clear.Reserved = synthetic
	if err := clear.Validate(); err != nil {
		t.Fatalf("default layout with ranges outside its blocks: %v", err)
	}
}

func TestBlocksHoldingTheHubListenPortAreRefusedAndTheConfiguredPortIsUsed(t *testing.T) {
	l := DefaultLayout(offsets)
	l.Base = 7650
	if err := l.Validate(); err == nil || !strings.Contains(err.Error(), "hub listen port 7710-7710") {
		t.Fatalf("default hub port: %v", err)
	}
	configured := DefaultLayout(offsets)
	configured.HubPort = 22350
	if err := configured.Validate(); err == nil || !strings.Contains(err.Error(), "hub listen port 22350-22350") {
		t.Fatalf("configured hub port: %v", err)
	}
	l.HubPort = 7000
	if err := l.Validate(); err != nil {
		t.Fatalf("canary: hub port moved away, still refused: %v", err)
	}
}

func TestLayoutsWithBadStrideSlotsOffsetsRangeOrReservedEntriesAreRefused(t *testing.T) {
	bad := []Layout{
		{Base: 22000, Stride: 0, Slots: 8},
		{Base: 22000, Stride: 100, Slots: 0},
		{Base: 22000, Stride: 100, Slots: 8, Offsets: map[string]int{"APP": 100}},
		{Base: 22000, Stride: 100, Slots: 8, Offsets: map[string]int{"APP": -1}},
		{Base: 500, Stride: 100, Slots: 2},
		{Base: 49100, Stride: 100, Slots: 2},
		{Base: 22000, Stride: 100, Slots: 8, Reserved: []Reserved{{Name: "toy", From: 9000, To: 9000}}},
	}
	for _, l := range bad {
		if err := l.Validate(); err == nil {
			t.Errorf("%+v accepted", l)
		}
	}
	if err := (Layout{Base: 22000, Stride: 100, Slots: 8, Offsets: map[string]int{"APP": 99}}).Validate(); err != nil {
		t.Fatalf("canary: %v", err)
	}
}

func TestBlockedPortsNamesEachTakenPort(t *testing.T) {
	ports := DefaultLayout(offsets).PortsForSlot(2)
	want := []Blocked{{Name: "APP_DB", Port: 22206}, {Name: "MOCK_C", Port: 22292}}
	if got := BlockedPorts(ports, occupied(22206, 22292)); !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
	if got := BlockedPorts(ports, allFree); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestProbeLoopbackSeesAPortHeldOnLoopbackAndAReleasedPortAsFree(t *testing.T) {
	holder, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := holder.Addr().(*net.TCPAddr).Port
	if ProbeLoopback(port) {
		holder.Close()
		t.Fatal("held port probed as free")
	}
	holder.Close()
	if !ProbeLoopback(port) || !ProbeLoopback(port) {
		t.Fatal("released port probed as taken")
	}
}

func TestLeaseMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "lease_test.go", leaseMutations)
}
