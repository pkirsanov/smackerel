#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNTIME_SCRIPT="$SCRIPT_DIR/runtime-leases.sh"
SOURCE_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TMP_ROOT="$(mktemp -d)"
TEST_ROOT="$TMP_ROOT/runtime-selftest-repo"
DOWNSTREAM_ROOT="$TMP_ROOT/runtime-downstream-repo"
CAP_ROOT="$TMP_ROOT/runtime-capacity-repo"
FAILURES=0

cleanup() {
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT INT TERM

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}

mkdir -p "$TEST_ROOT/.specify/memory" "$TEST_ROOT/.specify/runtime"

cat > "$TEST_ROOT/.specify/memory/bubbles.config.json" <<'EOF'
{
  "version": 2,
  "defaults": {
    "grill": { "mode": "off", "source": "repo-default" },
    "tdd": { "mode": "scenario-first", "defaultForModes": ["bugfix-fastlane", "chaos-hardening"], "source": "repo-default" },
    "autoCommit": { "mode": "off", "source": "repo-default" },
    "lockdown": { "default": false, "requireGrillForInvalidation": true, "source": "repo-default" },
    "regression": { "immutability": "protected-scenarios", "source": "repo-default" },
    "validation": { "certificationRequired": true, "source": "repo-default" },
    "runtime": { "leaseTtlMinutes": 20, "staleAfterMinutes": 60, "reusePolicy": "fingerprint-match-only", "source": "repo-default" }
  },
  "modeOverrides": {},
  "metrics": { "enabled": false, "activityTrackingEnabled": false }
}
EOF

cat > "$TEST_ROOT/.specify/memory/bubbles.session.json" <<'EOF'
{
  "sessionId": "workflow-session-a"
}
EOF

echo "Running runtime lease selftest..."

quoted_session_id='workflow-session-"quoted"-a'

acquire_one_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" BUBBLES_SESSION_ID="$quoted_session_id" BUBBLES_AGENT_NAME="bubbles.validate" bash "$RUNTIME_SCRIPT" acquire --purpose validation --environment dev --share-mode shared-compatible --fingerprint-input schema:v1 --resource container:test-a)"
lease_one="$(printf '%s\n' "$acquire_one_output" | sed -nE 's/leaseId=([^ ]+).*/\1/p')"
if [[ -n "$lease_one" ]]; then
  pass "Acquire returns a lease id"
else
  fail "Acquire should return a lease id"
fi

acquire_two_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" BUBBLES_SESSION_ID="workflow-session-b" BUBBLES_AGENT_NAME="bubbles.chaos" bash "$RUNTIME_SCRIPT" acquire --purpose validation --environment dev --share-mode shared-compatible --fingerprint-input schema:v1 --resource container:test-b)"
lease_two="$(printf '%s\n' "$acquire_two_output" | sed -nE 's/leaseId=([^ ]+).*/\1/p')"
if [[ "$lease_one" == "$lease_two" ]]; then
  pass "Compatible shared runtime is reused across sessions"
else
  fail "Compatible shared runtime should be reused"
fi

lookup_shared_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" bash "$RUNTIME_SCRIPT" lookup --compose-project "$(printf '%s\n' "$acquire_two_output" | sed -nE 's/.*composeProject=([^ ]+).*/\1/p')")"
if printf '%s\n' "$lookup_shared_output" | grep -Fq "attachedSessions=${quoted_session_id},workflow-session-b"; then
  pass "Lookup reports both attached shared-runtime sessions"
else
  fail "Lookup should report both attached shared-runtime sessions"
fi

acquire_three_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" BUBBLES_SESSION_ID="workflow-session-c" bash "$RUNTIME_SCRIPT" acquire --purpose validation --environment dev --share-mode shared-compatible --fingerprint-input schema:v2 --resource container:test-c)"
lease_three="$(printf '%s\n' "$acquire_three_output" | sed -nE 's/leaseId=([^ ]+).*/\1/p')"
if [[ -n "$lease_three" && "$lease_three" != "$lease_one" ]]; then
  pass "Incompatible shared runtime gets a new lease"
else
  fail "Incompatible shared runtime should not reuse the original lease"
fi

exclusive_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" BUBBLES_SESSION_ID="workflow-session-d" bash "$RUNTIME_SCRIPT" acquire --purpose smoke --environment dev --share-mode exclusive --fingerprint-input smoke:v1)"
exclusive_lease="$(printf '%s\n' "$exclusive_output" | sed -nE 's/leaseId=([^ ]+).*/\1/p')"
if [[ -n "$exclusive_lease" ]]; then
  pass "Exclusive runtime can be acquired"
else
  fail "Exclusive runtime acquisition should succeed"
fi

if BUBBLES_REPO_ROOT="$TEST_ROOT" BUBBLES_SESSION_ID="workflow-session-e" bash "$RUNTIME_SCRIPT" acquire --purpose smoke --environment dev --share-mode exclusive --fingerprint-input smoke:v1 >/dev/null 2>&1; then
  fail "Exclusive runtime should block concurrent acquisition"
else
  pass "Exclusive runtime blocks concurrent acquisition"
fi

stale_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" BUBBLES_SESSION_ID="workflow-session-f" bash "$RUNTIME_SCRIPT" acquire --purpose stale-check --environment dev --share-mode exclusive --ttl-minutes 0 --fingerprint-input stale:v1)"
stale_lease="$(printf '%s\n' "$stale_output" | sed -nE 's/leaseId=([^ ]+).*/\1/p')"
if [[ -n "$stale_lease" ]]; then
  pass "Zero-TTL lease created for stale detection"
else
  fail "Zero-TTL lease should still be created"
fi

BUBBLES_REPO_ROOT="$TEST_ROOT" bash "$RUNTIME_SCRIPT" reclaim-stale >/dev/null
doctor_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" bash "$RUNTIME_SCRIPT" doctor || true)"
if printf '%s\n' "$doctor_output" | grep -q 'stale=1'; then
  pass "Doctor reports stale leases"
else
  fail "Doctor should report stale leases"
fi

if printf '%s\n' "$doctor_output" | grep -q 'conflicts=1'; then
  pass "Doctor reports runtime conflicts"
else
  fail "Doctor should report runtime conflicts"
fi

detach_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" bash "$RUNTIME_SCRIPT" release "$lease_one" --session-id workflow-session-b)"
if printf '%s\n' "$detach_output" | grep -q 'Detached session from runtime lease'; then
  pass "Shared runtime release detaches a non-owner session"
else
  fail "Shared runtime release should detach a non-owner session"
fi

lookup_after_detach="$(BUBBLES_REPO_ROOT="$TEST_ROOT" bash "$RUNTIME_SCRIPT" lookup --lease-id "$lease_one")"
if printf '%s\n' "$lookup_after_detach" | grep -q 'status=active' && printf '%s\n' "$lookup_after_detach" | grep -Fq "attachedSessions=${quoted_session_id}"; then
  pass "Lease stays active while another shared session remains attached"
else
  fail "Lease should stay active after detaching one shared session"
fi

shared_release_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" bash "$RUNTIME_SCRIPT" release "$lease_one" --session-id "$quoted_session_id")"
if printf '%s\n' "$shared_release_output" | grep -q 'Released runtime lease'; then
  pass "Last attached shared session releases the lease"
else
  fail "Last attached shared session should release the lease"
fi

release_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" bash "$RUNTIME_SCRIPT" release "$exclusive_lease")"
if printf '%s\n' "$release_output" | grep -q 'Released runtime lease'; then
  pass "Release marks a lease released"
else
  fail "Release should mark the lease released"
fi

summary_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" bash "$RUNTIME_SCRIPT" summary)"
if printf '%s\n' "$summary_output" | grep -q 'active='; then
  pass "Summary reports aggregate runtime counts"
else
  fail "Summary should report aggregate runtime counts"
fi

# ---------------------------------------------------------------------------
# Resource-weighted admission (runtime.capacityWeight)
# ---------------------------------------------------------------------------

# Backward-compat: the existing TEST_ROOT config carries NO runtime.capacityWeight,
# so it resolves to 0 (admission DISABLED). Two heavy leases whose combined weight
# (8 + 8 = 16) would blow a budget of 10 BOTH acquire, proving the gate is a pure
# no-op until an operator configures a budget. This would only change if a default
# budget were wrongly introduced.
bc_heavy_one_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" BUBBLES_SESSION_ID="bc-heavy-1" BUBBLES_AGENT_NAME="bubbles.validate" bash "$RUNTIME_SCRIPT" acquire --purpose bc-heavy-a --environment dev --share-mode exclusive --weight heavy --fingerprint-input bc:a)"
bc_heavy_one="$(printf '%s\n' "$bc_heavy_one_output" | sed -nE 's/leaseId=([^ ]+).*/\1/p')"
bc_heavy_two_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" BUBBLES_SESSION_ID="bc-heavy-2" BUBBLES_AGENT_NAME="bubbles.validate" bash "$RUNTIME_SCRIPT" acquire --purpose bc-heavy-b --environment dev --share-mode exclusive --weight heavy --fingerprint-input bc:b)"
bc_heavy_two="$(printf '%s\n' "$bc_heavy_two_output" | sed -nE 's/leaseId=([^ ]+).*/\1/p')"
if [[ -n "$bc_heavy_one" && -n "$bc_heavy_two" && "$bc_heavy_one" != "$bc_heavy_two" ]]; then
  pass "Backward-compat: two heavy leases both acquire when capacityWeight is unset (admission disabled)"
else
  fail "Backward-compat: two heavy leases should both acquire when capacityWeight is unset"
fi

if printf '%s\n' "$bc_heavy_one_output" | grep -q 'weight=8'; then
  pass "Weight field persists on the lease record (weight=8) even with admission disabled"
else
  fail "Weight field should persist on the lease record even with admission disabled"
fi

bc_summary_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" bash "$RUNTIME_SCRIPT" summary)"
if printf '%s\n' "$bc_summary_output" | grep -q 'Runtime capacity: disabled'; then
  pass "Summary reports capacity disabled when capacityWeight is 0/unset"
else
  fail "Summary should report capacity disabled when capacityWeight is 0/unset"
fi

# Isolated capacity-enabled repo (capacityWeight=10, explicit default weightClasses).
mkdir -p "$CAP_ROOT/.specify/memory" "$CAP_ROOT/.specify/runtime"
cat > "$CAP_ROOT/.specify/memory/bubbles.config.json" <<'EOF'
{
  "version": 2,
  "defaults": {
    "runtime": { "leaseTtlMinutes": 20, "staleAfterMinutes": 60, "reusePolicy": "fingerprint-match-only", "capacityWeight": 10, "weightClasses": { "light": 1, "medium": 4, "heavy": 8 }, "source": "repo-default" }
  },
  "modeOverrides": {},
  "metrics": { "enabled": false, "activityTrackingEnabled": false }
}
EOF

# Case 1: heavy (8) under budget 10 -> succeeds and records weight=8.
cap_h1_output="$(BUBBLES_REPO_ROOT="$CAP_ROOT" BUBBLES_SESSION_ID="cap-s1" BUBBLES_AGENT_NAME="bubbles.validate" bash "$RUNTIME_SCRIPT" acquire --purpose cap-build --environment dev --share-mode exclusive --weight heavy --fingerprint-input cap:a)"
cap_h1="$(printf '%s\n' "$cap_h1_output" | sed -nE 's/leaseId=([^ ]+).*/\1/p')"
if [[ -n "$cap_h1" ]] && printf '%s\n' "$cap_h1_output" | grep -q 'weight=8'; then
  pass "Heavy lease acquires under the capacity budget and records weight=8"
else
  fail "Heavy lease should acquire under the capacity budget with weight=8"
fi

# Case 2 (ADVERSARIAL): a SECOND heavy (8) on a DIFFERENT purpose must be REFUSED
# purely by capacity (8 + 8 = 16 > 10). The different purpose means the
# exclusive-purpose guard does NOT fire, so ONLY the weighted-admission gate can
# block it. If the gate were removed this acquire would wrongly SUCCEED (exit 0,
# no message) and this assertion would FAIL.
cap_refuse_out="$(BUBBLES_REPO_ROOT="$CAP_ROOT" BUBBLES_SESSION_ID="cap-s2" BUBBLES_AGENT_NAME="bubbles.validate" bash "$RUNTIME_SCRIPT" acquire --purpose cap-other --environment dev --share-mode exclusive --weight heavy --fingerprint-input cap:b 2>&1)" && cap_refuse_rc=0 || cap_refuse_rc=$?
if [[ "$cap_refuse_rc" -ne 0 ]] && printf '%s\n' "$cap_refuse_out" | grep -q 'Runtime capacity exceeded'; then
  pass "ADVERSARIAL: second heavy lease is refused by weighted admission (capacity exceeded)"
else
  fail "ADVERSARIAL: second heavy lease should be refused with 'Runtime capacity exceeded' (got rc=$cap_refuse_rc)"
fi

# The refused acquire must NOT have created a lease; capacity stays at 8/10.
cap_after_refuse_summary="$(BUBBLES_REPO_ROOT="$CAP_ROOT" bash "$RUNTIME_SCRIPT" summary)"
if printf '%s\n' "$cap_after_refuse_summary" | grep -q 'Runtime capacity: 8/10 weight units'; then
  pass "Refused acquire creates no lease; summary capacity stays 8/10"
else
  fail "Refused acquire should not create a lease; summary capacity should stay 8/10"
fi

# Case 3: release the holder -> capacity frees -> a new heavy (8) acquires.
BUBBLES_REPO_ROOT="$CAP_ROOT" bash "$RUNTIME_SCRIPT" release "$cap_h1" >/dev/null
cap_h3_output="$(BUBBLES_REPO_ROOT="$CAP_ROOT" BUBBLES_SESSION_ID="cap-s3" bash "$RUNTIME_SCRIPT" acquire --purpose cap-build2 --environment dev --share-mode exclusive --weight heavy --fingerprint-input cap:c)"
cap_h3="$(printf '%s\n' "$cap_h3_output" | sed -nE 's/leaseId=([^ ]+).*/\1/p')"
if [[ -n "$cap_h3" ]]; then
  pass "Releasing the active heavy lease frees capacity for a new heavy lease"
else
  fail "A new heavy lease should acquire after the holder releases capacity"
fi
BUBBLES_REPO_ROOT="$CAP_ROOT" bash "$RUNTIME_SCRIPT" release "$cap_h3" >/dev/null

# Case 4: a STALE heavy lease must NOT consume capacity (the orphan-hang analog).
# A dead session's expired heavy lease frees its budget via the stale downgrade
# even though the record still exists.
cap_stale_output="$(BUBBLES_REPO_ROOT="$CAP_ROOT" BUBBLES_SESSION_ID="cap-stale" bash "$RUNTIME_SCRIPT" acquire --purpose cap-stale --environment dev --share-mode exclusive --weight heavy --ttl-minutes 0 --fingerprint-input cap:stale)"
cap_stale_lease="$(printf '%s\n' "$cap_stale_output" | sed -nE 's/leaseId=([^ ]+).*/\1/p')"
sleep 1
BUBBLES_REPO_ROOT="$CAP_ROOT" bash "$RUNTIME_SCRIPT" reclaim-stale >/dev/null
cap_fresh_output="$(BUBBLES_REPO_ROOT="$CAP_ROOT" BUBBLES_SESSION_ID="cap-fresh" bash "$RUNTIME_SCRIPT" acquire --purpose cap-fresh --environment dev --share-mode exclusive --weight heavy --fingerprint-input cap:fresh)"
cap_fresh_lease="$(printf '%s\n' "$cap_fresh_output" | sed -nE 's/leaseId=([^ ]+).*/\1/p')"
cap_stale_lookup="$(BUBBLES_REPO_ROOT="$CAP_ROOT" bash "$RUNTIME_SCRIPT" lookup --lease-id "$cap_stale_lease")"
if [[ -n "$cap_fresh_lease" ]] && printf '%s\n' "$cap_stale_lookup" | grep -q 'status=stale'; then
  pass "Stale heavy lease frees capacity for a fresh heavy lease while its record still exists"
else
  fail "Stale heavy lease should free capacity even though its record still exists"
fi
BUBBLES_REPO_ROOT="$CAP_ROOT" bash "$RUNTIME_SCRIPT" release "$cap_fresh_lease" >/dev/null

# Case 5: --wait. Hold the budget, then exercise immediate refuse, wait-loop
# timeout refuse, and post-release success (deterministic; no backgrounding).
cap_hold_output="$(BUBBLES_REPO_ROOT="$CAP_ROOT" BUBBLES_SESSION_ID="cap-hold" bash "$RUNTIME_SCRIPT" acquire --purpose cap-hold --environment dev --share-mode exclusive --weight heavy --fingerprint-input cap:hold)"
cap_hold_lease="$(printf '%s\n' "$cap_hold_output" | sed -nE 's/leaseId=([^ ]+).*/\1/p')"

# 5a: --wait 0 refuses immediately with the structured message.
cap_wait0_out="$(BUBBLES_REPO_ROOT="$CAP_ROOT" BUBBLES_SESSION_ID="cap-wait0" bash "$RUNTIME_SCRIPT" acquire --purpose cap-wait0 --environment dev --share-mode exclusive --weight heavy --wait 0 --fingerprint-input cap:w0 2>&1)" && cap_wait0_rc=0 || cap_wait0_rc=$?
if [[ "$cap_wait0_rc" -ne 0 ]] && printf '%s\n' "$cap_wait0_out" | grep -q 'Runtime capacity exceeded'; then
  pass "--wait 0 refuses immediately with the structured capacity message"
else
  fail "--wait 0 should refuse immediately with the structured capacity message (got rc=$cap_wait0_rc)"
fi

# 5b: --wait <n> (fast poll interval) enters the wait loop, times out, refuses.
cap_wait1_out="$(BUBBLES_REPO_ROOT="$CAP_ROOT" BUBBLES_SESSION_ID="cap-wait1" BUBBLES_RUNTIME_WAIT_INTERVAL_SECONDS=1 bash "$RUNTIME_SCRIPT" acquire --purpose cap-wait1 --environment dev --share-mode exclusive --weight heavy --wait 1 --fingerprint-input cap:w1 2>&1)" && cap_wait1_rc=0 || cap_wait1_rc=$?
if [[ "$cap_wait1_rc" -ne 0 ]] && printf '%s\n' "$cap_wait1_out" | grep -q 'Runtime capacity exceeded'; then
  pass "--wait <n> polls the wait loop and refuses on timeout when capacity never frees"
else
  fail "--wait <n> should refuse on timeout when capacity never frees (got rc=$cap_wait1_rc)"
fi

# 5c: release the holder -> a no-wait heavy acquire now succeeds.
BUBBLES_REPO_ROOT="$CAP_ROOT" bash "$RUNTIME_SCRIPT" release "$cap_hold_lease" >/dev/null
cap_after_output="$(BUBBLES_REPO_ROOT="$CAP_ROOT" BUBBLES_SESSION_ID="cap-after" bash "$RUNTIME_SCRIPT" acquire --purpose cap-after --environment dev --share-mode exclusive --weight heavy --fingerprint-input cap:after)"
cap_after_lease="$(printf '%s\n' "$cap_after_output" | sed -nE 's/leaseId=([^ ]+).*/\1/p')"
if [[ -n "$cap_after_lease" ]]; then
  pass "A no-wait heavy acquire succeeds once the holder releases capacity"
else
  fail "A heavy acquire should succeed once the holder releases capacity"
fi

# Case 6: --weight-units overrides --weight and the budget boundary is exclusive.
# cap-after (8) is active. A 2-unit lease fits exactly (8 + 2 = 10); a further
# 1-unit lease (10 + 1 = 11) is refused. Also proves --weight-units beats --weight.
cap_units_output="$(BUBBLES_REPO_ROOT="$CAP_ROOT" BUBBLES_SESSION_ID="cap-units" bash "$RUNTIME_SCRIPT" acquire --purpose cap-units --environment dev --share-mode exclusive --weight heavy --weight-units 2 --fingerprint-input cap:u 2>&1)"
if printf '%s\n' "$cap_units_output" | grep -q 'weight=2'; then
  pass "--weight-units overrides --weight (heavy) and admits at the exact budget boundary (weight=2)"
else
  fail "--weight-units should override --weight and record weight=2"
fi

cap_over_out="$(BUBBLES_REPO_ROOT="$CAP_ROOT" BUBBLES_SESSION_ID="cap-over" bash "$RUNTIME_SCRIPT" acquire --purpose cap-over --environment dev --share-mode exclusive --weight-units 1 --fingerprint-input cap:o 2>&1)" && cap_over_rc=0 || cap_over_rc=$?
if [[ "$cap_over_rc" -ne 0 ]] && printf '%s\n' "$cap_over_out" | grep -q 'Runtime capacity exceeded'; then
  pass "Capacity boundary is exclusive: a full 10/10 budget refuses a further 1-unit lease"
else
  fail "A 1-unit lease should be refused when capacity is exactly full (got rc=$cap_over_rc)"
fi

mkdir -p "$DOWNSTREAM_ROOT"
git -C "$DOWNSTREAM_ROOT" init -q

(
  cd "$DOWNSTREAM_ROOT"
  bash "$SOURCE_ROOT/install.sh" --local-source "$SOURCE_ROOT" --bootstrap >/dev/null
)

if [[ -x "$DOWNSTREAM_ROOT/.github/bubbles/scripts/runtime-leases.sh" ]]; then
  pass "Downstream bootstrap installs runtime lease script"
else
  fail "Downstream bootstrap should install runtime lease script"
fi

if [[ -f "$DOWNSTREAM_ROOT/.specify/runtime/.gitignore" ]]; then
  pass "Downstream bootstrap scaffolds runtime ignore rules"
else
  fail "Downstream bootstrap should scaffold runtime ignore rules"
fi

downstream_summary="$(cd "$DOWNSTREAM_ROOT" && bash .github/bubbles/scripts/cli.sh runtime summary)"
if printf '%s\n' "$downstream_summary" | grep -q 'Runtime leases: active='; then
  pass "Downstream CLI runtime summary works from installed .github layout"
else
  fail "Downstream CLI runtime summary should work from installed .github layout"
fi

downstream_acquire="$(cd "$DOWNSTREAM_ROOT" && BUBBLES_SESSION_ID="downstream-session-a" bash .github/bubbles/scripts/cli.sh runtime acquire --purpose validation --environment dev --share-mode shared-compatible --fingerprint-input downstream:v1)"
downstream_lease="$(printf '%s\n' "$downstream_acquire" | sed -nE 's/leaseId=([^ ]+).*/\1/p')"
if [[ -n "$downstream_lease" ]]; then
  pass "Downstream CLI can acquire a runtime lease"
else
  fail "Downstream CLI should acquire a runtime lease"
fi

downstream_release="$(cd "$DOWNSTREAM_ROOT" && bash .github/bubbles/scripts/cli.sh runtime release "$downstream_lease")"
if printf '%s\n' "$downstream_release" | grep -q 'Released runtime lease'; then
  pass "Downstream CLI can release a runtime lease"
else
  fail "Downstream CLI should release a runtime lease"
fi

if [[ "$FAILURES" -gt 0 ]]; then
  echo "runtime lease selftest failed with $FAILURES issue(s)."
  exit 1
fi

echo "runtime lease selftest passed."