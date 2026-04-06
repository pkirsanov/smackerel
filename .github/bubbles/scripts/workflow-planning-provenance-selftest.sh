#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/state-transition-guard.sh"

tmp_root="$(mktemp -d)"
failures=0

cleanup() {
  if [[ "$failures" -eq 0 ]] && [[ "${KEEP_SELFTEST_TMP:-0}" != "1" ]]; then
    rm -rf "$tmp_root"
  else
    echo "Preserving selftest workspace: $tmp_root"
  fi
}

trap cleanup EXIT

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

run_capture() {
  local log_file="$1"
  shift

  set +e
  "$@" >"$log_file" 2>&1
  local status=$?
  set -e

  echo "$status"
}

assert_log_contains() {
  local log_file="$1"
  local needle="$2"
  local label="$3"

  if grep -Fq "$needle" "$log_file"; then
    pass "$label"
  else
    fail "$label"
    echo "--- log excerpt: $log_file ---"
    sed -n '1,220p' "$log_file"
    echo "--- end log excerpt ---"
  fi
}

emit_fixture() {
  local feature_dir="$1"
  local scenario_test="$feature_dir/tests/planning-provenance.e2e.spec.ts"

  mkdir -p "$feature_dir/tests"

  cat <<'EOF' > "$scenario_test"
export const planningProvenanceRegression = true;
EOF

  cat <<'EOF' > "$feature_dir/spec.md"
# Planning Provenance Selftest Spec

## Purpose

Exercise the guard path where the workflow authored planning artifacts without recording the owning planning specialists.

## Outcome Contract

- Intent: detect planning artifacts that appear complete but lack analyst, UX, design, and planning provenance.
- Success Signal: the transition guard reports missing planning specialists in executionHistory.
- Hard Constraints: fixture stays self-contained and uses explicit artifact ownership boundaries.
- Failure Condition: guard allows the fixture through without detecting the missing planning owners.

## Actors & Personas

- Maintainer reviewing framework orchestration integrity.

## Business Scenarios

### Scenario: workflow writes planning artifacts without owner provenance

Given a feature packet with spec, design, scope, and report artifacts
When the packet claims product-to-delivery work happened without analyst, UX, design, or plan provenance
Then the transition guard must block promotion and report the missing planning specialists.

## UI Wireframes

- Placeholder wireframe marker so the guard requires UX provenance for this analyze-first mode.
EOF

  cat <<'EOF' > "$feature_dir/design.md"
# Planning Provenance Selftest Design

## Approach

Shape the packet so delivery-phase provenance exists for execution specialists, but omit the planning owners the workflow was required to dispatch.
EOF

  cat <<'EOF' > "$feature_dir/scopes.md"
# Scope 01: Planning Provenance Guard Fixture

**Status:** Done

### Goal

Demonstrate that the workflow cannot claim planning artifacts are valid when the owning specialists were never recorded.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Regression E2E | `e2e-ui` | `__SCENARIO_TEST__` | Guard fixture asserting missing planning provenance is rejected. | `selftest:planning-provenance` | Yes |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior -> Evidence: report.md#test-evidence
- [x] Planning provenance remains owner-recorded in execution history -> Evidence: report.md#summary
EOF

  sed -i "s|__SCENARIO_TEST__|$scenario_test|g" "$feature_dir/scopes.md"

  cat <<'EOF' > "$feature_dir/uservalidation.md"
# User Validation

## Checklist

- [x] Planning provenance guard fixture is available for validation.
EOF

  cat <<'EOF' > "$feature_dir/report.md"
# Report

### Summary

This fixture intentionally omits planning-owner provenance from executionHistory while leaving the planning artifacts present.

### Completion Statement

The guard should reject the packet because bubbles.workflow cannot stand in for bubbles.analyst, bubbles.ux, bubbles.design, or bubbles.plan.

### Test Evidence

```text
$ ls -la __FEATURE_DIR__/tests
total 12
drwxr-xr-x 2 selftest selftest 4096 Mar 27 00:00 .
drwxr-xr-x 3 selftest selftest 4096 Mar 27 00:00 ..
-rw-r--r-- 1 selftest selftest   49 Mar 27 00:00 planning-provenance.e2e.spec.ts
$ bash bubbles/scripts/state-transition-guard.sh __FEATURE_DIR__
Expected result: transition blocked because planning specialists are missing from executionHistory.
```
EOF

  sed -i "s|__FEATURE_DIR__|$feature_dir|g" "$feature_dir/report.md"

  cat <<'EOF' > "$feature_dir/state.json"
{
  "version": 3,
  "status": "done",
  "workflowMode": "product-to-delivery",
  "execution": {
    "completedPhaseClaims": [
      "implement",
      "test",
      "regression",
      "simplify",
      "stabilize",
      "security",
      "docs",
      "validate",
      "audit",
      "chaos"
    ]
  },
  "certification": {
    "certifiedCompletedPhases": [
      "implement",
      "test",
      "regression",
      "simplify",
      "stabilize",
      "security",
      "docs",
      "validate",
      "audit",
      "chaos"
    ],
    "completedScopes": ["01-planning-provenance-guard-fixture"],
    "scopeProgress": [],
    "lockdownState": {
      "mode": "off",
      "lockedScenarioIds": []
    },
    "status": "done"
  },
  "policySnapshot": {
    "grill": { "mode": "off", "source": "repo-default" },
    "tdd": { "mode": "off", "source": "repo-default" },
    "autoCommit": { "mode": "off", "source": "repo-default" },
    "lockdown": { "mode": "off", "source": "repo-default" },
    "regression": { "mode": "protect-existing-scenarios", "source": "repo-default" },
    "validation": { "mode": "required", "source": "workflow-forced" }
  },
  "transitionRequests": [],
  "reworkQueue": [],
  "executionHistory": [
    {
      "agent": "bubbles.workflow",
      "phasesExecuted": ["analyze", "bootstrap"],
      "completedAt": "2026-04-06T10:00:00Z",
      "summary": "Workflow orchestrator claimed planning work without dispatch provenance."
    },
    {
      "agent": "bubbles.implement",
      "phasesExecuted": ["implement"],
      "completedAt": "2026-04-06T10:01:00Z"
    },
    {
      "agent": "bubbles.test",
      "phasesExecuted": ["test"],
      "completedAt": "2026-04-06T10:02:00Z"
    },
    {
      "agent": "bubbles.regression",
      "phasesExecuted": ["regression"],
      "completedAt": "2026-04-06T10:03:00Z"
    },
    {
      "agent": "bubbles.simplify",
      "phasesExecuted": ["simplify"],
      "completedAt": "2026-04-06T10:04:00Z"
    },
    {
      "agent": "bubbles.stabilize",
      "phasesExecuted": ["stabilize"],
      "completedAt": "2026-04-06T10:05:00Z"
    },
    {
      "agent": "bubbles.security",
      "phasesExecuted": ["security"],
      "completedAt": "2026-04-06T10:06:00Z"
    },
    {
      "agent": "bubbles.docs",
      "phasesExecuted": ["docs"],
      "completedAt": "2026-04-06T10:07:00Z"
    },
    {
      "agent": "bubbles.validate",
      "phasesExecuted": ["validate"],
      "completedAt": "2026-04-06T10:08:00Z"
    },
    {
      "agent": "bubbles.audit",
      "phasesExecuted": ["audit"],
      "completedAt": "2026-04-06T10:09:00Z"
    },
    {
      "agent": "bubbles.chaos",
      "phasesExecuted": ["chaos"],
      "completedAt": "2026-04-06T10:10:00Z"
    }
  ],
  "lastUpdatedAt": "2026-04-06T10:11:00Z"
}
EOF
}

fixture_dir="$tmp_root/specs/905-planning-provenance-missing-owners"
mkdir -p "$tmp_root/specs"
emit_fixture "$fixture_dir"

echo "Running workflow planning provenance selftest..."
log_file="$tmp_root/workflow-planning-provenance.log"
status="$(run_capture "$log_file" bash "$GUARD_SCRIPT" "$fixture_dir")"

if [[ "$status" -ne 0 ]]; then
  pass "Planning provenance fixture fails the transition guard as expected"
else
  fail "Planning provenance fixture should fail the transition guard"
  sed -n '1,220p' "$log_file"
fi

assert_log_contains "$log_file" "Planning specialist 'bubbles.analyst' missing from executionHistory" "Guard reports missing analyst provenance"
assert_log_contains "$log_file" "Planning specialist 'bubbles.design' missing from executionHistory" "Guard reports missing design provenance"
assert_log_contains "$log_file" "Planning specialist 'bubbles.plan' missing from executionHistory" "Guard reports missing planning provenance"
assert_log_contains "$log_file" "Planning specialist 'bubbles.ux' missing from executionHistory" "Guard reports missing UX provenance when UI wireframes exist"
assert_log_contains "$log_file" "planning-first workflow compliance not proven" "Guard escalates the planning provenance failure"

echo "----------------------------------------"
if [[ "$failures" -gt 0 ]]; then
  echo "workflow-planning-provenance selftest failed with $failures issue(s)."
  exit 1
fi

echo "workflow-planning-provenance selftest passed."