package main

import (
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/api"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

var config = Config{NullZero: []reflect.Type{reflect.TypeFor[lifecycle.Verdict]()}, Roots: []reflect.Type{
	reflect.TypeFor[api.HubList](),
	reflect.TypeFor[api.RunDetail](),
	reflect.TypeFor[api.Submitted](),
	reflect.TypeFor[api.OpReply](),
	reflect.TypeFor[api.Problem](),
	reflect.TypeFor[api.ReportView](),
	reflect.TypeFor[api.KitView](),
	reflect.TypeFor[api.ExecView](),
	reflect.TypeFor[api.FlowView](),
	reflect.TypeFor[api.SourceView](),
	reflect.TypeFor[api.SignalsView](),
	reflect.TypeFor[api.SuitesView](),
	reflect.TypeFor[contract.CollectStage](),
	reflect.TypeFor[contract.Tone](),
}}

func main() {
	root := flag.String("root", ".", "repository root")
	out := flag.String("out", "", "path of the generated TypeScript file")
	flag.Parse()
	text, err := Render(*root, config)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(*out), 0o755)
	}
	if err == nil {
		err = os.WriteFile(*out, []byte(text), 0o644)
	}
	if err != nil {
		slog.Error("API type generation failed", "err", err)
		os.Exit(1)
	}
}
