package config

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
)

var ErrNotAllowed = errors.New("repository is not allowed by the hub config")

type CommonDir func(ctx context.Context, dir string) (string, error)

func (c Config) Allow(ctx context.Context, commonDir CommonDir, path string) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: the submitted path must be absolute, got %q", ErrNotAllowed, path)
	}
	got, err := commonDir(ctx, path)
	if err != nil {
		return "", fmt.Errorf("%w: %s is not inside a git repository", ErrNotAllowed, path)
	}
	for _, repo := range c.AllowedRepositories {
		want, err := commonDir(ctx, repo)
		if err != nil {
			continue
		}
		if want == got {
			return got, nil
		}
	}
	return "", fmt.Errorf("%w: %s belongs to the repository at %s, which is not listed in allowedRepositories of %s", ErrNotAllowed, path, got, FileName)
}
