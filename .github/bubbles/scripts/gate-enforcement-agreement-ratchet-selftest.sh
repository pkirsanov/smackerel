#!/usr/bin/env bash
# bubbles/scripts/gate-enforcement-agreement-ratchet-selftest.sh
#
# Hermetic selftest for gate-enforcement-agreement-ratchet.sh (IMP-058 SCOPE-6).
#
# Drives a disposable fixture registry so the real 72-gate baseline never
# gates this selftest's own pass/fail. Case 3 is the one that matters: a gate
# that newly disagrees, with nothing baselined, must fail loudly rather than
# pass silently -- that silent pass is exactly the regression this ratchet
# exists to catch.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/gate-enforcement-agreement-ratchet.sh"
NAME="gate-enforcement-agreement-ratchet-selftest"

failures=0
checks=0
ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
}

WORK="$(mktemp -d 2>/dev/null)" || { printf '%s: cannot create temp dir\n' "$NAME" >&2; exit 1; }
# shellcheck disable=SC2317
cleanup() { [[ -n "${WORK:-}" && -d "$WORK" ]] && rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

write_registry() {
  local dir="$1"
  shift
  mkdir -p "$dir/bubbles/registry"
  {
    printf '# GENERATED:GATE_ENFORCEMENT_START\n'
    printf 'gateEnforcement:\n'
    printf '  derived:\n'
    for row in "$@"; do
      printf '    %s\n' "$row"
    done
    printf '# GENERATED:GATE_ENFORCEMENT_END\n'
  } >"$dir/bubbles/registry/gates.yaml"
}

# --- 1. no disagreement at all: clean with an empty baseline -----------------
r1="$WORK/r1"
write_registry "$r1" \
  'G001: { enforcedBy: [ script:x.sh ], blocking: blocking, blockingBasis: script-exit-nonzero, agreement: agrees }'
set +e
out="$(bash "$TARGET" --repo-root "$r1" 2>&1)"
rc=$?
set -e
if [[ "$rc" -eq 0 ]] && printf '%s' "$out" | grep -q '0 gate(s) currently disagree'; then
  ok "a registry with zero disagreement passes with no baseline needed"
else
  bad "zero-disagreement registry passes" "rc=$rc: $out"
fi

# --- 2. disagreement present, fully baselined: passes ------------------------
r2="$WORK/r2"
write_registry "$r2" \
  'G010: { enforcedBy: [ script:x.sh ], blocking: blocking, blockingBasis: script-exit-nonzero, agreement: divergent, declaredEnforcedBy: [ guard-check:1 ] }' \
  'G011: { enforcedBy: [ behavioral:agents/y.md ], blocking: unknown, blockingBasis: behavioral-only-no-derivable-exit, agreement: contradiction, declaredEnforcedBy: [ unbound ] }'
bash "$TARGET" --repo-root "$r2" --update-baseline >/dev/null 2>&1
set +e
out="$(bash "$TARGET" --repo-root "$r2" 2>&1)"
rc=$?
set -e
if [[ "$rc" -eq 0 ]] && printf '%s' "$out" | grep -q 'OK - no gate disagrees beyond the recorded baseline'; then
  ok "divergent and contradiction gates both baseline-covered pass"
else
  bad "fully-baselined disagreement passes" "rc=$rc: $out"
fi

# --- 3. ADVERSARIAL: a NEW disagreement beyond the baseline fails ------------
r3="$WORK/r3"
write_registry "$r3" \
  'G010: { enforcedBy: [ script:x.sh ], blocking: blocking, blockingBasis: script-exit-nonzero, agreement: divergent, declaredEnforcedBy: [ guard-check:1 ] }'
bash "$TARGET" --repo-root "$r3" --update-baseline >/dev/null 2>&1
# Now a previously-clean gate starts disagreeing too -- this is the case a
# careless generator re-run or a hand edit could introduce silently.
write_registry "$r3" \
  'G010: { enforcedBy: [ script:x.sh ], blocking: blocking, blockingBasis: script-exit-nonzero, agreement: divergent, declaredEnforcedBy: [ guard-check:1 ] }' \
  'G099: { enforcedBy: [ script:z.sh ], blocking: blocking, blockingBasis: script-exit-nonzero, agreement: divergent, declaredEnforcedBy: [ guard-check:9 ] }'
set +e
out="$(bash "$TARGET" --repo-root "$r3" 2>&1)"
rc=$?
set -e
if [[ "$rc" -eq 1 ]] && printf '%s' "$out" | grep -q 'G099' && ! printf '%s' "$out" | grep -qE '^\s*G010\s*$'; then
  ok "a new disagreement not in the baseline fails, naming only the new id"
else
  bad "new disagreement beyond baseline fails" "rc=$rc: $out"
fi

# --- 4. a baselined id that stops disagreeing is reported as stale -----------
r4="$WORK/r4"
write_registry "$r4" \
  'G010: { enforcedBy: [ script:x.sh ], blocking: blocking, blockingBasis: script-exit-nonzero, agreement: divergent, declaredEnforcedBy: [ guard-check:1 ] }' \
  'G011: { enforcedBy: [ behavioral:agents/y.md ], blocking: unknown, blockingBasis: behavioral-only-no-derivable-exit, agreement: contradiction, declaredEnforcedBy: [ unbound ] }'
bash "$TARGET" --repo-root "$r4" --update-baseline >/dev/null 2>&1
# G011 got fixed: its declared field now matches evidence.
write_registry "$r4" \
  'G010: { enforcedBy: [ script:x.sh ], blocking: blocking, blockingBasis: script-exit-nonzero, agreement: divergent, declaredEnforcedBy: [ guard-check:1 ] }' \
  'G011: { enforcedBy: [ behavioral:agents/y.md ], blocking: unknown, blockingBasis: behavioral-only-no-derivable-exit, agreement: agrees }'
set +e
out="$(bash "$TARGET" --repo-root "$r4" --verbose 2>&1)"
rc=$?
set -e
if [[ "$rc" -eq 0 ]] && printf '%s' "$out" | grep -q 'no longer disagree' && printf '%s' "$out" | grep -q 'G011'; then
  ok "a fixed baseline entry is reported stale without failing the check"
else
  bad "stale baseline entry reported, check still passes" "rc=$rc: $out"
fi

# --- 5. --update-baseline writes a file that may only shrink, never grow by hand
r5="$WORK/r5"
write_registry "$r5" \
  'G010: { enforcedBy: [ script:x.sh ], blocking: blocking, blockingBasis: script-exit-nonzero, agreement: divergent, declaredEnforcedBy: [ guard-check:1 ] }'
bash "$TARGET" --repo-root "$r5" --update-baseline >/dev/null 2>&1
baseline_file="$r5/bubbles/registry/gate-enforcement-agreement.baseline"
if grep -qxF 'G010' "$baseline_file" 2>/dev/null && ! grep -q 'G099' "$baseline_file" 2>/dev/null; then
  ok "--update-baseline writes exactly the current disagreeing set"
else
  bad "--update-baseline writes exactly the current disagreeing set" "$(cat "$baseline_file" 2>/dev/null)"
fi

# --- 6. ADVERSARIAL: bypass-shaped flags are refused, not silently accepted --
set +e
out="$(bash "$TARGET" --repo-root "$WORK" --force 2>&1)"
rc=$?
set -e
if [[ "$rc" -eq 2 ]] && printf '%s' "$out" | grep -qi 'bypass-shaped flag'; then
  ok "a bypass-shaped flag is refused with a usage error, not accepted"
else
  bad "bypass-shaped flag refused" "rc=$rc: $out"
fi

# --- 7. missing registry is a usage error, never a silent pass --------------
r7="$WORK/r7-missing"
set +e
out="$(bash "$TARGET" --repo-root "$r7" 2>&1)"
rc=$?
set -e
if [[ "$rc" -eq 2 ]]; then
  ok "a missing gate registry is a usage error"
else
  bad "missing gate registry is a usage error" "rc=$rc: $out"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
