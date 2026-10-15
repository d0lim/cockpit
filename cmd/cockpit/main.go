package main

import (
	"fmt"
	"io"
	"os"

	"github.com/d0lim/cockpit/internal/version"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(stdout, "cockpit "+version.String())
		return 0
	}
	fmt.Fprintln(stderr, "usage: cockpit --version")
	return 2
}
