package compose

import (
	"bytes"
	"context"
	"testing"
)

func TestCLIRunWritesStderrToTheGivenWriterAndKeepsItInTheResultOtherwise(t *testing.T) {
	var out, errOut bytes.Buffer
	res, err := CLI{Bin: "/bin/sh"}.Run(context.Background(), Command{Args: []string{"-c", "echo out; echo err >&2"}, Stdout: &out, Stderr: &errOut})
	if err != nil || out.String() != "out\n" || errOut.String() != "err\n" || len(res.Stderr) != 0 {
		t.Fatalf("with writers: %q %q %q %v", out.String(), errOut.String(), res.Stderr, err)
	}
	res, err = CLI{Bin: "/bin/sh"}.Run(context.Background(), Command{Args: []string{"-c", "echo err >&2; exit 3"}})
	if err != nil || string(res.Stderr) != "err\n" || res.Code != 3 {
		t.Fatalf("without writers: %q %d %v", res.Stderr, res.Code, err)
	}
}
