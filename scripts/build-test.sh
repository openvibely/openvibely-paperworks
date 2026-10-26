#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/scripts" "$tmp/bin"
cp "$repo_root/scripts/build.sh" "$tmp/scripts/build.sh"
cp "$repo_root/scripts/runtime-targets.sh" "$tmp/scripts/runtime-targets.sh"
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
    printf '%s\n' "$*" >> "$MOCK_BUILD_ARGS"
    output=''
    while [[ $# -gt 0 ]]; do
      if [[ "$1" == -o ]]; then output="$2"; break; fi
      shift
    done
    [[ -n "$output" ]] || { echo 'missing build output path' >&2; exit 1; }
    mkdir -p "$(dirname "$output")"
    printf 'binary %s/%s\n' "$GOOS" "$GOARCH" > "$output"
    printf '%s\n' "$output" >> "$MOCK_OUTPUT_LOG"
    ;;
  version)
    [[ "${2:-}" == -m ]] || exit 1
    printf 'build info %s/%s\n' "$GOOS" "$GOARCH"
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
      MOCK_OUTPUT_LOG="$tmp/outputs" \
      MOCK_BUILD_ARGS="$tmp/build-args" \
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
: > "$tmp/targets"
: > "$tmp/outputs"
: > "$tmp/build-args"
run_build "go${version}" "$version" || fail 'Matching toolchain should pass.'
printf '%s\n' darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64 > "$tmp/expected-targets"
diff -u "$tmp/expected-targets" "$tmp/targets" || fail 'Release build matrix differs.'
printf '%s\n' \
  dist/paperworks-darwin-amd64 \
  dist/paperworks-darwin-arm64 \
  dist/paperworks-linux-amd64 \
  dist/paperworks-linux-arm64 \
  dist/paperworks-windows-amd64.exe \
  dist/paperworks-windows-arm64.exe > "$tmp/expected-outputs"
diff -u "$tmp/expected-outputs" "$tmp/outputs" || fail 'Build output names differ.'
while IFS= read -r target; do
  os="${target%/*}"
  arch="${target#*/}"
  name="paperworks-$os-$arch"
  [[ "$os" != windows ]] || name="$name.exe"
  [[ -s "$tmp/dist/$name" ]] || fail "Missing built binary: $name"
  [[ -s "$tmp/dist/$name.buildinfo.txt" ]] || fail "Missing build info: $name.buildinfo.txt"
  grep -q "build info $target" "$tmp/dist/$name.buildinfo.txt" || fail "Incorrect build info: $name.buildinfo.txt"
done < "$tmp/expected-targets"
grep -Fq -- '-mod=readonly -trimpath -buildvcs=false' "$tmp/build-args" || fail 'Reproducible build flags were not passed.'
grep -Fq -- "-ldflags=-s -w -buildid= -X github.com/openvibely/openvibely-paperworks/internal/cli.Version=0.1.0-dev" "$tmp/build-args" || fail 'Version build flags were not passed.'

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
