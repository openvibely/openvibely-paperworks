#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
bash scripts/build-test.sh
bash scripts/release-test.sh
# Reject forbidden equation dependencies even if accidentally added as indirect.
check_equation_graph() {
  graph="$(go list -m all)"
  case "$graph" in
    *github.com/srwiley/rasterx*|*github.com/srwiley/oksvg*|*github.com/kanrichan/resvg-go*|*github.com/go-tex/math*)
      echo 'Forbidden equation dependency in module graph' >&2; exit 1;;
  esac
}
check_equation_graph
# Run vet even if an acceptance assertion fails, and retain a failing exit code.
result=0
if [ -n "${PAPERWORKS_TEST_JSON:-}" ]; then
  go test -json -count=1 -timeout=20m ./... | tee "$PAPERWORKS_TEST_JSON" || result=1
else
  go test -count=1 -timeout=20m ./... || result=1
fi
go vet ./... || result=1
exit "$result"
