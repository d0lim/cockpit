#!/usr/bin/env bash
set -uo pipefail

project="$(cd "${COCKPIT_PROJECT:-.}" && pwd)"
archive="${COCKPIT_ARCHIVE:-cockpit-run.tar.gz}"
[[ "$archive" = /* ]] || archive="$GITHUB_WORKSPACE/$archive"
out="$RUNNER_TEMP/cockpit-report-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"
rm -rf "$out"

preexisting=0
if [[ -e "$archive" ]]; then
  preexisting=1
fi

args=(run --project "$project" --out "$out" --archive "$archive")
if [[ -n "${COCKPIT_SUITE:-}" ]]; then
  args+=(--suite "$COCKPIT_SUITE")
fi
if [[ "${COCKPIT_WIRE:-false}" = true ]]; then
  args+=(--wire)
fi
if [[ -n "${COCKPIT_DECLARATION_FILE:-}" ]]; then
  args+=(--declaration "$COCKPIT_DECLARATION_FILE")
fi

code=0
cockpit "${args[@]}" || code=$?
echo "exit-code=$code" >> "$GITHUB_OUTPUT"
if [[ "$preexisting" = 0 && -f "$archive" ]]; then
  echo "archive=$archive" >> "$GITHUB_OUTPUT"
fi

summary="$out/summary.json"
if [[ ! -f "$summary" ]]; then
  {
    echo "## cockpit run: no report bundle"
    echo
    echo "The run closed before its suite (exit code $code). The log of the Run the suite step names the stage and its last lines."
  } >> "$GITHUB_STEP_SUMMARY"
  exit 0
fi

echo "verdict=$(jq -r '.verdict' "$summary")" >> "$GITHUB_OUTPUT"
jq -r --arg code "$code" --arg declaration "${COCKPIT_DECLARATION_FILE:-.cockpit/project.yaml}" -f "$GITHUB_ACTION_PATH/scripts/action-summary.jq" "$summary" >> "$GITHUB_STEP_SUMMARY"
