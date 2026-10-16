package home

import (
	"os"
	"path/filepath"
	"testing"
)

func Test상태_디렉토리는_COCKPIT_HOME_XDG_STATE_HOME_기본_순서로_정한다(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	if got, err := Dir(env(map[string]string{"COCKPIT_HOME": "/x/home", "XDG_STATE_HOME": "/y"})); err != nil || got != "/x/home" {
		t.Fatalf("%q %v", got, err)
	}
	if got, err := Dir(env(map[string]string{"XDG_STATE_HOME": "/y"})); err != nil || got != filepath.Join("/y", "cockpit") {
		t.Fatalf("%q %v", got, err)
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	if got, err := Dir(env(map[string]string{})); err != nil || got != filepath.Join(userHome, ".local", "state", "cockpit") {
		t.Fatalf("%q %v", got, err)
	}
}
