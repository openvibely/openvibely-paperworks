#!/usr/bin/env bash
# Build-only dependencies: the Go version in go.mod and bash; no runtime package installation.
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/runtime-targets.sh
go_version="$(awk '$1 == "go" { print $2; exit }' go.mod)"
if [[ ! "$go_version" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]; then
  echo 'Could not read a valid Go version from go.mod' >&2
  exit 1
fi
expected_toolchain="go${go_version}"
if ! installed_toolchain="$(go env GOVERSION 2>/dev/null)"; then
  echo "Reproducible builds require ${expected_toolchain} (from go.mod); unable to determine the installed Go version." >&2
  exit 1
fi
if [[ "$installed_toolchain" != "$expected_toolchain" ]]; then
  echo "Reproducible builds require ${expected_toolchain} (from go.mod); found ${installed_toolchain}." >&2
  exit 1
fi
version="${PAPERWORKS_VERSION:-0.1.0-dev}"
if [[ ! "$version" =~ ^[a-zA-Z0-9.+_-]+$ ]]; then
  echo 'Invalid PAPERWORKS_VERSION' >&2
  exit 1
fi
export CGO_ENABLED=0 GOTOOLCHAIN="$expected_toolchain"
mkdir -p dist
for target in "${PAPERWORKS_RUNTIME_TARGETS[@]}"; do
  export GOOS="${target%/*}" GOARCH="${target#*/}"
  output="dist/$(paperworks_runtime_executable_name "$target")"
  echo "Building $target" >&2
  go build -mod=readonly -trimpath -buildvcs=false -ldflags="-s -w -buildid= -X github.com/openvibely/openvibely-paperworks/internal/cli.Version=$version" -o "$output" ./cmd/paperworks
  go version -m "$output" > "${output}.buildinfo.txt"
done
