package headless

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/store"
)

const claimLimit = 1000

func claimRunID(ctx context.Context, d compose.Docker, dir string, now time.Time, sha string) (string, string, error) {
	taken, err := takenRunIDs(ctx, d)
	if err != nil {
		return "", "", err
	}
	held := make(map[string]bool, len(taken))
	for _, id := range taken {
		held[id] = true
	}
	for n := 1; n <= claimLimit; n++ {
		id := store.HeadlessRunID(now, sha, n)
		if held[id] {
			continue
		}
		claim := filepath.Join(dir, "cockpit-"+id)
		err := os.Mkdir(claim, 0o700)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		return id, claim, nil
	}
	return "", "", fmt.Errorf("no free run id among the %d after %s", claimLimit, store.HeadlessRunID(now, sha, 0))
}

func takenRunIDs(ctx context.Context, d compose.Docker) ([]string, error) {
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
	return taken, nil
}
