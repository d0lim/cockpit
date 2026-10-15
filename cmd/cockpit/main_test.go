package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersion출력은_접두와_버전을_싣는다(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--version"}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if !strings.HasPrefix(out.String(), "cockpit ") || len(strings.TrimSpace(out.String())) <= len("cockpit ") {
		t.Fatalf("stdout %q", out.String())
	}
}

func Test인자_없는_호출은_usage와_종료_2(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, &out, &errOut); code != 2 || out.Len() != 0 || !strings.Contains(errOut.String(), "usage") {
		t.Fatalf("exit %d stdout %q stderr %q", code, out.String(), errOut.String())
	}
}
