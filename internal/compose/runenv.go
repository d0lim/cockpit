package compose

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
)

type RunEnvInput struct {
	RunID    string
	Layout   Layout
	PortBase int
	Ports    []contract.PortOffset
	Keys     map[string]string
}

func RunEnv(in RunEnvInput) (string, error) {
	entries := [][2]string{
		{"RUN_ID", in.RunID},
		{"RUN_SRC", in.Layout.Src},
		{"RUN_BUILD", in.Layout.Build},
		{"RUN_WORK", in.Layout.Work},
	}
	for _, p := range in.Ports {
		entries = append(entries, [2]string{PortVar(p), strconv.Itoa(HostPort(in.PortBase, p))})
	}
	for _, key := range sortedKeys(in.Keys) {
		entries = append(entries, [2]string{key, in.Keys[key]})
	}
	var b strings.Builder
	for _, e := range entries {
		if strings.TrimSpace(e[1]) == "" || strings.ContainsAny(e[1], "\n\r") {
			return "", fmt.Errorf("run.env value for %s is empty or spans lines", e[0])
		}
		b.WriteString(e[0] + "=" + e[1] + "\n")
	}
	return b.String(), nil
}

func WriteRunEnv(root *os.Root, in RunEnvInput) error {
	text, err := RunEnv(in)
	if err != nil {
		return err
	}
	return writePrivate(root, RunEnvFile, []byte(text))
}
