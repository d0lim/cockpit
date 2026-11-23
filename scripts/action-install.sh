#!/usr/bin/env bash
set -euo pipefail

bin="$RUNNER_TEMP/cockpit/bin"
mkdir -p "$bin"

if [[ -n "${COCKPIT_PATH:-}" ]]; then
  install -m 0755 "$COCKPIT_PATH" "$bin/cockpit"
  echo "$bin" >> "$GITHUB_PATH"
  "$bin/cockpit" --version
  exit 0
fi

version="${COCKPIT_VERSION:-}"
if [[ -z "$version" && "${ACTION_REF:-}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  version="$ACTION_REF"
fi
version="${version#v}"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "::error::set the version input to a cockpit release such as 0.6.0, or use the action at a release tag"
  exit 1
fi

case "$(uname -m)" in
  x86_64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "::error::no cockpit release for $(uname -m)"; exit 1 ;;
esac

name="cockpit_${version}_linux_${arch}.tar.gz"
base="https://github.com/d0lim/cockpit/releases/download/v${version}"
work="$(mktemp -d)"
curl -fsSL --retry 3 -o "$work/$name" "$base/$name"
curl -fsSL --retry 3 -o "$work/checksums.txt" "$base/checksums.txt"
(cd "$work" && grep " ${name}\$" checksums.txt | sha256sum -c -)
tar -xzf "$work/$name" -C "$work" cockpit
install -m 0755 "$work/cockpit" "$bin/cockpit"
rm -rf "$work"
echo "$bin" >> "$GITHUB_PATH"
"$bin/cockpit" --version
