package seal

import (
	"errors"
	"net"
	"strconv"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var canaryMutations = []porttest.Mutation{
	{"on linux the canary listens on the docker0 bridge", "return 127.0.0.1 for every operating system in CanaryHost", "red: linux with a docker0 address gets 127.0.0.1", "TestTheCanaryHostIsLoopbackOffLinuxAndTheDocker0AddressOnLinux"},
	{"the canary never listens on a wildcard or guessed address", "return an empty host when docker0 has no usable IPv4 address", "red: linux without docker0 or with only IPv6 yields a host instead of an error", "TestALinuxHubWithoutAUsableDocker0AddressCannotOpenTheCanary"},
	{"the canary counts real connections", "never increment the accepted counter", "red: Accepted stays 0 after two dials", "TestCanaryCountsConnectionsOnTheAddressItWasGiven"},
}

func TestCanaryMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "canary_test.go", canaryMutations)
}

func cidr(t *testing.T, s string) net.Addr {
	t.Helper()
	ip, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	n.IP = ip
	return n
}

func TestTheCanaryHostIsLoopbackOffLinuxAndTheDocker0AddressOnLinux(t *testing.T) {
	asked := ""
	addrs := func(iface string) ([]net.Addr, error) {
		asked = iface
		return []net.Addr{cidr(t, "fe80::1/64"), cidr(t, "172.17.0.1/16")}, nil
	}
	for _, goos := range []string{"darwin", "windows"} {
		host, err := CanaryHost(goos, addrs)
		if err != nil || host != "127.0.0.1" {
			t.Errorf("%s: host %q err %v", goos, host, err)
		}
	}
	if asked != "" {
		t.Fatalf("a non-linux hub read interface %q", asked)
	}
	host, err := CanaryHost("linux", addrs)
	if err != nil || host != "172.17.0.1" || asked != "docker0" {
		t.Fatalf("linux: host %q err %v interface %q", host, err, asked)
	}
}

func TestALinuxHubWithoutAUsableDocker0AddressCannotOpenTheCanary(t *testing.T) {
	cases := map[string]func(string) ([]net.Addr, error){
		"no docker0":       func(string) ([]net.Addr, error) { return nil, errors.New("route ip+net: no such network interface") },
		"only IPv6":        func(string) ([]net.Addr, error) { return []net.Addr{cidr(t, "fe80::1/64")}, nil },
		"no address":       func(string) ([]net.Addr, error) { return nil, nil },
		"unspecified IPv4": func(string) ([]net.Addr, error) { return []net.Addr{cidr(t, "0.0.0.0/0")}, nil },
	}
	evaluated := 0
	for name, addrs := range cases {
		host, err := CanaryHost("linux", addrs)
		if err == nil || host != "" {
			t.Errorf("%s: host %q err %v, want an error and no host", name, host, err)
		}
		evaluated++
	}
	if evaluated != 4 {
		t.Fatalf("evaluated %d cases", evaluated)
	}
	if host, err := CanaryHost("linux", func(string) ([]net.Addr, error) { return []net.Addr{cidr(t, "172.17.0.1/16")}, nil }); err != nil || host != "172.17.0.1" {
		t.Fatalf("canary: a usable docker0 address gave host %q err %v", host, err)
	}
}

func TestCanaryCountsConnectionsOnTheAddressItWasGiven(t *testing.T) {
	c, err := ListenCanaryAt("127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Accepted() != 0 {
		t.Fatal("a fresh canary counted a connection")
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(c.Port()))
	for range 2 {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 1)
		conn.Read(buf)
		conn.Close()
	}
	if c.Accepted() != 2 {
		t.Fatalf("accepted %d", c.Accepted())
	}
	if host, _, _ := net.SplitHostPort(c.ln.Addr().String()); host != "127.0.0.1" {
		t.Fatalf("canary listens on %s", host)
	}
}
