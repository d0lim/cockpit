package compose

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/d0lim/cockpit/internal/collect/worker"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports"
)

const (
	MinCollectorInterval = time.Second
	MaxCollectorInterval = time.Hour
	readDeadline         = 5 * time.Second
)

type CollectorDef struct {
	ID       string
	Code     string
	Interval time.Duration
	Allowed  []string
}

const readerModuleJS = `(function (report) {
	"use strict";
	return {
		collector: (def) => def,
		__report: (def) => report(def !== null && typeof def === "object" ? { id: String(def.id), intervalMs: def.intervalMs === undefined ? null : def.intervalMs } : null),
	};
})`

func collectorsRel(decl contract.Declaration) (string, error) {
	rel := strings.TrimPrefix(string(decl.Scripts.Collectors), string(decl.Scripts.Dir)+"/")
	if rel == string(decl.Scripts.Collectors) || !fs.ValidPath(rel) {
		return "", fmt.Errorf("%s is not inside %s", decl.Scripts.Collectors, decl.Scripts.Dir)
	}
	return rel, nil
}

func AllowedPorts(decl contract.Declaration) []string {
	allowed := []string{ports.PortDocker, ports.PortSignal, ports.PortState}
	if len(decl.Ports) > 0 {
		allowed = append(allowed, ports.PortHTTP)
	}
	if len(decl.Databases) > 0 {
		allowed = append(allowed, ports.PortSQL)
	}
	slices.Sort(allowed)
	return allowed
}

func collectorFiles(scriptsDir, rel string) ([]string, error) {
	root, err := os.OpenRoot(scriptsDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), rel)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".ts") {
			ids = append(ids, strings.TrimSuffix(e.Name(), ".ts"))
		}
	}
	slices.Sort(ids)
	return ids, nil
}

func CollectorDefs(ctx context.Context, decl contract.Declaration, scriptsDir string) ([]CollectorDef, error) {
	if decl.Scripts.Collectors == "" {
		return nil, nil
	}
	rel, err := collectorsRel(decl)
	if err != nil {
		return nil, err
	}
	ids, err := collectorFiles(scriptsDir, rel)
	if err != nil {
		return nil, err
	}
	allowed := AllowedPorts(decl)
	defs := make([]CollectorDef, 0, len(ids))
	for _, id := range ids {
		if !namePattern.MatchString(id) || slices.Contains(ReservedCollectorIDs, id) {
			return nil, fmt.Errorf("collector file %s/%s.ts does not name an allowed collector id", rel, id)
		}
		entry := path.Join(rel, id+".ts")
		code, err := worker.Bundle(ctx, scriptsDir, entry)
		if err != nil {
			return nil, fmt.Errorf("collector %s: %w", id, err)
		}
		interval, err := readInterval(ctx, scriptsDir, entry, id)
		if err != nil {
			return nil, fmt.Errorf("collector %s: %w", id, err)
		}
		defs = append(defs, CollectorDef{ID: id, Code: code, Interval: interval, Allowed: allowed})
	}
	return defs, nil
}

type collectorShape struct {
	ID         string   `json:"id"`
	IntervalMs *float64 `json:"intervalMs"`
}

func readInterval(ctx context.Context, scriptsDir, entry, id string) (time.Duration, error) {
	driver := "import * as mod from \"./" + entry + "\";\nimport { __report } from \"" + engine.ModuleCollect + "\";\nexport default () => __report(mod.default ?? mod.collector);\n"
	b, err := engine.BuildBundle(ctx, engine.BundleInput{Driver: driver, Root: scriptsDir, Site: engine.SiteHub})
	if err != nil {
		return 0, err
	}
	var shape *collectorShape
	module := func(h *engine.Host) (goja.Value, error) {
		rt := h.Runtime()
		factory, err := rt.RunScript(engine.ModuleCollect, readerModuleJS)
		if err != nil {
			return nil, err
		}
		build, ok := goja.AssertFunction(factory)
		if !ok {
			return nil, errors.New("the reader module did not evaluate to a function")
		}
		report := func(call goja.FunctionCall) goja.Value {
			if !goja.IsNull(call.Argument(0)) {
				shape = &collectorShape{}
				if err := h.DecodeArgs(call.Argument(0), shape); err != nil {
					panic(h.Error(err))
				}
			}
			return goja.Undefined()
		}
		return build(goja.Undefined(), rt.ToValue(report))
	}
	eng, err := engine.New(engine.Options{Site: engine.SiteHub, Budget: engine.Budget{Deadline: readDeadline}, Modules: map[string]engine.ModuleBuilder{engine.ModuleCollect: module}})
	if err != nil {
		return 0, err
	}
	if err := eng.Run(ctx, engine.Bundle{Code: b.Code}, "default"); err != nil {
		return 0, err
	}
	return checkShape(shape, id)
}

func checkShape(shape *collectorShape, id string) (time.Duration, error) {
	switch {
	case shape == nil:
		return 0, errors.New("the module does not export collector({ id, run })")
	case shape.ID != id:
		return 0, fmt.Errorf("the collector id %q does not match its file name %q", shape.ID, id)
	case shape.IntervalMs == nil:
		return 0, nil
	}
	ms := *shape.IntervalMs
	d := time.Duration(ms) * time.Millisecond
	if ms != math.Trunc(ms) || d < MinCollectorInterval || d > MaxCollectorInterval {
		return 0, fmt.Errorf("intervalMs %v must be a whole number of milliseconds from %d to %d", ms, MinCollectorInterval.Milliseconds(), MaxCollectorInterval.Milliseconds())
	}
	return d, nil
}
