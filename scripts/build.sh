#!/usr/bin/env bash
# Build-only dependencies: Go 1.27.3, bash; no runtime package installation.
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ "$(go env GOVERSION)" != "go1.27.3" ]]; then
  echo 'Reproducible builds require Go 1.27.3' >&2
  exit 1
fi
version="${PAPERWORKS_VERSION:-0.1.0-dev}"
if [[ ! "$version" =~ ^[a-zA-Z0-9.+_-]+$ ]]; then
  echo 'Invalid PAPERWORKS_VERSION' >&2
  exit 1
fi
export CGO_ENABLED=0 GOTOOLCHAIN=go1.27.3
mkdir -p dist
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64; do
  export GOOS="${target%/*}" GOARCH="${target#*/}"
  output="dist/paperworks-${GOOS}-${GOARCH}"
  if [[ "$GOOS" == windows ]]; then output="${output}.exe"; fi
  echo "Building $target" >&2
  go build -mod=readonly -trimpath -buildvcs=false -ldflags="-s -w -buildid= -X github.com/openvibely/openvibely-paperworks/internal/cli.Version=$version" -o "$output" ./cmd/paperworks
  go version -m "$output" > "${output}.buildinfo.txt"
done
