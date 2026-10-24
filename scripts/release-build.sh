#!/usr/bin/env bash
# Signed release packaging; uses the existing OV signing toolchain and credentials.
set -euo pipefail
cd "$(dirname "$0")/.."
version="${1:-}"
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo 'Usage: scripts/release-build.sh X.Y.Z [--dry-run]' >&2; exit 2; }
[[ $# -le 2 && ( $# -eq 1 || "$2" == --dry-run ) ]] || exit 2
ov="${PAPERWORKS_OV_ROOT:-$(cd .. && pwd)/openvibely}"
helpers="$ov/.openvibely/skills/openvibely_release_workflow/scripts"
out="$PWD/dist/$version"
if [[ "${2:-}" == --dry-run ]]; then
  echo "Build $version at $(git rev-parse HEAD) for darwin/linux/windows, amd64/arm64."
  echo "Sign and verify macOS/Windows using $helpers; notarize both macOS ZIPs."
  echo "Package into $out; compute SHA256SUMS after signing. No publishing."
  exit 0
fi
[[ "$(uname -s)" == Darwin ]] || { echo 'Release coordinator must run on macOS for signing/notarization.' >&2; exit 1; }
[[ -z "$(git status --porcelain --untracked-files=normal)" ]] || { echo 'Commit source changes before building a release.' >&2; exit 1; }
[[ ! -e "$out" ]] || { echo "Output already exists: $out (preserve it for notarization recovery)." >&2; exit 1; }
config="${PAPERWORKS_SIGNING_ENV:-$ov/.release-signing.env}"
[[ -f "$config" ]] || { echo "Missing signing configuration: $config" >&2; exit 1; }
set -a
source "$config"
set +a
: "${OPENVIBELY_MACOS_SIGN_IDENTITY:?Missing Developer ID identity}"
: "${OPENVIBELY_MACOS_NOTARY_PROFILE:?Missing notary profile}"
export SKIP_WAILS_DOCKER_SETUP=1
for helper in sign-macos.sh sign-windows.sh verify-windows.sh; do
  [[ -x "$helpers/$helper" ]] || { echo "Missing helper: $helpers/$helper" >&2; exit 1; }
done
PAPERWORKS_VERSION="$version" bash scripts/build.sh
mkdir -p "$out"
git rev-parse HEAD > "$out/BUILD_COMMIT"
for os in darwin linux windows; do
  for arch in amd64 arm64; do
    name="paperworks-$os-$arch"
    [[ "$os" != windows ]] || name="$name.exe"
    binary="$PWD/dist/$name"
    case "$os" in
      darwin) bash "$helpers/sign-macos.sh" "$binary" ;;
      windows) bash "$helpers/sign-windows.sh" "$binary"; bash "$helpers/verify-windows.sh" "$binary" ;;
    esac
    # Keep the established archive names and root executable paths used by OV skills.
    archive="$out/paperworks-${os}-${arch}.zip"
    (cd dist && COPYFILE_DISABLE=1 zip -X -q "$archive" "$name")
    unzip -tq "$archive"
    cp "dist/$name.buildinfo.txt" "$out/"
    if [[ "$os" == darwin ]]; then
      xcrun notarytool submit "$archive" --keychain-profile "$OPENVIBELY_MACOS_NOTARY_PROFILE" --wait --output-format json > "$out/notary-$arch.json"
      python3 - "$out/notary-$arch.json" <<'PY'
import json, sys
report = json.load(open(sys.argv[1]))
if report.get('status') != 'Accepted':
    raise SystemExit('Apple notarization not accepted: ' + str(report.get('status')))
PY
    fi
  done
done
(cd "$out" && shasum -a 256 ./*.zip ./*.buildinfo.txt BUILD_COMMIT > SHA256SUMS)
echo "Signed release prepared in $out. Review notes and artifacts before publishing."
