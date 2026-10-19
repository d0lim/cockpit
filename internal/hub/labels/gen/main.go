package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/d0lim/cockpit/internal/hub/labels"
)

func main() {
	out := flag.String("out", "", "path of labels.json")
	flag.Parse()
	data, err := labels.JSON()
	if err == nil {
		err = os.MkdirAll(filepath.Dir(*out), 0o755)
	}
	if err == nil {
		err = os.WriteFile(*out, data, 0o644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
