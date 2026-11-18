package recorder

import (
	"context"
	"errors"
	"testing"
)

func TestContainerNamesReadAsTheBareComposeServiceName(t *testing.T) {
	cases := map[string]string{
		"r-1-orders-1.r-1_default.":                        "orders",
		"r-1-orders-12.r-1_backplane.":                     "orders",
		"r-1-cockpit-runner-run-0123456789ab.r-1_default.": "cockpit-runner",
		"r-1-a-run-worker-2.r-1_default.":                  "a-run-worker",
		"r-1-job-run-1-3.r-1_default.":                     "job-run-1",
	}
	for ptr, want := range cases {
		if got, ok := serviceOf("r-1", ptr); !ok || got != want {
			t.Errorf("%s: %q %v, want %q", ptr, got, ok, want)
		}
	}
	if len(cases) != 5 {
		t.Fatalf("%d cases", len(cases))
	}
}

func TestNamesOutsideTheProjectOrWithoutAnInstanceAreNotReadAsServices(t *testing.T) {
	refused := 0
	for _, ptr := range []string{"other-orders-1.other_default.", "r-1-orders.r-1_default.", "orders.r-1_default.", "", "."} {
		if got, ok := serviceOf("r-1", ptr); ok {
			t.Errorf("%q read as %q", ptr, got)
		}
		refused++
	}
	if refused != 5 {
		t.Fatalf("%d cases", refused)
	}
}

func TestACallerWhoseLookupFailsKeepsItsAddress(t *testing.T) {
	r := startRig(t, okUpstream(), func(o *Options) {
		o.LookupAddr = func(context.Context, string) ([]string, error) { return nil, errors.New("no record") }
	})
	r.get("/x")
	if c := r.onlyCall(); c.Caller != "127.0.0.1" {
		t.Fatalf("caller %q", c.Caller)
	}
}

func TestACallerWhoseNameIsOutsideTheProjectKeepsItsAddress(t *testing.T) {
	r := startRig(t, okUpstream(), func(o *Options) {
		o.LookupAddr = func(context.Context, string) ([]string, error) { return []string{"stranger-1.net."}, nil }
	})
	r.get("/x")
	if c := r.onlyCall(); c.Caller != "127.0.0.1" {
		t.Fatalf("caller %q", c.Caller)
	}
}

func TestALookupRunsOncePerAddressWhileItsNameIsFresh(t *testing.T) {
	calls := 0
	n := newCallerNames("p", func(context.Context, string) ([]string, error) {
		calls++
		return []string{"p-orders-1.p_default."}, nil
	})
	for range 3 {
		n.note("10.0.0.2")
		if got := n.name(context.Background(), "10.0.0.2"); got != "orders" {
			t.Fatalf("name %q", got)
		}
	}
	if calls != 1 {
		t.Fatalf("%d lookups", calls)
	}
}
