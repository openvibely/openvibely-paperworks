#!/usr/bin/env bash
# Signed release packaging; uses the existing OV signing toolchain and credentials.
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/runtime-targets.sh
version="${1:-}"
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo 'Usage: scripts/release-build.sh X.Y.Z [--dry-run]' >&2; exit 2; }
[[ $# -le 2 && ( $# -eq 1 || "$2" == --dry-run ) ]] || exit 2
ov="${PAPERWORKS_OV_ROOT:-$(cd .. && pwd)/openvibely}"
helpers="$ov/.openvibely/skills/openvibely_release_workflow/scripts"
out="$PWD/dist/$version"
if [[ "${2:-}" == --dry-run ]]; then
  target_list="$(printf '%s ' "${PAPERWORKS_RUNTIME_TARGETS[@]}")"
  target_list="${target_list% }"
  echo "Build $version at $(git rev-parse HEAD) for targets: $target_list."
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
for helper in sign-windows.sh verify-windows.sh; do
  [[ -x "$helpers/$helper" ]] || { echo "Missing helper: $helpers/$helper" >&2; exit 1; }
done
for target in "${PAPERWORKS_RUNTIME_TARGETS[@]}"; do
  name="$(paperworks_runtime_executable_name "$target")"
  rm -f "dist/$name" "dist/$name.buildinfo.txt"
done
PAPERWORKS_VERSION="$version" bash scripts/build.sh
for target in "${PAPERWORKS_RUNTIME_TARGETS[@]}"; do
  name="$(paperworks_runtime_executable_name "$target")"
  binary="$PWD/dist/$name"
  [[ -s "$binary" ]] || { echo "Missing or empty built binary for $target: $binary" >&2; exit 1; }
  [[ -s "$binary.buildinfo.txt" ]] || { echo "Missing or empty build info for $target: $binary.buildinfo.txt" >&2; exit 1; }
done
mkdir -p "$out"
git rev-parse HEAD > "$out/BUILD_COMMIT"
for target in "${PAPERWORKS_RUNTIME_TARGETS[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"
  name="$(paperworks_runtime_executable_name "$target")"
  binary="$PWD/dist/$name"
  case "$os" in
    darwin)
      codesign --force --options runtime --timestamp --sign "$OPENVIBELY_MACOS_SIGN_IDENTITY" --entitlements scripts/macos-entitlements.plist "$binary"
      codesign --verify --strict --verbose=2 "$binary"
      # Exercise executable WASM memory, not just the version flag, before notarizing.
      native_arch="$(uname -m)"
      [[ "$native_arch" != x86_64 ]] || native_arch=amd64
      if [[ "$arch" == "$native_arch" ]]; then
        "$binary" pdf-fonts testdata/regression/pdf-radial-bounded-baseline/source.pdf > "$out/signed-pdf-smoke.json"
      fi
      ;;
    windows)
      bash "$helpers/sign-windows.sh" "$binary"
      bash "$helpers/verify-windows.sh" "$binary"
      ;;
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
(cd "$out" && shasum -a 256 ./*.zip ./*.buildinfo.txt BUILD_COMMIT > SHA256SUMS)
echo "Signed release prepared in $out. Review notes and artifacts before publishing."
