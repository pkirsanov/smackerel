#!/usr/bin/env bash
#
# bubbles spec-dashboard-selftest.sh
#
# Pins two behaviours of spec-dashboard.sh that were both silently wrong:
#
#   1. `grep -c` PRINTS 0 on no-match AND exits 1, so the old `|| echo "0"`
#      appended a second line. The variable became "0\n0" and `[[ -gt ]]` failed
#      with a syntax error, leaving the scope count wrong for that spec.
#
#   2. is-terminal-for-mode.sh exits 1 for "not terminal" but 2 for "mode is not
#      in the registry". Collapsing both loses a real signal. Note that status
#      "done" short-circuits BEFORE the mode is consulted, so the fixture below
#      uses a non-done terminal status — otherwise the case would pass for the
#      wrong reason and never exercise the unresolvable-mode path.
#
#   3. certification.status was extracted with a sed that replaced only the
#      matched span, so `"status": "done",` yielded `done,` — matching no case
#      arm and falling into "other". On one consuming repo that understated
#      completion as 45% when the true figure was 89%.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH="$SCRIPT_DIR/spec-dashboard.sh"

failures=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# A spec whose scopes.md contains ZERO "Status:" lines — the input that made
# grep -c emit "0\n0" through the old fallback.
mk_spec() {
  local root="$1" name="$2" status="$3" mode="$4" scopes_body="$5"
  mkdir -p "$root/specs/$name"
  printf '%s' "$scopes_body" >"$root/specs/$name/scopes.md"
  cat >"$root/specs/$name/state.json" <<EOF
{ "specId": "$name", "status": "$status", "workflowMode": "$mode" }
EOF
}

# A spec whose status lives under certification, with the trailing comma that
# broke the old extraction.
mk_certified_spec() {
  local root="$1" name="$2" status="$3" mode="$4"
  mkdir -p "$root/specs/$name"
  printf '# Scopes\n\n- Status: Done\n' >"$root/specs/$name/scopes.md"
  cat >"$root/specs/$name/state.json" <<EOF
{
  "specId": "$name",
  "workflowMode": "$mode",
  "certification": {
    "status": "$status",
    "certifiedBy": "bubbles.validate"
  }
}
EOF
}

root="$work/repo"
mkdir -p "$root/specs"
mk_spec "$root" "001-no-status-lines" "done" "full-delivery" $'# Scopes\n\nNo status markers here at all.\n'
mk_spec "$root" "002-invented-mode" "validated" "totally-invented-mode-xyz" $'# Scopes\n\n- Status: Done\n'
mk_certified_spec "$root" "003-certified-done" "done" "full-delivery"

set +e
out="$(cd "$root" && bash "$DASH" 2>&1)"
rc=$?
set -e

# --- Case 1: no "Status:" lines must not produce a shell syntax error ---------
if ! grep -q "syntax error in expression" <<<"$out"; then
  pass "a scopes.md with zero Status: lines does not trigger a grep -c syntax error"
else
  fail "grep -c no-match still yields a two-line count"
  echo "$out"
fi

# --- Case 2: an unregistered mode is reported, not silently swallowed ---------
if grep -q "not in the registry" <<<"$out" && grep -q "totally-invented-mode-xyz" <<<"$out"; then
  pass "a workflowMode absent from the registry is surfaced with its spec name"
else
  fail "an unregistered workflowMode must be reported, not counted as unfinished"
  echo "$out"
fi

# --- Case 3: a registry-valid mode is NOT flagged (adversarial pair) ----------
# Without this, a rule that flagged everything would pass case 2.
if ! grep -q "001-no-status-lines (full-delivery)" <<<"$out"; then
  pass "a mode that resolves is not reported as unregistered"
else
  fail "a resolvable mode must never be listed as unregistered"
  echo "$out"
fi

if [[ "$rc" -gt 1 ]]; then
  fail "spec-dashboard exited $rc on a well-formed fixture"
fi

# --- Case 4: certification.status must not carry the trailing JSON comma ------
# `"status": "done",` previously became `done,`, matched no case arm, and was
# silently counted as "other" — understating completion across the portfolio.
if grep -qE '^\s+Specs:.*1 done' <<<"$out" || grep -qE '[0-9]+ done' <<<"$out"; then
  if ! grep -qE 'done,' <<<"$out"; then
    pass "a certification.status is parsed without the trailing JSON comma"
  else
    fail "certification.status still carries a trailing comma"
    echo "$out"
  fi
else
  fail "expected a done count in the summary"
  echo "$out"
fi

if [[ "$failures" -eq 0 ]]; then
  echo "[spec-dashboard-selftest] OK"
else
  echo "[spec-dashboard-selftest] $failures failed"
  exit 1
fi
