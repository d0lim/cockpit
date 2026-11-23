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
	claimLimit   = 1000
	claimTimeout = 30 * time.Second
	claimLabel   = "io.cockpit.claim"
)

func claimName(id string) string { return "cockpit-claim-" + id }

func claimRunID(ctx context.Context, d compose.Docker, image string, now time.Time, sha string, limit time.Duration) (string, error) {
	for n := 1; n <= claimLimit; n++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		id := store.HeadlessRunID(now, sha, n)
		won, err := claimOne(ctx, d, image, id, limit)
		if err != nil {
			return "", err
		}
		if won {
			return id, nil
		}
	}
	return "", fmt.Errorf("no free run id among the %d after %s", claimLimit, store.HeadlessRunID(now, sha, 0))
}

func claimOne(parent context.Context, d compose.Docker, image, id string, limit time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), limit)
	defer cancel()
	res, err := d.Run(ctx, compose.Command{Args: []string{"create", "--name", claimName(id), "--label", claimLabel + "=" + id, "--network", "none", image}})
	if err != nil {
		return false, err
	}
	if res.Code != 0 {
		if bytes.Contains(res.Stderr, []byte("is already in use")) {
			return false, nil
		}
		return false, fmt.Errorf("docker create %s exited %d: %s", claimName(id), res.Code, strings.TrimSpace(string(res.Stderr)))
	}
	busy, err := occupied(ctx, d, id)
	if err == nil && !busy {
		return true, nil
	}
	if rmErr := removeClaim(parent, d, id, limit); rmErr != nil {
		return false, rmErr
	}
	return false, err
}

func occupied(ctx context.Context, d compose.Docker, id string) (bool, error) {
	filter := "label=" + compose.LabelRunID + "=" + id
	for _, args := range [][]string{{"ps", "-a", "-q", "--filter", filter}, {"network", "ls", "-q", "--filter", filter}} {
		res, err := d.Run(ctx, compose.Command{Args: args})
		if err != nil {
			return false, err
		}
		if res.Code != 0 {
			return false, fmt.Errorf("docker %s exited %d", strings.Join(args[:2], " "), res.Code)
		}
		if strings.TrimSpace(string(res.Stdout)) != "" {
			return true, nil
		}
	}
	return false, nil
}

func removeClaim(parent context.Context, d compose.Docker, id string, limit time.Duration) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), limit)
	defer cancel()
	res, err := d.Run(ctx, compose.Command{Args: []string{"rm", "-f", claimName(id)}})
	if err == nil && res.Code != 0 {
		err = fmt.Errorf("docker rm %s exited %d", claimName(id), res.Code)
	}
	return err
}

func (r *run) release(ctx context.Context) {
	if !r.claimed {
		return
	}
	r.settle(r.st.run("release run id", func() (string, error) {
		return "container " + claimName(r.meta.ID) + " removed", removeClaim(ctx, r.deps.Docker, r.meta.ID, claimTimeout)
	}))
}
