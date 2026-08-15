#!/usr/bin/env bash
set -euo pipefail

# Hermetic selftest for Gate G094 - capability_foundation_gate.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/capability-foundation-guard.sh"

if [[ ! -x "$GUARD" ]]; then
  echo "capability-foundation-guard-selftest: guard not executable at $GUARD" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-g094-selftest-XXXXXXXX)"
trap 'rm -rf "$WORKSPACE"' EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
FAILED_SCENARIOS=()

ok() { PASS_COUNT=$((PASS_COUNT + 1)); printf '  PASS: %s\n' "$*"; }
ko() { FAIL_COUNT=$((FAIL_COUNT + 1)); FAILED_SCENARIOS+=("$*"); printf '  FAIL: %s\n' "$*"; }

stage_spec() {
  local sid="$1"
  local created_at="$2"
  local dir="$WORKSPACE/$sid"
  mkdir -p "$dir"
  cat > "$dir/state.json" <<EOF
{
  "version": 3,
  "featureDir": "specs/$sid",
  "status": "in_progress",
  "workflowMode": "spec-scope-hardening",
  "createdAt": "$created_at"
}
EOF
  printf '%s' "$dir"
}

write_minimal_triggering_docs() {
  local dir="$1"
  cat > "$dir/spec.md" <<'EOF'
# Notification Feature

## Summary

Add provider delivery for notifications.
EOF
  cat > "$dir/design.md" <<'EOF'
# Design

Add an ntfy provider directly.
EOF
  cat > "$dir/scopes.md" <<'EOF'
# Scopes

## Scope 1: provider
**Status:** Not Started
**Depends On:** none
EOF
}

write_full_capability_docs() {
  local dir="$1"
  cat > "$dir/spec.md" <<'EOF'
# Notification Capability

## Domain Capability Model

NotificationIntent, NotificationPolicy, and DeliveryAttempt define provider-neutral notification behavior.

## UI Wireframes

### UI Primitives

| Primitive | Used By Screens | Composition Rule |
|-----------|-----------------|------------------|
| Provider badge | Provider setup, notification detail | Same status vocabulary |

### Screen: Provider Setup

[provider setup]

### Screen: Notification Detail

[notification detail]
EOF
  cat > "$dir/design.md" <<'EOF'
# Design

## Capability Foundation

NotificationDispatcher owns provider-neutral dispatch.

## Concrete Implementations

### ntfy Adapter

Uses the provider adapter contract.

### Email Adapter

Uses the provider adapter contract.

### Variation Axes

| Axis | Options |
|------|---------|
| Provider protocol | ntfy, email |
| Delivery timing | immediate, digest |
EOF
  cat > "$dir/scopes.md" <<'EOF'
# Scopes

## Scope 1: Notification Foundation
**Status:** Not Started
**Tags:** foundation:true
**Depends On:** none

## Scope 2: ntfy Adapter
**Status:** Not Started
**Depends On:** Scope 1 - Notification Foundation

## Scope 3: Email Adapter
**Status:** Not Started
**Depends On:** Scope 1 - Notification Foundation
EOF
}

run_guard() {
  local dir="$1"
  set +e
  bash "$GUARD" "$dir" > "$WORKSPACE/stdout.last" 2> "$WORKSPACE/stderr.last"
  local rc=$?
  set -e
  echo "$rc" > "$WORKSPACE/exit.last"
}

run_guard_raw() {
  set +e
  bash "$GUARD" "$@" > "$WORKSPACE/stdout.last" 2> "$WORKSPACE/stderr.last"
  local rc=$?
  set -e
  echo "$rc" > "$WORKSPACE/exit.last"
}

assert_exit() {
  local label="$1"
  local expected="$2"
  local actual
  actual="$(cat "$WORKSPACE/exit.last")"
  if [[ "$actual" -eq "$expected" ]]; then
    ok "$label exit=$actual"
  else
    ko "$label expected exit=$expected actual=$actual"
    cat "$WORKSPACE/stdout.last"
    cat "$WORKSPACE/stderr.last"
  fi
}

assert_stderr_contains() {
  local label="$1"
  local needle="$2"
  if grep -qF "$needle" "$WORKSPACE/stderr.last"; then
    ok "$label stderr contains '$needle'"
  else
    ko "$label stderr missing '$needle'"
    cat "$WORKSPACE/stderr.last"
  fi
}

assert_stdout_contains() {
  local label="$1"
  local needle="$2"
  if grep -qF "$needle" "$WORKSPACE/stdout.last"; then
    ok "$label stdout contains '$needle'"
  else
    ko "$label stdout missing '$needle'"
    cat "$WORKSPACE/stdout.last"
  fi
}

echo "=== capability-foundation-guard-selftest (Gate G094) ==="

echo ""
echo "--- S0: missing argument exits 2 ---"
run_guard_raw
assert_exit "S0 missing argument" 2
assert_stderr_contains "S0" "Usage: bash bubbles/scripts/capability-foundation-guard.sh"

echo ""
echo "--- S1: old spec is grandfathered even with provider trigger ---"
repo="$(stage_spec s1-grandfather 2026-05-24T12:00:00Z)"
write_minimal_triggering_docs "$repo"
run_guard "$repo"
assert_exit "S1 grandfather" 0
assert_stdout_contains "S1" "grandfathered"

echo ""
echo "--- S2: new provider-triggered spec missing sections fails ---"
repo="$(stage_spec s2-missing 2026-05-25T12:00:00Z)"
write_minimal_triggering_docs "$repo"
run_guard "$repo"
assert_exit "S2 missing sections" 1
assert_stderr_contains "S2" "Domain Capability Model"
assert_stderr_contains "S2" "Capability Foundation"

echo ""
echo "--- S3: full foundation docs pass ---"
repo="$(stage_spec s3-pass 2026-05-25T12:00:00Z)"
write_full_capability_docs "$repo"
run_guard "$repo"
assert_exit "S3 full foundation" 0
assert_stdout_contains "S3" "PASS Gate G094"

echo ""
echo "--- S4: empty single-implementation justification fails ---"
repo="$(stage_spec s4-empty-justification 2026-05-25T12:00:00Z)"
cat > "$repo/spec.md" <<'EOF'
# One Provider Work

### Single-Capability Justification

EOF
cat > "$repo/design.md" <<'EOF'
# Design

This mentions a provider.

### Single-Implementation Justification

EOF
run_guard "$repo"
assert_exit "S4 empty justification" 1
assert_stderr_contains "S4" "Single-Implementation Justification is empty"

echo ""
echo "--- S5: single implementation justified passes ---"
repo="$(stage_spec s5-single-pass 2026-05-25T12:00:00Z)"
cat > "$repo/spec.md" <<'EOF'
# One Provider Work

This mentions a provider.

### Single-Capability Justification

This is a one-time compatibility bridge inside an existing notification foundation. A broader foundation would be premature.
EOF
cat > "$repo/design.md" <<'EOF'
# Design

### Single-Implementation Justification

This is the only provider supported by the existing foundation for this bug-compatible bridge. No second provider, screen, or shared contract is introduced.
EOF
run_guard "$repo"
assert_exit "S5 single justified" 0
assert_stdout_contains "S5" "PASS Gate G094"

echo ""
echo "--- S6: multi-screen UI without UI primitives fails ---"
repo="$(stage_spec s6-ui-missing 2026-05-25T12:00:00Z)"
cat > "$repo/spec.md" <<'EOF'
# Notification Capability

## Domain Capability Model

Provider-neutral notification capability.

## UI Wireframes

### Screen: Provider Setup

[setup]

### Screen: Notification Detail

[detail]
EOF
cat > "$repo/design.md" <<'EOF'
# Design

### Single-Implementation Justification

This pass stays within one provider adapter while preserving the existing foundation.
EOF
run_guard "$repo"
assert_exit "S6 UI primitives missing" 1
assert_stderr_contains "S6" "UI Primitives"

echo ""
echo "--- S7: foundation split without scope dependency fails ---"
repo="$(stage_spec s7-scope-order 2026-05-25T12:00:00Z)"
write_full_capability_docs "$repo"
cat > "$repo/scopes.md" <<'EOF'
# Scopes

## Scope 1: ntfy Adapter
**Status:** Not Started
**Depends On:** none
EOF
run_guard "$repo"
assert_exit "S7 missing foundation scope order" 1
assert_stderr_contains "S7" "foundation:true"
assert_stderr_contains "S7" "Depends On"

echo ""
echo "=== Selftest verdict ==="
printf '  Total assertions: %d\n' "$((PASS_COUNT + FAIL_COUNT))"
printf '  Passed:           %d\n' "$PASS_COUNT"
# --- IMP-041 SCOPE-5: bidirectional proportionality (GF-11) -----------------
# Until now this gate only caught MISSING abstraction. A one-off evaluation
# could build a reusable foundation and every check applauded it. These cases
# cover the other direction, driven by the frozen executionShape rather than by
# keywords a planner can supply for itself.

write_session_shape() {
  printf '{"goalContract":{"semanticBoundary":{"executionShape":"%s"}}}\n' "$2" > "$1"
}

# S10 — a one-off goal must not stand up a reusable foundation.
S10_DIR="$(stage_spec s10-oneoff 2026-05-25T12:00:00Z)"
write_full_capability_docs "$S10_DIR"
write_session_shape "$WORKSPACE/s10-session.json" one-off
run_guard_raw "$S10_DIR" --session-file "$WORKSPACE/s10-session.json"
assert_exit "S10 one-off with a foundation scope" 1
assert_stderr_contains "S10" "cannot create a reusable foundation"

# S11 — the same shape with no foundation artefacts is fine, and is NOT dragged
# into the foundation-shaped requirements that this shape forbids.
S11_DIR="$(stage_spec s11-oneoff-clean 2026-05-25T12:00:00Z)"
write_minimal_triggering_docs "$S11_DIR"
write_session_shape "$WORKSPACE/s11-session.json" one-off
run_guard_raw "$S11_DIR" --session-file "$WORKSPACE/s11-session.json"
assert_stdout_contains "S11" "one-off shape"

# S12 — an existing-capability change may extend the established foundation,
# but standing a second one beside it is the expansion.
S12_DIR="$(stage_spec s12-parallel 2026-05-25T12:00:00Z)"
write_full_capability_docs "$S12_DIR"
printf '\n## Scope 4: Second Foundation\n**Status:** Not Started\n**Tags:** foundation:true\n' >> "$S12_DIR/scopes.md"
write_session_shape "$WORKSPACE/s12-session.json" existing-capability-change
run_guard_raw "$S12_DIR" --session-file "$WORKSPACE/s12-session.json"
assert_exit "S12 parallel foundation under existing-capability-change" 1
assert_stderr_contains "S12" "parallel one"

# S13 — the approved reusable-capability shape keeps the full strict path.
S13_DIR="$(stage_spec s13-reusable 2026-05-25T12:00:00Z)"
write_full_capability_docs "$S13_DIR"
write_session_shape "$WORKSPACE/s13-session.json" reusable-capability
run_guard_raw "$S13_DIR" --session-file "$WORKSPACE/s13-session.json"
assert_exit "S13 reusable-capability with a complete foundation" 0

# S14 — ADVERSARIAL non-vacuity for the whole scope: with NO session file the
# guard must behave exactly as before. Every case above could pass while the
# legacy keyword path silently changed for every spec already in the field.
S14_DIR="$(stage_spec s14-legacy 2026-05-25T12:00:00Z)"
write_full_capability_docs "$S14_DIR"
run_guard "$S14_DIR"
assert_exit "S14 no session file keeps the legacy path" 0
assert_stdout_contains "S14" "PASS Gate G094"

printf '  Failed:           %d\n' "$FAIL_COUNT"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "capability-foundation-guard-selftest: FAILED" >&2
  printf '  %s\n' "${FAILED_SCENARIOS[@]}" >&2
  exit 1
fi

echo "capability-foundation-guard-selftest: PASSED"
exit 0
