package collect_test

import (
	"os"
	"testing"

	"github.com/d0lim/cockpit/internal/collect/collecttest"
)

func TestMain(m *testing.M) {
	collecttest.MaybeServeChild()
	os.Exit(m.Run())
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}
