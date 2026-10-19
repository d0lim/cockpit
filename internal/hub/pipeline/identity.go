package pipeline

import (
	"fmt"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
)

func SameVersion(a, b string) bool {
	if a == b {
		return true
	}
	unknown := func(x, y string) bool { return x == "dev" && strings.HasPrefix(y, "dev+") }
	return unknown(a, b) || unknown(b, a)
}

func IdentityMismatch(m *contract.Manifest, pin *contract.RunnerImage, cockpitVersion string) string {
	if m == nil || m.Runner == nil {
		return "the manifest carries no runner identity"
	}
	if pin == nil || m.Runner.Image != pin.Tag || !SameVersion(m.Runner.Version, cockpitVersion) {
		pinTag := ""
		if pin != nil {
			pinTag = pin.Tag
		}
		return fmt.Sprintf("runner identity mismatch: the manifest says image %s version %s, the run pinned image %s version %s", m.Runner.Image, m.Runner.Version, pinTag, cockpitVersion)
	}
	return ""
}
