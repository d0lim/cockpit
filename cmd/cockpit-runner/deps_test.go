package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheRunnerBinaryLinksTheOTLPTypesButNotGRPC(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "cockpit-runner")
	build := exec.Command("go", "build", "-trimpath", "-o", bin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=arm64")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command("go", "version", "-m", bin).CombinedOutput()
	if err != nil {
		t.Fatalf("go version -m: %v\n%s", err, out)
	}
	deps := 0
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "dep" {
			continue
		}
		deps++
		if strings.HasPrefix(fields[1], "google.golang.org/grpc") || strings.HasPrefix(fields[1], "github.com/grpc-ecosystem/") {
			t.Errorf("the runner links %s", fields[1])
		}
	}
	if deps == 0 || !strings.Contains(string(out), "go.opentelemetry.io/proto/otlp") || !strings.Contains(string(out), "google.golang.org/protobuf") {
		t.Fatalf("go version -m listed %d deps without the OTLP types, so the check read nothing:\n%s", deps, out)
	}
}
