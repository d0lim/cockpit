package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/d0lim/cockpit/internal/skillbin"
)

func skillWrite(dir string, stdout, stderr io.Writer) int {
	names, err := skillbin.Write(dir)
	if err != nil {
		fmt.Fprintln(stderr, "cannot write the skill: "+err.Error())
		return 2
	}
	for _, name := range names {
		fmt.Fprintln(stdout, filepath.Join(dir, name))
	}
	return 0
}
