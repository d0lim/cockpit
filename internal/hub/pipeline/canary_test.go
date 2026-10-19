package pipeline

import (
	"net"
	"strconv"
	"testing"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/lifecycle"
)

func TestFakeRunsBindNoPortOfTheLayoutBlockSoTheyPassWhileThoseLoopbackPortsAreTaken(t *testing.T) {
	w := newWorld(t, nil, nil)
	held := 0
	for _, offset := range []int{0, 1, 2, compose.CanaryOffset} {
		l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(w.base+offset)))
		if err != nil {
			continue
		}
		defer l.Close()
		held++
	}
	m := w.submit()
	if m.Verdict != lifecycle.VerdictPassed {
		t.Fatalf("with %d block ports held: state %+v notice %q", held, m.State, m.Notice)
	}
	t.Logf("held %d of the 4 block ports during the run", held)
}
