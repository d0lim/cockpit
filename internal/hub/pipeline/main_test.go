package pipeline

import (
	"os"
	"testing"

	"github.com/d0lim/cockpit/internal/collect/collecttest"
)

func TestMain(m *testing.M) {
	collecttest.MaybeServeChild()
	os.Exit(m.Run())
}
