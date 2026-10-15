package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersion출력은_러너_접두와_버전을_싣는다(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"--version"}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if !strings.HasPrefix(out.String(), "cockpit-runner ") {
		t.Fatalf("stdout %q", out.String())
	}
}

func Test인자_없는_호출은_usage와_종료_2(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "usage") {
		t.Fatalf("exit %d stderr %q", code, errOut.String())
	}
}
