package version

import "runtime/debug"

var Version = "dev"

func String() string {
	if Version != "dev" {
		return Version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Version
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 12 {
			return Version + "+" + s.Value[:12]
		}
	}
	return Version
}
