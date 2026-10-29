#!/usr/bin/env bash
# Canonical runtime build and release packaging matrix.
PAPERWORKS_RUNTIME_TARGETS=(
  darwin/amd64
  darwin/arm64
  linux/amd64
  linux/arm64
  windows/amd64
  windows/arm64
)

paperworks_runtime_executable_name() {
  local target="${1:?runtime target is required}"
  local os="${target%/*}"
  local arch="${target#*/}"
  local name="paperworks-${os}-${arch}"
  if [[ "$os" == windows ]]; then
    name="${name}.exe"
  fi
  printf '%s\n' "$name"
}
