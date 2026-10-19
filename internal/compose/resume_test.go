package compose

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestResidentOrderSkipsOneShotsAndWaitsThroughThemForHealthyDependencies(t *testing.T) {
	config := map[string]any{"services": map[string]any{
		"db":       map[string]any{},
		"migrator": map[string]any{"restart": "no", "depends_on": map[string]any{"db": map[string]any{"condition": "service_healthy"}}},
		"app":      map[string]any{"depends_on": map[string]any{"migrator": map[string]any{"condition": "service_completed_successfully"}, "cache": map[string]any{}}},
		"cache":    map[string]any{},
		"builder":  map[string]any{"profiles": []any{"build"}},
	}}
	steps, err := ResidentOrder(config)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range steps {
		names = append(names, s.Service)
	}
	if strings.Join(names, ",") != "cache,db,app" {
		t.Fatalf("order %v", names)
	}
	if !slices.Equal(steps[2].Healthy, []string{"db"}) {
		t.Fatalf("app waits for %v", steps[2].Healthy)
	}
	cyclic := map[string]any{"services": map[string]any{
		"a": map[string]any{"depends_on": map[string]any{"b": map[string]any{}}},
		"b": map[string]any{"depends_on": map[string]any{"a": map[string]any{}}},
	}}
	if _, err := ResidentOrder(cyclic); err == nil {
		t.Fatal("a cycle was ordered")
	}
}

func TestStartStartsNothingAndReportsAMissingEnvironmentWhenContainersOrNetworksAreGone(t *testing.T) {
	config := `{"services":{"app":{},"db":{}},"networks":{"default":{"name":"r-toy_default"}}}`
	d := &scripted{reply: func(ctx context.Context, args []string) (Result, error) {
		switch {
		case args[0] == "compose" && slices.Contains(args, "config"):
			return Result{Stdout: []byte(config)}, nil
		case args[0] == "compose" && slices.Contains(args, "ps"):
			return Result{Stdout: []byte(`{"Service":"app","ID":"aaa","State":"exited"}`)}, nil
		case args[0] == "network":
			return Result{Code: 1}, nil
		}
		return Result{}, nil
	}}
	err := runnerProject(t, d).Start(context.Background(), time.Second, time.Millisecond)
	var missing *EnvMissing
	if !errors.As(err, &missing) || !slices.Equal(missing.Containers, []string{"db"}) || !slices.Equal(missing.Networks, []string{"r-toy_default"}) {
		t.Fatalf("err %v", err)
	}
	if slices.ContainsFunc(d.calls, func(c []string) bool { return c[0] == "start" }) {
		t.Fatal("a container was started")
	}
	present := &scripted{reply: func(ctx context.Context, args []string) (Result, error) {
		switch {
		case args[0] == "compose" && slices.Contains(args, "config"):
			return Result{Stdout: []byte(config)}, nil
		case args[0] == "compose" && slices.Contains(args, "ps"):
			return Result{Stdout: []byte(`[{"Service":"app","ID":"aaa","State":"exited"},{"Service":"db","ID":"bbb","State":"exited"}]`)}, nil
		}
		return Result{}, nil
	}}
	if err := runnerProject(t, present).Start(context.Background(), time.Second, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	var started []string
	for _, c := range present.calls {
		if c[0] == "start" {
			started = append(started, c[1])
		}
	}
	if strings.Join(started, ",") != "aaa,bbb" {
		t.Fatalf("started %v", started)
	}
}
