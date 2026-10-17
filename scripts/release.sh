#!/usr/bin/env bash
# Build installer-compatible ZIPs and checksums. Requires Go, zip and shasum/sha256sum.
set -euo pipefail
cd "$(dirname "$0")/.."
: "${PAPERWORKS_VERSION:?Set PAPERWORKS_VERSION to the release version without v}"
command -v zip >/dev/null
bash scripts/build.sh
release_dir="$PWD/dist"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
archives=()
for target in darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64; do
  executable=paperworks
  [[ "$target" != windows-* ]] || executable=paperworks.exe
  binary="dist/paperworks-${target}"
  [[ "$target" != windows-* ]] || binary="${binary}.exe"
  cp "$binary" "$stage/$executable"
  chmod 755 "$stage/$executable"
  archive="paperworks-${target}.zip"
  rm -f "dist/$archive"
  (cd "$stage" && zip -q -X "$release_dir/$archive" "$executable")
  rm "$stage/$executable"
  archives+=("$archive")
done
(
  cd dist
  if command -v sha256sum >/dev/null; then
    sha256sum "${archives[@]}" > SHA256SUMS
  else
    shasum -a 256 "${archives[@]}" > SHA256SUMS
  fi
)
echo "Release archives and SHA256SUMS written to dist/" >&2
