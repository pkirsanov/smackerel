#!/usr/bin/env bash
#
# autonomy-posture-guard-selftest.sh — hermetic proof that Gate G135 has teeth.
#
# Every case pairs a GREEN fixture with a drifted variant. A guard that only
# ever passes is decorative, so each check is proven to FAIL when its invariant
# is violated, not merely to succeed when it holds.

set -uo pipefail

SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"
GUARD="$SCRIPT_DIR/autonomy-posture-guard.sh"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." 2>/dev/null && pwd)"

# Fixture mutation goes through the framework's own portable in-place helper
# instead of a raw `sed -i`, so this selftest behaves identically on GNU and BSD
# (macOS) userland -- the same rule macos-portability-guard.sh enforces.
# shellcheck source=/dev/null
source "$SCRIPT_DIR/guard-lib.sh"

ISSUES=0
TMPS=()
trap '[[ ${#TMPS[@]} -gt 0 ]] && rm -rf "${TMPS[@]}" 2>/dev/null || true' EXIT INT TERM

pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  ISSUES=$((ISSUES + 1))
}

check() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$actual" == "$expected" ]]; then
    pass "$label"
  else
    fail "$label"
    echo "  expected: $expected"
    echo "  actual:   $actual"
  fi
}

# All fixtures live under ONE owned parent rather than eight loose siblings in a
# shared /tmp. Cleanup becomes a single atomic removal, and a stray rm -rf aimed
# at a sibling temp dir cannot pick off individual fixtures mid-run.
FIXTURE_ROOT="$(mktemp -d)"
TMPS+=("$FIXTURE_ROOT")
FIXTURE_N=0

make_fixture() {
  FIXTURE_N=$((FIXTURE_N + 1))
  local root="$FIXTURE_ROOT/fx$FIXTURE_N"
  mkdir -p "$root/agents/bubbles_shared" "$root/bubbles/scripts"
  cp "$REPO_ROOT/agents/bubbles_shared/critical-requirements.md" "$root/agents/bubbles_shared/"
  cp "$REPO_ROOT/bubbles/workflows.yaml" "$root/bubbles/"
  cp "$REPO_ROOT/bubbles/scripts/autonomy-resolve.sh" "$root/bubbles/scripts/"
  chmod +x "$root/bubbles/scripts/autonomy-resolve.sh"
  printf '%s' "$root"
}

guard_rc() {
  local root="$1"
  bash "$GUARD" --repo-root "$root" >/dev/null 2>&1
  echo $?
}

guard_out() {
  local root="$1"
  bash "$GUARD" --repo-root "$root" 2>&1
}

echo "Running autonomy-posture-guard selftest..."
echo "Scenario: the posture surface stays internally consistent, and drift is refused."

# --- Green baseline -----------------------------------------------------------
GREEN="$(make_fixture)"
check "a conformant posture surface passes (exit 0)" "0" "$(guard_rc "$GREEN")"

# --- 1. The floor must survive ------------------------------------------------
NO_FLOOR="$(make_fixture)"
grep -v '^## Autonomy Floor' "$NO_FLOOR/agents/bubbles_shared/critical-requirements.md" \
  >"$NO_FLOOR/agents/bubbles_shared/.tmp" &&
  mv "$NO_FLOOR/agents/bubbles_shared/.tmp" "$NO_FLOOR/agents/bubbles_shared/critical-requirements.md"
check "a deleted Autonomy Floor heading is refused" "1" "$(guard_rc "$NO_FLOOR")"

THIN_FLOOR="$(make_fixture)"
# Drop the floor's numbered items but keep its heading: the section can be
# hollowed out without ever deleting it, which is the subtler regression.
awk '
  /^## Autonomy Floor/ { inf = 1; print; next }
  inf && /^## / { inf = 0 }
  inf && /^[0-9]+\. / { next }
  { print }
' "$THIN_FLOOR/agents/bubbles_shared/critical-requirements.md" >"$THIN_FLOOR/agents/bubbles_shared/.tmp" &&
  mv "$THIN_FLOOR/agents/bubbles_shared/.tmp" "$THIN_FLOOR/agents/bubbles_shared/critical-requirements.md"
check "a hollowed-out floor (heading kept, items removed) is refused" "1" "$(guard_rc "$THIN_FLOOR")"

# --- 2. Enum and resolver may not drift apart ---------------------------------
ENUM_AHEAD="$(make_fixture)"
# A new posture declared in the registry but never taught to the resolver: the
# AUT-2 defect shape, a declaration describing behavior nothing implements.
# The replacement carries a POSIX escaped literal newline rather than `\n`:
# GNU sed expands `\n` in the RHS, BSD sed emits a literal `n`, which would
# leave this fixture undrifted on macOS and fail the assertion for the wrong
# reason.
bubbles_sed_inplace 's|^        interactive: |        supervised: placeholder posture\
        interactive: |' \
  "$ENUM_AHEAD/bubbles/workflows.yaml"
check "an enum value the resolver does not accept is refused" "1" "$(guard_rc "$ENUM_AHEAD")"

RESOLVER_AHEAD="$(make_fixture)"
bubbles_sed_inplace 's|^VALID_AUTONOMY=".*"$|VALID_AUTONOMY="full guarded interactive unattended rogue"|' \
  "$RESOLVER_AHEAD/bubbles/scripts/autonomy-resolve.sh"
check "a resolver value the enum does not declare is refused" "1" "$(guard_rc "$RESOLVER_AHEAD")"

if printf '%s' "$(guard_out "$RESOLVER_AHEAD")" | grep -q 'enum/resolver drift'; then
  pass "drift is reported as drift, naming both sides"
else
  fail "drift finding should name both the enum and the resolver"
fi

# --- 3. `unattended` may not become unbounded ---------------------------------
UNBOUNDED_OK="$(make_fixture)"
bubbles_sed_inplace 's|^    exit 3$|    exit 0|' "$UNBOUNDED_OK/bubbles/scripts/autonomy-resolve.sh"
check "a resolver that stops refusing an unbounded unattended is refused" "1" "$(guard_rc "$UNBOUNDED_OK")"

UNNAMED="$(make_fixture)"
bubbles_sed_inplace 's|E039-UNATTENDED-UNBOUNDED|SOMETHING-WENT-WRONG|' \
  "$UNNAMED/bubbles/scripts/autonomy-resolve.sh"
check "an unbounded refusal that drops its named code is refused" "1" "$(guard_rc "$UNNAMED")"

# --- 4. The bypass prohibition must hold --------------------------------------
BYPASS="$(make_fixture)"
# Make --skip silently accepted instead of refused.
bubbles_sed_inplace '/is not a supported flag/{n;s/exit 2/shift/;}' \
  "$BYPASS/bubbles/scripts/autonomy-resolve.sh"
check "a resolver that accepts a bypass flag is refused" "1" "$(guard_rc "$BYPASS")"

# --- The guard itself offers no bypass ----------------------------------------
for flag in --skip --force --ignore --no-verify; do
  bash "$GUARD" "$flag" >/dev/null 2>&1
  check "the guard refuses bypass flag $flag (exit 2)" "2" "$?"
done

bash "$GUARD" --help >/dev/null 2>&1
check "--help exits 0" "0" "$?"

bash "$GUARD" --bogus-flag >/dev/null 2>&1
check "an unknown flag is a usage error (exit 2)" "2" "$?"

echo
if [[ $ISSUES -eq 0 ]]; then
  echo "autonomy-posture-guard selftest passed."
  exit 0
fi
echo "autonomy-posture-guard selftest failed with $ISSUES issue(s)."
exit 1
