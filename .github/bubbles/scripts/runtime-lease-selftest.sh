#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RUNTIME_SCRIPT="$SCRIPT_DIR/runtime-leases.sh"
SOURCE_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TMP_ROOT="$(mktemp -d)"
TEST_ROOT="$TMP_ROOT/runtime-selftest-repo"
DOWNSTREAM_ROOT="$TMP_ROOT/runtime-downstream-repo"
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

acquire_one_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" BUBBLES_SESSION_ID="workflow-session-a" BUBBLES_AGENT_NAME="bubbles.validate" bash "$RUNTIME_SCRIPT" acquire --purpose validation --environment dev --share-mode shared-compatible --fingerprint-input schema:v1 --resource container:test-a)"
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
if printf '%s\n' "$lookup_shared_output" | grep -q 'attachedSessions=workflow-session-a,workflow-session-b'; then
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
if printf '%s\n' "$lookup_after_detach" | grep -q 'status=active' && printf '%s\n' "$lookup_after_detach" | grep -q 'attachedSessions=workflow-session-a'; then
  pass "Lease stays active while another shared session remains attached"
else
  fail "Lease should stay active after detaching one shared session"
fi

shared_release_output="$(BUBBLES_REPO_ROOT="$TEST_ROOT" bash "$RUNTIME_SCRIPT" release "$lease_one" --session-id workflow-session-a)"
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