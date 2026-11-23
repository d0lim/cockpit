package headless

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
)

func takenRunIDs(ctx context.Context, d compose.Docker, home string) ([]string, error) {
	var taken []string
	format := `{{.Label "` + compose.LabelRunID + `"}}`
	for _, args := range [][]string{
		{"ps", "-a", "--filter", "label=" + compose.LabelManaged, "--format", format},
		{"network", "ls", "--filter", "label=" + compose.LabelManaged, "--format", format},
	} {
		res, err := d.Run(ctx, compose.Command{Args: args})
		if err != nil {
			return nil, err
		}
		if res.Code != 0 {
			return nil, fmt.Errorf("docker %s exited %d", strings.Join(args[:2], " "), res.Code)
		}
		for _, line := range strings.Split(string(res.Stdout), "\n") {
			if id := strings.TrimSpace(line); id != "" {
				taken = append(taken, id)
			}
		}
	}
	entries, err := os.ReadDir(filepath.Join(home, "runs"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		taken = append(taken, e.Name())
	}
	return taken, nil
}
