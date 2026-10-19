package ui

import (
	"embed"
	"io/fs"
)

//go:generate sh -c "cd ../../../web && bun install --frozen-lockfile --silent && rm -rf ../internal/hub/ui/dist && bun build ./index.html ./live/worker.ts --outdir ../internal/hub/ui/dist --production"

//go:embed dist
var dist embed.FS

func Files() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
