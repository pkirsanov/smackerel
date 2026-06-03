#!/usr/bin/env bash
# regression-baseline-guard-selftest.sh
#
# Hermetic selftest for regression-baseline-guard.sh.
#
# The guard always exits 0 today (it emits informational warnings rather
# than hard failures), so this selftest verifies:
#   - The guard runs cleanly against a minimal valid spec dir (exit 0)
#     and emits the expected G044/G045/G046 section banners. [former G045/G046 absorbed into G044; banners retained for backward compat]
#   - When two specs share the same route in design.md, the guard
#     surfaces the route collision message in --verbose mode (still
#     exit 0, by design — but the warning text MUST appear).
#   - Missing report.md is tolerated as a baseline-establishment case.
#
# Cleans up the temp tree on exit via trap.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/regression-baseline-guard.sh"

if [[ ! -f "$GUARD" ]]; then
  echo "[selftest regression-baseline-guard] FAIL: target script missing at $GUARD" >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1"; failures=$((failures + 1)); }

specs_dir="$TMPDIR/specs"
mkdir -p "$specs_dir"

# --- Case 1: clean spec dir with baseline note in report.md → exit 0 ---
clean_spec="$specs_dir/001-clean-spec"
mkdir -p "$clean_spec"
cat > "$clean_spec/design.md" <<'EOF'
# Design

## Routes

- POST /api/v1/things — create a thing.
EOF
cat > "$clean_spec/state.json" <<'EOF'
{"version": 3, "status": "in_progress"}
EOF
cat > "$clean_spec/report.md" <<'EOF'
# Report

## Test Baseline Comparison

| Suite | Before | After | Delta |
|-------|--------|-------|-------|
| unit  | 100    | 102   | +2    |
EOF

echo "[selftest regression-baseline-guard] Case 1: clean spec → exit 0"
log1="$TMPDIR/log1.txt"
set +e
bash "$GUARD" "$clean_spec" >"$log1" 2>&1
status1=$?
set -e
if [[ "$status1" -eq 0 ]]; then
  pass "clean spec exits 0 (got $status1)"
else
  fail "clean spec should exit 0 (got $status1)"
  sed -n '1,60p' "$log1"
fi
for token in 'G044' 'G045' 'G046'; do
  if grep -Fq "$token" "$log1"; then
    pass "output contains $token banner"
  else
    fail "output missing $token banner"
    sed -n '1,80p' "$log1"
  fi
done
if grep -Fq 'Test baseline comparison found in report' "$log1"; then
  pass "baseline comparison detected"
else
  fail "expected baseline-comparison detection in output"
  sed -n '1,80p' "$log1"
fi

# --- Case 2: spec dir colliding with another spec on a route → still exit 0,
# but verbose output must surface the collision ------------------------
collider_a="$specs_dir/002-route-owner"
mkdir -p "$collider_a"
cat > "$collider_a/design.md" <<'EOF'
# Design A

POST /api/v1/duplicate
EOF
cat > "$collider_a/state.json" <<'EOF'
{"version": 3, "status": "done"}
EOF

collider_b="$specs_dir/003-route-collider"
mkdir -p "$collider_b"
cat > "$collider_b/design.md" <<'EOF'
# Design B

POST /api/v1/duplicate
EOF
cat > "$collider_b/state.json" <<'EOF'
{"version": 3, "status": "in_progress"}
EOF

echo "[selftest regression-baseline-guard] Case 2: route collision (--verbose) → exit 0 with warning"
log2="$TMPDIR/log2.txt"
set +e
bash "$GUARD" "$collider_b" --verbose >"$log2" 2>&1
status2=$?
set -e
if [[ "$status2" -eq 0 ]]; then
  pass "route-collision case exits 0 (got $status2)"
else
  fail "route-collision case should exit 0 (got $status2)"
  sed -n '1,80p' "$log2"
fi
if grep -Fq 'Route collision' "$log2"; then
  pass "verbose output surfaces 'Route collision' message"
else
  fail "expected 'Route collision' message in verbose output"
  sed -n '1,80p' "$log2"
fi

# --- Case 3: spec dir without report.md → exit 0 with baseline-warning ---
no_report="$specs_dir/004-no-report"
mkdir -p "$no_report"
cat > "$no_report/state.json" <<'EOF'
{"version": 3, "status": "in_progress"}
EOF

echo "[selftest regression-baseline-guard] Case 3: no report.md → exit 0"
log3="$TMPDIR/log3.txt"
set +e
bash "$GUARD" "$no_report" >"$log3" 2>&1
status3=$?
set -e
if [[ "$status3" -eq 0 ]]; then
  pass "missing-report case exits 0 (baseline establishment)"
else
  fail "missing-report case should exit 0 (got $status3)"
  sed -n '1,80p' "$log3"
fi
if grep -Fq 'No report.md found' "$log3"; then
  pass "missing-report message surfaces"
else
  fail "expected 'No report.md found' message"
  sed -n '1,80p' "$log3"
fi

if [[ "$failures" -eq 0 ]]; then
  echo "[selftest regression-baseline-guard] PASS"
  exit 0
fi

echo "[selftest regression-baseline-guard] FAIL: $failures assertion(s)"
exit 1
