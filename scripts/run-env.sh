#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
project="${COCKPIT_RUN_ENV_PROJECT:-$root/testdata/toy-project}"
work="$(mktemp -d "${TMPDIR:-/tmp}/cockpit-project-XXXXXX")"
trap 'rm -rf "$work"' EXIT
cp -R "$project/." "$work/"
rm -rf "$work/.env" "$work/e2e.private.env.json" "$work/mock/bin" "$work/.git"
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_COUNT=4 GIT_CONFIG_KEY_0=maintenance.auto GIT_CONFIG_VALUE_0=false GIT_CONFIG_KEY_1=gc.auto GIT_CONFIG_VALUE_1=0 GIT_CONFIG_KEY_2=maintenance.autoDetach GIT_CONFIG_VALUE_2=false GIT_CONFIG_KEY_3=gc.autoDetach GIT_CONFIG_VALUE_3=false
git -C "$work" init -q -b main
git -C "$work" add -A
git -C "$work" -c user.name=cockpit -c user.email=cockpit@example.invalid commit -q -m "run-env project"
"$root/bin/cockpit" run-env --project "$work" "$@"
