package compose

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/d0lim/cockpit/internal/contract"
)

const KitReadmeFile = "env/README.md"

//go:embed kit.md.tmpl
var kitTemplateText string

var kitTemplate = template.Must(template.New("kit").Parse(kitTemplateText))

type KitInput struct {
	RunID    string
	PortBase int
	Ports    []contract.PortOffset
	Notes    string
	Flows    []string
}

type kitRow struct {
	Port    int
	Name    string
	Display string
	Service string
	Open    string
}

func openText(runID string, port int, mode contract.OpenMode) string {
	switch mode {
	case contract.OpenBrowser:
		return fmt.Sprintf("http://%s.localhost:%d", runID, port)
	case contract.OpenLink:
		return fmt.Sprintf("http://127.0.0.1:%d", port)
	}
	return fmt.Sprintf("`127.0.0.1:%d`", port)
}

func KitReadme(in KitInput) (string, error) {
	rows := make([]kitRow, len(in.Ports))
	for i, p := range in.Ports {
		port := HostPort(in.PortBase, p)
		rows[i] = kitRow{Port: port, Name: string(p.Name), Display: p.Display, Service: p.Service, Open: openText(in.RunID, port, p.Open)}
	}
	var b strings.Builder
	err := kitTemplate.Execute(&b, map[string]any{
		"RunID":      in.RunID,
		"Canary":     CanaryPort(in.PortBase),
		"Rows":       rows,
		"Notes":      strings.TrimSpace(in.Notes),
		"FlowsKnown": in.Flows != nil,
		"Flows":      in.Flows,
	})
	if err != nil {
		return "", err
	}
	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

func WriteKit(root *os.Root, in KitInput) error {
	text, err := KitReadme(in)
	if err != nil {
		return err
	}
	return writePrivate(root, KitReadmeFile, []byte(text))
}
