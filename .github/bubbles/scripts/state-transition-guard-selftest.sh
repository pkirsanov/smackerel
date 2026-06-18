#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/state-transition-guard.sh"
OWNERSHIP_LINT_SCRIPT="$SCRIPT_DIR/agent-ownership-lint.sh"

# This selftest already exercises the transition guard's own status, artifact,
# scope, packet, timestamp, lockdown, and deferral checks. The delegated tail
# gates (G085-G095) each have dedicated selftests in framework-validate, so keep
# them out of this cumulative fixture suite to avoid repeated heavy scans.
export BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=1

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

assert_log_not_contains() {
  local log_file="$1"
  local needle="$2"
  local label="$3"

  if grep -Fq "$needle" "$log_file"; then
    fail "$label"
    echo "--- offending log excerpt: $log_file ---"
    grep -F "$needle" "$log_file" || true
    echo "--- end offending log excerpt ---"
  else
    pass "$label"
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

  # Mutate status only. We deliberately keep workflowMode as docs-only (the
  # base default) so that Check 17's full-delivery git-log probe is skipped
  # entirely (it gates on workflowMode == "full-delivery"). Check 17 with
  # full-delivery + a /tmp feature_dir would invoke `git log -- /tmp/...`
  # which fails with exit 128 inside the bubbles repo and aborts the script
  # under set -euo pipefail BEFORE Check 18 can run. Check 18 itself is
  # workflowMode-agnostic — it only cares about state.status — so the
  # docs-only/done mismatch (Check 3 ceiling fail) is harmless to our
  # assertions here. The unrelated checks may emit failures, but the script
  # continues and Check 18 runs to completion.
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
g040_pos_deferred_dir="$tmp_root/specs/920-g040-positive-deferred-prose"
g040_pos_skip_for_now_dir="$tmp_root/specs/921-g040-positive-skip-for-now"
g040_neg_followup_fields_dir="$tmp_root/specs/922-g040-negative-schema-yaml-only"
g040_neg_done_with_concerns_dir="$tmp_root/specs/923-g040-negative-done-with-concerns"
g040_neg_skip_markers_dir="$tmp_root/specs/924-g040-negative-skip-markers"
g040_pos_skip_marker_outside_dir="$tmp_root/specs/925-g040-positive-skip-marker-outside"
g040_neg_spec_063_excerpt_dir="$tmp_root/specs/926-g040-negative-spec-063-excerpt"
g040_pos_strict_done_mixed_dir="$tmp_root/specs/927-g040-positive-strict-done-mixed"
g064_framework_root="$tmp_root/framework-g064"
g064_feature_dir="$g064_framework_root/specs/902-transition-guard-selftest-illegal-child-workflow"
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
cp -R "$positive_feature_dir" "$negative_feature_dir"
emit_shared_infra_fixture "$shared_positive_feature_dir"
emit_shared_infra_negative_fixture "$shared_negative_feature_dir"
cp -R "$positive_feature_dir" "$workflow_mode_negative_feature_dir"
mutate_workflow_mode_contradiction "$workflow_mode_negative_feature_dir/state.json"
cp -R "$positive_feature_dir" "$planning_done_negative_feature_dir"
mutate_planning_mode_status "$planning_done_negative_feature_dir/state.json" "done" "true"
cp -R "$positive_feature_dir" "$planning_specs_hardened_positive_feature_dir"
mutate_planning_mode_status "$planning_specs_hardened_positive_feature_dir/state.json" "specs_hardened" "true"
emit_per_scope_fixture "$per_scope_positive_feature_dir" "Done" "scope-1-index-parity-proof"
emit_per_scope_fixture "$index_parity_negative_feature_dir" "In Progress" "scope-1-index-parity-proof"
emit_per_scope_fixture "$phantom_scope_negative_feature_dir" "Done" "scope-15-stochastic-sweep-remediation"
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

echo "Running product-to-planning ceiling selftest..."
planning_negative_log="$tmp_root/product-planning-negative.log"
planning_negative_status="$(run_capture "$planning_negative_log" bash "$GUARD_SCRIPT" "$planning_done_negative_feature_dir")"
if [[ "$planning_negative_status" -ne 0 ]]; then
  pass "product-to-planning/done fixture fails the transition guard as expected"
else
  fail "product-to-planning/done fixture should fail the transition guard"
  sed -n '1,220p' "$planning_negative_log"
fi
assert_log_contains "$planning_negative_log" "Workflow mode 'product-to-planning' ceiling is 'specs_hardened', NOT 'done'" "Planning-only mode blocks done status using registry ceiling"
assert_log_contains "$planning_negative_log" "planMaturityOnly=true is incompatible with status 'done'" "planMaturityOnly blocks delivery-done status"

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
g064_timeout_seconds="${BUBBLES_G064_SELFTEST_TIMEOUT_SECONDS:-120}"
g064_status="$(run_capture "$g064_log" timeout "$g064_timeout_seconds" env BUBBLES_REPO_ROOT="$g064_framework_root" bash "$g064_framework_root/bubbles/scripts/state-transition-guard.sh" "$g064_feature_dir")"
if [[ "$g064_status" -ne 0 ]]; then
  pass "Illegal child-workflow caller fixture fails the transition guard as expected"
else
  fail "Illegal child-workflow caller fixture should fail the transition guard"
  sed -n '1,220p' "$g064_log"
fi
assert_log_contains "$g064_log" "only orchestrators may enable child workflows" "Negative fixture triggers the G064 orchestrator-only child-workflow check"
assert_log_contains "$g064_log" "G042/G063/G064 cannot be certified" "Negative fixture surfaces the framework contract failure through guard Check 3G"

# ----------------------------------------------------------------------------
# G040 / Check 18 — deferral regex refinement (spec 001)
# ----------------------------------------------------------------------------
# These selftests exercise the refined Check 18 deferral-language scan. They
# verify that:
#   1. Real deferred-work prose under status=done still BLOCKS.
#   2. Schema-canonical followUp* field names (per completion-governance.md)
#      do NOT trigger Check 18 by themselves.
#   3. status=done_with_concerns short-circuits Check 18 with an INFO line.
#   4. <!-- bubbles:g040-skip-begin/end --> sentinel markers exclude only the
#      bracketed prose; deferral prose outside the markers still BLOCKS.
#
# Each fixture's overall guard exit may be non-zero for OTHER reasons (the
# fixture is shaped from a docs-only base). We only assert on Check 18 log
# content via assert_log_contains / assert_log_not_contains.

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

echo "Running G040 Check 18 — negative: status=done_with_concerns short-circuits..."
g040_neg_dwc_log="$tmp_root/g040-neg-dwc.log"
run_capture "$g040_neg_dwc_log" bash "$GUARD_SCRIPT" "$g040_neg_done_with_concerns_dir" >/dev/null
assert_log_contains "$g040_neg_dwc_log" "Check 18 skipped" "G040 Check 18 emits INFO skip line under done_with_concerns"
assert_log_not_contains "$g040_neg_dwc_log" "deferral language hit" "G040 Check 18 does NOT BLOCK under done_with_concerns even when 'deferred' language appears"

echo "Running G040 Check 18 — negative: skip-marker brackets exclude prose..."
g040_neg_markers_log="$tmp_root/g040-neg-markers.log"
run_capture "$g040_neg_markers_log" bash "$GUARD_SCRIPT" "$g040_neg_skip_markers_dir" >/dev/null
assert_log_not_contains "$g040_neg_markers_log" "deferral language hit" "G040 Check 18 ignores 'deferred' prose wrapped in bubbles:g040-skip-begin/end markers"

echo "Running G040 Check 18 — positive: marker pair does not protect prose outside..."
g040_pos_outside_log="$tmp_root/g040-pos-outside.log"
run_capture "$g040_pos_outside_log" bash "$GUARD_SCRIPT" "$g040_pos_skip_marker_outside_dir" >/dev/null
assert_log_contains "$g040_pos_outside_log" "deferral language hit" "G040 Check 18 BLOCKs on deferral prose OUTSIDE the marker pair"

echo "Running G040 Check 18 — negative: spec-063-shaped excerpt under done_with_concerns..."
g040_neg_063_log="$tmp_root/g040-neg-063.log"
run_capture "$g040_neg_063_log" bash "$GUARD_SCRIPT" "$g040_neg_spec_063_excerpt_dir" >/dev/null
assert_log_contains "$g040_neg_063_log" "Check 18 skipped" "G040 Check 18 emits INFO skip on spec-063-shaped done_with_concerns excerpt"
assert_log_not_contains "$g040_neg_063_log" "deferral language hit" "G040 Check 18 does NOT BLOCK on spec-063-shaped excerpt"

echo "Running G040 Check 18 — positive: status=done with mixed schema tokens AND real deferral..."
g040_pos_mixed_log="$tmp_root/g040-pos-mixed.log"
run_capture "$g040_pos_mixed_log" bash "$GUARD_SCRIPT" "$g040_pos_strict_done_mixed_dir" >/dev/null
assert_log_contains "$g040_pos_mixed_log" "deferral language hit" "G040 Check 18 BLOCKs under status=done when real deferral prose ('punted to Phase 3') accompanies schema followUp* tokens"

echo "----------------------------------------"
if [[ "$failures" -gt 0 ]]; then
  echo "state-transition-guard selftest failed with $failures issue(s)."
  exit 1
fi

echo "state-transition-guard selftest passed."