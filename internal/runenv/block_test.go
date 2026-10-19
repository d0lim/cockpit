package runenv

import (
	"net"
	"testing"

	"github.com/d0lim/cockpit/internal/hub/config"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var blockMutations = []porttest.Mutation{
	{"a block overlapping a reserved range is never used", "drop the ReservedPorts loop in blockUsable", "red: the slot over the reserved range is chosen", "TestFreeBlockSkipsReservedRangesAndBusyPortsAndFailsWhenNoSlotIsLeft"},
	{"a block with a busy port is never used", "drop the free check in blockUsable", "red: the slot with a listening port is chosen", "TestFreeBlockSkipsReservedRangesAndBusyPortsAndFailsWhenNoSlotIsLeft"},
}

func TestBlockMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "block_test.go", blockMutations)
}

func TestFreeBlockSkipsReservedRangesAndBusyPortsAndFailsWhenNoSlotIsLeft(t *testing.T) {
	cfg := config.Config{Limits: config.Limits{PortBase: 22000, Slots: 4}}
	allFree := func(int) bool { return true }
	if base, err := FreeBlock(cfg, allFree); err != nil || base != 22000 {
		t.Fatalf("free host: %d %v", base, err)
	}
	cfg.ReservedPorts = []config.PortRange{{Name: "old-hub", From: 22050, To: 22051}}
	if base, err := FreeBlock(cfg, allFree); err != nil || base != 22100 {
		t.Fatalf("reserved first slot: %d %v", base, err)
	}
	busy := func(p int) bool { return p != 22142 }
	if base, err := FreeBlock(cfg, busy); err != nil || base != 22200 {
		t.Fatalf("busy port in the second slot: %d %v", base, err)
	}
	none := func(p int) bool { return false }
	if _, err := FreeBlock(cfg, none); err == nil {
		t.Fatal("a host with no free port yielded a block")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if loopbackFree(port) {
		t.Fatalf("a listening port %d was reported free", port)
	}
}
