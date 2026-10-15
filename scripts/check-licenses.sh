#!/usr/bin/env bash
set -euo pipefail

dir="${1:-.}"
denied='go\.k6\.io/k6|github\.com/grafana/k6'

if hit="$(grep -oE "^[[:space:]]*(require[[:space:]]+)?($denied)(/[^[:space:]]*)?" "$dir/go.mod" | head -1)"; then
	name="$(printf '%s' "$hit" | sed -E 's/^[[:space:]]*(require[[:space:]]+)?//')"
	echo "denied module: $name" >&2
	exit 1
fi

cd "$dir"
self="$(go list -m)"
go tool go-licenses check ./... --disallowed_types=forbidden,restricted --ignore "$self"
echo "licenses ok"
