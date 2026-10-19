package main

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/hub/pipeline"
	"github.com/d0lim/cockpit/internal/hub/sse"
)

func TestPipelineEventsBecomeJobFramesUnderTheRunPipelineJobAndAMissingExitCodeReadsAsOne(t *testing.T) {
	zero, two := 0, 2
	cases := []struct {
		in   pipeline.Event
		want sse.JobEvent
	}{
		{pipeline.Event{Run: "r1", Phase: pipeline.PhaseStart}, sse.JobStarted("r1/pipeline", "pipeline", "")},
		{pipeline.Event{Run: "r1", Phase: pipeline.PhaseOut, Line: "building app"}, sse.JobOutput("r1/pipeline", "pipeline", "building app")},
		{pipeline.Event{Run: "r1", Phase: pipeline.PhaseExit, Code: &zero}, sse.JobExited("r1/pipeline", "pipeline", 0)},
		{pipeline.Event{Run: "r1", Phase: pipeline.PhaseExit, Code: &two}, sse.JobExited("r1/pipeline", "pipeline", 2)},
		{pipeline.Event{Run: "r1", Phase: pipeline.PhaseExit}, sse.JobExited("r1/pipeline", "pipeline", 1)},
	}
	ran := 0
	for _, c := range cases {
		if got := jobFrame(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%+v: got %+v, want %+v", c.in, got, c.want)
		}
		ran++
	}
	if ran != 5 || *jobFrame(cases[2].in).Code != 0 {
		t.Fatalf("ran %d cases, or the passed exit lost its zero code", ran)
	}
}

func TestAnInvalidConfigStopsTheHubWithANonZeroExitBeforeItListens(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("limits:\n  runningCap: 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	env := map[string]string{"COCKPIT_HOME": home, "COCKPIT_PORT": strconv.Itoa(port)}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out, errOut bytes.Buffer
	code := run(ctx, []string{"hub"}, func(k string) string { return env[k] }, &out, &errOut)
	if code != 1 || ctx.Err() != nil || !strings.Contains(errOut.String(), "limits.runningCap must be a positive integer, got 0") {
		t.Fatalf("exit %d, context %v, stderr %q", code, ctx.Err(), errOut.String())
	}
	if conn, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port)); err == nil {
		conn.Close()
		t.Fatal("something listens on the hub port")
	}
	if _, err := os.Stat(filepath.Join(home, "hub.lock")); !os.IsNotExist(err) {
		t.Fatalf("the hub lock was taken: %v", err)
	}
}
