package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/d0lim/cockpit/internal/scaffold"
	"github.com/d0lim/cockpit/internal/version"
)

func initProject(args []string, stdout, stderr io.Writer) int {
	dir, baseURL := ".", scaffold.DefaultBaseURL
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--base-url" && i+1 < len(args):
			baseURL = args[i+1]
			i++
		case strings.HasPrefix(args[i], "-") || dir != ".":
			fmt.Fprintln(stderr, usage)
			return 2
		default:
			dir = args[i]
		}
	}
	written, err := scaffold.Write(dir, baseURL)
	if err != nil {
		fmt.Fprintln(stderr, "cannot initialize: "+err.Error())
		return 2
	}
	for _, path := range written {
		fmt.Fprintln(stdout, "created "+path)
	}
	pkg := "@d0lim/cockpit-sdk"
	if version.Version != "dev" {
		pkg += "@" + version.Version
	}
	fmt.Fprintf(stdout, `
Next:
  1. Point baseUrl in e2e/env.json at a service in your compose project. Every origin a flow calls must be a value there.
  2. Install the type declarations: bun add -d %s
     Without a registry: cockpit sdk write e2e/.cockpit-sdk, then extend ./.cockpit-sdk/tsconfig.base.json in e2e/tsconfig.json.
  3. Start the stack and run the suite:
     export COCKPIT_RUNNER_IMAGE=$(cockpit runner-image)
     docker compose -f compose.yaml -f compose.cockpit.yaml up -d
     docker compose -f compose.yaml -f compose.cockpit.yaml run --rm runner run smoke
Results land in .e2e/. The usage guide is at https://github.com/d0lim/cockpit/blob/main/docs/guide.md
To run sealed runs on the hub, the declaration needs more fields: https://github.com/d0lim/cockpit/blob/main/docs/hub.md
`, pkg)
	return 0
}
