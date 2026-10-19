package main

import (
	"reflect"
	"testing"

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
