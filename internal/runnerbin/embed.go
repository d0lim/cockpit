package runnerbin

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
)

//go:embed bin
var files embed.FS

var ErrNotEmbedded = errors.New("runner not embedded")

func Lookup(goarch string) ([]byte, error) {
	b, err := files.ReadFile(fmt.Sprintf("bin/cockpit-runner-linux-%s.gz", goarch))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: linux/%s", ErrNotEmbedded, goarch)
	}
	return b, err
}
