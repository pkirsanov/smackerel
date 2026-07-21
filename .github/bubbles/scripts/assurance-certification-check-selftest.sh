#!/usr/bin/env bash
# Hermetic selftest for assurance-certification-check.sh (IMP-100 Phase 3 choke point #1 — enforcement).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/assurance-certification-check.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

if ! command -v jq >/dev/null 2>&1; then
  echo "assurance-certification-check-selftest: SKIP (jq not installed)"
  exit 0
fi

# run <label> <expected-exit> <guard-args...>
run() {
  local label="$1" exp="$2"; shift 2
  local rc=0
  bash "$GUARD" "$@" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq "$exp" ]]; then pass "$label"; else fail "$label (expected exit $exp, got $rc)"; fi
}

mk() { # mk <dir> <json>
  mkdir -p "$1"; printf '%s\n' "$2" > "$1/state.json"
}

echo "Running assurance-certification-check selftest..."

# T1: no state.json → no-op.
d="$TMP_ROOT/t1"; mkdir -p "$d"
run "T1 no state.json → no-op (exit 0)" 0 --feature-dir "$d"

# T2: state.json without a certification.assurance block → no-op.
d="$TMP_ROOT/t2"; mk "$d" '{ "status": "done", "certification": { "status": "done" } }'
run "T2 no assurance block → no-op (exit 0)" 0 --feature-dir "$d"

# ── consistent blocks → pass ───────────────────────────────────────────────
d="$TMP_ROOT/t3"; mk "$d" '{ "certification": { "assurance": { "level": "full", "missingForFull": [] } } }'
run "T3 full + no gaps → consistent (exit 0)" 0 --feature-dir "$d"

d="$TMP_ROOT/t4"; mk "$d" '{ "certification": { "assurance": { "level": "fast", "missingForFull": ["independent-audit"] } } }'
run "T4 fast + [independent-audit] → consistent (exit 0)" 0 --feature-dir "$d"

d="$TMP_ROOT/t5"; mk "$d" '{ "certification": { "assurance": { "level": "prototype", "missingForFull": ["all-tests-passing", "independent-audit"] } } }'
run "T5 prototype + gaps → consistent (exit 0)" 0 --feature-dir "$d"

# ── inconsistent blocks → REFUSE ───────────────────────────────────────────
d="$TMP_ROOT/t6"; mk "$d" '{ "certification": { "assurance": { "level": "full", "missingForFull": ["independent-audit"] } } }'
run "T6 full + non-empty gaps → refuse (exit 1)" 1 --feature-dir "$d"

d="$TMP_ROOT/t7"; mk "$d" '{ "certification": { "assurance": { "level": "fast", "missingForFull": [] } } }'
run "T7 fast + no gaps → refuse (exit 1)" 1 --feature-dir "$d"

d="$TMP_ROOT/t8"; mk "$d" '{ "certification": { "assurance": { "level": "fast", "missingForFull": ["test-coverage-complete"] } } }'
run "T8 fast without independent-audit gap → refuse (exit 1)" 1 --feature-dir "$d"

d="$TMP_ROOT/t9"; mk "$d" '{ "certification": { "assurance": { "level": "prototype", "missingForFull": [] } } }'
run "T9 prototype + no gaps → refuse (exit 1)" 1 --feature-dir "$d"

d="$TMP_ROOT/t10"; mk "$d" '{ "certification": { "assurance": { "level": "gold", "missingForFull": [] } } }'
run "T10 invalid level → refuse (exit 1)" 1 --feature-dir "$d"

# ── usage ──────────────────────────────────────────────────────────────────
run "T11 missing --feature-dir → exit 2" 2
run "T12 --feature-dir nonexistent → exit 2" 2 --feature-dir "$TMP_ROOT/nope"
run "T13 unknown flag → exit 2" 2 --feature-dir "$TMP_ROOT/t3" --ship-it
run "T14 --help → exit 0" 0 --help

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "assurance-certification-check-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "assurance-certification-check-selftest: all cases passed."
