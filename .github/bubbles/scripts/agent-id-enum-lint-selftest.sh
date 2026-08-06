#!/usr/bin/env bash
# bubbles/scripts/agent-id-enum-lint-selftest.sh
#
# Hermetic selftest for agent-id-enum-lint.sh (IMP-036 SCOPE-7).
#
# The load-bearing property is the RATCHET: a baseline exists so thousands of
# historical records do not make the lint unrunnable, but it must NOT be able to
# hide a newly-introduced bad id. Case 4 is the one that matters - it proves the
# lint still fails on a new unknown id while a populated baseline is in force.
# Without that case the baseline would be indistinguishable from an off switch.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/agent-id-enum-lint.sh"
NAME="agent-id-enum-lint-selftest"

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
# shellcheck disable=SC2317  # invoked indirectly by the EXIT/INT/TERM trap
cleanup() { [[ -n "${WORK:-}" && -d "$WORK" ]] && rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

# Hermetic enum: two registered agents, nothing else.
CAPS="$WORK/caps.yaml"
{
  printf 'version: 1\nagents:\n'
  printf '  bubbles.implement:\n    role: build\n'
  printf '  bubbles.workflow:\n    role: orchestrate\n'
} >"$CAPS"

BASE="$WORK/base.txt"
: >"$BASE"

# make_repo <dir> <agent-value>...
make_repo() {
  local dir="$1"; shift
  mkdir -p "$dir/specs/001-x"
  {
    printf '{"status":"done","executionHistory":['
    local first=1 a
    for a in "$@"; do
      [[ $first -eq 1 ]] || printf ','
      first=0
      printf '{"agent":"%s"}' "$a"
    done
    printf ']}\n'
  } >"$dir/specs/001-x/state.json"
}

run_lint() {
  BUBBLES_AGENT_CAPABILITIES_FILE="$CAPS" BUBBLES_AGENT_ID_BASELINE_FILE="$BASE" \
    bash "$TARGET" "$@" 2>&1
}

# --- 1. registered ids pass ---------------------------------------------------
r1="$WORK/r1"; make_repo "$r1" "bubbles.implement" "bubbles.workflow"
out="$(run_lint "$r1")"; rc=$?
if [[ $rc -eq 0 ]]; then ok "registered ids pass"; else bad "registered ids pass" "rc=$rc: $out"; fi

# --- 2. an unregistered id fails with an empty baseline ----------------------
r2="$WORK/r2"; make_repo "$r2" "bubbles.implement" "totally.made.up"
out="$(run_lint "$r2")"; rc=$?
if [[ $rc -eq 1 ]] && printf '%s' "$out" | grep -q 'totally.made.up'; then
  ok "unregistered id fails and is named"
else
  bad "unregistered id fails" "rc=$rc: $out"
fi

# --- 3. a baselined id passes -------------------------------------------------
printf 'legacy.agent\n' >"$BASE"
r3="$WORK/r3"; make_repo "$r3" "bubbles.implement" "legacy.agent"
out="$(run_lint "$r3")"; rc=$?
if [[ $rc -eq 0 ]]; then ok "baselined id passes"; else bad "baselined id passes" "rc=$rc: $out"; fi

# --- 4. ADVERSARIAL: a populated baseline must NOT hide a NEW bad id ---------
# This is the ratchet. If this case ever passes with rc=0 the baseline has
# become an off switch and the lint is decoration.
r4="$WORK/r4"; make_repo "$r4" "legacy.agent" "brand.new.bad.id"
out="$(run_lint "$r4")"; rc=$?
if [[ $rc -eq 1 ]] && printf '%s' "$out" | grep -q 'brand.new.bad.id' &&
  ! printf '%s' "$out" | grep -qE '^  legacy\.agent$'; then
  ok "baseline does NOT hide a new bad id (ratchet holds)"
else
  bad "baseline does not hide a new bad id" "rc=$rc: $out"
fi

# --- 5. trailing qualifier is stripped before the check ----------------------
: >"$BASE"
r5="$WORK/r5"; make_repo "$r5" "bubbles.workflow (parent-expanded bugfix-fastlane)"
out="$(run_lint "$r5")"; rc=$?
if [[ $rc -eq 0 ]] && printf '%s' "$out" | grep -q 'inline "(...)" qualifier'; then
  ok "qualifier stripped to base id, and reported as migration debt"
else
  bad "qualifier stripped and reported" "rc=$rc: $out"
fi

# --- 6. non-agent actors are part of the contract ----------------------------
r6="$WORK/r6"; make_repo "$r6" "manual" "operator" "human"
out="$(run_lint "$r6")"; rc=$?
if [[ $rc -eq 0 ]]; then ok "non-agent actors accepted"; else bad "non-agent actors accepted" "rc=$rc: $out"; fi

# --- 7. stale baseline entries are reported but do not fail ------------------
printf 'no.longer.used\n' >"$BASE"
out="$(run_lint "$r1")"; rc=$?
if [[ $rc -eq 0 ]] && printf '%s' "$out" | grep -q 'no longer observed'; then
  ok "stale baseline entry reported without failing"
else
  bad "stale baseline entry reported" "rc=$rc: $out"
fi

# --- 8. ADVERSARIAL: bypass-shaped flags are refused -------------------------
: >"$BASE"
bypass_ok=1
for flag in --skip --force --ignore --no-verify --bypass --allow-once; do
  out="$(run_lint "$r2" "$flag" 2>&1)"; rc=$?
  if [[ $rc -ne 2 ]]; then bypass_ok=0; bad "bypass flag '$flag' refused" "rc=$rc"; break; fi
done
[[ $bypass_ok -eq 1 ]] && ok "every bypass-shaped flag exits 2"

# --- 9. no target is a usage error (no default surface) ----------------------
out="$(BUBBLES_AGENT_CAPABILITIES_FILE="$CAPS" bash "$TARGET" 2>&1)"; rc=$?
if [[ $rc -eq 2 ]]; then ok "missing target exits 2 (no default surface)"; else bad "missing target exits 2" "rc=$rc"; fi

# --- 10. --update-baseline records exactly the unknown ids -------------------
run_lint "$r2" --update-baseline >/dev/null 2>&1
if grep -qx 'totally.made.up' "$BASE" 2>/dev/null && ! grep -qx 'bubbles.implement' "$BASE" 2>/dev/null; then
  ok "--update-baseline records unknown ids only"
else
  bad "--update-baseline records unknown ids only" "$(tr '\n' '|' <"$BASE" 2>/dev/null)"
fi

# --- 11. counts render as numbers, not doubled zeros -------------------------
: >"$BASE"
out="$(run_lint "$r1" --verbose)"
if ! printf '%s' "$out" | grep -qE 'invalid number|^0$'; then
  ok "counts render cleanly with an empty baseline"
else
  bad "counts render cleanly" "$out"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
