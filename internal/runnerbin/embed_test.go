package runnerbin

import (
	"errors"
	"os"
	"testing"
)

func TestLookupReturnsGzForEmbeddedArchAndErrNotEmbeddedOtherwise(t *testing.T) {
	for _, arch := range []string{"arm64", "amd64"} {
		_, statErr := os.Stat("bin/cockpit-runner-linux-" + arch + ".gz")
		b, err := Lookup(arch)
		if statErr == nil {
			if err != nil || len(b) == 0 {
				t.Fatalf("%s: embedded but got %v / %d bytes", arch, err, len(b))
			}
			continue
		}
		if !errors.Is(err, ErrNotEmbedded) || b != nil {
			t.Fatalf("%s: got %v / %d bytes", arch, err, len(b))
		}
	}
	if _, err := Lookup("riscv64"); !errors.Is(err, ErrNotEmbedded) {
		t.Fatalf("%v", err)
	}
}
