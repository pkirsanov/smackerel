#!/usr/bin/env bash
# bubbles/scripts/orphaned-scaffolding-guard-selftest.sh
#
# Hermetic selftest for orphaned-scaffolding-guard.sh and the
# `bubbles_removal_precondition_met` helper it enforces (IMP-047 S-A).
#
# Every case below is adversarial in the sense that matters: it passes only
# because the rule is implemented. Revert the guard to a no-op and cases 2, 3
# and 6 fail; revert the helper to `return 0` and case 1 fails; widen the
# scanner to raw text matching and cases 4 and 5 fail.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/orphaned-scaffolding-guard.sh"
GUARD_LIB="$SCRIPT_DIR/guard-lib.sh"
NAME="orphaned-scaffolding-guard-selftest"

for required in "$GUARD" "$GUARD_LIB"; do
  if [[ ! -f "$required" ]]; then
    printf '%s: required surface missing: %s\n' "$NAME" "$required" >&2
    exit 2
  fi
done

failures=0
checks=0

ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1))
  failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
  return 0
}

WORK="$(mktemp -d 2>/dev/null)" || {
  printf '%s: cannot create temp dir\n' "$NAME" >&2
  exit 1
}
# shellcheck disable=SC2317  # invoked indirectly by the EXIT/INT/TERM trap
cleanup() { [[ -n "${WORK:-}" && -d "$WORK" ]] && rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

# Build a fixture repo carrying only the surfaces the guard reads.
new_fixture() {
  local root="$WORK/$1"
  mkdir -p "$root/bubbles/scripts"
  cp "$GUARD" "$root/bubbles/scripts/orphaned-scaffolding-guard.sh"
  cp "$GUARD_LIB" "$root/bubbles/scripts/guard-lib.sh"
  printf '%s' "$root"
}

run_guard() {
  local root="$1"
  shift
  set +e
  bash "$GUARD" --repo-root "$root" "$@" >"$WORK/out" 2>"$WORK/err"
  GUARD_RC=$?
  set -e
  return "$GUARD_RC"
}

# --- 1. the helper can never report success ----------------------------------
# This is the property the whole rule rests on. If the helper is ever softened
# to `return 0`, an obsolete lint goes green again and nothing else here matters.
# shellcheck source=/dev/null
source "$GUARD_LIB"
set +e
helper_stderr="$(bubbles_removal_precondition_met "demo-lint" "its registry is empty" 2>&1 >/dev/null)"
helper_rc=$?
set -e
if [[ "$helper_rc" -eq 3 ]] &&
  printf '%s' "$helper_stderr" | grep -q 'REMOVAL PRECONDITION MET: its registry is empty' &&
  printf '%s' "$helper_stderr" | grep -q 'must be deleted, not scheduled'; then
  ok "bubbles_removal_precondition_met returns 3 and names the obsolete check"
else
  bad "helper returns 3 and names the obsolete check" "rc=$helper_rc out=$helper_stderr"
fi

# --- 2. ADVERSARIAL: the gates-block-reader shape is refused -----------------
# This reproduces the exact lines the retired lint printed before exiting 0.
shape="$(new_fixture gates-block-reader-shape)"
cat >"$shape/bubbles/scripts/legacy-inventory-lint.sh" <<'FIXTURE'
#!/usr/bin/env bash
NAME="legacy-inventory-lint"
printf '%s: nothing can depend on it; the block is safe to remove.\n' "$NAME"
printf '%s: the inventory is empty; SCOPE-13 removal precondition is met.\n' "$NAME"
exit 0
FIXTURE
if ! run_guard "$shape"; then
  rc="$GUARD_RC"
  if [[ "$rc" -eq 1 ]] &&
    grep -q 'FINDING: unrouted-removal-precondition: bubbles/scripts/legacy-inventory-lint.sh' "$WORK/out"; then
    ok "a lint that declares its own removal precondition and exits 0 is refused"
  else
    bad "orphaned lint shape is refused" "rc=$rc out=$(tr '\n' '|' <"$WORK/out")"
  fi
else
  bad "orphaned lint shape is refused" "guard exited 0 on the retired lint's own output"
fi

# --- 3. every unrouted declaration is reported, not just the first -----------
if [[ "$(grep -c 'FINDING: unrouted-removal-precondition' "$WORK/out")" == "2" ]]; then
  ok "every unrouted declaration is reported, not only the first"
else
  bad "all unrouted declarations reported" "$(grep -c 'FINDING' "$WORK/out") finding(s)"
fi

# --- 4. ADVERSARIAL: prose about the rule is not an emission of it -----------
# Without comment stripping this guard's own doc comments, the changelog and
# every explanatory header become findings, which is how a guard becomes noise.
prose="$(new_fixture prose-only)"
cat >"$prose/bubbles/scripts/documented-lint.sh" <<'FIXTURE'
#!/usr/bin/env bash
# This lint explains that a check reporting "removal precondition is met"
# must fail. It is safe to remove nothing here; the inventory is empty only
# in the comment above, which is prose and not an emission.
printf 'documented-lint: OK\n'
exit 0
FIXTURE
if run_guard "$prose"; then
  ok "a comment describing the rule is not treated as a declaration"
else
  bad "comments are not declarations" "rc=$GUARD_RC out=$(tr '\n' '|' <"$WORK/out")"
fi

# --- 5. ADVERSARIAL: a routed declaration is compliant, not a finding --------
# The rule requires ROUTING, not silence. A lint that correctly hands the
# declaration to the helper must pass, or the only way to satisfy the guard
# would be to stop reporting obsolescence at all -- the opposite of the intent.
routed="$(new_fixture routed)"
cat >"$routed/bubbles/scripts/honest-lint.sh" <<'FIXTURE'
#!/usr/bin/env bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/guard-lib.sh"
bubbles_removal_precondition_met "honest-lint" "the inventory is empty" || exit $?
exit 0
FIXTURE
if run_guard "$routed"; then
  ok "a declaration routed through the helper is compliant"
else
  bad "routed declaration is compliant" "rc=$GUARD_RC out=$(tr '\n' '|' <"$WORK/out")"
fi

# The routed fixture must still be visible as routed, so the guard reports what
# it counted rather than silently finding nothing.
if run_guard "$routed" --list && grep -q 'routed: bubbles/scripts/honest-lint.sh' "$WORK/out"; then
  ok "--list names the routed declaration it counted"
else
  bad "--list names routed declarations" "$(tr '\n' '|' <"$WORK/out")"
fi

# --- 6. ADVERSARIAL: a routed lint genuinely stops being green --------------
# The guard passing the routed shape is only correct if the routed shape itself
# refuses. Otherwise the rule would be satisfiable by routing to a no-op.
set +e
bash "$routed/bubbles/scripts/honest-lint.sh" >/dev/null 2>&1
honest_rc=$?
set -e
if [[ "$honest_rc" -eq 3 ]]; then
  ok "the routed lint itself exits non-zero instead of reporting PASS"
else
  bad "routed lint exits non-zero" "exit was $honest_rc"
fi

# --- 7. no bypass flag exists ------------------------------------------------
bypass_ok=1
for flag in --skip --force --ignore --allow-once; do
  set +e
  bash "$GUARD" --repo-root "$shape" "$flag" >/dev/null 2>&1
  rc=$?
  set -e
  if [[ "$rc" -ne 2 ]]; then
    bad "guard refuses the bypass flag $flag" "exit was $rc"
    bypass_ok=0
  fi
done
[[ "$bypass_ok" -eq 1 ]] && ok "every bypass-shaped flag is refused with a usage error"

# --- 8. a missing scripts directory is an environment error, not a PASS ------
empty="$WORK/empty-root"
mkdir -p "$empty"
set +e
bash "$GUARD" --repo-root "$empty" >/dev/null 2>&1
rc=$?
set -e
if [[ "$rc" -eq 2 ]]; then
  ok "an unreadable scan root exits 2 rather than reporting zero findings"
else
  bad "unreadable scan root exits 2" "exit was $rc"
fi

printf '\n%s: %d/%d checks passed\n' "$NAME" "$((checks - failures))" "$checks"
if [[ "$failures" -gt 0 ]]; then
  printf '%s: FAILED\n' "$NAME"
  exit 1
fi
printf '%s: OK\n' "$NAME"
exit 0
