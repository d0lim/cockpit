#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
version="${1:-}"
out="${2:-}"

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]]; then
	echo "version must be semver without a v prefix, got '$version'" >&2
	exit 1
fi
if [ -z "$out" ]; then
	echo "usage: build-npm.sh <version> <output directory>" >&2
	exit 1
fi

rm -rf "$out"
mkdir -p "$out"
cp "$root"/sdk/*.d.ts "$root/sdk/tsconfig.base.json" "$root/npm/README.md" "$out/"
docs_url="https://github.com/d0lim/cockpit/blob/main/docs"
for pair in guide:GUIDE hub:HUB; do
	sed -e 's/](guide\.md/](GUIDE.md/g' -e 's/](hub\.md/](HUB.md/g' -e "s#](\([a-z]*\)\.md#](${docs_url}/\1.md#g" \
		"$root/docs/${pair%%:*}.md" > "$out/${pair##*:}.md"
done
cp "$root/schema/declaration.json" "$out/declaration.schema.json"
{
	printf '/// <reference path="./cockpit-globals.d.ts" />\n'
	printf '/// <reference path="./cockpit-flow.d.ts" />\n'
	printf '/// <reference path="./cockpit-collect.d.ts" />\n'
} > "$out/index.d.ts"
sed "s/\"version\": \"0.0.0\"/\"version\": \"$version\"/" "$root/npm/package.json" > "$out/package.json"
grep -q "\"version\": \"$version\"" "$out/package.json"
echo "assembled @d0lim/cockpit-sdk $version in $out"
