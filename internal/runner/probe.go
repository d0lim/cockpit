package runner

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"
)

const probeTimeout = 5 * time.Second

type Attempt struct {
	Key   string
	Label string
	Try   func(ctx context.Context) error
}

func Attempts(canaryPort string) []Attempt {
	return []Attempt{
		{Key: "tcp", Label: "external IP TCP 1.1.1.1:443", Try: tcpTry("1.1.1.1:443")},
		{Key: "dns", Label: "external hostname DNS example.com", Try: dnsTry("example.com")},
		{Key: "host", Label: "host canary host.docker.internal:" + canaryPort, Try: tcpTry(net.JoinHostPort("host.docker.internal", canaryPort))},
		{Key: "udp", Label: "external UDP 1.1.1.1:53", Try: udpTry("1.1.1.1:53")},
	}
}

func Probe(ctx context.Context, attempts []Attempt, out io.Writer) {
	for i, a := range attempts {
		actx, cancel := context.WithTimeout(ctx, probeTimeout)
		rc := 0
		if err := a.Try(actx); err != nil {
			rc = 1
		}
		cancel()
		fmt.Fprintf(out, "probe=%d rc=%d\n", i+1, rc)
	}
}

func tcpTry(addr string) func(context.Context) error {
	return func(ctx context.Context) error {
		c, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return err
		}
		return c.Close()
	}
}

func dnsTry(host string) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := net.DefaultResolver.LookupHost(ctx, host)
		return err
	}
}

func udpTry(addr string) func(context.Context) error {
	return func(ctx context.Context) error {
		c, err := (&net.Dialer{}).DialContext(ctx, "udp", addr)
		if err != nil {
			return err
		}
		defer c.Close()
		_, err = c.Write([]byte("x"))
		return err
	}
}
