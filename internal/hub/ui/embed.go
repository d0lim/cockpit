package ui

import (
	"embed"
	"io/fs"
)

//go:generate sh -c "cd ../../../web && bun install --frozen-lockfile --silent && rm -rf ../internal/hub/ui/dist && mkdir -p ../internal/hub/ui/dist && bun x --bun tailwindcss -i ./screen/app.css -o ../internal/hub/ui/dist/app.css --minify && bun build ./screen/app.ts --outdir ../internal/hub/ui/dist --production --format=iife && cp node_modules/htmx.org/dist/htmx.min.js node_modules/htmx.org/dist/ext/hx-ws.min.js ../internal/hub/ui/dist/"

//go:embed dist
var dist embed.FS

func Files() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
