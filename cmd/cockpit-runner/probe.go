package main

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/d0lim/cockpit/internal/runner"
)

func probeMain(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: cockpit-runner probe <canary-port>")
		return 2
	}
	if n, err := strconv.Atoi(args[0]); err != nil || n < 1 || n > 65535 {
		fmt.Fprintf(stderr, "카나리 포트 %q 는 1~65535 사이의 정수가 아니다\n", args[0])
		return 2
	}
	runner.Probe(ctx, runner.Attempts(args[0]), stdout)
	return 0
}
