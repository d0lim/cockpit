package headless

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/store"
)

const (
	claimLimit = 1000
	claimLabel = "io.cockpit.claim"
)

func claimName(id string) string { return "cockpit-claim-" + id }

func claimRunID(ctx context.Context, d compose.Docker, image string, now time.Time, sha string) (string, error) {
	taken, err := takenRunIDs(ctx, d)
	if err != nil {
		return "", err
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
		res, err := d.Run(ctx, compose.Command{Args: []string{"create", "--name", claimName(id), "--label", claimLabel + "=" + id, "--network", "none", image}})
		if err != nil {
			return "", err
		}
		if res.Code == 0 {
			return id, nil
		}
		if !bytes.Contains(res.Stderr, []byte("is already in use")) {
			return "", fmt.Errorf("docker create %s exited %d: %s", claimName(id), res.Code, strings.TrimSpace(string(res.Stderr)))
		}
	}
	return "", fmt.Errorf("no free run id among the %d after %s", claimLimit, store.HeadlessRunID(now, sha, 0))
}

func (r *run) release(ctx context.Context) {
	if !r.claimed {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupLimit)
	defer cancel()
	r.settle(r.st.run("release run id", func() (string, error) {
		res, err := r.deps.Docker.Run(ctx, compose.Command{Args: []string{"rm", "-f", claimName(r.meta.ID)}})
		if err == nil && res.Code != 0 {
			err = fmt.Errorf("docker rm %s exited %d", claimName(r.meta.ID), res.Code)
		}
		return "container " + claimName(r.meta.ID) + " removed", err
	}))
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
