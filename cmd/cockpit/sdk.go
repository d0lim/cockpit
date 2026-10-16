package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/d0lim/cockpit/internal/sdkbin"
)

func sdkWrite(dir string, stdout, stderr io.Writer) int {
	names, err := sdkbin.Write(dir)
	if err != nil {
		fmt.Fprintln(stderr, "sdk 를 쓰지 못했다: "+err.Error())
		return 2
	}
	for _, name := range names {
		fmt.Fprintln(stdout, filepath.Join(dir, name))
	}
	return 0
}
