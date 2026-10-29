#!/usr/bin/env bash
# Exercise release packaging with real ZIP/checksums and isolated fake signers.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/repo/scripts" "$tmp/bin" "$tmp/ov/.openvibely/skills/openvibely_release_workflow/scripts"
cp "$root/scripts/release-build.sh" "$tmp/repo/scripts/"
cp "$root/scripts/runtime-targets.sh" "$tmp/repo/scripts/runtime-targets.sh"
cat > "$tmp/repo/scripts/build.sh" <<'BUILD'
#!/usr/bin/env bash
set -euo pipefail
source scripts/runtime-targets.sh
mkdir -p dist
for target in "${PAPERWORKS_RUNTIME_TARGETS[@]}"; do
  [[ "${MOCK_BUILD_FAIL_TARGET:-}" != "$target" ]] || { echo "mock build failure for $target" >&2; exit 1; }
  os="${target%/*}"
  arch="${target#*/}"
  name="paperworks-$os-$arch"
  [[ "$os" != windows ]] || name="$name.exe"
  [[ "${MOCK_MISSING_TARGET:-}" == "$target" ]] || echo "unsigned $target" > "dist/$name"
  [[ "${MOCK_MISSING_BUILDINFO_TARGET:-}" == "$target" ]] || echo "buildinfo $target" > "dist/$name.buildinfo.txt"
done
BUILD
cat > "$tmp/bin/uname" <<'MOCK'
#!/bin/sh
case "${1:-}" in -m) echo test-host ;; *) echo Darwin ;; esac
MOCK
cat > "$tmp/bin/git" <<'MOCK'
#!/bin/sh
case "$1" in
  status) ;;
  rev-parse) echo aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa ;;
  *) exit 1 ;;
esac
MOCK
cat > "$tmp/bin/codesign" <<'MOCK'
#!/usr/bin/env bash
if [[ "$1" == --verify ]]; then grep -q signed "${@: -1}"; else echo signed >> "${@: -1}"; fi
MOCK
cat > "$tmp/bin/xcrun" <<'MOCK'
#!/bin/sh
printf '{"status":"%s"}\n' "${MOCK_NOTARY_STATUS:-Accepted}"
MOCK
helpers="$tmp/ov/.openvibely/skills/openvibely_release_workflow/scripts"
for platform in macos windows; do
  cat > "$helpers/sign-$platform.sh" <<'MOCK'
#!/bin/sh
echo signed >> "$1"
MOCK
done
cat > "$helpers/verify-windows.sh" <<'MOCK'
#!/bin/sh
grep -q signed "$1"
MOCK
cat > "$tmp/ov/.release-signing.env" <<'CONFIG'
OPENVIBELY_MACOS_SIGN_IDENTITY=test
OPENVIBELY_MACOS_NOTARY_PROFILE=test
CONFIG
chmod +x "$tmp/bin/"* "$helpers/"*
export PATH="$tmp/bin:$PATH" PAPERWORKS_OV_ROOT="$tmp/ov"
cp "$tmp/repo/scripts/runtime-targets.sh" "$tmp/runtime-targets.original"
printf '\nPAPERWORKS_RUNTIME_TARGETS+=(freebsd/arm64)\n' >> "$tmp/repo/scripts/runtime-targets.sh"
dry_run="$(bash "$tmp/repo/scripts/release-build.sh" 0.0.9 --dry-run)"
expected_dry_run_line='Build 0.0.9 at aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa for targets: darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64 freebsd/arm64.'
actual_dry_run_line="$(printf '%s\n' "$dry_run" | sed -n '1p')"
[[ "$actual_dry_run_line" == "$expected_dry_run_line" ]] || {
  echo "Dry-run target line mismatch: $actual_dry_run_line" >&2; exit 1;
}
cp "$tmp/runtime-targets.original" "$tmp/repo/scripts/runtime-targets.sh"
bash "$tmp/repo/scripts/release-build.sh" 0.0.1 >/dev/null
out="$tmp/repo/dist/0.0.1"
printf '%s\n' \
  darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64 > "$tmp/expected-targets"
printf '%s\n' \
  paperworks-darwin-amd64.zip \
  paperworks-darwin-arm64.zip \
  paperworks-linux-amd64.zip \
  paperworks-linux-arm64.zip \
  paperworks-windows-amd64.zip \
  paperworks-windows-arm64.zip > "$tmp/expected-archives"
find "$out" -name '*.zip' -exec basename {} \; | sort > "$tmp/actual-archives"
sort "$tmp/expected-archives" -o "$tmp/expected-archives"
diff -u "$tmp/expected-archives" "$tmp/actual-archives" || { echo 'Release archive names differ.' >&2; exit 1; }
cat > "$tmp/expected-executables" <<'EOF'
darwin/amd64|paperworks-darwin-amd64
darwin/arm64|paperworks-darwin-arm64
linux/amd64|paperworks-linux-amd64
linux/arm64|paperworks-linux-arm64
windows/amd64|paperworks-windows-amd64.exe
windows/arm64|paperworks-windows-arm64.exe
EOF
while IFS='|' read -r target name; do
  os="${target%/*}"
  arch="${target#*/}"
  archive="$out/paperworks-$os-$arch.zip"
  [[ "$(unzip -Z1 "$archive")" == "$name" ]] || { echo "Unexpected archive entry for $target" >&2; exit 1; }
  grep -q "buildinfo $target" "$out/$name.buildinfo.txt" || { echo "Missing or incorrect packaged build info for $target" >&2; exit 1; }
  if [[ "$os" == linux ]]; then
    unzip -p "$archive" | grep -q "unsigned $target" || { echo "Linux archive content mismatch for $target" >&2; exit 1; }
  else
    unzip -p "$archive" | grep -q signed || { echo "Signed archive content missing for $target" >&2; exit 1; }
  fi
done < "$tmp/expected-executables"
[[ "$(find "$out" -maxdepth 1 -name '*.buildinfo.txt' | wc -l | tr -d ' ')" == 6 ]] || { echo 'Expected six packaged build-info files.' >&2; exit 1; }
(cd "$out" && shasum -a 256 -c SHA256SUMS >/dev/null)
if bash "$tmp/repo/scripts/release-build.sh" 0.0.1 >/dev/null 2>&1; then
  echo 'Existing release directory must be rejected' >&2; exit 1
fi
if MOCK_NOTARY_STATUS=Invalid bash "$tmp/repo/scripts/release-build.sh" 0.0.2 >/dev/null 2>&1; then
  echo 'Rejected notarization must fail packaging' >&2; exit 1
fi
[[ ! -e "$tmp/repo/dist/0.0.2/SHA256SUMS" ]]
if MOCK_BUILD_FAIL_TARGET=linux/arm64 bash "$tmp/repo/scripts/release-build.sh" 0.0.3 >/dev/null 2>&1; then
  echo 'A build failure must stop release packaging.' >&2; exit 1
fi
[[ ! -e "$tmp/repo/dist/0.0.3" ]] || { echo 'A failed build must not create a partial release directory.' >&2; exit 1; }
if output="$(MOCK_MISSING_TARGET=windows/arm64 bash "$tmp/repo/scripts/release-build.sh" 0.0.4 2>&1)"; then
  echo 'A missing built binary must stop release packaging.' >&2; exit 1
fi
case "$output" in
  *'Missing or empty built binary for windows/arm64:'*'paperworks-windows-arm64.exe'*) ;;
  *) echo "Missing Windows executable should report the expected .exe path: $output" >&2; exit 1 ;;
esac
[[ ! -e "$tmp/repo/dist/0.0.4" ]] || { echo 'A missing binary must not create a partial release directory.' >&2; exit 1; }
if MOCK_MISSING_BUILDINFO_TARGET=linux/arm64 bash "$tmp/repo/scripts/release-build.sh" 0.0.5 >/dev/null 2>&1; then
  echo 'Missing build info must stop release packaging.' >&2; exit 1
fi
[[ ! -e "$tmp/repo/dist/0.0.5" ]] || { echo 'Missing build info must not create a partial release directory.' >&2; exit 1; }
echo 'Release packaging regression checks passed.'
