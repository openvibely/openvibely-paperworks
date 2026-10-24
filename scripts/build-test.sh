#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/scripts" "$tmp/bin"
cp "$repo_root/scripts/build.sh" "$tmp/scripts/build.sh"
cat > "$tmp/go.mod" <<'EOF'
module example.test/build-version

go 1.27.3
EOF
cat > "$tmp/bin/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  env)
    [[ "${2:-}" == GOVERSION ]] || {
      echo "unexpected version check: args=$*" >&2
      exit 1
    }
    printf '%s\n' "$MOCK_GOVERSION"
    ;;
  build)
    printf "%s/%s\n" "$GOOS" "$GOARCH" >> "$MOCK_TARGET_LOG"
    [[ "${GOTOOLCHAIN:-}" == "$MOCK_EXPECTED_TOOLCHAIN" ]] || {
      echo "unexpected build GOTOOLCHAIN: ${GOTOOLCHAIN:-unset}" >&2
      exit 1
    }
    ;;
  version)
    [[ "${2:-}" == -m ]] || exit 1
    ;;
  *)
    echo "unexpected go command: $*" >&2
    exit 1
    ;;
esac
EOF
chmod +x "$tmp/bin/go"

module_go_version() {
  awk '$1 == "go" { print $2; exit }' "$tmp/go.mod"
}
run_build() {
  local installed_version="$1" expected_version="$2" output
  if ! output="$(cd "$tmp" && PATH="$tmp/bin:$PATH" \
      MOCK_TARGET_LOG="$tmp/targets" \
      MOCK_GOVERSION="$installed_version" \
      MOCK_EXPECTED_TOOLCHAIN="go${expected_version}" \
      bash scripts/build.sh 2>&1)"; then
    echo "$output" >&2
    return 1
  fi
}
fail() {
  echo "$*" >&2
  exit 1
}

version="$(module_go_version)"
run_build "go${version}" "$version" || fail 'Matching toolchain should pass.'
printf '%s\n' darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64 > "$tmp/expected-targets"
diff -u "$tmp/expected-targets" "$tmp/targets" || fail 'Release build matrix differs.'

if output="$(cd "$tmp" && PATH="$tmp/bin:$PATH" \
    MOCK_GOVERSION=go9.9.9 \
    MOCK_EXPECTED_TOOLCHAIN="go${version}" \
    bash scripts/build.sh 2>&1)"; then
  fail 'Mismatching toolchain should fail.'
fi
case "$output" in
  *"require go${version} (from go.mod); found go9.9.9"*) ;;
  *) fail "Mismatch should report canonical expected version go${version}; got: $output" ;;
esac

# Change only the module directive; the build script must follow it automatically.
sed 's/^go .*/go 1.42.7/' "$tmp/go.mod" > "$tmp/go.mod.new"
mv "$tmp/go.mod.new" "$tmp/go.mod"
version="$(module_go_version)"
run_build "go${version}" "$version" || fail 'Build script did not follow the changed go.mod directive.'

echo 'Build version regression checks passed.'
