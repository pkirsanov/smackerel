#!/usr/bin/env bash
# File: framework-validate-tier-selftest.sh
#
# Hermetic-ish selftest for the IMP-012 framework-validate tiering. Uses the
# `--list-tier` DRY-LIST mode (no checks execute) so it is fast and non-circular.
# Proves: a known core check is WOULD-RUN under --list-tier=core; a known
# non-core check is WOULD-SKIP under core but WOULD-RUN under full; both exit 0;
# and an unknown flag exits 2.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FV="$SCRIPT_DIR/framework-validate.sh"
BASH_BIN="$(command -v bash)"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/guard-lib.sh"

failures=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

# The contention fixture belongs to this selftest, not to whichever outer
# validator or evidence harness invoked it. Keep the EXIT cleanup armed before
# the fixture can be created; INT/TERM convert to an exit status and therefore
# take the same source-owned cleanup path.
lock_root=""
lock_fd_open="no"
cleanup_tier_selftest() {
  if [[ "$lock_fd_open" == "yes" ]]; then
    exec 8>&-
    lock_fd_open="no"
  fi
  if [[ -n "$lock_root" ]]; then
    rm -rf "$lock_root"
    lock_root=""
  fi
}
trap cleanup_tier_selftest EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Force source mode so the self-only checks list rather than emit their
# install-mode SKIP, keeping the listing deterministic.
export BUBBLES_FRAMEWORK_VALIDATE_MODE=source

set +e
core_list="$("$BASH_BIN" "$FV" --list-tier=core 2>&1)"
core_rc=$?
full_list="$("$BASH_BIN" "$FV" --list-tier=full 2>&1)"
full_rc=$?
set -e

# --- core tier: a known fast check runs, a known heavy check is skipped --------
if [[ "$core_rc" -eq 0 ]]; then
  pass "--list-tier=core exits 0 without executing checks"
else
  fail "--list-tier=core should exit 0 (got $core_rc)"
  echo "$core_list" | tail -5
fi
if grep -qE '^WOULD-RUN:.*Registry consistency' <<<"$core_list"; then
  pass "core tier WOULD-RUN a fast structural check (Registry consistency)"
else
  fail "core tier should run the Registry consistency check"
fi
if grep -qE '^WOULD-RUN:.*Scan-lib' <<<"$core_list"; then
  pass "core tier WOULD-RUN the scan-lib selftest"
else
  fail "core tier should run the scan-lib selftest"
fi
if grep -qE '^WOULD-SKIP \(non-core\):.*Finding closure selftest' <<<"$core_list"; then
  pass "core tier WOULD-SKIP a non-core check (Finding closure selftest)"
else
  fail "core tier should skip the non-core Finding closure selftest"
fi

# --- full tier: everything runs (the previously-skipped check now runs) --------
if [[ "$full_rc" -eq 0 ]] \
  && grep -qE '^WOULD-RUN:.*Finding closure selftest' <<<"$full_list" \
  && ! grep -qE '^WOULD-SKIP' <<<"$full_list"; then
  pass "full tier WOULD-RUN every check (no WOULD-SKIP lines)"
else
  fail "full tier should run every check with no skips (got rc=$full_rc)"
  grep -E '^WOULD-SKIP' <<<"$full_list" | head -3
fi

# --- unknown flag → exit 2 ----------------------------------------------------
set +e
"$BASH_BIN" "$FV" --bogus-flag >/dev/null 2>&1
bad_rc=$?
set -e
[[ "$bad_rc" -eq 2 ]] \
  && pass "an unknown framework-validate flag exits 2" \
  || fail "unknown flag should exit 2 (got $bad_rc)"

# --- IMP-049 SCOPE-3 / STAB-002: portable lock behavior -----------------------
# The lock exists to stop two runs corrupting each other's shared scratch
# fixtures. An invocation that executes no check builds none, so it must not
# wait. Before SCOPE-3 both assertions below returned exit 1 with a lock error.
# The third assertion is the safety property and matters more than the first
# two: an EXECUTING run must still contend, or the bypass has been widened into
# a hole. It is gated on flock because without it the lock degrades to a no-op
# and `--tier=core` would really execute (~260s) instead of refusing.
if command -v flock >/dev/null 2>&1; then
  # Own a private contention lock synchronously in this shell. The private
  # namespace keeps a nested selftest independent from an outer validator's
  # global lock, while closing descriptors 8 and 9 in each probe prevents the
  # child from inheriting either lock and making its own contention vacuous.
  lock_root="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-framework-validate-tier.XXXXXXXX")"
  lock_file="$lock_root/bubbles-framework-validate.lock"
  : >"$lock_file"
  exec 8>"$lock_file"
  lock_fd_open="yes"
  if flock -n 8; then
    set +e
    # A nested run inherits the outer validator's marker and bypasses the lock
    # legitimately, which would make these assertions vacuous. Clear it and
    # point the probes at the private namespace so only this synchronous lock
    # decides the contention result.
    held_help_out="$(TMPDIR="$lock_root" env -u BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD bash "$FV" --help 8>&- 9>&- 2>&1)"
    held_help_rc=$?
    held_list_out="$(TMPDIR="$lock_root" env -u BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD bash "$FV" --list-tier=core 8>&- 9>&- 2>&1)"
    held_list_rc=$?
    TMPDIR="$lock_root" env -u BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD bash "$FV" --tier=core 8>&- 9>&- >/dev/null 2>&1
    held_exec_rc=$?
    set -e

    [[ "$held_help_rc" -eq 0 && "$held_help_out" == *"Usage: framework-validate.sh"* ]] \
      && pass "--help answers while another run holds the lock" \
      || fail "--help should not block on the lock (got $held_help_rc)"

    [[ "$held_list_rc" -eq 0 && "$held_list_out" == *"WOULD-"* ]] \
      && pass "--list-tier dry-lists while another run holds the lock" \
      || fail "--list-tier should not block on the lock (got $held_list_rc)"

    [[ "$held_exec_rc" -eq 1 ]] \
      && pass "an EXECUTING run still contends for the lock" \
      || fail "executing run must still be refused while the lock is held (got $held_exec_rc)"
  else
    fail "tier selftest should acquire its private contention lock"
  fi
  cleanup_tier_selftest
else
  pass "flock absent — lock-bypass assertions skipped (protection is a no-op here)"
fi

# --- IMP-049 SCOPE-3: the lock pre-scan must not drift from the parser --------
# The pre-scan names the flags that execute checks; the parser below it names
# every flag it accepts. If someone adds an executing flag to the parser and
# forgets the pre-scan, that flag silently stops taking the lock and two real
# runs can corrupt each other's fixtures. Deriving both sets from the file keeps
# the duplication honest instead of trusting a comment.
prescan_exec="$(awk '/^_fv_executes_checks=true$/,/^fi$/' "$FV" \
  | grep -E '^[[:space:]]+--tier=core \|' \
  | sed 's/).*//' \
  | grep -oE '\-\-[a-z-]+(=[a-z]+)?' | sort -u)"
# Read only the case-arm LABELS (text before the closing paren). Taking whole
# lines also swallows `${_arg#--tier=}` from an arm's body, which looks like a
# bare `--tier` flag that no arm actually accepts.
parser_exec="$(awk '/^for _arg in "\$@"; do$/,/^done$/' "$FV" \
  | grep -E '^[[:space:]]+--' \
  | sed 's/).*//' \
  | grep -oE '\-\-[a-z-]+(=[a-z]+)?' \
  | grep -vE '^--(help|list-tier)' | sort -u)"

[[ -n "$prescan_exec" && "$prescan_exec" == "$parser_exec" ]] \
  && pass "lock pre-scan lists exactly the parser's executing flags" \
  || fail "lock pre-scan drifted from the parser (prescan=[${prescan_exec//$'\n'/,}] parser=[${parser_exec//$'\n'/,}])"

# --- BUG-037: live G128 caller preserves exact identity and child status ------
# Run a copied production framework-validate entrypoint. A PATH shim returns
# success for unrelated bash checks, but executes the copied production
# session-cap-guard.sh for the live G128 row. This keeps the assertion on the
# real caller path without recursively running the complete framework suite.
fv_g128_root="$(cd "$(mktemp -d)" && pwd -P)"
fv_g128_real_bash="$(command -v bash)"
fv_g128_real_env="$(command -v env)"
fv_g128_shim="$fv_g128_root/shim"
fv_g128_binding_root="$(cd "$(mktemp -d)" && pwd -P)"
fv_g128_control_file="$fv_g128_binding_root/repository-binding.json"
fv_g128_packet_file="$fv_g128_binding_root/actionable-packet.json"
mkdir -p "$fv_g128_root/bubbles" "$fv_g128_root/.specify/memory" "$fv_g128_root/agents" "$fv_g128_shim"
cp -R "$SCRIPT_DIR/../." "$fv_g128_root/bubbles/"
cp "$SCRIPT_DIR/../../VERSION" "$fv_g128_root/VERSION"
cp "$SCRIPT_DIR/../../install.sh" "$fv_g128_root/install.sh"
cp "$SCRIPT_DIR/../../agents/bubbles.workflow.agent.md" "$fv_g128_root/agents/bubbles.workflow.agent.md"
chmod 700 "$fv_g128_root/install.sh" "$fv_g128_root/bubbles/scripts/cli.sh"
git -C "$fv_g128_root" init -q
git -C "$fv_g128_root" config user.name "Bubbles Fixture"
git -C "$fv_g128_root" config user.email "fixture@example.invalid"
git -C "$fv_g128_root" add VERSION install.sh bubbles/scripts/cli.sh agents/bubbles.workflow.agent.md
git -C "$fv_g128_root" commit -q -m "fixture repository"
fv_g128_entry="$fv_g128_root/bubbles/scripts/framework-validate.sh"
fv_g128_guard="$fv_g128_root/bubbles/scripts/session-cap-guard.sh"
fv_g128_guard_baseline="$fv_g128_root/session-cap-guard.baseline.sh"
cp "$fv_g128_guard" "$fv_g128_guard_baseline"

set +e
fv_g128_preflight_output="$(bash "$fv_g128_root/bubbles/scripts/repository-binding.sh" preflight \
  --session-id host-current \
  --session-control-file "$fv_g128_control_file" \
  --expected-control-revision 0 \
  --request-class TARGETLESS_MODE \
  --workspace-root "$fv_g128_root" \
  --repository-root "$fv_g128_root" 2>&1)"
fv_g128_preflight_rc=$?
set -e
if [[ "$fv_g128_preflight_rc" -ne 0 ]]; then
  printf 'framework-validate-tier-selftest: BUG-037 binding preflight failed (exit=%s)\n%s\n' \
    "$fv_g128_preflight_rc" "$fv_g128_preflight_output" >&2
  exit 2
fi
fv_g128_packet_json=""
while IFS= read -r fv_g128_preflight_line; do
  case "$fv_g128_preflight_line" in
    \{*) fv_g128_packet_json="$fv_g128_preflight_line" ;;
  esac
done <<<"$fv_g128_preflight_output"
if [[ -z "$fv_g128_packet_json" ]]; then
  printf 'framework-validate-tier-selftest: BUG-037 preflight emitted no actionable packet\n' >&2
  exit 2
fi
printf '%s\n' "$fv_g128_packet_json" >"$fv_g128_packet_file"
chmod 600 "$fv_g128_packet_file"

cat > "$fv_g128_shim/bash" <<EOF
#!$fv_g128_real_bash
set -euo pipefail
if [[ "\${1:-}" == "$fv_g128_guard" ]]; then
  exec "$fv_g128_real_bash" "\$@"
fi
exit 0
EOF
chmod 700 "$fv_g128_shim/bash"

cat > "$fv_g128_shim/env" <<EOF
#!$fv_g128_real_bash
set -euo pipefail
for arg in "\$@"; do
  if [[ "\$arg" == "$fv_g128_guard" ]]; then
    exec "$fv_g128_real_env" "\$@"
  fi
done
exit 0
EOF
chmod 700 "$fv_g128_shim/env"

run_fv_g128_case() {
  local case_name="$1"
  local session_id="$2"
  local tier="${3:-full}"
  local log_file="$fv_g128_root/$case_name.log"
  local rc
  set +e
  PATH="$fv_g128_shim:$PATH" \
    BUBBLES_FRAMEWORK_VALIDATE_MODE=downstream \
    BUBBLES_FRAMEWORK_VALIDATE_LOCK_HELD=1 \
    BUBBLES_FRAMEWORK_VALIDATE_DEPTH=1 \
    BUBBLES_SESSION_ID="$session_id" \
    BUBBLES_SESSION_CONTROL_FILE="$fv_g128_control_file" \
    BUBBLES_BINDING_PACKET_FILE="$fv_g128_packet_file" \
    "$fv_g128_real_bash" "$fv_g128_entry" "--tier=$tier" >"$log_file" 2>&1
  rc=$?
  set -e
  printf '%s' "$rc"
}

show_fv_g128_failure_log() {
  local case_name="$1"
  printf '%s\n' "--- BUG-037 framework live G128 $case_name log ---"
  if ! grep -n -E 'Session cap guard \(live, G128\)|G128|Framework check' "$fv_g128_root/$case_name.log"; then
    printf '%s\n' 'no live G128 diagnostics found'
  fi
  printf '%s\n' "--- end BUG-037 framework live G128 $case_name log ---"
}

cat > "$fv_g128_root/.specify/memory/bubbles.session.json" <<'JSON'
{
  "sessionBudget": { "maxTotalConvergenceIterations": 1 },
  "sessionBudgetHistory": [{
    "recordSchemaVersion": 1,
    "hostSessionId": "host-current",
    "revision": 1,
    "supersedesRevision": null,
    "recordedAt": "2026-09-01T00:00:00Z",
    "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 2 }
  }],
  "convergenceLoops": [
    { "hostSessionId": "host-old", "specDir": "specs/old", "agent": "bubbles.goal", "iterationCount": 99 },
    { "hostSessionId": "host-current", "specDir": "specs/current", "agent": "bubbles.goal", "iterationCount": 1 }
  ]
}
JSON
for fv_g128_tier in core full; do
  fv_g128_pass_case="pass-$fv_g128_tier"
  fv_g128_pass_rc="$(run_fv_g128_case "$fv_g128_pass_case" host-current "$fv_g128_tier")"
  if [[ "$fv_g128_pass_rc" -eq 0 ]] &&
    grep -Fq 'G128 status=PASS exit=0 session="host-current"' "$fv_g128_root/$fv_g128_pass_case.log" &&
    grep -Fq 'Framework check PASS G128 status=PASS exit=0 source=guard' "$fv_g128_root/$fv_g128_pass_case.log" &&
    grep -Fq 'PASS: Session cap guard (live, G128)' "$fv_g128_root/$fv_g128_pass_case.log"; then
    pass "BUG-037 framework $fv_g128_tier tier forwards one exact host session to live G128"
  else
    fail "BUG-037 framework $fv_g128_tier tier should pass host-current only (exit=$fv_g128_pass_rc)"
    show_fv_g128_failure_log "$fv_g128_pass_case"
  fi
done

cat > "$fv_g128_root/.specify/memory/bubbles.session.json" <<'JSON'
{
  "sessionBudgetHistory": [{
    "recordSchemaVersion": 1,
    "hostSessionId": "host-current",
    "revision": 1,
    "supersedesRevision": null,
    "recordedAt": "2026-09-01T00:00:00Z",
    "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 2 }
  }],
  "convergenceLoops": [
    { "hostSessionId": "host-current", "specDir": "specs/current", "agent": "bubbles.goal", "iterationCount": 3 }
  ]
}
JSON
fv_g128_breach_rc="$(run_fv_g128_case breach host-current)"
if [[ "$fv_g128_breach_rc" -eq 1 ]] &&
  grep -Fq 'G128 status=BREACH exit=1 session="host-current"' "$fv_g128_root/breach.log" &&
  grep -Fq 'Framework check FAIL G128 status=BREACH exit=1 source=guard' "$fv_g128_root/breach.log" &&
  ! grep -Fq 'G128 INPUT-ERROR' "$fv_g128_root/breach.log"; then
  pass "BUG-037 framework validation preserves child exit 1 as BREACH"
else
  fail "BUG-037 framework live G128 should preserve BREACH/1 (exit=$fv_g128_breach_rc)"
  show_fv_g128_failure_log breach
fi

cat > "$fv_g128_root/.specify/memory/bubbles.session.json" <<'JSON'
{
  "sessionBudgetHistory": [{
    "recordSchemaVersion": 1,
    "hostSessionId": "host-current",
    "revision": 1,
    "supersedesRevision": null,
    "recordedAt": "2026-09-01T00:00:00Z",
    "budget": { "schemaVersion": 1, "maxTotalConvergenceIterations": 2 }
  }],
  "convergenceLoops": [
    { "hostSessionId": "host-current", "specDir": "specs/current", "agent": "bubbles.goal", "iterationCount": 1 }
  ]
}
JSON
fv_g128_input_rc="$(run_fv_g128_case input-error '')"
if [[ "$fv_g128_input_rc" -eq 1 ]] &&
  grep -Fq 'Framework check FAIL G128 status=INPUT-ERROR exit=2 source=caller reason=invalid-authority' "$fv_g128_root/input-error.log" &&
  ! grep -Eq '^G128 status=' "$fv_g128_root/input-error.log"; then
  pass "BUG-037 framework validation rejects missing authority before G128 invocation"
else
  fail "BUG-037 framework live G128 should reject missing authority inside aggregate failure (exit=$fv_g128_input_rc)"
  show_fv_g128_failure_log input-error
fi

fv_g128_invocation_sentinel="$fv_g128_root/g128-invoked"
cat >"$fv_g128_guard" <<EOF
#!/usr/bin/env bash
printf 'invoked\n' >>"$fv_g128_invocation_sentinel"
printf '%s\n' 'G128 status=PASS exit=0 session="host-other"'
exit 0
EOF
chmod 700 "$fv_g128_guard"
rm -f "$fv_g128_invocation_sentinel"
fv_g128_session_mismatch_rc="$(run_fv_g128_case session-mismatch host-other)"
if [[ "$fv_g128_session_mismatch_rc" -eq 1 ]] &&
  grep -Fq 'Framework check FAIL G128 status=INPUT-ERROR exit=2 source=caller reason=invalid-authority' "$fv_g128_root/session-mismatch.log" &&
  [[ ! -e "$fv_g128_invocation_sentinel" ]]; then
  pass "BUG-037 framework validation rejects packet/session mismatch before G128 invocation"
else
  fail "BUG-037 framework live G128 should reject packet/session mismatch before invocation (exit=$fv_g128_session_mismatch_rc)"
  show_fv_g128_failure_log session-mismatch
fi

stage_fv_g128_output_case() {
  local case_name="$1"
  case "$case_name" in
    empty)
      cat >"$fv_g128_guard" <<EOF
#!/usr/bin/env bash
printf 'invoked\n' >>"$fv_g128_invocation_sentinel"
printf '%s\n' 'diagnostic-without-final'
exit 0
EOF
      ;;
    duplicate)
      cat >"$fv_g128_guard" <<EOF
#!/usr/bin/env bash
printf 'invoked\n' >>"$fv_g128_invocation_sentinel"
printf '%s\n' 'G128 status=PASS exit=0 session="host-current"'
printf '%s\n' 'G128 status=PASS exit=0 session="host-current"'
exit 0
EOF
      ;;
    contradictory)
      cat >"$fv_g128_guard" <<EOF
#!/usr/bin/env bash
printf 'invoked\n' >>"$fv_g128_invocation_sentinel"
printf '%s\n' 'G128 status=PASS exit=1 session="host-current"'
exit 1
EOF
      ;;
    malformed)
      cat >"$fv_g128_guard" <<EOF
#!/usr/bin/env bash
printf 'invoked\n' >>"$fv_g128_invocation_sentinel"
printf '%s\n' 'G128 status=PASS exit=0 session="host-current" extra=bad'
exit 0
EOF
      ;;
    unknown)
      cat >"$fv_g128_guard" <<EOF
#!/usr/bin/env bash
printf 'invoked\n' >>"$fv_g128_invocation_sentinel"
printf '%s\n' 'G128 status=UNKNOWN exit=0 session="host-current"'
exit 0
EOF
      ;;
    mismatched)
      cat >"$fv_g128_guard" <<EOF
#!/usr/bin/env bash
printf 'invoked\n' >>"$fv_g128_invocation_sentinel"
printf '%s\n' 'G128 status=PASS exit=0 session="host-other"'
exit 0
EOF
      ;;
  esac
  chmod 700 "$fv_g128_guard"
}

for fv_g128_output_case in empty duplicate contradictory malformed unknown mismatched; do
  stage_fv_g128_output_case "$fv_g128_output_case"
  rm -f "$fv_g128_invocation_sentinel"
  fv_g128_output_rc="$(run_fv_g128_case "output-$fv_g128_output_case" host-current)"
  if [[ "$fv_g128_output_rc" -eq 1 ]] &&
    grep -Fq 'Framework check FAIL G128 status=INPUT-ERROR exit=2 source=caller reason=invalid-child-result' "$fv_g128_root/output-$fv_g128_output_case.log" &&
    ! grep -Eq '^G128 status=' "$fv_g128_root/output-$fv_g128_output_case.log" &&
    [[ -f "$fv_g128_invocation_sentinel" ]] &&
    [[ "$(wc -l <"$fv_g128_invocation_sentinel")" -eq 1 ]]; then
    pass "BUG-037 framework validation rejects $fv_g128_output_case child output after one invocation"
  else
    fail "BUG-037 framework live G128 should reject $fv_g128_output_case child output (exit=$fv_g128_output_rc)"
    show_fv_g128_failure_log "output-$fv_g128_output_case"
  fi
done

cat >"$fv_g128_guard" <<EOF
#!/usr/bin/env bash
printf 'invoked\n' >>"$fv_g128_invocation_sentinel"
printf '%s\n' 'G128 status=INPUT-ERROR exit=2 session="host-current" reason=fixture-input'
exit 2
EOF
chmod 700 "$fv_g128_guard"
rm -f "$fv_g128_invocation_sentinel"
fv_g128_valid_input_rc="$(run_fv_g128_case valid-input-error host-current)"
if [[ "$fv_g128_valid_input_rc" -eq 1 ]] &&
  grep -Fq 'G128 status=INPUT-ERROR exit=2 session="host-current" reason=fixture-input' "$fv_g128_root/valid-input-error.log" &&
  grep -Fq 'Framework check FAIL G128 status=INPUT-ERROR exit=2 source=guard' "$fv_g128_root/valid-input-error.log"; then
  pass "BUG-037 framework validation preserves a valid child INPUT-ERROR and exit 2 pair"
else
  fail "BUG-037 framework live G128 should preserve valid INPUT-ERROR/2 (exit=$fv_g128_valid_input_rc)"
  show_fv_g128_failure_log valid-input-error
fi

stage_fv_g128_availability_case() {
  local case_name="$1"
  rm -rf "$fv_g128_guard"
  case "$case_name" in
    missing) ;;
    symlink) ln -s "$fv_g128_guard_baseline" "$fv_g128_guard" ;;
    non-regular) mkdir "$fv_g128_guard" ;;
    non-executable)
      cp "$fv_g128_guard_baseline" "$fv_g128_guard"
      chmod 600 "$fv_g128_guard"
      ;;
    replaced)
      cat >"$fv_g128_guard" <<EOF
#!/usr/bin/env bash
cp "$fv_g128_guard_baseline" session-cap-guard.replacement
chmod 700 session-cap-guard.replacement
mv session-cap-guard.replacement session-cap-guard.sh
printf 'invoked\n' >>"$fv_g128_invocation_sentinel"
printf '%s\n' 'G128 status=PASS exit=0 session="host-current"'
exit 0
EOF
      chmod 700 "$fv_g128_guard"
      ;;
  esac
}

for fv_g128_availability_case in missing symlink non-regular non-executable replaced; do
  stage_fv_g128_availability_case "$fv_g128_availability_case"
  rm -f "$fv_g128_invocation_sentinel"
  fv_g128_availability_rc="$(run_fv_g128_case "availability-$fv_g128_availability_case" host-current)"
  if [[ "$fv_g128_availability_rc" -eq 1 ]] &&
    grep -Fq 'Framework check FAIL G128 status=INPUT-ERROR exit=2 source=caller reason=guard-unavailable' "$fv_g128_root/availability-$fv_g128_availability_case.log" &&
    ! grep -Eq '^G128 status=' "$fv_g128_root/availability-$fv_g128_availability_case.log"; then
    pass "BUG-037 framework validation rejects a $fv_g128_availability_case G128 guard"
  else
    fail "BUG-037 framework live G128 should reject a $fv_g128_availability_case guard (exit=$fv_g128_availability_rc)"
    show_fv_g128_failure_log "availability-$fv_g128_availability_case"
  fi
done

rm -rf "$fv_g128_root" "$fv_g128_binding_root"

if [[ "$failures" -eq 0 ]]; then
  echo "[framework-validate-tier-selftest] OK"
else
  echo "[framework-validate-tier-selftest] $failures failed"
  exit 1
fi
