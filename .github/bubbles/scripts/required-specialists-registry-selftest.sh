#!/usr/bin/env bash
# required-specialists-registry-selftest.sh — prove Check 6's registry read.
#
# IMP-042 SCOPE-13 / REG-10.
#
# state-transition-guard.sh Check 6 (Gate G022) used to carry the
# mode -> required-specialist table as a hardcoded case that
# required-specialists.yaml mirrored, with a shadow comparator holding the two in
# step. The guard now reads the registry directly, so the comparator is gone and
# the risk moved: the registry read itself must be correct.
#
# Two parsers back that read -- yq, and a bounded awk fallback for a minimal
# PATH. A silent disagreement between them would change G022 enforcement
# depending on what happens to be installed, so the central assertion here is
# that they agree for EVERY mode.

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REGISTRY="$SCRIPT_DIR/../registry/required-specialists.yaml"
GUARD="$SCRIPT_DIR/state-transition-guard.sh"
LABEL="required-specialists-registry-selftest"

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

if [[ ! -f "$REGISTRY" ]]; then
  fail "registry missing at $REGISTRY"
  printf '[%s] %s passed, %s failed\n' "$LABEL" "$pass_count" "$fail_count"
  exit 1
fi

# The awk fallback exactly as the guard embeds it.
awk_lookup() {
  awk -v want="$1" '
    /^modes:[[:space:]]*$/ { in_modes = 1; next }
    /^[a-zA-Z]/ { in_modes = 0 }
    in_modes == 1 && $0 ~ /^[[:space:]]+[a-z0-9][a-z0-9-]*:[[:space:]]*\[/ {
      key = $0
      sub(/^[[:space:]]*/, "", key)
      sub(/:.*$/, "", key)
      if (key != want) next
      val = $0
      sub(/^[^[]*\[/, "", val)
      sub(/\].*$/, "", val)
      gsub(/,/, " ", val)
      gsub(/"/, "", val)
      gsub(/[[:space:]]+/, " ", val)
      sub(/^ /, "", val)
      sub(/ $/, "", val)
      print val
    }
  ' "$2"
}

mode_keys="$(awk '
  /^modes:[[:space:]]*$/ { in_modes = 1; next }
  /^[a-zA-Z]/ { in_modes = 0 }
  in_modes == 1 && $0 ~ /^[[:space:]]+[a-z0-9][a-z0-9-]*:[[:space:]]*\[/ {
    key = $0; sub(/^[[:space:]]*/, "", key); sub(/:.*$/, "", key); print key
  }
' "$REGISTRY")"
mode_count="$(printf '%s\n' "$mode_keys" | grep -c . || true)"

if [[ "$mode_count" -ge 20 ]]; then
  pass "registry parses ($mode_count modes)"
else
  fail "registry parsed only $mode_count mode(s); expected the full delivery table"
fi

# --- The load-bearing assertion: both parsers agree for every mode ----------
if command -v yq >/dev/null 2>&1; then
  disagreements=0
  while IFS= read -r mode; do
    [[ -n "$mode" ]] || continue
    yq_val="$(_rs_mode="$mode" yq -r '.modes[strenv(_rs_mode)] // [] | join(" ")' "$REGISTRY" 2>/dev/null || true)"
    awk_val="$(awk_lookup "$mode" "$REGISTRY")"
    if [[ "$yq_val" != "$awk_val" ]]; then
      fail "parser disagreement for '$mode': yq='$yq_val' awk='$awk_val'"
      disagreements=$((disagreements + 1))
    fi
  done <<<"$mode_keys"
  if [[ "$disagreements" -eq 0 ]]; then
    pass "yq and the awk fallback agree for all $mode_count modes"
  fi
else
  printf '[%s] SKIP parser agreement (yq not installed)\n' "$LABEL"
fi

# --- A known mode resolves to its exact ordered list ------------------------
full_delivery="$(awk_lookup full-delivery "$REGISTRY")"
expected_full="implement test regression simplify gaps harden stabilize security validate audit chaos docs"
if [[ "$full_delivery" == "$expected_full" ]]; then
  pass "full-delivery resolves to its exact ordered specialist list"
else
  fail "full-delivery resolved to '$full_delivery'"
fi

# --- An unknown mode resolves empty, so the phaseOrder fallback engages -----
unknown="$(awk_lookup definitely-not-a-mode "$REGISTRY")"
if [[ -z "$unknown" ]]; then
  pass "an unregistered mode resolves empty and falls through to derivation"
else
  fail "an unregistered mode resolved to '$unknown'"
fi

# --- The guard must actually read the registry, not a reinstated table ------
if grep -q 'required-specialists.yaml' "$GUARD"; then
  pass "Check 6 reads the registry"
else
  fail "Check 6 no longer references the registry"
fi
if grep -qE '^    (full-delivery|bugfix-fastlane|value-first-e2e-batch)\)$' "$GUARD"; then
  fail "a hardcoded mode->specialist case arm was reinstated in the guard"
else
  pass "no hardcoded mode->specialist table remains in the guard"
fi

# --- Adversarial: a truncated list must be visible to the parser ------------
probe="$(mktemp)"
sed 's/^  full-delivery: .*/  full-delivery: [ implement ]/' "$REGISTRY" >"$probe"
probe_val="$(awk_lookup full-delivery "$probe")"
if [[ "$probe_val" == "implement" ]]; then
  pass "a changed registry list changes what the parser returns"
else
  fail "parser ignored a changed registry list (returned '$probe_val')"
fi
rm -f "$probe"

printf '[%s] %s passed, %s failed\n' "$LABEL" "$pass_count" "$fail_count"
[[ "$fail_count" -eq 0 ]] || exit 1
exit 0
