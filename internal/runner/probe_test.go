package runner

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func Test프로브는_시도마다_probe_n_rc_줄을_내고_오류는_1_성공은_0_이다(t *testing.T) {
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

func Test기본_시도_넷은_tcp_dns_host_카나리_udp_순서이고_TCP_와_DNS_시도는_실제로_붙는다(t *testing.T) {
	attempts := Attempts("7700")
	if len(attempts) != 4 || attempts[0].Key != "tcp" || attempts[1].Key != "dns" || attempts[2].Key != "host" || attempts[3].Key != "udp" {
		t.Fatalf("%+v", attempts)
	}
	if !strings.Contains(attempts[2].Label, "7700") {
		t.Fatalf("카나리 포트가 라벨에 없다: %q", attempts[2].Label)
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
		t.Fatalf("열린 포트: %v", err)
	}
	if err := tcpTry("127.0.0.1:1")(ctx); err == nil {
		t.Fatal("닫힌 포트가 붙었다")
	}
	if err := dnsTry("localhost")(ctx); err != nil {
		t.Fatalf("localhost: %v", err)
	}
}
