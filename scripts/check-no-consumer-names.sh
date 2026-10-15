#!/usr/bin/env bash
set -euo pipefail

root="${1:-.}"
dirs=(internal cmd web sdk schema)
pattern="${COCKPIT_CONSUMER_NAMES:-}"

files=()
for d in "${dirs[@]}"; do
	[ -d "$root/$d" ] || continue
	while IFS= read -r f; do files+=("$f"); done < <(find "$root/$d" -type f ! -path '*/node_modules/*' ! -path '*/dist/*' | sort)
done

echo "scanned ${#files[@]} files"
if [ "${#files[@]}" -eq 0 ]; then
	echo "no files scanned under $root (${dirs[*]})" >&2
	exit 1
fi

if [ -z "$pattern" ]; then
	echo "names skipped: COCKPIT_CONSUMER_NAMES is empty"
	exit 0
fi

if grep -nHIiE -- "$pattern" "${files[@]}"; then
	echo "consumer names found" >&2
	exit 1
fi
echo "names ok"
