package home

import (
	"os"
	"path/filepath"
)

const (
	EnvHome  = "COCKPIT_HOME"
	xdgState = "XDG_STATE_HOME"
	appDir   = "cockpit"
)

func Dir(getenv func(string) string) (string, error) {
	if v := getenv(EnvHome); v != "" {
		return v, nil
	}
	if v := getenv(xdgState); v != "" {
		return filepath.Join(v, appDir), nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(userHome, ".local", "state", appDir), nil
}
