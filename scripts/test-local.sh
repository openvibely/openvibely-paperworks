#!/usr/bin/env bash
# Native test bootstrap. Go is required; no package manager or administrator access.
set -euo pipefail
case "${1:-}" in
  "") setup_only=0 ;;
  --deps-only) setup_only=1 ;;
  *) echo 'Usage: scripts/test-local.sh [--deps-only]' >&2; exit 2 ;;
esac
cd "$(dirname "$0")/.."
fail() { echo "$*" >&2; exit 1; }
command -v go >/dev/null 2>&1 || fail 'Install Go first, then run make deps.'
platform="$(uname -s)"
case "$platform" in
  Darwin) os=darwin; cache="${XDG_CACHE_HOME:-$HOME/Library/Caches}/paperworks-tests" ;;
  Linux) os=linux; cache="${XDG_CACHE_HOME:-$HOME/.cache}/paperworks-tests" ;;
  *) fail "Unsupported platform: $platform. Configure dependencies and use scripts/test.sh." ;;
esac
case "$(uname -m)" in
  arm64|aarch64) arch=arm64 ;;
  x86_64) arch=x64 ;;
  *) fail 'No verified dependency downloads for this CPU architecture.' ;;
esac
mkdir -p "$cache"
work="$(mktemp -d "$cache/setup.XXXXXX")"
mounted=0
cleanup() {
  if [ "$mounted" = 1 ]; then hdiutil detach "$work/mount" -quiet || true; fi
  rm -rf "$work"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
checksum() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}';
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}
download() {
  local url="$1" expected="$2" destination="$3"
  if [ -f "$destination" ] && [ "$(checksum "$destination")" = "$expected" ]; then return; fi
  echo "Downloading $(basename "$destination")"
  curl --fail --location --retry 3 "$url" --output "$work/download"
  [ "$(checksum "$work/download")" = "$expected" ] || fail "Checksum mismatch: $url"
  mv "$work/download" "$destination"
}

# Reuse a working Node 22+; otherwise install a verified official archive in cache.
if ! command -v node >/dev/null 2>&1 || ! node -e 'process.exit(Number(process.versions.node.split(".")[0]) >= 22 ? 0 : 1)'; then
  node_release="node-v22.22.2-$os-$arch"
  case "$os-$arch" in
    darwin-arm64) node_sha=db4b275b83736df67533529a18cc55de2549a8329ace6c7bcc68f8d22d3c9000 ;;
    darwin-x64) node_sha=12a6abb9c2902cf48a21120da13f87fde1ed1b71a13330712949e8db818708ba ;;
    linux-arm64) node_sha=b2f3a96f31486bfc365192ad65ced14833ad2a3c2e1bcefec4846902f264fa28 ;;
    linux-x64) node_sha=978978a635eef872fa68beae09f0aad0bbbae6757e444da80b570964a97e62a3 ;;
  esac
  if [ ! -x "$cache/$node_release/bin/node" ]; then
    download "https://nodejs.org/dist/v22.22.2/$node_release.tar.gz" "$node_sha" "$cache/$node_release.tar.gz"
    tar -xzf "$cache/$node_release.tar.gz" -C "$work"
    mv "$work/$node_release" "$cache/$node_release"
  fi
  export PATH="$cache/$node_release/bin:$PATH"
fi

if [ -z "${DOCUMENT_SOFFICE:-}" ]; then
  if command -v soffice >/dev/null 2>&1; then
    export DOCUMENT_SOFFICE="$(command -v soffice)"
  elif command -v libreoffice >/dev/null 2>&1; then
    export DOCUMENT_SOFFICE="$(command -v libreoffice)"
  elif [ "$platform" = Darwin ] && [ -x /Applications/LibreOffice.app/Contents/MacOS/soffice ]; then
    export DOCUMENT_SOFFICE=/Applications/LibreOffice.app/Contents/MacOS/soffice
  elif [ "$platform" = Darwin ]; then
    office_dir="$cache/libreoffice-25.8.7-$arch"
    if [ ! -x "$office_dir/LibreOffice.app/Contents/MacOS/soffice" ]; then
      case "$arch" in
        arm64) office_cpu=aarch64; office_sha=e7556aa61e282f89578ebaf35afdb09c94dcf9d6ee7c137004377bee81a6e900 ;;
        x64) office_cpu=x86-64; office_sha=110439f207b5e420d7c8e441180c8374431761d549a1c14b584045ac56cd71c5 ;;
      esac
      office_download_arch="$office_cpu"
      [ "$arch" != x64 ] || office_download_arch=x86_64
      dmg="LibreOffice_25.8.7_MacOS_$office_cpu.dmg"
      download "https://download.documentfoundation.org/libreoffice/stable/25.8.7/mac/$office_download_arch/$dmg" "$office_sha" "$cache/$dmg"
      mkdir -p "$work/mount" "$work/office"
      hdiutil attach "$cache/$dmg" -nobrowse -readonly -mountpoint "$work/mount" -quiet
      mounted=1
      ditto "$work/mount/LibreOffice.app" "$work/office/LibreOffice.app"
      hdiutil detach "$work/mount" -quiet
      mounted=0
      mv "$work/office" "$office_dir"
    fi
    export DOCUMENT_SOFFICE="$office_dir/LibreOffice.app/Contents/MacOS/soffice"
  else
    fail 'Linux needs an installed LibreOffice (soffice on PATH or DOCUMENT_SOFFICE). No verified portable Linux bundle is configured; no tests were run.'
  fi
fi
[ -x "$DOCUMENT_SOFFICE" ] || fail "DOCUMENT_SOFFICE is not executable: $DOCUMENT_SOFFICE"
"$DOCUMENT_SOFFICE" --version
node --version
go version

sheet_sha=cc015130aa8521e7f088f88898eba949ccdcbfb38df0bd129b44b7273c3a6f41
if [ -n "${PAPERWORKS_TEST_SHEETJS:-}" ]; then
  [ -f "$PAPERWORKS_TEST_SHEETJS" ] && [ "$(checksum "$PAPERWORKS_TEST_SHEETJS")" = "$sheet_sha" ] || fail 'Configured SheetJS file has the wrong checksum.'
else
  reader="$cache/xlsx-0.20.3.full.min.js"
  download https://cdn.sheetjs.com/xlsx-0.20.3/package/dist/xlsx.full.min.js "$sheet_sha" "$reader"
  export PAPERWORKS_TEST_SHEETJS="$reader"
fi
cleanup
trap - EXIT INT TERM
if [ "$setup_only" = 1 ]; then
  echo "Dependencies ready. Run go test ./... or make test."
  exit 0
fi
exec bash scripts/test.sh
