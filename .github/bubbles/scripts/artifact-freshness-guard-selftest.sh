#!/usr/bin/env bash
# artifact-freshness-guard-selftest.sh
#
# Hermetic selftest for artifact-freshness-guard.sh.
#
# Stages a minimal feature dir under a temp directory, then invokes the
# guard and asserts:
#   - A scopes.md whose superseded section keeps an executable Status
#     marker (or a DoD checkbox) trips a freshness violation and exits
#     non-zero.
#   - A scopes.md with no superseded section, plus a spec.md that does
#     not declare any superseded boundary, exits 0 cleanly.
#
# Cleans up the temp tree on exit via trap.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/artifact-freshness-guard.sh"

if [[ ! -f "$GUARD" ]]; then
  echo "[selftest artifact-freshness-guard] FAIL: target script missing at $GUARD" >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1"; failures=$((failures + 1)); }

write_state() {
  local feature_dir="$1"
  cat > "$feature_dir/state.json" <<'EOF'
{
  "version": 3,
  "status": "in_progress",
  "scopeLayout": "single-file"
}
EOF
}

# --- Case 1: clean feature dir (no superseded section) → exit 0 ---
clean_feature="$TMPDIR/specs/100-clean-feature"
mkdir -p "$clean_feature"
write_state "$clean_feature"

cat > "$clean_feature/spec.md" <<'EOF'
# Spec — Active Feature

## Goal

Ship the active feature.
EOF

cat > "$clean_feature/scopes.md" <<'EOF'
# Scopes

## Scope 01: Active Scope

**Status:** Done

### Definition of Done

- [x] Behavior implemented -> Evidence: report.md#test-evidence
EOF

echo "[selftest artifact-freshness-guard] Case 1: clean feature → exit 0"
log1="$TMPDIR/log1.txt"
set +e
bash "$GUARD" "$clean_feature" >"$log1" 2>&1
status1=$?
set -e
if [[ "$status1" -eq 0 ]]; then
  pass "clean feature exits 0 (got $status1)"
else
  fail "clean feature should exit 0 (got $status1)"
  sed -n '1,120p' "$log1"
fi
if grep -Fq 'RESULT: PASS' "$log1"; then
  pass "output contains 'RESULT: PASS'"
else
  fail "expected 'RESULT: PASS' in output"
  sed -n '1,120p' "$log1"
fi

# --- Case 2: superseded scope section keeps executable status/DoD → exit 1 ---
broken_feature="$TMPDIR/specs/200-broken-feature"
mkdir -p "$broken_feature"
write_state "$broken_feature"

cat > "$broken_feature/spec.md" <<'EOF'
# Spec — Mixed History
EOF

cat > "$broken_feature/scopes.md" <<'EOF'
# Scopes

## Scope 01: Current Scope

**Status:** Done

### Definition of Done

- [x] Current behavior implemented -> Evidence: report.md#test-evidence

## Superseded Scopes

### Scope 99: Old Scope That Should Be Frozen

**Status:** Done

### Definition of Done

- [x] Old behavior implemented -> Evidence: report.md#legacy
EOF

echo "[selftest artifact-freshness-guard] Case 2: executable superseded section → exit 1"
log2="$TMPDIR/log2.txt"
set +e
bash "$GUARD" "$broken_feature" >"$log2" 2>&1
status2=$?
set -e
if [[ "$status2" -ne 0 ]]; then
  pass "executable superseded section exits non-zero (got $status2)"
else
  fail "executable superseded section should exit non-zero (got $status2)"
  sed -n '1,160p' "$log2"
fi
if grep -Fq 'superseded scope section' "$log2"; then
  pass "output surfaces superseded-scope violation"
else
  fail "expected 'superseded scope section' in output"
  sed -n '1,160p' "$log2"
fi

if [[ "$failures" -eq 0 ]]; then
  echo "[selftest artifact-freshness-guard] PASS"
  exit 0
fi

echo "[selftest artifact-freshness-guard] FAIL: $failures assertion(s)"
exit 1
