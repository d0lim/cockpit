package runnerbin

import (
	"errors"
	"testing"
)

func Test러너가_내장되지_않은_트리에서는_ErrNotEmbedded(t *testing.T) {
	for _, arch := range []string{"arm64", "amd64"} {
		b, err := Lookup(arch)
		if !errors.Is(err, ErrNotEmbedded) || b != nil {
			t.Fatalf("%s: got %v / %d bytes", arch, err, len(b))
		}
	}
}
