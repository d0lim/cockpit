#!/usr/bin/env bash
set -euo pipefail

dir="${1:?usage: publish-npm.sh <package directory>}"
name="$(sed -n 's/^  "name": "\(.*\)",$/\1/p' "$dir/package.json")"
version="$(sed -n 's/^  "version": "\(.*\)",$/\1/p' "$dir/package.json")"
if [ -z "$name" ] || [ -z "$version" ]; then
	echo "could not read name and version from $dir/package.json" >&2
	exit 1
fi

if ! out="$(npm view "$name" version 2>&1)"; then
	if printf '%s' "$out" | grep -q E404; then
		echo "::notice::$name is not on the registry yet. Publish the first version by hand, then add this workflow as its trusted publisher on npmjs.com."
		exit 0
	fi
	printf '%s\n' "$out" >&2
	exit 1
fi

if [ "$(npm view "$name@$version" version 2>/dev/null || true)" = "$version" ]; then
	echo "::notice::$name@$version is already published"
	exit 0
fi

cd "$dir"
npm publish --access public
echo "published $name@$version"
