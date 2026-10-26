#!/usr/bin/env bash
set -euo pipefail

tag="${1:?usage: release-notes.sh <tag>}"
heading="## Declarations changed"

prev="$(git describe --tags --abbrev=0 --match 'v*' "$tag^" 2>/dev/null || true)"
if [ -z "$prev" ]; then
	echo "no tag before $tag; the release notes get no declarations section"
	exit 0
fi

changes="$(git diff "$prev" "$tag" -- sdk)"
if [ -z "$changes" ]; then
	echo "declarations unchanged since $prev"
	exit 0
fi

host_api() {
	git show "$1:internal/contract/hostapi.go" | sed -n 's/^const HostAPI = \([0-9][0-9]*\)$/\1/p'
}
old="$(host_api "$prev")"
new="$(host_api "$tag")"
if [ -z "$old" ] || [ -z "$new" ]; then
	echo "cannot read HostAPI at $prev or $tag" >&2
	exit 1
fi

notes="$(mktemp)"
trap 'rm -f "$notes"' EXIT
gh release view "$tag" --json body --jq .body > "$notes"
if grep -qxF "$heading" "$notes"; then
	echo "the release notes of $tag already have a declarations section"
	exit 0
fi

{
	printf '\n%s\n\n' "$heading"
	if [ "$old" = "$new" ]; then
		printf 'Host API %s, unchanged since %s: these changes only add to it.\n' "$new" "$prev"
	else
		printf 'Host API raised from %s to %s since %s: scripts written for host API %s need changes, and the runner refuses their declaration until it says `hostApi: %s`.\n' "$old" "$new" "$prev" "$old" "$new"
	fi
	printf '\nAfter upgrading, run `cockpit sdk write <dir>` over the committed declarations and review the diff.\n\n'
	printf '```diff\n%s\n```\n' "$changes"
} >> "$notes"
gh release edit "$tag" --notes-file "$notes"
echo "added the declarations section to the release notes of $tag"
