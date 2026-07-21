#!/usr/bin/env bash
# Hermetic selftest for release-assurance-gate.sh (IMP-100 Phase 3 choke point #3).
# macOS+WSL portable — no `timeout`; yq-gated (the guard consumes yq).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/release-assurance-gate.sh"
FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }
TMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

if ! command -v yq >/dev/null 2>&1; then
  echo "release-assurance-gate-selftest: SKIP (yq not installed)"
  exit 0
fi

# Standard train registry: mvp floor=fast, prod floor=full, exp floor=unset(→fast default).
TRAINS='trains:
  - id: mvp
    minimum_assurance: fast
  - id: prod
    minimum_assurance: full
  - id: exp'

setup_trains() { mkdir -p "$1/config"; printf '%s\n' "$TRAINS" > "$1/config/release-trains.yaml"; }
add_spec() { mkdir -p "$1/specs/$2"; printf '%s\n' "$3" > "$1/specs/$2/state.json"; }
run() {
  local label="$1" exp="$2" root="$3"
  local rc=0
  bash "$GUARD" "$root" >/dev/null 2>&1 && rc=0 || rc=$?
  if [[ "$rc" -eq "$exp" ]]; then pass "$label"; else fail "$label (expected exit $exp, got $rc)"; fi
}

echo "Running release-assurance-gate selftest..."

# T1: no release-trains.yaml → no-op.
d="$TMP_ROOT/t1"; mkdir -p "$d"
run "T1 no release-trains.yaml → no-op (exit 0)" 0 "$d"

# T2: trains file present, no specs → nothing to check.
d="$TMP_ROOT/t2"; setup_trains "$d"
run "T2 trains but no specs → clean (exit 0)" 0 "$d"

# T3: full on prod (floor=full) → deployable.
d="$TMP_ROOT/t3"; setup_trains "$d"
add_spec "$d" 001-a '{ "releaseTrain": "prod", "certification": { "assurance": { "level": "full", "missingForFull": [] } } }'
run "T3 full on prod(full) → deployable (exit 0)" 0 "$d"

# T4: fast on mvp (floor=fast) → deployable.
d="$TMP_ROOT/t4"; setup_trains "$d"
add_spec "$d" 001-a '{ "releaseTrain": "mvp", "certification": { "assurance": { "level": "fast", "missingForFull": ["independent-audit"] } } }'
run "T4 fast on mvp(fast) → deployable (exit 0)" 0 "$d"

# T5: fast on prod (floor=full) → NOT deployable.
d="$TMP_ROOT/t5"; setup_trains "$d"
add_spec "$d" 001-a '{ "releaseTrain": "prod", "certification": { "assurance": { "level": "fast", "missingForFull": ["independent-audit"] } } }'
run "T5 fast on prod(full) → refuse (exit 1)" 1 "$d"

# T6: prototype on mvp (floor=fast) → NEVER deployable.
d="$TMP_ROOT/t6"; setup_trains "$d"
add_spec "$d" 001-a '{ "releaseTrain": "mvp", "certification": { "assurance": { "level": "prototype", "missingForFull": ["all-tests-passing"] } } }'
run "T6 prototype on mvp → refuse (exit 1)" 1 "$d"

# T7: full on mvp (floor=fast) → deployable (full always meets floor).
d="$TMP_ROOT/t7"; setup_trains "$d"
add_spec "$d" 001-a '{ "releaseTrain": "mvp", "certification": { "assurance": { "level": "full", "missingForFull": [] } } }'
run "T7 full on mvp(fast) → deployable (exit 0)" 0 "$d"

# T8: spec with no certification.assurance → skipped.
d="$TMP_ROOT/t8"; setup_trains "$d"
add_spec "$d" 001-a '{ "releaseTrain": "prod", "status": "done" }'
run "T8 no assurance block → skipped (exit 0)" 0 "$d"

# T9: assurance present but no releaseTrain → skipped.
d="$TMP_ROOT/t9"; setup_trains "$d"
add_spec "$d" 001-a '{ "certification": { "assurance": { "level": "prototype", "missingForFull": ["x"] } } }'
run "T9 assurance but no releaseTrain → skipped (exit 0)" 0 "$d"

# T10: exp train (unset minimum_assurance → default fast); fast is deployable.
d="$TMP_ROOT/t10"; setup_trains "$d"
add_spec "$d" 001-a '{ "releaseTrain": "exp", "certification": { "assurance": { "level": "fast", "missingForFull": ["independent-audit"] } } }'
run "T10 fast on exp(default fast) → deployable (exit 0)" 0 "$d"

# T11: high riskClass escalates the floor to full → fast on mvp+high → refuse.
d="$TMP_ROOT/t11"; setup_trains "$d"
add_spec "$d" 001-a '{ "releaseTrain": "mvp", "riskClass": "high", "certification": { "assurance": { "level": "fast", "missingForFull": ["independent-audit"] } } }'
run "T11 fast on mvp(fast) but riskClass=high → refuse (escalated to full) (exit 1)" 1 "$d"

# T12: invalid train minimum_assurance → error.
d="$TMP_ROOT/t12"; mkdir -p "$d/config"
printf '%s\n' 'trains:
  - id: bad
    minimum_assurance: prototype' > "$d/config/release-trains.yaml"
add_spec "$d" 001-a '{ "releaseTrain": "bad", "certification": { "assurance": { "level": "full", "missingForFull": [] } } }'
run "T12 invalid train minimum_assurance=prototype → refuse (exit 1)" 1 "$d"

# T13: two specs, one deployable + one not → overall refuse.
d="$TMP_ROOT/t13"; setup_trains "$d"
add_spec "$d" 001-a '{ "releaseTrain": "mvp", "certification": { "assurance": { "level": "fast", "missingForFull": ["independent-audit"] } } }'
add_spec "$d" 002-b '{ "releaseTrain": "prod", "certification": { "assurance": { "level": "fast", "missingForFull": ["independent-audit"] } } }'
run "T13 mixed set (one under-assured) → refuse (exit 1)" 1 "$d"

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "release-assurance-gate-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "release-assurance-gate-selftest: all cases passed."
