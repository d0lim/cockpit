package runner

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestProbePrintsOneLinePerAttemptWithRcOneOnErrorAndZeroOnSuccess(t *testing.T) {
	attempts := []Attempt{
		{Key: "tcp", Try: func(context.Context) error { return nil }},
		{Key: "dns", Try: func(context.Context) error { return errors.New("no") }},
		{Key: "host", Try: func(context.Context) error { return nil }},
		{Key: "udp", Try: func(context.Context) error { return errors.New("no") }},
	}
	var out strings.Builder
	Probe(context.Background(), attempts, &out)
	if out.String() != "probe=1 rc=0\nprobe=2 rc=1\nprobe=3 rc=0\nprobe=4 rc=1\n" {
		t.Fatalf("%q", out.String())
	}
}

func TestDefaultAttemptsAreTCPThenDNSThenHostCanaryThenUDPAndTCPAndDNSTriesAreReal(t *testing.T) {
	attempts := Attempts("7700")
	if len(attempts) != 4 || attempts[0].Key != "tcp" || attempts[1].Key != "dns" || attempts[2].Key != "host" || attempts[3].Key != "udp" {
		t.Fatalf("%+v", attempts)
	}
	if !strings.Contains(attempts[2].Label, "7700") {
		t.Fatalf("the canary port is missing from the label: %q", attempts[2].Label)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tcpTry(ln.Addr().String())(ctx); err != nil {
		t.Fatalf("open port: %v", err)
	}
	if err := tcpTry("127.0.0.1:1")(ctx); err == nil {
		t.Fatal("a closed port accepted the connection")
	}
	if err := dnsTry("localhost")(ctx); err != nil {
		t.Fatalf("localhost: %v", err)
	}
}
