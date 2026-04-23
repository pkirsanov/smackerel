#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/state-transition-guard.sh"
OWNERSHIP_LINT_SCRIPT="$SCRIPT_DIR/agent-ownership-lint.sh"

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

clone_framework_surface() {
  local destination_root="$1"

  mkdir -p "$destination_root"
  cp -R "$SCRIPT_DIR/.." "$destination_root/bubbles"
  cp -R "$SCRIPT_DIR/../../agents" "$destination_root/agents"
}

inject_illegal_child_workflow_caller() {
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
      print "    canInvokeChildWorkflows: true"
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

  if grep -Fq "$needle" "$log_file"; then
    pass "$label"
  else
    fail "$label"
    echo "--- log excerpt: $log_file ---"
    sed -n '1,160p' "$log_file"
    echo "--- end log excerpt ---"
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

  sed -i "s|__SCENARIO_TEST__|$scenario_test|g" "$feature_dir/scopes.md"
  sed -i "s|__BROADER_TEST__|$broader_test|g" "$feature_dir/scopes.md"

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

  sed -i "s|__FEATURE_DIR__|$feature_dir|g" "$feature_dir/report.md"

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
    "validation": { "mode": "required", "source": "workflow-forced" }
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

  sed -i "s|__CANARY_TEST__|$canary_test|g" "$feature_dir/scopes.md"
  sed -i "s|__BROADER_TEST__|$broader_test|g" "$feature_dir/scopes.md"

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

  sed -i "s|__FEATURE_DIR__|$feature_dir|g" "$feature_dir/report.md"

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
    "validation": { "mode": "required", "source": "workflow-forced" }
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

  sed -i "s|__BROADER_TEST__|$canary_test|g" "$feature_dir/scopes.md"

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

  sed -i "s|__FEATURE_DIR__|$feature_dir|g" "$feature_dir/report.md"

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
    "validation": { "mode": "required", "source": "workflow-forced" }
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

  sed -i "s|__SCENARIO_TEST__|$scenario_test|g" "$scope_dir/scope.md"

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

  sed -i "s|__FEATURE_DIR__|$feature_dir|g" "$scope_dir/report.md"

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
g064_framework_root="$tmp_root/framework-g064"
g064_feature_dir="$g064_framework_root/specs/902-transition-guard-selftest-illegal-child-workflow"
mkdir -p "$tmp_root/specs"

emit_base_fixture "$positive_feature_dir"
cp -R "$positive_feature_dir" "$negative_feature_dir"
emit_shared_infra_fixture "$shared_positive_feature_dir"
emit_shared_infra_negative_fixture "$shared_negative_feature_dir"
cp -R "$positive_feature_dir" "$workflow_mode_negative_feature_dir"
mutate_workflow_mode_contradiction "$workflow_mode_negative_feature_dir/state.json"
emit_per_scope_fixture "$per_scope_positive_feature_dir" "Done" "scope-1-index-parity-proof"
emit_per_scope_fixture "$index_parity_negative_feature_dir" "In Progress" "scope-1-index-parity-proof"
emit_per_scope_fixture "$phantom_scope_negative_feature_dir" "Done" "scope-15-stochastic-sweep-remediation"
cp -R "$positive_feature_dir" "$execution_history_negative_feature_dir"
mutate_execution_history_implausible "$execution_history_negative_feature_dir/state.json"
cp -R "$positive_feature_dir" "$lockdown_round_negative_feature_dir"
mutate_lockdown_round_mismatch "$lockdown_round_negative_feature_dir/state.json"
clone_framework_surface "$g064_framework_root"
mkdir -p "$g064_framework_root/specs"
emit_base_fixture "$g064_feature_dir"
inject_illegal_child_workflow_caller "$g064_framework_root/bubbles/agent-capabilities.yaml"

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
  pass "Docs-only positive fixture passes the transition guard"
else
  fail "Docs-only positive fixture should pass the transition guard"
  sed -n '1,220p' "$positive_log"
  echo "--- artifact-lint output for positive fixture ---"
  set +e
  bash "$SCRIPT_DIR/artifact-lint.sh" "$positive_feature_dir"
  set -e
  echo "--- end artifact-lint output ---"
fi
assert_log_contains "$positive_log" "Framework ownership lint passed" "Positive fixture exercises guard Check 3G"
assert_log_contains "$positive_log" "TRANSITION PERMITTED" "Positive fixture reaches a permitted transition verdict"

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

echo "Running workflowMode contradiction selftest..."
workflow_mode_log="$tmp_root/workflow-mode.log"
workflow_mode_status="$(run_capture "$workflow_mode_log" bash "$GUARD_SCRIPT" "$workflow_mode_negative_feature_dir")"
if [[ "$workflow_mode_status" -ne 0 ]]; then
  pass "workflowMode contradiction fixture fails the transition guard as expected"
else
  fail "workflowMode contradiction fixture should fail the transition guard"
  sed -n '1,220p' "$workflow_mode_log"
fi
assert_log_contains "$workflow_mode_log" "workflowMode contradiction" "Negative fixture triggers Check 2B"

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

echo "Running negative child-workflow-policy selftest..."
g064_log="$tmp_root/g064-guard.log"
g064_status="$(run_capture "$g064_log" bash "$g064_framework_root/bubbles/scripts/state-transition-guard.sh" "$g064_feature_dir")"
if [[ "$g064_status" -ne 0 ]]; then
  pass "Illegal child-workflow caller fixture fails the transition guard as expected"
else
  fail "Illegal child-workflow caller fixture should fail the transition guard"
  sed -n '1,220p' "$g064_log"
fi
assert_log_contains "$g064_log" "only orchestrators may enable child workflows" "Negative fixture triggers the G064 orchestrator-only child-workflow check"
assert_log_contains "$g064_log" "G042/G063/G064 cannot be certified" "Negative fixture surfaces the framework contract failure through guard Check 3G"

echo "----------------------------------------"
if [[ "$failures" -gt 0 ]]; then
  echo "state-transition-guard selftest failed with $failures issue(s)."
  exit 1
fi

echo "state-transition-guard selftest passed."