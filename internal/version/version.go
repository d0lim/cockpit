package version

import "runtime/debug"

var Version = "dev"

func Revision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return ""
}

func Commit() string {
	if rev := Revision(); rev != "" {
		return rev
	}
	return String()
}

func String() string {
	if Version != "dev" {
		return Version
	}
	if rev := Revision(); len(rev) >= 12 {
		return Version + "+" + rev[:12]
	}
	return Version
}
