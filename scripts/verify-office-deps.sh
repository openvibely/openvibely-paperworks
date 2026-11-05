#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
source "$script_dir/office-test-packages.sh"

for package in "${office_test_packages[@]}"; do
  status="$(dpkg-query -W -f='${db:Status-Status}' "$package" 2>/dev/null || true)"
  if [[ "$status" != installed ]]; then
    echo "Required office test package is not installed: $package" >&2
    exit 1
  fi
done

command -v soffice >/dev/null 2>&1 || {
  echo 'Required office test executable is missing: soffice' >&2
  exit 1
}
command -v node >/dev/null 2>&1 || {
  echo 'Required test executable is missing: node' >&2
  exit 1
}

dpkg-query -W -f='${binary:Package}=${Version} (${db:Status-Status})\n' "${office_test_packages[@]}"
soffice --version
node --version
