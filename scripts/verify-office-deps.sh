#!/usr/bin/env bash
set -euo pipefail

required_packages=(
  libreoffice-writer
  libreoffice-calc
  libreoffice-impress
  fonts-liberation
  fonts-dejavu-core
  fonts-noto-core
)

for package in "${required_packages[@]}"; do
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

dpkg-query -W -f='${binary:Package}=${Version} (${db:Status-Status})\n' "${required_packages[@]}"
soffice --version
node --version
