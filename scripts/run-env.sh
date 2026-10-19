#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
project="${COCKPIT_RUN_ENV_PROJECT:-$root/testdata/toy-project}"
work="$(mktemp -d "${TMPDIR:-/tmp}/cockpit-project-XXXXXX")"
trap 'rm -rf "$work"' EXIT
cp -R "$project/." "$work/"
rm -rf "$work/.env" "$work/e2e.private.env.json" "$work/mock/bin" "$work/.git"
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
git -C "$work" init -q -b main
git -C "$work" add -A
git -C "$work" -c user.name=cockpit -c user.email=cockpit@example.invalid commit -q -m "run-env project"
"$root/bin/cockpit" run-env --project "$work" "$@"
