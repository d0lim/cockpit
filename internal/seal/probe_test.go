package seal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/runner"
)

var probeMutations = []porttest.Mutation{
	{"a sealed attempt that succeeds is a violation", "treat rc=0 in judgeSealed as a pass", "red: probe.tcp with rc=0 passes and the verdict is passed", "TestSealedProbesFailingAndControlsSucceedingPassAndAnyLeakFailsTheSeal"},
	{"a canary knock during the sealed probe is a violation", "drop the knocks check in judgeSealed", "red: probe.host passes after the canary accepted a connection", "TestSealedProbesFailingAndControlsSucceedingPassAndAnyLeakFailsTheSeal"},
	{"a missing control makes the seal inconclusive, not passed", "make judgeControl pass without result lines", "red: an offline control yields a passed verdict", "TestMissingResultLinesOrAFailedControlMakeTheSealInconclusive"},
	{"the canary counts real connections", "never increment the accepted counter", "red: Accepted stays 0 after two dials", "TestCanaryCountsConnectionsOnLoopbackOnly"},
	{"the hub judges the attempts the runner makes, in order", "reorder or rename the attempts table", "red: the keys differ from runner.Attempts", "TestProbeAttemptsMatchTheRunnerProbeModeInOrder"},
}

func TestProbeMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "probe_test.go", probeMutations)
}

type fakeCanary struct{ n int }

func (f *fakeCanary) Accepted() int { return f.n }

func lines(codes ...int) []string {
	out := make([]string, len(codes))
	for i, c := range codes {
		out[i] = fmt.Sprintf("probe=%d rc=%d", i+1, c)
	}
	return out
}

func fakeProbes(c *fakeCanary, sealed, control ProbeRun, sealedKnocks, controlKnocks int) RunProbe {
	return func(ctx context.Context, service string) (ProbeRun, error) {
		if service == "probe" {
			c.n += sealedKnocks
			return sealed, nil
		}
		c.n += controlKnocks
		return control, nil
	}
}

func byName(results []Result, name string) Result {
	for _, r := range results {
		if r.Name == name {
			return r
		}
	}
	return Result{Name: "(missing)"}
}

func TestSealedProbesFailingAndControlsSucceedingPassAndAnyLeakFailsTheSeal(t *testing.T) {
	c := &fakeCanary{}
	good := Probes(context.Background(), fakeProbes(c, ProbeRun{Lines: lines(1, 1, 1, 1)}, ProbeRun{Lines: lines(0, 0, 0, 0)}, 0, 1), c, 22099)
	if len(good) != 8 || Verdict(good) != "passed" {
		t.Fatalf("clean probes: %d results, verdict %s: %+v", len(good), Verdict(good), good)
	}
	c = &fakeCanary{}
	leak := Probes(context.Background(), fakeProbes(c, ProbeRun{Lines: lines(0, 1, 1, 1)}, ProbeRun{Lines: lines(0, 0, 0, 0)}, 0, 1), c, 22099)
	if r := byName(leak, "probe.tcp"); r.OK || r.Inconclusive || Verdict(leak) != "failed" {
		t.Fatalf("tcp leak: %+v verdict %s", r, Verdict(leak))
	}
	c = &fakeCanary{}
	knock := Probes(context.Background(), fakeProbes(c, ProbeRun{Lines: lines(1, 1, 1, 1)}, ProbeRun{Lines: lines(0, 0, 0, 0)}, 1, 1), c, 22099)
	if r := byName(knock, "probe.host"); r.OK || r.Inconclusive || Verdict(knock) != "failed" {
		t.Fatalf("canary knock: %+v", r)
	}
	if r := byName(knock, "control.host"); !r.OK {
		t.Fatalf("control host after one control knock: %+v", r)
	}
}

func TestMissingResultLinesOrAFailedControlMakeTheSealInconclusive(t *testing.T) {
	silent := ProbeRun{Code: 125}
	sealed, control := ProbeRun{Lines: lines(1, 1, 1, 1)}, ProbeRun{Lines: lines(0, 0, 0, 0)}
	cases := map[string]func(c *fakeCanary) RunProbe{
		"sealed probe printed nothing": func(c *fakeCanary) RunProbe { return fakeProbes(c, silent, control, 0, 1) },
		"control offline":              func(c *fakeCanary) RunProbe { return fakeProbes(c, sealed, ProbeRun{Lines: lines(1, 1, 1, 1)}, 0, 0) },
		"control run failed": func(c *fakeCanary) RunProbe {
			return func(ctx context.Context, s string) (ProbeRun, error) {
				if s == "probe" {
					return sealed, nil
				}
				return ProbeRun{}, errors.New("compose run failed")
			}
		},
	}
	for name, build := range cases {
		c := &fakeCanary{}
		results := Probes(context.Background(), build(c), c, 22099)
		if Verdict(results) != "error" {
			t.Errorf("%s: verdict %s", name, Verdict(results))
		}
		inconclusive := 0
		for _, r := range results {
			if r.Inconclusive {
				inconclusive++
			}
		}
		if inconclusive != 4 {
			t.Errorf("%s: %d inconclusive results, want the 4 attempts of the silent or failed run", name, inconclusive)
		}
	}
}

func TestCanaryCountsConnectionsOnLoopbackOnly(t *testing.T) {
	c, err := ListenCanary(0)
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

func TestProbeAttemptsMatchTheRunnerProbeModeInOrder(t *testing.T) {
	var runnerKeys, ours []string
	for _, a := range runner.Attempts("1") {
		runnerKeys = append(runnerKeys, a.Key)
	}
	for _, a := range attempts {
		ours = append(ours, a.key)
	}
	if !slices.Equal(runnerKeys, ours) || len(ours) != 4 {
		t.Fatalf("runner %v, seal %v", runnerKeys, ours)
	}
}
