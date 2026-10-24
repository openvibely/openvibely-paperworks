#!/usr/bin/env bash
# Exercise release packaging with real ZIP/checksums and isolated fake signers.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/repo/scripts" "$tmp/bin" "$tmp/ov/.openvibely/skills/openvibely_release_workflow/scripts"
cp "$root/scripts/release-build.sh" "$tmp/repo/scripts/"
cat > "$tmp/repo/scripts/build.sh" <<'BUILD'
mkdir -p dist
for os in darwin linux windows; do
  for arch in amd64 arm64; do
    name="paperworks-$os-$arch"
    [[ "$os" != windows ]] || name="$name.exe"
    echo unsigned > "dist/$name"
    echo buildinfo > "dist/$name.buildinfo.txt"
  done
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
bash "$tmp/repo/scripts/release-build.sh" 0.0.1 >/dev/null
out="$tmp/repo/dist/0.0.1"
[[ "$(find "$out" -name '*.zip' | wc -l | tr -d ' ')" == 6 ]]
for os in darwin windows; do
  for arch in amd64 arm64; do
    unzip -p "$out/paperworks-$os-$arch.zip" | grep -q signed
  done
done
(cd "$out" && shasum -a 256 -c SHA256SUMS >/dev/null)
if bash "$tmp/repo/scripts/release-build.sh" 0.0.1 >/dev/null 2>&1; then
  echo 'Existing release directory must be rejected' >&2; exit 1
fi
if MOCK_NOTARY_STATUS=Invalid bash "$tmp/repo/scripts/release-build.sh" 0.0.2 >/dev/null 2>&1; then
  echo 'Rejected notarization must fail packaging' >&2; exit 1
fi
[[ ! -e "$tmp/repo/dist/0.0.2/SHA256SUMS" ]]
echo 'Release packaging regression checks passed.'
