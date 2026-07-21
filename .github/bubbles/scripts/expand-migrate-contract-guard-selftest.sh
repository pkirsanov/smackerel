#!/usr/bin/env bash
# Hermetic selftest for expand-migrate-contract-guard.sh
# (IMP-100 Phase 4 / IMP-026 SCOPE-2). macOS+WSL portable; jq-gated.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/expand-migrate-contract-guard.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

if ! command -v jq >/dev/null 2>&1; then
  echo "expand-migrate-contract-guard-selftest: SKIP (jq not installed)"
  exit 0
fi

# --- fixtures -----------------------------------------------------------------
VALID='{
  "refactorPattern": "expand-migrate-contract",
  "scopeProgress": [
    {"scope":1,"name":"Expand new API","refactorPhase":"expand","dependsOn":[]},
    {"scope":2,"name":"Migrate web","refactorPhase":"migrate","dependsOn":[1]},
    {"scope":3,"name":"Migrate mobile","refactorPhase":"migrate","dependsOn":[1]},
    {"scope":4,"name":"Contract old API","refactorPhase":"contract","dependsOn":[2,3]}
  ]
}'

NO_PATTERN='{
  "scopeProgress": [
    {"scope":1,"name":"A","dependsOn":[]},
    {"scope":2,"name":"B","dependsOn":[1]}
  ]
}'

CONTRACT_MISSING_MIGRATE='{
  "refactorPattern": "expand-migrate-contract",
  "scopeProgress": [
    {"scope":1,"name":"Expand","refactorPhase":"expand","dependsOn":[]},
    {"scope":2,"name":"Migrate web","refactorPhase":"migrate","dependsOn":[1]},
    {"scope":3,"name":"Migrate mobile","refactorPhase":"migrate","dependsOn":[1]},
    {"scope":4,"name":"Contract","refactorPhase":"contract","dependsOn":[2]}
  ]
}'

MIGRATE_NO_EXPAND='{
  "refactorPattern": "expand-migrate-contract",
  "scopeProgress": [
    {"scope":1,"name":"Expand","refactorPhase":"expand","dependsOn":[]},
    {"scope":2,"name":"Migrate","refactorPhase":"migrate","dependsOn":[]},
    {"scope":3,"name":"Contract","refactorPhase":"contract","dependsOn":[2]}
  ]
}'

MISSING_CONTRACT='{
  "refactorPattern": "expand-migrate-contract",
  "scopeProgress": [
    {"scope":1,"name":"Expand","refactorPhase":"expand","dependsOn":[]},
    {"scope":2,"name":"Migrate","refactorPhase":"migrate","dependsOn":[1]}
  ]
}'

INVALID_PHASE='{
  "refactorPattern": "expand-migrate-contract",
  "scopeProgress": [
    {"scope":1,"name":"Expand","refactorPhase":"expand","dependsOn":[]},
    {"scope":2,"name":"Migrate","refactorPhase":"migrate","dependsOn":[1]},
    {"scope":3,"name":"Contract","refactorPhase":"contract","dependsOn":[2]},
    {"scope":4,"name":"Mystery","refactorPhase":"refactor","dependsOn":[1]}
  ]
}'

EXPAND_DEPENDS_ON_MIGRATE='{
  "refactorPattern": "expand-migrate-contract",
  "scopeProgress": [
    {"scope":1,"name":"Expand","refactorPhase":"expand","dependsOn":[2]},
    {"scope":2,"name":"Migrate","refactorPhase":"migrate","dependsOn":[1]},
    {"scope":3,"name":"Contract","refactorPhase":"contract","dependsOn":[2]}
  ]
}'

VALID_CERT_LOCATION='{
  "refactorPattern": "expand-migrate-contract",
  "certification": {
    "scopeProgress": [
      {"scope":1,"name":"Expand","refactorPhase":"expand","dependsOn":[]},
      {"scope":2,"name":"Migrate","refactorPhase":"migrate","dependsOn":[1]},
      {"scope":3,"name":"Contract","refactorPhase":"contract","dependsOn":[2]}
    ]
  }
}'

mk() {
  local dir="$1" content="$2"
  mkdir -p "$dir"
  printf '%s\n' "$content" > "$dir/state.json"
}
mk_block() {
  mkdir -p "$1/.github"
  printf '%s\n' 'expandMigrateContractGuard: block' > "$1/.github/bubbles-project.yaml"
}
run() {
  local label="$1" exp="$2" dir="$3"
  local rc=0
  bash "$GUARD" "$dir" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq "$exp" ]]; then pass "$label"; else fail "$label (expected exit $exp, got $rc)"; fi
}

echo "Running expand-migrate-contract-guard selftest..."

# T1: no state.json → no-op.
d="$TMP_ROOT/t1"
mkdir -p "$d"
run "T1 no state.json → exit 0" 0 "$d"

# T2: state.json without refactorPattern → no-op.
d="$TMP_ROOT/t2"
mk "$d" "$NO_PATTERN"
mk_block "$d"
run "T2 no refactorPattern (even in block) → no-op (exit 0)" 0 "$d"

# T3: valid pattern, advisory → OK.
d="$TMP_ROOT/t3"
mk "$d" "$VALID"
run "T3 valid EMC, advisory → OK (exit 0)" 0 "$d"

# T4: valid pattern, block → OK.
d="$TMP_ROOT/t4"
mk "$d" "$VALID"
mk_block "$d"
run "T4 valid EMC, block → OK (exit 0)" 0 "$d"

# T5: contract missing a migrate dep, block → violation.
d="$TMP_ROOT/t5"
mk "$d" "$CONTRACT_MISSING_MIGRATE"
mk_block "$d"
run "T5 contract missing a migrate dep, block → exit 1" 1 "$d"

# T6: migrate without expand dep, block → violation.
d="$TMP_ROOT/t6"
mk "$d" "$MIGRATE_NO_EXPAND"
mk_block "$d"
run "T6 migrate without expand dep, block → exit 1" 1 "$d"

# T7: missing contract scope, block → violation.
d="$TMP_ROOT/t7"
mk "$d" "$MISSING_CONTRACT"
mk_block "$d"
run "T7 missing contract scope, block → exit 1" 1 "$d"

# T8: invalid refactorPhase, block → violation.
d="$TMP_ROOT/t8"
mk "$d" "$INVALID_PHASE"
mk_block "$d"
run "T8 invalid refactorPhase, block → exit 1" 1 "$d"

# T9: expand depends on a migrate scope, block → violation.
d="$TMP_ROOT/t9"
mk "$d" "$EXPAND_DEPENDS_ON_MIGRATE"
mk_block "$d"
run "T9 expand depends on migrate, block → exit 1" 1 "$d"

# T10: contract-missing-migrate but ADVISORY → exit 0 (advisory never blocks).
d="$TMP_ROOT/t10"
mk "$d" "$CONTRACT_MISSING_MIGRATE"
run "T10 violation but advisory → exit 0" 0 "$d"

# T11: valid via .certification.scopeProgress location, block → OK.
d="$TMP_ROOT/t11"
mk "$d" "$VALID_CERT_LOCATION"
mk_block "$d"
run "T11 valid EMC via certification.scopeProgress, block → OK (exit 0)" 0 "$d"

# T12: missing feature dir → usage error.
run "T12 missing feature dir → exit 2" 2 "$TMP_ROOT/does-not-exist"

# T13: malformed JSON → runtime error.
d="$TMP_ROOT/t13"
mkdir -p "$d"
printf '%s\n' '{ not valid json' > "$d/state.json"
run "T13 malformed state.json → exit 2" 2 "$d"

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "expand-migrate-contract-guard-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "expand-migrate-contract-guard-selftest: all cases passed."
