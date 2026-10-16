package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
)

func call(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), args, func(string) string { return "" }, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestVersion출력은_러너_접두와_버전을_싣는다(t *testing.T) {
	code, out, errOut := call("--version")
	if code != 0 || !strings.HasPrefix(out, "cockpit-runner ") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out, errOut)
	}
}

func Test인자_없는_호출과_모르는_명령은_usage와_종료_2(t *testing.T) {
	for _, args := range [][]string{nil, {"fly"}} {
		code, _, errOut := call(args...)
		if code != 2 || !strings.Contains(errOut, "usage") {
			t.Fatalf("%v: exit %d stderr %q", args, code, errOut)
		}
	}
}

func Test하위_명령의_인자_오류는_종료_2_와_사유다(t *testing.T) {
	cases := [][]string{{"run"}, {"run", "Bad/name"}, {"probe"}, {"probe", "x"}, {"probe", "70000"}, {"edge"}, {"edge", "nope"}}
	for _, args := range cases {
		code, _, errOut := call(args...)
		if code != 2 || errOut == "" {
			t.Fatalf("%v: exit %d stderr %q", args, code, errOut)
		}
	}
}

func Test엣지는_컨텍스트가_끝나면_0_으로_돌아온다(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	var out, errOut bytes.Buffer
	if code := run(ctx, []string{"edge", fmt.Sprintf("%d=127.0.0.1:1", port)}, func(string) string { return "" }, &out, &errOut); code != 0 {
		t.Fatalf("exit %d stderr %q", code, errOut.String())
	}
}
