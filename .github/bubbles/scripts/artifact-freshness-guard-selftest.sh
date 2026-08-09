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

# --- Case 3: "suppressed"/"superseded" as DOMAIN VOCABULARY inside a
# sentence-shaped heading is NOT a freshness boundary → exit 0 ---
#
# Regression guard. The predicate used to match the word anywhere in a heading,
# so a behavioural scenario titled "Only suppressed dispositions are eligible"
# was read as "everything below this point is archived" and condemned every
# later heading in the spec as stale.
vocab_feature="$TMPDIR/specs/300-domain-vocabulary"
mkdir -p "$vocab_feature"
write_state "$vocab_feature"

cat > "$vocab_feature/spec.md" <<'EOF'
# Spec — Domain Vocabulary

## BS-300-005: Only suppressed dispositions are eligible

A disposition that was suppressed upstream is the only eligible input.

## BS-300-006: A superseded observation is withheld

An observation superseded by a later one is withheld rather than shown.

## Functional Requirements

These are ACTIVE requirements and must not be reported as stale.

## Lifecycle

Also active.
EOF

cat > "$vocab_feature/scopes.md" <<'EOF'
# Scopes

## Scope 01: Active Scope

**Status:** Done

### Definition of Done

- [x] Behavior implemented -> Evidence: report.md#test-evidence
EOF

echo "[selftest artifact-freshness-guard] Case 3: domain vocabulary is not a boundary → exit 0"
log3="$TMPDIR/log3.txt"
set +e
bash "$GUARD" "$vocab_feature" >"$log3" 2>&1
status3=$?
set -e
if [[ "$status3" -eq 0 ]]; then
  pass "sentence-shaped heading containing 'suppressed' does not trip a boundary (got $status3)"
else
  fail "domain vocabulary must not trip a freshness boundary (got $status3)"
  sed -n '1,160p' "$log3"
fi
if grep -Fq 'active-looking heading' "$log3"; then
  fail "no heading should be reported active-after-boundary in Case 3"
  sed -n '1,160p' "$log3"
else
  pass "no active-after-boundary findings emitted for domain vocabulary"
fi

# --- Case 4 (ADVERSARIAL): a REAL section marker MUST still be detected, so
# Case 3's narrowing cannot silently disable the check it narrows ---
boundary_feature="$TMPDIR/specs/400-real-boundary"
mkdir -p "$boundary_feature"
write_state "$boundary_feature"

cat > "$boundary_feature/spec.md" <<'EOF'
# Spec — Real Boundary

## Goal

Ship the active feature.

## Superseded Requirements

Frozen history below this point.

## Functional Requirements

This heading is ACTIVE-looking and sits after a real boundary, so it MUST fail.
EOF

cat > "$boundary_feature/scopes.md" <<'EOF'
# Scopes

## Scope 01: Active Scope

**Status:** Done

### Definition of Done

- [x] Behavior implemented -> Evidence: report.md#test-evidence
EOF

echo "[selftest artifact-freshness-guard] Case 4 (adversarial): real boundary still detected → exit 1"
log4="$TMPDIR/log4.txt"
set +e
bash "$GUARD" "$boundary_feature" >"$log4" 2>&1
status4=$?
set -e
if [[ "$status4" -ne 0 ]]; then
  pass "a real 'Superseded Requirements' section marker still trips the boundary (got $status4)"
else
  fail "narrowing must not disable boundary detection for real section markers (got $status4)"
  sed -n '1,160p' "$log4"
fi
if grep -Fq 'active-looking heading' "$log4"; then
  pass "active-after-boundary violation still surfaced"
else
  fail "expected 'active-looking heading' in output"
  sed -n '1,160p' "$log4"
fi

if [[ "$failures" -eq 0 ]]; then
  echo "[selftest artifact-freshness-guard] PASS"
  exit 0
fi

echo "[selftest artifact-freshness-guard] FAIL: $failures assertion(s)"
exit 1
