#!/usr/bin/env bash
# env-pollution-scan.sh — verifies test code does NOT write to prod
# monitoring, prod backup paths, or knb manifest pointers.
#
# Enforces G115 (env-pollution-isolation). Run on pre-push.
# Exit 0 = clean. Exit 1 = pollution patterns found.

set -euo pipefail

REPO_ROOT="${1:-.}"
FAILED=0

err() { echo "[env-pollution-scan][ERROR] $*" >&2; FAILED=1; }
info() { echo "[env-pollution-scan] $*"; }

# Heuristic test-code locations. Override per-repo via TEST_PATHS env.
TEST_PATHS="${TEST_PATHS:-tests/ **/__tests__/ **/test/ **/tests/ **/e2e/ **/*.test.* **/*_test.*}"

# Forbidden patterns in test code:
#   1. Direct push to prod prometheus pushgateway / prod loki / prod jaeger
#   2. Writes to /srv/backups/ (knb-side prod backup root)
#   3. Mutations to knb-side manifest.yaml
#   4. Pushes to prometheus jobs labeled env=prod/home-lab
PATTERNS=(
  'pushgateway.*prod'
  'pushgateway.*home-lab'
  'loki.*tenant.*prod'
  'loki.*tenant.*home-lab'
  '/srv/backups/'
  'knb/[a-z]+/home-lab/manifest\.yaml'
  'config/release-trains\.yaml'
  'config/feature-flags\.[a-z0-9_-]+\.yaml'
  'env[ =:]*"prod"'
  'env[ =:]*"home-lab"'
)

# Allowed contexts (test code may MENTION these in comments/docstrings).
# We only flag when paired with a write/mutation verb on the same line.
WRITE_VERBS='write|push|publish|emit|send|record|append|update|patch|put|post'

found_any=0
for p_glob in $TEST_PATHS; do
  # shellcheck disable=SC2086
  files="$(find $REPO_ROOT -path "*/node_modules/*" -prune -o -path "*/target/*" -prune -o -path "*/.venv/*" -prune -o -type f \( -name '*.ts' -o -name '*.tsx' -o -name '*.js' -o -name '*.py' -o -name '*.rs' -o -name '*.go' -o -name '*.sh' \) -print 2>/dev/null | grep -E "($(echo "$p_glob" | sed 's/\*\*\///g; s/\*//g'))" 2>/dev/null || true)"
  for f in $files; do
    for pat in "${PATTERNS[@]}"; do
      if grep -nE "($WRITE_VERBS).*$pat|$pat.*($WRITE_VERBS)" "$f" 2>/dev/null; then
        err "$f: write to forbidden prod surface matching '$pat'"
        found_any=1
      fi
    done
  done
done

if [[ "$FAILED" -ne 0 ]]; then
  err "env-pollution-scan FAILED — test code MUST NOT write to prod monitoring/backup/manifest"
  exit 1
fi

info "env-pollution-scan PASSED (no test-to-prod-surface writes detected)"
exit 0
