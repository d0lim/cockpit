package runner

import (
	"bufio"
	"context"
	"net"
	"testing"
	"time"
)

func echoServer(t *testing.T, tag string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				line, _ := bufio.NewReader(c).ReadString('\n')
				c.Write([]byte(tag + ":" + line))
			}()
		}
	}()
	return ln.Addr().String()
}

func TestEdgeRoutesEachEntryToItsUpstreamAndClosesConnectionWhenUpstreamIsUnreachable(t *testing.T) {
	a, b := echoServer(t, "A"), echoServer(t, "B")
	routes := []Route{{Listen: "127.0.0.1:0", Upstream: a}, {Listen: "127.0.0.1:0", Upstream: b}, {Listen: "127.0.0.1:0", Upstream: "127.0.0.1:1"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	proxies, err := ListenAll(routes)
	if err != nil {
		t.Fatal(err)
	}
	go ServeAll(ctx, proxies)
	ask := func(addr, msg string) string {
		c, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(2 * time.Second))
		c.Write([]byte(msg + "\n"))
		line, _ := bufio.NewReader(c).ReadString('\n')
		return line
	}
	if got := ask(proxies[0].Addr().String(), "hi"); got != "A:hi\n" {
		t.Fatalf("%q", got)
	}
	if got := ask(proxies[1].Addr().String(), "yo"); got != "B:yo\n" {
		t.Fatalf("%q", got)
	}
	if got := ask(proxies[2].Addr().String(), "x"); got != "" {
		t.Fatalf("unreachable upstream: %q", got)
	}
}

func TestEdgeEntrySyntaxIsPortEqualsHostColonPort(t *testing.T) {
	r, err := ParseRoute("18080=api:8080")
	if err != nil || r != (Route{Listen: ":18080", Upstream: "api:8080"}) {
		t.Fatalf("%+v %v", r, err)
	}
	for _, bad := range []string{"", "18080", "x=api:8080", "18080=api", "18080=:8080", "70000=a:1", "18080=a:0"} {
		if _, err := ParseRoute(bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
}
