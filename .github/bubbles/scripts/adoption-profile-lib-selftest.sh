#!/usr/bin/env bash
# adoption-profile-lib-selftest.sh — one parser, one unknown-value policy.
#
# IMP-042 SCOPE-13 / REG-12.
#
# The parsing helpers were defined up to four times and had already drifted:
# cli.sh's active-profile read piped through `head -1` and developer-profile.sh's
# did not, and cli.sh answered an unrecognised profile with a silent fallback to
# `delivery` while the other two exited 1 on the same input. These assertions pin
# the single behaviour so a copy cannot quietly reappear.

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/adoption-profile-lib.sh"
LABEL="adoption-profile-lib-selftest"

pass_count=0
fail_count=0
pass() {
  printf '[%s] PASS %s\n' "$LABEL" "$*"
  pass_count=$((pass_count + 1))
}
fail() {
  printf '[%s] FAIL %s\n' "$LABEL" "$*" >&2
  fail_count=$((fail_count + 1))
}

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT INT TERM

cat >"$work/profiles.yaml" <<'YAML'
version: 1
profiles:
  foundation:
    label: Foundation
  delivery:
    label: Delivery
  assured:
    label: Assured
YAML

ids="$(bubbles_adoption_profile_ids "$work/profiles.yaml" | tr '\n' ' ')"
if [[ "$ids" == "foundation delivery assured " ]]; then
  pass "ids parse in declaration order"
else
  fail "ids parsed as '$ids'"
fi

if [[ -z "$(bubbles_adoption_profile_ids "$work/absent.yaml")" ]]; then
  pass "a missing registry yields no ids rather than an error"
else
  fail "a missing registry produced output"
fi

# --- The head -1 divergence: two declarations must yield exactly one value ---
printf '{ "adoptionProfile": "assured", "other": 1, "adoptionProfile": "foundation" }\n' >"$work/two.json"
two="$(bubbles_active_adoption_profile "$work/two.json")"
if [[ "$two" == "assured" ]]; then
  pass "a config with two declarations yields the first, on one line"
else
  fail "expected 'assured' from the first declaration, got '$two'"
fi

printf '{ "adoptionProfile": "delivery" }\n' >"$work/one.json"
if [[ "$(bubbles_active_adoption_profile "$work/one.json")" == "delivery" ]]; then
  pass "a single declaration resolves"
else
  fail "a single declaration did not resolve"
fi

printf '{ "somethingElse": true }\n' >"$work/none.json"
if [[ -z "$(bubbles_active_adoption_profile "$work/none.json")" ]]; then
  pass "a config declaring no profile resolves empty"
else
  fail "a config with no profile produced a value"
fi

if bubbles_adoption_profile_is_explicit "$work/none.json"; then
  fail "a config with no adoptionProfile key counted as explicit"
else
  pass "a config with no adoptionProfile key is not explicit"
fi

if bubbles_adoption_profile_is_explicit "$work/one.json"; then
  pass "a config declaring a profile is explicit"
else
  fail "a declared profile did not count as explicit"
fi

# --- Unknown-value policy ---------------------------------------------------
if bubbles_adoption_profile_is_known delivery "$work/profiles.yaml"; then
  pass "a registered profile is known"
else
  fail "a registered profile was not known"
fi

if bubbles_adoption_profile_is_known producton "$work/profiles.yaml"; then
  fail "a typo'd profile was treated as known"
else
  pass "a typo'd profile is not known"
fi

if bubbles_adoption_profile_is_known "" "$work/profiles.yaml"; then
  fail "an empty profile was treated as known"
else
  pass "an empty profile is not known"
fi

# --- No caller may reintroduce a silent fallback -----------------------------
# Execute each caller with a bogus profile. Grepping the callers for the refusal
# string only proved the text existed, not that the path was reachable -- the
# assertion would have passed on dead code. A known profile is exercised too, so
# a caller that simply always fails cannot satisfy this.
bogus="definitely-not-a-profile"
known="$(bubbles_adoption_profile_ids "$SCRIPT_DIR/../adoption-profiles.yaml" | head -1)"

check_caller() {
  local label="$1" expect_rc="$2"
  shift 2
  local out rc
  out="$("$@" 2>&1)"
  rc=$?
  if [[ "$expect_rc" == "nonzero" ]]; then
    if [[ "$rc" -ne 0 ]] && printf '%s\n' "$out" | grep -qi 'unknown adoption profile'; then
      pass "$label refuses an unknown profile (exit $rc)"
    else
      fail "$label did not refuse an unknown profile (exit $rc)"
    fi
  else
    if [[ "$rc" -eq 0 ]]; then
      pass "$label accepts the registered profile '$known'"
    else
      fail "$label rejected the registered profile '$known' (exit $rc)"
    fi
  fi
}

if [[ -n "$known" ]]; then
  check_caller "repo-readiness.sh" nonzero bash "$SCRIPT_DIR/repo-readiness.sh" . --profile "$bogus"
  check_caller "repo-readiness.sh" zero bash "$SCRIPT_DIR/repo-readiness.sh" . --profile "$known"
  check_caller "cli.sh repo-readiness" nonzero bash "$SCRIPT_DIR/cli.sh" repo-readiness . --profile "$bogus"
else
  fail "no registered adoption profiles resolved; cannot exercise callers"
fi

printf '[%s] %s passed, %s failed\n' "$LABEL" "$pass_count" "$fail_count"
[[ "$fail_count" -eq 0 ]] || exit 1
exit 0
