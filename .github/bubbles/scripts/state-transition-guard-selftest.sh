#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/state-transition-guard.sh"
OWNERSHIP_LINT_SCRIPT="$SCRIPT_DIR/agent-ownership-lint.sh"
# shellcheck source=/dev/null
source "$SCRIPT_DIR/guard-lib.sh"

# This selftest already exercises the transition guard's own status, artifact,
# scope, packet, timestamp, lockdown, and deferral checks. The delegated tail
# gates (G085-G095) each have dedicated selftests in framework-validate, so keep
# them out of this cumulative fixture suite to avoid repeated heavy scans.
export BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=1

selftest_tmp_base="${TMPDIR:-$HOME/.cache}"
mkdir -p "$selftest_tmp_base"
tmp_root="$(mktemp -d "$selftest_tmp_base/bubbles-transition-guard-selftest.XXXXXX")"
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

clone_framework_surface() {
  local destination_root="$1"

  mkdir -p "$destination_root"
  cp -R "$SCRIPT_DIR/.." "$destination_root/bubbles"
  cp -R "$SCRIPT_DIR/../../agents" "$destination_root/agents"
}

inject_unauthorized_workflow_runner() {
  local capabilities_file="$1"
  local tmp_file
  tmp_file="$(mktemp)"

  awk '
    BEGIN { inserted=0 }
    /^  bubbles\.implement:$/ {
      print
      in_block=1
      next
    }
    in_block && /^    class: execution-owner$/ {
      print
      print "    canExecuteWorkflowModes: true"
      inserted=1
      in_block=0
      next
    }
    { print }
    END {
      if (inserted == 0) {
        exit 1
      }
    }
  ' "$capabilities_file" > "$tmp_file"

  mv "$tmp_file" "$capabilities_file"
}

assert_log_contains() {
  local log_file="$1"
  local needle="$2"
  local label="$3"

  if grep -Fq -- "$needle" "$log_file"; then
    pass "$label"
  else
    fail "$label"
    echo "--- log excerpt: $log_file ---"
    sed -n '1,160p' "$log_file"
    echo "--- end log excerpt ---"
  fi
}

assert_log_not_contains() {
  local log_file="$1"
  local needle="$2"
  local label="$3"

  if grep -Fq -- "$needle" "$log_file"; then
    fail "$label"
    echo "--- offending log excerpt: $log_file ---"
    grep -F "$needle" "$log_file" || true
    echo "--- end offending log excerpt ---"
  else
    pass "$label"
  fi
}

assert_transition_result() {
  local log_file="$1"
  local expected_mode="$2"
  local expected_profile="$3"
  local expected_target="$4"
  local expected_not_applicable="$5"
  local expected_verdict="$6"
  local expected_exit="$7"
  local label="$8"

  if awk \
    -v expected_mode="$expected_mode" \
    -v expected_profile="$expected_profile" \
    -v expected_target="$expected_target" \
    -v expected_na="$expected_not_applicable" \
    -v expected_verdict="$expected_verdict" \
    -v expected_exit="$expected_exit" '
    BEGIN {
      field_count = split("schemaVersion workflowMode auditProfile targetStatus contractDigest targetRevision applicableCheckClasses notApplicableChecks passedGateIds failedGateIds failedChecks blockingCode failureCount exitStatus verdict", fields, " ")
    }
    $0 == "BEGIN TRANSITION_GUARD_RESULT_V1" {
      begin_count++
      active = 1
      field_index = 0
      next
    }
    $0 == "END TRANSITION_GUARD_RESULT_V1" {
      end_count++
      active = 0
      next
    }
    active {
      field_index++
      expected_prefix = fields[field_index] ": "
      if (field_index > field_count || index($0, expected_prefix) != 1) {
        invalid = 1
        next
      }
      values[fields[field_index]] = substr($0, length(expected_prefix) + 1)
    }
    END {
      if (begin_count != 1 || end_count != 1 || field_index != field_count) invalid = 1
      if (values["schemaVersion"] != "transition-guard-result/v1") invalid = 1
      if (values["workflowMode"] != expected_mode) invalid = 1
      if (values["auditProfile"] != expected_profile) invalid = 1
      if (values["targetStatus"] != expected_target) invalid = 1
      if (values["notApplicableChecks"] != expected_na) invalid = 1
      if (values["verdict"] != expected_verdict || values["exitStatus"] != expected_exit) invalid = 1
      for (field_number = 7; field_number <= 11; field_number++) {
        if (values[fields[field_number]] !~ /^\[[A-Za-z0-9,-]*\]$/) invalid = 1
      }
      if (values["failureCount"] !~ /^[0-9]+$/) invalid = 1
      failure_count = values["failureCount"] + 0
      if (expected_verdict == "PASS" && (failure_count != 0 || values["blockingCode"] != "none")) invalid = 1
      if (expected_verdict == "FAIL" && (failure_count < 1 || values["blockingCode"] == "none")) invalid = 1
      if (expected_verdict == "BLOCKED" && (failure_count < 1 || values["blockingCode"] !~ /^E009-/)) invalid = 1
      if (expected_mode == "UNRESOLVED") {
        if (values["contractDigest"] != "UNRESOLVED" || values["targetRevision"] != "UNRESOLVED") invalid = 1
      } else {
        if (values["contractDigest"] !~ /^sha256:[0-9a-f]+$/ || length(values["contractDigest"]) != 71) invalid = 1
        if (values["targetRevision"] !~ /^sha256:[0-9a-f]+$/ || length(values["targetRevision"]) != 71) invalid = 1
      }
      exit invalid ? 1 : 0
    }
  ' "$log_file"; then
    pass "$label"
  else
    fail "$label"
    echo "--- invalid transition result: $log_file ---"
    sed -n '/BEGIN TRANSITION_GUARD_RESULT_V1/,/END TRANSITION_GUARD_RESULT_V1/p' "$log_file"
    echo "--- end invalid transition result ---"
  fi
}

assert_transition_list_contains() {
  local log_file="$1"
  local field="$2"
  local expected_item="$3"
  local label="$4"

  if awk -v field="$field" -v expected_item="$expected_item" '
    index($0, field ": [") == 1 {
      value = substr($0, length(field) + 4)
      sub(/\]$/, "", value)
      count = split(value, items, ",")
      for (item_number = 1; item_number <= count; item_number++) {
        if (items[item_number] == expected_item) found = 1
      }
    }
    END { exit found ? 0 : 1 }
  ' "$log_file"; then
    pass "$label"
  else
    fail "$label"
    grep -F -- "$field:" "$log_file" || true
  fi
}

emit_base_fixture() {
  local feature_dir="$1"
  local scenario_test="$feature_dir/tests/docs-scenario-regression.e2e.spec.ts"
  local broader_test="$feature_dir/tests/docs-broader-regression.e2e.spec.ts"

  mkdir -p "$feature_dir/tests"

  cat <<'EOF' > "$scenario_test"
export const docsScenarioRegression = true;
EOF

  cat <<'EOF' > "$broader_test"
export const docsBroaderRegression = true;
EOF

  cat <<'EOF' > "$feature_dir/spec.md"
# Guard Selftest Spec

## Purpose

Exercise the docs-only promotion path with a minimal but coherent artifact set.
EOF

  cat <<'EOF' > "$feature_dir/design.md"
# Guard Selftest Design

## Approach

Use a docs-only workflow mode so the transition guard still evaluates state integrity, artifact integrity, and routing contracts without requiring implementation-heavy runtime proof.
EOF

  cat <<'EOF' > "$feature_dir/uservalidation.md"
# User Validation

## Checklist

- [x] Baseline docs-only validation path is available for the selftest fixture.
EOF

  cat <<'EOF' > "$feature_dir/scopes.md"
# Scope 01: Docs-Only Guard Fixture

**Status:** Done

### Goal

Keep the fixture small while still exercising the real transition guard against a coherent docs-only feature directory.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Regression E2E | `e2e-ui` | `__SCENARIO_TEST__` | Scenario-specific regression row required by the guard. | `selftest:scenario-regression` | Yes |
| Regression E2E | `e2e-ui` | `__BROADER_TEST__` | Broader regression row required by the guard. | `selftest:broader-regression` | Yes |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior -> Evidence: report.md#test-evidence
- [x] Broader E2E regression suite passes -> Evidence: report.md#test-evidence
- [x] Documentation route metadata is recorded consistently across artifacts -> Evidence: report.md#summary
EOF

  bubbles_sed_inplace "s|__SCENARIO_TEST__|$scenario_test|g" "$feature_dir/scopes.md"
  bubbles_sed_inplace "s|__BROADER_TEST__|$broader_test|g" "$feature_dir/scopes.md"

  cat <<'EOF' > "$feature_dir/report.md"
# Report

### Summary

Docs-only transition-guard selftest fixture.

### Completion Statement

The temporary fixture is shaped to satisfy the docs-only promotion ceiling while still exercising the guard's state, artifact, and routing checks.

### Test Evidence

```text
$ bash bubbles/scripts/agent-ownership-lint.sh
Agent ownership lint passed.
$ ls -la __FEATURE_DIR__/tests
total 16
drwxr-xr-x 2 selftest selftest 4096 Mar 27 00:00 .
drwxr-xr-x 3 selftest selftest 4096 Mar 27 00:00 ..
-rw-r--r-- 1 selftest selftest   41 Mar 27 00:00 docs-broader-regression.e2e.spec.ts
-rw-r--r-- 1 selftest selftest   42 Mar 27 00:00 docs-scenario-regression.e2e.spec.ts
```
EOF

  bubbles_sed_inplace "s|__FEATURE_DIR__|$feature_dir|g" "$feature_dir/report.md"

  cat <<'EOF' > "$feature_dir/state.json"
{
  "version": 3,
  "status": "docs_updated",
  "workflowMode": "docs-only",
  "execution": {
    "completedPhaseClaims": ["docs"]
  },
  "certification": {
    "certifiedCompletedPhases": ["docs"],
    "completedScopes": ["01-docs-guard-fixture"],
    "scopeProgress": [],
    "lockdownState": {
      "mode": "off",
      "lockedScenarioIds": []
    },
    "status": "docs_updated"
  },
  "policySnapshot": {
    "grill": { "mode": "off", "source": "repo-default" },
    "tdd": { "mode": "off", "source": "repo-default" },
    "autoCommit": { "mode": "off", "source": "repo-default" },
    "lockdown": { "mode": "off", "source": "repo-default" },
    "regression": { "mode": "protect-existing-scenarios", "source": "repo-default" },
    "validation": { "mode": "required", "source": "workflow-forced" },
    "workflowMode": "docs-only"
  },
  "transitionRequests": [],
  "reworkQueue": [],
  "executionHistory": [
    {
      "phase": "docs",
      "completedAt": "2026-03-27T10:00:07Z"
    }
  ],
  "lastUpdatedAt": "2026-03-27T10:00:09Z"
}
EOF
}

emit_honest_planning_fixture() {
  local feature_dir="$1"
  local future_test="$feature_dir/tests/regression/planning-maturity-future-test.sh"

  mkdir -p "$feature_dir"

  cat <<'EOF' > "$feature_dir/spec.md"
# Guard Planning-Maturity Fixture

## Problem

A planning workflow must preserve honest incomplete delivery state while
evaluating its registry-required planning contract.

## User Scenarios & Testing

### SCN-009-S03-001 - Preserve honest planning maturity

```gherkin
Scenario: Planning maturity preserves honest incomplete delivery
Given a registry-bound planning packet
When the transition guard evaluates planning maturity
Then incomplete delivery remains honest and non-terminal
```

## Requirements

- **FR-009-S03-001:** Planning maturity preserves honest incomplete delivery.
EOF

  cat <<'EOF' > "$feature_dir/design.md"
# Guard Planning-Maturity Fixture Design

## Approach

Resolve one immutable transition contract and keep structural, planning, and
honesty checks active while delivery completion remains non-applicable.

## Change Boundary

Only the temporary planning packet is evaluated. No implementation artifact is
created to satisfy a delivery check.

## Consumer Impact Sweep

No route, identifier, command, or external consumer changes in this fixture.

## Shared Infrastructure Impact Sweep

No shared infrastructure or persistent state changes in this fixture.
EOF

  cat <<'EOF' > "$feature_dir/uservalidation.md"
# User Validation

## Checklist

- [x] Planning maturity and delivery completion are visibly distinct.
EOF

  cat <<'EOF' > "$feature_dir/scopes.md"
# Scope 01: Honest Planning Maturity

**Status:** Not Started

## Goal

Preserve honest incomplete delivery state at the planning ceiling.

## Gherkin Scenarios

### SCN-009-S03-001 - Preserve honest planning maturity

```gherkin
Scenario: Planning maturity preserves honest incomplete delivery
Given a registry-bound planning packet
When the transition guard evaluates planning maturity
Then incomplete delivery remains honest and non-terminal
```

## Implementation Plan

1. Activate the registry-derived planning profile in the canonical guard.
2. Preserve every structural and planning integrity check.

## Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Regression E2E | `e2e` | `__FUTURE_TEST__` | Regression for SCN-009-S03-001 through the production guard. | `bash __FUTURE_TEST__` | No |
| Broader regression | `regression` | `__FUTURE_TEST__` | Preserve planning and delivery profile isolation. | `bash __FUTURE_TEST__` | No |

### Definition of Done

- [ ] Planning maturity preserves honest incomplete delivery for SCN-009-S03-001.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior protect SCN-009-S03-001.
- [ ] Broader E2E regression suite passes with profile isolation active.
EOF
  bubbles_sed_inplace "s|__FUTURE_TEST__|$future_test|g" "$feature_dir/scopes.md"

  cat <<'EOF' > "$feature_dir/report.md"
# Report

### Summary

This report belongs to an honestly unimplemented planning scope.

### Completion Statement

No delivery completion is claimed at the planning ceiling.

### Test Evidence

Execution-evidence code blocks: zero. The implementation scope remains Not Started.

### Code Diff Evidence

No delivery implementation delta is claimed by this planning-only fixture.

### Scope Evidence

Scope 01 remains Not Started with implementation DoD unchecked.

### Validation Evidence

Validation evaluates planning maturity only.

### Audit Evidence

No delivery certification is claimed.
EOF

  cat <<'EOF' > "$feature_dir/scenario-manifest.json"
{
  "version": 1,
  "scenarios": [
    {
      "scenarioId": "SCN-009-S03-001",
      "title": "Planning maturity preserves honest incomplete delivery",
      "status": "planned",
      "scope": "Scope 01",
      "requirements": ["FR-009-S03-001"],
      "requiredTestType": "e2e",
      "linkedTests": ["__FUTURE_TEST__"],
      "evidenceRefs": []
    }
  ]
}
EOF
  bubbles_sed_inplace "s|__FUTURE_TEST__|$future_test|g" "$feature_dir/scenario-manifest.json"

  cat <<'EOF' > "$feature_dir/state.json"
{
  "version": 3,
  "status": "specs_hardened",
  "workflowMode": "product-to-planning",
  "planningOnly": true,
  "planMaturityOnly": true,
  "planningOnlyJustification": "This fixture evaluates planning maturity without delivery claims.",
  "execution": {
    "currentScope": null,
    "currentPhase": "plan",
    "completedPhaseClaims": ["analyze", "ux", "design", "plan"]
  },
  "certification": {
    "status": "specs_hardened",
    "certifiedCompletedPhases": ["analyze", "ux", "design", "plan"],
    "completedScopes": [],
    "scopeProgress": [
      {
        "scopeId": "S01",
        "scopeName": "Honest Planning Maturity",
        "status": "not_started"
      }
    ],
    "lockdownState": {
      "mode": "off",
      "lockedScenarioIds": []
    }
  },
  "policySnapshot": {
    "grill": { "mode": "off", "source": "repo-default" },
    "tdd": { "mode": "off", "source": "repo-default" },
    "autoCommit": { "mode": "off", "source": "repo-default" },
    "lockdown": { "mode": "off", "source": "repo-default" },
    "regression": { "mode": "protect-existing-scenarios", "source": "repo-default" },
    "validation": { "mode": "required", "source": "workflow-forced" },
    "workflowMode": "product-to-planning"
  },
  "transitionRequests": [],
  "reworkQueue": [],
  "executionHistory": [
    {
      "phase": "analyze",
      "agent": "bubbles.analyst",
      "outcome": "completed_diagnostic",
      "startedAt": "2026-07-10T10:00:00Z",
      "completedAt": "2026-07-10T10:01:13Z"
    },
    {
      "phase": "ux",
      "agent": "bubbles.ux",
      "outcome": "completed_diagnostic",
      "startedAt": "2026-07-10T10:02:01Z",
      "completedAt": "2026-07-10T10:04:29Z"
    },
    {
      "phase": "design",
      "agent": "bubbles.design",
      "outcome": "completed_diagnostic",
      "startedAt": "2026-07-10T10:05:17Z",
      "completedAt": "2026-07-10T10:08:52Z"
    },
    {
      "phase": "plan",
      "agent": "bubbles.plan",
      "outcome": "completed_diagnostic",
      "startedAt": "2026-07-10T10:09:31Z",
      "completedAt": "2026-07-10T10:14:47Z"
    }
  ],
  "lastUpdatedAt": "2026-07-10T10:15:03Z"
}
EOF
}

set_fixture_contract() {
  local state_file="$1"
  local workflow_mode="$2"
  local status="$3"

  python3 - "$state_file" "$workflow_mode" "$status" <<'PY'
import json
import sys

path, workflow_mode, status = sys.argv[1:]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

data["workflowMode"] = workflow_mode
data["status"] = status
snapshot = data.get("policySnapshot")
if isinstance(snapshot, dict):
    snapshot["workflowMode"] = workflow_mode
certification = data.get("certification")
if isinstance(certification, dict):
    certification["status"] = status

if workflow_mode == "autonomous-goal":
    data.pop("planningOnly", None)
    data.pop("planMaturityOnly", None)
    data.pop("planningOnlyJustification", None)

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY
}

# Flip ONLY policySnapshot.tdd.mode -> scenario-first on an existing fixture so a
# clone of an otherwise-passing packet carries a live scenario-first TDD policy.
# This isolates Check 3E's RED->GREEN requirement as the single differentiator
# between the planning-maturity and delivery-completion audit profiles.
set_fixture_tdd_scenario_first() {
  local state_file="$1"

  python3 - "$state_file" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

snapshot = data.get("policySnapshot")
if not isinstance(snapshot, dict):
    snapshot = {}
    data["policySnapshot"] = snapshot
snapshot["tdd"] = {"mode": "scenario-first", "source": "repo-default"}

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY
}

mark_first_dod_checked() {
  local scope_file="$1"
  local temp_file
  temp_file="$(mktemp)"
  awk '
    !changed && /^- \[ \] / {
      sub(/^- \[ \] /, "- [x] ")
      changed=1
    }
    { print }
    END { if (!changed) exit 1 }
  ' "$scope_file" > "$temp_file"
  mv "$temp_file" "$scope_file"
}

mark_scope_done() {
  local scope_file="$1"
  bubbles_sed_inplace 's/^\*\*Status:\*\* Not Started/**Status:** Done/' "$scope_file"
}

break_gherkin_dod_fidelity() {
  local scope_file="$1"
  bubbles_sed_inplace \
    's/^Scenario: Planning maturity preserves honest incomplete delivery$/Scenario: Rotating archived credentials deletes obsolete transport records/' \
    "$scope_file"
}

remove_planning_only_linkage() {
  local state_file="$1"
  local temp_file
  temp_file="$(mktemp)"
  jq 'del(.planningOnly, .planningOnlyJustification)' "$state_file" > "$temp_file"
  mv "$temp_file" "$state_file"
}

emit_shared_infra_fixture() {
  local feature_dir="$1"
  local canary_test="$feature_dir/tests/auth-bootstrap-canary.e2e.spec.ts"
  local broader_test="$feature_dir/tests/auth-bootstrap-broader.e2e.spec.ts"

  mkdir -p "$feature_dir/tests"

  cat <<'EOF' > "$canary_test"
export const authBootstrapCanary = true;
EOF

  cat <<'EOF' > "$broader_test"
export const authBootstrapBroader = true;
EOF

  cat <<'EOF' > "$feature_dir/spec.md"
# Shared Infrastructure Guard Selftest Spec

## Purpose

Exercise the shared fixture/bootstrap blast-radius checks on a docs-only artifact set.
EOF

  cat <<'EOF' > "$feature_dir/design.md"
# Shared Infrastructure Guard Selftest Design

## Approach

Use a shared auth bootstrap fixture scenario to prove the transition guard enforces Shared Infrastructure Impact Sweep and Change Boundary planning requirements.
EOF

  cat <<'EOF' > "$feature_dir/uservalidation.md"
# User Validation

## Checklist

- [x] Shared infrastructure planning path is available for the selftest fixture.
EOF

  cat <<'EOF' > "$feature_dir/scopes.md"
# Scope 01: Shared Auth Bootstrap Fixture Refactor

**Status:** Done

### Goal

Keep the fixture small while still exercising the guard's shared auth bootstrap fixture planning checks.

### Shared Infrastructure Impact Sweep

- Blast radius: shared auth fixture, bootstrap helper, and session bootstrap contract
- Downstream contract surfaces: ordering, timing, session storage injection, tenant context, role hydration

### Change Boundary

- Allowed file families: tests/auth-fixture/**, tests/bootstrap/**
- Excluded surfaces: backend handler tests, unrelated API mocks, cross-directory cleanup

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Canary: Shared bootstrap contract | `e2e-ui` | `__CANARY_TEST__` | Validates ordering, timing, and session bootstrap contract before broader reruns. | `selftest:auth-bootstrap-canary` | Yes |
| Regression E2E | `e2e-ui` | `__BROADER_TEST__` | Broader auth bootstrap regression row required by the guard. | `selftest:auth-bootstrap-broader` | Yes |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior -> Evidence: report.md#test-evidence
- [x] Broader E2E regression suite passes -> Evidence: report.md#test-evidence
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns -> Evidence: report.md#test-evidence
- [x] Rollback or restore path for shared infrastructure changes is documented and verified -> Evidence: report.md#summary
- [x] Change Boundary is respected and zero excluded file families were changed -> Evidence: report.md#summary
EOF

  bubbles_sed_inplace "s|__CANARY_TEST__|$canary_test|g" "$feature_dir/scopes.md"
  bubbles_sed_inplace "s|__BROADER_TEST__|$broader_test|g" "$feature_dir/scopes.md"

  cat <<'EOF' > "$feature_dir/report.md"
# Report

### Summary

Shared auth bootstrap fixture selftest with documented rollback/restore path and explicit change boundary.

### Completion Statement

The temporary fixture is shaped to satisfy the docs-only promotion ceiling while exercising the shared-infrastructure planning checks.

### Test Evidence

```text
$ ls -la __FEATURE_DIR__/tests
total 16
drwxr-xr-x 2 selftest selftest 4096 Mar 27 00:00 .
drwxr-xr-x 3 selftest selftest 4096 Mar 27 00:00 ..
-rw-r--r-- 1 selftest selftest   40 Mar 27 00:00 auth-bootstrap-broader.e2e.spec.ts
-rw-r--r-- 1 selftest selftest   39 Mar 27 00:00 auth-bootstrap-canary.e2e.spec.ts
```
EOF

  bubbles_sed_inplace "s|__FEATURE_DIR__|$feature_dir|g" "$feature_dir/report.md"

  cat <<'EOF' > "$feature_dir/state.json"
{
  "version": 3,
  "status": "docs_updated",
  "workflowMode": "docs-only",
  "execution": {
    "completedPhaseClaims": ["docs"]
  },
  "certification": {
    "certifiedCompletedPhases": ["docs"],
    "completedScopes": ["01-shared-auth-bootstrap-fixture-refactor"],
    "scopeProgress": [],
    "lockdownState": {
      "mode": "off",
      "lockedScenarioIds": []
    },
    "status": "docs_updated"
  },
  "policySnapshot": {
    "grill": { "mode": "off", "source": "repo-default" },
    "tdd": { "mode": "off", "source": "repo-default" },
    "autoCommit": { "mode": "off", "source": "repo-default" },
    "lockdown": { "mode": "off", "source": "repo-default" },
    "regression": { "mode": "protect-existing-scenarios", "source": "repo-default" },
    "validation": { "mode": "required", "source": "workflow-forced" },
    "workflowMode": "docs-only"
  },
  "transitionRequests": [],
  "reworkQueue": [],
  "executionHistory": [
    {
      "phase": "docs",
      "completedAt": "2026-03-27T10:10:07Z"
    }
  ],
  "lastUpdatedAt": "2026-03-27T10:10:09Z"
}
EOF
}

emit_shared_infra_negative_fixture() {
  local feature_dir="$1"
  local canary_test="$feature_dir/tests/auth-bootstrap-broader.e2e.spec.ts"

  mkdir -p "$feature_dir/tests"

  cat <<'EOF' > "$canary_test"
export const authBootstrapBroaderOnly = true;
EOF

  cat <<'EOF' > "$feature_dir/spec.md"
# Shared Infrastructure Negative Guard Selftest Spec

## Purpose

Exercise the negative shared auth bootstrap fixture path with missing planning controls.
EOF

  cat <<'EOF' > "$feature_dir/design.md"
# Shared Infrastructure Negative Guard Selftest Design

## Approach

Use a shared auth bootstrap fixture refactor without blast-radius planning so the transition guard blocks promotion.
EOF

  cat <<'EOF' > "$feature_dir/uservalidation.md"
# User Validation

## Checklist

- [x] Negative shared infrastructure path is available for the selftest fixture.
EOF

  cat <<'EOF' > "$feature_dir/scopes.md"
# Scope 01: Shared Auth Bootstrap Fixture Refactor

**Status:** Done

### Goal

Exercise the guard's negative shared auth fixture path by omitting blast-radius controls.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Regression E2E | `e2e-ui` | `__BROADER_TEST__` | Broader auth bootstrap regression row required by the guard. | `selftest:auth-bootstrap-broader` | Yes |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior -> Evidence: report.md#test-evidence
- [x] Broader E2E regression suite passes -> Evidence: report.md#test-evidence
EOF

  bubbles_sed_inplace "s|__BROADER_TEST__|$canary_test|g" "$feature_dir/scopes.md"

  cat <<'EOF' > "$feature_dir/report.md"
# Report

### Summary

Negative shared auth bootstrap fixture selftest missing blast-radius controls.

### Completion Statement

The temporary fixture intentionally omits Shared Infrastructure Impact Sweep and Change Boundary sections.

### Test Evidence

```text
$ ls -la __FEATURE_DIR__/tests
total 12
drwxr-xr-x 2 selftest selftest 4096 Mar 27 00:00 .
drwxr-xr-x 3 selftest selftest 4096 Mar 27 00:00 ..
-rw-r--r-- 1 selftest selftest   43 Mar 27 00:00 auth-bootstrap-broader.e2e.spec.ts
```
EOF

  bubbles_sed_inplace "s|__FEATURE_DIR__|$feature_dir|g" "$feature_dir/report.md"

  cat <<'EOF' > "$feature_dir/state.json"
{
  "version": 3,
  "status": "docs_updated",
  "workflowMode": "docs-only",
  "execution": {
    "completedPhaseClaims": ["docs"]
  },
  "certification": {
    "certifiedCompletedPhases": ["docs"],
    "completedScopes": ["01-shared-auth-bootstrap-fixture-refactor"],
    "scopeProgress": [],
    "lockdownState": {
      "mode": "off",
      "lockedScenarioIds": []
    },
    "status": "docs_updated"
  },
  "policySnapshot": {
    "grill": { "mode": "off", "source": "repo-default" },
    "tdd": { "mode": "off", "source": "repo-default" },
    "autoCommit": { "mode": "off", "source": "repo-default" },
    "lockdown": { "mode": "off", "source": "repo-default" },
    "regression": { "mode": "protect-existing-scenarios", "source": "repo-default" },
    "validation": { "mode": "required", "source": "workflow-forced" },
    "workflowMode": "docs-only"
  },
  "transitionRequests": [],
  "reworkQueue": [],
  "executionHistory": [
    {
      "phase": "docs",
      "completedAt": "2026-03-27T10:11:07Z"
    }
  ],
  "lastUpdatedAt": "2026-03-27T10:11:09Z"
}
EOF
}

emit_per_scope_fixture() {
  local feature_dir="$1"
  local index_status="$2"
  local completed_scope_entry="$3"
  local scope_dir="$feature_dir/scopes/01-index-parity-proof"
  local scenario_test="$feature_dir/tests/per-scope-regression.e2e.spec.ts"

  mkdir -p "$scope_dir" "$feature_dir/tests"

  cat <<'EOF' > "$scenario_test"
export const perScopeRegression = true;
EOF

  cat <<'EOF' > "$feature_dir/spec.md"
# Per-Scope Guard Selftest Spec

## Purpose

Exercise the per-scope-directory transition guard paths for index parity and completed scope integrity.
EOF

  cat <<'EOF' > "$feature_dir/design.md"
# Per-Scope Guard Selftest Design

## Approach

Use a minimal per-scope-directory artifact set so the guard evaluates _index.md parity and completedScopes mapping against real scope artifacts.
EOF

  cat <<'EOF' > "$feature_dir/uservalidation.md"
# User Validation

## Checklist

- [x] Per-scope-directory validation path is available for the selftest fixture.
EOF

  cat > "$feature_dir/scopes/_index.md" <<EOF
# Scopes Index

## Dependency Graph

| Scope | Title | Depends On | Status |
| --- | --- | --- | --- |
| 01 | Index parity proof | None | $index_status |
EOF

  cat <<'EOF' > "$scope_dir/scope.md"
# Scope 01: Index Parity Proof

**Status:** Done

### Goal

Keep the per-scope fixture minimal while still exercising _index.md parity and completedScopes artifact mapping.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Regression E2E | `e2e-ui` | `__SCENARIO_TEST__` | Per-scope regression row required by the guard. | `selftest:per-scope-regression` | Yes |
| Regression E2E | `e2e-ui` | `__SCENARIO_TEST__` | Broader regression row required by the guard. | `selftest:per-scope-broader-regression` | Yes |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior -> Evidence: report.md#test-evidence
- [x] Broader E2E regression suite passes -> Evidence: report.md#test-evidence
- [x] Documentation route metadata is recorded consistently across artifacts -> Evidence: report.md#summary
EOF

  bubbles_sed_inplace "s|__SCENARIO_TEST__|$scenario_test|g" "$scope_dir/scope.md"

  cat <<'EOF' > "$scope_dir/report.md"
# Report

### Summary

Per-scope-directory transition-guard selftest fixture.

### Completion Statement

The temporary fixture is shaped to satisfy per-scope artifact requirements while exercising _index.md parity and completedScopes mapping.

### Test Evidence

```text
$ ls -la __FEATURE_DIR__/tests
total 12
drwxr-xr-x 2 selftest selftest 4096 Mar 27 00:00 .
drwxr-xr-x 4 selftest selftest 4096 Mar 27 00:00 ..
-rw-r--r-- 1 selftest selftest   36 Mar 27 00:00 per-scope-regression.e2e.spec.ts
```
EOF

  bubbles_sed_inplace "s|__FEATURE_DIR__|$feature_dir|g" "$scope_dir/report.md"

  cat > "$feature_dir/state.json" <<EOF
{
  "version": 3,
  "status": "docs_updated",
  "workflowMode": "docs-only",
  "execution": {
    "completedPhaseClaims": ["docs"]
  },
  "certification": {
    "certifiedCompletedPhases": ["docs"],
    "completedScopes": ["$completed_scope_entry"],
    "scopeProgress": [
      {
        "scopeDir": "scopes/01-index-parity-proof"
      }
    ],
    "lockdownState": {
      "mode": "off",
      "lockedScenarioIds": []
    },
    "status": "docs_updated"
  },
  "policySnapshot": {
    "grill": { "mode": "off", "source": "repo-default" },
    "tdd": { "mode": "off", "source": "repo-default" },
    "autoCommit": { "mode": "off", "source": "repo-default" },
    "lockdown": { "mode": "off", "source": "repo-default" },
    "regression": { "mode": "protect-existing-scenarios", "source": "repo-default" },
    "validation": { "mode": "required", "source": "workflow-forced" },
    "workflowMode": "docs-only"
  },
  "transitionRequests": [],
  "reworkQueue": [],
  "executionHistory": [
    {
      "phase": "docs",
      "completedAt": "2026-03-27T10:20:07Z"
    }
  ],
  "lastUpdatedAt": "2026-03-27T10:20:09Z"
}
EOF
}

mutate_workflow_mode_contradiction() {
  local state_file="$1"

  python3 - "$state_file" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

snapshot = data.get("policySnapshot")
if isinstance(snapshot, dict):
    snapshot["workflowMode"] = "full-delivery"

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY
}

mutate_planning_mode_status() {
  local state_file="$1"
  local status="$2"
  local plan_maturity_only="$3"

  python3 - "$state_file" "$status" "$plan_maturity_only" <<'PY'
import json
import sys

path, status, plan_maturity_only = sys.argv[1:4]
with open(path, encoding="utf-8") as handle:
  data = json.load(handle)

data["status"] = status
data["workflowMode"] = "product-to-planning"
data["planMaturityOnly"] = plan_maturity_only == "true"
if status == "specs_hardened":
  data["planningOnly"] = True
  data["planningOnlyJustification"] = "Selftest planning packet intentionally has no implementation target."

snapshot = data.get("policySnapshot")
if isinstance(snapshot, dict):
  snapshot["workflowMode"] = "product-to-planning"

cert = data.get("certification")
if isinstance(cert, dict):
  cert["status"] = status

with open(path, "w", encoding="utf-8") as handle:
  json.dump(data, handle, indent=2)
  handle.write("\n")
PY
}

mutate_delivery_contract() {
  local state_file="$1"

  python3 - "$state_file" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
  data = json.load(handle)

data["status"] = "in_progress"
data["workflowMode"] = "autonomous-goal"
snapshot = data.get("policySnapshot")
if isinstance(snapshot, dict):
  snapshot["workflowMode"] = "autonomous-goal"

execution = data.get("execution")
if not isinstance(execution, dict):
  execution = {}
  data["execution"] = execution
execution["completedPhaseClaims"] = ["test", "validate", "audit", "docs"]

certification = data.get("certification")
if not isinstance(certification, dict):
  certification = {}
  data["certification"] = certification
certification["status"] = "in_progress"
certification["certifiedCompletedPhases"] = ["test", "validate", "audit", "docs"]

data["executionHistory"] = [
  {
    "phase": "test",
    "agent": "bubbles.test",
    "phasesExecuted": ["test"],
    "runStartedAt": "2026-03-27T10:00:00Z",
    "runCompletedAt": "2026-03-27T10:00:47Z",
    "completedAt": "2026-03-27T10:00:47Z",
  },
  {
    "phase": "validate",
    "agent": "bubbles.validate",
    "phasesExecuted": ["validate"],
    "runStartedAt": "2026-03-27T10:01:13Z",
    "runCompletedAt": "2026-03-27T10:02:31Z",
    "completedAt": "2026-03-27T10:02:31Z",
  },
  {
    "phase": "audit",
    "agent": "bubbles.audit",
    "phasesExecuted": ["audit"],
    "runStartedAt": "2026-03-27T10:03:02Z",
    "runCompletedAt": "2026-03-27T10:06:08Z",
    "completedAt": "2026-03-27T10:06:08Z",
  },
  {
    "phase": "docs",
    "agent": "bubbles.docs",
    "phasesExecuted": ["docs"],
    "runStartedAt": "2026-03-27T10:07:19Z",
    "runCompletedAt": "2026-03-27T10:11:44Z",
    "completedAt": "2026-03-27T10:11:44Z",
  },
]

with open(path, "w", encoding="utf-8") as handle:
  json.dump(data, handle, indent=2)
  handle.write("\n")
PY
}

emit_g040_fixture() {
  # G040 / Check 18 selftest fixture builder.
  #
  # Args:
  #   feature_dir            — destination directory
  #   status                 — "done" or "done_with_concerns"
  #   prose                  — narrative line to inject into report.md OUTSIDE
  #                            the existing fenced code block
  #   use_skip_markers       — "yes" to wrap prose in
  #                            <!-- bubbles:g040-skip-begin/end -->; "no" otherwise
  #   include_followup_yaml  — "yes" to also append a worked done_with_concerns
  #                            schema example with followUpOwner/Action/Target/Follows
  #   legacy_compatibility   — "yes" to mark the fixture as a legacy read-only
  #                            done_with_concerns artifact under G092
  local feature_dir="$1"
  local status="$2"
  local prose="$3"
  local use_skip_markers="$4"
  local include_followup_yaml="$5"
  local legacy_compatibility="${6:-no}"

  emit_base_fixture "$feature_dir"
  mutate_delivery_contract "$feature_dir/state.json"

  # Mutate status only. autonomous-goal is a supported delivery contract and avoids
  # Check 17's full-delivery git-log probe over the temporary fixture.
  python3 - "$feature_dir/state.json" "$status" "$legacy_compatibility" <<'PY'
import json
import sys

path, new_status, legacy_compatibility = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

data["status"] = new_status
cert = data.setdefault("certification", {})
cert["status"] = new_status
if legacy_compatibility == "yes":
    data["legacyStatusCompatibility"] = True
else:
    data.pop("legacyStatusCompatibility", None)

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY

  {
    echo ""
    echo "## Follow-Up Narrative"
    if [[ "$use_skip_markers" == "yes" ]]; then
      echo "<!-- bubbles:g040-skip-begin -->"
      echo "$prose"
      echo "<!-- bubbles:g040-skip-end -->"
    else
      echo "$prose"
    fi
    if [[ "$include_followup_yaml" == "yes" ]]; then
      echo ""
      echo "concerns:"
      echo "  - id: CONCERN-1"
      echo "    severity: low"
      echo "    description: Selftest concern shape only."
      echo "    followUpOwner: bubbles.bug"
      echo "    followUpAction: new-spec"
      echo "    followUpTarget: BUG-099"
      echo "followUps:"
      echo "  - target: BUG-099"
      echo "    owner: bubbles.bug"
    fi
  } >> "$feature_dir/report.md"
}

mutate_execution_history_implausible() {
  local state_file="$1"

  python3 - "$state_file" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

execution = data.get("execution")
if not isinstance(execution, dict):
    execution = {}
    data["execution"] = execution

execution["executionHistory"] = [
    {
        "agent": "bubbles.implement",
        "runStartedAt": "2026-03-27T10:00:00Z",
        "runCompletedAt": "2026-03-27T10:05:00Z",
        "phasesExecuted": ["implement"],
    },
    {
        "agent": "bubbles.test",
        "runStartedAt": "2026-03-27T10:15:00Z",
        "runCompletedAt": "2026-03-27T10:20:00Z",
        "phasesExecuted": ["implement"],
    },
    {
        "agent": "bubbles.audit",
        "runStartedAt": "2026-03-27T10:30:00Z",
        "runCompletedAt": "2026-03-27T10:35:00Z",
        "phasesExecuted": ["audit"],
    },
]
execution["completedPhaseClaims"] = []
cert = data.get("certification")
if isinstance(cert, dict):
  cert["certifiedCompletedPhases"] = []
  state = cert.get("lockdownState")
  if isinstance(state, dict):
    state["round"] = 2
    state["lastCleanRound"] = 2

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY
}

mutate_lockdown_round_mismatch() {
  local state_file="$1"

  python3 - "$state_file" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

execution = data.get("execution")
if not isinstance(execution, dict):
    execution = {}
    data["execution"] = execution

execution["executionHistory"] = [
    {
        "agent": "bubbles.implement",
        "runStartedAt": "2026-03-27T10:00:00Z",
        "runCompletedAt": "2026-03-27T10:05:00Z",
        "phasesExecuted": ["implement"],
    },
    {
        "agent": "bubbles.test",
        "runStartedAt": "2026-03-27T10:11:00Z",
        "runCompletedAt": "2026-03-27T10:16:00Z",
        "phasesExecuted": ["implement"],
    },
    {
        "agent": "bubbles.audit",
        "runStartedAt": "2026-03-27T10:29:00Z",
        "runCompletedAt": "2026-03-27T10:35:00Z",
        "phasesExecuted": ["audit"],
    },
]
execution["completedPhaseClaims"] = []
cert = data.get("certification")
if isinstance(cert, dict):
    cert["certifiedCompletedPhases"] = []
    state = cert.get("lockdownState")
    if isinstance(state, dict):
        state["round"] = 3
        state["lastCleanRound"] = 2

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY
}

mutate_dict_shaped_phase_claims() {
  local state_file="$1"

  python3 - "$state_file" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

# Regression shape for the Check 6 / Check 6B unhashable-dict crash:
#   - certifiedCompletedPhases is EMPTY, forcing Check 6's fallback onto
#     execution.completedPhaseClaims;
#   - completedPhaseClaims entries are DICT objects (the real runtime shape),
#     which previously blew up `dict.fromkeys(...)` with
#     `TypeError: cannot use 'dict' as a dict key (unhashable type: 'dict')`.
# workflowMode=iterate keeps required_specialists small (validate, audit) so the
# selftest can positively assert Check 6 reads the phase names OUT of the dicts,
# and the matching executionHistory lets Check 6B validate their provenance.
data["workflowMode"] = "iterate"
snapshot = data.get("policySnapshot")
if isinstance(snapshot, dict):
    snapshot["workflowMode"] = "iterate"

execution = data.get("execution")
if not isinstance(execution, dict):
    execution = {}
    data["execution"] = execution

execution["completedPhaseClaims"] = [
    {"phase": "validate", "agent": "bubbles.validate"},
    {"phase": "audit", "agent": "bubbles.audit"},
]
execution["executionHistory"] = [
    {
        "agent": "bubbles.validate",
        "runStartedAt": "2026-03-27T10:40:00Z",
        "runCompletedAt": "2026-03-27T10:45:00Z",
        "phasesExecuted": ["validate"],
    },
    {
        "agent": "bubbles.audit",
        "runStartedAt": "2026-03-27T10:50:00Z",
        "runCompletedAt": "2026-03-27T10:56:00Z",
        "phasesExecuted": ["audit"],
    },
]

cert = data.get("certification")
if isinstance(cert, dict):
    cert["certifiedCompletedPhases"] = []

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY
}

positive_feature_dir="$tmp_root/specs/900-transition-guard-selftest-pass"
negative_feature_dir="$tmp_root/specs/901-transition-guard-selftest-missing-owner"
shared_positive_feature_dir="$tmp_root/specs/903-transition-guard-selftest-shared-pass"
shared_negative_feature_dir="$tmp_root/specs/904-transition-guard-selftest-shared-missing-controls"
workflow_mode_negative_feature_dir="$tmp_root/specs/905-transition-guard-selftest-workflowmode-mismatch"
per_scope_positive_feature_dir="$tmp_root/specs/906-transition-guard-selftest-per-scope-pass"
index_parity_negative_feature_dir="$tmp_root/specs/907-transition-guard-selftest-index-mismatch"
phantom_scope_negative_feature_dir="$tmp_root/specs/908-transition-guard-selftest-phantom-scope"
execution_history_negative_feature_dir="$tmp_root/specs/909-transition-guard-selftest-execution-history"
lockdown_round_negative_feature_dir="$tmp_root/specs/910-transition-guard-selftest-lockdown-round"
planning_done_negative_feature_dir="$tmp_root/specs/911-transition-guard-selftest-product-planning-done"
planning_specs_hardened_positive_feature_dir="$tmp_root/specs/912-transition-guard-selftest-product-planning-specs-hardened"
s03_planning_feature_dir="$tmp_root/specs/913-bug009-s03-planning-pass"
s03_hardening_feature_dir="$tmp_root/specs/914-bug009-s03-hardening-pass"
s03_delivery_negative_dir="$tmp_root/specs/915-bug009-s03-delivery-negative"
s03_checked_evidence_dir="$tmp_root/specs/916-bug009-s03-checked-evidence"
s03_done_honesty_dir="$tmp_root/specs/917-bug009-s03-done-honesty"
s03_g068_dir="$tmp_root/specs/918-bug009-s03-g068-negative"
s03_delivery_checked_dir="$tmp_root/specs/919-bug009-s03-delivery-checked-evidence"
g060_planning_na_dir="$tmp_root/specs/928-bug026-g060-planning-not-applicable"
g060_delivery_enforced_dir="$tmp_root/specs/929-bug026-g060-delivery-enforced"
g040_planning_na_dir="$tmp_root/specs/930-g040-planning-not-applicable"
g040_pos_deferred_dir="$tmp_root/specs/920-g040-positive-deferred-prose"
g040_pos_skip_for_now_dir="$tmp_root/specs/921-g040-positive-skip-for-now"
g040_neg_followup_fields_dir="$tmp_root/specs/922-g040-negative-schema-yaml-only"
g040_neg_done_with_concerns_dir="$tmp_root/specs/923-g040-negative-done-with-concerns"
g040_neg_skip_markers_dir="$tmp_root/specs/924-g040-negative-skip-markers"
g040_pos_skip_marker_outside_dir="$tmp_root/specs/925-g040-positive-skip-marker-outside"
g040_neg_spec_063_excerpt_dir="$tmp_root/specs/926-g040-negative-spec-063-excerpt"
g040_pos_strict_done_mixed_dir="$tmp_root/specs/927-g040-positive-strict-done-mixed"
g064_framework_root="$tmp_root/framework-g064"
g064_feature_dir="$g064_framework_root/specs/902-transition-guard-selftest-unauthorized-workflow-runner"
mkdir -p "$tmp_root/specs"
clone_framework_surface "$tmp_root"
git -C "$tmp_root" init -q
export BUBBLES_REPO_ROOT="$tmp_root"

dogfood_done_dir="$tmp_root/specs/899-transition-guard-selftest-dogfood-done"
mkdir -p "$dogfood_done_dir"
cat <<'EOF' > "$dogfood_done_dir/state.json"
{
  "status": "done"
}
EOF

emit_base_fixture "$positive_feature_dir"
mutate_delivery_contract "$positive_feature_dir/state.json"
cp -R "$positive_feature_dir" "$negative_feature_dir"
emit_shared_infra_fixture "$shared_positive_feature_dir"
mutate_delivery_contract "$shared_positive_feature_dir/state.json"
emit_shared_infra_negative_fixture "$shared_negative_feature_dir"
mutate_delivery_contract "$shared_negative_feature_dir/state.json"
cp -R "$positive_feature_dir" "$workflow_mode_negative_feature_dir"
mutate_workflow_mode_contradiction "$workflow_mode_negative_feature_dir/state.json"
cp -R "$positive_feature_dir" "$planning_done_negative_feature_dir"
mutate_planning_mode_status "$planning_done_negative_feature_dir/state.json" "done" "true"
cp -R "$positive_feature_dir" "$planning_specs_hardened_positive_feature_dir"
mutate_planning_mode_status "$planning_specs_hardened_positive_feature_dir/state.json" "specs_hardened" "true"
emit_honest_planning_fixture "$s03_planning_feature_dir"
cp -R "$s03_planning_feature_dir" "$s03_hardening_feature_dir"
set_fixture_contract "$s03_hardening_feature_dir/state.json" "spec-scope-hardening" "specs_hardened"
cp -R "$s03_planning_feature_dir" "$s03_delivery_negative_dir"
set_fixture_contract "$s03_delivery_negative_dir/state.json" "autonomous-goal" "in_progress"
cp -R "$s03_planning_feature_dir" "$s03_checked_evidence_dir"
mark_first_dod_checked "$s03_checked_evidence_dir/scopes.md"
cp -R "$s03_planning_feature_dir" "$s03_done_honesty_dir"
mark_scope_done "$s03_done_honesty_dir/scopes.md"
cp -R "$s03_planning_feature_dir" "$s03_g068_dir"
break_gherkin_dod_fidelity "$s03_g068_dir/scopes.md"
cp -R "$s03_delivery_negative_dir" "$s03_delivery_checked_dir"
mark_first_dod_checked "$s03_delivery_checked_dir/scopes.md"
# BUG-026 G060 profile-awareness: isolate Check 3E's scenario-first enforcement to
# the audit profile. Each fixture is cloned from an already-PASSING packet and
# only flips policySnapshot.tdd.mode -> scenario-first, so the SOLE differentiator
# is the RED->GREEN evidence requirement:
#   * planning-maturity-v1  -> Check 3E NOT_APPLICABLE (plan hardening, no runtime test surface yet)
#   * delivery-completion-v1 -> Check 3E STILL enforces G060 (delivery is unchanged)
cp -R "$s03_planning_feature_dir" "$g060_planning_na_dir"
set_fixture_tdd_scenario_first "$g060_planning_na_dir/state.json"
cp -R "$positive_feature_dir" "$g060_delivery_enforced_dir"
set_fixture_tdd_scenario_first "$g060_delivery_enforced_dir/state.json"
# G040 Check 18 planning-maturity exemption: an honest planning packet carrying a
# forward-looking domain label ("Authorized Outcome Follow-Up") the context-free
# deferral regex would otherwise flag. Under planning maturity Check 18 is
# NOT_APPLICABLE so this must not block plan hardening. Delivery-side G040
# enforcement stays covered by the g040_pos_* fixtures.
cp -R "$s03_planning_feature_dir" "$g040_planning_na_dir"
printf '\nThe Authorized Outcome Follow-Up surface is a planned MVP capability of this executable-capability graph.\n' >> "$g040_planning_na_dir/scopes.md"
emit_per_scope_fixture "$per_scope_positive_feature_dir" "Done" "scope-1-index-parity-proof"
mutate_delivery_contract "$per_scope_positive_feature_dir/state.json"
emit_per_scope_fixture "$index_parity_negative_feature_dir" "In Progress" "scope-1-index-parity-proof"
mutate_delivery_contract "$index_parity_negative_feature_dir/state.json"
emit_per_scope_fixture "$phantom_scope_negative_feature_dir" "Done" "scope-15-stochastic-sweep-remediation"
mutate_delivery_contract "$phantom_scope_negative_feature_dir/state.json"
cp -R "$positive_feature_dir" "$execution_history_negative_feature_dir"
mutate_execution_history_implausible "$execution_history_negative_feature_dir/state.json"
cp -R "$positive_feature_dir" "$lockdown_round_negative_feature_dir"
mutate_lockdown_round_mismatch "$lockdown_round_negative_feature_dir/state.json"

# G040 / Check 18 fixtures (spec 001-stg-check18-deferral-regex-refinement)
emit_g040_fixture "$g040_pos_deferred_dir" "done" \
  "Several action items were deferred to next sprint per planning notes." \
  "no" "no"
emit_g040_fixture "$g040_pos_skip_for_now_dir" "done" \
  "Decision: skip for now and revisit in a follow-up cycle." \
  "no" "no"
emit_g040_fixture "$g040_neg_followup_fields_dir" "done" \
  "Schema worked example follows in YAML form below." \
  "no" "yes"
emit_g040_fixture "$g040_neg_done_with_concerns_dir" "done_with_concerns" \
  "Concern routed to bubbles.bug for follow-up tracking; nothing was deferred." \
  "no" "yes" "yes"
emit_g040_fixture "$g040_neg_skip_markers_dir" "done" \
  "The historical narrative below is bracketed because it discusses content that was deferred to next sprint in a prior release." \
  "yes" "no"
emit_g040_fixture "$g040_pos_skip_marker_outside_dir" "done" \
  "First sentence sits inside markers and is fine." \
  "yes" "no"
# Append a SECOND deferral-prose paragraph OUTSIDE the marker pair so the
# guard must still BLOCK on the unbracketed content.
{
  echo ""
  echo "## Trailing Outside-Marker Section"
  echo "Despite the bracketed narrative above, this paragraph admits work was deferred to next sprint and remains unmarked."
} >> "$g040_pos_skip_marker_outside_dir/report.md"

emit_g040_fixture "$g040_neg_spec_063_excerpt_dir" "done_with_concerns" \
  "Audit narrative: each concern listed in the followUps section was tracked separately under bubbles.bug ownership." \
  "no" "yes" "yes"
emit_g040_fixture "$g040_pos_strict_done_mixed_dir" "done" \
  "The schema field name followUpOwner appears here intentionally as part of the mixed-content fixture, but real deferral prose follows." \
  "no" "yes"
# Append unambiguous deferral prose OUTSIDE markers so Check 18 must still BLOCK.
{
  echo ""
  echo "## Genuinely Deferred Item"
  echo "We punted to Phase 3 the entire migration of legacy adapters; that work was not done in this scope."
} >> "$g040_pos_strict_done_mixed_dir/report.md"

clone_framework_surface "$g064_framework_root"
mkdir -p "$g064_framework_root/specs"
emit_base_fixture "$g064_feature_dir"
mutate_delivery_contract "$g064_feature_dir/state.json"
inject_unauthorized_workflow_runner "$g064_framework_root/bubbles/agent-capabilities.yaml"

cat <<'EOF' > "$negative_feature_dir/rework-queue.json"
[
  {
    "reworkId": "RW-901-001",
    "status": "closed",
    "reason": "Concrete packet fields must remain present even after closure.",
    "dodItems": ["DOD-01-01"]
  }
]
EOF

# BUG-006 fixtures: Check 4B canonicality + Check 5 counting must ignore
# >-prefixed header/summary blockquotes (e.g. a planning rollup line), while
# still flagging a genuine non-canonical PLAIN scope-status line.
bug006_blockquote_pass_dir="$tmp_root/specs/930-bug006-status-blockquote-pass"
cp -R "$positive_feature_dir" "$bug006_blockquote_pass_dir"
bug006_tmp="$(mktemp)"
{
  echo "> **Status:** all scopes Not Started (planning refreshed 2026-06-17)"
  echo ""
  cat "$bug006_blockquote_pass_dir/scopes.md"
} > "$bug006_tmp"
mv "$bug006_tmp" "$bug006_blockquote_pass_dir/scopes.md"

bug006_noncanonical_neg_dir="$tmp_root/specs/931-bug006-noncanonical-plain-status"
cp -R "$positive_feature_dir" "$bug006_noncanonical_neg_dir"
{
  echo ""
  echo "## Scope 02: Non-Canonical Status Probe"
  echo ""
  echo "**Status:** Deferred"
} >> "$bug006_noncanonical_neg_dir/scopes.md"

# BUG-007 fixture: Check 8C must NOT fire on benign prose that merely co-mentions
# a trigger word (session) and a generic word (flow). The note deliberately avoids
# any shared/global qualifier or fixture/bootstrap/harness infra noun.
bug007_benign_dir="$tmp_root/specs/932-bug007-benign-session-flow"
cp -R "$positive_feature_dir" "$bug007_benign_dir"
{
  echo ""
  echo "### Additional Benign Note"
  echo ""
  echo "The regression session re-runs the booking user flow end to end."
} >> "$bug007_benign_dir/scopes.md"

# Check 6 / Check 6B regression fixture (unhashable-dict crash). A state.json
# with an EMPTY certifiedCompletedPhases and DICT-shaped completedPhaseClaims
# previously crashed the guard with a Python TypeError and read every required
# phase as missing (false G022). The mutator reshapes the passing base fixture
# into that exact shape under workflowMode=iterate.
dict_phase_claims_dir="$tmp_root/specs/933-transition-guard-selftest-dict-phase-claims"
cp -R "$positive_feature_dir" "$dict_phase_claims_dir"
mutate_dict_shaped_phase_claims "$dict_phase_claims_dir/state.json"

echo "Running agent ownership lint precheck..."
lint_log="$tmp_root/agent-ownership-lint.log"
lint_status="$(run_capture "$lint_log" bash "$OWNERSHIP_LINT_SCRIPT")"
if [[ "$lint_status" -eq 0 ]]; then
  pass "Agent ownership lint precheck passes"
else
  fail "Agent ownership lint precheck failed"
  sed -n '1,160p' "$lint_log"
fi

echo "Running positive transition-guard selftest..."
positive_log="$tmp_root/positive-guard.log"
positive_status="$(run_capture "$positive_log" bash "$GUARD_SCRIPT" "$positive_feature_dir")"
if [[ "$positive_status" -eq 0 ]]; then
  pass "Supported delivery positive fixture passes the transition guard"
else
  fail "Supported delivery positive fixture should pass the transition guard"
  sed -n '1,220p' "$positive_log"
  echo "--- artifact-lint output for positive fixture ---"
  set +e
  bash "$SCRIPT_DIR/artifact-lint.sh" "$positive_feature_dir"
  set -e
  echo "--- end artifact-lint output ---"
fi
assert_log_contains "$positive_log" "Framework ownership lint passed" "Positive fixture exercises guard Check 3G"
assert_log_contains "$positive_log" "TRANSITION PERMITTED" "Positive fixture reaches a permitted transition verdict"

# BUG-022 managed zero-result canary: the real passing guard must serialize
# empty failure collections without weakening nounset or the result grammar.
assert_log_not_contains "$positive_log" "unbound variable" "BUG-022 empty result collections do not abort under nounset"
assert_log_contains "$positive_log" "notApplicableChecks: []" "BUG-022 empty not-applicable checks serialize exactly"
assert_log_contains "$positive_log" "failedGateIds: []" "BUG-022 empty failed gates serialize exactly"
assert_log_contains "$positive_log" "failedChecks: []" "BUG-022 empty failed checks serialize exactly"

# --- G053 Check 13B: shell (.sh) runtime-path recognition ---
# Regression guard for the G053<->G093 alignment fix. The G093 delivery-delta
# guard's path_family already classifies *.sh as `runtime`; G053 Check 13B's
# Code Diff Evidence runtime-path regex must agree, otherwise a shell-only
# delivery (e.g. a git-hook or operator script fix) passes G093 but is wrongly
# rejected by G053. bugfix-fastlane requires impl-delta (so Check 13B runs) but
# does NOT trigger Check 17's full-delivery git-log probe over the /tmp fixture.
echo "Running G053 Check 13B shell-runtime-path recognition selftest..."
g053_sh_dir="$tmp_root/specs/940-g053-shell-runtime-path"
cp -R "$positive_feature_dir" "$g053_sh_dir"
g053_sh_state_tmp="$(mktemp)"
sed 's/"workflowMode": "autonomous-goal"/"workflowMode": "bugfix-fastlane"/g' "$g053_sh_dir/state.json" > "$g053_sh_state_tmp"
mv "$g053_sh_state_tmp" "$g053_sh_dir/state.json"
# Overwrite report.md so the ONLY runtime-extension token is the shell path —
# otherwise the base fixture's `.ts` test filenames would make the case pass
# regardless of the fix (tautological).
cat <<'EOF' > "$g053_sh_dir/report.md"
# Report

### Summary

G053 Check 13B shell-runtime-path recognition fixture.

### Code Diff Evidence

**Command:** git show HEAD --stat
**Exit Code:** 0
**Claim Source:** executed

```
$ git show HEAD --stat -- scripts/tooling/example-runtime.sh
 scripts/tooling/example-runtime.sh | 4 +++-
 1 file changed, 3 insertions(+), 1 deletion(-)
```
EOF
g053_sh_log="$tmp_root/g053-shell-runtime.log"
run_capture "$g053_sh_log" bash "$GUARD_SCRIPT" "$g053_sh_dir" >/dev/null
assert_log_contains "$g053_sh_log" \
  "Implementation delta evidence recorded with git-backed proof and non-artifact file paths (Gate G053)" \
  "G053 Check 13B accepts a shell (.sh) runtime path in Code Diff Evidence (parity with G093 path_family)"

# Negative twin: an artifact-only Code Diff Evidence (no runtime-extension path)
# must STILL be rejected — proving the positive case passes specifically because
# the shell path is now recognized, not because the check went vacuous.
g053_artifact_dir="$tmp_root/specs/941-g053-artifact-only"
cp -R "$positive_feature_dir" "$g053_artifact_dir"
g053_artifact_state_tmp="$(mktemp)"
sed 's/"workflowMode": "autonomous-goal"/"workflowMode": "bugfix-fastlane"/g' "$g053_artifact_dir/state.json" > "$g053_artifact_state_tmp"
mv "$g053_artifact_state_tmp" "$g053_artifact_dir/state.json"
cat <<'EOF' > "$g053_artifact_dir/report.md"
# Report

### Summary

G053 Check 13B artifact-only negative fixture.

### Code Diff Evidence

**Command:** git show HEAD --stat
**Exit Code:** 0
**Claim Source:** executed

```
$ git show HEAD --stat -- specs/941-g053-artifact-only/design.md
 specs/941-g053-artifact-only/design.md | 2 +-
 1 file changed, 1 insertion(+), 1 deletion(-)
```
EOF
g053_artifact_log="$tmp_root/g053-artifact-only.log"
run_capture "$g053_artifact_log" bash "$GUARD_SCRIPT" "$g053_artifact_dir" >/dev/null
assert_log_contains "$g053_artifact_log" \
  "Code Diff Evidence does not show any non-artifact runtime/source/config file paths" \
  "G053 Check 13B still rejects an artifact-only Code Diff Evidence (non-vacuous)"

# --- Check 8: shell (.sh) test-path recognition (Test File Existence) ---
# Regression guard for the Check 8 extension-alternation parity fix. Check 8's
# test-path extraction regex historically recognized only
# (spec|test|rs|ts|tsx|js|jsx); a Test Plan citing a REAL shell test (e.g. a
# reconcile-regression.sh) was wrongly warned "No concrete test file paths found
# in Test Plan". This mirrors the G053<->G093 shell-path alignment (commit
# 4e41c1d) and line 2341's runtime-path family (sh|bash|dart|java|scala).
echo "Running Check 8 shell-test-path recognition selftest..."
check8_sh_dir="$tmp_root/specs/942-check8-shell-test-path"
cp -R "$per_scope_positive_feature_dir" "$check8_sh_dir"
check8_sh_test="$check8_sh_dir/tests/scripts/reconcile-regression.sh"
mkdir -p "$check8_sh_dir/tests/scripts"
cat <<'EOF' > "$check8_sh_test"
#!/usr/bin/env bash
# Selftest fixture: a real shell test whose absolute path Check 8 must recognize.
echo "reconcile regression ok"
EOF
chmod +x "$check8_sh_test"
# Overwrite the scope Test Plan so the ONLY File/Location cell is the real .sh
# absolute path -- otherwise the base fixture's `.e2e.spec.ts` row would make
# Check 8 pass regardless of the fix (tautological). Keep Status:Done + the DoD
# rows intact so the fixture stays otherwise valid.
cat <<'EOF' > "$check8_sh_dir/scopes/01-index-parity-proof/scope.md"
# Scope 01: Index Parity Proof

**Status:** Done

### Goal

Exercise Check 8 test-path extraction against a real shell (.sh) test file.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Regression E2E | `e2e-ui` | `__SH_TEST__` | Shell regression test whose real .sh path Check 8 must recognize. | `selftest:reconcile-regression` | Yes |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior -> Evidence: report.md#test-evidence
- [x] Broader E2E regression suite passes -> Evidence: report.md#test-evidence
- [x] Documentation route metadata is recorded consistently across artifacts -> Evidence: report.md#summary
EOF
bubbles_sed_inplace "s|__SH_TEST__|$check8_sh_test|g" "$check8_sh_dir/scopes/01-index-parity-proof/scope.md"
check8_sh_log="$tmp_root/check8-shell-test-path.log"
run_capture "$check8_sh_log" bash "$GUARD_SCRIPT" "$check8_sh_dir" >/dev/null
assert_log_contains "$check8_sh_log" \
  "Test file exists: $check8_sh_test" \
  "Check 8 recognizes a shell (.sh) test path in the Test Plan"
assert_log_not_contains "$check8_sh_log" \
  "No concrete test file paths found in Test Plan" \
  "Check 8 does not fall through to the placeholder warning when a real .sh test path is present"

# Negative twin: a placeholder-only File/Location (`[path]`, which Check 8
# explicitly ignores) must STILL warn "No concrete test file paths found" --
# proving the positive above passes specifically because `.sh` is now recognized,
# not because Check 8 went vacuous.
echo "Running Check 8 placeholder-only non-vacuity selftest..."
check8_ph_dir="$tmp_root/specs/943-check8-placeholder-only"
cp -R "$per_scope_positive_feature_dir" "$check8_ph_dir"
cat <<'EOF' > "$check8_ph_dir/scopes/01-index-parity-proof/scope.md"
# Scope 01: Index Parity Proof

**Status:** Done

### Goal

Prove Check 8 falls through to the placeholder warning when no concrete test path is present.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Regression E2E | `e2e-ui` | `[path]` | Placeholder-only File/Location that Check 8 explicitly ignores. | `[command]` | Yes |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior -> Evidence: report.md#test-evidence
- [x] Broader E2E regression suite passes -> Evidence: report.md#test-evidence
- [x] Documentation route metadata is recorded consistently across artifacts -> Evidence: report.md#summary
EOF
check8_ph_log="$tmp_root/check8-placeholder-only.log"
run_capture "$check8_ph_log" bash "$GUARD_SCRIPT" "$check8_ph_dir" >/dev/null
assert_log_contains "$check8_ph_log" \
  "No concrete test file paths found in Test Plan" \
  "Check 8 non-vacuity: placeholder-only Test Plan still warns (positive passes because .sh is recognized, not because Check 8 is vacuous)"

# --- Check 8: command-wrapped shell (.sh) test-path extraction ---
# Regression guard for the Check 8 whole-backtick-block extraction bug. Test
# Plans routinely cite a shell test as a COMMAND, not a bare path -- e.g.
# `bash tests/x.sh` or `bash -n a.sh && shellcheck -x a.sh`. The original
# extraction captured the ENTIRE backtick block, so the command string
# ("bash tests/x.sh") was treated as a bogus non-existent file path and Check 8
# false-BLOCKed ("references non-existent file" / "DO NOT EXIST") even though the
# real .sh file exists. The fix isolates the path TOKEN within the block. The
# 942 case above used a BARE .sh path, so it passed regardless of this bug; this
# case exercises the command-wrapped pattern that actually regressed downstream.
echo "Running Check 8 command-wrapped shell-test extraction selftest..."
check8_cmd_dir="$tmp_root/specs/944-check8-command-wrapped-sh-test"
cp -R "$per_scope_positive_feature_dir" "$check8_cmd_dir"
check8_cmd_test="$check8_cmd_dir/tests/scripts/reconcile-regression.sh"
mkdir -p "$check8_cmd_dir/tests/scripts"
cat <<'EOF' > "$check8_cmd_test"
#!/usr/bin/env bash
# Selftest fixture: a real shell test cited via a COMMAND in the Test Plan.
echo "reconcile regression ok"
EOF
chmod +x "$check8_cmd_test"
# The File/Location + Command cells wrap the real .sh path inside shell COMMANDS
# (`bash <path>`, `bash -n <path> && shellcheck -x <path>`). Check 8 must extract
# the path token and confirm existence, NOT treat the command string as missing.
cat <<'EOF' > "$check8_cmd_dir/scopes/01-index-parity-proof/scope.md"
# Scope 01: Index Parity Proof

**Status:** Done

### Goal

Exercise Check 8 test-path extraction against a shell (.sh) test cited as a command.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Regression E2E | `e2e-ui` | `bash __SH_TEST__` | Shell regression test cited via a command wrapper. | `bash -n __SH_TEST__ && shellcheck -x __SH_TEST__` | Yes |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior -> Evidence: report.md#test-evidence
- [x] Broader E2E regression suite passes -> Evidence: report.md#test-evidence
- [x] Documentation route metadata is recorded consistently across artifacts -> Evidence: report.md#summary
EOF
bubbles_sed_inplace "s|__SH_TEST__|$check8_cmd_test|g" "$check8_cmd_dir/scopes/01-index-parity-proof/scope.md"
check8_cmd_log="$tmp_root/check8-command-wrapped-sh-test.log"
run_capture "$check8_cmd_log" bash "$GUARD_SCRIPT" "$check8_cmd_dir" >/dev/null
assert_log_contains "$check8_cmd_log" \
  "Test file exists: $check8_cmd_test" \
  "Check 8 extracts the .sh path token from a command-wrapped Test Plan cell"
assert_log_not_contains "$check8_cmd_log" \
  "references non-existent file" \
  "Check 8 does not false-BLOCK a command-wrapped .sh test whose file exists"

# BUG-019 managed twins: compound MJS paths must remain whole while ordinary
# suffixes and shell command contexts retain their existing behavior.
echo "Running BUG-019 Check 8 compound-MJS compatibility selftest..."
check8_mjs_dir="$tmp_root/specs/945-check8-compound-mjs"
cp -R "$per_scope_positive_feature_dir" "$check8_mjs_dir"
check8_spec_mjs="$check8_mjs_dir/tests/example.spec.mjs"
check8_test_mjs="$check8_mjs_dir/tests/example.test.mjs"
check8_spec_ts="$check8_mjs_dir/tests/example.spec.ts"
check8_test_js="$check8_mjs_dir/tests/example.test.js"
check8_mjs_shell="$check8_mjs_dir/tests/example.sh"
mkdir -p "$check8_mjs_dir/tests"
printf '%s\n' 'export const compoundSpec = true;' > "$check8_spec_mjs"
printf '%s\n' 'export const compoundTest = true;' > "$check8_test_mjs"
printf '%s\n' 'export const ordinarySpec = true;' > "$check8_spec_ts"
printf '%s\n' 'export const ordinaryTest = true;' > "$check8_test_js"
printf '%s\n' '#!/usr/bin/env bash' 'printf "%s\n" "shell control"' > "$check8_mjs_shell"
chmod +x "$check8_mjs_shell"
cat <<'EOF' > "$check8_mjs_dir/scopes/01-index-parity-proof/scope.md"
# Scope 01: BUG-019 Compound MJS Compatibility

**Status:** Done

### Goal

Prove Check 8 preserves complete compound MJS paths and existing controls.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Regression E2E | `e2e-api` | `__SPEC_MJS__` | Compound spec MJS path remains complete. | `__SPEC_MJS__` | Yes |
| Regression E2E | `e2e-api` | `__TEST_MJS__` | Compound test MJS path remains complete. | `__TEST_MJS__` | Yes |
| Regression E2E | `e2e-api` | `__SPEC_TS__` | Ordinary spec TypeScript control remains complete. | `__SPEC_TS__` | Yes |
| Regression E2E | `e2e-api` | `__TEST_JS__` | Ordinary test JavaScript control remains complete. | `__TEST_JS__` | Yes |
| Regression E2E | `e2e-api` | `bash -n __SHELL__ && shellcheck -x __SHELL__` | Shell wrapper keeps the first accepted path. | `bash __SHELL__` | Yes |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior -> Evidence: report.md#test-evidence
- [x] Broader E2E regression suite passes -> Evidence: report.md#test-evidence
- [x] Documentation route metadata is recorded consistently across artifacts -> Evidence: report.md#summary
EOF
bubbles_sed_inplace "s|__SPEC_MJS__|$check8_spec_mjs|g" "$check8_mjs_dir/scopes/01-index-parity-proof/scope.md"
bubbles_sed_inplace "s|__TEST_MJS__|$check8_test_mjs|g" "$check8_mjs_dir/scopes/01-index-parity-proof/scope.md"
bubbles_sed_inplace "s|__SPEC_TS__|$check8_spec_ts|g" "$check8_mjs_dir/scopes/01-index-parity-proof/scope.md"
bubbles_sed_inplace "s|__TEST_JS__|$check8_test_js|g" "$check8_mjs_dir/scopes/01-index-parity-proof/scope.md"
bubbles_sed_inplace "s|__SHELL__|$check8_mjs_shell|g" "$check8_mjs_dir/scopes/01-index-parity-proof/scope.md"
check8_mjs_log="$tmp_root/check8-compound-mjs.log"
check8_mjs_status="$(run_capture "$check8_mjs_log" bash "$GUARD_SCRIPT" "$check8_mjs_dir")"
if [[ "$check8_mjs_status" -eq 0 ]]; then
  pass "BUG-019 compound-MJS compatibility fixture passes the transition guard"
else
  fail "BUG-019 compound-MJS compatibility fixture should pass the transition guard"
  sed -n '1,220p' "$check8_mjs_log"
fi
assert_log_contains "$check8_mjs_log" "Test file exists: $check8_spec_mjs" "BUG-019 Check 8 preserves the complete .spec.mjs path"
assert_log_contains "$check8_mjs_log" "Test file exists: $check8_test_mjs" "BUG-019 Check 8 preserves the complete .test.mjs path"
assert_log_contains "$check8_mjs_log" "Test file exists: $check8_spec_ts" "BUG-019 Check 8 preserves the ordinary .spec.ts control"
assert_log_contains "$check8_mjs_log" "Test file exists: $check8_test_js" "BUG-019 Check 8 preserves the ordinary .test.js control"
assert_log_contains "$check8_mjs_log" "Test file exists: $check8_mjs_shell" "BUG-019 Check 8 preserves the command-wrapped shell control"
assert_log_not_contains "$check8_mjs_log" "references non-existent file: ${check8_spec_mjs%.mjs}" "BUG-019 Check 8 never checks the shorter .spec prefix"
assert_log_not_contains "$check8_mjs_log" "references non-existent file: ${check8_test_mjs%.mjs}" "BUG-019 Check 8 never checks the shorter .test prefix"

# The negative twin uses an existing complete path so substring extraction
# would become observable, but every declared context is intentionally inert.
echo "Running BUG-019 Check 8 adversarial-context selftest..."
check8_mjs_adversarial_dir="$tmp_root/specs/946-check8-compound-mjs-adversarial"
cp -R "$per_scope_positive_feature_dir" "$check8_mjs_adversarial_dir"
check8_mjs_adversarial_real="$check8_mjs_adversarial_dir/tests/example.spec.mjs"
mkdir -p "$check8_mjs_adversarial_dir/tests"
printf '%s\n' 'export const adversarialControl = true;' > "$check8_mjs_adversarial_real"
cat <<'EOF' > "$check8_mjs_adversarial_dir/scopes/01-index-parity-proof/scope.md"
# Scope 01: BUG-019 Adversarial Contexts

**Status:** Done

### Goal

Prove unsupported suffixes, prose, and unrecognized commands stay inert.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Adversarial Regression E2E | `e2e-api` | `__REAL_MJS__.backup` | Extension-prefix adversary is rejected. | `__REAL_MJS__.backup` | Yes |
| Adversarial Regression E2E | `e2e-api` | `the prose token __REAL_MJS__ is illustrative` | Extension-shaped prose is inert. | `node --test __REAL_MJS__` | Yes |
| Adversarial Regression E2E | `e2e-api` | `node --test __REAL_MJS__` | Unrecognized command wrapper is inert. | `node --test __REAL_MJS__` | Yes |
| Adversarial Regression E2E | `e2e-api` | `bash -c __REAL_MJS__` | Shell command-string syntax is not interpreted. | `bash -c __REAL_MJS__` | Yes |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior -> Evidence: report.md#test-evidence
- [x] Broader E2E regression suite passes -> Evidence: report.md#test-evidence
- [x] Documentation route metadata is recorded consistently across artifacts -> Evidence: report.md#summary
EOF
bubbles_sed_inplace "s|__REAL_MJS__|$check8_mjs_adversarial_real|g" "$check8_mjs_adversarial_dir/scopes/01-index-parity-proof/scope.md"
check8_mjs_adversarial_log="$tmp_root/check8-compound-mjs-adversarial.log"
check8_mjs_adversarial_status="$(run_capture "$check8_mjs_adversarial_log" bash "$GUARD_SCRIPT" "$check8_mjs_adversarial_dir")"
if [[ "$check8_mjs_adversarial_status" -eq 0 ]]; then
  pass "BUG-019 adversarial-context fixture passes without accepting a test path"
else
  fail "BUG-019 adversarial-context fixture should pass without accepting a test path"
  sed -n '1,220p' "$check8_mjs_adversarial_log"
fi
assert_log_contains "$check8_mjs_adversarial_log" \
  "No concrete test file paths found in Test Plan" \
  "BUG-019 invalid contexts reach the no-concrete-path branch"
assert_log_not_contains "$check8_mjs_adversarial_log" \
  "Test file exists:" \
  "BUG-019 invalid contexts never reach the existing-file branch"
assert_log_not_contains "$check8_mjs_adversarial_log" \
  "references non-existent file" \
  "BUG-019 invalid contexts never reach the missing-file branch"

echo "Running positive shared-infrastructure selftest..."
shared_positive_log="$tmp_root/shared-positive-guard.log"
shared_positive_status="$(run_capture "$shared_positive_log" bash "$GUARD_SCRIPT" "$shared_positive_feature_dir")"
if [[ "$shared_positive_status" -eq 0 ]]; then
  pass "Shared-infrastructure positive fixture passes the transition guard"
else
  fail "Shared-infrastructure positive fixture should pass the transition guard"
  sed -n '1,260p' "$shared_positive_log"
fi
assert_log_contains "$shared_positive_log" "Shared Infrastructure Impact Sweep section" "Positive shared fixture exercises guard Check 8C"
assert_log_contains "$shared_positive_log" "Change Boundary section" "Positive shared fixture exercises guard Check 8D"

echo "Running negative shared-infrastructure selftest..."
shared_negative_log="$tmp_root/shared-negative-guard.log"
shared_negative_status="$(run_capture "$shared_negative_log" bash "$GUARD_SCRIPT" "$shared_negative_feature_dir")"
if [[ "$shared_negative_status" -ne 0 ]]; then
  pass "Negative shared-infrastructure fixture fails the transition guard as expected"
else
  fail "Negative shared-infrastructure fixture should fail the transition guard"
  sed -n '1,260p' "$shared_negative_log"
fi
assert_log_contains "$shared_negative_log" "has no Shared Infrastructure Impact Sweep section" "Negative shared fixture triggers the blast-radius planning check"
assert_log_contains "$shared_negative_log" "has no Change Boundary section" "Negative shared fixture triggers the change-boundary check"

echo "Running BUG-006 header-blockquote status selftest (Check 4B/5 ignore summary blockquotes)..."
bug006_pass_log="$tmp_root/bug006-blockquote-pass.log"
bug006_pass_status="$(run_capture "$bug006_pass_log" bash "$GUARD_SCRIPT" "$bug006_blockquote_pass_dir")"
if [[ "$bug006_pass_status" -eq 0 ]]; then
  pass "BUG-006: fixture with a header '> **Status:** …' summary blockquote still passes the transition guard"
else
  fail "BUG-006: fixture with a header '> **Status:** …' summary blockquote should still pass the transition guard"
  sed -n '1,220p' "$bug006_pass_log"
fi
assert_log_not_contains "$bug006_pass_log" "Non-canonical scope status detected" "BUG-006: header summary blockquote is not flagged as a non-canonical scope status (Check 4B)"
assert_log_not_contains "$bug006_pass_log" "still marked 'Not Started'" "BUG-006: header summary blockquote does not inflate the Not Started scope count (Check 5)"
assert_log_contains "$bug006_pass_log" "All scope statuses are canonical" "BUG-006: real plain scope statuses still validated as canonical"

echo "Running BUG-006 non-canonical plain-status selftest (no over-exclusion)..."
bug006_neg_log="$tmp_root/bug006-noncanonical-neg.log"
bug006_neg_status="$(run_capture "$bug006_neg_log" bash "$GUARD_SCRIPT" "$bug006_noncanonical_neg_dir")"
if [[ "$bug006_neg_status" -ne 0 ]]; then
  pass "BUG-006: a plain non-canonical scope status still fails the transition guard"
else
  fail "BUG-006: a plain non-canonical scope status should still fail the transition guard"
  sed -n '1,220p' "$bug006_neg_log"
fi
assert_log_contains "$bug006_neg_log" "Non-canonical scope status detected" "BUG-006: a plain '**Status:** Deferred' scope line is STILL flagged non-canonical (no over-exclusion)"

echo "Running BUG-007 benign session/flow selftest (Check 8C not over-triggered)..."
bug007_benign_log="$tmp_root/bug007-benign.log"
bug007_benign_status="$(run_capture "$bug007_benign_log" bash "$GUARD_SCRIPT" "$bug007_benign_dir")"
if [[ "$bug007_benign_status" -eq 0 ]]; then
  pass "BUG-007: benign 'session'+'flow' prose fixture still passes the transition guard"
else
  fail "BUG-007: benign 'session'+'flow' prose fixture should still pass the transition guard"
  sed -n '1,220p' "$bug007_benign_log"
fi
assert_log_not_contains "$bug007_benign_log" "has no Shared Infrastructure Impact Sweep section" "BUG-007: benign 'session'+'flow' prose does not trigger the shared-infra blast-radius check (Check 8C)"

# Check 6 / Check 6B — dict-shaped completedPhaseClaims must NOT crash the guard.
# Regression for `TypeError: cannot use 'dict' as a dict key`. We assert on the
# Check 6 / 6B log content only; the fixture's overall exit may be non-zero for
# unrelated ceiling reasons (mirrors the G040 fixture convention above).
echo "Running Check 6/6B dict-shaped phase-claim regression selftest..."
dict_phase_claims_log="$tmp_root/dict-phase-claims.log"
run_capture "$dict_phase_claims_log" bash "$GUARD_SCRIPT" "$dict_phase_claims_dir" >/dev/null
assert_log_not_contains "$dict_phase_claims_log" "Traceback (most recent call last)" "Check 6/6B: dict-shaped completedPhaseClaims does NOT crash the guard with a Python Traceback"
assert_log_not_contains "$dict_phase_claims_log" "unhashable type: 'dict'" "Check 6/6B: the unhashable-dict TypeError is not raised on dict-shaped completedPhaseClaims"
assert_log_contains "$dict_phase_claims_log" "Required phase 'validate' recorded in execution/certification phase records" "Check 6: phase name 'validate' is read OUT of the dict-shaped completedPhaseClaims (empty certifiedCompletedPhases)"
assert_log_contains "$dict_phase_claims_log" "Required phase 'audit' recorded in execution/certification phase records" "Check 6: phase name 'audit' is read OUT of the dict-shaped completedPhaseClaims (empty certifiedCompletedPhases)"
assert_log_contains "$dict_phase_claims_log" "Phase 'validate' has specialist provenance from bubbles.validate" "Check 6B: dict-shaped claim is normalized to its phase name and validated for provenance (not silently swallowed)"

echo "Running negative packet-field selftest..."
negative_log="$tmp_root/negative-guard.log"
negative_status="$(run_capture "$negative_log" bash "$GUARD_SCRIPT" "$negative_feature_dir")"
if [[ "$negative_status" -ne 0 ]]; then
  pass "Negative fixture fails the transition guard as expected"
else
  fail "Negative fixture should fail the transition guard"
  sed -n '1,220p' "$negative_log"
fi
assert_log_contains "$negative_log" "missing a concrete owning specialist" "Negative fixture triggers the concrete owner packet check"
assert_log_contains "$negative_log" "Gate G063" "Negative fixture reports the new concrete-result gate"
assert_log_not_contains "$negative_log" "unbound variable" "BUG-022 genuine failure does not abort under nounset"
assert_log_contains "$negative_log" "BEGIN TRANSITION_GUARD_RESULT_V1" "BUG-022 genuine failure emits a result start"
assert_log_contains "$negative_log" "END TRANSITION_GUARD_RESULT_V1" "BUG-022 genuine failure emits a result end"
assert_log_contains "$negative_log" "exitStatus: 1" "BUG-022 genuine failure preserves a nonzero structured exit"
assert_log_contains "$negative_log" "verdict: FAIL" "BUG-022 genuine failure preserves the failing verdict"

echo "Running workflowMode contradiction selftest..."
workflow_mode_log="$tmp_root/workflow-mode.log"
workflow_mode_status="$(run_capture "$workflow_mode_log" bash "$GUARD_SCRIPT" "$workflow_mode_negative_feature_dir")"
if [[ "$workflow_mode_status" -ne 0 ]]; then
  pass "workflowMode contradiction fixture fails the transition guard as expected"
else
  fail "workflowMode contradiction fixture should fail the transition guard"
  sed -n '1,220p' "$workflow_mode_log"
fi
assert_log_contains "$workflow_mode_log" "E009-STATE-MODE-MISMATCH" "Contradictory workflow metadata fails loud through the S02 contract"
assert_log_contains "$workflow_mode_log" "verdict: BLOCKED" "Contradictory workflow metadata emits a blocked transition result"

echo "Running product-to-planning ceiling selftest..."
planning_negative_log="$tmp_root/product-planning-negative.log"
planning_negative_status="$(run_capture "$planning_negative_log" bash "$GUARD_SCRIPT" "$planning_done_negative_feature_dir")"
if [[ "$planning_negative_status" -ne 0 ]]; then
  pass "product-to-planning/done fixture fails the transition guard as expected"
else
  fail "product-to-planning/done fixture should fail the transition guard"
  sed -n '1,220p' "$planning_negative_log"
fi
assert_log_contains "$planning_negative_log" "E009-TARGET-MISMATCH" "Planning-only mode blocks done status through the registry-derived contract"
assert_log_contains "$planning_negative_log" "blockingCode: E009-TARGET-MISMATCH" "Planning done contradiction is machine-readable"

planning_lint_log="$tmp_root/product-planning-artifact-lint-negative.log"
planning_lint_status="$(run_capture "$planning_lint_log" bash "$SCRIPT_DIR/artifact-lint.sh" "$planning_done_negative_feature_dir")"
if [[ "$planning_lint_status" -ne 0 ]]; then
  pass "artifact-lint blocks product-to-planning/done fixture as expected"
else
  fail "artifact-lint should block product-to-planning/done fixture"
  sed -n '1,220p' "$planning_lint_log"
fi
assert_log_contains "$planning_lint_log" "Workflow mode 'product-to-planning' ceiling is 'specs_hardened', NOT 'done'" "Artifact lint uses registry ceiling for product-to-planning"

planning_positive_log="$tmp_root/product-planning-positive.log"
planning_positive_status="$(run_capture "$planning_positive_log" bash "$GUARD_SCRIPT" "$planning_specs_hardened_positive_feature_dir")"
if [[ "$planning_positive_status" -eq 0 ]]; then
  pass "product-to-planning/specs_hardened fixture passes the transition guard"
else
  fail "product-to-planning/specs_hardened fixture should pass the transition guard"
  sed -n '1,260p' "$planning_positive_log"
fi
assert_log_contains "$planning_positive_log" "Workflow mode 'product-to-planning' permits current status 'specs_hardened'" "Planning-only mode permits specs_hardened status"
assert_log_contains "$planning_positive_log" "planMaturityOnly=true is not claiming delivery-done status" "planMaturityOnly is allowed below done"

echo "Running BUG-009 S03 guard profile activation matrix..."
s03_not_applicable='[Check-4-completion,Check-5-all-done,Check-8-file-existence,Check-11-execution-evidence]'

s03_planning_log="$tmp_root/s03-planning-pass.log"
s03_planning_status="$(run_capture "$s03_planning_log" bash "$GUARD_SCRIPT" "$s03_planning_feature_dir")"
if [[ "$s03_planning_status" -eq 0 ]]; then
  pass "BUG-009 S03: honest product-to-planning packet passes via legacy one-argument invocation"
else
  fail "BUG-009 S03: honest product-to-planning packet should pass"
  sed -n '1,260p' "$s03_planning_log"
fi
assert_transition_result "$s03_planning_log" \
  product-to-planning planning-maturity-v1 specs_hardened "$s03_not_applicable" PASS 0 \
  "BUG-009 S03: planning success emits one complete ordered transition result"
assert_log_contains "$s03_planning_log" "NOT_APPLICABLE: Check-4-completion" "BUG-009 S03: unchecked implementation DoD is explicitly non-applicable"
assert_log_contains "$s03_planning_log" "NOT_APPLICABLE: Check-5-all-done" "BUG-009 S03: incomplete implementation scopes are explicitly non-applicable"
assert_log_contains "$s03_planning_log" "NOT_APPLICABLE: Check-8-file-existence" "BUG-009 S03: future test file presence is explicitly non-applicable"
assert_log_contains "$s03_planning_log" "NOT_APPLICABLE: Check-11-execution-evidence" "BUG-009 S03: honest unimplemented reports need no delivery evidence block"
assert_log_contains "$s03_planning_log" "--- Check 4A:" "BUG-009 S03: Check 4A remains active under planning"
assert_log_contains "$s03_planning_log" "--- Check 4B:" "BUG-009 S03: Check 4B remains active under planning"
assert_log_contains "$s03_planning_log" "--- Check 5B:" "BUG-009 S03: Check 5B remains active under planning"
assert_log_contains "$s03_planning_log" "--- Check 5C:" "BUG-009 S03: Check 5C remains active under planning"
assert_log_contains "$s03_planning_log" "--- Check 8A:" "BUG-009 S03: Check 8A remains active under planning"
assert_log_contains "$s03_planning_log" "--- Check 8B:" "BUG-009 S03: Check 8B remains active under planning"
assert_log_contains "$s03_planning_log" "--- Check 8C:" "BUG-009 S03: Check 8C remains active under planning"
assert_log_contains "$s03_planning_log" "--- Check 8D:" "BUG-009 S03: Check 8D remains active under planning"
assert_log_contains "$s03_planning_log" "--- Check 9:" "BUG-009 S03: checked-item evidence audit remains active under planning"
assert_log_contains "$s03_planning_log" "No undeclared source code edits detected" "BUG-009 S03: G073 remains active and clean"
assert_log_contains "$s03_planning_log" "Gherkin scenarios have faithful DoD items" "BUG-009 S03: G068 remains active and clean"

s03_hardening_log="$tmp_root/s03-hardening-pass.log"
s03_hardening_status="$(run_capture "$s03_hardening_log" bash "$GUARD_SCRIPT" "$s03_hardening_feature_dir")"
if [[ "$s03_hardening_status" -eq 0 ]]; then
  pass "BUG-009 S03: honest spec-scope-hardening packet passes"
else
  fail "BUG-009 S03: honest spec-scope-hardening packet should pass"
  sed -n '1,260p' "$s03_hardening_log"
fi
assert_transition_result "$s03_hardening_log" \
  spec-scope-hardening planning-maturity-v1 specs_hardened "$s03_not_applicable" PASS 0 \
  "BUG-009 S03: both designed planning modes share the same explicit profile contract"

s03_contract_json="$tmp_root/s03-planning-contract.json"
bash "$SCRIPT_DIR/transition-contract-resolver.sh" "$s03_planning_feature_dir" > "$s03_contract_json"
s03_contract_digest="$(jq -r '.contractDigest' "$s03_contract_json")"
s03_assertions_log="$tmp_root/s03-matching-assertions.log"
s03_assertions_status="$(run_capture "$s03_assertions_log" bash "$GUARD_SCRIPT" "$s03_planning_feature_dir" \
  --target-status specs_hardened \
  --expect-workflow-mode product-to-planning \
  --expect-contract-digest "$s03_contract_digest")"
if [[ "$s03_assertions_status" -eq 0 ]]; then
  pass "BUG-009 S03: matching target, mode, and digest assertions preserve the derived planning contract"
else
  fail "BUG-009 S03: matching assertion-only flags should pass"
fi
assert_log_contains "$s03_assertions_log" "contractDigest: $s03_contract_digest" "BUG-009 S03: assertion flags cannot replace the registry-derived digest"
assert_transition_result "$s03_assertions_log" \
  product-to-planning planning-maturity-v1 specs_hardened "$s03_not_applicable" PASS 0 \
  "BUG-009 S03: assertion-only invocation emits the same result contract"

s03_target_mismatch_log="$tmp_root/s03-target-mismatch.log"
s03_target_mismatch_status="$(run_capture "$s03_target_mismatch_log" bash "$GUARD_SCRIPT" "$s03_planning_feature_dir" --target-status "done")"
if [[ "$s03_target_mismatch_status" -eq 2 ]]; then
  pass "BUG-009 S03: mismatched target assertion blocks with guard exit 2"
else
  fail "BUG-009 S03: mismatched target assertion should exit 2 (observed $s03_target_mismatch_status)"
fi
assert_log_contains "$s03_target_mismatch_log" "E009-TARGET-MISMATCH" "BUG-009 S03: target assertion mismatch preserves S02 E009 semantics"
assert_transition_result "$s03_target_mismatch_log" \
  UNRESOLVED UNRESOLVED UNRESOLVED '[]' BLOCKED 2 \
  "BUG-009 S03: target mismatch emits one complete blocked result"

s03_digest_mismatch_log="$tmp_root/s03-digest-mismatch.log"
s03_digest_mismatch_status="$(run_capture "$s03_digest_mismatch_log" bash "$GUARD_SCRIPT" "$s03_planning_feature_dir" \
  --expect-contract-digest "sha256:0000000000000000000000000000000000000000000000000000000000000000")"
if [[ "$s03_digest_mismatch_status" -eq 2 ]]; then
  pass "BUG-009 S03: stale digest assertion blocks with guard exit 2"
else
  fail "BUG-009 S03: stale digest assertion should exit 2 (observed $s03_digest_mismatch_status)"
fi
assert_log_contains "$s03_digest_mismatch_log" "E009-TARGET-MISMATCH" "BUG-009 S03: stale digest mismatch preserves S02 E009 semantics"
assert_transition_result "$s03_digest_mismatch_log" \
  UNRESOLVED UNRESOLVED UNRESOLVED '[]' BLOCKED 2 \
  "BUG-009 S03: digest mismatch cannot omit or malform the blocked result"

s03_profile_flag_log="$tmp_root/s03-profile-flag.log"
s03_profile_flag_status="$(run_capture "$s03_profile_flag_log" bash "$GUARD_SCRIPT" "$s03_planning_feature_dir" --profile planning-maturity-v1)"
if [[ "$s03_profile_flag_status" -eq 2 ]]; then
  pass "BUG-009 S03: caller-selected profile syntax is rejected"
else
  fail "BUG-009 S03: caller-selected profile syntax should exit 2 (observed $s03_profile_flag_status)"
fi
assert_log_contains "$s03_profile_flag_log" "E009-USAGE" "BUG-009 S03: policy-selecting flags fail loud"
assert_transition_result "$s03_profile_flag_log" \
  UNRESOLVED UNRESOLVED UNRESOLVED '[]' BLOCKED 2 \
  "BUG-009 S03: rejected profile syntax still emits the mandatory blocked result"

s03_resolver_once_root="$tmp_root/s03-resolver-once-framework"
clone_framework_surface "$s03_resolver_once_root"
s03_resolver_once_feature="$s03_resolver_once_root/specs/001-resolver-once"
emit_honest_planning_fixture "$s03_resolver_once_feature"
mv "$s03_resolver_once_root/bubbles/scripts/transition-contract-resolver.sh" \
  "$s03_resolver_once_root/bubbles/scripts/transition-contract-resolver.real.sh"
cat <<'EOF' > "$s03_resolver_once_root/bubbles/scripts/transition-contract-resolver.sh"
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
: "${BUBBLES_TRANSITION_RESOLVER_COUNT_FILE:?count file required}"
printf '%s\n' call >> "$BUBBLES_TRANSITION_RESOLVER_COUNT_FILE"
exec bash "$SCRIPT_DIR/transition-contract-resolver.real.sh" "$@"
EOF
s03_resolver_count_file="$tmp_root/s03-resolver-count.txt"
s03_resolver_once_log="$tmp_root/s03-resolver-once.log"
s03_resolver_once_status="$(run_capture "$s03_resolver_once_log" env \
  BUBBLES_TRANSITION_RESOLVER_COUNT_FILE="$s03_resolver_count_file" \
  bash "$s03_resolver_once_root/bubbles/scripts/state-transition-guard.sh" "$s03_resolver_once_feature")"
if [[ "$s03_resolver_once_status" -eq 0 \
  && "$(wc -l < "$s03_resolver_count_file" | tr -d '[:space:]')" -eq 1 ]]; then
  pass "BUG-009 S03: guard resolves the transition contract exactly once per invocation"
else
  fail "BUG-009 S03: guard must resolve exactly once (exit=$s03_resolver_once_status count=$(wc -l < "$s03_resolver_count_file" | tr -d '[:space:]'))"
fi

s03_delivery_log="$tmp_root/s03-delivery-negative.log"
s03_delivery_status="$(run_capture "$s03_delivery_log" bash "$GUARD_SCRIPT" "$s03_delivery_negative_dir")"
if [[ "$s03_delivery_status" -eq 1 ]]; then
  pass "BUG-009 S03: the honest incomplete packet fails under done-ceiling delivery semantics"
else
  fail "BUG-009 S03: done-ceiling negative control should exit 1 (observed $s03_delivery_status)"
  sed -n '1,260p' "$s03_delivery_log"
fi
assert_log_contains "$s03_delivery_log" "UNCHECKED DoD items" "BUG-009 S03: delivery Check 4 completion remains blocking"
assert_log_contains "$s03_delivery_log" "still marked 'Not Started'" "BUG-009 S03: delivery Check 5 all-Done remains blocking"
assert_log_contains "$s03_delivery_log" "Test Plan references non-existent file" "BUG-009 S03: delivery Check 8 file existence remains blocking"
assert_log_contains "$s03_delivery_log" "has ZERO evidence code blocks" "BUG-009 S03: delivery Check 11 execution evidence remains blocking"
assert_log_not_contains "$s03_delivery_log" "NOT_APPLICABLE: Check-" "BUG-009 S03: delivery mode receives no planning exemption"
assert_transition_result "$s03_delivery_log" \
  autonomous-goal delivery-completion-v1 "done" '[]' FAIL 1 \
  "BUG-009 S03: done-mode negative control emits one complete delivery failure result"

s03_delivery_checked_log="$tmp_root/s03-delivery-checked.log"
run_capture "$s03_delivery_checked_log" bash "$GUARD_SCRIPT" "$s03_delivery_checked_dir" >/dev/null
assert_log_contains "$s03_delivery_checked_log" "DoD item [x] has NO evidence block" "BUG-009 S03: delivery Check 9 checked-item evidence remains blocking"

echo "Running BUG-026 G060 profile-awareness matrix (Check 3E honors the audit profile)..."
# Case 1 — planning-maturity exemption: a product-to-planning/specs_hardened packet
# whose policySnapshot now declares tdd.mode=scenario-first but carries NO RED->GREEN
# markers. Before the fix Check 3E demanded RED->GREEN and blocked this planning
# transition; after the fix the planning-maturity-v1 profile makes Check 3E
# NOT_APPLICABLE, so G060 no longer blocks plan hardening.
g060_planning_log="$tmp_root/g060-planning-not-applicable.log"
g060_planning_status="$(run_capture "$g060_planning_log" bash "$GUARD_SCRIPT" "$g060_planning_na_dir")"
if [[ "$g060_planning_status" -eq 0 ]]; then
  pass "BUG-026 G060: planning-maturity transition with scenario-first tdd and no RED→GREEN is not blocked by Check 3E"
else
  fail "BUG-026 G060: planning-maturity transition should not be blocked by Check 3E (observed $g060_planning_status)"
  sed -n '1,260p' "$g060_planning_log"
fi
assert_log_contains "$g060_planning_log" "NOT_APPLICABLE: Check-3E scenario-first TDD evidence" "BUG-026 G060: Check 3E is explicitly non-applicable under planning maturity"
assert_log_not_contains "$g060_planning_log" "no RED→GREEN ordering was found" "BUG-026 G060: planning maturity emits no scenario-first enforcement failure"
assert_transition_result "$g060_planning_log" \
  product-to-planning planning-maturity-v1 specs_hardened "$s03_not_applicable" PASS 0 \
  "BUG-026 G060: planning maturity with scenario-first tdd still emits one complete passing result"

# Case 2 — enforcement intact (regression guard): an autonomous-goal/done packet
# that resolves delivery-completion-v1, is snapshot-bearing, and now declares
# tdd.mode=scenario-first with NO RED->GREEN markers. Check 3E MUST still fail G060
# — the fix does not weaken delivery enforcement. The fixture is otherwise clean,
# so G060 is the isolated blocking gate.
g060_delivery_log="$tmp_root/g060-delivery-enforced.log"
g060_delivery_status="$(run_capture "$g060_delivery_log" bash "$GUARD_SCRIPT" "$g060_delivery_enforced_dir")"
if [[ "$g060_delivery_status" -eq 1 ]]; then
  pass "BUG-026 G060: delivery-completion transition with scenario-first tdd and no RED→GREEN still fails Check 3E"
else
  fail "BUG-026 G060: delivery-completion regression control should exit 1 (observed $g060_delivery_status)"
  sed -n '1,260p' "$g060_delivery_log"
fi
assert_log_contains "$g060_delivery_log" "no RED→GREEN ordering was found" "BUG-026 G060: delivery completion still enforces scenario-first RED→GREEN evidence"
assert_log_not_contains "$g060_delivery_log" "NOT_APPLICABLE: Check-3E" "BUG-026 G060: delivery completion receives no planning-maturity Check 3E exemption"
assert_log_contains "$g060_delivery_log" "failedGateIds: [G060]" "BUG-026 G060: G060 is the isolated blocking gate for delivery completion"
assert_transition_result "$g060_delivery_log" \
  autonomous-goal delivery-completion-v1 "done" '[]' FAIL 1 \
  "BUG-026 G060: delivery enforcement failure emits one complete blocked-by-gate result"

echo "Running G040 Check 18 planning-maturity exemption (deferral scan honors the audit profile)..."
# Planning-maturity exemption: a product-to-planning/specs_hardened packet whose
# scope carries a forward-looking domain label ("Authorized Outcome Follow-Up")
# that the context-free deferral regex would otherwise flag. Under planning
# maturity Check 18 is NOT_APPLICABLE, so G040 no longer blocks plan hardening.
# Delivery-side G040 enforcement remains covered by the g040_pos_* cases below.
g040_planning_log="$tmp_root/g040-planning-not-applicable.log"
g040_planning_status="$(run_capture "$g040_planning_log" bash "$GUARD_SCRIPT" "$g040_planning_na_dir")"
if [[ "$g040_planning_status" -eq 0 ]]; then
  pass "G040 Check 18: planning-maturity packet with a forward-looking domain label (Authorized Outcome Follow-Up) is not blocked by the deferral scan"
else
  fail "G040 Check 18: planning-maturity should not be blocked by the deferral scan (observed $g040_planning_status)"
  sed -n '1,260p' "$g040_planning_log"
fi
assert_log_contains "$g040_planning_log" "NOT_APPLICABLE: Check-18 deferral-language scan" "G040 Check 18: deferral scan is explicitly non-applicable under planning maturity"
assert_log_not_contains "$g040_planning_log" "deferral language hit" "G040 Check 18: planning maturity emits no deferral enforcement failure"
assert_transition_result "$g040_planning_log" \
  product-to-planning planning-maturity-v1 specs_hardened "$s03_not_applicable" PASS 0 \
  "G040 Check 18: planning maturity with a forward-looking domain label still emits one complete passing result"

s03_checked_log="$tmp_root/s03-checked-evidence.log"
s03_checked_status="$(run_capture "$s03_checked_log" bash "$GUARD_SCRIPT" "$s03_checked_evidence_dir")"
if [[ "$s03_checked_status" -eq 1 ]]; then
  pass "BUG-009 S03: checked planning DoD without evidence fails"
else
  fail "BUG-009 S03: checked planning DoD without evidence should exit 1"
fi
assert_log_contains "$s03_checked_log" "DoD item [x] has NO evidence block" "BUG-009 S03: Check 9 honesty remains universal"
assert_transition_result "$s03_checked_log" \
  product-to-planning planning-maturity-v1 specs_hardened "$s03_not_applicable" FAIL 1 \
  "BUG-009 S03: planning honesty failure retains explicit non-applicable delivery checks"

s03_done_log="$tmp_root/s03-done-honesty.log"
s03_done_status="$(run_capture "$s03_done_log" bash "$GUARD_SCRIPT" "$s03_done_honesty_dir")"
if [[ "$s03_done_status" -eq 1 ]]; then
  pass "BUG-009 S03: falsely Done planning scope fails"
else
  fail "BUG-009 S03: falsely Done planning scope should exit 1"
fi
assert_log_contains "$s03_done_log" "Planning scope claims Done while unchecked DoD remain" "BUG-009 S03: planning status honesty remains blocking"

s03_g068_log="$tmp_root/s03-g068.log"
s03_g068_status="$(run_capture "$s03_g068_log" bash "$GUARD_SCRIPT" "$s03_g068_dir")"
if [[ "$s03_g068_status" -eq 1 ]]; then
  pass "BUG-009 S03: broken Gherkin-to-DoD fidelity fails planning guard"
else
  fail "BUG-009 S03: G068 adversary should exit 1"
fi
assert_log_contains "$s03_g068_log" "DoD-Gherkin content fidelity gap" "BUG-009 S03: G068 failure is visible and not hidden by delivery non-applicability"
assert_log_contains "$s03_g068_log" "failedGateIds: [G068]" "BUG-009 S03: G068 is machine-readable in the result ledger"

s03_planning_revert_log="$tmp_root/s03-planning-revert.log"
run_capture "$s03_planning_revert_log" bash "$GUARD_SCRIPT" "$s03_g068_dir" --revert-on-fail >/dev/null
if [[ "$(jq -r '.status' "$s03_g068_dir/state.json")" == "specs_hardened" \
  && "$(jq -r '.certification.status' "$s03_g068_dir/state.json")" == "specs_hardened" ]]; then
  pass "BUG-009 S03: --revert-on-fail does not rewrite planning state"
else
  fail "BUG-009 S03: planning --revert-on-fail must leave specs_hardened state unchanged"
fi
assert_log_contains "$s03_planning_revert_log" "--revert-on-fail is delivery-only" "BUG-009 S03: planning reversion refusal is explicit"

s03_delivery_revert_dir="$tmp_root/specs/945-bug009-s03-delivery-revert"
cp -R "$s03_delivery_negative_dir" "$s03_delivery_revert_dir"
set_fixture_contract "$s03_delivery_revert_dir/state.json" autonomous-goal "done"
s03_delivery_revert_log="$tmp_root/s03-delivery-revert.log"
run_capture "$s03_delivery_revert_log" bash "$GUARD_SCRIPT" "$s03_delivery_revert_dir" --revert-on-fail >/dev/null
if [[ "$(jq -r '.status' "$s03_delivery_revert_dir/state.json")" == "in_progress" \
  && "$(jq -r '.certification.status' "$s03_delivery_revert_dir/state.json")" == "in_progress" \
  && "$(jq -c '.certification.certifiedCompletedPhases' "$s03_delivery_revert_dir/state.json")" == "[]" ]]; then
  pass "BUG-009 S03: delivery --revert-on-fail retains its state rollback behavior"
else
  fail "BUG-009 S03: delivery --revert-on-fail did not restore in_progress and clear completion claims"
fi

s03_g073_root="$tmp_root/s03-g073-repo"
s03_g073_feature="$s03_g073_root/specs/001-g073-source-lockout"
emit_honest_planning_fixture "$s03_g073_feature"
git -C "$s03_g073_root" init -q
git -C "$s03_g073_root" add -f specs
git -C "$s03_g073_root" -c user.name='Bubbles Selftest' -c user.email='bubbles-selftest@example.invalid' \
  commit -q -m 'test: seed planning fixture'
mkdir -p "$s03_g073_root/runtime"
printf '%s\n' 'print("undeclared source edit")' > "$s03_g073_root/runtime/undeclared.py"
git -C "$s03_g073_root" add -f runtime/undeclared.py
s03_g073_log="$tmp_root/s03-g073.log"
s03_g073_status="$(run_capture "$s03_g073_log" bash "$GUARD_SCRIPT" "$s03_g073_feature")"
if [[ "$s03_g073_status" -eq 1 ]]; then
  pass "BUG-009 S03: G073 source-edit adversary blocks planning guard"
else
  fail "BUG-009 S03: G073 source-edit adversary should exit 1"
fi
assert_log_contains "$s03_g073_log" "forbids source code edits, but staged file modified: runtime/undeclared.py" "BUG-009 S03: G073 reports the concrete source edit"
assert_log_contains "$s03_g073_log" "blockingCode: SOURCE_EDIT_LOCKOUT" "BUG-009 S03: G073 maps to the source-lockout blocking code"

s03_planning_gates_root="$tmp_root/s03-planning-gates-framework"
clone_framework_surface "$s03_planning_gates_root"
s03_g087_feature="$s03_planning_gates_root/specs/001-g087-linkage-negative"
s03_g091_feature="$s03_planning_gates_root/specs/002-g091-chain-negative"
emit_honest_planning_fixture "$s03_g087_feature"
emit_honest_planning_fixture "$s03_g091_feature"
remove_planning_only_linkage "$s03_g087_feature/state.json"
git -C "$s03_planning_gates_root" init -q
git -C "$s03_planning_gates_root" add -f bubbles agents specs
git -C "$s03_planning_gates_root" -c user.name='Bubbles Selftest' -c user.email='bubbles-selftest@example.invalid' \
  commit -q -m 'test: seed planning gate fixtures'

s03_g087_log="$tmp_root/s03-g087.log"
s03_g087_status="$(run_capture "$s03_g087_log" env \
  BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=0 \
  BUBBLES_REPO_ROOT="$s03_planning_gates_root" \
  bash "$s03_planning_gates_root/bubbles/scripts/state-transition-guard.sh" "$s03_g087_feature")"
if [[ "$s03_g087_status" -eq 1 ]]; then
  pass "BUG-009 S03: G087 linkage adversary blocks the real planning guard"
else
  fail "BUG-009 S03: G087 linkage adversary should exit 1 (observed $s03_g087_status)"
fi
assert_log_contains "$s03_g087_log" "Planning packet implementation linkage failed — Gate G087" "BUG-009 S03: G087 remains active under planning profile"
assert_transition_list_contains "$s03_g087_log" failedGateIds G087 "BUG-009 S03: G087 failure is machine-readable"

printf '%s\n' 'Fallback route: invoke bubbles.design -> bubbles.plan when planning artifacts are missing.' \
  >> "$s03_planning_gates_root/agents/bubbles.workflow.agent.md"
git -C "$s03_planning_gates_root" add -f agents/bubbles.workflow.agent.md
git -C "$s03_planning_gates_root" -c user.name='Bubbles Selftest' -c user.email='bubbles-selftest@example.invalid' \
  commit -q -m 'test: inject G091 planning-chain adversary'
s03_g091_log="$tmp_root/s03-g091.log"
s03_g091_status="$(run_capture "$s03_g091_log" env \
  BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=0 \
  BUBBLES_REPO_ROOT="$s03_planning_gates_root" \
  bash "$s03_planning_gates_root/bubbles/scripts/state-transition-guard.sh" "$s03_g091_feature")"
if [[ "$s03_g091_status" -eq 1 ]]; then
  pass "BUG-009 S03: G091 chain adversary blocks the real planning guard"
else
  fail "BUG-009 S03: G091 chain adversary should exit 1 (observed $s03_g091_status)"
fi
assert_log_contains "$s03_g091_log" "Planning workflow chain guard failed — Gate G091" "BUG-009 S03: G091 remains active under planning profile"
assert_transition_list_contains "$s03_g091_log" failedGateIds G091 "BUG-009 S03: G091 failure is machine-readable"

echo "Running positive per-scope parity selftest..."
per_scope_positive_log="$tmp_root/per-scope-positive.log"
per_scope_positive_status="$(run_capture "$per_scope_positive_log" bash "$GUARD_SCRIPT" "$per_scope_positive_feature_dir")"
if [[ "$per_scope_positive_status" -eq 0 ]]; then
  pass "Per-scope positive fixture passes the transition guard"
else
  fail "Per-scope positive fixture should pass the transition guard"
  sed -n '1,260p' "$per_scope_positive_log"
fi
assert_log_contains "$per_scope_positive_log" "_index.md statuses match scope.md statuses" "Positive per-scope fixture exercises Check 5B"
assert_log_contains "$per_scope_positive_log" "All completedScopes entries map to real scope artifacts" "Positive per-scope fixture exercises Check 5C"

echo "Running negative _index parity selftest..."
index_parity_log="$tmp_root/index-parity.log"
index_parity_status="$(run_capture "$index_parity_log" bash "$GUARD_SCRIPT" "$index_parity_negative_feature_dir")"
if [[ "$index_parity_status" -ne 0 ]]; then
  pass "Negative _index parity fixture fails the transition guard as expected"
else
  fail "Negative _index parity fixture should fail the transition guard"
  sed -n '1,260p' "$index_parity_log"
fi
assert_log_contains "$index_parity_log" "_index.md says" "Negative per-scope fixture triggers Check 5B"

echo "Running negative phantom scope selftest..."
phantom_scope_log="$tmp_root/phantom-scope.log"
phantom_scope_status="$(run_capture "$phantom_scope_log" bash "$GUARD_SCRIPT" "$phantom_scope_negative_feature_dir")"
if [[ "$phantom_scope_status" -ne 0 ]]; then
  pass "Negative phantom scope fixture fails the transition guard as expected"
else
  fail "Negative phantom scope fixture should fail the transition guard"
  sed -n '1,260p' "$phantom_scope_log"
fi
assert_log_contains "$phantom_scope_log" "Phantom scope in completedScopes" "Negative per-scope fixture triggers Check 5C"

echo "Running executionHistory plausibility selftest..."
execution_history_log="$tmp_root/execution-history.log"
execution_history_status="$(run_capture "$execution_history_log" bash "$GUARD_SCRIPT" "$execution_history_negative_feature_dir")"
if [[ "$execution_history_status" -ne 0 ]]; then
  pass "Implausible executionHistory fixture fails the transition guard as expected"
else
  fail "Implausible executionHistory fixture should fail the transition guard"
  sed -n '1,260p' "$execution_history_log"
fi
assert_log_contains "$execution_history_log" "identical 900s intervals" "Negative fixture triggers Check 7A"

echo "Running lockdown round consistency selftest..."
lockdown_round_log="$tmp_root/lockdown-round.log"
lockdown_round_status="$(run_capture "$lockdown_round_log" bash "$GUARD_SCRIPT" "$lockdown_round_negative_feature_dir")"
if [[ "$lockdown_round_status" -ne 0 ]]; then
  pass "Lockdown round mismatch fixture fails the transition guard as expected"
else
  fail "Lockdown round mismatch fixture should fail the transition guard"
  sed -n '1,260p' "$lockdown_round_log"
fi
assert_log_contains "$lockdown_round_log" "lockdownState.round=3" "Negative fixture triggers Check 7B"

echo "Running negative workflow-runner-authorization selftest..."
g064_log="$tmp_root/g064-guard.log"
g064_timeout_seconds="${BUBBLES_G064_SELFTEST_TIMEOUT_SECONDS:-120}"
g064_status="$(run_capture "$g064_log" bubbles_run_with_timeout "$g064_timeout_seconds" env BUBBLES_REPO_ROOT="$g064_framework_root" bash "$g064_framework_root/bubbles/scripts/state-transition-guard.sh" "$g064_feature_dir")"
if [[ "$g064_status" -ne 0 ]]; then
  pass "Unauthorized workflow runner fixture fails the transition guard as expected"
else
  fail "Unauthorized workflow runner fixture should fail the transition guard"
  sed -n '1,220p' "$g064_log"
fi
assert_log_contains "$g064_log" "enables workflow execution without a grant" "Negative fixture triggers the G064 unauthorized workflow-runner check"
assert_log_contains "$g064_log" "G064 cannot be certified" "Negative fixture surfaces the workflow grant failure through guard Check 3H"

# ----------------------------------------------------------------------------
# G040 / Check 18 — deferral regex refinement (spec 001)
# ----------------------------------------------------------------------------
# These selftests exercise the refined Check 18 deferral-language scan. They
# verify that:
#   1. Real deferred-work prose under status=done still BLOCKS.
#   2. Schema-canonical followUp* field names (per completion-governance.md)
#      do NOT trigger Check 18 by themselves.
#   3. stale done_with_concerns metadata fails before downstream checks.
#   4. <!-- bubbles:g040-skip-begin/end --> sentinel markers exclude only the
#      bracketed prose; deferral prose outside the markers still BLOCKS.
#
# Each valid-target fixture reaches Check 18 through a supported delivery
# contract. Contradictory legacy terminal metadata is asserted at the resolver
# boundary instead of being treated as an evaluable transition.

echo "Running G040 Check 18 — positive: deferred-work prose BLOCKs..."
g040_pos_deferred_log="$tmp_root/g040-pos-deferred.log"
run_capture "$g040_pos_deferred_log" bash "$GUARD_SCRIPT" "$g040_pos_deferred_dir" >/dev/null
assert_log_contains "$g040_pos_deferred_log" "deferral language hit" "G040 Check 18 BLOCKs on raw 'deferred to next sprint' prose"

echo "Running G040 Check 18 — positive: 'skip for now' BLOCKs..."
g040_pos_skip_log="$tmp_root/g040-pos-skip.log"
run_capture "$g040_pos_skip_log" bash "$GUARD_SCRIPT" "$g040_pos_skip_for_now_dir" >/dev/null
assert_log_contains "$g040_pos_skip_log" "deferral language hit" "G040 Check 18 BLOCKs on 'skip for now' prose"

echo "Running G040 Check 18 — negative: schema followUp* fields do NOT trigger..."
g040_neg_followup_log="$tmp_root/g040-neg-followup.log"
run_capture "$g040_neg_followup_log" bash "$GUARD_SCRIPT" "$g040_neg_followup_fields_dir" >/dev/null
assert_log_not_contains "$g040_neg_followup_log" "deferral language hit" "G040 Check 18 ignores schema followUpOwner/followUpAction/followUpTarget/followUps tokens"

echo "Running transition metadata negative: done_with_concerns fails loud..."
g040_neg_dwc_log="$tmp_root/g040-neg-dwc.log"
run_capture "$g040_neg_dwc_log" bash "$GUARD_SCRIPT" "$g040_neg_done_with_concerns_dir" >/dev/null
assert_log_contains "$g040_neg_dwc_log" "E009-TARGET-MISMATCH" "done_with_concerns is rejected as a contradictory terminal target"
assert_log_contains "$g040_neg_dwc_log" "verdict: BLOCKED" "done_with_concerns metadata emits a blocked transition result"

echo "Running G040 Check 18 — negative: skip-marker brackets exclude prose..."
g040_neg_markers_log="$tmp_root/g040-neg-markers.log"
run_capture "$g040_neg_markers_log" bash "$GUARD_SCRIPT" "$g040_neg_skip_markers_dir" >/dev/null
assert_log_not_contains "$g040_neg_markers_log" "deferral language hit" "G040 Check 18 ignores 'deferred' prose wrapped in bubbles:g040-skip-begin/end markers"

echo "Running G040 Check 18 — positive: marker pair does not protect prose outside..."
g040_pos_outside_log="$tmp_root/g040-pos-outside.log"
run_capture "$g040_pos_outside_log" bash "$GUARD_SCRIPT" "$g040_pos_skip_marker_outside_dir" >/dev/null
assert_log_contains "$g040_pos_outside_log" "deferral language hit" "G040 Check 18 BLOCKs on deferral prose OUTSIDE the marker pair"

echo "Running transition metadata negative: spec-063-shaped done_with_concerns fails loud..."
g040_neg_063_log="$tmp_root/g040-neg-063.log"
run_capture "$g040_neg_063_log" bash "$GUARD_SCRIPT" "$g040_neg_spec_063_excerpt_dir" >/dev/null
assert_log_contains "$g040_neg_063_log" "E009-TARGET-MISMATCH" "spec-063-shaped legacy terminal metadata is rejected before audit checks"
assert_log_contains "$g040_neg_063_log" "blockingCode: E009-TARGET-MISMATCH" "legacy terminal mismatch remains machine-readable"

echo "Running G040 Check 18 — positive: status=done with mixed schema tokens AND real deferral..."
g040_pos_mixed_log="$tmp_root/g040-pos-mixed.log"
run_capture "$g040_pos_mixed_log" bash "$GUARD_SCRIPT" "$g040_pos_strict_done_mixed_dir" >/dev/null
assert_log_contains "$g040_pos_mixed_log" "deferral language hit" "G040 Check 18 BLOCKs under status=done when real deferral prose ('punted to Phase 3') accompanies schema followUp* tokens"

# ----------------------------------------------------------------------------
# Check 14 — Implementation Completeness: word-boundary TODO/FIXME/HACK/STUB scan
# ----------------------------------------------------------------------------
# Regression guard for the raw-substring defect where bare-word markers embedded
# inside legitimate identifiers/strings/comments false-triggered Check 14 and
# mis-blocked completely legitimate code — e.g. `STUB` inside `BILLING_STUB_STRIPE`,
# `HACK` inside `HACKATHON`, `TODO` inside `TODO_LIST`. Real-world proof: a
# gateway file at services/gateway/src/domain/billing/provider.rs
# reported 24 bogus "TODO/STUB markers" where all 24 hits were the tested env-var
# name `BILLING_STUB_STRIPE` and its doc comments — zero real markers.
#
# Check 14 only reaches a marker scan for backtick-wrapped impl paths inside a
# fully-scaffolded passing feature, so we assert the operative core directly: the
# EXACT regex extracted from the guard source (no test/source drift) run through
# Check 14's own `grep -cnE '<regex>' <file> || true` line-count contract against
# two fixtures — one that MUST report zero (identifier-embedded false positives
# eliminated) and one that MUST still flag every genuine marker (true positives
# preserved). The two distinctive markers `unimplemented!` / `NotImplementedError`
# stay plain substrings, byte-identical to the original, so they cannot regress.
echo "Running Check 14 — word-boundary marker scan (false-positive regression)..."

check14_regex="$(grep -E 'file_todos=.*grep -cnE' "$GUARD_SCRIPT" | sed -E "s/^.*grep -cnE '([^']*)'.*\$/\1/" || true)"
if [[ -z "$check14_regex" ]]; then
  fail "Check 14 regex could not be extracted from $GUARD_SCRIPT (guard shape changed)"
else
  pass "Check 14 regex extracted from guard source (no test/source drift)"

  check14_must_not="$tmp_root/check14-must-not-flag.txt"
  cat <<'EOF' > "$check14_must_not"
BILLING_STUB_STRIPE
std::env::var("BILLING_STUB_STRIPE")
HACKATHON_MODE
TODO_LIST
STUBBORN
/// (`BILLING_STUB_STRIPE` truthy, i.e. `1` / `true`).
        std::env::set_var("BILLING_STUB_STRIPE", "1");
EOF

  check14_must_flag="$tmp_root/check14-must-flag.txt"
  cat <<'EOF' > "$check14_must_flag"
// TODO: fix
# FIXME later
// HACK workaround
// STUB: implement
STUB
    unimplemented!()
        raise NotImplementedError
EOF

  # Replicate Check 14's exact line-count contract (grep -cnE '<regex>' file || true).
  check14_neg_count="$({ grep -cnE "$check14_regex" "$check14_must_not"; } || true)"
  check14_pos_count="$({ grep -cnE "$check14_regex" "$check14_must_flag"; } || true)"

  if [[ "$check14_neg_count" -eq 0 ]]; then
    pass "Check 14 does NOT flag identifier-embedded markers (BILLING_STUB_STRIPE, HACKATHON_MODE, TODO_LIST, STUBBORN) — 0 hits"
  else
    fail "Check 14 false-positives on identifier-embedded markers ($check14_neg_count hits, expected 0)"
    echo "--- offending must-not-flag lines ---"
    grep -nE "$check14_regex" "$check14_must_not" || true
    echo "--- end ---"
  fi

  if [[ "$check14_pos_count" -eq 7 ]]; then
    pass "Check 14 still flags all 7 genuine markers (// TODO, # FIXME, // HACK, // STUB, bare STUB, unimplemented!(), NotImplementedError)"
  else
    fail "Check 14 regressed on genuine markers ($check14_pos_count hits, expected 7)"
    echo "--- genuine-marker lines matched ---"
    grep -nE "$check14_regex" "$check14_must_flag" || true
    echo "--- end ---"
  fi
fi

echo "----------------------------------------"
if [[ "$failures" -gt 0 ]]; then
  echo "state-transition-guard selftest failed with $failures issue(s)."
  exit 1
fi

echo "state-transition-guard selftest passed."