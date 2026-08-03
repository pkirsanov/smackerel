#!/usr/bin/env bash
# state-certification-reconcile-selftest.sh — hermetic coverage for
# state-certification-reconcile.sh (IMP-032 SCOPE-4b).
#
# The tool's value is entirely in what it REFUSES, so most cases here assert a
# refusal and then assert the state file is byte-identical afterwards.
#
# The PASS path is exercised without weakening production: the script resolves
# its guard as "$SCRIPT_DIR/state-transition-guard.sh", so a copy of the script
# placed next to a stub guard yields a hermetic PASS. No environment override
# exists in the shipped tool, because an override that can fake a PASS verdict
# would reintroduce the forging vector the tool is built to close.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RECONCILE="$SCRIPT_DIR/state-certification-reconcile.sh"
REAL_GUARD="$SCRIPT_DIR/state-transition-guard.sh"

for required in "$RECONCILE" "$REAL_GUARD"; do
  if [[ ! -f "$required" ]]; then
    printf 'state-certification-reconcile-selftest: required surface missing: %s\n' "$required" >&2
    exit 2
  fi
done

if ! command -v jq >/dev/null 2>&1; then
  echo "state-certification-reconcile-selftest: SKIP (jq not installed)"
  exit 0
fi

selftest_tmp_base="${TMPDIR:-$HOME/.cache}"
mkdir -p "$selftest_tmp_base"
WORKSPACE="$(mktemp -d "$selftest_tmp_base/bubbles-cert-reconcile-selftest.XXXXXX")"
cleanup() {
  rm -rf "$WORKSPACE"
}
trap cleanup EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0

pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  printf 'PASS: %s\n' "$1"
}

fail_test() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  printf 'FAIL: %s\n' "$1" >&2
}

# write_spec <spec-dir> <status> <cert-json|-> ; creates state.json
write_spec() {
  local dir="$1" status="$2" cert="$3"
  mkdir -p "$dir"
  local cert_arg='null'
  [[ "$cert" == "-" ]] || cert_arg="$cert"
  jq -n \
    --arg status "$status" \
    --argjson certification "$cert_arg" \
    '{version: 3, status: $status, workflowMode: "full-delivery"}
     | if $certification == null then . else .certification = $certification end' \
    > "$dir/state.json"
}

# stub_harness <dir> <verdict> <exit-code> — a copy of the tool beside a stub guard
stub_harness() {
  local dir="$1" verdict="$2" code="$3"
  mkdir -p "$dir"
  cp "$RECONCILE" "$dir/state-certification-reconcile.sh"
  cat > "$dir/state-transition-guard.sh" <<EOF
#!/usr/bin/env bash
cat <<'BLOCK'
BEGIN TRANSITION_GUARD_RESULT_V1
verdict: $verdict
blockingCode: E009-STUBBED
END TRANSITION_GUARD_RESULT_V1
BLOCK
exit $code
EOF
  chmod +x "$dir/state-transition-guard.sh"
  printf '%s' "$dir/state-certification-reconcile.sh"
}

# --- usage and input validation -------------------------------------------

if bash "$RECONCILE" --help >/dev/null 2>&1; then
  pass "--help exits 0"
else
  fail_test "--help should exit 0"
fi

rc=0; bash "$RECONCILE" >/dev/null 2>&1 || rc=$?
[[ "$rc" -eq 2 ]] && pass "missing target exits 2" || fail_test "missing target should exit 2 (got $rc)"

rc=0; bash "$RECONCILE" --bogus /tmp >/dev/null 2>&1 || rc=$?
[[ "$rc" -eq 2 ]] && pass "unknown option exits 2" || fail_test "unknown option should exit 2 (got $rc)"

rc=0; bash "$RECONCILE" "$WORKSPACE/nope/state.json" >/dev/null 2>&1 || rc=$?
[[ "$rc" -eq 2 ]] && pass "missing state.json exits 2" || fail_test "missing state.json should exit 2 (got $rc)"

malformed="$WORKSPACE/malformed"
mkdir -p "$malformed"
printf 'not json' > "$malformed/state.json"
rc=0; bash "$RECONCILE" "$malformed" >/dev/null 2>&1 || rc=$?
[[ "$rc" -eq 2 ]] && pass "malformed JSON exits 2" || fail_test "malformed JSON should exit 2 (got $rc)"

# --- nothing to reconcile --------------------------------------------------

nocert="$WORKSPACE/nocert"
write_spec "$nocert" "done" "-"
rc=0; bash "$RECONCILE" "$nocert" >/dev/null 2>&1 || rc=$?
[[ "$rc" -eq 2 ]] && pass "absent certification block is refused, not synthesized" || fail_test "absent certification should exit 2 (got $rc)"

agree="$WORKSPACE/agree"
write_spec "$agree" "done" '{"status":"done"}'
agree_before="$(cat "$agree/state.json")"
rc=0; bash "$RECONCILE" "$agree" >/dev/null 2>&1 || rc=$?
[[ "$rc" -eq 0 ]] && pass "agreeing mirrors exit 0" || fail_test "agreeing mirrors should exit 0 (got $rc)"
[[ "$(cat "$agree/state.json")" == "$agree_before" ]] && pass "agreeing mirrors leave the file untouched" || fail_test "agreeing mirrors must not rewrite the file"

# --- ownership -------------------------------------------------------------

owned="$WORKSPACE/owned"
write_spec "$owned" "done" '{"status":"not_started"}'
owned_before="$(cat "$owned/state.json")"
rc=0; env -u BUBBLES_AGENT_NAME bash "$RECONCILE" "$owned" --apply >/dev/null 2>&1 || rc=$?
[[ "$rc" -eq 2 ]] && pass "--apply without the validate role exits 2" || fail_test "--apply without ownership should exit 2 (got $rc)"
[[ "$(cat "$owned/state.json")" == "$owned_before" ]] && pass "--apply without ownership writes nothing" || fail_test "--apply without ownership must not write"

rc=0; BUBBLES_AGENT_NAME=bubbles.implement bash "$RECONCILE" "$owned" --apply >/dev/null 2>&1 || rc=$?
[[ "$rc" -eq 2 ]] && pass "--apply from a non-validate agent exits 2" || fail_test "non-validate agent should exit 2 (got $rc)"

# --- refusal path (stubbed BLOCKED guard) ----------------------------------

blocked_dir="$WORKSPACE/blockedharness"
BLOCKED_TOOL="$(stub_harness "$blocked_dir" "BLOCKED" 2)"
blockedspec="$WORKSPACE/blockedspec"
write_spec "$blockedspec" "done" '{"status":"not_started"}'
blocked_before="$(cat "$blockedspec/state.json")"
out="$WORKSPACE/blocked.out"
rc=0; BUBBLES_AGENT_NAME=bubbles.validate bash "$BLOCKED_TOOL" "$blockedspec" --apply >"$out" 2>&1 || rc=$?
[[ "$rc" -eq 3 ]] && pass "a non-passing guard yields the dedicated refusal code 3" || fail_test "non-passing guard should exit 3 (got $rc)"
[[ "$(cat "$blockedspec/state.json")" == "$blocked_before" ]] && pass "refusal restores state.json byte-for-byte" || fail_test "refusal must restore the original state.json"
grep -q 'E009-STUBBED' "$out" && pass "refusal surfaces the guard blockingCode" || fail_test "refusal should report the guard blockingCode"
grep -qi 'bubbles.validate' "$out" && pass "refusal names the routing owner" || fail_test "refusal should route to bubbles.validate"

# --- pass path (stubbed PASS guard) ----------------------------------------

pass_dir="$WORKSPACE/passharness"
PASS_TOOL="$(stub_harness "$pass_dir" "PASS" 0)"

dryspec="$WORKSPACE/dryspec"
write_spec "$dryspec" "done" '{"status":"not_started"}'
dry_before="$(cat "$dryspec/state.json")"
dryout="$WORKSPACE/dry.out"
rc=0; bash "$PASS_TOOL" "$dryspec" >"$dryout" 2>/dev/null || rc=$?
[[ "$rc" -eq 0 ]] && pass "passing guard in dry-run exits 0" || fail_test "passing guard dry-run should exit 0 (got $rc)"
[[ "$(cat "$dryspec/state.json")" == "$dry_before" ]] && pass "dry-run is the default and writes nothing" || fail_test "dry-run must not write"
[[ "$(jq -r '.certification.status' "$dryout")" == "done" ]] && pass "dry-run prints the reconciled candidate" || fail_test "dry-run should print the candidate with certification.status advanced"
[[ "$(jq -r 'has("certifiedAt") or (.certification | has("certifiedAt"))' "$dryout")" == "false" ]] && pass "candidate never invents certifiedAt" || fail_test "candidate must not invent certifiedAt"

applyspec="$WORKSPACE/applyspec"
write_spec "$applyspec" "done" '{"status":"not_started"}'
rc=0; BUBBLES_AGENT_NAME=bubbles.validate bash "$PASS_TOOL" "$applyspec" --apply >/dev/null 2>&1 || rc=$?
[[ "$rc" -eq 0 ]] && pass "--apply after a PASS verdict exits 0" || fail_test "--apply after PASS should exit 0 (got $rc)"
[[ "$(jq -r '.certification.status' "$applyspec/state.json")" == "done" ]] && pass "--apply advances certification.status" || fail_test "--apply should advance certification.status"
[[ "$(jq -r '.status' "$applyspec/state.json")" == "done" ]] && pass "--apply never lowers top-level status" || fail_test "--apply must leave top-level status intact"
[[ "$(jq -r '.certification | has("certifiedAt")' "$applyspec/state.json")" == "false" ]] && pass "--apply never fabricates certifiedAt" || fail_test "--apply must not fabricate certifiedAt"

preserve="$WORKSPACE/preserve"
write_spec "$preserve" "done" '{"status":"not_started","certifiedAt":"2026-01-01T00:00:00Z"}'
rc=0; BUBBLES_AGENT_NAME=bubbles.validate bash "$PASS_TOOL" "$preserve" --apply >/dev/null 2>&1 || rc=$?
[[ "$(jq -r '.certification.certifiedAt' "$preserve/state.json")" == "2026-01-01T00:00:00Z" ]] && pass "an existing certifiedAt is preserved verbatim" || fail_test "existing certifiedAt must be preserved"

# --- real guard integration ------------------------------------------------

realspec="$WORKSPACE/specs/013-real-divergence"
write_spec "$realspec" "done" '{"status":"not_started"}'
real_before="$(cat "$realspec/state.json")"
rc=0; BUBBLES_AGENT_NAME=bubbles.validate bash "$RECONCILE" "$realspec" --apply >/dev/null 2>&1 || rc=$?
[[ "$rc" -eq 3 ]] && pass "the real guard refuses an unearned status" || fail_test "real guard should refuse an unearned status (got $rc)"
[[ "$(cat "$realspec/state.json")" == "$real_before" ]] && pass "the real-guard path restores state.json byte-for-byte" || fail_test "real-guard path must restore the original state.json"

printf '\nstate-certification-reconcile-selftest: %d passed, %d failed\n' "$PASS_COUNT" "$FAIL_COUNT"
[[ "$FAIL_COUNT" -eq 0 ]] || exit 1
exit 0
