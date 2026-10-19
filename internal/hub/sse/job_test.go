package sse

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var jobMutations = []porttest.Mutation{
	{"an exit frame keeps a zero exit code", "make Code a plain int with omitempty", "red: the exit frame has no code", "TestJobFramesCarryTheJobEventFieldsBesideKindTargetAndAt"},
	{"job frames go out as job kind", "PublishJob publishes KindLog", "red: the frame head names log", "TestJobFramesCarryTheJobEventFieldsBesideKindTargetAndAt"},
	{"the title is required", "tag Title with omitempty", "red: a frame without a title passes the schema", "TestAJobFrameWithAnUnknownFieldAMissingTitleOrAStringCodeFailsTheSchema"},
}

func jobSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	s, err := jsonschema.ForType(reflect.TypeFor[JobEvent](), nil)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestJobFramesCarryTheJobEventFieldsBesideKindTargetAndAt(t *testing.T) {
	b := New("1", 0, nil)
	events := []JobEvent{JobStarted("j1", "build", "make app"), JobOutput("j1", "build", "step 1/3"), JobExited("j1", "build", 0)}
	want := []map[string]any{
		{"jobId": "j1", "title": "build", "phase": "start", "text": "make app"},
		{"jobId": "j1", "title": "build", "phase": "out", "text": "step 1/3"},
		{"jobId": "j1", "title": "build", "phase": "exit", "code": float64(0)},
	}
	schema := jobSchema(t)
	checked := 0
	for i, e := range events {
		published := b.PublishJob(runA, e)
		raw, err := published.Data()
		if err != nil {
			t.Fatal(err)
		}
		var frame map[string]any
		if err := json.Unmarshal(raw, &frame); err != nil {
			t.Fatal(err)
		}
		if published.Kind != KindJob || frame["kind"] != "job" || frame["target"] != runA || frame["at"] == nil {
			t.Fatalf("frame head %s", raw)
		}
		delete(frame, "kind")
		delete(frame, "target")
		delete(frame, "at")
		if err := schema.Validate(frame); err != nil {
			t.Fatalf("%s does not match the JobEvent schema: %v", raw, err)
		}
		if !reflect.DeepEqual(frame, want[i]) {
			t.Fatalf("frame %d: %v, want %v", i, frame, want[i])
		}
		checked++
	}
	if checked != 3 {
		t.Fatalf("checked %d frames", checked)
	}
}

func TestAJobFrameWithAnUnknownFieldAMissingTitleOrAStringCodeFailsTheSchema(t *testing.T) {
	schema := jobSchema(t)
	bad := []map[string]any{
		{"jobId": "j1", "title": "build", "phase": "start", "extra": true},
		{"jobId": "j1", "phase": "out"},
		{"jobId": "j1", "title": "build", "phase": "exit", "code": "0"},
	}
	refused := 0
	for _, doc := range bad {
		if schema.Validate(doc) == nil {
			t.Errorf("accepted %v", doc)
			continue
		}
		refused++
	}
	if refused != 3 || schema.Validate(map[string]any{"jobId": "j1", "title": "build", "phase": "out"}) != nil {
		t.Fatalf("refused %d, or the minimal frame is refused", refused)
	}
}

func TestJobMutationRecordMatchesThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "job_test.go", jobMutations)
}
