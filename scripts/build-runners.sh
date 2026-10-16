#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
version="${1:-${COCKPIT_VERSION:-dev}}"
out="$root/internal/runnerbin/bin"
mkdir -p "$out"

for arch in arm64 amd64; do
	tmp="$(mktemp)"
	(cd "$root" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags "-s -w -X github.com/d0lim/cockpit/internal/version.Version=$version" -o "$tmp" ./cmd/cockpit-runner)
	gzip -9 -n -c "$tmp" > "$out/cockpit-runner-linux-$arch.gz"
	rm -f "$tmp"
	echo "built cockpit-runner linux/$arch version=$version gz=$(wc -c < "$out/cockpit-runner-linux-$arch.gz" | tr -d ' ') bytes"
done
printf '%s\n' "$version" > "$out/VERSION"
