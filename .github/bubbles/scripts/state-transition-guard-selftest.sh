#!/usr/bin/env bash
# Capability: dod-gherkin-fidelity-threshold
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/state-transition-guard.sh"
PLANNING_CHECKS_SCRIPT="$SCRIPT_DIR/guards/planning-checks.sh"
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
assertions=0

# PATH is an input to this harness, not a trust root. Keep a silent adversarial
# env executable first for the entire suite so any executable dependency on
# PATH-resolved `env` turns the affected capture log empty and fails its
# existing content assertions. Trusted launchers below apply assignments
# directly in Bash and invoke the guard with an explicit bash command.
fake_env_dir="$tmp_root/fake-env-path"
mkdir -p "$fake_env_dir"
cat <<'EOF' > "$fake_env_dir/env"
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$fake_env_dir/env"
PATH="$fake_env_dir:$PATH"
export PATH

cleanup() {
  if [[ "$failures" -eq 0 ]] && [[ "${KEEP_SELFTEST_TMP:-0}" != "1" ]]; then
    rm -rf "$tmp_root"
  else
    echo "Preserving selftest workspace: $tmp_root"
  fi
}

trap cleanup EXIT

pass() {
  assertions=$((assertions + 1))
  echo "PASS: $1"
}

fail() {
  assertions=$((assertions + 1))
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

run_capture_from() {
  local working_directory="$1"
  local log_file="$2"
  shift 2

  set +e
  (cd "$working_directory" && "$@") >"$log_file" 2>&1
  local status=$?
  set -e

  echo "$status"
}

run_guard_fast_disabled() {
  local guard_script="$1"
  local feature_dir="$2"

  BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=0 \
    bash "$guard_script" "$feature_dir"
}

run_guard_with_repo_root_and_lint_timeout() {
  local repository_root="$1"
  local lint_timeout="$2"
  local guard_script="$3"
  local feature_dir="$4"

  BUBBLES_REPO_ROOT="$repository_root" \
    BUBBLES_ARTIFACT_LINT_TIMEOUT="$lint_timeout" \
    bash "$guard_script" "$feature_dir"
}

run_guard_with_resolver_count() {
  local count_file="$1"
  local guard_script="$2"
  local feature_dir="$3"

  BUBBLES_TRANSITION_RESOLVER_COUNT_FILE="$count_file" \
    bash "$guard_script" "$feature_dir"
}

run_guard_with_repo_root_fast_disabled() {
  local repository_root="$1"
  local guard_script="$2"
  local feature_dir="$3"

  BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=0 \
    BUBBLES_REPO_ROOT="$repository_root" \
    bash "$guard_script" "$feature_dir"
}

sha256_text() {
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s' "$1" | sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    printf '%s' "$1" | shasum -a 256 | awk '{print $1}'
  else
    printf 'state-transition-guard-selftest: sha256sum or shasum is required\n' >&2
    return 2
  fi
}

clone_framework_surface() {
  local destination_root="$1"

  mkdir -p "$destination_root"
  cp -R "$SCRIPT_DIR/.." "$destination_root/bubbles"
  cp -R "$SCRIPT_DIR/../../agents" "$destination_root/agents"
}

run_strict_manifest_containment_regressions() {
  local focused_root="$tmp_root/strict-containment-focused"
  local g064_root="$focused_root/g064"
  local planning_root="$focused_root/planning-gates"
  local basename_root="$focused_root/basename"
  local g064_dir="$g064_root/specs/001-g064-negative"
  local g087_dir="$planning_root/specs/001-g087-negative"
  local g091_dir="$planning_root/specs/002-g091-negative"
  local basename_planning_dir="$basename_root/specs/001-basename-planning"
  local basename_delivery_dir="$basename_root/specs/002-basename-delivery"
  local feature_dir log_file status

  echo "Running focused strict manifest-containment regressions..."

  clone_framework_surface "$g064_root"
  emit_base_fixture "$g064_dir"
  mutate_delivery_contract "$g064_dir/state.json"
  inject_unauthorized_workflow_runner "$g064_root/bubbles/agent-capabilities.yaml"
  git -C "$g064_root" init -q
  log_file="$focused_root/g064.log"
  status="$(BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=0 \
    run_capture_from "$g064_root" "$log_file" \
    bash "$g064_root/bubbles/scripts/state-transition-guard.sh" "$g064_dir")"
  if [[ "$status" -eq 1 ]]; then
    pass "Focused containment: G064 adversary exits exactly 1"
  else
    fail "Focused containment: G064 adversary must exit exactly 1 (observed $status)"
  fi
  assert_log_contains "$log_file" "enables workflow execution without a grant" \
    "Focused containment: G064 diagnostic remains isolated"

  clone_framework_surface "$planning_root"
  emit_honest_planning_fixture "$g087_dir"
  emit_honest_planning_fixture "$g091_dir"
  remove_planning_only_linkage "$g087_dir/state.json"
  git -C "$planning_root" init -q
  git -C "$planning_root" add -f bubbles agents specs
  git -C "$planning_root" -c user.name='Bubbles Selftest' -c user.email='bubbles-selftest@example.invalid' \
    commit -q -m 'test: seed focused planning gate fixtures'

  log_file="$focused_root/g087.log"
  status="$(BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=0 \
    run_capture_from "$planning_root" "$log_file" \
    bash "$planning_root/bubbles/scripts/state-transition-guard.sh" "$g087_dir")"
  if [[ "$status" -eq 1 ]]; then
    pass "Focused containment: G087 adversary exits exactly 1"
  else
    fail "Focused containment: G087 adversary must exit exactly 1 (observed $status)"
  fi
  assert_log_contains "$log_file" "Planning packet implementation linkage failed — Gate G087" \
    "Focused containment: G087 diagnostic remains isolated"

  printf '%s\n' 'Fallback route: invoke bubbles.design -> bubbles.plan when planning artifacts are missing.' \
    >> "$planning_root/agents/bubbles.workflow.agent.md"
  git -C "$planning_root" add -f agents/bubbles.workflow.agent.md
  git -C "$planning_root" -c user.name='Bubbles Selftest' -c user.email='bubbles-selftest@example.invalid' \
    commit -q -m 'test: inject focused G091 planning-chain adversary'
  log_file="$focused_root/g091.log"
  status="$(BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=0 \
    run_capture_from "$planning_root" "$log_file" \
    bash "$planning_root/bubbles/scripts/state-transition-guard.sh" "$g091_dir")"
  if [[ "$status" -eq 1 ]]; then
    pass "Focused containment: G091 adversary exits exactly 1"
  else
    fail "Focused containment: G091 adversary must exit exactly 1 (observed $status)"
  fi
  assert_log_contains "$log_file" "Planning workflow chain guard failed — Gate G091" \
    "Focused containment: G091 diagnostic remains isolated"

  clone_framework_surface "$basename_root"
  emit_honest_planning_fixture "$basename_planning_dir"
  emit_honest_planning_fixture "$basename_delivery_dir"
  for feature_dir in "$basename_planning_dir" "$basename_delivery_dir"; do
    bubbles_sed_inplace \
      's;^| Broader regression |.*$;| Broader regression | `regression` | `rlbasenameonlyfixture.js` | Preserve planning and delivery profile isolation. | `bash rlbasenameonlyfixture.js` | No |;' \
      "$feature_dir/scopes.md"
  done
  set_fixture_contract "$basename_delivery_dir/state.json" "autonomous-goal" "done"
  git -C "$basename_root" init -q

  log_file="$focused_root/basename-planning.log"
  status="$(run_capture_from "$basename_root" "$log_file" bash "$basename_root/bubbles/scripts/state-transition-guard.sh" "$basename_planning_dir")"
  if [[ "$status" -eq 0 ]]; then
    pass "Focused containment: basename-only planning fixture exits 0"
  else
    fail "Focused containment: basename-only planning fixture must exit 0 (observed $status)"
  fi
  assert_log_contains "$log_file" "planning maturity: rlbasenameonlyfixture.js" \
    "Focused containment: basename-only planning exemption is reached"

  log_file="$focused_root/basename-delivery.log"
  status="$(run_capture_from "$basename_root" "$log_file" bash "$basename_root/bubbles/scripts/state-transition-guard.sh" "$basename_delivery_dir")"
  if [[ "$status" -eq 1 ]]; then
    pass "Focused containment: basename-only delivery adversary exits exactly 1"
  else
    fail "Focused containment: basename-only delivery adversary must exit exactly 1 (observed $status)"
  fi
  assert_log_contains "$log_file" "non-existent or non-resolvable file: rlbasenameonlyfixture.js" \
    "Focused containment: basename-only delivery enforcement remains active"
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

inject_phase_owner() {
  local workflows_file="$1"
  local phase="$2"
  local owner="$3"
  local tmp_file
  tmp_file="$(mktemp)"

  awk -v phase="$phase" -v owner="$owner" '
    $0 == "  " phase ":" { in_phase=1 }
    in_phase && /^    owner:/ {
      print "    owner: " owner
      changed=1
      in_phase=0
      next
    }
    { print }
    END { if (changed != 1) exit 1 }
  ' "$workflows_file" > "$tmp_file"

  mv "$tmp_file" "$workflows_file"
}

inject_agent_owns_phases() {
  local capabilities_file="$1"
  local agent="$2"
  local value="$3"
  local tmp_file
  tmp_file="$(mktemp)"

  awk -v agent="$agent" -v value="$value" '
    $0 == "  " agent ":" { in_agent=1 }
    in_agent && /^    ownsPhases:/ {
      print "    ownsPhases: " value
      changed=1
      in_agent=0
      next
    }
    { print }
    END { if (changed != 1) exit 1 }
  ' "$capabilities_file" > "$tmp_file"

  mv "$tmp_file" "$capabilities_file"
}

rename_capability_agent() {
  local capabilities_file="$1"
  local old_agent="$2"
  local new_agent="$3"
  local tmp_file
  tmp_file="$(mktemp)"

  awk -v old_agent="$old_agent" -v new_agent="$new_agent" '
    $0 == "  " old_agent ":" {
      print "  " new_agent ":"
      changed=1
      next
    }
    { print }
    END { if (changed != 1) exit 1 }
  ' "$capabilities_file" > "$tmp_file"

  mv "$tmp_file" "$capabilities_file"
}

malform_workflow_runner_grants() {
  local capabilities_file="$1"
  local replacement="${2:-malformed}"
  local tmp_file
  tmp_file="$(mktemp)"

  awk -v replacement="$replacement" '
    /^workflowModeGrants:$/ { in_grants=1; print; next }
    in_grants && /^  agents:$/ {
      print "  agents: " replacement
      skipping_agents=1
      changed=1
      next
    }
    skipping_agents && /^[^[:space:]]/ {
      skipping_agents=0
      in_grants=0
      print
      next
    }
    skipping_agents { next }
    { print }
    END { if (changed != 1) exit 1 }
  ' "$capabilities_file" > "$tmp_file"

  mv "$tmp_file" "$capabilities_file"
}

inject_unknown_workflow_runner() {
  local capabilities_file="$1"
  local runner="$2"
  local tmp_file
  tmp_file="$(mktemp)"

  awk -v runner="$runner" '
    /^workflowModeGrants:$/ { in_grants=1 }
    in_grants && /^  agents:$/ { in_agents=1; print; next }
    in_agents && /^    [a-z][a-z0-9.-]*:$/ {
      print "    " runner ":"
      print "      modes: [ iterate ]"
      changed=1
      in_agents=0
    }
    { print }
    END { if (changed != 1) exit 1 }
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

check43_panel_text() {
  local log_file="$1"
  awk '
    /^check=43 verdict=/ { active=1 }
    active { print }
    active && /^effect=(COLLISION_ACCEPTED|TRANSITION_BLOCKED)$/ { active=0 }
  ' "$log_file"
}

assert_check43_contains() {
  local log_file="$1"
  local needle="$2"
  local label="$3"
  local panel
  panel="$(check43_panel_text "$log_file")"

  if printf '%s\n' "$panel" | grep -Fq -- "$needle"; then
    pass "$label"
  else
    fail "$label"
    printf '%s\n' "--- Check 43 panel: $log_file ---" "${panel:-<missing>}" "--- end Check 43 panel ---"
  fi
}

assert_check43_fields_in_order() {
  local log_file="$1"
  local label="$2"
  shift 2
  local panel_file="$tmp_root/check43-order.$$.log"
  local previous=0
  local needle line

  check43_panel_text "$log_file" > "$panel_file"
  for needle in "$@"; do
    line="$(awk -v after="$previous" -v needle="$needle" '
      NR > after && index($0, needle) { print NR; exit }
    ' "$panel_file")"
    if [[ -z "$line" ]]; then
      fail "$label (missing or out of order: $needle)"
      rm -f "$panel_file"
      return
    fi
    previous="$line"
  done
  rm -f "$panel_file"
  pass "$label"
}

# Canonical expectation of the guard's TRANSITION_GUARD_RESULT_V1 field order.
# This is the ONLY copy of that order in this file: assert_transition_result
# walks it positionally, and assert_transition_result_contract_matches_emitter
# compares it against the order re-derived from the guard's own emitter source.
TRANSITION_RESULT_FIELDS="schemaVersion workflowMode auditProfile targetStatus contractDigest targetRevision applicableCheckClasses notApplicableChecks passedGateIds failedGateIds failedChecks blockingCode parentExpandedPhases failureCount exitStatus verdict"

# Fields whose value must be a bracketed list. Named rather than addressed by
# index, so inserting a field cannot silently re-point this check at the wrong
# columns the way the positional 7..11 range could.
TRANSITION_RESULT_LIST_FIELDS="applicableCheckClasses notApplicableChecks passedGateIds failedGateIds failedChecks"

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
    -v expected_exit="$expected_exit" \
    -v expected_fields="$TRANSITION_RESULT_FIELDS" \
    -v list_fields="$TRANSITION_RESULT_LIST_FIELDS" '
    BEGIN {
      field_count = split(expected_fields, fields, " ")
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
      list_field_count = split(list_fields, list_field_names, " ")
      for (list_index = 1; list_index <= list_field_count; list_index++) {
        if (values[list_field_names[list_index]] !~ /^\[[A-Za-z0-9,-]*\]$/) invalid = 1
      }
      if (values["parentExpandedPhases"] !~ /^[0-9]+$/) invalid = 1
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

# Re-derive the emitted field order from the guard's emitter block instead of
# keeping a third hardcoded copy of it.
guard_emitted_result_fields() {
  awk '
    index($0, "BEGIN TRANSITION_GUARD_RESULT_V1") > 0 { active = 1; next }
    index($0, "END TRANSITION_GUARD_RESULT_V1") > 0 { if (active) exit }
    active && match($0, /[A-Za-z][A-Za-z0-9]*: /) {
      printf "%s%s", separator, substr($0, RSTART, RLENGTH - 2)
      separator = " "
    }
    END { printf "\n" }
  ' "$GUARD_SCRIPT"
}

# IMP-036 SCOPE-2 added parentExpandedPhases to the guard's emitter but to
# neither of its two consumers, so every assert_transition_result call silently
# accepted any result for months. Comparing the consumer's expectation against
# the emitter's own source on every run is what stops that recurring.
assert_transition_result_contract_matches_emitter() {
  local label="$1"
  local emitted_fields
  emitted_fields="$(guard_emitted_result_fields)"

  if [[ "$emitted_fields" == "$TRANSITION_RESULT_FIELDS" ]]; then
    pass "$label"
  else
    fail "$label"
    echo "--- emitter order, derived from $GUARD_SCRIPT ---"
    echo "$emitted_fields"
    echo "--- consumer order, TRANSITION_RESULT_FIELDS ---"
    echo "$TRANSITION_RESULT_FIELDS"
    echo "--- end TRANSITION_GUARD_RESULT_V1 contract mismatch ---"
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
test('docsScenarioRegression', () => {});
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

set_g061_transition_request() {
  local state_file="$1"
  local request_id="$2"
  local routed_spec="$3"
  local routing_class="$4"
  local cross_repo="$5"

  python3 -c '
import json, sys

state_file = sys.argv[1]
with open(state_file) as handle:
    state = json.load(handle)
state["status"] = "in_progress"
state["workflowMode"] = "autonomous-goal"
state["policySnapshot"]["workflowMode"] = "autonomous-goal"
state["certification"]["status"] = "in_progress"
state["transitionRequests"] = [{
    "id": sys.argv[2],
    "status": "open",
    "routedTo": "bubbles.validate",
    "routedToSpec": sys.argv[3],
    "productAction": "none",
    "routingClass": sys.argv[4],
    "crossRepoFollowUp": json.loads(sys.argv[5]),
}]
with open(state_file, "w") as handle:
    json.dump(state, handle, indent=2)
    handle.write("\n")
' "$state_file" "$request_id" "$routed_spec" "$routing_class" "$cross_repo"
}

set_g061_transition_requests_json() {
  local state_file="$1"
  local requests_json="$2"

  python3 -c '
import json, sys

state_file = sys.argv[1]
with open(state_file) as handle:
    state = json.load(handle)
state["status"] = "in_progress"
state["workflowMode"] = "autonomous-goal"
state["policySnapshot"]["workflowMode"] = "autonomous-goal"
state["certification"]["status"] = "in_progress"
state["transitionRequests"] = json.loads(sys.argv[2])
with open(state_file, "w") as handle:
    json.dump(state, handle, indent=2)
    handle.write("\n")
' "$state_file" "$requests_json"
}

assert_g061_blocked_case() {
  local feature_dir="$1"
  local case_name="$2"
  local requests_json="$3"
  local expected_problem="$4"
  local label="$5"
  local log_file="$tmp_root/g061-$case_name.log"

  set_g061_transition_requests_json "$feature_dir/state.json" "$requests_json"
  run_capture "$log_file" bash "$GUARD_SCRIPT" "$feature_dir" >/dev/null
  assert_log_contains "$log_file" "$expected_problem" "$label"
}

run_g061_regression_cases() {
  local fixture_repo="$tmp_root/g061-fixture-repo"
  local same_repo_dir="$fixture_repo/specs/061-same-repo-specialist"
  local same_repo_alias="$fixture_repo/specs/061-same-repo-alias"
  local external_blocked_dir="$fixture_repo/specs/062-external-blocked"
  local external_allowed_dir="$fixture_repo/specs/063-external-allowed"
  local same_repo_log="$tmp_root/g061-same-repo.log"
  local external_blocked_log="$tmp_root/g061-external-blocked.log"
  local external_allowed_log="$tmp_root/g061-external-allowed.log"
  local string_false_log="$tmp_root/g061-string-false.log"
  local traversal_log="$tmp_root/g061-traversal.log"
  local absolute_log="$tmp_root/g061-absolute.log"
  local symlink_log="$tmp_root/g061-symlink.log"
  local non_list_log="$tmp_root/g061-non-list.log"
  local non_object_log="$tmp_root/g061-non-object.log"

  clone_framework_surface "$fixture_repo"

  emit_base_fixture "$same_repo_dir"
  set_g061_transition_request \
    "$same_repo_dir/state.json" \
    "TR-G061-SAME-REPO" \
    "specs/061-same-repo-specialist" \
    "specialist" \
    "false"
  run_capture "$same_repo_log" bash "$GUARD_SCRIPT" "$same_repo_dir" >/dev/null
  assert_log_contains "$same_repo_log" \
    "--- Check 3F: Transition And Rework Packets (Gate G061) ---" \
    "G061 same-repo case reaches Check 3F"
  assert_log_contains "$same_repo_log" \
    "transitionRequest TR-G061-SAME-REPO is open-but-routed to 'bubbles.validate' (Gate G061 allowance)" \
    "G061 allows bubbles.validate routing to the currently guarded spec"
  assert_log_not_contains "$same_repo_log" \
    "transitionRequest TR-G061-SAME-REPO (status=open) lacks routing fields" \
    "G061 does not classify a same-spec specialist route as external"

  emit_base_fixture "$external_blocked_dir"
  set_g061_transition_request \
    "$external_blocked_dir/state.json" \
    "TR-G061-EXTERNAL-BLOCKED" \
    "specs/999-upstream-target" \
    "specialist" \
    "false"
  run_capture "$external_blocked_log" bash "$GUARD_SCRIPT" "$external_blocked_dir" >/dev/null
  assert_log_contains "$external_blocked_log" \
    "transitionRequest TR-G061-EXTERNAL-BLOCKED (status=open) lacks routing fields: routed externally but crossRepoFollowUp is not true (Gate G061)" \
    "G061 blocks an external/upstream route without crossRepoFollowUp"
  assert_log_not_contains "$external_blocked_log" \
    "transitionRequest TR-G061-EXTERNAL-BLOCKED is open-but-routed" \
    "G061 does not admit the incomplete external route"

  emit_base_fixture "$external_allowed_dir"
  set_g061_transition_request \
    "$external_allowed_dir/state.json" \
    "TR-G061-EXTERNAL-ALLOWED" \
    "specs/999-upstream-target" \
    "specialist" \
    "true"
  run_capture "$external_allowed_log" bash "$GUARD_SCRIPT" "$external_allowed_dir" >/dev/null
  assert_log_contains "$external_allowed_log" \
    "transitionRequest TR-G061-EXTERNAL-ALLOWED is open-but-routed to 'bubbles.validate' (Gate G061 allowance)" \
    "G061 allows a complete external route with crossRepoFollowUp"
  assert_log_not_contains "$external_allowed_log" \
    "transitionRequest TR-G061-EXTERNAL-ALLOWED (status=open) lacks routing fields" \
    "G061 keeps the complete external route non-blocking"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "commit-false" \
    '[{"id":"TR-G061-COMMIT-FALSE","status":"open","routedTo":"bubbles.validate","routedToCommit":"abcdef0","productAction":"none","routingClass":"specialist","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-COMMIT-FALSE (status=open) lacks routing fields: routed externally but crossRepoFollowUp is not true (Gate G061)" \
    "G061 requires crossRepoFollowUp for a commit route"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "ticket-false" \
    '[{"id":"TR-G061-TICKET-FALSE","status":"open","routedTo":"bubbles.validate","routedToTicket":"https://example.invalid/issues/61","productAction":"none","routingClass":"specialist","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-TICKET-FALSE (status=open) lacks routing fields: routed externally but crossRepoFollowUp is not true (Gate G061)" \
    "G061 requires crossRepoFollowUp for a ticket route"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "external-class-false" \
    '[{"id":"TR-G061-EXTERNAL-CLASS-FALSE","status":"open","routedTo":"bubbles.validate","routedToSpec":"specs/061-same-repo-specialist","productAction":"none","routingClass":"external","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-EXTERNAL-CLASS-FALSE (status=open) lacks routing fields: routed externally but crossRepoFollowUp is not true (Gate G061)" \
    "G061 requires crossRepoFollowUp for an explicit external routing class"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "upstream-class-false" \
    '[{"id":"TR-G061-UPSTREAM-CLASS-FALSE","status":"open","routedTo":"bubbles.validate","routedToSpec":"specs/061-same-repo-specialist","productAction":"none","routingClass":"upstream","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-UPSTREAM-CLASS-FALSE (status=open) lacks routing fields: routed externally but crossRepoFollowUp is not true (Gate G061)" \
    "G061 requires crossRepoFollowUp for an explicit upstream routing class"

  set_g061_transition_request \
    "$external_blocked_dir/state.json" \
    "TR-G061-STRING-FALSE" \
    "specs/999-upstream-target" \
    "specialist" \
    '"false"'
  run_capture "$string_false_log" bash "$GUARD_SCRIPT" "$external_blocked_dir" >/dev/null
  assert_log_contains "$string_false_log" \
    "transitionRequest TR-G061-STRING-FALSE (status=open) lacks routing fields: crossRepoFollowUp must be a JSON boolean (Gate G061)" \
    "G061 rejects a string crossRepoFollowUp value with a type-specific reason"
  assert_log_not_contains "$string_false_log" \
    "transitionRequest TR-G061-STRING-FALSE is open-but-routed" \
    "G061 does not treat a string crossRepoFollowUp value as true"

  set_g061_transition_request \
    "$same_repo_dir/state.json" \
    "TR-G061-TRAVERSAL" \
    "specs/./061-same-repo-specialist" \
    "specialist" \
    "false"
  run_capture "$traversal_log" bash "$GUARD_SCRIPT" "$same_repo_dir" >/dev/null
  assert_log_contains "$traversal_log" \
    "transitionRequest TR-G061-TRAVERSAL (status=open) lacks routing fields: routed externally but crossRepoFollowUp is not true (Gate G061)" \
    "G061 rejects an ambiguous traversal alias of the guarded spec"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "duplicate-separator" \
    '[{"id":"TR-G061-DUPLICATE-SEPARATOR","status":"open","routedTo":"bubbles.validate","routedToSpec":"specs//061-same-repo-specialist","productAction":"none","routingClass":"specialist","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-DUPLICATE-SEPARATOR (status=open) lacks routing fields: routed externally but crossRepoFollowUp is not true (Gate G061)" \
    "G061 rejects a duplicate-separator alias of the guarded spec"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "backslash" \
    '[{"id":"TR-G061-BACKSLASH","status":"open","routedTo":"bubbles.validate","routedToSpec":"specs/061-same-repo\\specialist","productAction":"none","routingClass":"specialist","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-BACKSLASH (status=open) lacks routing fields: routed externally but crossRepoFollowUp is not true (Gate G061)" \
    "G061 rejects a backslash alias of the guarded spec"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "surrounding-whitespace" \
    '[{"id":"TR-G061-SURROUNDING-WHITESPACE","status":"open","routedTo":"bubbles.validate","routedToSpec":" specs/061-same-repo-specialist ","productAction":"none","routingClass":"specialist","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-SURROUNDING-WHITESPACE (status=open) lacks routing fields: routed externally but crossRepoFollowUp is not true (Gate G061)" \
    "G061 rejects a surrounding-whitespace alias of the guarded spec"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "parent-traversal" \
    '[{"id":"TR-G061-PARENT-TRAVERSAL","status":"open","routedTo":"bubbles.validate","routedToSpec":"specs/alias/../061-same-repo-specialist","productAction":"none","routingClass":"specialist","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-PARENT-TRAVERSAL (status=open) lacks routing fields: routed externally but crossRepoFollowUp is not true (Gate G061)" \
    "G061 rejects a parent-traversal alias of the guarded spec"

  set_g061_transition_request \
    "$same_repo_dir/state.json" \
    "TR-G061-ABSOLUTE" \
    "$same_repo_dir" \
    "specialist" \
    "false"
  run_capture "$absolute_log" bash "$GUARD_SCRIPT" "$same_repo_dir" >/dev/null
  assert_log_contains "$absolute_log" \
    "transitionRequest TR-G061-ABSOLUTE (status=open) lacks routing fields: routed externally but crossRepoFollowUp is not true (Gate G061)" \
    "G061 rejects an absolute alias of the guarded spec"

  ln -s "061-same-repo-specialist" "$same_repo_alias"
  set_g061_transition_request \
    "$same_repo_dir/state.json" \
    "TR-G061-SYMLINK" \
    "specs/061-same-repo-alias" \
    "specialist" \
    "false"
  run_capture "$symlink_log" bash "$GUARD_SCRIPT" "$same_repo_dir" >/dev/null
  assert_log_contains "$symlink_log" \
    "transitionRequest TR-G061-SYMLINK (status=open) lacks routing fields: routed externally but crossRepoFollowUp is not true (Gate G061)" \
    "G061 does not resolve a symlink alias as the guarded spec"

  set_g061_transition_requests_json \
    "$same_repo_dir/state.json" \
    '{"id":"TR-G061-NON-LIST"}'
  run_capture "$non_list_log" bash "$GUARD_SCRIPT" "$same_repo_dir" >/dev/null
  assert_log_contains "$non_list_log" \
    "transitionRequest <queue> (status=malformed) lacks routing fields: transitionRequests is not a list (Gate G061)" \
    "G061 blocks a non-list transitionRequests queue"

  set_g061_transition_requests_json \
    "$same_repo_dir/state.json" \
    '["TR-G061-NON-OBJECT"]'
  run_capture "$non_object_log" bash "$GUARD_SCRIPT" "$same_repo_dir" >/dev/null
  assert_log_contains "$non_object_log" \
    "transitionRequest <entry:0> (status=malformed) lacks routing fields: transitionRequests[0] is not an object (Gate G061)" \
    "G061 blocks a non-object transitionRequests entry"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "type-status" \
    '[{"id":"TR-G061-TYPE-STATUS","status":1,"routedTo":"bubbles.validate","routedToSpec":"specs/061-same-repo-specialist","productAction":"none","routingClass":"specialist","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-TYPE-STATUS (status=<invalid>) lacks routing fields: status must be a JSON string (Gate G061)" \
    "G061 blocks a present non-string status"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "type-id" \
    '[{"id":1,"transitionRequestId":"TR-G061-TYPE-ID","status":"open","routedTo":"bubbles.validate","routedToSpec":"specs/061-same-repo-specialist","productAction":"none","routingClass":"specialist","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-TYPE-ID (status=open) lacks routing fields: id must be a JSON string (Gate G061)" \
    "G061 blocks a present non-string id"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "type-transition-request-id" \
    '[{"id":"TR-G061-TYPE-TRANSITION-REQUEST-ID","transitionRequestId":1,"status":"open","routedTo":"bubbles.validate","routedToSpec":"specs/061-same-repo-specialist","productAction":"none","routingClass":"specialist","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-TYPE-TRANSITION-REQUEST-ID (status=open) lacks routing fields: transitionRequestId must be a JSON string (Gate G061)" \
    "G061 blocks a present non-string transitionRequestId"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "type-routed-to" \
    '[{"id":"TR-G061-TYPE-ROUTED-TO","status":"open","routedTo":1,"routedToSpec":"specs/061-same-repo-specialist","productAction":"none","routingClass":"specialist","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-TYPE-ROUTED-TO (status=open) lacks routing fields: routedTo must be a JSON string (Gate G061)" \
    "G061 blocks a present non-string routedTo"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "type-routed-to-commit" \
    '[{"id":"TR-G061-TYPE-ROUTED-TO-COMMIT","status":"open","routedTo":"bubbles.validate","routedToCommit":1,"routedToSpec":"specs/061-same-repo-specialist","productAction":"none","routingClass":"specialist","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-TYPE-ROUTED-TO-COMMIT (status=open) lacks routing fields: routedToCommit must be a JSON string (Gate G061)" \
    "G061 blocks a present non-string routedToCommit"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "type-routed-to-spec" \
    '[{"id":"TR-G061-TYPE-ROUTED-TO-SPEC","status":"open","routedTo":"bubbles.validate","routedToCommit":"abcdef0","routedToSpec":1,"productAction":"none","routingClass":"specialist","crossRepoFollowUp":true}]' \
    "transitionRequest TR-G061-TYPE-ROUTED-TO-SPEC (status=open) lacks routing fields: routedToSpec must be a JSON string (Gate G061)" \
    "G061 blocks a present non-string routedToSpec"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "type-routed-to-ticket" \
    '[{"id":"TR-G061-TYPE-ROUTED-TO-TICKET","status":"open","routedTo":"bubbles.validate","routedToSpec":"specs/061-same-repo-specialist","routedToTicket":1,"productAction":"none","routingClass":"specialist","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-TYPE-ROUTED-TO-TICKET (status=open) lacks routing fields: routedToTicket must be a JSON string (Gate G061)" \
    "G061 blocks a present non-string routedToTicket"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "type-product-action" \
    '[{"id":"TR-G061-TYPE-PRODUCT-ACTION","status":"open","routedTo":"bubbles.validate","routedToSpec":"specs/061-same-repo-specialist","productAction":1,"routingClass":"specialist","crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-TYPE-PRODUCT-ACTION (status=open) lacks routing fields: productAction must be a JSON string (Gate G061)" \
    "G061 blocks a present non-string productAction"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "type-routing-class" \
    '[{"id":"TR-G061-TYPE-ROUTING-CLASS","status":"open","routedTo":"bubbles.validate","routedToSpec":"specs/061-same-repo-specialist","productAction":"none","routingClass":1,"crossRepoFollowUp":false}]' \
    "transitionRequest TR-G061-TYPE-ROUTING-CLASS (status=open) lacks routing fields: routingClass must be a JSON string (Gate G061)" \
    "G061 blocks a present non-string routingClass"

  assert_g061_blocked_case \
    "$same_repo_dir" \
    "type-cross-repo-follow-up" \
    '[{"id":"TR-G061-TYPE-CROSS-REPO-FOLLOW-UP","status":"open","routedTo":"bubbles.validate","routedToSpec":"specs/999-upstream-target","productAction":"none","routingClass":"specialist","crossRepoFollowUp":1}]' \
    "transitionRequest TR-G061-TYPE-CROSS-REPO-FOLLOW-UP (status=open) lacks routing fields: crossRepoFollowUp must be a JSON boolean (Gate G061)" \
    "G061 blocks numeric crossRepoFollowUp instead of treating 1 as true"
}

if [[ "${BUBBLES_STATE_TRANSITION_GUARD_G061_ONLY:-0}" == "1" ]]; then
  run_g061_regression_cases
  if [[ "$failures" -gt 0 ]]; then
    echo "state-transition-guard G061 selftest failed with $failures issue(s)."
    exit 1
  fi
  echo "state-transition-guard G061 selftest passed."
  exit 0
fi

if [[ "${BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FOCUS:-}" \
  != "TP-01-04-security-boundary-group" ]] \
  && [[ "${BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FOCUS:-}" \
    != "BUG032-REG-C5A-TYPE-COLUMN-001" ]]; then
  run_g061_regression_cases
fi

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
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-009-S03-001",
      "title": "Planning maturity preserves honest incomplete delivery",
      "status": "planned",
      "scope": "Scope 01",
      "requirements": ["FR-009-S03-001"],
      "requiredTestType": "e2e-ui",
      "linkedTests": ["__FUTURE_TEST__"],
      "evidenceRefs": []
    }
  ]
}
EOF
  # Keep the manifest sentinel intact. It is the v1 compatibility spelling for
  # a classified planned reference; replacing it with an absolute fixture path
  # would violate the reader's repository-relative path contract.

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
    "currentPhase": "bootstrap",
    "completedPhaseClaims": ["analyze", "bootstrap"]
  },
  "certification": {
    "status": "specs_hardened",
    "certifiedCompletedPhases": ["analyze", "bootstrap"],
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
      "phasesExecuted": ["analyze"],
      "outcome": "completed_diagnostic",
      "startedAt": "2026-07-10T10:00:00Z",
      "completedAt": "2026-07-10T10:01:13Z"
    },
    {
      "phase": "analyze",
      "agent": "bubbles.ux",
      "phasesExecuted": ["analyze"],
      "outcome": "completed_diagnostic",
      "startedAt": "2026-07-10T10:02:01Z",
      "completedAt": "2026-07-10T10:04:29Z"
    },
    {
      "phase": "bootstrap",
      "agent": "bubbles.design",
      "phasesExecuted": ["bootstrap"],
      "outcome": "completed_diagnostic",
      "startedAt": "2026-07-10T10:05:17Z",
      "completedAt": "2026-07-10T10:08:52Z"
    },
    {
      "phase": "bootstrap",
      "agent": "bubbles.plan",
      "phasesExecuted": ["bootstrap"],
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

write_g057_manifest() {
  local feature_dir="$1"
  local document="$2"
  printf '%s\n' "$document" > "$feature_dir/scenario-manifest.json"
}

append_g057_scenario() {
  local feature_dir="$1"
  local scenario_id="$2"

  cat <<EOF >> "$feature_dir/spec.md"

## G057 Scenario

### $scenario_id - G057 profile classification

\`\`\`gherkin
Scenario: G057 classifies each scenario independently
Given one transition-profile-bound scenario
When Check 3C evaluates its normalized references
Then the scenario is classified without borrowing another scenario's counts
\`\`\`
EOF

  cat <<EOF >> "$feature_dir/scopes.md"

## G057 Scenario

### $scenario_id - G057 profile classification

\`\`\`gherkin
Scenario: G057 classifies each scenario independently
Given one transition-profile-bound scenario
When Check 3C evaluates its normalized references
Then the scenario is classified without borrowing another scenario's counts
\`\`\`
EOF

  bubbles_sed_inplace \
    's/Documentation route metadata is recorded consistently across artifacts/G057 classifies each scenario independently without borrowing another scenario count/' \
    "$feature_dir/scopes.md"
}

append_g057_delivery_receipts() {
  local scenario_id="$1"
  local receipt_log="$tmp_root/.specify/runtime/tool-calls.jsonl"
  local source_revision="0000000000000000000000000000000000000001"
  local test_identity="tests/docs-scenario-regression.e2e.spec.ts::docsScenarioRegression"
  local negative_control="remove the scenario-specific assertion; the regression no longer discriminates G057 behavior"
  local phase exit_code timestamp

  mkdir -p "$(dirname "$receipt_log")"
  for phase in red implement green regression; do
    exit_code=0
    case "$phase" in
      red)
        exit_code=1
        timestamp="2026-08-31T20:00:00Z"
        ;;
      implement) timestamp="2026-08-31T20:01:00Z" ;;
      green) timestamp="2026-08-31T20:02:00Z" ;;
      regression) timestamp="2026-08-31T20:03:00Z" ;;
    esac
    printf '{"schemaVersion":2,"ts":"%s","sessionId":"g057-%s-%s","cmd":"bash bubbles/scripts/state-transition-guard-selftest.sh","exitCode":%s,"stdoutHash":"9f2c1a77b3e45d6081ca2be7f4d0913ac5e8b26df1074a3c9e5b0d8f6a271c43","scenarioBinding":{"scenarioId":"%s","phase":"%s","testIdentity":"%s","sourceRevision":"%s","negativeControl":"%s","claim":"G057 classifies each scenario independently","implementationRefs":["bubbles/scripts/guards/control-plane-checks.sh"]}}\n' \
      "$timestamp" "$scenario_id" "$phase" "$exit_code" "$scenario_id" "$phase" \
      "$test_identity" "$source_revision" "$negative_control" >> "$receipt_log"
  done
}

assert_g057_valid_case() {
  local feature_dir="$1"
  local case_name="$2"
  local expected_message="$3"
  local require_clean_exit="${4:-true}"
  local log_file="$tmp_root/g057-$case_name.log"
  local status

  status="$(run_capture "$log_file" bash "$GUARD_SCRIPT" "$feature_dir")"
  if [[ "$require_clean_exit" == "false" ]]; then
    pass "G057 $case_name fixture completed for G057 assertions"
  elif [[ "$status" -eq 0 ]]; then
    pass "G057 $case_name fixture exits 0"
  else
    fail "G057 $case_name fixture must exit 0 (observed $status)"
  fi
  assert_log_contains "$log_file" \
    "$expected_message" \
    "G057 $case_name satisfies its per-scenario profile contract"
  assert_log_not_contains "$log_file" "scenario-manifest.json violates the per-scenario" \
    "G057 $case_name emits no G057 policy failure"
  assert_log_not_contains "$log_file" "scenario-manifest.json is malformed or has an unsupported projection (Gate G057)" \
    "G057 $case_name emits no G057 projection failure"
  assert_log_contains "$log_file" "every linked test resolves to a real file and title (Gate G057)" \
    "G057 $case_name reaches linked-test resolution"
  assert_log_contains "$log_file" "scenario obligation matrix is coherent (Gate G057)" \
    "G057 $case_name reaches obligation checks"
  assert_log_contains "$log_file" "declared test mechanisms support their claims (Gate G057)" \
    "G057 $case_name reaches mechanism checks"
}

assert_g057_policy_failure() {
  local feature_dir="$1"
  local case_name="$2"
  local log_file="$tmp_root/g057-$case_name.log"
  local status

  status="$(run_capture "$log_file" bash "$GUARD_SCRIPT" "$feature_dir")"
  if [[ "$status" -eq 1 ]]; then
    pass "G057 $case_name fixture exits exactly 1"
  else
    fail "G057 $case_name fixture must exit exactly 1 (observed $status)"
  fi
  assert_log_contains "$log_file" \
    "scenario-manifest.json violates the per-scenario" \
    "G057 $case_name fails its per-scenario profile contract"
  assert_log_contains "$log_file" "failureCount: 1" \
    "G057 $case_name records exactly one failure"
  assert_log_contains "$log_file" "failedGateIds: [G057]" \
    "G057 $case_name isolates the failed gate list to G057"
  assert_log_not_contains "$log_file" "every linked test resolves to a real file and title (Gate G057)" \
    "G057 $case_name suppresses child resolution after policy failure"
  assert_log_not_contains "$log_file" "scenario obligation matrix is coherent (Gate G057)" \
    "G057 $case_name suppresses child obligation checks after policy failure"
  assert_log_not_contains "$log_file" "declared test mechanisms support their claims (Gate G057)" \
    "G057 $case_name suppresses child mechanism checks after policy failure"
}

assert_g057_malformed_once() {
  local feature_dir="$1"
  local case_name="$2"
  local log_file="$tmp_root/g057-$case_name.log"
  local status

  status="$(run_capture "$log_file" bash "$GUARD_SCRIPT" "$feature_dir")"
  if [[ "$status" -eq 1 ]]; then
    pass "G057 $case_name fixture exits exactly 1"
  else
    fail "G057 $case_name fixture must exit exactly 1 (observed $status)"
  fi
  assert_log_contains "$log_file" \
    "scenario-manifest.json is malformed or has an unsupported projection (Gate G057)" \
    "G057 $case_name reports the isolated projection failure"
  assert_log_contains "$log_file" "failureCount: 1" \
    "G057 $case_name records exactly one failure"
  assert_log_contains "$log_file" "failedGateIds: [G057]" \
    "G057 $case_name isolates the failed gate list to G057"
  assert_log_not_contains "$log_file" "scenario-manifest.json violates the per-scenario" \
    "G057 $case_name does not cascade into policy validation"
  assert_log_not_contains "$log_file" "every linked test resolves to a real file and title (Gate G057)" \
    "G057 $case_name suppresses child resolution after malformed projection"
  assert_log_not_contains "$log_file" "scenario obligation matrix is coherent (Gate G057)" \
    "G057 $case_name suppresses child obligation checks after malformed projection"
  assert_log_not_contains "$log_file" "declared test mechanisms support their claims (Gate G057)" \
    "G057 $case_name suppresses child mechanism checks after malformed projection"
}

assert_g057_known_id_reconciliation_failure() {
  local feature_dir="$1"
  local case_name="$2"
  local missing_id="$3"
  local log_file="$tmp_root/g057-$case_name.log"
  local status

  status="$(run_capture "$log_file" bash "$GUARD_SCRIPT" "$feature_dir")"
  if [[ "$status" -eq 1 ]]; then
    pass "G057 $case_name fixture exits exactly 1"
  else
    fail "G057 $case_name fixture must exit exactly 1 (observed $status)"
  fi
  assert_log_contains "$log_file" \
    "identified-subset exact matching failed: scenario-manifest.json does not contain every known resolved Gherkin scenario ID (Gate G057)" \
    "G057 $case_name rejects the missing known stable ID"
  assert_log_contains "$log_file" "Missing known scenario ID(s): $missing_id" \
    "G057 $case_name identifies the missing known stable ID"
  assert_log_contains "$log_file" "failureCount: 1" \
    "G057 $case_name records exactly one failure"
  assert_log_contains "$log_file" "failedGateIds: [G057]" \
    "G057 $case_name isolates the failed gate list to G057"
  assert_log_not_contains "$log_file" "every linked test resolves to a real file and title (Gate G057)" \
    "G057 $case_name suppresses child resolution after reconciliation failure"
  assert_log_not_contains "$log_file" "scenario obligation matrix is coherent (Gate G057)" \
    "G057 $case_name suppresses child obligation checks after reconciliation failure"
  assert_log_not_contains "$log_file" "declared test mechanisms support their claims (Gate G057)" \
    "G057 $case_name suppresses child mechanism checks after reconciliation failure"
}

assert_g057_count_mismatch() {
  local feature_dir="$1"
  local case_name="$2"
  local manifest_count="$3"
  local scope_count="$4"
  local log_file="$tmp_root/g057-$case_name.log"
  local status

  status="$(run_capture "$log_file" bash "$GUARD_SCRIPT" "$feature_dir")"
  if [[ "$status" -eq 1 ]]; then
    pass "G057 $case_name fixture exits exactly 1"
  else
    fail "G057 $case_name fixture must exit exactly 1 (observed $status)"
  fi
  assert_log_contains "$log_file" \
    "legacy residual cardinality cannot match because scenario-manifest.json tracks $manifest_count scenarios but resolved scopes define exactly $scope_count Gherkin scenarios (Gate G057)" \
    "G057 $case_name rejects unequal total scenario counts"
  assert_log_contains "$log_file" "failureCount: 1" \
    "G057 $case_name records exactly one failure"
  assert_log_contains "$log_file" "failedGateIds: [G057]" \
    "G057 $case_name isolates the failed gate list to G057"
  assert_log_not_contains "$log_file" "every linked test resolves to a real file and title (Gate G057)" \
    "G057 $case_name suppresses child resolution after count mismatch"
  assert_log_not_contains "$log_file" "scenario obligation matrix is coherent (Gate G057)" \
    "G057 $case_name suppresses child obligation checks after count mismatch"
  assert_log_not_contains "$log_file" "declared test mechanisms support their claims (Gate G057)" \
    "G057 $case_name suppresses child mechanism checks after count mismatch"
}

assert_g057_duplicate_scope_id_failure() {
  local feature_dir="$1"
  local case_name="$2"
  local log_file="$tmp_root/g057-$case_name.log"
  local status

  status="$(run_capture "$log_file" bash "$GUARD_SCRIPT" "$feature_dir")"
  if [[ "$status" -eq 1 ]]; then
    pass "G057 $case_name fixture exits exactly 1"
  else
    fail "G057 $case_name fixture must exit exactly 1 (observed $status)"
  fi
  assert_log_contains "$log_file" \
    "resolved scope artifacts contain duplicate Gherkin scenario IDs (Gate G057)" \
    "G057 $case_name rejects duplicate identified scope multiplicity"
  assert_log_contains "$log_file" "failureCount: 1" \
    "G057 $case_name records exactly one failure"
  assert_log_contains "$log_file" "failedGateIds: [G057]" \
    "G057 $case_name isolates the failed gate list to G057"
}

run_focused_g057_assertions() {
  local assertion_start="$assertions"

  echo "Running focused G057 profile-aware scenario manifest regressions..."
  assert_g057_valid_case "$g057_planning_planned_dir" "planning planned-only" "PLANNED/NA: SCN-G057-101"
  assert_g057_valid_case "$g057_planning_authored_dir" "planning authored" "SCN-G057-102 has authored scenario coverage"
  assert_g057_policy_failure "$g057_planning_neither_dir" "planning neither"
  assert_g057_valid_case "$g057_delivery_valid_dir" "delivery authored plus evidence" "SCN-G057-104 has authored scenario coverage"
  assert_g057_policy_failure "$g057_delivery_planned_dir" "delivery planned-only"
  assert_g057_policy_failure "$g057_delivery_no_evidence_dir" "delivery authored without evidence"
  assert_g057_policy_failure "$g057_delivery_mixed_dir" "delivery mixed planned and authored"
  assert_g057_malformed_once "$g057_delivery_missing_file_dir" "delivery missing authored file"
  assert_g057_policy_failure "$g057_delivery_wrong_type_dir" "delivery incompatible type"
  assert_g057_policy_failure "$g057_delivery_string_untyped_dir" "delivery authored string without type"
  assert_g057_policy_failure "$g057_delivery_object_untyped_dir" "delivery authored object without type"
  assert_g057_policy_failure "$g057_delivery_null_evidence_member_dir" "delivery null evidence member"
  assert_g057_policy_failure "$g057_delivery_numeric_evidence_member_dir" "delivery numeric evidence member"
  assert_g057_policy_failure "$g057_delivery_blank_evidence_member_dir" "delivery blank evidence member"
  assert_g057_policy_failure "$g057_delivery_whitespace_evidence_member_dir" "delivery whitespace evidence member"
  assert_g057_valid_case "$g057_v2_dir" "strict v2 positive" "identified-subset exact matching covers every resolved Gherkin scenario ID; legacy residual cardinality is 0 = 0"
  assert_g057_valid_case "$g057_scoped_id_dir" "arbitrary-segment cross-surface ID positive" "identified-subset exact matching covers every resolved Gherkin scenario ID; legacy residual cardinality is 0 = 0"
  assert_g057_malformed_once "$g057_unknown_version_dir" "unknown version"
  assert_g057_policy_failure "$g057_missing_title_dir" "missing title"
  assert_g057_malformed_once "$g057_null_links_dir" "null linked tests"
  assert_g057_policy_failure "$g057_scalar_evidence_dir" "scalar evidence refs"
  assert_g057_malformed_once "$g057_null_planned_dir" "null planned tests"
  assert_g057_malformed_once "$g057_invalid_link_member_dir" "invalid linked-test member object"
  assert_g057_malformed_once "$g057_invalid_planned_member_dir" "invalid planned-test member object"
  assert_g057_malformed_once "$g057_malformed_scoped_id_dir" "malformed scoped ID"
  assert_g057_malformed_once "$g057_duplicate_effective_id_dir" "duplicate effective ID"
  assert_g057_valid_case "$g057_legacy_count_only_dir" "all-unidentified equal count" "identified-subset exact matching covers all 0 known Gherkin scenario ID(s); legacy residual cardinality matches 1 unidentified scope heading(s) without inferring identity (1 total = 1 total)"
  assert_g057_valid_case "$g057_mixed_heading_valid_dir" "positive mixed stable and legacy" "identified-subset exact matching covers all 1 known Gherkin scenario ID(s); legacy residual cardinality matches 1 unidentified scope heading(s) without inferring identity (2 total = 2 total)"
  assert_g057_known_id_reconciliation_failure "$g057_mixed_heading_wrong_id_dir" \
    "mixed stable and legacy heading with wrong known ID" "SCN-009-S03-001"
  assert_g057_duplicate_scope_id_failure "$g057_duplicate_scope_known_id_dir" \
    "duplicate scope known ID"
  assert_g057_malformed_once "$g057_duplicate_effective_id_dir" "duplicate manifest known ID"

  assert_g057_count_mismatch "$g057_undercount_dir" "undercount" 1 2
  assert_g057_count_mismatch "$g057_mixed_heading_overcount_dir" \
    "mixed stable and legacy heading overcount" 3 2
  assert_g057_count_mismatch "$g057_all_identified_surplus_dir" \
    "all-identified surplus" 2 1
  echo "Focused G057 selector completed $((assertions - assertion_start)) assertion(s)."
}

remove_planning_only_linkage() {
  local state_file="$1"
  local temp_file
  temp_file="$(mktemp)"
  jq 'del(.planningOnly, .planningOnlyJustification)' "$state_file" > "$temp_file"
  mv "$temp_file" "$state_file"
}

set_completed_scopes_precedence_fixture() {
  local state_file="$1"
  local temp_file
  temp_file="$(mktemp)"

  jq '.completedScopes = []' "$state_file" > "$temp_file"
  mv "$temp_file" "$state_file"
}

expand_to_compact_three_scope_fixture() {
  local feature_dir="$1"
  local state_file="$feature_dir/state.json"

  cat <<'EOF' >> "$feature_dir/scopes.md"

## Scope 02: Compact Count Proof

**Status:** Done

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Regression E2E | `e2e-ui` | `__SCENARIO_TEST__` | Scenario regression. | `selftest:scenario-regression` | Yes |
| Regression E2E | `e2e-ui` | `__BROADER_TEST__` | Broader regression. | `selftest:broader-regression` | Yes |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior -> Evidence: report.md#test-evidence
- [x] Broader E2E regression suite passes -> Evidence: report.md#test-evidence

## Scope 03: Compact Count Bound

**Status:** Done

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Regression E2E | `e2e-ui` | `__SCENARIO_TEST__` | Scenario regression. | `selftest:scenario-regression` | Yes |
| Regression E2E | `e2e-ui` | `__BROADER_TEST__` | Broader regression. | `selftest:broader-regression` | Yes |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior -> Evidence: report.md#test-evidence
- [x] Broader E2E regression suite passes -> Evidence: report.md#test-evidence
EOF

  bubbles_sed_inplace \
    "s|__SCENARIO_TEST__|$feature_dir/tests/docs-scenario-regression.e2e.spec.ts|g" \
    "$feature_dir/scopes.md"
  bubbles_sed_inplace \
    "s|__BROADER_TEST__|$feature_dir/tests/docs-broader-regression.e2e.spec.ts|g" \
    "$feature_dir/scopes.md"

  python3 - "$state_file" <<'PY'
import json
import sys

path = sys.argv[1]
entries = [
    "01-docs-guard-fixture",
    "02-compact-count-proof",
    "03-compact-count-bound",
]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

data["certification"]["completedScopes"] = entries
rendered = json.dumps(data, indent=2)
expanded = (
    '    "completedScopes": [\n'
    '      "01-docs-guard-fixture",\n'
    '      "02-compact-count-proof",\n'
    '      "03-compact-count-bound"\n'
    '    ]'
)
compact = (
    '    "completedScopes": '
    '["01-docs-guard-fixture","02-compact-count-proof","03-compact-count-bound"]'
)
if expanded not in rendered:
    raise SystemExit("expand_to_compact_three_scope_fixture: expected array shape missing")

with open(path, "w", encoding="utf-8") as handle:
    handle.write(rendered.replace(expanded, compact, 1))
    handle.write("\n")
PY

  grep -Fq \
    '"completedScopes": ["01-docs-guard-fixture","02-compact-count-proof","03-compact-count-bound"]' \
    "$state_file"
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

run_bug033_timeout_guard_assertions() {
  local fixture_root="$tmp_root/bug033-timeout-receipt-repo"
  local feature_dir="$fixture_root/specs/951-bug033-timeout-identity"
  local receipt_log="$fixture_root/.specify/runtime/tool-calls.jsonl"
  local output_hash case_log status

  echo "Running focused BUG-033 timeout wrapper regressions..."
  clone_framework_surface "$fixture_root"
  emit_base_fixture "$feature_dir"
  mutate_delivery_contract "$feature_dir/state.json"
  git -C "$fixture_root" init -q
  mkdir -p "$(dirname "$receipt_log")"
  output_hash="$(sha256_text 'bug033-timeout-nonempty-output')"

  cat > "$receipt_log" <<EOF
{"ts":"2026-09-02T09:00:01Z","sessionId":"bug033-timeout-bare","spec":"specs/alpha","scope":"SCOPE-1","cmd":"bash bubbles/scripts/scenario-test-resolve-selftest.sh alpha","exitCode":0,"durationMs":101,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:00:03Z","sessionId":"bug033-timeout-short-v","spec":"specs/alpha","scope":"SCOPE-1","cmd":"timeout -v 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh alpha","exitCode":0,"durationMs":103,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:00:05Z","sessionId":"bug033-timeout-nested","spec":"specs/alpha","scope":"SCOPE-1","cmd":"bash -c env CHECK=1 gtimeout --verbose 150 sh bubbles/scripts/scenario-test-resolve-selftest.sh alpha","exitCode":0,"durationMs":105,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:00:07Z","sessionId":"bug033-gtimeout-options","spec":"specs/alpha","scope":"SCOPE-1","cmd":"gtimeout --signal TERM --kill-after=5 --foreground --preserve-status 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh alpha","exitCode":0,"durationMs":107,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
EOF
  case_log="$tmp_root/bug033-timeout-transparent.log"
  status="$(run_capture "$case_log" bash "$GUARD_SCRIPT" "$feature_dir")"
  if [[ "$status" -eq 0 ]]; then
    pass "BUG-033 timeout: Check 43 accepts -v and nested timeout/gtimeout wrappers as transparent"
  else
    fail "BUG-033 timeout: transparent wrappers must pass the whole guard (observed $status)"
  fi
  assert_log_not_contains "$case_log" "reason=command-identity-mismatch" \
    "BUG-033 timeout: transparent timeout spellings do not produce a clone allegation"

  cat > "$receipt_log" <<EOF
{"ts":"2026-09-02T09:05:01Z","sessionId":"bug033-timeout-path-child","spec":"specs/alpha","scope":"SCOPE-1","cmd":"bash bubbles/scripts/scenario-test-resolve-selftest.sh alpha","exitCode":0,"durationMs":151,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:05:03Z","sessionId":"bug033-timeout-path-attacker","spec":"specs/beta","scope":"SCOPE-1","cmd":"/tmp/timeout 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh beta","exitCode":0,"durationMs":153,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:05:05Z","sessionId":"bug033-timeout-path-system","spec":"specs/gamma","scope":"SCOPE-1","cmd":"/usr/bin/timeout 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh gamma","exitCode":0,"durationMs":155,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
EOF
  case_log="$tmp_root/bug033-timeout-path-qualified.log"
  status="$(run_capture "$case_log" bash "$GUARD_SCRIPT" "$feature_dir")"
  if [[ "$status" -ne 0 ]]; then
    pass "BUG-033 timeout trust bound: path-qualified timeout tokens remain opaque"
  else
    fail "BUG-033 timeout trust bound: unverified timeout paths must not collapse to the nested child"
  fi
  # The BUG-033 Check-43 merge replaced this section's plain "family=X"
  # diagnostic line with a richer identity_a/identity_b REFUSED panel
  # (command-identity-mismatch), so the assertions below check that panel's
  # actual field names instead of the superseded format.
  assert_log_contains "$case_log" "reason=command-identity-mismatch" \
    "BUG-033 timeout trust bound: path-qualified impersonation remains a clone allegation"
  assert_log_contains "$case_log" "identity_b=/tmp/timeout 150 bash bubbles/scripts/scenario-test-resolve-selftest.sh beta" \
    "BUG-033 timeout trust bound: path-qualified system and attacker wrappers retain timeout family"
  assert_log_contains "$case_log" "identity_a=bubbles/scripts/scenario-test-resolve-selftest.sh alpha" \
    "BUG-033 timeout trust bound: nested child remains distinct from opaque wrappers"

  cat > "$receipt_log" <<EOF
{"ts":"2026-09-02T09:10:01Z","sessionId":"bug033-timeout-malformed-k","spec":"specs/alpha","scope":"SCOPE-1","cmd":"timeout -k --verbose 150 cargo test","exitCode":0,"durationMs":201,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:10:03Z","sessionId":"bug033-timeout-malformed-s","spec":"specs/beta","scope":"SCOPE-1","cmd":"timeout -s --verbose 150 cargo test","exitCode":0,"durationMs":203,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:10:05Z","sessionId":"bug033-timeout-unknown","spec":"specs/gamma","scope":"SCOPE-1","cmd":"timeout --unknown 150 cargo test","exitCode":0,"durationMs":205,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:10:07Z","sessionId":"bug033-timeout-no-duration","spec":"specs/delta","scope":"SCOPE-1","cmd":"timeout -v cargo test","exitCode":0,"durationMs":207,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:10:09Z","sessionId":"bug033-timeout-near-miss","spec":"specs/epsilon","scope":"SCOPE-1","cmd":"mytimeout 150 cargo test","exitCode":0,"durationMs":209,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:10:11Z","sessionId":"bug033-timeout-real-child","spec":"specs/zeta","scope":"SCOPE-1","cmd":"cargo test","exitCode":0,"durationMs":211,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:10:13Z","sessionId":"bug033-timeout-unknown-signal","spec":"specs/eta","scope":"SCOPE-1","cmd":"timeout -s BOGUS 150 cargo test","exitCode":0,"durationMs":213,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:10:15Z","sessionId":"bug033-timeout-terminal-option","spec":"specs/theta","scope":"SCOPE-1","cmd":"timeout --help 150 cargo test","exitCode":0,"durationMs":215,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:10:17Z","sessionId":"bug033-timeout-cluster","spec":"specs/iota","scope":"SCOPE-1","cmd":"timeout -vfp 150 cargo test","exitCode":0,"durationMs":217,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:10:19Z","sessionId":"bug033-timeout-attached-k","spec":"specs/kappa","scope":"SCOPE-1","cmd":"timeout -k.5 150 cargo test","exitCode":0,"durationMs":219,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:10:21Z","sessionId":"bug033-timeout-attached-s","spec":"specs/lambda","scope":"SCOPE-1","cmd":"timeout -sTERM 150 cargo test","exitCode":0,"durationMs":221,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
EOF
  case_log="$tmp_root/bug033-timeout-opaque.log"
  status="$(run_capture "$case_log" bash "$GUARD_SCRIPT" "$feature_dir")"
  if [[ "$status" -ne 0 ]]; then
    pass "BUG-033 timeout bound: malformed, unknown, attached, clustered, missing-duration, and near-miss wrappers remain opaque"
  else
    fail "BUG-033 timeout bound: opaque timeout syntax must not be attributed to cargo"
  fi
  assert_log_contains "$case_log" "reason=command-identity-mismatch" \
    "BUG-033 timeout bound: opaque syntax sharing substantive stdout remains a clone allegation"
  assert_log_contains "$case_log" "identity_a=timeout -k --verbose 150 cargo test" \
    "BUG-033 timeout bound: malformed timeout syntax retains timeout as its family"

  cat > "$receipt_log" <<EOF
{"ts":"2026-09-02T09:15:01Z","sessionId":"bug033-timeout-near-miss","spec":"specs/alpha","scope":"SCOPE-1","cmd":"mytimeout 150 cargo test","exitCode":0,"durationMs":251,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:15:03Z","sessionId":"bug033-timeout-real-child","spec":"specs/beta","scope":"SCOPE-1","cmd":"cargo test","exitCode":0,"durationMs":253,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
EOF
  case_log="$tmp_root/bug033-timeout-near-miss.log"
  status="$(run_capture "$case_log" bash "$GUARD_SCRIPT" "$feature_dir")"
  if [[ "$status" -ne 0 ]]; then
    pass "BUG-033 timeout bound: an exact-basename near miss remains opaque"
  else
    fail "BUG-033 timeout bound: mytimeout must not be normalized as timeout"
  fi
  assert_log_contains "$case_log" "identity_a=mytimeout 150 cargo test" \
    "BUG-033 timeout bound: an exact-basename near miss retains its own family"

  cat > "$receipt_log" <<EOF
{"ts":"2026-09-02T09:20:01Z","sessionId":"bug033-timeout-cargo","spec":"specs/alpha","scope":"SCOPE-1","cmd":"timeout -v 150 cargo test","exitCode":0,"durationMs":301,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-09-02T09:20:03Z","sessionId":"bug033-timeout-npm","spec":"specs/beta","scope":"SCOPE-1","cmd":"gtimeout --preserve-status 150 npm run test","exitCode":0,"durationMs":303,"stdoutHash":"$output_hash","stdoutBytes":128,"tags":["test"]}
EOF
  case_log="$tmp_root/bug033-timeout-different-child.log"
  status="$(run_capture "$case_log" bash "$GUARD_SCRIPT" "$feature_dir")"
  if [[ "$status" -ne 0 ]]; then
    pass "BUG-033 timeout bound: transparent wrappers do not hide different child programs"
  else
    fail "BUG-033 timeout bound: cargo and npm children sharing stdout must still refuse"
  fi
  assert_log_contains "$case_log" "identity_a=cargo test" \
    "BUG-033 timeout bound: the whole guard names the cargo child"
  assert_log_contains "$case_log" "identity_b=npm run test" \
    "BUG-033 timeout bound: the whole guard names the npm child"
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

emit_g040_exposure_fixture() {
  local feature_dir="$1"
  local reason="$2"

  emit_base_fixture "$feature_dir"
  mutate_delivery_contract "$feature_dir/state.json"

  python3 - "$feature_dir/state.json" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

data["status"] = "done"
data.setdefault("certification", {})["status"] = "done"

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY

  {
    echo ""
    echo "### Exposure"
    echo ""
    echo "- **Exposure-Deferred:** $reason -> spec.md#exposure"
  } >> "$feature_dir/scopes.md"
}

emit_g040_cw_fixture() {
  # G040 / Check 18 certifying-window fixture builder (report.md marker parity
  # with artifact-lint.sh Check 3). Exercises the prior-window suppression added
  # to the Check 18 report scan.
  #
  # Args:
  #   feature_dir   — destination directory
  #   marker_count  — number of <!-- bubbles:certifying-window-begin --> markers
  #                   to emit (0, 1, or 2)
  #   pre_prose     — narrative placed BEFORE the first marker (prior-window
  #                   history region)
  #   post_prose    — narrative placed AFTER the marker (current certifying
  #                   window); pass "" to omit the current-window section
  local feature_dir="$1"
  local marker_count="$2"
  local pre_prose="$3"
  local post_prose="${4:-}"

  emit_base_fixture "$feature_dir"
  mutate_delivery_contract "$feature_dir/state.json"

  # Promote to a delivery-completion (done) transition so Check 18 is active,
  # mirroring emit_g040_fixture's status mutation.
  python3 - "$feature_dir/state.json" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

data["status"] = "done"
cert = data.setdefault("certification", {})
cert["status"] = "done"

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY

  {
    echo ""
    echo "## Prior-Window History"
    echo ""
    echo "$pre_prose"
    if [[ "$marker_count" -ge 1 ]]; then
      echo ""
      echo "<!-- bubbles:certifying-window-begin -->"
    fi
    if [[ "$marker_count" -ge 2 ]]; then
      echo ""
      echo "<!-- bubbles:certifying-window-begin -->"
    fi
    if [[ -n "$post_prose" ]]; then
      echo ""
      echo "## Current Certifying Window"
      echo ""
      echo "$post_prose"
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

# Check 6B phase -> owning-agent resolution. $2 selects a registered phase and
# $3 selects the recording agent so one shape drives declared-owner,
# capability-owner, and adversarial cases.
mutate_phase_provenance() {
  local state_file="$1"
  local phase="$2"
  local recording_agent="$3"

  python3 - "$state_file" "$phase" "$recording_agent" <<'PY'
import json
import sys

path = sys.argv[1]
phase = sys.argv[2]
recording_agent = sys.argv[3]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

# iterate keeps required_specialists small so this fixture isolates Check 6B
# provenance rather than tripping unrelated delivery-completion requirements.
data["workflowMode"] = "iterate"
snapshot = data.get("policySnapshot")
if isinstance(snapshot, dict):
    snapshot["workflowMode"] = "iterate"

execution = data.get("execution")
if not isinstance(execution, dict):
    execution = {}
    data["execution"] = execution

execution["completedPhaseClaims"] = [phase]
execution["executionHistory"] = [
    {
        "agent": recording_agent,
    "phasesExecuted": [phase],
        "startedAt": "2026-01-01T00:00:00Z",
        "completedAt": "2026-01-01T00:20:00Z",
    },
]

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY
}

mutate_analyze_phase_provenance() {
  mutate_phase_provenance "$1" "analyze" "$2"
}

mutate_unregistered_phase_claim() {
  local state_file="$1"

  python3 - "$state_file" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
  data = json.load(handle)

data["workflowMode"] = "iterate"
snapshot = data.get("policySnapshot")
if isinstance(snapshot, dict):
  snapshot["workflowMode"] = "iterate"

execution = data.get("execution")
if not isinstance(execution, dict):
  execution = {}
  data["execution"] = execution

execution["completedPhaseClaims"] = ["totally-made-up-phase"]
execution.pop("executionHistory", None)
data.pop("executionHistory", None)

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

mutate_partial_certified_phases_with_dict_claims() {
  local state_file="$1"

  python3 - "$state_file" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

# Regression shape for the Check 6 short-circuit defect. Check 6 used to select
# its phase source with `certification_phases or execution_phase_claims or
# legacy_phases`, so a NON-EMPTY certifiedCompletedPhases won outright and
# execution.completedPhaseClaims was never evaluated at all. Observed live: a
# packet certified for ["validate"] alongside fourteen execution claims had every
# other phase reported as unrecorded (G022) while Check 6B — which reads
# completedPhaseClaims directly — PASSED those same entries. One guard run
# asserted both "phase not recorded" and "that phase's record has valid
# provenance" for identical data. The sibling fixture above pins the EMPTY-
# certification path; this one pins the PARTIAL-certification path, which the
# `or` left completely unguarded.
#
# certifiedCompletedPhases carries only 'validate'; the remaining required phase
# 'audit' exists ONLY as a dict-shaped execution claim. Under workflowMode=iterate
# (required specialists: validate, audit) the two sources satisfy the requirement
# only when Check 6 MERGES them. 'implement' is a second, non-required dict claim
# so the mixed string+dict input is normalized across more than one record.
data["workflowMode"] = "iterate"
snapshot = data.get("policySnapshot")
if isinstance(snapshot, dict):
    snapshot["workflowMode"] = "iterate"

execution = data.get("execution")
if not isinstance(execution, dict):
    execution = {}
    data["execution"] = execution

execution["completedPhaseClaims"] = [
    {"phase": "audit", "agent": "bubbles.audit"},
    {"phase": "implement", "agent": "bubbles.implement"},
]
execution["executionHistory"] = [
    {
        "agent": "bubbles.implement",
        "runStartedAt": "2026-03-27T11:00:00Z",
        "runCompletedAt": "2026-03-27T11:09:00Z",
        "phasesExecuted": ["implement"],
    },
    {
        "agent": "bubbles.validate",
        "runStartedAt": "2026-03-27T11:20:00Z",
        "runCompletedAt": "2026-03-27T11:26:00Z",
        "phasesExecuted": ["validate"],
    },
    {
        "agent": "bubbles.audit",
        "runStartedAt": "2026-03-27T11:35:00Z",
        "runCompletedAt": "2026-03-27T11:43:00Z",
        "phasesExecuted": ["audit"],
    },
]

cert = data.get("certification")
if isinstance(cert, dict):
    cert["certifiedCompletedPhases"] = ["validate"]

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY
}

run_bug032_c5a_type_column_regression() {
  local base_fixture="$1"
  local fixture_root="$2"
  local type_first_dir="$fixture_root/bug032-c5a-type-first-control"
  local type_third_dir="$fixture_root/bug032-c5a-type-third-adversarial"
  local type_first_log="$fixture_root/bug032-c5a-type-first-control.log"
  local type_third_log="$fixture_root/bug032-c5a-type-third-adversarial.log"
  local type_first_status=0
  local type_third_status=0
  local type_first_mismatches=0
  local type_third_mismatches=0

  printf '%s\n' \
    'BUG032_C5A_SCENARIO_BINDING finding=BUG032-REG-C5A-TYPE-COLUMN-001 scenario=SCN-032-022 negativeControl=Type-first-equivalent'

  cp -R "$base_fixture" "$type_first_dir"
  cat <<EOF >> "$type_first_dir/scopes.md"

### Performance Contract

The p95 latency budget is 200 ms.

### Test Plan

| Type | Test ID | Description | File/Location | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Stress | TP-C5A-TYPE-COLUMN | Exercise the active p95 latency budget of 200 ms under pressure. | $type_first_dir/tests/docs-scenario-regression.e2e.spec.ts | selftest:stress-regression | No |

### Definition of Done

- [x] SCN-032-022 stress test verifies the active p95 latency budget of 200 ms. -> Evidence: report.md#test-evidence
EOF
  type_first_status="$(run_capture "$type_first_log" \
    bash "$GUARD_SCRIPT" "$type_first_dir")"
  [[ "$type_first_status" -eq 0 ]] \
    || type_first_mismatches=$((type_first_mismatches + 1))
  grep -Fq -- 'SLA-sensitive scope includes stress coverage: scopes.md' \
    "$type_first_log" \
    || type_first_mismatches=$((type_first_mismatches + 1))
  if [[ "$type_first_mismatches" -eq 0 ]]; then
    pass "BUG-032 SCN-032-022 Type-first canonical Stress row remains accepted"
  else
    printf 'BUG032_C5A_TYPE_FIRST_CONTROL_MISMATCH status=%s mismatches=%s\n' \
      "$type_first_status" "$type_first_mismatches"
    fail "BUG-032 SCN-032-022 Type-first canonical Stress row remains accepted"
  fi

  cp -R "$base_fixture" "$type_third_dir"
  cat <<EOF >> "$type_third_dir/scopes.md"

### Performance Contract

The p95 latency budget is 200 ms.

### Test Plan

| Test ID | Description | Type | File/Location | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| TP-C5A-TYPE-COLUMN | Exercise the active p95 latency budget of 200 ms under pressure. | Stress | $type_third_dir/tests/docs-scenario-regression.e2e.spec.ts | selftest:stress-regression | No |

### Definition of Done

- [x] SCN-032-022 stress test verifies the active p95 latency budget of 200 ms. -> Evidence: report.md#test-evidence
EOF
  type_third_status="$(run_capture "$type_third_log" \
    bash "$GUARD_SCRIPT" "$type_third_dir")"
  [[ "$type_third_status" -eq 0 ]] \
    || type_third_mismatches=$((type_third_mismatches + 1))
  grep -Fq -- 'SLA-sensitive scope includes stress coverage: scopes.md' \
    "$type_third_log" \
    || type_third_mismatches=$((type_third_mismatches + 1))
  if [[ "$type_third_mismatches" -eq 0 ]]; then
    pass "BUG032-REG-C5A-TYPE-COLUMN-001 / SCN-032-022 accepts Stress when Type is the third column"
  else
    printf 'BUG032_C5A_TYPE_THIRD_MISMATCH scenario=SCN-032-022 status=%s mismatches=%s missingStressRow=%s missingStressDod=%s\n' \
      "$type_third_status" "$type_third_mismatches" \
      "$(grep -Fq -- 'SLA-sensitive scope is missing canonical Stress Test Plan row' "$type_third_log" && printf 1 || printf 0)" \
      "$(grep -Fq -- 'SLA-sensitive scope is missing faithful stress DoD item' "$type_third_log" && printf 1 || printf 0)"
    fail "BUG032-REG-C5A-TYPE-COLUMN-001 / SCN-032-022 accepts Stress when Type is the third column"
  fi
}

if [[ "${BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FOCUS:-}" \
  == "BUG032-REG-C5A-TYPE-COLUMN-001" ]]; then
  bug032_c5a_focus_base="$tmp_root/bug032-c5a-focus-base"
  bug032_c5a_initial_failures="$failures"
  emit_base_fixture "$bug032_c5a_focus_base"
  mutate_delivery_contract "$bug032_c5a_focus_base/state.json"
  run_bug032_c5a_type_column_regression "$bug032_c5a_focus_base" "$tmp_root"
  if [[ "$failures" -eq "$bug032_c5a_initial_failures" ]]; then
    printf 'BUG032_C5A_FOCUSED_VERDICT=PASS\n'
    exit 0
  fi
  printf 'BUG032_C5A_FOCUSED_VERDICT=RED failures=%s\n' \
    "$((failures - bug032_c5a_initial_failures))"
  exit 1
fi

run_bug032_iteration10_security_assertions() {
  local focus_root="$tmp_root/bug032-iteration10-security"
  local focus_repo="$focus_root/repo"
  local focus_shadow_bin="$focus_root/shadow-bin"
  local focus_real_cat=""
  local previous_repo_root="${BUBBLES_REPO_ROOT:-}"
  local had_previous_repo_root=0
  local case_dir=""
  local case_log=""
  local case_status=0
  local source_path=""
  local swap_target=""
  local counter_file=""
  local first_scope=""
  local second_scope=""
  local sec001_failures=0
  local sec003_failures=0

  BUG032_ITER10_ASSERTION_PASSES=0
  BUG032_ITER10_ASSERTION_FAILURES=0
  BUG032_ITER10_SEC001_FAILURES=0
  BUG032_ITER10_SEC003_FAILURES=0
  [[ -v BUBBLES_REPO_ROOT ]] && had_previous_repo_root=1

  bug032_iter10_assertion() {
    local finding_id="$1"
    local polarity="$2"
    local assertion_status="$3"

    if [[ "$assertion_status" -eq 0 ]]; then
      BUG032_ITER10_ASSERTION_PASSES=$((BUG032_ITER10_ASSERTION_PASSES + 1))
      printf 'BUG032_ITER10_ASSERTION_PASS finding=%s polarity=%s\n' \
        "$finding_id" "$polarity"
    else
      BUG032_ITER10_ASSERTION_FAILURES=$((BUG032_ITER10_ASSERTION_FAILURES + 1))
      printf 'BUG032_ITER10_ASSERTION_FAIL finding=%s polarity=%s\n' \
        "$finding_id" "$polarity"
      case "$finding_id" in
        BUG032-SEC-001*) sec001_failures=$((sec001_failures + 1)) ;;
        BUG032-SEC-003*) sec003_failures=$((sec003_failures + 1)) ;;
      esac
    fi
  }

  bug032_iter10_emit_two_scope_fixture() {
    local destination="$1"
    local first_scope_contract="$2"

    emit_per_scope_fixture "$destination" "Done" "scope-1-index-parity-proof"
    mutate_delivery_contract "$destination/state.json"
    first_scope="$destination/scopes/01-index-parity-proof"
    second_scope="$destination/scopes/02-secondary-control"
    cp -R "$first_scope" "$second_scope"
    bubbles_sed_inplace \
      's/Scope 01: Index Parity Proof/Scope 02: Secondary Control/' \
      "$second_scope/scope.md"
    printf '%s\n' '| 02 | Secondary control | 01 | Done |' \
      >> "$destination/scopes/_index.md"
    python3 - "$destination/state.json" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)
certification = data["certification"]
certification["completedScopes"] = [
    "scope-1-index-parity-proof",
    "scope-2-secondary-control",
]
certification["scopeProgress"] = [
    {"scopeDir": "scopes/01-index-parity-proof"},
    {"scopeDir": "scopes/02-secondary-control"},
]
with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY
    if [[ "$first_scope_contract" == "yes" ]]; then
      printf '\n%s\n' 'The p95 latency budget is 200 ms.' \
        >> "$first_scope/scope.md"
    fi
  }

  mkdir -p "$focus_root" "$focus_shadow_bin"
  clone_framework_surface "$focus_repo"
  git -C "$focus_repo" init -q
  export BUBBLES_REPO_ROOT="$focus_repo"

  case_dir="$focus_repo/specs/980-bug032-sec001a-stale-index"
  bug032_iter10_emit_two_scope_fixture "$case_dir" yes
  case_log="$focus_root/sec001a-stale-index.log"
  case_status="$(run_capture "$case_log" bash "$GUARD_SCRIPT" "$case_dir")"
  if [[ "$case_status" -ne 0 ]] \
    && grep -Fq -- \
      'SLA-sensitive scope is missing canonical Stress Test Plan row: scopes/01-index-parity-proof/scope.md' \
      "$case_log" \
    && grep -Fq -- \
      'SLA-sensitive scope is missing faithful stress DoD item: scopes/01-index-parity-proof/scope.md' \
      "$case_log" \
    && ! grep -Fq -- 'No SLA-sensitive scopes detected for Gate G026' "$case_log"; then
    bug032_iter10_assertion BUG032-SEC-001A-CHECK5A-STALE-INDEX adversarial 0
  else
    printf 'BUG032_SEC001A_STALE_INDEX_MISMATCH status=%s\n' "$case_status"
    bug032_iter10_assertion BUG032-SEC-001A-CHECK5A-STALE-INDEX adversarial 1
  fi

  case_dir="$focus_repo/specs/981-bug032-sec001a-neutral-control"
  bug032_iter10_emit_two_scope_fixture "$case_dir" no
  case_log="$focus_root/sec001a-neutral-control.log"
  case_status="$(run_capture "$case_log" bash "$GUARD_SCRIPT" "$case_dir")"
  if [[ "$case_status" -eq 0 ]] \
    && grep -Fq -- 'No SLA-sensitive scopes detected for Gate G026' "$case_log" \
    && ! grep -Fq -- 'SLA-sensitive scope is missing' "$case_log" \
    && ! grep -Fq -- 'context projection failed' "$case_log"; then
    bug032_iter10_assertion BUG032-SEC-001A-CHECK5A-STALE-INDEX control 0
  else
    printf 'BUG032_SEC001A_NEUTRAL_CONTROL_MISMATCH status=%s\n' "$case_status"
    bug032_iter10_assertion BUG032-SEC-001A-CHECK5A-STALE-INDEX control 1
  fi

  focus_real_cat="$(command -v cat)"
  cat <<'EOF' > "$focus_shadow_bin/cat"
#!/usr/bin/env bash
set -u

for argument in "$@"; do
  if [[ -n "${BUG032_ITER10_SOURCE_PATH:-}" ]] \
    && [[ "$argument" == "$BUG032_ITER10_SOURCE_PATH" ]]; then
    printf '%s\n' call >> "${BUG032_ITER10_CAT_COUNTER:?}"
    if [[ "${BUG032_ITER10_CAT_MODE:-delegate}" == "swap-to-symlink" ]] \
      && [[ ! -L "$argument" ]]; then
      rm -f -- "$argument"
      ln -s -- "${BUG032_ITER10_SWAP_TARGET:?}" "$argument"
    fi
  fi
done
exec "${BUG032_ITER10_REAL_CAT:?}" "$@"
EOF
  chmod +x "$focus_shadow_bin/cat"

  case_dir="$focus_repo/specs/982-bug032-sec001b-source-toctou"
  emit_base_fixture "$case_dir"
  mutate_delivery_contract "$case_dir/state.json"
  source_path="$case_dir/scopes.md"
  swap_target="$case_dir/scopes-swap-target.md"
  cp "$source_path" "$swap_target"
  printf '\n%s\n' 'Remove the public route.' >> "$swap_target"
  counter_file="$focus_root/sec001b-source-toctou.count"
  : > "$counter_file"
  case_log="$focus_root/sec001b-source-toctou.log"
  case_status="$(run_capture "$case_log" env \
    PATH="$focus_shadow_bin:$PATH" \
    BUG032_ITER10_REAL_CAT="$focus_real_cat" \
    BUG032_ITER10_SOURCE_PATH="$source_path" \
    BUG032_ITER10_CAT_COUNTER="$counter_file" \
    BUG032_ITER10_CAT_MODE=swap-to-symlink \
    BUG032_ITER10_SWAP_TARGET="$swap_target" \
    bash "$GUARD_SCRIPT" "$case_dir")"
  if [[ "$case_status" -ne 0 ]] \
    && grep -Fq -- 'check: Context projection' "$case_log" \
    && grep -Fq -- 'reason: context-read-error' "$case_log" \
    && grep -Fq -- 'boundary: input-read' "$case_log" \
    && ! grep -Fq -- 'classification: direct-positive' "$case_log"; then
    bug032_iter10_assertion BUG032-SEC-001B-SOURCE-TOCTOU adversarial 0
  else
    printf 'BUG032_SEC001B_SOURCE_TOCTOU_MISMATCH status=%s catCalls=%s\n' \
      "$case_status" "$(wc -l < "$counter_file" | tr -d '[:space:]')"
    bug032_iter10_assertion BUG032-SEC-001B-SOURCE-TOCTOU adversarial 1
  fi

  case_dir="$focus_repo/specs/983-bug032-sec001b-regular-control"
  emit_base_fixture "$case_dir"
  mutate_delivery_contract "$case_dir/state.json"
  source_path="$case_dir/scopes.md"
  counter_file="$focus_root/sec001b-regular-control.count"
  : > "$counter_file"
  case_log="$focus_root/sec001b-regular-control.log"
  case_status="$(run_capture "$case_log" env \
    PATH="$focus_shadow_bin:$PATH" \
    BUG032_ITER10_REAL_CAT="$focus_real_cat" \
    BUG032_ITER10_SOURCE_PATH="$source_path" \
    BUG032_ITER10_CAT_COUNTER="$counter_file" \
    BUG032_ITER10_CAT_MODE=delegate \
    bash "$GUARD_SCRIPT" "$case_dir")"
  if [[ "$case_status" -eq 0 ]] \
    && [[ "$(wc -l < "$counter_file" | tr -d '[:space:]')" -ge 1 ]] \
    && ! grep -Fq -- 'context projection failed' "$case_log" \
    && ! grep -Fq -- 'check: Check 8B' "$case_log"; then
    bug032_iter10_assertion BUG032-SEC-001B-SOURCE-TOCTOU control 0
  else
    printf 'BUG032_SEC001B_REGULAR_CONTROL_MISMATCH status=%s catCalls=%s\n' \
      "$case_status" "$(wc -l < "$counter_file" | tr -d '[:space:]')"
    bug032_iter10_assertion BUG032-SEC-001B-SOURCE-TOCTOU control 1
  fi

  local line_exclusion_mismatches=0
  local -a line_exclusion_slugs=(case-a case-b)
  local -a line_exclusion_prose=(
    'followUpOwner: bubbles.plan; Move this work to a separate ticket.'
    'No deferred work is accepted; implement this in a future scope.'
  )
  local -a line_exclusion_phrases=('separate ticket' 'future scope')
  local -a line_exclusion_forms=('separate ticket' 'future scope')
  local line_exclusion_index=0
  for line_exclusion_index in "${!line_exclusion_slugs[@]}"; do
    case_dir="$focus_repo/specs/984-bug032-sec003a-${line_exclusion_slugs[$line_exclusion_index]}"
    emit_g040_fixture "$case_dir" "done" \
      "${line_exclusion_prose[$line_exclusion_index]}" no no
    case_log="$focus_root/sec003a-${line_exclusion_slugs[$line_exclusion_index]}.log"
    run_capture "$case_log" bash "$GUARD_SCRIPT" "$case_dir" >/dev/null
    if ! grep -Fq -- 'deferral language hit' "$case_log" \
      || ! grep -Fqi -- "canonical-phrase: ${line_exclusion_phrases[$line_exclusion_index]}" "$case_log" \
      || ! grep -Fq -- "matched-form: ${line_exclusion_forms[$line_exclusion_index]}" "$case_log"; then
      line_exclusion_mismatches=$((line_exclusion_mismatches + 1))
    fi
  done
  if [[ "$line_exclusion_mismatches" -eq 0 ]]; then
    bug032_iter10_assertion BUG032-SEC-003A-LINE-WIDE-EXCLUSION adversarial 0
  else
    printf 'BUG032_SEC003A_LINE_WIDE_EXCLUSION_MISMATCH cases=%s\n' \
      "$line_exclusion_mismatches"
    bug032_iter10_assertion BUG032-SEC-003A-LINE-WIDE-EXCLUSION adversarial 1
  fi

  local line_control_mismatches=0
  local -a line_control_slugs=(case-a case-b)
  local -a line_control_prose=(
    'followUpOwner: bubbles.plan'
    'No deferred work remains.'
  )
  local line_control_index=0
  for line_control_index in "${!line_control_slugs[@]}"; do
    case_dir="$focus_repo/specs/986-bug032-sec003a-control-${line_control_slugs[$line_control_index]}"
    emit_g040_fixture "$case_dir" "done" \
      "${line_control_prose[$line_control_index]}" no no
    case_log="$focus_root/sec003a-control-${line_control_slugs[$line_control_index]}.log"
    run_capture "$case_log" bash "$GUARD_SCRIPT" "$case_dir" >/dev/null
    if grep -Fq -- 'deferral language hit' "$case_log" \
      || ! grep -Fq -- \
        'Zero deferral language found in scope and report artifacts (Gate G040)' \
        "$case_log"; then
      line_control_mismatches=$((line_control_mismatches + 1))
    fi
  done
  if [[ "$line_control_mismatches" -eq 0 ]]; then
    bug032_iter10_assertion BUG032-SEC-003A-LINE-WIDE-EXCLUSION control 0
  else
    printf 'BUG032_SEC003A_LINE_CONTROL_MISMATCH cases=%s\n' \
      "$line_control_mismatches"
    bug032_iter10_assertion BUG032-SEC-003A-LINE-WIDE-EXCLUSION control 1
  fi

  BUG032_ITER10_SEC001_FAILURES="$sec001_failures"
  BUG032_ITER10_SEC003_FAILURES="$sec003_failures"
  printf 'BUG032_ITER10_ASSERTION_SUMMARY pass=%s fail=%s sec001=%s sec003=%s\n' \
    "$BUG032_ITER10_ASSERTION_PASSES" "$BUG032_ITER10_ASSERTION_FAILURES" \
    "$BUG032_ITER10_SEC001_FAILURES" "$BUG032_ITER10_SEC003_FAILURES"

  unset -f bug032_iter10_assertion bug032_iter10_emit_two_scope_fixture
  if [[ "$had_previous_repo_root" -eq 1 ]]; then
    export BUBBLES_REPO_ROOT="$previous_repo_root"
  else
    unset BUBBLES_REPO_ROOT
  fi
}

if [[ "${BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FOCUS:-}" \
  == "TP-01-04-security-boundary-group" ]]; then
  run_bug032_iteration10_security_assertions
  if [[ "$BUG032_ITER10_ASSERTION_FAILURES" -eq 0 ]]; then
    printf 'BUG032_ITER10_FOCUSED_VERDICT=PASS\n'
    exit 0
  fi
  failures="$BUG032_ITER10_ASSERTION_FAILURES"
  printf 'BUG032_ITER10_FOCUSED_VERDICT=RED failures=%s\n' \
    "$BUG032_ITER10_ASSERTION_FAILURES"
  exit 1
fi

assert_transition_result_contract_matches_emitter \
  "TRANSITION_GUARD_RESULT_V1 emitter field order matches this suite's expectation"

if [[ "${BUBBLES_STATE_TRANSITION_GUARD_BUG033_TIMEOUT_ONLY:-0}" == "1" ]]; then
  run_bug033_timeout_guard_assertions
  if [[ "$failures" -gt 0 ]]; then
    echo "state-transition-guard BUG-033 timeout selftest failed with $failures issue(s)."
    exit 1
  fi
  echo "state-transition-guard BUG-033 timeout selftest passed."
  exit 0
fi

if [[ "${BUBBLES_STATE_TRANSITION_GUARD_CONTAINMENT_ONLY:-0}" == "1" ]]; then
  run_strict_manifest_containment_regressions
  if [[ "$failures" -gt 0 ]]; then
    echo "state-transition-guard strict-containment selftest failed with $failures issue(s)."
    exit 1
  fi
  echo "state-transition-guard strict-containment selftest passed."
  exit 0
fi

positive_feature_dir="$tmp_root/specs/900-transition-guard-selftest-pass"
repo_root_isolation_feature_dir="$tmp_root/specs/900b-transition-guard-repo-root-isolation"
repo_root_isolation_ambient_dir="$tmp_root/ambient-hostile-cwd"
negative_feature_dir="$tmp_root/specs/901-transition-guard-selftest-missing-owner"
shared_positive_feature_dir="$tmp_root/specs/903-transition-guard-selftest-shared-pass"
shared_negative_feature_dir="$tmp_root/specs/904-transition-guard-selftest-shared-missing-controls"
workflow_mode_negative_feature_dir="$tmp_root/specs/905-transition-guard-selftest-workflowmode-mismatch"
per_scope_positive_feature_dir="$tmp_root/specs/906-transition-guard-selftest-per-scope-pass"
index_parity_negative_feature_dir="$tmp_root/specs/907-transition-guard-selftest-index-mismatch"
phantom_scope_negative_feature_dir="$tmp_root/specs/908-transition-guard-selftest-phantom-scope"
compact_completed_scopes_feature_dir="$tmp_root/specs/908c-transition-guard-selftest-compact-completed-scopes"
completed_scopes_precedence_feature_dir="$tmp_root/specs/908d-transition-guard-selftest-completed-scopes-precedence"
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
g057_id_only_dir="$tmp_root/specs/960-g057-id-only"
g057_scenario_id_only_dir="$tmp_root/specs/961-g057-scenario-id-only"
g057_mixed_aliases_dir="$tmp_root/specs/962-g057-mixed-aliases"
g057_both_aliases_dir="$tmp_root/specs/963-g057-both-aliases"
g057_blank_id_fallback_dir="$tmp_root/specs/964-g057-blank-id-fallback"
g057_invalid_id_fallback_dir="$tmp_root/specs/964b-g057-invalid-id-fallback"
g057_top_level_array_dir="$tmp_root/specs/965-g057-top-level-array"
g057_planned_tests_dir="$tmp_root/specs/965b-g057-planned-tests"
g057_duplicate_id_dir="$tmp_root/specs/966-g057-duplicate-id"
g057_duplicate_scenario_id_dir="$tmp_root/specs/966b-g057-duplicate-scenario-id"
g057_cross_alias_duplicate_dir="$tmp_root/specs/967-g057-cross-alias-duplicate"
g057_whitespace_duplicate_id_dir="$tmp_root/specs/967b-g057-whitespace-duplicate-id"
g057_whitespace_cross_alias_dir="$tmp_root/specs/967c-g057-whitespace-cross-alias"
g057_non_object_dir="$tmp_root/specs/968-g057-non-object"
g057_blank_identity_dir="$tmp_root/specs/969-g057-blank-identity"
g057_wrong_type_id_fallback_dir="$tmp_root/specs/970-g057-wrong-type-id-fallback"
g057_wrong_type_legacy_ignored_dir="$tmp_root/specs/970b-g057-wrong-type-legacy-ignored"
g057_wrong_type_legacy_id_dir="$tmp_root/specs/971-g057-wrong-type-legacy-id"
g057_wrong_type_both_dir="$tmp_root/specs/971b-g057-wrong-type-both"
g057_malformed_dir="$tmp_root/specs/972-g057-malformed"
g057_unsupported_dir="$tmp_root/specs/973-g057-unsupported"
g057_undercount_dir="$tmp_root/specs/974-g057-undercount"
g057_blank_title_dir="$tmp_root/specs/975-g057-blank-title"
g057_wrong_type_title_dir="$tmp_root/specs/976-g057-wrong-type-title"
g057_invalid_test_type_dir="$tmp_root/specs/977-g057-invalid-test-type"
g057_scalar_links_dir="$tmp_root/specs/978-g057-scalar-links"
g057_null_evidence_dir="$tmp_root/specs/979-g057-null-evidence"
g057_scalar_planned_tests_dir="$tmp_root/specs/980-g057-scalar-planned-tests"
g057_invalid_planned_test_dir="$tmp_root/specs/981-g057-invalid-planned-test"
g057_invalid_string_identity_dir="$tmp_root/specs/982-g057-invalid-string-identity"
g057_planning_planned_dir="$tmp_root/specs/983-g057-planning-planned"
g057_planning_authored_dir="$tmp_root/specs/984-g057-planning-authored"
g057_planning_neither_dir="$tmp_root/specs/985-g057-planning-neither"
g057_delivery_valid_dir="$tmp_root/specs/986-g057-delivery-valid"
g057_delivery_planned_dir="$tmp_root/specs/987-g057-delivery-planned"
g057_delivery_no_evidence_dir="$tmp_root/specs/988-g057-delivery-no-evidence"
g057_delivery_mixed_dir="$tmp_root/specs/989-g057-delivery-mixed"
g057_delivery_missing_file_dir="$tmp_root/specs/990-g057-delivery-missing-file"
g057_delivery_wrong_type_dir="$tmp_root/specs/991-g057-delivery-wrong-type"
g057_delivery_string_untyped_dir="$tmp_root/specs/991b-g057-delivery-string-untyped"
g057_delivery_object_untyped_dir="$tmp_root/specs/991c-g057-delivery-object-untyped"
g057_delivery_null_evidence_member_dir="$tmp_root/specs/991d-g057-delivery-null-evidence-member"
g057_delivery_numeric_evidence_member_dir="$tmp_root/specs/991e-g057-delivery-numeric-evidence-member"
g057_delivery_blank_evidence_member_dir="$tmp_root/specs/991f-g057-delivery-blank-evidence-member"
g057_delivery_whitespace_evidence_member_dir="$tmp_root/specs/991g-g057-delivery-whitespace-evidence-member"
g057_v2_dir="$tmp_root/specs/992-g057-v2"
g057_scoped_id_dir="$tmp_root/specs/993-g057-scoped-id"
g057_unknown_version_dir="$tmp_root/specs/994-g057-unknown-version"
g057_malformed_scoped_id_dir="$tmp_root/specs/995-g057-malformed-scoped-id"
g057_duplicate_effective_id_dir="$tmp_root/specs/996-g057-duplicate-effective-id"
g057_missing_title_dir="$tmp_root/specs/997-g057-missing-title"
g057_null_links_dir="$tmp_root/specs/998-g057-null-links"
g057_scalar_evidence_dir="$tmp_root/specs/999-g057-scalar-evidence"
g057_null_planned_dir="$tmp_root/specs/1000-g057-null-planned"
g057_invalid_link_member_dir="$tmp_root/specs/1001-g057-invalid-link-member"
g057_invalid_planned_member_dir="$tmp_root/specs/1002-g057-invalid-planned-member"
g057_legacy_count_only_dir="$tmp_root/specs/1003-g057-legacy-count-only"
g057_mixed_heading_wrong_id_dir="$tmp_root/specs/1004-g057-mixed-heading-wrong-id"
g057_mixed_heading_overcount_dir="$tmp_root/specs/1005-g057-mixed-heading-overcount"
g057_mixed_heading_valid_dir="$tmp_root/specs/1006-g057-mixed-heading-valid"
g057_duplicate_scope_known_id_dir="$tmp_root/specs/1007-g057-duplicate-scope-known-id"
g057_all_identified_surplus_dir="$tmp_root/specs/1008-g057-all-identified-surplus"
g060_planning_na_dir="$tmp_root/specs/928-bug026-g060-planning-not-applicable"
g060_delivery_enforced_dir="$tmp_root/specs/929-bug026-g060-delivery-enforced"
g040_planning_na_dir="$tmp_root/specs/930-g040-planning-not-applicable"
g040_pos_deferred_dir="$tmp_root/specs/920-g040-positive-deferred-prose"
g040_pos_skip_for_now_dir="$tmp_root/specs/921-g040-positive-skip-for-now"
g040_neg_followup_fields_dir="$tmp_root/specs/922-g040-negative-schema-yaml-only"
g040_neg_placeholder_noun_dir="$tmp_root/specs/938-g040-negative-placeholder-noun"
g040_pos_placeholder_admission_dir="$tmp_root/specs/939-g040-positive-placeholder-admission"
g040_neg_done_with_concerns_dir="$tmp_root/specs/923-g040-negative-done-with-concerns"
g040_neg_skip_markers_dir="$tmp_root/specs/924-g040-negative-skip-markers"
g040_pos_skip_marker_outside_dir="$tmp_root/specs/925-g040-positive-skip-marker-outside"
g040_neg_spec_063_excerpt_dir="$tmp_root/specs/926-g040-negative-spec-063-excerpt"
g040_pos_strict_done_mixed_dir="$tmp_root/specs/927-g040-positive-strict-done-mixed"
g040_cw_pre_skipped_dir="$tmp_root/specs/934-g040-cw-pre-marker-skipped"
g040_cw_post_blocks_dir="$tmp_root/specs/935-g040-cw-post-marker-blocks"
g040_cw_no_marker_dir="$tmp_root/specs/936-g040-cw-no-marker-full-enforcement"
g040_cw_two_markers_dir="$tmp_root/specs/937-g040-cw-two-markers-fail-loud"
# Keep deferral terms out of these paths because emit_base_fixture records each
# absolute fixture path in scopes.md, which Check 18 scans.
g040_neg_exposure_label_dir="$tmp_root/specs/942-g040-negative-exposure-label"
g040_pos_exposure_reason_dir="$tmp_root/specs/943-g040-positive-exposure-reason"
fast_lane_profile_dir="$tmp_root/specs/940-fast-lane-profile-resolve"
framework_proposal_profile_dir="$tmp_root/specs/941-framework-proposal-profile-resolve"
g064_framework_root="$tmp_root/framework-g064"
g064_feature_dir="$g064_framework_root/specs/902-transition-guard-selftest-unauthorized-workflow-runner"
broken_phase_owner_root="$tmp_root/framework-broken-phase-owner"
broken_phase_owner_dir="$broken_phase_owner_root/specs/944-transition-guard-selftest-broken-phase-owner"
malformed_capability_owner_root="$tmp_root/framework-malformed-capability-owner"
malformed_capability_owner_dir="$malformed_capability_owner_root/specs/945-transition-guard-selftest-malformed-capability-owner"
missing_capability_owner_root="$tmp_root/framework-missing-capability-owner"
missing_capability_owner_dir="$missing_capability_owner_root/specs/946-transition-guard-selftest-missing-capability-owner"
explicit_owner_conflict_root="$tmp_root/framework-explicit-owner-conflict"
explicit_owner_conflict_dir="$explicit_owner_conflict_root/specs/947-transition-guard-selftest-explicit-owner-conflict"
malformed_runner_grants_root="$tmp_root/framework-malformed-runner-grants"
malformed_runner_grants_dir="$malformed_runner_grants_root/specs/950-transition-guard-selftest-malformed-runner-grants"
sequence_runner_grants_root="$tmp_root/framework-sequence-runner-grants"
sequence_runner_grants_dir="$sequence_runner_grants_root/specs/951-transition-guard-selftest-sequence-runner-grants"
unknown_runner_grant_root="$tmp_root/framework-unknown-runner-grant"
unknown_runner_grant_dir="$unknown_runner_grant_root/specs/952-transition-guard-selftest-unknown-runner-grant"
mkdir -p "$tmp_root/specs"
clone_framework_surface "$tmp_root"
git -C "$tmp_root" init -q
export BUBBLES_REPO_ROOT="$tmp_root"
GUARD_SCRIPT="$tmp_root/bubbles/scripts/state-transition-guard.sh"
cd "$tmp_root"

if [[ "${BUBBLES_STATE_TRANSITION_GUARD_BUG033_ONLY:-0}" != "1" ]]; then
dogfood_done_dir="$tmp_root/specs/899-transition-guard-selftest-dogfood-done"
mkdir -p "$dogfood_done_dir"
cat <<'EOF' > "$dogfood_done_dir/state.json"
{
  "status": "done"
}
EOF

emit_base_fixture "$positive_feature_dir"
mutate_delivery_contract "$positive_feature_dir/state.json"
cp -R "$positive_feature_dir" "$repo_root_isolation_feature_dir"
mkdir -p "$repo_root_isolation_ambient_dir/.github"
cat <<'EOF' > "$repo_root_isolation_ambient_dir/.github/bubbles-project.yaml"
scans:
  testEnvDependency:
    patterns: Agent ownership lint passed
EOF
mkdir -p "$tmp_root/.specify/memory"
cat <<'EOF' > "$tmp_root/.specify/memory/bubbles.session.json"
{
  "executionRuntime": "manual"
}
EOF
cat <<'EOF' > "$repo_root_isolation_feature_dir/uservalidation.md"
# User Validation

## Checklist

- [x] The guarded-repository root isolation behavior is accepted.

## Human Acceptance Record

- acceptedBy: repository-root-selftest-human
- acceptedAt: 2026-08-19T08:00:00Z
- method: human-interactive
EOF
cp -R "$positive_feature_dir" "$negative_feature_dir"
cp -R "$positive_feature_dir" "$compact_completed_scopes_feature_dir"
expand_to_compact_three_scope_fixture "$compact_completed_scopes_feature_dir"
cp -R "$positive_feature_dir" "$completed_scopes_precedence_feature_dir"
set_completed_scopes_precedence_fixture "$completed_scopes_precedence_feature_dir/state.json"
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
# Canonical positive: this document conforms to scenario-manifest.schema.json.
cp -R "$s03_planning_feature_dir" "$g057_id_only_dir"
write_g057_manifest "$g057_id_only_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-001","title":"Planning maturity preserves honest incomplete delivery","requiredTestType":"e2e-ui","linkedTests":["__FUTURE_TEST__"],"evidenceRefs":[]}]}'
# Compatibility positives: legacy scenarioId and the legacy top-level array are
# accepted read formats but are not canonical documents for new producers.
cp -R "$s03_planning_feature_dir" "$g057_scenario_id_only_dir"
write_g057_manifest "$g057_scenario_id_only_dir" '{"schemaVersion":1,"scenarios":[{"scenarioId":"SCN-G057-002","title":"Legacy identity compatibility","requiredTestType":"e2e-ui","linkedTests":["__FUTURE_TEST__"],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_mixed_aliases_dir"
write_g057_manifest "$g057_mixed_aliases_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-003","title":"Canonical identity record","requiredTestType":"e2e-ui","linkedTests":["__FUTURE_TEST__"],"evidenceRefs":[]},{"scenarioId":"SCN-G057-004","title":"Legacy identity record","requiredTestType":"e2e-ui","linkedTests":["__FUTURE_TEST__"],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_both_aliases_dir"
write_g057_manifest "$g057_both_aliases_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-005","scenarioId":"SCN-G057-999","title":"Canonical identity wins","requiredTestType":"e2e-ui","linkedTests":["__FUTURE_TEST__"],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_blank_id_fallback_dir"
write_g057_manifest "$g057_blank_id_fallback_dir" '{"schemaVersion":1,"scenarios":[{"id":"   ","scenarioId":"SCN-G057-006","title":"Blank canonical identity falls back","requiredTestType":"e2e-ui","linkedTests":["__FUTURE_TEST__"],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_invalid_id_fallback_dir"
write_g057_manifest "$g057_invalid_id_fallback_dir" '{"schemaVersion":1,"scenarios":[{"id":"not-a-scenario-id","scenarioId":"SCN-G057-016","title":"Invalid canonical identity falls back","requiredTestType":"e2e-ui","linkedTests":["__FUTURE_TEST__"],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_top_level_array_dir"
write_g057_manifest "$g057_top_level_array_dir" '[{"scenarioId":"SCN-G057-007","title":"Legacy array compatibility","requiredTestType":"e2e-ui","linkedTests":["__FUTURE_TEST__"],"evidenceRefs":[]}]'
cp -R "$s03_planning_feature_dir" "$g057_planned_tests_dir"
write_g057_manifest "$g057_planned_tests_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-017","title":"Canonical planned test metadata","requiredTestType":"e2e-ui","linkedTests":["__FUTURE_TEST__"],"plannedTests":[{"path":"tests/future.spec.ts","title":"future behavior","type":"e2e-ui"}],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_duplicate_id_dir"
write_g057_manifest "$g057_duplicate_id_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-008","title":"First duplicate","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]},{"id":"SCN-G057-008","title":"Second duplicate","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_duplicate_scenario_id_dir"
write_g057_manifest "$g057_duplicate_scenario_id_dir" '{"schemaVersion":1,"scenarios":[{"scenarioId":"SCN-G057-018","title":"First legacy duplicate","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]},{"scenarioId":"SCN-G057-018","title":"Second legacy duplicate","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_cross_alias_duplicate_dir"
write_g057_manifest "$g057_cross_alias_duplicate_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-009","title":"Canonical duplicate","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]},{"scenarioId":"SCN-G057-009","title":"Cross alias duplicate","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_whitespace_duplicate_id_dir"
write_g057_manifest "$g057_whitespace_duplicate_id_dir" '{"schemaVersion":1,"scenarios":[{"id":" SCN-G057-019","title":"Whitespace duplicate first","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]},{"id":"SCN-G057-019 ","title":"Whitespace duplicate second","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_whitespace_cross_alias_dir"
write_g057_manifest "$g057_whitespace_cross_alias_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-020","title":"Canonical whitespace duplicate","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]},{"scenarioId":"  SCN-G057-020  ","title":"Legacy whitespace duplicate","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_non_object_dir"
write_g057_manifest "$g057_non_object_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-010","title":"Valid neighbor","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]},"SCN-G057-011"]}'
cp -R "$s03_planning_feature_dir" "$g057_blank_identity_dir"
write_g057_manifest "$g057_blank_identity_dir" '{"schemaVersion":1,"scenarios":[{"id":" ","scenarioId":"\t","title":"Blank identities","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_wrong_type_id_fallback_dir"
write_g057_manifest "$g057_wrong_type_id_fallback_dir" '{"schemaVersion":1,"scenarios":[{"id":57,"scenarioId":"SCN-G057-012","title":"Invalid canonical identity falls back","requiredTestType":"e2e-ui","linkedTests":["__FUTURE_TEST__"],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_wrong_type_legacy_ignored_dir"
write_g057_manifest "$g057_wrong_type_legacy_ignored_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-015","scenarioId":57,"title":"Canonical identity ignores invalid legacy alias","requiredTestType":"e2e-ui","linkedTests":["__FUTURE_TEST__"],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_wrong_type_legacy_id_dir"
write_g057_manifest "$g057_wrong_type_legacy_id_dir" '{"schemaVersion":1,"scenarios":[{"scenarioId":57,"title":"Wrong legacy identity type","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_wrong_type_both_dir"
write_g057_manifest "$g057_wrong_type_both_dir" '{"schemaVersion":1,"scenarios":[{"id":57,"scenarioId":58,"title":"Both identities have wrong types","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_malformed_dir"
write_g057_manifest "$g057_malformed_dir" '{"schemaVersion":1,"scenarios":['
cp -R "$s03_planning_feature_dir" "$g057_unsupported_dir"
write_g057_manifest "$g057_unsupported_dir" '{"schemaVersion":2,"scenarios":[{"id":"SCN-G057-013"}]}'
cp -R "$s03_planning_feature_dir" "$g057_undercount_dir"
cat <<'EOF' >> "$g057_undercount_dir/scopes.md"

```gherkin
Scenario: A second scope contract remains represented
Given two resolved scope scenarios
When the scenario manifest tracks only one record
Then G057 reports a real undercount
```

- [ ] A second scope contract remains represented when G057 checks the manifest count.
EOF
write_g057_manifest "$g057_undercount_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-014","title":"Only one represented scenario","requiredTestType":"e2e-ui","linkedTests":["__FUTURE_TEST__"],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_blank_title_dir"
write_g057_manifest "$g057_blank_title_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-021","title":"   ","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_wrong_type_title_dir"
write_g057_manifest "$g057_wrong_type_title_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-022","title":57,"requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_invalid_test_type_dir"
write_g057_manifest "$g057_invalid_test_type_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-023","title":"Unknown test taxonomy","requiredTestType":"e2e","linkedTests":[],"evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_scalar_links_dir"
write_g057_manifest "$g057_scalar_links_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-024","title":"Scalar linked tests","requiredTestType":"e2e-ui","linkedTests":"tests/demo.spec.ts","evidenceRefs":[]}]}'
cp -R "$s03_planning_feature_dir" "$g057_null_evidence_dir"
write_g057_manifest "$g057_null_evidence_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-025","title":"Null evidence refs","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":null}]}'
cp -R "$s03_planning_feature_dir" "$g057_scalar_planned_tests_dir"
write_g057_manifest "$g057_scalar_planned_tests_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-026","title":"Scalar planned tests","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[],"plannedTests":"tests/future.spec.ts"}]}'
cp -R "$s03_planning_feature_dir" "$g057_invalid_planned_test_dir"
write_g057_manifest "$g057_invalid_planned_test_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-027","title":"Invalid planned test member","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[],"plannedTests":[{"path":"tests/future.spec.ts","title":"future behavior"}]}]}'
cp -R "$s03_planning_feature_dir" "$g057_invalid_string_identity_dir"
write_g057_manifest "$g057_invalid_string_identity_dir" '{"schemaVersion":1,"scenarios":[{"id":"not-a-scenario-id","title":"Invalid string identity","requiredTestType":"e2e-ui","linkedTests":[],"evidenceRefs":[]}]}'

# Binding G057 profile matrix. These fixtures use the reader's normalized
# authored/planned projection and keep every path repository-relative.
cp -R "$s03_planning_feature_dir" "$g057_planning_planned_dir"
bubbles_sed_inplace 's/SCN-009-S03-001/SCN-G057-101/g' "$g057_planning_planned_dir/spec.md"
bubbles_sed_inplace 's/SCN-009-S03-001/SCN-G057-101/g' "$g057_planning_planned_dir/scopes.md"
write_g057_manifest "$g057_planning_planned_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-101","title":"Planning maturity preserves honest incomplete delivery","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/future.spec.ts","title":"future behavior","type":"e2e-ui"}],"evidenceRefs":[]}]}'

cp -R "$s03_planning_feature_dir" "$g057_planning_authored_dir"
bubbles_sed_inplace 's/SCN-009-S03-001/SCN-G057-102/g' "$g057_planning_authored_dir/spec.md"
bubbles_sed_inplace 's/SCN-009-S03-001/SCN-G057-102/g' "$g057_planning_authored_dir/scopes.md"
mkdir -p "$g057_planning_authored_dir/tests"
printf '%s\n' "test('g057 planning authored behavior', () => {});" > "$g057_planning_authored_dir/tests/g057-planning-authored.e2e.spec.ts"
write_g057_manifest "$g057_planning_authored_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-102","title":"Planning maturity preserves honest incomplete delivery","requiredTestType":"e2e-ui","linkedTests":[{"file":"specs/984-g057-planning-authored/tests/g057-planning-authored.e2e.spec.ts","title":"g057 planning authored behavior","type":"e2e-ui"}],"evidenceRefs":[]}]}'

cp -R "$s03_planning_feature_dir" "$g057_planning_neither_dir"
bubbles_sed_inplace 's/SCN-009-S03-001/SCN-G057-103/g' "$g057_planning_neither_dir/spec.md"
bubbles_sed_inplace 's/SCN-009-S03-001/SCN-G057-103/g' "$g057_planning_neither_dir/scopes.md"
write_g057_manifest "$g057_planning_neither_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-103","title":"Planning maturity preserves honest incomplete delivery","requiredTestType":"e2e-ui","linkedTests":[],"plannedTests":[],"evidenceRefs":[]}]}'

cp -R "$positive_feature_dir" "$g057_delivery_valid_dir"
append_g057_scenario "$g057_delivery_valid_dir" "SCN-G057-104"
write_g057_manifest "$g057_delivery_valid_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-104","title":"G057 profile classification","requiredTestType":"e2e-ui","linkedTests":[{"file":"specs/986-g057-delivery-valid/tests/docs-scenario-regression.e2e.spec.ts","title":"docsScenarioRegression","type":"e2e-ui"}],"evidenceRefs":["report.md#test-evidence"]}]}'

cp -R "$g057_delivery_valid_dir" "$g057_delivery_planned_dir"
write_g057_manifest "$g057_delivery_planned_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-104","title":"G057 profile classification","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/future.spec.ts","title":"future behavior","type":"e2e-ui"}],"evidenceRefs":[]}]}'

cp -R "$g057_delivery_valid_dir" "$g057_delivery_no_evidence_dir"
write_g057_manifest "$g057_delivery_no_evidence_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-104","title":"G057 profile classification","requiredTestType":"e2e-ui","linkedTests":[{"file":"specs/988-g057-delivery-no-evidence/tests/docs-scenario-regression.e2e.spec.ts","title":"docsScenarioRegression","type":"e2e-ui"}],"evidenceRefs":[]}]}'

cp -R "$g057_delivery_valid_dir" "$g057_delivery_mixed_dir"
write_g057_manifest "$g057_delivery_mixed_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-104","title":"G057 profile classification","requiredTestType":"e2e-ui","linkedTests":[{"file":"specs/989-g057-delivery-mixed/tests/docs-scenario-regression.e2e.spec.ts","title":"docsScenarioRegression","type":"e2e-ui"}],"plannedTests":[{"path":"tests/future.spec.ts","title":"future behavior","type":"e2e-ui"}],"evidenceRefs":["report.md#test-evidence"]}]}'

cp -R "$g057_delivery_valid_dir" "$g057_delivery_missing_file_dir"
write_g057_manifest "$g057_delivery_missing_file_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-104","title":"G057 profile classification","requiredTestType":"e2e-ui","linkedTests":[{"file":"specs/990-g057-delivery-missing-file/tests/absent.e2e.spec.ts","title":"absent behavior","type":"e2e-ui"}],"evidenceRefs":["report.md#test-evidence"]}]}'

cp -R "$g057_delivery_valid_dir" "$g057_delivery_wrong_type_dir"
write_g057_manifest "$g057_delivery_wrong_type_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-104","title":"G057 profile classification","requiredTestType":"e2e-ui","linkedTests":[{"file":"specs/991-g057-delivery-wrong-type/tests/docs-scenario-regression.e2e.spec.ts","title":"docsScenarioRegression","type":"unit"}],"evidenceRefs":["report.md#test-evidence"]}]}'

cp -R "$g057_delivery_valid_dir" "$g057_delivery_string_untyped_dir"
write_g057_manifest "$g057_delivery_string_untyped_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-104","title":"G057 profile classification","requiredTestType":"e2e-ui","linkedTests":["specs/991b-g057-delivery-string-untyped/tests/docs-scenario-regression.e2e.spec.ts"],"evidenceRefs":["report.md#test-evidence"]}]}'

cp -R "$g057_delivery_valid_dir" "$g057_delivery_object_untyped_dir"
write_g057_manifest "$g057_delivery_object_untyped_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-104","title":"G057 profile classification","requiredTestType":"e2e-ui","linkedTests":[{"file":"specs/991c-g057-delivery-object-untyped/tests/docs-scenario-regression.e2e.spec.ts","title":"docsScenarioRegression"}],"evidenceRefs":["report.md#test-evidence"]}]}'

cp -R "$g057_delivery_valid_dir" "$g057_delivery_null_evidence_member_dir"
write_g057_manifest "$g057_delivery_null_evidence_member_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-104","title":"G057 profile classification","requiredTestType":"e2e-ui","linkedTests":[{"file":"specs/991d-g057-delivery-null-evidence-member/tests/docs-scenario-regression.e2e.spec.ts","title":"docsScenarioRegression","type":"e2e-ui"}],"evidenceRefs":[null]}]}'

cp -R "$g057_delivery_valid_dir" "$g057_delivery_numeric_evidence_member_dir"
write_g057_manifest "$g057_delivery_numeric_evidence_member_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-104","title":"G057 profile classification","requiredTestType":"e2e-ui","linkedTests":[{"file":"specs/991e-g057-delivery-numeric-evidence-member/tests/docs-scenario-regression.e2e.spec.ts","title":"docsScenarioRegression","type":"e2e-ui"}],"evidenceRefs":[57]}]}'

cp -R "$g057_delivery_valid_dir" "$g057_delivery_blank_evidence_member_dir"
write_g057_manifest "$g057_delivery_blank_evidence_member_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-104","title":"G057 profile classification","requiredTestType":"e2e-ui","linkedTests":[{"file":"specs/991f-g057-delivery-blank-evidence-member/tests/docs-scenario-regression.e2e.spec.ts","title":"docsScenarioRegression","type":"e2e-ui"}],"evidenceRefs":[""]}]}'

cp -R "$g057_delivery_valid_dir" "$g057_delivery_whitespace_evidence_member_dir"
write_g057_manifest "$g057_delivery_whitespace_evidence_member_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-104","title":"G057 profile classification","requiredTestType":"e2e-ui","linkedTests":[{"file":"specs/991g-g057-delivery-whitespace-evidence-member/tests/docs-scenario-regression.e2e.spec.ts","title":"docsScenarioRegression","type":"e2e-ui"}],"evidenceRefs":["  \t  "]}]}'

cp -R "$positive_feature_dir" "$g057_v2_dir"
append_g057_scenario "$g057_v2_dir" "SCN-G057-105"
write_g057_manifest "$g057_v2_dir" '{"schemaVersion":2,"scenarios":[{"id":"SCN-G057-105","title":"G057 profile classification","requiredTestType":"e2e-ui","linkedTests":[{"file":"specs/992-g057-v2/tests/docs-scenario-regression.e2e.spec.ts","testId":"docsScenarioRegression","type":"e2e-ui"}],"evidenceRefs":["report.md#test-evidence"]}]}'
append_g057_delivery_receipts "SCN-G057-104"
append_g057_delivery_receipts "SCN-G057-105"

cp -R "$s03_planning_feature_dir" "$g057_scoped_id_dir"
bubbles_sed_inplace 's/SCN-009-S03-001/SCN-WEB-API-MOBILE-42/g' "$g057_scoped_id_dir/spec.md"
bubbles_sed_inplace 's/SCN-009-S03-001/SCN-WEB-API-MOBILE-42/g' "$g057_scoped_id_dir/scopes.md"
write_g057_manifest "$g057_scoped_id_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-WEB-API-MOBILE-42","title":"Planning maturity preserves honest incomplete delivery","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/future.spec.ts","title":"future behavior","type":"e2e-ui"}],"evidenceRefs":[]}]}'

cp -R "$s03_planning_feature_dir" "$g057_unknown_version_dir"
write_g057_manifest "$g057_unknown_version_dir" '{"schemaVersion":99,"scenarios":[{"id":"SCN-009-S03-001","title":"Planning maturity preserves honest incomplete delivery","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/future.spec.ts","title":"future behavior","type":"e2e-ui"}],"evidenceRefs":[]}]}'

cp -R "$s03_planning_feature_dir" "$g057_malformed_scoped_id_dir"
bubbles_sed_inplace 's/SCN-009-S03-001/SCN-009-001-X/g' "$g057_malformed_scoped_id_dir/spec.md"
bubbles_sed_inplace 's/SCN-009-S03-001/SCN-009-001-X/g' "$g057_malformed_scoped_id_dir/scopes.md"
write_g057_manifest "$g057_malformed_scoped_id_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-009-001-X","title":"Planning maturity preserves honest incomplete delivery","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/future.spec.ts","title":"future behavior","type":"e2e-ui"}],"evidenceRefs":[]}]}'

cp -R "$s03_planning_feature_dir" "$g057_duplicate_effective_id_dir"
write_g057_manifest "$g057_duplicate_effective_id_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-009-S03-001","title":"First duplicate","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/first.spec.ts","title":"first","type":"e2e-ui"}],"evidenceRefs":[]},{"scenarioId":"SCN-009-S03-001","title":"Second duplicate","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/second.spec.ts","title":"second","type":"e2e-ui"}],"evidenceRefs":[]}]}'

cp -R "$s03_planning_feature_dir" "$g057_missing_title_dir"
write_g057_manifest "$g057_missing_title_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-009-S03-001","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/future.spec.ts","title":"future behavior","type":"e2e-ui"}],"evidenceRefs":[]}]}'

cp -R "$s03_planning_feature_dir" "$g057_null_links_dir"
write_g057_manifest "$g057_null_links_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-009-S03-001","title":"Null linked tests","requiredTestType":"e2e-ui","linkedTests":null,"evidenceRefs":[]}]}'

cp -R "$s03_planning_feature_dir" "$g057_scalar_evidence_dir"
write_g057_manifest "$g057_scalar_evidence_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-009-S03-001","title":"Scalar evidence refs","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/future.spec.ts","title":"future behavior","type":"e2e-ui"}],"evidenceRefs":"report.md#test-evidence"}]}'

cp -R "$s03_planning_feature_dir" "$g057_null_planned_dir"
write_g057_manifest "$g057_null_planned_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-009-S03-001","title":"Null planned tests","requiredTestType":"e2e-ui","plannedTests":null,"evidenceRefs":[]}]}'

cp -R "$s03_planning_feature_dir" "$g057_invalid_link_member_dir"
write_g057_manifest "$g057_invalid_link_member_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-009-S03-001","title":"Invalid linked-test member","requiredTestType":"e2e-ui","linkedTests":[{}],"evidenceRefs":[]}]}'

cp -R "$s03_planning_feature_dir" "$g057_invalid_planned_member_dir"
write_g057_manifest "$g057_invalid_planned_member_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-009-S03-001","title":"Invalid planned-test member","requiredTestType":"e2e-ui","plannedTests":[{}],"evidenceRefs":[]}]}'

cp -R "$s03_planning_feature_dir" "$g057_legacy_count_only_dir"
bubbles_sed_inplace 's/^### SCN-009-S03-001/### Legacy scenario without stable ID/' "$g057_legacy_count_only_dir/scopes.md"
write_g057_manifest "$g057_legacy_count_only_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-G057-106","title":"Legacy count-only compatibility","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/future.spec.ts","title":"future behavior","type":"e2e-ui"}],"evidenceRefs":[]}]}'

cp -R "$s03_planning_feature_dir" "$g057_mixed_heading_wrong_id_dir"
cat <<'EOF' >> "$g057_mixed_heading_wrong_id_dir/scopes.md"

## Legacy Compatibility Scenario

```gherkin
Scenario: A residual legacy heading remains count-compatible
Given one stable scenario and one legacy scenario
When G057 reconciles identities
Then the stable identity still matches exactly
```

## Definition of Done

- [ ] Given one stable scenario and one legacy scenario, when G057 reconciles identities, then the stable identity still matches exactly and the residual legacy heading remains count-compatible.
EOF
write_g057_manifest "$g057_mixed_heading_wrong_id_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-WRONG-900","title":"Wrong known identity","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/wrong.spec.ts","title":"wrong known identity","type":"e2e-ui"}],"evidenceRefs":[]},{"id":"SCN-LEGACY-901","title":"Legacy count representative","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/legacy.spec.ts","title":"legacy representative","type":"e2e-ui"}],"evidenceRefs":[]}]}'

cp -R "$g057_mixed_heading_wrong_id_dir" "$g057_mixed_heading_overcount_dir"
write_g057_manifest "$g057_mixed_heading_overcount_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-009-S03-001","title":"Known stable identity","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/known.spec.ts","title":"known stable identity","type":"e2e-ui"}],"evidenceRefs":[]},{"id":"SCN-LEGACY-901","title":"Legacy count representative","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/legacy.spec.ts","title":"legacy representative","type":"e2e-ui"}],"evidenceRefs":[]},{"id":"SCN-EXTRA-902","title":"Overcount must fail","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/extra.spec.ts","title":"extra overcount","type":"e2e-ui"}],"evidenceRefs":[]}]}'

cp -R "$g057_mixed_heading_wrong_id_dir" "$g057_mixed_heading_valid_dir"
write_g057_manifest "$g057_mixed_heading_valid_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-009-S03-001","title":"Known stable identity","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/known.spec.ts","title":"known stable identity","type":"e2e-ui"}],"evidenceRefs":[]},{"id":"SCN-LEGACY-901","title":"Unidentified residual cardinality representative","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/legacy.spec.ts","title":"legacy residual","type":"e2e-ui"}],"evidenceRefs":[]}]}'

cp -R "$s03_planning_feature_dir" "$g057_duplicate_scope_known_id_dir"
cat <<'EOF' >> "$g057_duplicate_scope_known_id_dir/scopes.md"

### SCN-009-S03-001 - Duplicate stable identity

```gherkin
Scenario: A duplicate stable identity is rejected
Given two scope scenarios carry one stable ID
When G057 reconciles identified multiplicity
Then the duplicate is rejected
```

## Definition of Done

- [ ] Given two scope scenarios carry one stable ID, when G057 reconciles identified multiplicity, then the duplicate stable identity is rejected.
EOF
write_g057_manifest "$g057_duplicate_scope_known_id_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-009-S03-001","title":"Planning maturity preserves honest incomplete delivery","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/future.spec.ts","title":"future behavior","type":"e2e-ui"}],"evidenceRefs":[]},{"id":"SCN-LEGACY-902","title":"Cardinality peer","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/peer.spec.ts","title":"peer","type":"e2e-ui"}],"evidenceRefs":[]}]}'

cp -R "$s03_planning_feature_dir" "$g057_all_identified_surplus_dir"
write_g057_manifest "$g057_all_identified_surplus_dir" '{"schemaVersion":1,"scenarios":[{"id":"SCN-009-S03-001","title":"Known stable identity","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/known.spec.ts","title":"known","type":"e2e-ui"}],"evidenceRefs":[]},{"id":"SCN-SURPLUS-902","title":"Surplus identified manifest record","requiredTestType":"e2e-ui","plannedTests":[{"path":"tests/surplus.spec.ts","title":"surplus","type":"e2e-ui"}],"evidenceRefs":[]}]}'
if [[ "${BUBBLES_STATE_TRANSITION_GUARD_G057_ONLY:-0}" == "1" ]]; then
  run_focused_g057_assertions
  if [[ "$failures" -gt 0 ]]; then
    echo "state-transition-guard G057 selftest failed with $failures issue(s)."
    exit 1
  fi
  echo "state-transition-guard G057 selftest passed."
  exit 0
fi

if [[ "${BUBBLES_STATE_TRANSITION_GUARD_G061_ONLY:-0}" == "1" ]]; then
  run_g061_regression_cases
  if [[ "$failures" -gt 0 ]]; then
    echo "state-transition-guard G061 selftest failed with $failures issue(s)."
    exit 1
  fi
  echo "state-transition-guard G061 selftest passed."
  exit 0
fi

run_g061_regression_cases
# The other two profiles transition-contract-resolver.sh supports. Both were
# unreachable through the guard until the contract validator's auditProfile
# allow-list was widened to the resolver's full four-profile set, so neither had
# any fixture at all. The fast-lane packet is an otherwise-passing delivery
# packet re-pointed at rapid-tool-delivery; the framework-proposal packet is the
# honestly-unimplemented delivery negative re-pointed at framework-health, so it
# carries exactly the four artifacts the declared exclusions cover.
cp -R "$positive_feature_dir" "$fast_lane_profile_dir"
set_fixture_contract "$fast_lane_profile_dir/state.json" "rapid-tool-delivery" "in_progress"
cp -R "$s03_delivery_negative_dir" "$framework_proposal_profile_dir"
set_fixture_contract "$framework_proposal_profile_dir/state.json" "framework-health" "in_progress"
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
# The bare noun `placeholder` is ordinary UI/DOM/test vocabulary, and it appears
# most often in prose that FORBIDS one. Every sentence below asserts the OPPOSITE
# of deferral, so none may block. The last clause is adversarial: it names a
# placeholder as the object of a completed action, which is still not an
# admission that anything was left unfinished.
emit_g040_fixture "$g040_neg_placeholder_noun_dir" "done" \
  "The empty state renders with no placeholder card, and the builder must not synthesise a placeholder item. The record placeholder text is asserted verbatim. The adversarial probe confirmed the node was replaced with a placeholder and then restored byte-identical." \
  "no" "no"
# Adversarial twin of the case above: the narrowing must NOT have disabled the
# term. A genuine admission that something IS a placeholder still blocks.
emit_g040_fixture "$g040_pos_placeholder_admission_dir" "done" \
  "The scoring weight is a placeholder value until the calibration lands." \
  "no" "no"
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

# G040 / Check 18 certifying-window boundary fixtures (report.md marker parity
# with artifact-lint.sh Check 3). Four adversarial shapes: pre-marker deferral
# is frozen prior-window history (skipped); post-marker deferral still blocks;
# a marker-less report is enforced in full; two markers fail loud with no
# exemption.
emit_g040_cw_fixture "$g040_cw_pre_skipped_dir" 1 \
  "Several action items were deferred to next sprint in the prior release cycle." ""
emit_g040_cw_fixture "$g040_cw_post_blocks_dir" 1 \
  "All prior-round work is complete and green." \
  "One migration task was deferred to next sprint and is not done."
emit_g040_cw_fixture "$g040_cw_no_marker_dir" 0 \
  "Several action items were deferred to next sprint per planning notes." ""
emit_g040_cw_fixture "$g040_cw_two_markers_dir" 2 \
  "Several action items were deferred to next sprint in the prior release cycle." ""

emit_g040_exposure_fixture "$g040_neg_exposure_label_dir" \
  "this scope ships guard configuration only and has no runnable consumer surface"
emit_g040_exposure_fixture "$g040_pos_exposure_reason_dir" \
  "punted to a future iteration"

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

# Check 6 regression fixture (short-circuit `or` on the phase source). A state.json
# with a NON-EMPTY but PARTIAL certifiedCompletedPhases (["validate"]) plus the
# remaining required phase carried ONLY as a dict-shaped execution claim
# ({"phase": "audit", ...}) previously reported 'audit' as unrecorded, because a
# truthy certification list short-circuited execution.completedPhaseClaims out of
# the selection entirely. The sibling fixture above covers the EMPTY-certification
# path; this one covers the PARTIAL path the `or` left unguarded.
partial_certified_phase_claims_dir="$tmp_root/specs/940-transition-guard-selftest-partial-certified-phase-claims"
cp -R "$positive_feature_dir" "$partial_certified_phase_claims_dir"
mutate_partial_certified_phases_with_dict_claims "$partial_certified_phase_claims_dir/state.json"

# Check 6B phase -> owning-agent resolution fixtures. The positive case records
# the REAL owner of `analyze` (bubbles.analyst); the adversarial twin records an
# unrelated agent for the same claim and must still be refused, so the fix
# cannot pass by disabling the impersonation check.
analyst_owned_phase_dir="$tmp_root/specs/941-transition-guard-selftest-analyst-owned-phase"
cp -R "$positive_feature_dir" "$analyst_owned_phase_dir"
mutate_analyze_phase_provenance "$analyst_owned_phase_dir/state.json" "bubbles.analyst"

ux_owned_phase_dir="$tmp_root/specs/948-transition-guard-selftest-ux-owned-phase"
cp -R "$positive_feature_dir" "$ux_owned_phase_dir"
mutate_phase_provenance "$ux_owned_phase_dir/state.json" "analyze" "bubbles.ux"

plan_owned_phase_dir="$tmp_root/specs/949-transition-guard-selftest-plan-owned-phase"
cp -R "$positive_feature_dir" "$plan_owned_phase_dir"
mutate_phase_provenance "$plan_owned_phase_dir/state.json" "bootstrap" "bubbles.plan"

analyze_wrong_agent_dir="$tmp_root/specs/942-transition-guard-selftest-analyze-wrong-agent"
cp -R "$positive_feature_dir" "$analyze_wrong_agent_dir"
mutate_analyze_phase_provenance "$analyze_wrong_agent_dir/state.json" "bubbles.simplify"

unregistered_phase_dir="$tmp_root/specs/943-transition-guard-selftest-unregistered-phase"
cp -R "$positive_feature_dir" "$unregistered_phase_dir"
mutate_unregistered_phase_claim "$unregistered_phase_dir/state.json"

clone_framework_surface "$broken_phase_owner_root"
mkdir -p "$broken_phase_owner_root/specs"
cp -R "$positive_feature_dir" "$broken_phase_owner_dir"
mutate_analyze_phase_provenance "$broken_phase_owner_dir/state.json" "bubbles.analyst"
inject_phase_owner "$broken_phase_owner_root/bubbles/workflows.yaml" "analyze" "bubbles.nonexistent-phase-owner"

clone_framework_surface "$malformed_capability_owner_root"
mkdir -p "$malformed_capability_owner_root/specs"
cp -R "$positive_feature_dir" "$malformed_capability_owner_dir"
mutate_phase_provenance "$malformed_capability_owner_dir/state.json" "bootstrap" "bubbles.plan"
inject_agent_owns_phases "$malformed_capability_owner_root/bubbles/agent-capabilities.yaml" "bubbles.plan" "bootstrap"

clone_framework_surface "$missing_capability_owner_root"
mkdir -p "$missing_capability_owner_root/specs"
cp -R "$positive_feature_dir" "$missing_capability_owner_dir"
mutate_phase_provenance "$missing_capability_owner_dir/state.json" "bootstrap" "bubbles.plan"
rename_capability_agent "$missing_capability_owner_root/bubbles/agent-capabilities.yaml" "bubbles.plan" "bubbles.nonexistent-capability-owner"

clone_framework_surface "$explicit_owner_conflict_root"
mkdir -p "$explicit_owner_conflict_root/specs"
cp -R "$positive_feature_dir" "$explicit_owner_conflict_dir"
mutate_phase_provenance "$explicit_owner_conflict_dir/state.json" "docs" "bubbles.simplify"
inject_agent_owns_phases "$explicit_owner_conflict_root/bubbles/agent-capabilities.yaml" "bubbles.simplify" "[ simplify, docs ]"

clone_framework_surface "$malformed_runner_grants_root"
mkdir -p "$malformed_runner_grants_root/specs"
cp -R "$positive_feature_dir" "$malformed_runner_grants_dir"
mutate_phase_provenance "$malformed_runner_grants_dir/state.json" "analyze" "bubbles.ux"
malform_workflow_runner_grants "$malformed_runner_grants_root/bubbles/agent-capabilities.yaml"

clone_framework_surface "$sequence_runner_grants_root"
mkdir -p "$sequence_runner_grants_root/specs"
cp -R "$positive_feature_dir" "$sequence_runner_grants_dir"
mutate_phase_provenance "$sequence_runner_grants_dir/state.json" "analyze" "bubbles.ux"
malform_workflow_runner_grants "$sequence_runner_grants_root/bubbles/agent-capabilities.yaml" "[ bubbles.workflow ]"

clone_framework_surface "$unknown_runner_grant_root"
mkdir -p "$unknown_runner_grant_root/specs"
cp -R "$positive_feature_dir" "$unknown_runner_grant_dir"
mutate_phase_provenance "$unknown_runner_grant_dir/state.json" "analyze" "bubbles.ux"
inject_unknown_workflow_runner "$unknown_runner_grant_root/bubbles/agent-capabilities.yaml" "bubbles.nonexistent-workflow-runner"

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

echo "Running guarded-repository root isolation selftest..."
repo_root_isolation_log="$tmp_root/repo-root-isolation-guard.log"
repo_root_isolation_status="$(
  cd "$repo_root_isolation_ambient_dir"
  run_capture "$repo_root_isolation_log" \
    run_guard_fast_disabled "$GUARD_SCRIPT" "$repo_root_isolation_feature_dir"
)"
if [[ "$repo_root_isolation_status" -eq 0 ]]; then
  pass "Guarded-repository fixture passes from a hostile ambient CWD"
else
  fail "Guarded-repository fixture was contaminated by ambient CWD"
  sed -n '1,220p' "$repo_root_isolation_log"
fi
assert_log_not_contains "$repo_root_isolation_log" \
  "$repo_root_isolation_ambient_dir/.github/bubbles-project.yaml" \
  "Guard ignores ambient project config outside the guarded repository"
assert_log_contains "$repo_root_isolation_log" \
  "Retro convergence health SLO is pass/degraded (Gate G090)" \
  "G090 evaluates convergence health against the guarded repository root"

if [[ "${BUBBLES_STATE_TRANSITION_GUARD_ROOT_ISOLATION_ONLY:-0}" == "1" ]]; then
  if [[ "$failures" -gt 0 ]]; then
    echo "state-transition-guard root-isolation selftest failed with $failures issue(s)."
    exit 1
  fi
  echo "state-transition-guard root-isolation selftest passed."
  exit 0
fi

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

# --- Check 13: a lint TIMEOUT must not be reported as a lint FAILURE ---
# Check 13 is fail-closed, so both outcomes block; the defect being guarded is
# the DIAGNOSIS, not the exit code. The cap was a flat 60s while a large spec's
# lint is load-sensitive (32s idle, 73-90s under concurrent load), so the same
# packet was reported as having lint failures on a busy machine and passing on
# an idle one. A reader told "Artifact lint FAILED" hunts for findings that do
# not exist.
#
# The timeout is forced by making the LINT slow, never by making the cap tight.
# An earlier form set BUBBLES_ARTIFACT_LINT_TIMEOUT=1 and bet that the real lint
# would lose that race; measured, this fixture's lint runs in ~0.6s, so on an
# idle machine it COMPLETED and Check 13 printed "Artifact lint passes (exit 0)".
# The case passed only on loaded hosts and failed on healthy ones -- it
# reproduced the exact timing defect it exists to catch. A sub-second cap does
# not repair that either: guard-lib's fallback watchdog (hosts with neither
# `timeout` nor `gtimeout`, e.g. a stock macOS PATH) gates its poll loop on
# `[ "$waited" -lt "$secs" ]`, an INTEGER test that errors on "0.1" and skips the
# loop entirely, so no SIGTERM is sent and 124 never comes back.
#
# So the guard is run from a staged framework clone whose artifact-lint.sh is a
# stub that sleeps 10x the cap. The timed command's duration is a guaranteed
# lower bound rather than an estimate of real lint speed, so the exit-124 path
# fires on every host, at any load, through all three bubbles_run_with_timeout
# paths (the cap stays an integer the fallback watchdog can compare).
c13_lint_cap_seconds=2
c13_stub_root="$tmp_root/framework-c13-lint-timeout"
clone_framework_surface "$c13_stub_root"
c13_stub_guard="$c13_stub_root/bubbles/scripts/state-transition-guard.sh"
c13_stub_lint="$c13_stub_root/bubbles/scripts/artifact-lint.sh"
c13_stub_feature_dir="$c13_stub_root/specs/944-check13-lint-timeout"
mkdir -p "$c13_stub_root/specs"
cp -R "$positive_feature_dir" "$c13_stub_feature_dir"

cat <<'EOF' > "$c13_stub_lint"
#!/usr/bin/env bash
# Selftest stub: a lint that CANNOT complete inside the cap, by construction.
sleep 20
EOF

c13_timeout_log="$tmp_root/check13-timeout.log"
run_capture "$c13_timeout_log" run_guard_with_repo_root_and_lint_timeout \
  "$c13_stub_root" "$c13_lint_cap_seconds" \
  "$c13_stub_guard" "$c13_stub_feature_dir" >/dev/null
assert_log_contains "$c13_timeout_log" \
  "this is a TIMEOUT, not a lint failure" \
  "Check 13 reports a lint that did not COMPLETE as a timeout, naming the cap"

# Adversarial twin: the timeout path must NOT borrow the failure wording, or the
# distinction is cosmetic and a reader still cannot tell the two apart.
assert_log_not_contains "$c13_timeout_log" \
  "Artifact lint FAILED (exit" \
  "Check 13 timeout path is distinct from the completed-and-rejected wording" # portable-ok: assertion prose, not a timeout invocation

# Second twin, controlled pair: same staged clone, same fixture, same cap -- the
# ONLY difference is that the stub now returns immediately. Isolating the lint's
# duration is what proves the case above fires on the cap being exceeded and not
# on anything about the staged clone. (Varying only the cap, as the earlier form
# did, no longer isolates the cause now that the lint itself is injected.)
cat <<'EOF' > "$c13_stub_lint"
#!/usr/bin/env bash
# Selftest stub: a lint that completes instantly, so Check 13 must NOT time out.
exit 0
EOF
c13_completes_log="$tmp_root/check13-completes.log"
run_capture "$c13_completes_log" run_guard_with_repo_root_and_lint_timeout \
  "$c13_stub_root" "$c13_lint_cap_seconds" \
  "$c13_stub_guard" "$c13_stub_feature_dir" >/dev/null
assert_log_not_contains "$c13_completes_log" \
  "this is a TIMEOUT, not a lint failure" \
  "Check 13 does not take the timeout path when the same staged lint completes (timeout case is non-tautological)" # portable-ok: assertion prose, not a timeout invocation

# Third twin: the REAL guard running the REAL lint at the DEFAULT cap must not
# report a timeout either, so the staged pair above cannot mask a regression that
# makes the shipped configuration time out spuriously.
c13_default_log="$tmp_root/check13-default.log"
run_capture "$c13_default_log" bash "$GUARD_SCRIPT" "$positive_feature_dir" >/dev/null
assert_log_not_contains "$c13_default_log" \
  "this is a TIMEOUT, not a lint failure" \
  "Check 13 does not take the timeout path with the real lint at the default cap" # portable-ok: assertion prose, not a timeout invocation

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

# Check 6 — a PARTIAL certifiedCompletedPhases must NOT short-circuit
# execution.completedPhaseClaims out of the phase source. Regression for the `or`
# selection that made Check 6 and Check 6B contradict each other on identical
# data. Same convention as the fixture above: assert on Check 6 / 6B log content
# only, since the fixture's overall exit may be non-zero for unrelated ceiling
# reasons.
echo "Running Check 6 partial-certification phase-source regression selftest..."
partial_certified_log="$tmp_root/partial-certified-phase-claims.log"
run_capture "$partial_certified_log" bash "$GUARD_SCRIPT" "$partial_certified_phase_claims_dir" >/dev/null
assert_log_contains "$partial_certified_log" "Required phase 'validate' recorded in execution/certification phase records" "Check 6: the certified phase is still counted after the merge (certification source not dropped)"
assert_log_contains "$partial_certified_log" "Required phase 'audit' recorded in execution/certification phase records" "Check 6: an execution claim IS evaluated even though certifiedCompletedPhases is non-empty (no short-circuit)"
assert_log_not_contains "$partial_certified_log" "Required phase 'audit' NOT in execution/certification phase records" "Check 6: a phase present only in completedPhaseClaims is NOT reported missing under partial certification (Gate G022 false positive)"
assert_log_not_contains "$partial_certified_log" "Traceback (most recent call last)" "Check 6: mixed string+dict phase sources do NOT crash the guard with a Python Traceback"
assert_log_not_contains "$partial_certified_log" "unhashable type: 'dict'" "Check 6: dict claim records normalize to phase names instead of raising the unhashable-dict TypeError"
# Check 6 / Check 6B agreement on 'audit'. The provenance line ALONE does not
# prove agreement: Check 6B reads completedPhaseClaims directly and is immune to
# the `or` short-circuit, so it passed even while Check 6 was blocking the same
# record. The paired negative is what makes the agreement claim real; it repeats
# the Check 6 needle asserted above, kept adjacent so both halves sit together.
assert_log_contains "$partial_certified_log" "Phase 'audit' has specialist provenance from bubbles.audit" "Check 6B: the 'audit' claim record carries specialist provenance from bubbles.audit"
assert_log_not_contains "$partial_certified_log" "Required phase 'audit' NOT in execution/certification phase records" "Check 6 emits no BLOCK for the same 'audit' record Check 6B accepted above (the two checks agree)"

# Check 6B — a phase whose owning agent is NOT named "bubbles.<phase>" must be
# resolved through the owner table. Asserting on Check 6B log content only, per
# the fixture convention above; the fixture's overall exit may be non-zero for
# unrelated ceiling reasons.
echo "Running Check 6B phase-owner resolution regression selftest..."
analyst_owned_log="$tmp_root/analyst-owned-phase.log"
run_capture "$analyst_owned_log" bash "$GUARD_SCRIPT" "$analyst_owned_phase_dir" >/dev/null
assert_log_contains "$analyst_owned_log" "Phase 'analyze' has specialist provenance from bubbles.analyst" "Check 6B: the 'analyze' phase resolves to its real owner bubbles.analyst"
assert_log_not_contains "$analyst_owned_log" "Phase 'analyze' is in completedPhaseClaims but no specialist or parent-expanded provenance found" "Check 6B: an honest bubbles.analyst record is NOT reported as missing provenance (Gate G022 false positive)"
assert_log_not_contains "$analyst_owned_log" "bubbles.analyze" "Check 6B: the guard never demands a 'bubbles.analyze' agent, which has never existed"

echo "Running Check 6B UX capability-owner regression selftest..."
ux_owned_log="$tmp_root/ux-owned-phase.log"
run_capture "$ux_owned_log" bash "$GUARD_SCRIPT" "$ux_owned_phase_dir" >/dev/null
assert_log_contains "$ux_owned_log" "Phase 'analyze' has specialist provenance from bubbles.ux" "Check 6B: the analyze phase accepts its capability-declared UX specialist"
assert_log_not_contains "$ux_owned_log" "Phase 'analyze' is in completedPhaseClaims but no specialist or parent-expanded provenance found" "Check 6B: UX-only analyze provenance is not hidden by analyst-first fixture ordering"

echo "Running Check 6B plan capability-owner regression selftest..."
plan_owned_log="$tmp_root/plan-owned-phase.log"
run_capture "$plan_owned_log" bash "$GUARD_SCRIPT" "$plan_owned_phase_dir" >/dev/null
assert_log_contains "$plan_owned_log" "Phase 'bootstrap' has specialist provenance from bubbles.plan" "Check 6B: the bootstrap phase accepts its capability-declared planning specialist"
assert_log_not_contains "$plan_owned_log" "Phase 'bootstrap' is in completedPhaseClaims but no specialist or parent-expanded provenance found" "Check 6B: plan-only bootstrap provenance is not hidden by design-first fixture ordering"

# Adversarial twin. The owner table must not become a blanket pass: an unrelated
# agent recording the same claim is impersonation and MUST still be refused.
echo "Running Check 6B phase-owner adversarial selftest..."
analyze_wrong_agent_log="$tmp_root/analyze-wrong-agent.log"
run_capture "$analyze_wrong_agent_log" bash "$GUARD_SCRIPT" "$analyze_wrong_agent_dir" >/dev/null
assert_log_contains "$analyze_wrong_agent_log" "Phase 'analyze' is in completedPhaseClaims but no specialist or parent-expanded provenance found" "Check 6B: an unrelated agent claiming 'analyze' is STILL refused (the impersonation check can fail)"
assert_log_not_contains "$analyze_wrong_agent_log" "Phase 'analyze' has specialist provenance from bubbles.analyst" "Check 6B: bubbles.simplify is not accepted as provenance for the analyze phase"

# Unknown phase names are framework-vocabulary errors, not instructions to
# invent an owner. The adversarial executor matches the old synthesized shape
# only by phase, so this test fails if the guard silently skips the claim or
# resumes demanding `bubbles.totally-made-up-phase`.
echo "Running Check 6B unregistered-phase adversarial selftest..."
unregistered_phase_log="$tmp_root/unregistered-phase.log"
run_capture "$unregistered_phase_log" bash "$GUARD_SCRIPT" "$unregistered_phase_dir" >/dev/null
assert_log_contains "$unregistered_phase_log" "Phase 'totally-made-up-phase' is not registered in phase registry" "Check 6B: an unregistered phase is refused as a registry-integrity error"
assert_log_contains "$unregistered_phase_log" "refusing without synthesizing a phantom owner" "Check 6B: the refusal explains that no owner was invented"
assert_log_not_contains "$unregistered_phase_log" "bubbles.totally-made-up-phase" "Check 6B: the guard never demands a fabricated agent identity"
assert_log_contains "$unregistered_phase_log" "framework integrity check failed" "Check 6B: an unknown phase cannot degrade to a pass"
assert_log_not_contains "$unregistered_phase_log" "phase provenance check skipped" "Check 6B: missing execution history cannot bypass phase registry validation"

echo "Running Check 6B broken declared-owner adversarial selftest..."
broken_phase_owner_log="$tmp_root/broken-phase-owner.log"
run_capture "$broken_phase_owner_log" bash "$GUARD_SCRIPT" "$broken_phase_owner_dir" >/dev/null
assert_log_contains "$broken_phase_owner_log" "Phase 'analyze' resolves to owner 'bubbles.nonexistent-phase-owner', but that owner has no shipped agent definition" "Check 6B: a registered phase with a nonexistent declared owner is refused as framework corruption"
assert_log_not_contains "$broken_phase_owner_log" "Phase 'analyze' has specialist provenance from bubbles.analyst" "Check 6B: legacy provenance cannot conceal a broken declared owner"
assert_log_contains "$broken_phase_owner_log" "framework integrity check failed" "Check 6B: a broken declared owner cannot degrade to a pass"

echo "Running Check 6B malformed capability-owner registry selftest..."
malformed_capability_owner_log="$tmp_root/malformed-capability-owner.log"
run_capture "$malformed_capability_owner_log" bash "$GUARD_SCRIPT" "$malformed_capability_owner_dir" >/dev/null
assert_log_contains "$malformed_capability_owner_log" "could not query capability owners for phase 'bootstrap'" "Check 6B: malformed capability ownership is refused instead of treated as an empty owner set"
assert_log_not_contains "$malformed_capability_owner_log" "Phase 'bootstrap' has specialist provenance from bubbles.plan" "Check 6B: a parser failure cannot silently admit plan provenance"
assert_log_contains "$malformed_capability_owner_log" "framework integrity check failed" "Check 6B: malformed capability ownership cannot degrade to a pass"

echo "Running Check 6B nonexistent capability-owner selftest..."
missing_capability_owner_log="$tmp_root/missing-capability-owner.log"
run_capture "$missing_capability_owner_log" bash "$GUARD_SCRIPT" "$missing_capability_owner_dir" >/dev/null
assert_log_contains "$missing_capability_owner_log" "Phase 'bootstrap' resolves to owner 'bubbles.nonexistent-capability-owner', but that owner has no shipped agent definition" "Check 6B: a capability-declared owner without an agent definition is refused"
assert_log_not_contains "$missing_capability_owner_log" "Phase 'bootstrap' has specialist provenance from bubbles.plan" "Check 6B: another valid capability owner cannot conceal a nonexistent co-owner"
assert_log_contains "$missing_capability_owner_log" "framework integrity check failed" "Check 6B: a nonexistent capability owner cannot degrade to a pass"

echo "Running Check 6B explicit-owner precedence selftest..."
explicit_owner_conflict_log="$tmp_root/explicit-owner-conflict.log"
run_capture "$explicit_owner_conflict_log" bash "$GUARD_SCRIPT" "$explicit_owner_conflict_dir" >/dev/null
assert_log_contains "$explicit_owner_conflict_log" "Phase 'docs' is in completedPhaseClaims but no specialist or parent-expanded provenance found" "Check 6B: capability metadata cannot widen an explicitly workflow-owned phase"
assert_log_not_contains "$explicit_owner_conflict_log" "Phase 'docs' has specialist provenance from bubbles.simplify" "Check 6B: explicit bubbles.docs ownership wins over a stale capability co-owner"

echo "Running Check 6B malformed workflow-runner grants selftest..."
malformed_runner_grants_log="$tmp_root/malformed-runner-grants.log"
run_capture "$malformed_runner_grants_log" bash "$GUARD_SCRIPT" "$malformed_runner_grants_dir" >/dev/null
assert_log_contains "$malformed_runner_grants_log" "must be a mapping, observed !!str" "Check 6B: scalar active-runner grants are refused instead of replaced by a hardcoded owner list"
assert_log_not_contains "$malformed_runner_grants_log" "Phase 'analyze' has specialist provenance from bubbles.ux" "Check 6B: malformed runner grants cannot silently admit capability provenance"
assert_log_contains "$malformed_runner_grants_log" "framework integrity check failed" "Check 6B: malformed workflow-runner grants cannot degrade to a pass"

echo "Running Check 6B sequence-shaped workflow-runner grants selftest..."
sequence_runner_grants_log="$tmp_root/sequence-runner-grants.log"
sequence_runner_grants_status="$(run_capture "$sequence_runner_grants_log" bash "$GUARD_SCRIPT" "$sequence_runner_grants_dir")"
if [[ "$sequence_runner_grants_status" -ne 0 ]]; then
  pass "Check 6B: sequence-shaped workflow-runner grants fail the transition guard"
else
  fail "Check 6B: sequence-shaped workflow-runner grants must not pass"
fi
assert_log_contains "$sequence_runner_grants_log" "must be a mapping, observed !!seq" "Check 6B: successful yq output with the wrong container shape is refused"
assert_log_not_contains "$sequence_runner_grants_log" "Phase 'analyze' has specialist provenance from bubbles.ux" "Check 6B: sequence-shaped grants cannot silently admit capability provenance"
assert_log_contains "$sequence_runner_grants_log" "framework integrity check failed" "Check 6B: sequence-shaped workflow-runner grants cannot degrade to a pass"

echo "Running Check 6B nonexistent workflow-runner identity selftest..."
unknown_runner_grant_log="$tmp_root/unknown-runner-grant.log"
unknown_runner_grant_status="$(run_capture "$unknown_runner_grant_log" bash "$GUARD_SCRIPT" "$unknown_runner_grant_dir")"
if [[ "$unknown_runner_grant_status" -ne 0 ]]; then
  pass "Check 6B: a workflow-runner grant without a shipped agent definition fails the transition guard"
else
  fail "Check 6B: a nonexistent workflow-runner identity must not pass"
fi
assert_log_contains "$unknown_runner_grant_log" "resolves to owner 'bubbles.nonexistent-workflow-runner', but that owner has no shipped agent definition" "Check 6B: every extracted workflow-runner identity must resolve to a shipped agent"
assert_log_not_contains "$unknown_runner_grant_log" "Phase 'analyze' has specialist provenance from bubbles.ux" "Check 6B: a valid capability owner cannot conceal a nonexistent workflow runner"
assert_log_contains "$unknown_runner_grant_log" "framework integrity check failed" "Check 6B: a nonexistent workflow runner cannot degrade to a pass"

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

run_focused_g057_assertions

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
s03_resolver_once_status="$(run_capture "$s03_resolver_once_log" run_guard_with_resolver_count \
  "$s03_resolver_count_file" \
  "$s03_resolver_once_root/bubbles/scripts/state-transition-guard.sh" "$s03_resolver_once_feature")"
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

echo "Running four-profile transition-contract resolution matrix..."
# transition-contract-resolver.sh supports four audit profiles. The guard's
# contract validator once accepted only two, so a packet on either other profile
# was rejected as MALFORMED (E009-AUDIT-PROFILE-CONTRADICTION) before any
# applicability branch could run — rapid-tool-delivery and framework-health could
# not clear the guard at all. These cases pin BOTH previously-unreachable
# profiles: each must RESOLVE, and only framework-proposal-v1 may actually skip
# the delivery-completion checks it declares non-applicable.
fast_lane_profile_log="$tmp_root/fast-lane-profile-resolve.log"
fast_lane_profile_status="$(run_capture "$fast_lane_profile_log" bash "$GUARD_SCRIPT" "$fast_lane_profile_dir")"
if [[ "$fast_lane_profile_status" -ne 2 ]]; then
  pass "Four-profile matrix: rapid-tool-delivery resolves its transition contract instead of blocking (exit $fast_lane_profile_status)"
else
  fail "Four-profile matrix: rapid-tool-delivery must not be blocked at contract resolution (observed $fast_lane_profile_status)"
  sed -n '1,260p' "$fast_lane_profile_log"
fi
assert_log_not_contains "$fast_lane_profile_log" "E009-AUDIT-PROFILE-CONTRADICTION" "Four-profile matrix: delivery-completion-fast-v1 is not rejected as a malformed contract"
assert_log_not_contains "$fast_lane_profile_log" "auditProfile: UNRESOLVED" "Four-profile matrix: rapid-tool-delivery reaches a resolved audit profile"
assert_log_contains "$fast_lane_profile_log" "auditProfile: delivery-completion-fast-v1" "Four-profile matrix: rapid-tool-delivery reports its declared fast-lane profile"
assert_log_contains "$fast_lane_profile_log" "applicableCheckClasses: [universal,mode-required,delivery-completion]" "Four-profile matrix: the fast lane asserts the full delivery-completion classes"
assert_log_contains "$fast_lane_profile_log" "notApplicableChecks: []" "Four-profile matrix: the fast lane declares no delivery exclusion"
assert_log_not_contains "$fast_lane_profile_log" "NOT_APPLICABLE: Check-4-completion" "Four-profile matrix: the fast lane skips no completion check"
assert_log_not_contains "$fast_lane_profile_log" "NOT_APPLICABLE: Check-11-execution-evidence" "Four-profile matrix: the fast lane skips no execution-evidence check"

framework_proposal_profile_log="$tmp_root/framework-proposal-profile-resolve.log"
framework_proposal_profile_status="$(run_capture "$framework_proposal_profile_log" bash "$GUARD_SCRIPT" "$framework_proposal_profile_dir")"
if [[ "$framework_proposal_profile_status" -ne 2 ]]; then
  pass "Four-profile matrix: framework-health resolves its transition contract instead of blocking (exit $framework_proposal_profile_status)"
else
  fail "Four-profile matrix: framework-health must not be blocked at contract resolution (observed $framework_proposal_profile_status)"
  sed -n '1,260p' "$framework_proposal_profile_log"
fi
assert_log_not_contains "$framework_proposal_profile_log" "E009-AUDIT-PROFILE-CONTRADICTION" "Four-profile matrix: framework-proposal-v1 is not rejected as a malformed contract"
assert_log_not_contains "$framework_proposal_profile_log" "auditProfile: UNRESOLVED" "Four-profile matrix: framework-health reaches a resolved audit profile"
assert_log_contains "$framework_proposal_profile_log" "auditProfile: framework-proposal-v1" "Four-profile matrix: framework-health reports its declared proposal profile"
assert_log_contains "$framework_proposal_profile_log" "notApplicableChecks: $s03_not_applicable" "Four-profile matrix: framework-health declares the four delivery-completion exclusions"
assert_log_contains "$framework_proposal_profile_log" "NOT_APPLICABLE: Check-4-completion" "Four-profile matrix: framework proposal skips the DoD completion check"
assert_log_contains "$framework_proposal_profile_log" "NOT_APPLICABLE: Check-5-all-done" "Four-profile matrix: framework proposal skips the all-scopes-Done check"
assert_log_contains "$framework_proposal_profile_log" "NOT_APPLICABLE: Check-8-file-existence" "Four-profile matrix: framework proposal skips the test-file existence check"
assert_log_contains "$framework_proposal_profile_log" "NOT_APPLICABLE: Check-11-execution-evidence" "Four-profile matrix: framework proposal skips the delivery execution-evidence check"
assert_log_not_contains "$framework_proposal_profile_log" "UNCHECKED DoD items" "Four-profile matrix: a skipped Check 4 emits no completion failure"
assert_log_not_contains "$framework_proposal_profile_log" "still marked 'Not Started'" "Four-profile matrix: a skipped Check 5 emits no all-Done failure"
assert_log_not_contains "$framework_proposal_profile_log" "Test Plan references non-existent file" "Four-profile matrix: a skipped Check 8 emits no missing-file failure"
assert_log_not_contains "$framework_proposal_profile_log" "has ZERO evidence code blocks" "Four-profile matrix: a skipped Check 11 emits no missing-evidence failure"

# Paired contrast against the SAME artifact content under delivery-completion-v1
# (the s03 delivery negative above is the identical packet on autonomous-goal):
# the exclusions are enforced for framework-proposal-v1 ONLY, so every one of
# these four checks must still adjudicate — and fail — under the live profile.
assert_log_contains "$s03_delivery_log" "UNCHECKED DoD items" "Four-profile matrix: delivery-completion-v1 still runs the DoD completion check"
assert_log_contains "$s03_delivery_log" "still marked 'Not Started'" "Four-profile matrix: delivery-completion-v1 still runs the all-scopes-Done check"
assert_log_contains "$s03_delivery_log" "Test Plan references non-existent file" "Four-profile matrix: delivery-completion-v1 still runs the test-file existence check"
assert_log_contains "$s03_delivery_log" "has ZERO evidence code blocks" "Four-profile matrix: delivery-completion-v1 still runs the execution-evidence check"

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
s03_g087_status="$(run_capture "$s03_g087_log" run_guard_with_repo_root_fast_disabled \
  "$s03_planning_gates_root" \
  "$s03_planning_gates_root/bubbles/scripts/state-transition-guard.sh" "$s03_g087_feature")"
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
s03_g091_status="$(run_capture "$s03_g091_log" run_guard_with_repo_root_fast_disabled \
  "$s03_planning_gates_root" \
  "$s03_planning_gates_root/bubbles/scripts/state-transition-guard.sh" "$s03_g091_feature")"
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

echo "Running compact completedScopes counting regression selftest..."
compact_completed_scopes_log="$tmp_root/compact-completed-scopes.log"
compact_completed_scopes_status="$(run_capture "$compact_completed_scopes_log" bash "$GUARD_SCRIPT" "$compact_completed_scopes_feature_dir")"
if [[ "$compact_completed_scopes_status" -eq 0 ]]; then
  pass "Compact three-entry completedScopes passes the transition guard"
else
  fail "Compact three-entry completedScopes should pass the transition guard (observed $compact_completed_scopes_status)"
fi
assert_log_contains "$compact_completed_scopes_log" "completedScopes count matches artifact Done scope count (3)" "Compact completedScopes counts every entry"

echo "Running completedScopes certification precedence selftest..."
completed_scopes_precedence_log="$tmp_root/completed-scopes-precedence.log"
completed_scopes_precedence_status="$(run_capture "$completed_scopes_precedence_log" bash "$GUARD_SCRIPT" "$completed_scopes_precedence_feature_dir")"
if [[ "$completed_scopes_precedence_status" -eq 0 ]]; then
  pass "Certification completedScopes takes precedence over the legacy top-level array"
else
  fail "Certification completedScopes precedence fixture should pass (observed $completed_scopes_precedence_status)"
fi
assert_log_contains "$completed_scopes_precedence_log" "completedScopes count matches artifact Done scope count (1)" "Certification completedScopes is authoritative"

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
g064_status="$(run_capture "$g064_log" bubbles_run_with_timeout "$g064_timeout_seconds" \
  run_guard_fast_disabled \
  "$g064_framework_root/bubbles/scripts/state-transition-guard.sh" "$g064_feature_dir")"
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

echo "Running G040 Check 18 — negative: prohibition and UI 'placeholder' nouns do NOT trigger..."
g040_neg_placeholder_noun_log="$tmp_root/g040-neg-placeholder-noun.log"
run_capture "$g040_neg_placeholder_noun_log" bash "$GUARD_SCRIPT" "$g040_neg_placeholder_noun_dir" >/dev/null
assert_log_not_contains "$g040_neg_placeholder_noun_log" "deferral language hit" "G040 Check 18 ignores 'no placeholder card', 'must not synthesise a placeholder', the UI record placeholder, and an adversarial 'replaced with a placeholder' probe description"

echo "Running G040 Check 18 — positive (adversarial twin): a real placeholder admission still BLOCKs..."
g040_pos_placeholder_admission_log="$tmp_root/g040-pos-placeholder-admission.log"
run_capture "$g040_pos_placeholder_admission_log" bash "$GUARD_SCRIPT" "$g040_pos_placeholder_admission_dir" >/dev/null
assert_log_contains "$g040_pos_placeholder_admission_log" "deferral language hit" "G040 Check 18 still BLOCKs on 'is a placeholder value until' — the narrowing did not disable the term"

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

echo "Running G040 Check 18 mandated exposure-label pair..."
g040_neg_exposure_label_log="$tmp_root/g040-neg-exposure-label.log"
run_capture "$g040_neg_exposure_label_log" bash "$GUARD_SCRIPT" "$g040_neg_exposure_label_dir" >/dev/null
assert_log_not_contains "$g040_neg_exposure_label_log" "deferral language hit" \
  "G040 Check 18 ignores the vertical-delivery-mandated Exposure-Deferred label when its reason is benign"

g040_pos_exposure_reason_log="$tmp_root/g040-pos-exposure-reason.log"
run_capture "$g040_pos_exposure_reason_log" bash "$GUARD_SCRIPT" "$g040_pos_exposure_reason_dir" >/dev/null
assert_log_contains "$g040_pos_exposure_reason_log" "deferral language hit" \
  "G040 Check 18 still blocks a deferring reason after the Exposure-Deferred label"
# BUG032-HARDEN9-G040-BOUNDARIES-012 / SCN-032-028: the prohibited
# phrases are complete lexical units, not prefixes. Benign `separate clause`
# and `future workflow` prose must pass while the exact `separate ticket` and
# `future scope` controls still block outside historical evidence.
g040_bug032_benign_clause_dir="$tmp_root/specs/958-g040-bug032-benign-separate-clause"
g040_bug032_benign_workflow_dir="$tmp_root/specs/959-g040-bug032-benign-future-workflow"
g040_bug032_block_ticket_dir="$tmp_root/specs/960-g040-bug032-block-separate-ticket"
g040_bug032_block_scope_dir="$tmp_root/specs/961-g040-bug032-block-future-scope"
emit_g040_fixture "$g040_bug032_benign_clause_dir" "done" \
  "A separate preservation clause stays negative." "no" "no"
emit_g040_fixture "$g040_bug032_benign_workflow_dir" "done" \
  "These are declared future workflow obligations." "no" "no"
emit_g040_fixture "$g040_bug032_block_ticket_dir" "done" \
  "Move this work to a separate ticket." "no" "no"
emit_g040_fixture "$g040_bug032_block_scope_dir" "done" \
  "Implement this in a future scope." "no" "no"

g040_bug032_benign_failures=0
for g040_bug032_case in clause workflow; do
  if [[ "$g040_bug032_case" == "clause" ]]; then
    g040_bug032_dir="$g040_bug032_benign_clause_dir"
  else
    g040_bug032_dir="$g040_bug032_benign_workflow_dir"
  fi
  g040_bug032_log="$tmp_root/g040-bug032-benign-$g040_bug032_case.log"
  run_capture "$g040_bug032_log" bash "$GUARD_SCRIPT" "$g040_bug032_dir" >/dev/null
  if grep -Fq -- 'deferral language hit' "$g040_bug032_log" \
    || ! grep -Fq -- 'Zero deferral language found in scope and report artifacts (Gate G040)' "$g040_bug032_log"; then
    g040_bug032_benign_failures=$((g040_bug032_benign_failures + 1))
    printf 'BUG032_SCN028_BENIGN_BOUNDARY_MISMATCH case=%s hits=%s\n' \
      "$g040_bug032_case" \
      "$(grep -cF -- 'deferral language hit' "$g040_bug032_log" || true)"
  fi
done
if [[ "$g040_bug032_benign_failures" -eq 0 ]]; then
  pass "BUG-032 G040 permits benign separate-clause and future-workflow wording"
else
  fail "BUG-032 G040 benign lexical-boundary matrix has $g040_bug032_benign_failures mismatch(es)"
fi

g040_bug032_block_failures=0
for g040_bug032_case in ticket scope; do
  if [[ "$g040_bug032_case" == "ticket" ]]; then
    g040_bug032_dir="$g040_bug032_block_ticket_dir"
  else
    g040_bug032_dir="$g040_bug032_block_scope_dir"
  fi
  g040_bug032_log="$tmp_root/g040-bug032-block-$g040_bug032_case.log"
  g040_bug032_status="$(run_capture "$g040_bug032_log" bash "$GUARD_SCRIPT" "$g040_bug032_dir")"
  if [[ "$g040_bug032_status" -eq 0 ]] \
    || ! grep -Fq -- 'deferral language hit' "$g040_bug032_log"; then
    g040_bug032_block_failures=$((g040_bug032_block_failures + 1))
    printf 'BUG032_SCN028_BLOCK_BOUNDARY_MISMATCH case=%s status=%s hits=%s\n' \
      "$g040_bug032_case" "$g040_bug032_status" \
      "$(grep -cF -- 'deferral language hit' "$g040_bug032_log" || true)"
  fi
done
if [[ "$g040_bug032_block_failures" -eq 0 ]]; then
  pass "BUG-032 G040 still blocks exact separate-ticket and future-scope phrases"
else
  fail "BUG-032 G040 prohibited lexical-boundary matrix has $g040_bug032_block_failures mismatch(es)"
fi

# ----------------------------------------------------------------------------
# G040 / Check 18 — certifying-window boundary (report.md marker parity with
# artifact-lint.sh Check 3). Four adversarial cases prove the boundary is a
# real audit-trail-preservation exemption and NOT a silent weakening:
#   (1) a forbidden phrase BEFORE a single marker is SKIPPED (prior-window);
#   (2) the SAME phrase AFTER the marker still BLOCKS (current-window strict);
#   (3) a report with NO marker still enforces in FULL (pre-marker phrase blocks);
#   (4) TWO markers fail loud AND grant no exemption (pre-marker phrase blocks).
# ----------------------------------------------------------------------------
echo "Running G040 Check 18 — certifying-window: pre-marker deferral is SKIPPED..."
g040_cw_pre_log="$tmp_root/g040-cw-pre-skipped.log"
run_capture "$g040_cw_pre_log" bash "$GUARD_SCRIPT" "$g040_cw_pre_skipped_dir" >/dev/null
assert_log_not_contains "$g040_cw_pre_log" "deferral language hit" "G040 Check 18: deferral prose BEFORE a single certifying-window marker is frozen prior-window history (not re-adjudicated)"
assert_log_contains "$g040_cw_pre_log" "before <!-- bubbles:certifying-window-begin --> (prior-window history)" "G040 Check 18: emits the prior-window skip info line (mirrors artifact-lint Check 3)"

echo "Running G040 Check 18 — certifying-window: post-marker deferral still BLOCKS..."
g040_cw_post_log="$tmp_root/g040-cw-post-blocks.log"
run_capture "$g040_cw_post_log" bash "$GUARD_SCRIPT" "$g040_cw_post_blocks_dir" >/dev/null
assert_log_contains "$g040_cw_post_log" "deferral language hit" "G040 Check 18: deferral prose AFTER the marker (current certifying window) still BLOCKS"

echo "Running G040 Check 18 — certifying-window: NO marker enforces in full..."
g040_cw_none_log="$tmp_root/g040-cw-no-marker.log"
run_capture "$g040_cw_none_log" bash "$GUARD_SCRIPT" "$g040_cw_no_marker_dir" >/dev/null
assert_log_contains "$g040_cw_none_log" "deferral language hit" "G040 Check 18: a marker-less report.md is enforced in FULL (the marker can never silently disable G040)"

echo "Running G040 Check 18 — certifying-window: TWO markers fail loud (no exemption)..."
g040_cw_two_log="$tmp_root/g040-cw-two-markers.log"
run_capture "$g040_cw_two_log" bash "$GUARD_SCRIPT" "$g040_cw_two_markers_dir" >/dev/null
assert_log_contains "$g040_cw_two_log" "Multiple <!-- bubbles:certifying-window-begin --> markers" "G040 Check 18: >1 certifying-window marker fails loud (ambiguous window start)"
assert_log_contains "$g040_cw_two_log" "deferral language hit" "G040 Check 18: ambiguous (>1 marker) report grants NO exemption — pre-marker deferral still BLOCKS (full enforcement)"

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

# BUG-032 D1 / SCN-032-013 through SCN-032-016: Check 8B must classify
# explicit consumer-interface mutations without treating an unrelated object
# mutation as a route mutation. Every assertion below sources the exact private
# production helper between stable markers. There is no copied grammar or
# legacy-regex fallback.
echo "Running BUG-032 Check 8B consumer-interface mutation classifier..."

check8b_classifier_file="$tmp_root/bug032-check8b-classifier.sh"
check8b_source_output_file="$tmp_root/bug032-check8b-source-output.txt"
check8b_production_epoch_file="$tmp_root/bug032-check8b-production-epoch.sh"
check8b_marker_begin_count=0
check8b_marker_end_count=0
check8b_marker_begin_line=0
check8b_marker_end_line=0
check8b_source_line_number=0
check8b_marker_state="before"
check8b_marker_stream_failures=0
check8b_source_read_passes=0
check8b_extracted_begin_count=0
check8b_extracted_end_count=0
check8b_extracted_line_count=0
check8b_extracted_body_line_count=0
check8b_extracted_function_count=0
check8b_extracted_entrypoint_count=0
check8b_extracted_first_line=""
check8b_extracted_last_line=""

check8b_parse_function_declaration() {
  local source_line="$1"
  local plain_pattern='^[[:space:]]*([_a-zA-Z][_a-zA-Z0-9]*)[[:space:]]*\([[:space:]]*\)[[:space:]]*\{(.*)$'
  local keyword_pattern='^[[:space:]]*function[[:space:]]+([_a-zA-Z][_a-zA-Z0-9]*)[[:space:]]*(\([[:space:]]*\))?[[:space:]]*\{(.*)$'

  CHECK8B_DECLARATION_NAME=""
  CHECK8B_DECLARATION_REMAINDER=""
  CHECK8B_DECLARATION_STYLE=""
  if [[ "$source_line" =~ $plain_pattern ]]; then
    CHECK8B_DECLARATION_NAME="${BASH_REMATCH[1]}"
    CHECK8B_DECLARATION_REMAINDER="${BASH_REMATCH[2]}"
    CHECK8B_DECLARATION_STYLE="name-parens"
  elif [[ "$source_line" =~ $keyword_pattern ]]; then
    CHECK8B_DECLARATION_NAME="${BASH_REMATCH[1]}"
    CHECK8B_DECLARATION_REMAINDER="${BASH_REMATCH[3]}"
    if [[ -n "${BASH_REMATCH[2]}" ]]; then
      CHECK8B_DECLARATION_STYLE="function-name-parens"
    else
      CHECK8B_DECLARATION_STYLE="function-name"
    fi
  else
    return 1
  fi
  [[ -n "$CHECK8B_DECLARATION_NAME" && -n "$CHECK8B_DECLARATION_STYLE" ]]
}

check8b_file_sha256() {
  local file_path="$1"
  local digest_record=""

  if command -v sha256sum >/dev/null 2>&1; then
    digest_record="$(sha256sum "$file_path")" || return 2
  elif command -v shasum >/dev/null 2>&1; then
    digest_record="$(shasum -a 256 "$file_path")" || return 2
  else
    return 2
  fi
  printf '%s\n' "${digest_record%% *}"
}

check8b_file_bytes() {
  local file_path="$1"
  local byte_count=""

  byte_count="$(wc -c < "$file_path")" || return 2
  byte_count="${byte_count//[[:space:]]/}"
  printf '%s\n' "$byte_count"
}

check8b_source_epoch_matches() {
  local source_path="$1"
  local expected_sha256="$2"
  local expected_bytes="$3"
  local observed_sha256=""
  local observed_bytes=""

  observed_sha256="$(check8b_file_sha256 "$source_path")" || return 2
  observed_bytes="$(check8b_file_bytes "$source_path")" || return 2
  [[ "$observed_sha256" == "$expected_sha256" ]] \
    && [[ "$observed_bytes" -eq "$expected_bytes" ]]
}

check8b_source_epoch_sha_before="$(check8b_file_sha256 "$PLANNING_CHECKS_SCRIPT")"
check8b_source_epoch_bytes_before="$(check8b_file_bytes "$PLANNING_CHECKS_SCRIPT")"
cp "$PLANNING_CHECKS_SCRIPT" "$check8b_production_epoch_file"
check8b_source_epoch_sha_after_snapshot="$(check8b_file_sha256 "$PLANNING_CHECKS_SCRIPT")"
check8b_source_epoch_bytes_after_snapshot="$(check8b_file_bytes "$PLANNING_CHECKS_SCRIPT")"
check8b_pinned_epoch_sha="$(check8b_file_sha256 "$check8b_production_epoch_file")"
check8b_pinned_epoch_bytes="$(check8b_file_bytes "$check8b_production_epoch_file")"
check8b_epoch_failures=0
if [[ "$check8b_source_epoch_sha_before" != "$check8b_source_epoch_sha_after_snapshot" ]] \
  || [[ "$check8b_source_epoch_bytes_before" -ne "$check8b_source_epoch_bytes_after_snapshot" ]] \
  || [[ "$check8b_source_epoch_sha_before" != "$check8b_pinned_epoch_sha" ]] \
  || [[ "$check8b_source_epoch_bytes_before" -ne "$check8b_pinned_epoch_bytes" ]]; then
  check8b_epoch_failures=$((check8b_epoch_failures + 1))
fi

: > "$check8b_classifier_file"
while IFS= read -r check8b_source_line || [[ -n "$check8b_source_line" ]]; do
  check8b_source_line_number=$((check8b_source_line_number + 1))
  case "$check8b_source_line" in
    '# BEGIN CHECK8B FINITE CLASSIFIER')
      check8b_marker_begin_count=$((check8b_marker_begin_count + 1))
      if [[ "$check8b_marker_state" == "before" ]] \
        && [[ "$check8b_marker_begin_count" -eq 1 ]]; then
        check8b_marker_begin_line="$check8b_source_line_number"
        check8b_marker_state="inside"
        printf '%s\n' "$check8b_source_line" >> "$check8b_classifier_file"
        check8b_extracted_begin_count=$((check8b_extracted_begin_count + 1))
        check8b_extracted_line_count=$((check8b_extracted_line_count + 1))
        check8b_extracted_first_line="$check8b_source_line"
        check8b_extracted_last_line="$check8b_source_line"
      else
        check8b_marker_stream_failures=$((check8b_marker_stream_failures + 1))
      fi
      ;;
    '# END CHECK8B FINITE CLASSIFIER')
      check8b_marker_end_count=$((check8b_marker_end_count + 1))
      if [[ "$check8b_marker_state" == "inside" ]] \
        && [[ "$check8b_marker_end_count" -eq 1 ]]; then
        check8b_marker_end_line="$check8b_source_line_number"
        printf '%s\n' "$check8b_source_line" >> "$check8b_classifier_file"
        check8b_extracted_end_count=$((check8b_extracted_end_count + 1))
        check8b_extracted_line_count=$((check8b_extracted_line_count + 1))
        check8b_extracted_last_line="$check8b_source_line"
        check8b_marker_state="after"
      else
        check8b_marker_stream_failures=$((check8b_marker_stream_failures + 1))
      fi
      ;;
    *)
      if [[ "$check8b_marker_state" == "inside" ]]; then
        printf '%s\n' "$check8b_source_line" >> "$check8b_classifier_file"
        check8b_extracted_line_count=$((check8b_extracted_line_count + 1))
        check8b_extracted_body_line_count=$((check8b_extracted_body_line_count + 1))
        check8b_extracted_last_line="$check8b_source_line"
        if check8b_parse_function_declaration "$check8b_source_line"; then
          check8b_extracted_function_count=$((check8b_extracted_function_count + 1))
          [[ "$CHECK8B_DECLARATION_NAME" == "check8b_classify_line" ]] \
            && check8b_extracted_entrypoint_count=$((check8b_extracted_entrypoint_count + 1))
        fi
      fi
      ;;
  esac
done < "$check8b_production_epoch_file"
check8b_source_read_passes=1
check8b_classifier_source_status=1
check8b_classifier_syntax_status=1
check8b_classifier_ready=0
check8b_sourceability_failures=0

if [[ "$check8b_source_read_passes" -ne 1 ]] \
  || [[ "$check8b_marker_stream_failures" -ne 0 ]] \
  || [[ "$check8b_marker_state" != "after" ]] \
  || [[ "$check8b_marker_begin_count" -ne 1 ]] \
  || [[ "$check8b_marker_end_count" -ne 1 ]] \
  || [[ "$check8b_marker_begin_line" -ge "$check8b_marker_end_line" ]] \
  || [[ "$check8b_extracted_line_count" -lt 10 ]] \
  || [[ "$check8b_extracted_body_line_count" -lt 8 ]] \
  || [[ "$check8b_extracted_function_count" -lt 2 ]] \
  || [[ "$check8b_extracted_entrypoint_count" -ne 1 ]] \
  || [[ "$check8b_extracted_begin_count" -ne 1 ]] \
  || [[ "$check8b_extracted_end_count" -ne 1 ]] \
  || [[ "$check8b_extracted_line_count" -ne $((check8b_marker_end_line - check8b_marker_begin_line + 1)) ]] \
  || [[ "$check8b_extracted_body_line_count" -ne $((check8b_marker_end_line - check8b_marker_begin_line - 1)) ]] \
  || [[ "$check8b_extracted_first_line" != '# BEGIN CHECK8B FINITE CLASSIFIER' ]] \
  || [[ "$check8b_extracted_last_line" != '# END CHECK8B FINITE CLASSIFIER' ]]; then
  check8b_sourceability_failures=$((check8b_sourceability_failures + 1))
fi

if bash -n "$check8b_classifier_file"; then
  check8b_classifier_syntax_status=0
else
  check8b_classifier_syntax_status=$?
  check8b_sourceability_failures=$((check8b_sourceability_failures + 1))
fi

CHECK8B_CLASSIFICATION="source-sentinel-classification"
CHECK8B_VERB="source-sentinel-verb"
CHECK8B_MUTATION_TARGET="source-sentinel-target"
CHECK8B_DIRECT_SURFACES="source-sentinel-direct"
CHECK8B_PRESERVED_SURFACES="source-sentinel-preserved"
CHECK8B_REASON="source-sentinel-reason"
CHECK8B_UNRESOLVED_PHRASE="source-sentinel-unresolved"
CHECK8B_BOUNDARY="source-sentinel-boundary"
CHECK8B_TOKEN_COUNT=37
CHECK8B_CANDIDATE_COUNT=5
_CHECK8B_TOKENS=(source-sentinel-token "source sentinel token two")
_CHECK8B_CLAUSE_IDS=(37 38)
_CHECK8B_CANDIDATE_INDEXES=(37 38)

check8b_trace_strip_inert_text() {
  local input_text="$1"
  local input_length="${#input_text}"
  local index=0
  local character=""
  local next_character=""
  local following_character=""
  local previous_character=""
  local lexical_state="plain"
  local parameter_depth=0

  CHECK8B_TRACE_EXECUTABLE_TEXT=""
  CHECK8B_TRACE_UNCLOSED_QUOTE=0
  while [[ "$index" -lt "$input_length" ]]; do
    character="${input_text:$index:1}"
    next_character=""
    following_character=""
    previous_character=""
    [[ $((index + 1)) -lt "$input_length" ]] \
      && next_character="${input_text:$((index + 1)):1}"
    [[ $((index + 2)) -lt "$input_length" ]] \
      && following_character="${input_text:$((index + 2)):1}"
    [[ "$index" -gt 0 ]] \
      && previous_character="${input_text:$((index - 1)):1}"

    case "$lexical_state" in
      plain)
        if [[ "$parameter_depth" -gt 0 ]]; then
          if [[ "$character" == '$' && "$next_character" == '{' ]]; then
            CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT$character$next_character"
            parameter_depth=$((parameter_depth + 1))
            index=$((index + 2))
            continue
          fi
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT$character"
          [[ "$character" != '}' ]] || parameter_depth=$((parameter_depth - 1))
        elif [[ "$character" == '$' && "$next_character" == '{' ]]; then
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT$character$next_character"
          parameter_depth=1
          index=$((index + 2))
          continue
        elif [[ "$character" == "<" && "$next_character" == "<" ]] \
          && [[ "$previous_character" != "<" ]] \
          && [[ "$following_character" != "<" ]]; then
          return 1
        elif [[ "$character" == "\\" ]]; then
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT "
          if [[ $((index + 1)) -lt "$input_length" ]]; then
            if [[ "$next_character" == $'\n' ]]; then
              CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT"$'\n'
            else
              CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT "
            fi
            index=$((index + 2))
            continue
          fi
        elif [[ "$character" == "'" ]]; then
          lexical_state="single-quote"
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT "
        elif [[ "$character" == '"' ]]; then
          lexical_state="double-quote"
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT "
        elif [[ "$character" == "#" ]] \
          && { [[ "$index" -eq 0 ]] \
            || [[ "$previous_character" =~ [[:space:]\;\&\|\(\)\{\}] ]]; }; then
          lexical_state="comment"
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT "
        else
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT$character"
        fi
        ;;
      single-quote)
        if [[ "$character" == "'" ]]; then
          lexical_state="plain"
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT "
        elif [[ "$character" == $'\n' ]]; then
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT"$'\n'
        else
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT "
        fi
        ;;
      double-quote)
        if [[ "$character" == "\\" ]]; then
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT "
          if [[ $((index + 1)) -lt "$input_length" ]]; then
            if [[ "$next_character" == $'\n' ]]; then
              CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT"$'\n'
            else
              CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT "
            fi
            index=$((index + 2))
            continue
          fi
        elif [[ "$character" == '"' ]]; then
          lexical_state="plain"
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT "
        elif [[ "$character" == $'\n' ]]; then
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT"$'\n'
        else
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT "
        fi
        ;;
      comment)
        if [[ "$character" == $'\n' ]]; then
          lexical_state="plain"
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT"$'\n'
        else
          CHECK8B_TRACE_EXECUTABLE_TEXT="$CHECK8B_TRACE_EXECUTABLE_TEXT "
        fi
        ;;
    esac
    index=$((index + 1))
  done

  if [[ "$parameter_depth" -ne 0 ]]; then
    return 1
  fi
  if [[ "$lexical_state" == "single-quote" ]] \
    || [[ "$lexical_state" == "double-quote" ]]; then
    # shellcheck disable=SC2034  # diagnostic trace flag for a future failure dump
    CHECK8B_TRACE_UNCLOSED_QUOTE=1
    return 1
  fi
  return 0
}

check8b_trace_has_executable_assignment() {
  local executable_text="$1"
  local variable_name="$2"
  local assignment_word_pattern='^[[:alpha:]_][[:alnum:]_]*(\[[^]]+\])?[+]?='
  local target_assignment_pattern="^${variable_name}(\\[[^]]+\\])?[+]?="
  local command_segment=""
  local command_word=""
  local command_index=0

  [[ "$variable_name" =~ ^[[:alpha:]_][[:alnum:]_]*$ ]] || return 1
  check8b_trace_split_command_list "$executable_text" || return 1
  for command_segment in "${CHECK8B_TRACE_COMMAND_SEGMENTS[@]}"; do
    check8b_trace_command_words "$command_segment" || continue
    command_index="$CHECK8B_TRACE_COMMAND_WORD_START"
    [[ "$command_index" -lt "${#CHECK8B_TRACE_COMMAND_WORDS[@]}" ]] || continue
    command_word="${CHECK8B_TRACE_COMMAND_WORDS[$command_index]}"
    case "$command_word" in
      local|declare|typeset|readonly|export)
        command_index=$((command_index + 1))
        while [[ "$command_index" -lt "${#CHECK8B_TRACE_COMMAND_WORDS[@]}" ]]; do
          command_word="${CHECK8B_TRACE_COMMAND_WORDS[$command_index]}"
          case "$command_word" in
            -p|-f|-F|+p|+f|+F) break ;;
            --|-[[:alpha:]]*|+[[:alpha:]]*) ;;
            *)
              [[ "$command_word" =~ $target_assignment_pattern ]] && return 0
              ;;
          esac
          command_index=$((command_index + 1))
        done
        ;;
      *)
        while [[ "$command_index" -lt "${#CHECK8B_TRACE_COMMAND_WORDS[@]}" ]]; do
          command_word="${CHECK8B_TRACE_COMMAND_WORDS[$command_index]}"
          [[ "$command_word" =~ $assignment_word_pattern ]] || break
          [[ "$command_word" =~ $target_assignment_pattern ]] && return 0
          command_index=$((command_index + 1))
        done
        ;;
    esac
  done
  return 1
}

check8b_trace_append_command_segment() {
  local command_segment="$1"

  command_segment="${command_segment#"${command_segment%%[![:space:]]*}"}"
  command_segment="${command_segment%"${command_segment##*[![:space:]]}"}"
  [[ -n "$command_segment" ]] || return 0
  CHECK8B_TRACE_COMMAND_SEGMENTS+=("$command_segment")
}

check8b_trace_split_command_list() {
  local executable_text="$1"
  local input_length="${#executable_text}"
  local index=0
  local character=""
  local next_character=""
  local previous_character=""
  local command_segment=""
  local parameter_depth=0
  local parenthesis_depth=0
  local conditional_depth=0

  CHECK8B_TRACE_COMMAND_SEGMENTS=()
  CHECK8B_TRACE_COMMAND_PARSE_UNCERTAIN=0
  while [[ "$index" -lt "$input_length" ]]; do
    character="${executable_text:$index:1}"
    next_character=""
    previous_character=""
    [[ $((index + 1)) -lt "$input_length" ]] \
      && next_character="${executable_text:$((index + 1)):1}"
    [[ "$index" -gt 0 ]] \
      && previous_character="${executable_text:$((index - 1)):1}"

    if [[ "$parameter_depth" -gt 0 ]]; then
      command_segment="$command_segment$character"
      if [[ "$character" == '$' && "$next_character" == '{' ]]; then
        command_segment="$command_segment$next_character"
        parameter_depth=$((parameter_depth + 1))
        index=$((index + 2))
        continue
      fi
      [[ "$character" != '}' ]] || parameter_depth=$((parameter_depth - 1))
      index=$((index + 1))
      continue
    fi

    if [[ "$conditional_depth" -gt 0 ]]; then
      if [[ "$character" == '$' && "$next_character" == '{' ]]; then
        command_segment="$command_segment$character$next_character"
        parameter_depth=1
        index=$((index + 2))
        continue
      fi
      command_segment="$command_segment$character"
      if [[ "$character" == ']' && "$next_character" == ']' ]]; then
        command_segment="$command_segment$next_character"
        conditional_depth=0
        index=$((index + 2))
        continue
      fi
      index=$((index + 1))
      continue
    fi

    if [[ "$parenthesis_depth" -gt 0 ]]; then
      if [[ "$character" == '$' && "$next_character" == '{' ]]; then
        command_segment="$command_segment$character$next_character"
        parameter_depth=1
        index=$((index + 2))
        continue
      fi
      command_segment="$command_segment$character"
      if [[ "$character" == '(' ]]; then
        parenthesis_depth=$((parenthesis_depth + 1))
      elif [[ "$character" == ')' ]]; then
        parenthesis_depth=$((parenthesis_depth - 1))
      fi
      index=$((index + 1))
      continue
    fi

    if [[ "$character" == '$' && "$next_character" == '{' ]]; then
      command_segment="$command_segment$character$next_character"
      parameter_depth=1
      index=$((index + 2))
      continue
    fi
    if [[ "$character" == '[' && "$next_character" == '[' ]] \
      && { [[ "$index" -eq 0 ]] \
        || [[ "$previous_character" =~ [[:space:]\;\&\|\(\)\{\}] ]]; }; then
      command_segment="$command_segment$character$next_character"
      conditional_depth=1
      index=$((index + 2))
      continue
    fi
    if [[ "$character" == '$' && "$next_character" == '(' ]]; then
      command_segment="$command_segment$character$next_character"
      parenthesis_depth=1
      index=$((index + 2))
      continue
    fi
    if [[ "$character" == '(' ]]; then
      command_segment="$command_segment$character"
      parenthesis_depth=1
      index=$((index + 1))
      continue
    fi
    if [[ "$character" == ')' ]] \
      || [[ "$character" == ';' ]] \
      || [[ "$character" == '{' ]] \
      || [[ "$character" == '}' ]] \
      || [[ "$character" == $'\n' ]]; then
      check8b_trace_append_command_segment "$command_segment"
      command_segment=""
      index=$((index + 1))
      continue
    fi
    if { [[ "$character" == '&' && "$next_character" == '&' ]] \
      || [[ "$character" == '|' && "$next_character" == '|' ]]; }; then
      check8b_trace_append_command_segment "$command_segment"
      command_segment=""
      index=$((index + 2))
      continue
    fi
    command_segment="$command_segment$character"
    index=$((index + 1))
  done

  if [[ "$parameter_depth" -ne 0 ]] \
    || [[ "$parenthesis_depth" -ne 0 ]] \
    || [[ "$conditional_depth" -ne 0 ]]; then
    # shellcheck disable=SC2034  # diagnostic trace flag for a future failure dump
    CHECK8B_TRACE_COMMAND_PARSE_UNCERTAIN=1
    CHECK8B_TRACE_COMMAND_SEGMENTS=()
    return 1
  fi
  check8b_trace_append_command_segment "$command_segment"
  return 0
}

check8b_trace_command_words() {
  local command_segment="$1"
  local command_word=""

  CHECK8B_TRACE_COMMAND_WORDS=()
  CHECK8B_TRACE_COMMAND_WORD_START=0
  IFS=$' \t\r\n' read -r -a CHECK8B_TRACE_COMMAND_WORDS <<< "$command_segment"
  while [[ "$CHECK8B_TRACE_COMMAND_WORD_START" -lt "${#CHECK8B_TRACE_COMMAND_WORDS[@]}" ]]; do
    command_word="${CHECK8B_TRACE_COMMAND_WORDS[$CHECK8B_TRACE_COMMAND_WORD_START]}"
    case "$command_word" in
      if|elif|then|else|do|while|until|'!')
        CHECK8B_TRACE_COMMAND_WORD_START=$((CHECK8B_TRACE_COMMAND_WORD_START + 1))
        ;;
      *) break ;;
    esac
  done
  [[ "$CHECK8B_TRACE_COMMAND_WORD_START" -lt "${#CHECK8B_TRACE_COMMAND_WORDS[@]}" ]]
}

check8b_trace_has_executable_call() {
  local executable_text="$1"
  local command_name="$2"
  local assignment_word_pattern='^[[:alpha:]_][[:alnum:]_]*(\[[^]]+\])?[+]?='
  local command_segment=""
  local command_word=""
  local command_index=0

  [[ "$command_name" =~ ^[[:alpha:]_][[:alnum:]_]*$ ]] || return 1
  check8b_trace_split_command_list "$executable_text" || return 1
  for command_segment in "${CHECK8B_TRACE_COMMAND_SEGMENTS[@]}"; do
    check8b_trace_command_words "$command_segment" || continue
    command_index="$CHECK8B_TRACE_COMMAND_WORD_START"
    while [[ "$command_index" -lt "${#CHECK8B_TRACE_COMMAND_WORDS[@]}" ]]; do
      command_word="${CHECK8B_TRACE_COMMAND_WORDS[$command_index]}"
      [[ "$command_word" =~ $assignment_word_pattern ]] || break
      command_index=$((command_index + 1))
    done
    [[ "$command_index" -lt "${#CHECK8B_TRACE_COMMAND_WORDS[@]}" ]] || continue
    [[ "${CHECK8B_TRACE_COMMAND_WORDS[$command_index]}" == "$command_name" ]] && return 0
  done
  return 1
}

check8b_trace_has_executable_relationship_call() {
  local executable_text="$1"
  local relationship_name=""

  for relationship_name in \
    _check8b_is_surface _check8b_is_surface_head_at _check8b_surface_from \
    _check8b_passive_surface _check8b_named_surface_before _check8b_target_after; do
    check8b_trace_has_executable_call "$executable_text" "$relationship_name" && return 0
  done
  return 1
}

check8b_function_brace_scan_line() {
  local source_line="$1"
  local initial_depth="$2"
  local depth="$initial_depth"
  local declaration_remainder=""
  local closing_remainder=""
  local trailing_text=""

  CHECK8B_FUNCTION_BRACE_SCAN_FAILURES=0
  CHECK8B_FUNCTION_BRACE_CLOSED=0
  CHECK8B_FUNCTION_BRACE_TRAILING_CODE=0
  CHECK8B_FUNCTION_BRACE_FINAL_DEPTH="$initial_depth"

  if [[ "$initial_depth" -eq 0 ]]; then
    check8b_parse_function_declaration "$source_line" || {
      CHECK8B_FUNCTION_BRACE_SCAN_FAILURES=$((CHECK8B_FUNCTION_BRACE_SCAN_FAILURES + 1))
      return 1
    }
    depth=1
    declaration_remainder="$CHECK8B_DECLARATION_REMAINDER"
    declaration_remainder="${declaration_remainder#"${declaration_remainder%%[![:space:]]*}"}"
    declaration_remainder="${declaration_remainder%"${declaration_remainder##*[![:space:]]}"}"
    if [[ "$declaration_remainder" =~ (^|[[:space:]\;])\}([[:space:]]*\;)?[[:space:]]*$ ]]; then
      CHECK8B_FUNCTION_BRACE_CLOSED=1
      depth=0
    fi
    CHECK8B_FUNCTION_BRACE_FINAL_DEPTH="$depth"
    return 0
  fi

  # Production declarations and calibration fixtures keep their top-level
  # closing brace unindented. Nested groups remain indented and therefore stay
  # body syntax. Bash -n and the independent structural oracle validate that
  # body; this declaration-only detector owns only the source-time boundary.
  if [[ "$source_line" == '}' ]] || [[ "$source_line" == '};' ]]; then
    CHECK8B_FUNCTION_BRACE_CLOSED=1
    CHECK8B_FUNCTION_BRACE_FINAL_DEPTH=0
    return 0
  fi
  if [[ "$source_line" == '}'* ]]; then
    closing_remainder="${source_line#\}}"
    trailing_text="${closing_remainder#"${closing_remainder%%[![:space:]]*}"}"
    trailing_text="${trailing_text%"${trailing_text##*[![:space:]]}"}"
    if [[ "$trailing_text" != ';' ]]; then
      # shellcheck disable=SC2034  # diagnostic trace flag for a future failure dump
      CHECK8B_FUNCTION_BRACE_TRAILING_CODE=1
      CHECK8B_FUNCTION_BRACE_SCAN_FAILURES=$((CHECK8B_FUNCTION_BRACE_SCAN_FAILURES + 1))
      return 1
    fi
    # shellcheck disable=SC2034  # diagnostic trace flag for a future failure dump
    CHECK8B_FUNCTION_BRACE_CLOSED=1
    CHECK8B_FUNCTION_BRACE_FINAL_DEPTH=0
    return 0
  fi
  CHECK8B_FUNCTION_BRACE_FINAL_DEPTH="$depth"
  return 0
}

check8b_source_declaration_only_contract() {
  local source_file="$1"
  local source_line=""
  local source_tail=""
  local marker_state="before"
  local active_function=""
  local function_brace_depth=0

  CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES=0
  CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE=0
  CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON="none"
  CHECK8B_SOURCE_DECLARATION_FUNCTION_COUNT=0
  CHECK8B_SOURCE_DECLARATION_BEGIN_COUNT=0
  CHECK8B_SOURCE_DECLARATION_END_COUNT=0
  CHECK8B_SOURCE_DECLARATION_STYLE_NAME_PARENS=0
  CHECK8B_SOURCE_DECLARATION_STYLE_FUNCTION_NAME=0
  CHECK8B_SOURCE_DECLARATION_STYLE_FUNCTION_NAME_PARENS=0
  CHECK8B_SOURCE_DECLARATION_LINE_NUMBER=0

  while IFS= read -r source_line || [[ -n "$source_line" ]]; do
    CHECK8B_SOURCE_DECLARATION_LINE_NUMBER=$((CHECK8B_SOURCE_DECLARATION_LINE_NUMBER + 1))
    if [[ -n "$active_function" ]]; then
      if ! check8b_function_brace_scan_line "$source_line" "$function_brace_depth"; then
        CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES=$((CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES + 1))
        if [[ "$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE" -eq 0 ]]; then
          CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE="$CHECK8B_SOURCE_DECLARATION_LINE_NUMBER"
          CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON="malformed-function-body:$active_function"
        fi
        active_function=""
        function_brace_depth=0
        continue
      fi
      function_brace_depth="$CHECK8B_FUNCTION_BRACE_FINAL_DEPTH"
      [[ "$function_brace_depth" -ne 0 ]] || active_function=""
      continue
    fi

    source_tail="${source_line#"${source_line%%[![:space:]]*}"}"
    source_tail="${source_tail%"${source_tail##*[![:space:]]}"}"
    [[ -n "$source_tail" ]] || continue
    if [[ "$source_tail" == '# BEGIN CHECK8B FINITE CLASSIFIER' ]]; then
      CHECK8B_SOURCE_DECLARATION_BEGIN_COUNT=$((CHECK8B_SOURCE_DECLARATION_BEGIN_COUNT + 1))
      if [[ "$marker_state" == "before" ]] \
        && [[ "$CHECK8B_SOURCE_DECLARATION_BEGIN_COUNT" -eq 1 ]]; then
        marker_state="inside"
      else
        CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES=$((CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES + 1))
        if [[ "$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE" -eq 0 ]]; then
          CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE="$CHECK8B_SOURCE_DECLARATION_LINE_NUMBER"
          CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON="misordered-begin-marker"
        fi
      fi
      continue
    fi
    if [[ "$source_tail" == '# END CHECK8B FINITE CLASSIFIER' ]]; then
      CHECK8B_SOURCE_DECLARATION_END_COUNT=$((CHECK8B_SOURCE_DECLARATION_END_COUNT + 1))
      if [[ "$marker_state" == "inside" ]] \
        && [[ "$CHECK8B_SOURCE_DECLARATION_END_COUNT" -eq 1 ]]; then
        marker_state="after"
      else
        CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES=$((CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES + 1))
        if [[ "$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE" -eq 0 ]]; then
          CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE="$CHECK8B_SOURCE_DECLARATION_LINE_NUMBER"
          CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON="misordered-end-marker"
        fi
      fi
      continue
    fi
    if [[ "$source_tail" == \#* ]]; then
      if [[ "$marker_state" != "inside" ]]; then
        CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES=$((CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES + 1))
        if [[ "$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE" -eq 0 ]]; then
          CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE="$CHECK8B_SOURCE_DECLARATION_LINE_NUMBER"
          CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON="comment-outside-marker-block"
        fi
      fi
      continue
    fi
    if [[ "$marker_state" == "inside" ]] \
      && check8b_parse_function_declaration "$source_line"; then
      CHECK8B_SOURCE_DECLARATION_FUNCTION_COUNT=$((CHECK8B_SOURCE_DECLARATION_FUNCTION_COUNT + 1))
      case "$CHECK8B_DECLARATION_STYLE" in
        name-parens) CHECK8B_SOURCE_DECLARATION_STYLE_NAME_PARENS=$((CHECK8B_SOURCE_DECLARATION_STYLE_NAME_PARENS + 1)) ;;
        function-name) CHECK8B_SOURCE_DECLARATION_STYLE_FUNCTION_NAME=$((CHECK8B_SOURCE_DECLARATION_STYLE_FUNCTION_NAME + 1)) ;;
        function-name-parens) CHECK8B_SOURCE_DECLARATION_STYLE_FUNCTION_NAME_PARENS=$((CHECK8B_SOURCE_DECLARATION_STYLE_FUNCTION_NAME_PARENS + 1)) ;;
        *)
          CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES=$((CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES + 1))
          if [[ "$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE" -eq 0 ]]; then
            CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE="$CHECK8B_SOURCE_DECLARATION_LINE_NUMBER"
            CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON="unsupported-function-declaration"
          fi
          ;;
      esac
      if ! check8b_function_brace_scan_line "$source_line" 0; then
        CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES=$((CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES + 1))
        if [[ "$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE" -eq 0 ]]; then
          CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE="$CHECK8B_SOURCE_DECLARATION_LINE_NUMBER"
          CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON="malformed-function-declaration:$CHECK8B_DECLARATION_NAME"
        fi
        continue
      fi
      function_brace_depth="$CHECK8B_FUNCTION_BRACE_FINAL_DEPTH"
      if [[ "$function_brace_depth" -gt 0 ]]; then
        active_function="$CHECK8B_DECLARATION_NAME"
      fi
      continue
    fi

    CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES=$((CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES + 1))
    if [[ "$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE" -eq 0 ]]; then
      CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE="$CHECK8B_SOURCE_DECLARATION_LINE_NUMBER"
      if [[ "$marker_state" == "inside" ]]; then
        CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON="top-level-non-function"
      else
        CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON="content-outside-marker-block"
      fi
    fi
  done < "$source_file"

  if [[ -n "$active_function" ]] || [[ "$function_brace_depth" -ne 0 ]]; then
    CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES=$((CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES + 1))
    if [[ "$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE" -eq 0 ]]; then
      CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE="$CHECK8B_SOURCE_DECLARATION_LINE_NUMBER"
      CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON="unclosed-function:$active_function"
    fi
  fi
  if [[ "$marker_state" != "after" ]] \
    || [[ "$CHECK8B_SOURCE_DECLARATION_BEGIN_COUNT" -ne 1 ]] \
    || [[ "$CHECK8B_SOURCE_DECLARATION_END_COUNT" -ne 1 ]]; then
    CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES=$((CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES + 1))
    if [[ "$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE" -eq 0 ]]; then
      CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE="$CHECK8B_SOURCE_DECLARATION_LINE_NUMBER"
      CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON="marker-contract"
    fi
  fi
  [[ "$CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES" -eq 0 ]]
}

check8b_sourceability_nested_fixture="$tmp_root/bug032-check8b-sourceability-nested-fixture.sh"
check8b_sourceability_malformed_fixture="$tmp_root/bug032-check8b-sourceability-malformed-fixture.sh"
cat <<'EOF' > "$check8b_sourceability_nested_fixture"
# BEGIN CHECK8B FINITE CLASSIFIER
check8b_nested_sourceability_fixture() {
  local -a values=(one two)
  local index=0
  if [[ "${#values[@]}" -eq 2 ]]; then
    case "${values[$index]}" in
      one)
        for index in "${!values[@]}"; do
          (( index >= 0 )) || return 1
        done
        ;;
      *) return 1 ;;
    esac
  fi
  return 0
}
# END CHECK8B FINITE CLASSIFIER
EOF
cat <<'EOF' > "$check8b_sourceability_malformed_fixture"
# BEGIN CHECK8B FINITE CLASSIFIER
check8b_malformed_sourceability_fixture() {
  local value=one
} printf '%s\n' forbidden-trailing-code
# END CHECK8B FINITE CLASSIFIER
EOF
check8b_sourceability_calibration_failures=0
if check8b_source_declaration_only_contract "$check8b_sourceability_nested_fixture" \
  && [[ "$CHECK8B_SOURCE_DECLARATION_FUNCTION_COUNT" -eq 1 ]] \
  && [[ "$CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES" -eq 0 ]]; then
  pass "BUG-032 TI-07 declaration-only detector accepts nested if, case, loop, array, conditional, and arithmetic syntax inside one valid function body"
else
  check8b_sourceability_calibration_failures=$((check8b_sourceability_calibration_failures + 1))
  echo "BUG032_TI07_VALID_NESTED_BODY_REJECTED failures=$CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES line=$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE reason=$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON functions=$CHECK8B_SOURCE_DECLARATION_FUNCTION_COUNT"
fi
if check8b_source_declaration_only_contract "$check8b_sourceability_malformed_fixture"; then
  check8b_sourceability_calibration_failures=$((check8b_sourceability_calibration_failures + 1))
  echo 'BUG032_TI07_MALFORMED_TRAILING_BODY_ACCEPTED'
elif [[ "$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON" == "malformed-function-body:check8b_malformed_sourceability_fixture" ]]; then
  pass "BUG-032 TI-07 declaration-only detector still rejects executable text after the closing function brace"
else
  check8b_sourceability_calibration_failures=$((check8b_sourceability_calibration_failures + 1))
  echo "BUG032_TI07_MALFORMED_BODY_WRONG_REASON failures=$CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES line=$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE reason=$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON"
fi
if [[ "$check8b_sourceability_calibration_failures" -eq 0 ]]; then
  pass "BUG-032 TI-07 sourceability brace scanner calibration preserves valid nested grammar and malformed-body rejection"
else
  fail "BUG-032 TI-07 sourceability brace scanner calibration has $check8b_sourceability_calibration_failures failure(s)"
fi

declare -A check8b_source_assignment_names=()

check8b_source_inventory_note_failure() {
  local failure_reason="$1"

  CHECK8B_SOURCE_ASSIGNMENT_INVENTORY_FAILURES=$((CHECK8B_SOURCE_ASSIGNMENT_INVENTORY_FAILURES + 1))
  if [[ "$CHECK8B_SOURCE_ASSIGNMENT_INVENTORY_FIRST_REASON" == "none" ]]; then
    CHECK8B_SOURCE_ASSIGNMENT_INVENTORY_FIRST_REASON="$failure_reason"
  fi
}

check8b_source_inventory_parenthesis_delta() {
  local input_text="$1"
  local input_index=0
  local input_character=""

  CHECK8B_SOURCE_INVENTORY_PAREN_DELTA=0
  while [[ "$input_index" -lt "${#input_text}" ]]; do
    input_character="${input_text:$input_index:1}"
    if [[ "$input_character" == "(" ]]; then
      CHECK8B_SOURCE_INVENTORY_PAREN_DELTA=$((CHECK8B_SOURCE_INVENTORY_PAREN_DELTA + 1))
    elif [[ "$input_character" == ")" ]]; then
      CHECK8B_SOURCE_INVENTORY_PAREN_DELTA=$((CHECK8B_SOURCE_INVENTORY_PAREN_DELTA - 1))
    fi
    input_index=$((input_index + 1))
  done
}

check8b_source_inventory_add_operand() {
  local operand="$1"
  local allow_bare_name="$2"
  local assignment_pattern='^([[:alpha:]_][[:alnum:]_]*)(\[.+\])?(\+)?=(.*)$'
  local bare_name_pattern='^([[:alpha:]_][[:alnum:]_]*)(\[.+\])?$'
  local inventory_name=""

  CHECK8B_SOURCE_INVENTORY_OPERAND_IS_ASSIGNMENT=0
  CHECK8B_SOURCE_INVENTORY_OPERAND_VALUE=""
  if [[ "$operand" =~ $assignment_pattern ]]; then
    inventory_name="${BASH_REMATCH[1]}"
    CHECK8B_SOURCE_INVENTORY_OPERAND_IS_ASSIGNMENT=1
    CHECK8B_SOURCE_INVENTORY_OPERAND_VALUE="${BASH_REMATCH[4]}"
  elif [[ "$allow_bare_name" -eq 1 ]] && [[ "$operand" =~ $bare_name_pattern ]]; then
    inventory_name="${BASH_REMATCH[1]}"
  else
    return 1
  fi
  check8b_source_assignment_names["$inventory_name"]=1
  return 0
}

check8b_source_inventory_parse_segment() {
  local command_segment="$1"
  local command_index=0
  local command_word=""
  local declaration_mode=0
  local declaration_options=1
  local parenthesis_depth=0

  check8b_trace_command_words "$command_segment" || return 0
  command_index="$CHECK8B_TRACE_COMMAND_WORD_START"
  command_word="${CHECK8B_TRACE_COMMAND_WORDS[$command_index]}"
  case "$command_word" in
    local|declare|typeset|readonly|export)
      declaration_mode=1
      command_index=$((command_index + 1))
      ;;
    case|for|select|return|break|continue|shift|unset|printf|read|true|false|:|'[['|'((')
      return 0
      ;;
  esac

  while [[ "$command_index" -lt "${#CHECK8B_TRACE_COMMAND_WORDS[@]}" ]]; do
    command_word="${CHECK8B_TRACE_COMMAND_WORDS[$command_index]}"
    if [[ "$parenthesis_depth" -gt 0 ]]; then
      check8b_source_inventory_parenthesis_delta "$command_word"
      parenthesis_depth=$((parenthesis_depth + CHECK8B_SOURCE_INVENTORY_PAREN_DELTA))
      if [[ "$parenthesis_depth" -lt 0 ]]; then
        check8b_source_inventory_note_failure "unbalanced-assignment-value:$command_word"
        return 1
      fi
      command_index=$((command_index + 1))
      continue
    fi

    if [[ "$declaration_mode" -eq 1 ]] && [[ "$declaration_options" -eq 1 ]]; then
      case "$command_word" in
        --)
          declaration_options=0
          command_index=$((command_index + 1))
          continue
          ;;
        -p|-f|-F|+p|+f|+F)
          return 0
          ;;
        -[[:alpha:]]*|+[[:alpha:]]*)
          command_index=$((command_index + 1))
          continue
          ;;
      esac
      declaration_options=0
    fi

    if check8b_source_inventory_add_operand "$command_word" "$declaration_mode"; then
      if [[ "$CHECK8B_SOURCE_INVENTORY_OPERAND_IS_ASSIGNMENT" -eq 1 ]]; then
        check8b_source_inventory_parenthesis_delta "$CHECK8B_SOURCE_INVENTORY_OPERAND_VALUE"
        parenthesis_depth="$CHECK8B_SOURCE_INVENTORY_PAREN_DELTA"
        if [[ "$parenthesis_depth" -lt 0 ]]; then
          check8b_source_inventory_note_failure "unbalanced-assignment-value:$command_word"
          return 1
        fi
      fi
      command_index=$((command_index + 1))
      continue
    fi

    if [[ "$declaration_mode" -eq 1 ]]; then
      check8b_source_inventory_note_failure "unparseable-declaration-operand:$command_word"
      return 1
    fi
    if [[ "$command_word" == *"="* ]]; then
      check8b_source_inventory_note_failure "unparseable-bare-assignment:$command_word"
      return 1
    fi
    break
  done

  if [[ "$parenthesis_depth" -ne 0 ]]; then
    check8b_source_inventory_note_failure "unclosed-assignment-value"
    return 1
  fi
  return 0
}

check8b_source_assignment_inventory_build() {
  local source_file="$1"
  local source_line=""
  local source_tail=""
  local marker_state="before"
  local marker_begin_count=0
  local marker_end_count=0
  local bounded_source=""
  local executable_source=""
  local command_segment=""

  check8b_source_assignment_names=()
  CHECK8B_SOURCE_ASSIGNMENT_INVENTORY_FAILURES=0
  CHECK8B_SOURCE_ASSIGNMENT_INVENTORY_FIRST_REASON="none"
  while IFS= read -r source_line || [[ -n "$source_line" ]]; do
    source_tail="${source_line#"${source_line%%[![:space:]]*}"}"
    source_tail="${source_tail%"${source_tail##*[![:space:]]}"}"
    if [[ "$source_tail" == '# BEGIN CHECK8B FINITE CLASSIFIER' ]]; then
      marker_begin_count=$((marker_begin_count + 1))
      if [[ "$marker_state" == "before" ]] && [[ "$marker_begin_count" -eq 1 ]]; then
        marker_state="inside"
      else
        check8b_source_inventory_note_failure "misordered-begin-marker"
      fi
      continue
    fi
    if [[ "$source_tail" == '# END CHECK8B FINITE CLASSIFIER' ]]; then
      marker_end_count=$((marker_end_count + 1))
      if [[ "$marker_state" == "inside" ]] && [[ "$marker_end_count" -eq 1 ]]; then
        marker_state="after"
      else
        check8b_source_inventory_note_failure "misordered-end-marker"
      fi
      continue
    fi
    if [[ "$marker_state" == "inside" ]]; then
      bounded_source="${bounded_source}${bounded_source:+$'\n'}$source_line"
    fi
  done < "$source_file"

  if [[ "$marker_state" != "after" ]] \
    || [[ "$marker_begin_count" -ne 1 ]] \
    || [[ "$marker_end_count" -ne 1 ]]; then
    check8b_source_inventory_note_failure "marker-contract"
  fi
  if ! check8b_trace_strip_inert_text "$bounded_source"; then
    check8b_source_inventory_note_failure "inert-text-parse"
  else
    executable_source="$CHECK8B_TRACE_EXECUTABLE_TEXT"
    if ! check8b_trace_split_command_list "$executable_source"; then
      check8b_source_inventory_note_failure "command-segment-parse"
    else
      for command_segment in "${CHECK8B_TRACE_COMMAND_SEGMENTS[@]}"; do
        check8b_source_inventory_parse_segment "$command_segment" || true
      done
    fi
  fi
  [[ "$CHECK8B_SOURCE_ASSIGNMENT_INVENTORY_FAILURES" -eq 0 ]]
}

check8b_source_contract_matches() {
  local source_file="$1"
  local source_output_file="$2"
  local expected_errexit="$3"
  shift 3
  local source_status=0
  local source_attempted=0
  local detector_matched=0
  local inventory_matched=0
  local source_contract_failures=0
  local source_state_name=""
  local source_state_value=""
  local state_capture_file="${source_output_file}.declare-state"
  local options_before=""
  local options_after=""
  local shopts_before=""
  local shopts_after=""
  local traps_before=""
  local traps_after=""
  local ifs_before="$IFS"
  local pwd_before="$PWD"
  local errexit_before="off"
  local errexit_after="off"
  local argv_index=0
  local -a argv_before=("$@")
  local -a argv_after=()
  local -A tracked_state=()
  local -A state_before=()
  local caller_state_clean=1

  if check8b_source_assignment_inventory_build "$source_file"; then
    inventory_matched=1
    for source_state_name in "${!check8b_source_assignment_names[@]}"; do
      tracked_state["$source_state_name"]=1
    done
  else
    source_contract_failures=$((source_contract_failures + 1))
  fi
  for source_state_name in "${!tracked_state[@]}"; do
    if declare -p "$source_state_name" > "$state_capture_file" 2>/dev/null; then
      IFS= read -r source_state_value < "$state_capture_file" || source_state_value=""
      state_before["$source_state_name"]="$source_state_value"
    else
      state_before["$source_state_name"]="__CHECK8B_ABSENT__"
    fi
  done

  [[ $- == *e* ]] && errexit_before="on"
  options_before="$(set +o)"
  shopts_before="$(shopt -p)"
  traps_before="$(trap -p)"
  : > "$source_output_file"
  if [[ "$inventory_matched" -eq 1 ]] \
    && check8b_source_declaration_only_contract "$source_file"; then
    detector_matched=1
    source_attempted=1
    unset -f check8b_classify_line 2>/dev/null || true
    # shellcheck disable=SC1090  # exact marker block is selected from the production source at runtime
    source "$source_file" > "$source_output_file" 2>&1
    source_status=$?
  else
    source_status=125
    source_contract_failures=$((source_contract_failures + 1))
  fi
  [[ $- == *e* ]] && errexit_after="on"
  options_after="$(set +o)"
  shopts_after="$(shopt -p)"
  traps_after="$(trap -p)"
  argv_after=("$@")

  if [[ "$source_attempted" -eq 1 ]]; then
    [[ "$source_status" -eq 0 ]] || source_contract_failures=$((source_contract_failures + 1))
    declare -F check8b_classify_line >/dev/null 2>&1 \
      || source_contract_failures=$((source_contract_failures + 1))
  fi
  [[ "$errexit_before" == "$expected_errexit" ]] \
    || source_contract_failures=$((source_contract_failures + 1))
  [[ "$errexit_after" == "$errexit_before" ]] \
    || source_contract_failures=$((source_contract_failures + 1))
  [[ ! -s "$source_output_file" ]] \
    || source_contract_failures=$((source_contract_failures + 1))
  [[ "$options_before" == "$options_after" ]] \
    || source_contract_failures=$((source_contract_failures + 1))
  [[ "$shopts_before" == "$shopts_after" ]] \
    || source_contract_failures=$((source_contract_failures + 1))
  [[ "$traps_before" == "$traps_after" ]] \
    || source_contract_failures=$((source_contract_failures + 1))
  [[ "$ifs_before" == "$IFS" ]] \
    || source_contract_failures=$((source_contract_failures + 1))
  [[ "$pwd_before" == "$PWD" ]] \
    || source_contract_failures=$((source_contract_failures + 1))
  if [[ "${#argv_before[@]}" -ne "${#argv_after[@]}" ]]; then
    source_contract_failures=$((source_contract_failures + 1))
  else
    for argv_index in "${!argv_before[@]}"; do
      [[ "${argv_before[$argv_index]}" == "${argv_after[$argv_index]}" ]] \
        || source_contract_failures=$((source_contract_failures + 1))
    done
  fi
  for source_state_name in "${!tracked_state[@]}"; do
    if declare -p "$source_state_name" > "$state_capture_file" 2>/dev/null; then
      IFS= read -r source_state_value < "$state_capture_file" || source_state_value=""
    else
      source_state_value="__CHECK8B_ABSENT__"
    fi
    if [[ "$source_state_value" != "${state_before[$source_state_name]}" ]]; then
      caller_state_clean=0
      source_contract_failures=$((source_contract_failures + 1))
    fi
  done

  CHECK8B_SOURCE_CONTRACT_MATCHED=0
  # shellcheck disable=SC2034  # diagnostic trace flag for a future failure dump
  CHECK8B_SOURCE_CALLER_STATE_CLEAN="$caller_state_clean"
  [[ "$source_contract_failures" -eq 0 ]] \
    && [[ "$inventory_matched" -eq 1 ]] \
    && [[ "$detector_matched" -eq 1 ]] \
    && [[ "$source_attempted" -eq 1 ]] \
    && CHECK8B_SOURCE_CONTRACT_MATCHED=1
  printf 'BUG032_TI05_SOURCE_CONTRACT sourceStatus=%s inventoryMatched=%s inventoryFailures=%s inventoryReason=%s detectorMatched=%s sourceAttempted=%s declarationFunctions=%s detectorFailures=%s detectorFirstLine=%s detectorReason=%s callerStateClean=%s errexitBefore=%s errexitAfter=%s trackedState=%s argvCount=%s failures=%s matched=%s\n' \
    "$source_status" "$inventory_matched" \
    "$CHECK8B_SOURCE_ASSIGNMENT_INVENTORY_FAILURES" \
    "$CHECK8B_SOURCE_ASSIGNMENT_INVENTORY_FIRST_REASON" \
    "$detector_matched" "$source_attempted" \
    "$CHECK8B_SOURCE_DECLARATION_FUNCTION_COUNT" \
    "$CHECK8B_SOURCE_DECLARATION_ONLY_FAILURES" \
    "$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_LINE" \
    "$CHECK8B_SOURCE_DECLARATION_ONLY_FIRST_REASON" "$caller_state_clean" \
    "$errexit_before" "$errexit_after" \
    "${#tracked_state[@]}" "${#argv_before[@]}" \
    "$source_contract_failures" "$CHECK8B_SOURCE_CONTRACT_MATCHED"
  return 0
}

check8b_source_probe_has_contract() {
  local probe_file="$1"
  local expected_match="$2"
  local expected_detector="$3"
  local expected_source_attempted="$4"
  local expected_detector_reason="$5"
  local probe_line=""
  local match_count=0
  local detector_failures=-1

  while IFS= read -r probe_line || [[ -n "$probe_line" ]]; do
    printf '%s\n' "$probe_line"
    if [[ "$probe_line" == BUG032_TI05_SOURCE_CONTRACT\ * ]] \
      && [[ "$probe_line" == *" inventoryMatched=1 "* ]] \
      && [[ "$probe_line" == *" inventoryFailures=0 "* ]] \
      && [[ "$probe_line" == *" detectorMatched=$expected_detector "* ]] \
      && [[ "$probe_line" == *" sourceAttempted=$expected_source_attempted "* ]] \
      && [[ "$probe_line" == *" detectorReason=$expected_detector_reason "* ]] \
      && [[ "$probe_line" == *" callerStateClean=1 "* ]] \
      && [[ "$probe_line" == *" matched=$expected_match" ]]; then
      if [[ "$probe_line" =~ detectorFailures=([0-9]+) ]]; then
        detector_failures="${BASH_REMATCH[1]}"
      fi
      if { [[ "$expected_detector" -eq 1 ]] && [[ "$detector_failures" -eq 0 ]]; } \
        || { [[ "$expected_detector" -eq 0 ]] && [[ "$detector_failures" -gt 0 ]]; }; then
        match_count=$((match_count + 1))
      fi
    fi
  done < "$probe_file"
  [[ "$match_count" -eq 1 ]]
}

check8b_source_inventory_fixture="$tmp_root/bug032-check8b-source-inventory-fixture.sh"
check8b_source_inventory_invalid_fixture="$tmp_root/bug032-check8b-source-inventory-invalid-fixture.sh"
cat <<'EOF' > "$check8b_source_inventory_fixture"
# BEGIN CHECK8B FINITE CLASSIFIER
_check8b_inventory_fixture() {
  CHECK8B_CLASSIFICATION="irrelevant"
  _CHECK8B_TOKENS[3]="token"
  ti05_nested_index_probe[${#ti05_nested_index_probe[@]}]="nested"
  ti05_arbitrary_scalar_probe="scalar"
  ti05_arbitrary_array_probe=(one two)
  local -a ti05_local_array=(three four)
  local -r ti05_local_one=five ti05_local_two=six
  declare -A ti05_local_assoc=([seven]=eight) ti05_second_assoc=([nine]=ten)
  typeset +x ti05_typeset_scalar=eleven ti05_typeset_indexed[2]=twelve
  readonly ti05_readonly_scalar=thirteen ti05_readonly_second=fourteen
  export ti05_export_scalar=fifteen ti05_export_second=sixteen
}
# END CHECK8B FINITE CLASSIFIER
EOF
cat <<'EOF' > "$check8b_source_inventory_invalid_fixture"
# BEGIN CHECK8B FINITE CLASSIFIER
_check8b_inventory_invalid_fixture() {
  ti05_unparseable[=value
}
# END CHECK8B FINITE CLASSIFIER
EOF
check8b_source_inventory_calibration_failures=0
if ! check8b_source_assignment_inventory_build "$check8b_classifier_file" \
  || [[ -z "${check8b_source_assignment_names[CHECK8B_CLASSIFICATION]:-}" ]] \
  || [[ -z "${check8b_source_assignment_names[_CHECK8B_TOKENS]:-}" ]]; then
  check8b_source_inventory_calibration_failures=$((check8b_source_inventory_calibration_failures + 1))
  echo "BUG032_TI05_SOURCE_INVENTORY_PRODUCTION_FIXTURE_FAILED reason=$CHECK8B_SOURCE_ASSIGNMENT_INVENTORY_FIRST_REASON names=${#check8b_source_assignment_names[@]}"
fi
if ! check8b_source_assignment_inventory_build "$check8b_source_inventory_fixture" \
  || [[ -z "${check8b_source_assignment_names[CHECK8B_CLASSIFICATION]:-}" ]] \
  || [[ -z "${check8b_source_assignment_names[_CHECK8B_TOKENS]:-}" ]] \
  || [[ -z "${check8b_source_assignment_names[ti05_nested_index_probe]:-}" ]] \
  || [[ -z "${check8b_source_assignment_names[ti05_arbitrary_scalar_probe]:-}" ]] \
  || [[ -z "${check8b_source_assignment_names[ti05_arbitrary_array_probe]:-}" ]] \
  || [[ -z "${check8b_source_assignment_names[ti05_local_array]:-}" ]] \
  || [[ -z "${check8b_source_assignment_names[ti05_local_one]:-}" ]] \
  || [[ -z "${check8b_source_assignment_names[ti05_local_two]:-}" ]] \
  || [[ -z "${check8b_source_assignment_names[ti05_local_assoc]:-}" ]] \
  || [[ -z "${check8b_source_assignment_names[ti05_second_assoc]:-}" ]] \
  || [[ -z "${check8b_source_assignment_names[ti05_typeset_indexed]:-}" ]] \
  || [[ -z "${check8b_source_assignment_names[ti05_readonly_second]:-}" ]] \
  || [[ -z "${check8b_source_assignment_names[ti05_export_second]:-}" ]]; then
  check8b_source_inventory_calibration_failures=$((check8b_source_inventory_calibration_failures + 1))
  echo "BUG032_TI05_SOURCE_INVENTORY_VALID_FIXTURE_FAILED reason=$CHECK8B_SOURCE_ASSIGNMENT_INVENTORY_FIRST_REASON names=${#check8b_source_assignment_names[@]}"
fi
if check8b_source_assignment_inventory_build "$check8b_source_inventory_invalid_fixture"; then
  check8b_source_inventory_calibration_failures=$((check8b_source_inventory_calibration_failures + 1))
  echo 'BUG032_TI05_SOURCE_INVENTORY_ACCEPTED_UNPARSEABLE_ASSIGNMENT'
fi
if [[ "$check8b_source_inventory_calibration_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B source-derived state inventory discovers scalar, indexed-array, declaration, and arbitrary canary names and rejects malformed assignment syntax"
else
  fail "BUG-032 Check 8B source-derived state inventory calibration has $check8b_source_inventory_calibration_failures failure(s)"
fi

bug032_ti05_existing_caller="stable-before-source"
unset \
  bug032_ti05_new_scalar bug032_ti05_second_scalar \
  bug032_ti05_new_array bug032_ti05_new_assoc \
  bug032_ti05_declare_one bug032_ti05_declare_two \
  bug032_ti05_typeset_one bug032_ti05_typeset_two \
  bug032_ti05_readonly_one bug032_ti05_readonly_two \
  bug032_ti05_export_one bug032_ti05_export_two \
  _CHECK8B_FUTURE_ARRAY _CHECK8B_DYNAMIC_SOURCE_MUTATION \
  ti05_source_time_scalar_canary ti05_source_time_array_canary 2>/dev/null || true

check8b_source_probe_on="$tmp_root/bug032-check8b-source-probe-errexit-on.log"
check8b_source_probe_off="$tmp_root/bug032-check8b-source-probe-errexit-off.log"
(
  set -e
  check8b_source_contract_matches "$check8b_classifier_file" \
    "$check8b_source_output_file.on" on 'source arg one' '' 'source:*' '--literal'
) > "$check8b_source_probe_on"
(
  set +e
  check8b_source_contract_matches "$check8b_classifier_file" \
    "$check8b_source_output_file.off" off 'source arg one' '' 'source:*' '--literal'
) > "$check8b_source_probe_off"

if [[ "$check8b_sourceability_failures" -eq 0 ]] \
  && [[ "$check8b_epoch_failures" -eq 0 ]] \
  && check8b_source_epoch_matches "$PLANNING_CHECKS_SCRIPT" \
    "$check8b_pinned_epoch_sha" "$check8b_pinned_epoch_bytes" \
  && check8b_source_probe_has_contract "$check8b_source_probe_on" 1 1 1 none \
  && check8b_source_probe_has_contract "$check8b_source_probe_off" 1 1 1 none; then
  :
else
  check8b_sourceability_failures=$((check8b_sourceability_failures + 1))
fi

unset -f check8b_classify_line 2>/dev/null || true
if check8b_source_declaration_only_contract "$check8b_classifier_file"; then
  # shellcheck disable=SC1090  # exact marker block is selected from the pinned production epoch
  source "$check8b_classifier_file" > "$check8b_source_output_file" 2>&1
  check8b_classifier_source_status=$?
else
  check8b_classifier_source_status=125
fi
if [[ "$check8b_classifier_source_status" -eq 0 ]] \
  && declare -F check8b_classify_line >/dev/null 2>&1 \
  && [[ ! -s "$check8b_source_output_file" ]] \
  && check8b_source_epoch_matches "$PLANNING_CHECKS_SCRIPT" \
    "$check8b_pinned_epoch_sha" "$check8b_pinned_epoch_bytes"; then
  check8b_classifier_source_status=0
  check8b_classifier_ready=1
else
  check8b_sourceability_failures=$((check8b_sourceability_failures + 1))
fi

check8b_source_mutation_labels=(output nounset errexit-on-to-off errexit-off-to-on trap ifs pwd argv two-assignments-one-line indexed-array associative-array declare-multioperand typeset-multioperand readonly-multioperand export-multioperand existing-caller-state result-state clause-array future-contract-array dynamic-state arbitrary-scalar-state arbitrary-array-state)
check8b_source_mutation_errexit=(on on on off on on on on on on on on on on on on on on on on on on)
check8b_source_mutation_payloads=(
  "printf '%s\\n' forbidden-source-output"
  'set +u'
  'set +e'
  'set -e'
  "trap ':' USR1"
  'IFS=:'
  'cd /'
  'set -- source-mutated-argv'
  'bug032_ti05_new_scalar=one bug032_ti05_second_scalar=two'
  'bug032_ti05_new_array=(created array)'
  'declare -A bug032_ti05_new_assoc=([created]=array)'
  'declare bug032_ti05_declare_one=one bug032_ti05_declare_two=two'
  'typeset bug032_ti05_typeset_one=one bug032_ti05_typeset_two=two'
  'readonly bug032_ti05_readonly_one=one bug032_ti05_readonly_two=two'
  'export bug032_ti05_export_one=one bug032_ti05_export_two=two'
  'bug032_ti05_existing_caller=mutated'
  'CHECK8B_REASON=source-mutated-reason'
  '_CHECK8B_CLAUSE_IDS=(source-mutated-clause)'
  '_CHECK8B_FUTURE_ARRAY=(source-mutated-future)'
  '_CHECK8B_DYNAMIC_SOURCE_MUTATION=created'
  'ti05_source_time_scalar_canary=mutated'
  'ti05_source_time_array_canary=(mutated array)'
)
check8b_source_mutation_failures=0
check8b_source_probe_parent_is_clean() {
  local canary_name=""

  for canary_name in \
    bug032_ti05_new_scalar bug032_ti05_second_scalar \
    bug032_ti05_new_array bug032_ti05_new_assoc \
    bug032_ti05_declare_one bug032_ti05_declare_two \
    bug032_ti05_typeset_one bug032_ti05_typeset_two \
    bug032_ti05_readonly_one bug032_ti05_readonly_two \
    bug032_ti05_export_one bug032_ti05_export_two \
    _CHECK8B_FUTURE_ARRAY _CHECK8B_DYNAMIC_SOURCE_MUTATION \
    ti05_source_time_scalar_canary ti05_source_time_array_canary; do
    declare -p "$canary_name" >/dev/null 2>&1 && return 1
  done
  [[ "$bug032_ti05_existing_caller" == "stable-before-source" ]]
}
if [[ "${#check8b_source_mutation_labels[@]}" -ne "${#check8b_source_mutation_errexit[@]}" ]] \
  || [[ "${#check8b_source_mutation_labels[@]}" -ne "${#check8b_source_mutation_payloads[@]}" ]]; then
  check8b_source_mutation_failures=$((check8b_source_mutation_failures + 1))
  echo "BUG032_TI05_SOURCE_MUTATION_MATRIX_SHAPE labels=${#check8b_source_mutation_labels[@]} errexit=${#check8b_source_mutation_errexit[@]} payloads=${#check8b_source_mutation_payloads[@]}"
fi
for check8b_index in "${!check8b_source_mutation_labels[@]}"; do
  check8b_source_mutant="$tmp_root/bug032-check8b-source-mutant-$check8b_index.sh"
  check8b_source_mutant_output="$tmp_root/bug032-check8b-source-mutant-$check8b_index.out"
  check8b_source_mutant_probe="$tmp_root/bug032-check8b-source-mutant-$check8b_index.probe"
  : > "$check8b_source_mutant"
  while IFS= read -r check8b_source_mutant_line || [[ -n "$check8b_source_mutant_line" ]]; do
    if [[ "$check8b_source_mutant_line" == '# END CHECK8B FINITE CLASSIFIER' ]]; then
      printf '%s\n' "${check8b_source_mutation_payloads[$check8b_index]}" >> "$check8b_source_mutant"
    fi
    printf '%s\n' "$check8b_source_mutant_line" >> "$check8b_source_mutant"
  done < "$check8b_classifier_file"
  if ! check8b_source_assignment_inventory_build "$check8b_source_mutant"; then
    check8b_source_mutation_failures=$((check8b_source_mutation_failures + 1))
    echo "BUG032_TI05_SOURCE_MUTATION_INVENTORY_FAILED class=${check8b_source_mutation_labels[$check8b_index]} reason=$CHECK8B_SOURCE_ASSIGNMENT_INVENTORY_FIRST_REASON"
  fi
  case "${check8b_source_mutation_labels[$check8b_index]}" in
    arbitrary-scalar-state)
      if [[ -z "${check8b_source_assignment_names[ti05_source_time_scalar_canary]:-}" ]]; then
        check8b_source_mutation_failures=$((check8b_source_mutation_failures + 1))
        echo 'BUG032_TI05_ARBITRARY_SCALAR_NOT_DERIVED'
      fi
      ;;
    arbitrary-array-state)
      if [[ -z "${check8b_source_assignment_names[ti05_source_time_array_canary]:-}" ]]; then
        check8b_source_mutation_failures=$((check8b_source_mutation_failures + 1))
        echo 'BUG032_TI05_ARBITRARY_ARRAY_NOT_DERIVED'
      fi
      ;;
  esac
  (
    if [[ "${check8b_source_mutation_errexit[$check8b_index]}" == "on" ]]; then
      set -e
    else
      set +e
    fi
    check8b_source_contract_matches "$check8b_source_mutant" \
      "$check8b_source_mutant_output" \
      "${check8b_source_mutation_errexit[$check8b_index]}" \
      'source arg one' '' 'source:*' '--literal'
  ) > "$check8b_source_mutant_probe"
  if check8b_source_probe_has_contract \
    "$check8b_source_mutant_probe" 0 0 0 top-level-non-function; then
    pass "BUG-032 Check 8B pre-source declaration-only detector rejects ${check8b_source_mutation_labels[$check8b_index]} top-level mutation before source"
  else
    check8b_source_mutation_failures=$((check8b_source_mutation_failures + 1))
    echo "BUG032_TI05_SOURCE_MUTATION_ACCEPTED class=${check8b_source_mutation_labels[$check8b_index]} expectedDetectorMatched=0 expectedSourceAttempted=0"
  fi
  if ! check8b_source_probe_parent_is_clean; then
    check8b_source_mutation_failures=$((check8b_source_mutation_failures + 1))
    echo "BUG032_TI05_SOURCE_PROBE_ISOLATION_FAILED class=${check8b_source_mutation_labels[$check8b_index]}"
  fi
done
if check8b_source_probe_parent_is_clean; then
  pass "BUG-032 Check 8B source mutation probes remain isolated and leave the parent caller canaries unchanged"
else
  check8b_source_mutation_failures=$((check8b_source_mutation_failures + 1))
  fail "BUG-032 Check 8B source mutation probe isolation changed a parent caller canary"
fi
unset -f check8b_source_probe_parent_is_clean
unset \
  bug032_ti05_existing_caller bug032_ti05_new_scalar bug032_ti05_second_scalar \
  bug032_ti05_new_array bug032_ti05_new_assoc \
  bug032_ti05_declare_one bug032_ti05_declare_two \
  bug032_ti05_typeset_one bug032_ti05_typeset_two \
  bug032_ti05_readonly_one bug032_ti05_readonly_two \
  bug032_ti05_export_one bug032_ti05_export_two \
  _CHECK8B_FUTURE_ARRAY _CHECK8B_DYNAMIC_SOURCE_MUTATION \
  ti05_source_time_scalar_canary ti05_source_time_array_canary 2>/dev/null || true
[[ "$check8b_source_mutation_failures" -eq 0 ]] \
  || check8b_sourceability_failures=$((check8b_sourceability_failures + 1))

check8b_epoch_mutation_source="$tmp_root/bug032-check8b-epoch-mutation-source.sh"
cp "$check8b_production_epoch_file" "$check8b_epoch_mutation_source"
check8b_epoch_mutation_sha="$(check8b_file_sha256 "$check8b_epoch_mutation_source")"
check8b_epoch_mutation_bytes="$(check8b_file_bytes "$check8b_epoch_mutation_source")"
printf '%s\n' '# deliberate source-epoch mutation after observation' >> "$check8b_epoch_mutation_source"
if check8b_source_epoch_matches "$check8b_epoch_mutation_source" \
  "$check8b_epoch_mutation_sha" "$check8b_epoch_mutation_bytes"; then
  check8b_epoch_failures=$((check8b_epoch_failures + 1))
  echo 'BUG032_TI05_SOURCE_EPOCH_MUTATION_ACCEPTED'
else
  pass "BUG-032 Check 8B source epoch oracle detects a production-byte change between observation and use"
fi
[[ "$check8b_epoch_failures" -eq 0 ]] \
  || check8b_sourceability_failures=$((check8b_sourceability_failures + 1))

if [[ "$check8b_sourceability_failures" -eq 0 ]] \
  && [[ "$check8b_classifier_ready" -eq 1 ]]; then
  pass "BUG-032 Check 8B finite classifier has one atomic marker block whose pre-source declaration-only detector admits only complete supported function declarations and inert marker-bounded comments"
  pass "BUG-032 Check 8B marker discovery and exact marker-bounded extraction share one ordered pinned production-source epoch"
  pass "BUG-032 Check 8B sourceability preserves exact argv and caller options with errexit initially on and off"
  pass "BUG-032 Check 8B declaration-only detector rejects two assignments on one line, arrays, multioperand declare/typeset/readonly/export forms, existing-caller mutation, printf, and every other calibrated top-level executable before source"
  pass "BUG-032 Check 8B finite classifier extracted from production markers (no test/source drift)"
else
  check8b_source_output_bytes="$(wc -c < "$check8b_source_output_file" 2>/dev/null || printf 'unavailable')"
  fail "BUG-032 Check 8B finite classifier marker/sourceability contract failed (sourceabilityFailures=$check8b_sourceability_failures epochFailures=$check8b_epoch_failures pinnedSha=$check8b_pinned_epoch_sha pinnedBytes=$check8b_pinned_epoch_bytes readPasses=$check8b_source_read_passes streamFailures=$check8b_marker_stream_failures begin=$check8b_marker_begin_count end=$check8b_marker_end_count beginLine=$check8b_marker_begin_line endLine=$check8b_marker_end_line extractedLines=$check8b_extracted_line_count bodyLines=$check8b_extracted_body_line_count functions=$check8b_extracted_function_count entrypoints=$check8b_extracted_entrypoint_count extractedBegin=$check8b_extracted_begin_count extractedEnd=$check8b_extracted_end_count syntaxStatus=$check8b_classifier_syntax_status sourceStatus=$check8b_classifier_source_status sourceOutputBytes=$check8b_source_output_bytes mutationProbeFailures=$check8b_source_mutation_failures)"
fi

bug032_check8b_classify() {
  local declaration="$1"

  CHECK8B_LAST_STATUS=2
  CHECK8B_CLASSIFICATION="__check8b_unset__"
  CHECK8B_VERB="__check8b_unset__"
  CHECK8B_MUTATION_TARGET="__check8b_unset__"
  CHECK8B_DIRECT_SURFACES="__check8b_unset__"
  CHECK8B_PRESERVED_SURFACES="__check8b_unset__"
  CHECK8B_REASON="__check8b_unset__"
  CHECK8B_UNRESOLVED_PHRASE="__check8b_unset__"
  CHECK8B_BOUNDARY="__check8b_unset__"
  CHECK8B_TOKEN_COUNT=-1
  CHECK8B_CANDIDATE_COUNT=-1
  _CHECK8B_TOKENS=(__check8b_stale_token__)
  _CHECK8B_CLAUSE_IDS=(999)
  _CHECK8B_CANDIDATE_INDEXES=(999)
  if [[ "$check8b_classifier_ready" -ne 1 ]]; then
    return 2
  fi
  if check8b_classify_line "$declaration"; then
    CHECK8B_LAST_STATUS=0
  else
    CHECK8B_LAST_STATUS=$?
  fi
  return "$CHECK8B_LAST_STATUS"
}

# The helper result is one closed record. Every fixture passes expectations
# authored from the BUG-032 design contract; the matcher never derives an
# expected field from the helper output it is checking.
check8b_helper_record_matches() {
  [[ "$#" -eq 14 ]] || return 1
  local expected_status="$1"
  local expected_classification="$2"
  local expected_verb="$3"
  local expected_target="$4"
  local expected_direct="$5"
  local expected_preserved="$6"
  local expected_reason="$7"
  local expected_unresolved="$8"
  local expected_boundary="$9"
  local expected_token_count="${10}"
  local expected_candidate_count="${11}"
  local expected_retained_tokens="${12}"
  local expected_retained_clause_ids="${13}"
  local expected_retained_candidates="${14}"
  local result_name=""

  for result_name in \
    CHECK8B_LAST_STATUS CHECK8B_CLASSIFICATION CHECK8B_VERB \
    CHECK8B_MUTATION_TARGET CHECK8B_DIRECT_SURFACES \
    CHECK8B_PRESERVED_SURFACES CHECK8B_REASON \
    CHECK8B_UNRESOLVED_PHRASE CHECK8B_BOUNDARY CHECK8B_TOKEN_COUNT \
    CHECK8B_CANDIDATE_COUNT; do
    declare -p "$result_name" >/dev/null 2>&1 || return 1
  done
  for result_name in \
    _CHECK8B_TOKENS _CHECK8B_CLAUSE_IDS _CHECK8B_CANDIDATE_INDEXES; do
    declare -p "$result_name" >/dev/null 2>&1 || return 1
  done

  [[ "$CHECK8B_LAST_STATUS" -eq "$expected_status" ]] \
    && [[ "$CHECK8B_CLASSIFICATION" == "$expected_classification" ]] \
    && [[ "$CHECK8B_VERB" == "$expected_verb" ]] \
    && [[ "$CHECK8B_MUTATION_TARGET" == "$expected_target" ]] \
    && [[ "$CHECK8B_DIRECT_SURFACES" == "$expected_direct" ]] \
    && [[ "$CHECK8B_PRESERVED_SURFACES" == "$expected_preserved" ]] \
    && [[ "$CHECK8B_REASON" == "$expected_reason" ]] \
    && [[ "$CHECK8B_UNRESOLVED_PHRASE" == "$expected_unresolved" ]] \
    && [[ "$CHECK8B_BOUNDARY" == "$expected_boundary" ]] \
    && [[ "$CHECK8B_TOKEN_COUNT" -eq "$expected_token_count" ]] \
    && [[ "$CHECK8B_CANDIDATE_COUNT" -eq "$expected_candidate_count" ]] \
    && [[ "${#_CHECK8B_TOKENS[@]}" -eq "$expected_retained_tokens" ]] \
    && [[ "${#_CHECK8B_CLAUSE_IDS[@]}" -eq "$expected_retained_clause_ids" ]] \
    && [[ "${#_CHECK8B_CANDIDATE_INDEXES[@]}" -eq "$expected_retained_candidates" ]]
}

check8b_helper_matcher_calibration_failures=0
CHECK8B_LAST_STATUS=0
CHECK8B_CLASSIFICATION="direct-positive"
CHECK8B_VERB="remove"
CHECK8B_MUTATION_TARGET="route"
CHECK8B_DIRECT_SURFACES="remove:route"
CHECK8B_PRESERVED_SURFACES="none"
CHECK8B_REASON="direct"
CHECK8B_UNRESOLVED_PHRASE="none"
CHECK8B_BOUNDARY="not-applicable"
CHECK8B_TOKEN_COUNT=5
CHECK8B_CANDIDATE_COUNT=1
_CHECK8B_TOKENS=(remove the public api route)
_CHECK8B_CLAUSE_IDS=(0 0 0 0 0)
_CHECK8B_CANDIDATE_INDEXES=(0)
if ! check8b_helper_record_matches \
  0 direct-positive remove route remove:route none direct none \
  not-applicable 5 1 5 5 1; then
  check8b_helper_matcher_calibration_failures=$((check8b_helper_matcher_calibration_failures + 1))
  echo 'BUG032_TI02_HELPER_MATCHER_REJECTED_COMPLETE_FIXTURE'
fi
CHECK8B_REASON="mixed"
if check8b_helper_record_matches \
  0 direct-positive remove route remove:route none direct none \
  not-applicable 5 1 5 5 1; then
  check8b_helper_matcher_calibration_failures=$((check8b_helper_matcher_calibration_failures + 1))
  echo 'BUG032_TI02_HELPER_MATCHER_ACCEPTED_ONE_FIELD_MUTATION'
fi
CHECK8B_REASON="direct"
unset CHECK8B_BOUNDARY
if check8b_helper_record_matches \
  0 direct-positive remove route remove:route none direct none \
  not-applicable 5 1 5 5 1; then
  check8b_helper_matcher_calibration_failures=$((check8b_helper_matcher_calibration_failures + 1))
  echo 'BUG032_TI02_HELPER_MATCHER_ACCEPTED_MISSING_FIELD'
fi
CHECK8B_BOUNDARY="not-applicable"
CHECK8B_UNRESOLVED_PHRASE=""
if check8b_helper_record_matches \
  0 direct-positive remove route remove:route none direct none \
  not-applicable 5 1 5 5 1; then
  check8b_helper_matcher_calibration_failures=$((check8b_helper_matcher_calibration_failures + 1))
  echo 'BUG032_TI02_HELPER_MATCHER_ACCEPTED_EMPTY_FIELD'
fi
if [[ "$check8b_helper_matcher_calibration_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B exact helper matcher rejects one-field mutations plus missing and empty fields"
else
  fail "BUG-032 Check 8B exact helper matcher calibration has $check8b_helper_matcher_calibration_failures failure(s)"
fi

check8b_must_not="$tmp_root/bug032-check8b-must-not-flag.txt"
cat <<'EOF' > "$check8b_must_not"
The stale generation path is replaced by the current generated artifact.
The provider implementation is replaced without changing its contract.
The lifecycle state is replaced by the successor state; its public contract is unchanged.
The generated artifact replaces a stale artifact path; route and endpoint identities are unchanged.
EOF

check8b_must_not_failures=0
check8b_must_not_token_counts=(11 9 14 14)
check8b_index=0
while IFS= read -r check8b_line || [[ -n "$check8b_line" ]]; do
  if bug032_check8b_classify "$check8b_line"; then :; else :; fi
  if ! check8b_helper_record_matches \
    0 irrelevant none none none none none none not-applicable \
    "${check8b_must_not_token_counts[$check8b_index]}" 0 \
    "${check8b_must_not_token_counts[$check8b_index]}" \
    "${check8b_must_not_token_counts[$check8b_index]}" 0; then
    check8b_must_not_failures=$((check8b_must_not_failures + 1))
    echo "BUG032_CHECK8B_UNEXPECTED status=$CHECK8B_LAST_STATUS class=$CHECK8B_CLASSIFICATION verb=$CHECK8B_VERB target=$CHECK8B_MUTATION_TARGET direct=$CHECK8B_DIRECT_SURFACES preserved=$CHECK8B_PRESERVED_SURFACES reason=$CHECK8B_REASON unresolved=$CHECK8B_UNRESOLVED_PHRASE boundary=$CHECK8B_BOUNDARY tokenCount=$CHECK8B_TOKEN_COUNT candidateCount=$CHECK8B_CANDIDATE_COUNT retainedTokens=${#_CHECK8B_TOKENS[@]} retainedClauseIds=${#_CHECK8B_CLAUSE_IDS[@]} retainedCandidates=${#_CHECK8B_CANDIDATE_INDEXES[@]} line=$check8b_line"
  fi
  check8b_index=$((check8b_index + 1))
done < "$check8b_must_not"
if [[ "$check8b_must_not_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B ignores stale-generation, provider, lifecycle, and artifact replacement semantics"
else
  fail "BUG-032 Check 8B false-positives on $check8b_must_not_failures replacement declaration(s) that do not mutate a consumer interface"
fi

check8b_exact_inflections="$tmp_root/bug032-check8b-exact-inflections.txt"
cat <<'EOF' > "$check8b_exact_inflections"
The migration renames the public route from /old to /new.
The migration removes the legacy path after compatibility expires.
EOF
check8b_inflection_count=0
check8b_inflection_verbs=(rename remove)
check8b_inflection_targets=(route path)
check8b_inflection_token_counts=(10 9)
check8b_index=0
while IFS= read -r check8b_line || [[ -n "$check8b_line" ]]; do
  bug032_check8b_classify "$check8b_line"
  if check8b_helper_record_matches \
    0 direct-positive "${check8b_inflection_verbs[$check8b_index]}" \
    "${check8b_inflection_targets[$check8b_index]}" \
    "${check8b_inflection_verbs[$check8b_index]}:${check8b_inflection_targets[$check8b_index]}" \
    none direct none not-applicable \
    "${check8b_inflection_token_counts[$check8b_index]}" 1 \
    "${check8b_inflection_token_counts[$check8b_index]}" \
    "${check8b_inflection_token_counts[$check8b_index]}" 1; then
    check8b_inflection_count=$((check8b_inflection_count + 1))
  fi
  check8b_index=$((check8b_index + 1))
done < "$check8b_exact_inflections"
if [[ "$check8b_inflection_count" -eq 2 ]]; then
  pass "BUG032-IV-F1 Check 8B triggers on the exact mutation inflections 'renames' and 'removes'"
else
  fail "BUG032-IV-F1 Check 8B detected $check8b_inflection_count of 2 exact 'renames'/'removes' mutations"
fi

check8b_unrelated_clause='Remove stale cache entries after replacement; the public API contract is unchanged.'
bug032_check8b_classify "$check8b_unrelated_clause"
if check8b_helper_record_matches \
  0 negative remove 'stale cache entries' none contract preserved-surface none \
  not-applicable 12 1 12 12 1; then
  pass "BUG032-IV-F2 Check 8B does not bridge cache cleanup to an unchanged public API contract in another clause"
else
  fail "BUG032-IV-F2 Check 8B did not classify the punctuated cache cleanup as negative (classification=$CHECK8B_CLASSIFICATION mutationTarget=$CHECK8B_MUTATION_TARGET preservedSurfaces=$CHECK8B_PRESERVED_SURFACES)"
fi

check8b_must_flag="$tmp_root/bug032-check8b-must-flag.txt"
cat <<'EOF' > "$check8b_must_flag"
The public route is renamed from /old to /new.
The legacy path is removed after migration.
Rename the endpoint from v1 to v2.
The public contract is deprecated.
Move the identifier to the canonical key.
EOF
check8b_pos_count=0
check8b_must_flag_verbs=(rename remove rename deprecate move)
check8b_must_flag_targets=(route path endpoint contract identifier)
check8b_must_flag_token_counts=(9 7 7 5 7)
check8b_index=0
while IFS= read -r check8b_line || [[ -n "$check8b_line" ]]; do
  bug032_check8b_classify "$check8b_line"
  if check8b_helper_record_matches \
    0 direct-positive "${check8b_must_flag_verbs[$check8b_index]}" \
    "${check8b_must_flag_targets[$check8b_index]}" \
    "${check8b_must_flag_verbs[$check8b_index]}:${check8b_must_flag_targets[$check8b_index]}" \
    none direct none not-applicable \
    "${check8b_must_flag_token_counts[$check8b_index]}" 1 \
    "${check8b_must_flag_token_counts[$check8b_index]}" \
    "${check8b_must_flag_token_counts[$check8b_index]}" 1; then
    check8b_pos_count=$((check8b_pos_count + 1))
  fi
  check8b_index=$((check8b_index + 1))
done < "$check8b_must_flag"
if [[ "$check8b_pos_count" -eq 5 ]]; then
  pass "BUG-032 Check 8B still flags all 5 explicit route/path/endpoint/contract/identifier mutations"
else
  fail "BUG-032 Check 8B detected $check8b_pos_count of 5 explicit consumer-interface mutations"
fi

check8b_scn013_unpunctuated='Remove stale cache entries before invoking the public API route without changing that route'
check8b_scn013_punctuated="${check8b_scn013_unpunctuated}."
bug032_check8b_classify 'Remove the public API route'
if check8b_helper_record_matches \
  0 direct-positive remove route remove:route none direct none \
  not-applicable 5 1 5 5 1; then
  pass "BUG-032 SCN-032-013 pre-fix direct public-route removal control remains positive"
else
  fail "BUG-032 SCN-032-013 pre-fix direct public-route removal control must remain a complete positive record (status=$CHECK8B_LAST_STATUS classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON)"
fi

bug032_check8b_classify "$check8b_scn013_unpunctuated"
if check8b_helper_record_matches \
  0 negative remove 'stale cache entries' none route preserved-surface none \
  not-applicable 14 1 14 14 1; then
  pass "BUG-032 Check 8B classifies unpunctuated cache removal plus preserved public route as negative"
else
  fail "BUG-032 Check 8B classifies unpunctuated cache removal plus preserved public route as one complete negative record (status=$CHECK8B_LAST_STATUS classification=$CHECK8B_CLASSIFICATION mutationTarget=$CHECK8B_MUTATION_TARGET preservedSurfaces=$CHECK8B_PRESERVED_SURFACES reason=$CHECK8B_REASON)"
fi

check8b_scn013_negative_failures=0
for check8b_line in "$check8b_scn013_punctuated" "$check8b_scn013_unpunctuated"; do
  bug032_check8b_classify "$check8b_line"
  if ! check8b_helper_record_matches \
    0 negative remove 'stale cache entries' none route preserved-surface none \
    not-applicable 14 1 14 14 1; then
    check8b_scn013_negative_failures=$((check8b_scn013_negative_failures + 1))
    echo "BUG032_SCN013_NEGATIVE_MISMATCH classification=$CHECK8B_CLASSIFICATION mutationTarget=$CHECK8B_MUTATION_TARGET preservedSurfaces=$CHECK8B_PRESERVED_SURFACES reason=$CHECK8B_REASON"
  fi
done
if [[ "$check8b_scn013_negative_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B classifies punctuated and unpunctuated cache removal plus preserved public route as negative"
else
  fail "BUG-032 Check 8B negative fixture matrix has $check8b_scn013_negative_failures mismatch(es)"
fi

check8b_scn013_direct_failures=0
check8b_scn013_direct_token_counts=(5 6)
check8b_index=0
for check8b_line in 'Remove the public API route' 'The public API route is removed'; do
  bug032_check8b_classify "$check8b_line"
  if ! check8b_helper_record_matches \
    0 direct-positive remove route remove:route none direct none \
    not-applicable "${check8b_scn013_direct_token_counts[$check8b_index]}" 1 \
    "${check8b_scn013_direct_token_counts[$check8b_index]}" \
    "${check8b_scn013_direct_token_counts[$check8b_index]}" 1; then
    check8b_scn013_direct_failures=$((check8b_scn013_direct_failures + 1))
    echo "BUG032_SCN013_DIRECT_MISMATCH classification=$CHECK8B_CLASSIFICATION directSurfaces=$CHECK8B_DIRECT_SURFACES reason=$CHECK8B_REASON line=$check8b_line"
  fi
  check8b_index=$((check8b_index + 1))
done
if [[ "$check8b_scn013_direct_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B keeps active and passive public-route removal direct-positive"
else
  fail "BUG-032 Check 8B direct active/passive fixture matrix has $check8b_scn013_direct_failures mismatch(es)"
fi

check8b_scn013_mixed='Preserve the public API route while removing the legacy redirect'
bug032_check8b_classify "$check8b_scn013_mixed"
if check8b_helper_record_matches \
  0 mixed-surface remove redirect remove:redirect route mixed none \
  not-applicable 10 1 10 10 1; then
  pass "BUG-032 Check 8B classifies preserved route plus removed redirect as mixed-surface"
else
  fail "BUG-032 Check 8B did not preserve mixed-surface separation (classification=$CHECK8B_CLASSIFICATION directSurfaces=$CHECK8B_DIRECT_SURFACES preservedSurfaces=$CHECK8B_PRESERVED_SURFACES reason=$CHECK8B_REASON)"
fi

check8b_scn013_conflict='Remove the public API route without changing that route'
bug032_check8b_classify "$check8b_scn013_conflict"
if check8b_helper_record_matches \
  0 ambiguous remove unresolved none route conflict none \
  not-applicable 9 1 9 9 1; then
  pass "BUG-032 Check 8B blocks same-surface mutation and preservation as ambiguous"
else
  fail "BUG-032 Check 8B did not block the same-surface contradiction (classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON)"
fi

check8b_scn013_unresolved='Remove stale cache entries near the public API route'
bug032_check8b_classify "$check8b_scn013_unresolved"
if check8b_helper_record_matches \
  0 ambiguous remove unresolved none none unresolved 'public api route' \
  not-applicable 9 1 9 9 1; then
  pass "BUG-032 Check 8B blocks unresolved nearby route mention as ambiguous"
else
  fail "BUG-032 Check 8B did not block the unresolved nearby route mention (classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON)"
fi

check8b_scn013_token_limit='Remove'
for _ in {1..127}; do
  check8b_scn013_token_limit="$check8b_scn013_token_limit filler"
done
check8b_scn013_token_limit="$check8b_scn013_token_limit route"
bug032_check8b_classify "$check8b_scn013_token_limit"
if check8b_helper_record_matches \
  0 ambiguous none unresolved none none token-limit none \
  tokens=129/128:first-overflow 128 0 128 128 0; then
  pass "BUG-032 Check 8B blocks relevant declarations above 128 tokens as ambiguous"
else
  fail "BUG-032 Check 8B did not fail closed at the 128-token bound (classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON)"
fi

check8b_scn013_candidate_limit='remove route remove path remove endpoint remove contract remove api remove url remove slug remove identifier remove redirect'
bug032_check8b_classify "$check8b_scn013_candidate_limit"
if check8b_helper_record_matches \
  0 ambiguous remove unresolved none none candidate-limit none \
  candidates=9/8:first-overflow 18 9 18 18 8; then
  pass "BUG-032 Check 8B blocks more than eight relationship candidates as ambiguous"
else
  fail "BUG-032 Check 8B did not fail closed above eight relationship candidates (classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON)"
fi

bug032_run_check8b_guard_case() {
  local case_slug="$1"
  local declaration="$2"
  local case_dir="$tmp_root/specs/954-bug032-check8b-$case_slug"

  cp -R "$positive_feature_dir" "$case_dir"
  {
    printf '\n### Consumer Interface Declaration\n\n'
    printf '%s\n' "$declaration"
  } >> "$case_dir/scopes.md"
  BUG032_CHECK8B_GUARD_LOG="$tmp_root/bug032-check8b-$case_slug.log"
  BUG032_CHECK8B_GUARD_STATUS="$(run_capture "$BUG032_CHECK8B_GUARD_LOG" bash "$GUARD_SCRIPT" "$case_dir")"
}

# The operator contract is one closed, ordered record. This parser rejects a
# missing record, duplicate expected records, malformed or reordered fields,
# legacy camelCase/narrative output, and an extra field after `correction`.
# It validates every Check 8B record in the log, not merely one convenient
# substring, so one valid record cannot hide a malformed sibling. Explicit
# non-Check-8B records own their fields until a separator or new check record;
# target-shaped fields without such an owner remain malformed orphans.
check8b_guard_record_matches() {
  local log_file="$1"
  local expected_scope="$2"
  local expected_classification="$3"
  local expected_target="$4"
  local expected_direct="$5"
  local expected_preserved="$6"
  local expected_reason="$7"
  local expected_boundary="$8"
  local expected_impact="$9"
  local expected_sweep="${10}"
  local expected_completion="${11}"
  local expected_inventory="${12}"
  local expected_result="${13}"
  local expected_correction="${14}"

  awk \
    -v expected_scope="$expected_scope" \
    -v expected_classification="$expected_classification" \
    -v expected_target="$expected_target" \
    -v expected_direct="$expected_direct" \
    -v expected_preserved="$expected_preserved" \
    -v expected_reason="$expected_reason" \
    -v expected_boundary="$expected_boundary" \
    -v expected_impact="$expected_impact" \
    -v expected_sweep="$expected_sweep" \
    -v expected_completion="$expected_completion" \
    -v expected_inventory="$expected_inventory" \
    -v expected_result="$expected_result" \
    -v expected_correction="$expected_correction" '
    BEGIN {
      field_count = split("check scope source-location classification mutation-target direct-surfaces preserved-surfaces reason boundary impact-checks impact-sweep-section impact-completion-item impact-consumer-inventory result correction", fields, " ")
      expected[1] = "Check 8B"
      expected[2] = expected_scope
      expected[4] = expected_classification
      expected[5] = expected_target
      expected[6] = expected_direct
      expected[7] = expected_preserved
      expected[8] = expected_reason
      expected[9] = expected_boundary
      expected[10] = expected_impact
      expected[11] = expected_sweep
      expected[12] = expected_completion
      expected[13] = expected_inventory
      expected[14] = expected_result
      expected[15] = expected_correction
    }
    {
      line = $0
      if (line == "check: Check 8B") {
        if (active) malformed++
        record_count++
        active = 1
        foreign_record_active = 0
        closed_tail = 0
        field_index = 1
        for (value_index = 1; value_index <= field_count; value_index++) values[value_index] = ""
        values[1] = "Check 8B"
        next
      }

      if (line ~ /^check: [^[:space:]]/) {
        if (active) malformed++
        active = 0
        foreign_record_active = 1
        closed_tail = 0
        next
      }

      if (foreign_record_active) {
        if (line ~ /^--- .* ---$/ || line ~ /^BEGIN[_ ]TRANSITION_GUARD_RESULT/) {
          foreign_record_active = 0
          closed_tail = 0
        }
        next
      }

      if (line ~ /Check 8B[[:space:]]+scope=/ ||
          line ~ /(^|[^[:alnum:]_-])(mutationTarget|directSurfaces|preservedSurfaces|impactChecks|impactSweepSection|impactCompletionItem|impactConsumerInventory)([^[:alnum:]_-]|$)/) {
        legacy_count++
        malformed++
        if (active) active = 0
        next
      }

      if (active) {
        field_index++
        expected_prefix = fields[field_index] ": "
        if (field_index > field_count || index(line, expected_prefix) != 1) {
          malformed++
          active = 0
          next
        }
        values[field_index] = substr(line, length(expected_prefix) + 1)
        if (field_index == field_count) {
          complete_count++
          record_matches = 1
          for (value_index = 1; value_index <= field_count; value_index++) {
            if (value_index == 3) {
              source_prefix = expected_scope ":"
              source_line = substr(values[value_index], length(source_prefix) + 1)
              if (index(values[value_index], source_prefix) != 1 || source_line !~ /^[0-9]+$/) record_matches = 0
            } else if (values[value_index] != expected[value_index]) {
              record_matches = 0
            }
          }
          if (record_matches) expected_match_count++
          active = 0
          closed_tail = 1
        }
        next
      }

      if (line ~ /^(scope|source-location|classification|mutation-target|direct-surfaces|preserved-surfaces|reason|boundary|impact-checks|impact-sweep-section|impact-completion-item|impact-consumer-inventory|result|correction): /) {
        orphan_field_count++
        malformed++
        next
      }

      if (line ~ /^--- Check [0-9A-Z]+:/ || line ~ /^BEGIN[_ ]TRANSITION_GUARD_RESULT/) {
        closed_tail = 0
        next
      }

      if (closed_tail && line ~ /^[a-z][a-z0-9-]*: /) {
        extra_field_count++
        malformed++
      }
    }
    END {
      if (active) malformed++
      if (record_count != 1 || complete_count != 1 || expected_match_count != 1 ||
          legacy_count != 0 || orphan_field_count != 0 || extra_field_count != 0 || malformed != 0) exit 1
    }
  ' "$log_file"
}

check8b_direct_correction_for() {
  local direct_surfaces="$1"
  CHECK8B_EXPECTED_CORRECTION="Add only the missing Consumer Impact Sweep section, completion item, and affected-consumer inventory for direct surfaces: $direct_surfaces."
}

check8b_record_oracle_valid="$tmp_root/bug032-check8b-record-oracle-valid.log"
check8b_record_oracle_foreign_then_valid="$tmp_root/bug032-check8b-record-oracle-foreign-then-valid.log"
check8b_record_oracle_orphan="$tmp_root/bug032-check8b-record-oracle-orphan.log"
check8b_record_oracle_missing="$tmp_root/bug032-check8b-record-oracle-missing.log"
check8b_record_oracle_incomplete="$tmp_root/bug032-check8b-record-oracle-incomplete.log"
check8b_record_oracle_incomplete_sibling="$tmp_root/bug032-check8b-record-oracle-incomplete-sibling.log"
check8b_record_oracle_duplicate="$tmp_root/bug032-check8b-record-oracle-duplicate.log"
check8b_record_oracle_nonmatching_sibling="$tmp_root/bug032-check8b-record-oracle-nonmatching-sibling.log"
check8b_record_oracle_unordered="$tmp_root/bug032-check8b-record-oracle-unordered.log"
check8b_record_oracle_legacy="$tmp_root/bug032-check8b-record-oracle-legacy.log"
check8b_record_oracle_legacy_after_separator="$tmp_root/bug032-check8b-record-oracle-legacy-after-separator.log"
check8b_record_oracle_open="$tmp_root/bug032-check8b-record-oracle-open.log"
check8b_record_oracle_open_after_prose="$tmp_root/bug032-check8b-record-oracle-open-after-prose.log"
cat <<'EOF' > "$check8b_record_oracle_valid"
check: Check 8B
scope: scopes.md
source-location: scopes.md:24
classification: negative
mutation-target: route
direct-surfaces: none
preserved-surfaces: route
reason: preserved-surface
boundary: not-applicable
impact-checks: skipped
impact-sweep-section: skipped
impact-completion-item: skipped
impact-consumer-inventory: skipped
result: continue
correction: none
EOF
cat <<'EOF' > "$check8b_record_oracle_foreign_then_valid"
check: Context projection
scope: scopes.md
consumers: Check 8B,Check 5A
projection-status: complete
producer-status: complete
input-read-status: complete
source-location: scope-start
active-count: 10
fixture-count: 4
structural-status: preserved
reason: none
boundary: complete
check-8b-disposition: pending
check-8b-impact-checks: pending
check-5a-disposition: pending
check-5a-stress-checks: pending
result: continue
correction: none
--- Check 8B: Consumer Trace Planning For Renames/Removals ---
EOF
while IFS= read -r check8b_record_line || [[ -n "$check8b_record_line" ]]; do
  printf '%s\n' "$check8b_record_line" >> "$check8b_record_oracle_foreign_then_valid"
done < "$check8b_record_oracle_valid"
printf '%s\n' 'scope: scopes.md' > "$check8b_record_oracle_orphan"
printf '%s\n' 'unrelated guard output with no diagnostic record' > "$check8b_record_oracle_missing"
while IFS= read -r check8b_record_line || [[ -n "$check8b_record_line" ]]; do
  [[ "$check8b_record_line" == 'correction: none' ]] && continue
  printf '%s\n' "$check8b_record_line" >> "$check8b_record_oracle_incomplete"
done < "$check8b_record_oracle_valid"
while IFS= read -r check8b_record_line || [[ -n "$check8b_record_line" ]]; do
  [[ "$check8b_record_line" == 'correction: none' ]] && continue
  printf '%s\n' "$check8b_record_line" >> "$check8b_record_oracle_incomplete_sibling"
done < "$check8b_record_oracle_valid"
printf '%s\n' '' 'unrelated prose cannot close or forgive an incomplete record' >> "$check8b_record_oracle_incomplete_sibling"
while IFS= read -r check8b_record_line || [[ -n "$check8b_record_line" ]]; do
  printf '%s\n' "$check8b_record_line" >> "$check8b_record_oracle_incomplete_sibling"
done < "$check8b_record_oracle_valid"
while IFS= read -r check8b_record_line || [[ -n "$check8b_record_line" ]]; do
  printf '%s\n' "$check8b_record_line" >> "$check8b_record_oracle_duplicate"
done < "$check8b_record_oracle_valid"
while IFS= read -r check8b_record_line || [[ -n "$check8b_record_line" ]]; do
  printf '%s\n' "$check8b_record_line" >> "$check8b_record_oracle_duplicate"
done < "$check8b_record_oracle_valid"
while IFS= read -r check8b_record_line || [[ -n "$check8b_record_line" ]]; do
  printf '%s\n' "$check8b_record_line" >> "$check8b_record_oracle_nonmatching_sibling"
done < "$check8b_record_oracle_valid"
cat <<'EOF' >> "$check8b_record_oracle_nonmatching_sibling"
check: Check 8B
scope: scopes.md
source-location: scopes.md:31
classification: direct-positive
mutation-target: route
direct-surfaces: remove:route
preserved-surfaces: none
reason: direct
boundary: not-applicable
impact-checks: run
impact-sweep-section: missing
impact-completion-item: missing
impact-consumer-inventory: missing
result: blocked
correction: Add only the missing Consumer Impact Sweep section, completion item, and affected-consumer inventory for direct surfaces: remove:route.
EOF
cat <<'EOF' > "$check8b_record_oracle_unordered"
check: Check 8B
scope: scopes.md
source-location: scopes.md:24
classification: negative
mutation-target: route
direct-surfaces: none
preserved-surfaces: route
boundary: not-applicable
reason: preserved-surface
impact-checks: skipped
impact-sweep-section: skipped
impact-completion-item: skipped
impact-consumer-inventory: skipped
result: continue
correction: none
EOF
cat <<'EOF' > "$check8b_record_oracle_legacy"
Check 8B scope=scopes.md classification=negative mutationTarget=route directSurfaces=none preservedSurfaces=route reason=preserved-surface impactChecks=skipped result=continue correction=none
EOF
while IFS= read -r check8b_record_line || [[ -n "$check8b_record_line" ]]; do
  printf '%s\n' "$check8b_record_line" >> "$check8b_record_oracle_legacy_after_separator"
done < "$check8b_record_oracle_valid"
cat <<'EOF' >> "$check8b_record_oracle_legacy_after_separator"
check: Context projection
scope: scopes.md
result: continue
correction: none
--- unrelated separator ---
INFO: Check 8B scope=scopes.md classification=negative mutationTarget=route directSurfaces=none preservedSurfaces=route reason=preserved-surface impactChecks=skipped result=continue correction=none
EOF
cat <<'EOF' > "$check8b_record_oracle_open"
check: Check 8B
scope: scopes.md
source-location: scopes.md:24
classification: negative
mutation-target: route
direct-surfaces: none
preserved-surfaces: route
reason: preserved-surface
boundary: not-applicable
impact-checks: skipped
impact-sweep-section: skipped
impact-completion-item: skipped
impact-consumer-inventory: skipped
result: continue
correction: none
debug-field: forbidden
EOF
while IFS= read -r check8b_record_line || [[ -n "$check8b_record_line" ]]; do
  printf '%s\n' "$check8b_record_line" >> "$check8b_record_oracle_open_after_prose"
done < "$check8b_record_oracle_valid"
cat <<'EOF' >> "$check8b_record_oracle_open_after_prose"

unrelated prose and a blank line cannot make the closed record open-ended
debug-field: forbidden-after-prose
EOF

check8b_expect_guard_oracle_rejection() {
  local oracle_file="$1"
  local oracle_label="$2"

  if check8b_guard_record_matches "$oracle_file" \
    scopes.md negative route none route preserved-surface not-applicable \
    skipped skipped skipped skipped continue none; then
    fail "BUG-032 Check 8B guard oracle rejects $oracle_label"
  else
    pass "BUG-032 Check 8B guard oracle rejects $oracle_label"
  fi
}

if check8b_guard_record_matches "$check8b_record_oracle_valid" \
  scopes.md negative route none route preserved-surface not-applicable \
  skipped skipped skipped skipped continue none; then
  pass "BUG-032 Check 8B guard oracle accepts exactly one complete ordered expected record"
else
  fail "BUG-032 Check 8B guard oracle rejected its one complete ordered expected record"
fi
if check8b_guard_record_matches "$check8b_record_oracle_foreign_then_valid" \
  scopes.md negative route none route preserved-surface not-applicable \
  skipped skipped skipped skipped continue none; then
  pass "BUG-032 Check 8B guard oracle ignores a foreign Context projection record"
else
  fail "BUG-032 Check 8B guard oracle let a foreign Context projection poison the expected record"
fi
check8b_expect_guard_oracle_rejection "$check8b_record_oracle_orphan" 'a true orphan target field'
check8b_expect_guard_oracle_rejection "$check8b_record_oracle_missing" 'a missing record'
check8b_expect_guard_oracle_rejection "$check8b_record_oracle_incomplete" 'an incomplete record'
check8b_expect_guard_oracle_rejection "$check8b_record_oracle_incomplete_sibling" 'an incomplete record hidden before a complete expected sibling'
check8b_expect_guard_oracle_rejection "$check8b_record_oracle_duplicate" 'duplicate complete matching records'
check8b_expect_guard_oracle_rejection "$check8b_record_oracle_nonmatching_sibling" 'one matching record plus one complete nonmatching sibling'
check8b_expect_guard_oracle_rejection "$check8b_record_oracle_unordered" 'an unordered record'
check8b_expect_guard_oracle_rejection "$check8b_record_oracle_legacy" 'legacy camelCase narrative output'
check8b_expect_guard_oracle_rejection "$check8b_record_oracle_legacy_after_separator" 'legacy camelCase narrative after a foreign record and nonstandard separator'
check8b_expect_guard_oracle_rejection "$check8b_record_oracle_open" 'an extra field after correction'
check8b_expect_guard_oracle_rejection "$check8b_record_oracle_open_after_prose" 'an extra field after correction separated by blank and prose lines'

bug032_run_check8b_guard_case "negative-unpunctuated" "$check8b_scn013_unpunctuated"
if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
  && check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
    scopes.md negative 'stale cache entries' none route preserved-surface \
    not-applicable skipped skipped skipped skipped continue none; then
  pass "BUG-032 Check 8B unpunctuated negative fixture imposes no Consumer Impact Sweep requirement"
else
  fail "BUG-032 Check 8B unpunctuated negative guard record must be closed, ordered, and exact (status=$BUG032_CHECK8B_GUARD_STATUS)"
fi

check8b_direct_guard_failures=0
for check8b_direct_case in active passive; do
  if [[ "$check8b_direct_case" == "active" ]]; then
    check8b_line='Remove the public API route'
  else
    check8b_line='The public API route is removed'
  fi
  bug032_run_check8b_guard_case "direct-$check8b_direct_case" "$check8b_line"
  check8b_direct_correction_for remove:route
  if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
    || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
      scopes.md direct-positive route remove:route none direct not-applicable \
      run missing missing missing blocked "$CHECK8B_EXPECTED_CORRECTION"; then
    check8b_direct_guard_failures=$((check8b_direct_guard_failures + 1))
    echo "BUG032_SCN013_DIRECT_GUARD_RECORD_MISMATCH case=$check8b_direct_case status=$BUG032_CHECK8B_GUARD_STATUS"
  fi
done
if [[ "$check8b_direct_guard_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B active and passive public-route removals invoke all three impact-planning checks"
else
  fail "BUG-032 Check 8B direct guard matrix has $check8b_direct_guard_failures impact-planning mismatch(es)"
fi

bug032_run_check8b_guard_case "mixed-surface" "$check8b_scn013_mixed"
check8b_direct_correction_for remove:redirect
if [[ "$BUG032_CHECK8B_GUARD_STATUS" -ne 0 ]] \
  && check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
    scopes.md mixed-surface redirect remove:redirect route mixed not-applicable \
    run missing missing missing blocked "$CHECK8B_EXPECTED_CORRECTION"; then
  pass "BUG-032 Check 8B mixed-surface guard path evaluates impact planning only for the removed redirect"
else
  fail "BUG-032 Check 8B mixed-surface guard record did not isolate redirect impact (status=$BUG032_CHECK8B_GUARD_STATUS)"
fi

check8b_ambiguous_guard_failures=0
for check8b_ambiguous_case in conflict unresolved token-limit; do
  case "$check8b_ambiguous_case" in
    conflict)
      check8b_line="$check8b_scn013_conflict"
      check8b_expected_preserved=route
      check8b_expected_boundary=not-applicable
      check8b_expected_correction='Rewrite only this declaration so route is either removed or preserved, not both.'
      ;;
    unresolved)
      check8b_line="$check8b_scn013_unresolved"
      check8b_expected_preserved=none
      check8b_expected_boundary=not-applicable
      check8b_expected_correction='Rewrite only this declaration to state whether the public API route changes or remains unchanged.'
      ;;
    token-limit)
      check8b_line="$check8b_scn013_token_limit"
      check8b_expected_preserved=none
      check8b_expected_boundary='tokens=129/128:first-overflow'
      check8b_expected_correction='Split only this declaration before token 129 while preserving its meaning.'
      ;;
  esac
  bug032_run_check8b_guard_case "ambiguous-$check8b_ambiguous_case" "$check8b_line"
  if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
    || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
      scopes.md ambiguous unresolved none "$check8b_expected_preserved" \
      "$check8b_ambiguous_case" "$check8b_expected_boundary" \
      skipped skipped skipped skipped blocked "$check8b_expected_correction"; then
    check8b_ambiguous_guard_failures=$((check8b_ambiguous_guard_failures + 1))
    echo "BUG032_SCN013_AMBIGUOUS_GUARD_RECORD_MISMATCH case=$check8b_ambiguous_case status=$BUG032_CHECK8B_GUARD_STATUS"
  fi
done
if [[ "$check8b_ambiguous_guard_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B ambiguous guard paths fail closed with reason-specific local rewrite guidance"
else
  fail "BUG-032 Check 8B ambiguous guard matrix has $check8b_ambiguous_guard_failures mismatch(es)"
fi

# BUG032-CR-01 / SCN-032-014: negation belongs to its local phrase. An
# unrelated leading `no` clause or `without` adjunct must not suppress a later
# active or passive route removal. The owned-negation twins prevent a broad
# "ignore negation" implementation from satisfying the regression.
check8b_negation_direct_lines=(
  'No cache migration is required; remove the public route.'
  'No cache migration is required. The public route is removed.'
  'Without migration remove the public route.'
  'The public route without migration is removed.'
)
check8b_owned_negation_lines=(
  'Do not remove the public route.'
  'The public route is not removed.'
  'Without removing the public route, purge stale cache entries.'
  'No public route is removed.'
)
check8b_negation_direct_token_counts=(9 10 6 7)
check8b_owned_negation_token_counts=(6 6 9 5)

check8b_negation_direct_failures=0
for check8b_index in "${!check8b_negation_direct_lines[@]}"; do
  check8b_line="${check8b_negation_direct_lines[$check8b_index]}"
  bug032_check8b_classify "$check8b_line"
  if ! check8b_helper_record_matches \
    0 direct-positive remove route remove:route none direct none \
    not-applicable "${check8b_negation_direct_token_counts[$check8b_index]}" 1 \
    "${check8b_negation_direct_token_counts[$check8b_index]}" \
    "${check8b_negation_direct_token_counts[$check8b_index]}" 1; then
    check8b_negation_direct_failures=$((check8b_negation_direct_failures + 1))
    echo "BUG032_CR01_DIRECT_MISMATCH index=$check8b_index classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON mutationTarget=$CHECK8B_MUTATION_TARGET directSurfaces=$CHECK8B_DIRECT_SURFACES preservedSurfaces=$CHECK8B_PRESERVED_SURFACES line=$check8b_line"
  fi

  bug032_run_check8b_guard_case "phrase-local-direct-$check8b_index" "$check8b_line"
  check8b_direct_correction_for remove:route
  if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
    || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
      scopes.md direct-positive route remove:route none direct not-applicable \
      run missing missing missing blocked "$CHECK8B_EXPECTED_CORRECTION"; then
    check8b_negation_direct_failures=$((check8b_negation_direct_failures + 1))
    echo "BUG032_CR01_DIRECT_GUARD_RECORD_MISMATCH index=$check8b_index status=$BUG032_CHECK8B_GUARD_STATUS line=$check8b_line"
  fi
done
if [[ "$check8b_negation_direct_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B keeps direct mutations positive after unrelated no or without phrases"
else
  fail "BUG-032 Check 8B unrelated-negation direct matrix has $check8b_negation_direct_failures mismatch(es)"
fi

check8b_owned_negation_failures=0
for check8b_index in "${!check8b_owned_negation_lines[@]}"; do
  check8b_line="${check8b_owned_negation_lines[$check8b_index]}"
  bug032_check8b_classify "$check8b_line"
  if ! check8b_helper_record_matches \
    0 negative remove route none route preserved-surface none \
    not-applicable "${check8b_owned_negation_token_counts[$check8b_index]}" 1 \
    "${check8b_owned_negation_token_counts[$check8b_index]}" \
    "${check8b_owned_negation_token_counts[$check8b_index]}" 1; then
    check8b_owned_negation_failures=$((check8b_owned_negation_failures + 1))
    echo "BUG032_CR01_OWNED_MISMATCH index=$check8b_index classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON mutationTarget=$CHECK8B_MUTATION_TARGET directSurfaces=$CHECK8B_DIRECT_SURFACES preservedSurfaces=$CHECK8B_PRESERVED_SURFACES line=$check8b_line"
  fi

  bug032_run_check8b_guard_case "owned-negation-$check8b_index" "$check8b_line"
  if [[ "$BUG032_CHECK8B_GUARD_STATUS" -ne 0 ]] \
    || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
      scopes.md negative route none route preserved-surface not-applicable \
      skipped skipped skipped skipped continue none; then
    check8b_owned_negation_failures=$((check8b_owned_negation_failures + 1))
    echo "BUG032_CR01_OWNED_GUARD_RECORD_MISMATCH index=$check8b_index status=$BUG032_CHECK8B_GUARD_STATUS line=$check8b_line"
  fi
done
if [[ "$check8b_owned_negation_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B keeps owned negation negative"
else
  fail "BUG-032 Check 8B owned-negation matrix has $check8b_owned_negation_failures mismatch(es)"
fi

# BUG032-CR-02 / SCN-032-015: a trailing ordinary artifact noun leaves the
# mutation object unresolved. Each ambiguity has both a direct twin and a
# separate-clause preservation twin, and all three forms drive the real guard.
check8b_tail_lines=(
  'Remove the public API route example.'
  'Remove the endpoint test.'
  'Remove the contract fixture.'
  'Remove the link documentation.'
)
check8b_tail_direct_lines=(
  'Remove the public API route.'
  'Remove the endpoint.'
  'Remove the contract.'
  'Remove the link.'
)
check8b_tail_preserved_lines=(
  'Remove the route example. The public API route remains unchanged.'
  'Remove the endpoint test. The endpoint remains unchanged.'
  'Remove the contract fixture. The contract remains unchanged.'
  'Remove the link documentation. The link remains unchanged.'
)
check8b_tail_surfaces=(route endpoint contract link)
check8b_tail_phrases=('route example' 'endpoint test' 'contract fixture' 'link documentation')
check8b_tail_token_counts=(6 4 4 4)
check8b_tail_direct_token_counts=(5 3 3 3)
check8b_tail_preserved_token_counts=(10 8 8 8)

check8b_tail_ambiguous_failures=0
check8b_tail_twin_failures=0
for check8b_index in "${!check8b_tail_lines[@]}"; do
  check8b_surface="${check8b_tail_surfaces[$check8b_index]}"
  check8b_phrase="${check8b_tail_phrases[$check8b_index]}"
  check8b_line="${check8b_tail_lines[$check8b_index]}"
  bug032_check8b_classify "$check8b_line"
  if ! check8b_helper_record_matches \
    0 ambiguous remove unresolved none none surface-tail "$check8b_phrase" \
    not-applicable "${check8b_tail_token_counts[$check8b_index]}" 1 \
    "${check8b_tail_token_counts[$check8b_index]}" \
    "${check8b_tail_token_counts[$check8b_index]}" 1; then
    check8b_tail_ambiguous_failures=$((check8b_tail_ambiguous_failures + 1))
    echo "BUG032_CR02_TAIL_MISMATCH index=$check8b_index classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON mutationTarget=$CHECK8B_MUTATION_TARGET unresolvedPhrase=$CHECK8B_UNRESOLVED_PHRASE directSurfaces=$CHECK8B_DIRECT_SURFACES preservedSurfaces=$CHECK8B_PRESERVED_SURFACES boundary=$CHECK8B_BOUNDARY"
  fi
  bug032_run_check8b_guard_case "surface-tail-$check8b_index" "$check8b_line"
  check8b_expected_correction="Rewrite only this declaration to distinguish artifact '$check8b_phrase' from consumer surface '$check8b_surface'."
  if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
    || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
      scopes.md ambiguous unresolved none none surface-tail not-applicable \
      skipped skipped skipped skipped blocked "$check8b_expected_correction"; then
    check8b_tail_ambiguous_failures=$((check8b_tail_ambiguous_failures + 1))
    echo "BUG032_CR02_TAIL_GUARD_RECORD_MISMATCH index=$check8b_index status=$BUG032_CHECK8B_GUARD_STATUS phrase=$check8b_phrase"
  fi

  check8b_line="${check8b_tail_direct_lines[$check8b_index]}"
  bug032_check8b_classify "$check8b_line"
  if ! check8b_helper_record_matches \
    0 direct-positive remove "$check8b_surface" "remove:$check8b_surface" \
    none direct none not-applicable \
    "${check8b_tail_direct_token_counts[$check8b_index]}" 1 \
    "${check8b_tail_direct_token_counts[$check8b_index]}" \
    "${check8b_tail_direct_token_counts[$check8b_index]}" 1; then
    check8b_tail_twin_failures=$((check8b_tail_twin_failures + 1))
    echo "BUG032_CR02_DIRECT_TWIN_MISMATCH index=$check8b_index classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON mutationTarget=$CHECK8B_MUTATION_TARGET directSurfaces=$CHECK8B_DIRECT_SURFACES preservedSurfaces=$CHECK8B_PRESERVED_SURFACES"
  fi
  bug032_run_check8b_guard_case "surface-tail-direct-$check8b_index" "$check8b_line"
  check8b_direct_correction_for "remove:$check8b_surface"
  if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
    || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
      scopes.md direct-positive "$check8b_surface" "remove:$check8b_surface" \
      none direct not-applicable run missing missing missing blocked \
      "$CHECK8B_EXPECTED_CORRECTION"; then
    check8b_tail_twin_failures=$((check8b_tail_twin_failures + 1))
    echo "BUG032_CR02_DIRECT_TWIN_GUARD_RECORD_MISMATCH index=$check8b_index status=$BUG032_CHECK8B_GUARD_STATUS"
  fi

  check8b_line="${check8b_tail_preserved_lines[$check8b_index]}"
  bug032_check8b_classify "$check8b_line"
  if ! check8b_helper_record_matches \
    0 negative remove "$check8b_phrase" none "$check8b_surface" \
    preserved-surface none not-applicable \
    "${check8b_tail_preserved_token_counts[$check8b_index]}" 1 \
    "${check8b_tail_preserved_token_counts[$check8b_index]}" \
    "${check8b_tail_preserved_token_counts[$check8b_index]}" 1; then
    check8b_tail_twin_failures=$((check8b_tail_twin_failures + 1))
    echo "BUG032_CR02_PRESERVED_TWIN_MISMATCH index=$check8b_index classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON mutationTarget=$CHECK8B_MUTATION_TARGET directSurfaces=$CHECK8B_DIRECT_SURFACES preservedSurfaces=$CHECK8B_PRESERVED_SURFACES boundary=$CHECK8B_BOUNDARY"
  fi
  bug032_run_check8b_guard_case "surface-tail-preserved-$check8b_index" "$check8b_line"
  if [[ "$BUG032_CHECK8B_GUARD_STATUS" -ne 0 ]] \
    || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
      scopes.md negative "$check8b_phrase" none "$check8b_surface" \
      preserved-surface not-applicable skipped skipped skipped skipped continue none; then
    check8b_tail_twin_failures=$((check8b_tail_twin_failures + 1))
    echo "BUG032_CR02_PRESERVED_TWIN_GUARD_RECORD_MISMATCH index=$check8b_index status=$BUG032_CHECK8B_GUARD_STATUS"
  fi
done
if [[ "$check8b_tail_ambiguous_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B blocks trailing artifact nouns as surface-tail"
else
  fail "BUG-032 Check 8B trailing-artifact matrix has $check8b_tail_ambiguous_failures ambiguity mismatch(es)"
fi
if [[ "$check8b_tail_twin_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B discriminates direct and explicitly preserved trailing-noun twins"
else
  fail "BUG-032 Check 8B trailing-noun twin matrix has $check8b_tail_twin_failures mismatch(es)"
fi

# BUG032-HARDEN9-C8B-PASSIVE-OBJECT-001 / SCN-032-013: a passive
# mutation must own its actual object. A route used before an unrelated cache
# removal is preserved context, not the object being removed. The direct
# passive route control prevents a broad passive-form exemption.
check8b_scn013_passive_object='The public API route is used while stale cache entries are removed.'
check8b_scn013_passive_direct_control='The public API route is removed.'
check8b_scn013_passive_behavior_failures=0
bug032_check8b_classify "$check8b_scn013_passive_object"
if ! check8b_helper_record_matches \
  0 negative remove 'stale cache entries' none route preserved-surface none \
  not-applicable 12 1 12 12 1; then
  check8b_scn013_passive_behavior_failures=$((check8b_scn013_passive_behavior_failures + 1))
  printf 'BUG032_SCN013_PASSIVE_OBJECT_HELPER_MISMATCH status=%s classification=%s target=%s direct=%s preserved=%s reason=%s\n' \
    "$CHECK8B_LAST_STATUS" "$CHECK8B_CLASSIFICATION" \
    "$CHECK8B_MUTATION_TARGET" "$CHECK8B_DIRECT_SURFACES" \
    "$CHECK8B_PRESERVED_SURFACES" "$CHECK8B_REASON"
fi
bug032_run_check8b_guard_case "scn013-passive-object" "$check8b_scn013_passive_object"
if [[ "$BUG032_CHECK8B_GUARD_STATUS" -ne 0 ]] \
  || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
    scopes.md negative 'stale cache entries' none route preserved-surface \
    not-applicable skipped skipped skipped skipped continue none; then
  check8b_scn013_passive_behavior_failures=$((check8b_scn013_passive_behavior_failures + 1))
  printf 'BUG032_SCN013_PASSIVE_OBJECT_GUARD_MISMATCH status=%s\n' \
    "$BUG032_CHECK8B_GUARD_STATUS"
fi
if [[ "$check8b_scn013_passive_behavior_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B keeps a used passive surface separate from an unrelated passive object"
else
  fail "BUG-032 Check 8B passive-object behavior matrix has $check8b_scn013_passive_behavior_failures mismatch(es)"
fi

check8b_scn013_passive_control_failures=0
bug032_check8b_classify "$check8b_scn013_passive_direct_control"
if ! check8b_helper_record_matches \
  0 direct-positive remove route remove:route none direct none \
  not-applicable 6 1 6 6 1; then
  check8b_scn013_passive_control_failures=$((check8b_scn013_passive_control_failures + 1))
  printf 'BUG032_SCN013_PASSIVE_DIRECT_CONTROL_HELPER_MISMATCH classification=%s target=%s direct=%s reason=%s\n' \
    "$CHECK8B_CLASSIFICATION" "$CHECK8B_MUTATION_TARGET" \
    "$CHECK8B_DIRECT_SURFACES" "$CHECK8B_REASON"
fi
bug032_run_check8b_guard_case "scn013-passive-direct-control" "$check8b_scn013_passive_direct_control"
check8b_direct_correction_for remove:route
if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
  || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
    scopes.md direct-positive route remove:route none direct not-applicable \
    run missing missing missing blocked "$CHECK8B_EXPECTED_CORRECTION"; then
  check8b_scn013_passive_control_failures=$((check8b_scn013_passive_control_failures + 1))
  printf 'BUG032_SCN013_PASSIVE_DIRECT_CONTROL_GUARD_MISMATCH status=%s\n' \
    "$BUG032_CHECK8B_GUARD_STATUS"
fi
if [[ "$check8b_scn013_passive_control_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B passive direct-route control remains direct-positive"
else
  fail "BUG-032 Check 8B passive direct-route control matrix has $check8b_scn013_passive_control_failures mismatch(es)"
fi

# BUG032-HARDEN9-C8B-PASSIVE-TAIL-002 / SCN-032-015: passive tails
# carry the same unresolved/direct/explicitly-clarified three-way contract as
# active tails. Every form runs through both the sourced production helper and
# the real guard consumer.
check8b_scn015_passive_unresolved_lines=(
  'The endpoint test is removed.'
  'The route example is removed.'
)
check8b_scn015_passive_direct_lines=(
  'The endpoint is removed.'
  'The public route is removed.'
)
check8b_scn015_passive_clarified_lines=(
  'The endpoint test is removed. The public endpoint remains unchanged.'
  'The route example is removed. The public route remains unchanged.'
)
check8b_scn015_passive_surfaces=(endpoint route)
check8b_scn015_passive_phrases=('endpoint test' 'route example')
check8b_scn015_passive_direct_token_counts=(4 5)
check8b_scn015_passive_behavior_failures=0
check8b_scn015_passive_direct_failures=0
for check8b_index in "${!check8b_scn015_passive_unresolved_lines[@]}"; do
  check8b_surface="${check8b_scn015_passive_surfaces[$check8b_index]}"
  check8b_phrase="${check8b_scn015_passive_phrases[$check8b_index]}"

  check8b_line="${check8b_scn015_passive_unresolved_lines[$check8b_index]}"
  bug032_check8b_classify "$check8b_line"
  if ! check8b_helper_record_matches \
    0 ambiguous remove unresolved none none surface-tail "$check8b_phrase" \
    not-applicable 5 1 5 5 1; then
    check8b_scn015_passive_behavior_failures=$((check8b_scn015_passive_behavior_failures + 1))
    printf 'BUG032_SCN015_PASSIVE_UNRESOLVED_HELPER_MISMATCH index=%s classification=%s target=%s direct=%s preserved=%s reason=%s unresolved=%s\n' \
      "$check8b_index" "$CHECK8B_CLASSIFICATION" "$CHECK8B_MUTATION_TARGET" \
      "$CHECK8B_DIRECT_SURFACES" "$CHECK8B_PRESERVED_SURFACES" \
      "$CHECK8B_REASON" "$CHECK8B_UNRESOLVED_PHRASE"
  fi
  bug032_run_check8b_guard_case "scn015-passive-unresolved-$check8b_index" "$check8b_line"
  check8b_expected_correction="Rewrite only this declaration to distinguish artifact '$check8b_phrase' from consumer surface '$check8b_surface'."
  if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
    || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
      scopes.md ambiguous unresolved none none surface-tail not-applicable \
      skipped skipped skipped skipped blocked "$check8b_expected_correction"; then
    check8b_scn015_passive_behavior_failures=$((check8b_scn015_passive_behavior_failures + 1))
    printf 'BUG032_SCN015_PASSIVE_UNRESOLVED_GUARD_MISMATCH index=%s status=%s\n' \
      "$check8b_index" "$BUG032_CHECK8B_GUARD_STATUS"
  fi

  check8b_line="${check8b_scn015_passive_clarified_lines[$check8b_index]}"
  bug032_check8b_classify "$check8b_line"
  if ! check8b_helper_record_matches \
    0 negative remove "$check8b_phrase" none "$check8b_surface" \
    preserved-surface none not-applicable 10 1 10 10 1; then
    check8b_scn015_passive_behavior_failures=$((check8b_scn015_passive_behavior_failures + 1))
    printf 'BUG032_SCN015_PASSIVE_CLARIFIED_HELPER_MISMATCH index=%s classification=%s target=%s direct=%s preserved=%s reason=%s\n' \
      "$check8b_index" "$CHECK8B_CLASSIFICATION" "$CHECK8B_MUTATION_TARGET" \
      "$CHECK8B_DIRECT_SURFACES" "$CHECK8B_PRESERVED_SURFACES" "$CHECK8B_REASON"
  fi
  bug032_run_check8b_guard_case "scn015-passive-clarified-$check8b_index" "$check8b_line"
  if [[ "$BUG032_CHECK8B_GUARD_STATUS" -ne 0 ]] \
    || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
      scopes.md negative "$check8b_phrase" none "$check8b_surface" \
      preserved-surface not-applicable skipped skipped skipped skipped continue none; then
    check8b_scn015_passive_behavior_failures=$((check8b_scn015_passive_behavior_failures + 1))
    printf 'BUG032_SCN015_PASSIVE_CLARIFIED_GUARD_MISMATCH index=%s status=%s\n' \
      "$check8b_index" "$BUG032_CHECK8B_GUARD_STATUS"
  fi

  check8b_line="${check8b_scn015_passive_direct_lines[$check8b_index]}"
  bug032_check8b_classify "$check8b_line"
  if ! check8b_helper_record_matches \
    0 direct-positive remove "$check8b_surface" "remove:$check8b_surface" \
    none direct none not-applicable \
    "${check8b_scn015_passive_direct_token_counts[$check8b_index]}" 1 \
    "${check8b_scn015_passive_direct_token_counts[$check8b_index]}" \
    "${check8b_scn015_passive_direct_token_counts[$check8b_index]}" 1; then
    check8b_scn015_passive_direct_failures=$((check8b_scn015_passive_direct_failures + 1))
    printf 'BUG032_SCN015_PASSIVE_DIRECT_HELPER_MISMATCH index=%s classification=%s target=%s direct=%s reason=%s\n' \
      "$check8b_index" "$CHECK8B_CLASSIFICATION" "$CHECK8B_MUTATION_TARGET" \
      "$CHECK8B_DIRECT_SURFACES" "$CHECK8B_REASON"
  fi
  bug032_run_check8b_guard_case "scn015-passive-direct-$check8b_index" "$check8b_line"
  check8b_direct_correction_for "remove:$check8b_surface"
  if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
    || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
      scopes.md direct-positive "$check8b_surface" "remove:$check8b_surface" \
      none direct not-applicable run missing missing missing blocked \
      "$CHECK8B_EXPECTED_CORRECTION"; then
    check8b_scn015_passive_direct_failures=$((check8b_scn015_passive_direct_failures + 1))
    printf 'BUG032_SCN015_PASSIVE_DIRECT_GUARD_MISMATCH index=%s status=%s\n' \
      "$check8b_index" "$BUG032_CHECK8B_GUARD_STATUS"
  fi
done
if [[ "$check8b_scn015_passive_behavior_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B blocks passive artifact tails and accepts explicit passive clarification"
else
  fail "BUG-032 Check 8B passive artifact-tail matrix has $check8b_scn015_passive_behavior_failures mismatch(es)"
fi
if [[ "$check8b_scn015_passive_direct_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B passive tail direct controls remain positive"
else
  fail "BUG-032 Check 8B passive artifact-tail direct-control matrix has $check8b_scn015_passive_direct_failures mismatch(es)"
fi

# BUG032-HARDEN9-C8B-PASSIVE-NEGATION-003 / SCN-032-014: the two-token
# auxiliary `will be` remains direct, while `will not be` owns the negation.
check8b_scn014_passive_will_not='The public route will not be removed.'
check8b_scn014_passive_will_be='The public route will be removed.'
check8b_scn014_passive_negative_failures=0
bug032_check8b_classify "$check8b_scn014_passive_will_not"
if ! check8b_helper_record_matches \
  0 negative remove route none route preserved-surface none \
  not-applicable 7 1 7 7 1; then
  check8b_scn014_passive_negative_failures=$((check8b_scn014_passive_negative_failures + 1))
  printf 'BUG032_SCN014_PASSIVE_WILL_NOT_HELPER_MISMATCH classification=%s target=%s direct=%s preserved=%s reason=%s\n' \
    "$CHECK8B_CLASSIFICATION" "$CHECK8B_MUTATION_TARGET" \
    "$CHECK8B_DIRECT_SURFACES" "$CHECK8B_PRESERVED_SURFACES" "$CHECK8B_REASON"
fi
bug032_run_check8b_guard_case "scn014-passive-will-not" "$check8b_scn014_passive_will_not"
if [[ "$BUG032_CHECK8B_GUARD_STATUS" -ne 0 ]] \
  || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
    scopes.md negative route none route preserved-surface not-applicable \
    skipped skipped skipped skipped continue none; then
  check8b_scn014_passive_negative_failures=$((check8b_scn014_passive_negative_failures + 1))
  printf 'BUG032_SCN014_PASSIVE_WILL_NOT_GUARD_MISMATCH status=%s\n' \
    "$BUG032_CHECK8B_GUARD_STATUS"
fi
if [[ "$check8b_scn014_passive_negative_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B preserves passive will-not negation"
else
  fail "BUG-032 Check 8B passive will-not negation matrix has $check8b_scn014_passive_negative_failures mismatch(es)"
fi

check8b_scn014_passive_positive_failures=0
bug032_check8b_classify "$check8b_scn014_passive_will_be"
if ! check8b_helper_record_matches \
  0 direct-positive remove route remove:route none direct none \
  not-applicable 6 1 6 6 1; then
  check8b_scn014_passive_positive_failures=$((check8b_scn014_passive_positive_failures + 1))
  printf 'BUG032_SCN014_PASSIVE_WILL_BE_HELPER_MISMATCH classification=%s target=%s direct=%s reason=%s\n' \
    "$CHECK8B_CLASSIFICATION" "$CHECK8B_MUTATION_TARGET" \
    "$CHECK8B_DIRECT_SURFACES" "$CHECK8B_REASON"
fi
bug032_run_check8b_guard_case "scn014-passive-will-be" "$check8b_scn014_passive_will_be"
check8b_direct_correction_for remove:route
if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
  || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
    scopes.md direct-positive route remove:route none direct not-applicable \
    run missing missing missing blocked "$CHECK8B_EXPECTED_CORRECTION"; then
  check8b_scn014_passive_positive_failures=$((check8b_scn014_passive_positive_failures + 1))
  printf 'BUG032_SCN014_PASSIVE_WILL_BE_GUARD_MISMATCH status=%s\n' \
    "$BUG032_CHECK8B_GUARD_STATUS"
fi
if [[ "$check8b_scn014_passive_positive_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B passive will-be control remains direct-positive"
else
  fail "BUG-032 Check 8B passive will-be direct-control matrix has $check8b_scn014_passive_positive_failures mismatch(es)"
fi

# BUG032-HARDEN9-C8B-CONTEXT-004 / SCN-032-020: the complete scope
# consumer, rather than the line helper in isolation, owns active-vs-fixture
# context. The same mutation sentence appears in each ignored context and in an
# active declaration that must still block as surface-tail.
check8b_scn020_contexts=(gherkin examples test-plan)
check8b_scn020_context_failures=0
for check8b_scn020_context in "${check8b_scn020_contexts[@]}"; do
  check8b_scn020_dir="$tmp_root/specs/955-bug032-scn020-$check8b_scn020_context"
  cp -R "$positive_feature_dir" "$check8b_scn020_dir"
  case "$check8b_scn020_context" in
    gherkin)
      cat <<'EOF' >> "$check8b_scn020_dir/scopes.md"

### Fixture Gherkin

```gherkin
Given fixture prose says "Remove the endpoint test."
```
EOF
      ;;
    examples)
      cat <<'EOF' >> "$check8b_scn020_dir/scopes.md"

### Examples

| declaration |
| --- |
| Remove the endpoint test. |
EOF
      ;;
    test-plan)
      cat <<'EOF' >> "$check8b_scn020_dir/scopes.md"

### Test Plan

| Test Type | Description | Expected Result |
| --- | --- | --- |
| Functional fixture | Remove the endpoint test. | The fixture remains inert. |
EOF
      ;;
  esac
  check8b_scn020_log="$tmp_root/bug032-scn020-$check8b_scn020_context.log"
  check8b_scn020_status="$(run_capture "$check8b_scn020_log" bash "$GUARD_SCRIPT" "$check8b_scn020_dir")"
  if [[ "$check8b_scn020_status" -ne 0 ]] \
    || grep -Fq -- 'check: Check 8B' "$check8b_scn020_log" \
    || grep -Fq -- 'Check 8B blocked an ambiguous declaration' "$check8b_scn020_log"; then
    check8b_scn020_context_failures=$((check8b_scn020_context_failures + 1))
    printf 'BUG032_SCN020_CONTEXT_MISMATCH context=%s status=%s check8bRecords=%s ambiguityBlocks=%s\n' \
      "$check8b_scn020_context" "$check8b_scn020_status" \
      "$(grep -cF -- 'check: Check 8B' "$check8b_scn020_log" || true)" \
      "$(grep -cF -- 'Check 8B blocked an ambiguous declaration' "$check8b_scn020_log" || true)"
  fi
done
if [[ "$check8b_scn020_context_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B ignores Gherkin Examples and Test Plan fixture prose"
else
  fail "BUG-032 Check 8B fixture-context matrix has $check8b_scn020_context_failures mismatch(es)"
fi

bug032_run_check8b_guard_case "scn020-active-control" 'Remove the endpoint test.'
if [[ "$BUG032_CHECK8B_GUARD_STATUS" -ne 0 ]] \
  && check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
    scopes.md ambiguous unresolved none none surface-tail not-applicable \
    skipped skipped skipped skipped blocked \
    "Rewrite only this declaration to distinguish artifact 'endpoint test' from consumer surface 'endpoint'."; then
  pass "BUG-032 Check 8B still evaluates identical active declarations"
else
  fail "BUG-032 Check 8B active fixture twin has unexpected status=$BUG032_CHECK8B_GUARD_STATUS"
fi

# BUG032-CR-03 / SCN-032-016: construct the exact 128/129-token pair without
# an external generator. In the overflow case `route` is token 129 and must not
# be retained or inspected.
check8b_token_neutral_126=""
for ((check8b_index = 1; check8b_index <= 126; check8b_index++)); do
  check8b_token_neutral_126="${check8b_token_neutral_126}${check8b_token_neutral_126:+ }neutral$check8b_index"
done
check8b_token_128="$check8b_token_neutral_126 remove route"
check8b_token_129="overflow129 $check8b_token_neutral_126 remove route"

# DEBUG tracing observes entry into production helper functions without copying
# or replacing their grammar. The role registry is deliberately closed over
# every declaration in the marked block. Roles come from declaration bodies,
# so irrelevant renames or inlining do not stale a copied name table. A new
# body with no recognized role fails closed instead of becoming a vacuous pass.
declare -A check8b_production_functions=()
declare -A check8b_trace_roles=()
declare -A check8b_trace_observed_declarations=()

check8b_trace_role_from_body() {
  local function_name="$1"
  local function_body="$2"
  local semantic_body=""
  local closed_result_name=""
  local closed_result_assignments=0
  local candidate_coordination=0
  local relationship_coordination=0

  CHECK8B_TRACE_DERIVED_ROLE="unknown"
  CHECK8B_TRACE_ENTRYPOINT_ASSIGNMENT_COUNT=0
  CHECK8B_TRACE_ENTRYPOINT_CANDIDATE_COORDINATION=0
  CHECK8B_TRACE_ENTRYPOINT_RELATIONSHIP_COORDINATION=0
  if ! check8b_trace_strip_inert_text "$function_body"; then
    return 1
  fi
  semantic_body="$CHECK8B_TRACE_EXECUTABLE_TEXT"
  if ! check8b_trace_split_command_list "$semantic_body"; then
    return 1
  fi

  if [[ "$function_name" == "check8b_classify_line" ]]; then
    for closed_result_name in \
      CHECK8B_CLASSIFICATION CHECK8B_VERB CHECK8B_MUTATION_TARGET \
      CHECK8B_DIRECT_SURFACES CHECK8B_PRESERVED_SURFACES CHECK8B_REASON \
      CHECK8B_UNRESOLVED_PHRASE CHECK8B_BOUNDARY CHECK8B_TOKEN_COUNT \
      CHECK8B_CANDIDATE_COUNT _CHECK8B_TOKENS _CHECK8B_CANDIDATE_INDEXES; do
      if check8b_trace_has_executable_assignment "$semantic_body" "$closed_result_name"; then
        closed_result_assignments=$((closed_result_assignments + 1))
      fi
    done
    check8b_trace_has_executable_call "$semantic_body" "_check8b_mutation_verb" \
      && candidate_coordination=1
    check8b_trace_has_executable_relationship_call "$semantic_body" \
      && relationship_coordination=1
    CHECK8B_TRACE_ENTRYPOINT_ASSIGNMENT_COUNT="$closed_result_assignments"
    CHECK8B_TRACE_ENTRYPOINT_CANDIDATE_COORDINATION="$candidate_coordination"
    CHECK8B_TRACE_ENTRYPOINT_RELATIONSHIP_COORDINATION="$relationship_coordination"
    if [[ "$closed_result_assignments" -eq 12 ]] \
      && [[ "$candidate_coordination" -eq 1 ]] \
      && [[ "$relationship_coordination" -eq 1 ]]; then
      CHECK8B_TRACE_DERIVED_ROLE="entrypoint"
    fi
  elif [[ "$function_name" != _check8b_* ]]; then
    CHECK8B_TRACE_DERIVED_ROLE="unknown"
  elif check8b_trace_has_executable_assignment "$semantic_body" "_CHECK8B_WORD_RESULT"; then
    CHECK8B_TRACE_DERIVED_ROLE="candidate-semantic"
  elif check8b_trace_has_executable_assignment "$semantic_body" "_CHECK8B_SURFACE_RESULT" \
    || check8b_trace_has_executable_assignment "$semantic_body" "_CHECK8B_SURFACE_INDEX" \
    || check8b_trace_has_executable_assignment "$semantic_body" "_CHECK8B_REFERENCE_RESULT" \
    || check8b_trace_has_executable_assignment "$semantic_body" "_CHECK8B_TARGET_RESULT" \
    || check8b_trace_has_executable_relationship_call "$semantic_body"; then
    CHECK8B_TRACE_DERIVED_ROLE="relationship"
  elif check8b_trace_has_executable_assignment "$semantic_body" "_CHECK8B_CSV_RESULT" \
    || check8b_trace_has_executable_call "$semantic_body" "case"; then
    CHECK8B_TRACE_DERIVED_ROLE="semantic-support"
  fi
}

check8b_trace_register_function_role() {
  local function_name="$1"
  local function_body="$2"
  local role=""

  if ! check8b_trace_role_from_body "$function_name" "$function_body"; then
    CHECK8B_TRACE_ROLE_CONTRACT_FAILURES=$((CHECK8B_TRACE_ROLE_CONTRACT_FAILURES + 1))
    CHECK8B_TRACE_ROLE_UNKNOWN_NAMES="${CHECK8B_TRACE_ROLE_UNKNOWN_NAMES}${CHECK8B_TRACE_ROLE_UNKNOWN_NAMES:+,}lexical:$function_name"
    return 1
  fi
  role="$CHECK8B_TRACE_DERIVED_ROLE"
  check8b_trace_roles["$function_name"]="$role"
  case "$role" in
    candidate-semantic) CHECK8B_TRACE_ROLE_CANDIDATE_DECLARATIONS=$((CHECK8B_TRACE_ROLE_CANDIDATE_DECLARATIONS + 1)) ;;
    semantic-support) CHECK8B_TRACE_ROLE_SEMANTIC_SUPPORT_DECLARATIONS=$((CHECK8B_TRACE_ROLE_SEMANTIC_SUPPORT_DECLARATIONS + 1)) ;;
    relationship) CHECK8B_TRACE_ROLE_RELATIONSHIP_DECLARATIONS=$((CHECK8B_TRACE_ROLE_RELATIONSHIP_DECLARATIONS + 1)) ;;
    entrypoint) CHECK8B_TRACE_ROLE_ENTRYPOINT_DECLARATIONS=$((CHECK8B_TRACE_ROLE_ENTRYPOINT_DECLARATIONS + 1)) ;;
    unknown)
      CHECK8B_TRACE_ROLE_CONTRACT_FAILURES=$((CHECK8B_TRACE_ROLE_CONTRACT_FAILURES + 1))
      CHECK8B_TRACE_ROLE_UNKNOWN_NAMES="${CHECK8B_TRACE_ROLE_UNKNOWN_NAMES}${CHECK8B_TRACE_ROLE_UNKNOWN_NAMES:+,}$function_name"
      ;;
  esac
  return 0
}

check8b_trace_role_contract_build() {
  local classifier_file="$1"
  local source_line=""
  local function_name=""
  local function_body=""
  local declaration_tail=""
  local active_function=""
  local function_brace_depth=0

  CHECK8B_TRACE_ROLE_CONTRACT_FAILURES=0
  CHECK8B_TRACE_ROLE_UNKNOWN_NAMES=""
  CHECK8B_TRACE_ROLE_MISSING_NAMES=""
  CHECK8B_TRACE_STYLE_NAME_PARENS=0
  CHECK8B_TRACE_STYLE_FUNCTION_NAME=0
  CHECK8B_TRACE_STYLE_FUNCTION_NAME_PARENS=0
  CHECK8B_TRACE_ROLE_CANDIDATE_DECLARATIONS=0
  CHECK8B_TRACE_ROLE_SEMANTIC_SUPPORT_DECLARATIONS=0
  CHECK8B_TRACE_ROLE_RELATIONSHIP_DECLARATIONS=0
  CHECK8B_TRACE_ROLE_ENTRYPOINT_DECLARATIONS=0
  check8b_production_functions=()
  check8b_trace_roles=()
  check8b_trace_observed_declarations=()

  while IFS= read -r source_line || [[ -n "$source_line" ]]; do
    if [[ -z "$active_function" ]]; then
      if ! check8b_parse_function_declaration "$source_line"; then
        continue
      fi
      function_name="$CHECK8B_DECLARATION_NAME"
      declaration_tail="$CHECK8B_DECLARATION_REMAINDER"
      case "$CHECK8B_DECLARATION_STYLE" in
        name-parens) CHECK8B_TRACE_STYLE_NAME_PARENS=$((CHECK8B_TRACE_STYLE_NAME_PARENS + 1)) ;;
        function-name) CHECK8B_TRACE_STYLE_FUNCTION_NAME=$((CHECK8B_TRACE_STYLE_FUNCTION_NAME + 1)) ;;
        function-name-parens) CHECK8B_TRACE_STYLE_FUNCTION_NAME_PARENS=$((CHECK8B_TRACE_STYLE_FUNCTION_NAME_PARENS + 1)) ;;
        *) CHECK8B_TRACE_ROLE_CONTRACT_FAILURES=$((CHECK8B_TRACE_ROLE_CONTRACT_FAILURES + 1)) ;;
      esac
      if [[ -n "${check8b_trace_observed_declarations[$function_name]:-}" ]]; then
        CHECK8B_TRACE_ROLE_CONTRACT_FAILURES=$((CHECK8B_TRACE_ROLE_CONTRACT_FAILURES + 1))
        CHECK8B_TRACE_ROLE_UNKNOWN_NAMES="${CHECK8B_TRACE_ROLE_UNKNOWN_NAMES}${CHECK8B_TRACE_ROLE_UNKNOWN_NAMES:+,}duplicate:$function_name"
        continue
      fi
      check8b_production_functions["$function_name"]=1
      check8b_trace_observed_declarations["$function_name"]=1
      function_body="$declaration_tail"
      if ! check8b_function_brace_scan_line "$source_line" 0; then
        CHECK8B_TRACE_ROLE_CONTRACT_FAILURES=$((CHECK8B_TRACE_ROLE_CONTRACT_FAILURES + 1))
        CHECK8B_TRACE_ROLE_UNKNOWN_NAMES="${CHECK8B_TRACE_ROLE_UNKNOWN_NAMES}${CHECK8B_TRACE_ROLE_UNKNOWN_NAMES:+,}brace:$function_name"
        continue
      fi
      function_brace_depth="$CHECK8B_FUNCTION_BRACE_FINAL_DEPTH"
      if [[ "$function_brace_depth" -eq 0 ]]; then
        check8b_trace_register_function_role "$function_name" "$function_body" || true
        function_body=""
      else
        active_function="$function_name"
      fi
      continue
    fi

    if check8b_parse_function_declaration "$source_line"; then
      CHECK8B_TRACE_ROLE_CONTRACT_FAILURES=$((CHECK8B_TRACE_ROLE_CONTRACT_FAILURES + 1))
      CHECK8B_TRACE_ROLE_UNKNOWN_NAMES="${CHECK8B_TRACE_ROLE_UNKNOWN_NAMES}${CHECK8B_TRACE_ROLE_UNKNOWN_NAMES:+,}nested:$CHECK8B_DECLARATION_NAME"
    fi
    function_body="${function_body}${function_body:+$'\n'}$source_line"
    if ! check8b_function_brace_scan_line "$source_line" "$function_brace_depth"; then
      CHECK8B_TRACE_ROLE_CONTRACT_FAILURES=$((CHECK8B_TRACE_ROLE_CONTRACT_FAILURES + 1))
      CHECK8B_TRACE_ROLE_UNKNOWN_NAMES="${CHECK8B_TRACE_ROLE_UNKNOWN_NAMES}${CHECK8B_TRACE_ROLE_UNKNOWN_NAMES:+,}brace:$active_function"
      active_function=""
      function_body=""
      function_brace_depth=0
      continue
    fi
    function_brace_depth="$CHECK8B_FUNCTION_BRACE_FINAL_DEPTH"
    if [[ "$function_brace_depth" -eq 0 ]]; then
      check8b_trace_register_function_role "$active_function" "$function_body" || true
      active_function=""
      function_body=""
      continue
    fi
  done < "$classifier_file"

  if [[ -n "$active_function" ]]; then
    CHECK8B_TRACE_ROLE_CONTRACT_FAILURES=$((CHECK8B_TRACE_ROLE_CONTRACT_FAILURES + 1))
    CHECK8B_TRACE_ROLE_UNKNOWN_NAMES="${CHECK8B_TRACE_ROLE_UNKNOWN_NAMES}${CHECK8B_TRACE_ROLE_UNKNOWN_NAMES:+,}unclosed:$active_function"
  fi
  [[ "$CHECK8B_TRACE_ROLE_CANDIDATE_DECLARATIONS" -ge 1 ]] \
    || CHECK8B_TRACE_ROLE_MISSING_NAMES="${CHECK8B_TRACE_ROLE_MISSING_NAMES}${CHECK8B_TRACE_ROLE_MISSING_NAMES:+,}candidate-semantic"
  [[ "$CHECK8B_TRACE_ROLE_SEMANTIC_SUPPORT_DECLARATIONS" -ge 1 ]] \
    || CHECK8B_TRACE_ROLE_MISSING_NAMES="${CHECK8B_TRACE_ROLE_MISSING_NAMES}${CHECK8B_TRACE_ROLE_MISSING_NAMES:+,}semantic-support"
  [[ "$CHECK8B_TRACE_ROLE_RELATIONSHIP_DECLARATIONS" -ge 1 ]] \
    || CHECK8B_TRACE_ROLE_MISSING_NAMES="${CHECK8B_TRACE_ROLE_MISSING_NAMES}${CHECK8B_TRACE_ROLE_MISSING_NAMES:+,}relationship"
  [[ "$CHECK8B_TRACE_ROLE_ENTRYPOINT_DECLARATIONS" -eq 1 ]] \
    || CHECK8B_TRACE_ROLE_MISSING_NAMES="${CHECK8B_TRACE_ROLE_MISSING_NAMES}${CHECK8B_TRACE_ROLE_MISSING_NAMES:+,}entrypoint:$CHECK8B_TRACE_ROLE_ENTRYPOINT_DECLARATIONS"
  if [[ -n "$CHECK8B_TRACE_ROLE_MISSING_NAMES" ]]; then
    CHECK8B_TRACE_ROLE_CONTRACT_FAILURES=$((CHECK8B_TRACE_ROLE_CONTRACT_FAILURES + 1))
  fi
  [[ "$CHECK8B_TRACE_ROLE_CONTRACT_FAILURES" -eq 0 ]]
}

check8b_trace_role_contract_failures=0
if ! check8b_trace_role_contract_build "$check8b_classifier_file"; then
  check8b_trace_role_contract_failures=$((check8b_trace_role_contract_failures + 1))
  echo "BUG032_TI03_TRACE_ROLE_CONTRACT_INVALID unknown=$CHECK8B_TRACE_ROLE_UNKNOWN_NAMES missing=$CHECK8B_TRACE_ROLE_MISSING_NAMES failures=$CHECK8B_TRACE_ROLE_CONTRACT_FAILURES"
fi

check8b_trace_unknown_mutant="$tmp_root/bug032-check8b-trace-role-unknown-mutant.sh"
cp "$check8b_classifier_file" "$check8b_trace_unknown_mutant"
cat <<'EOF' >> "$check8b_trace_unknown_mutant"
function _check8b_unclassified_probe() {
  # _CHECK8B_WORD_RESULT _CHECK8B_SURFACE_RESULT _CHECK8B_TOKENS[
  :
}
EOF
if check8b_trace_role_contract_build "$check8b_trace_unknown_mutant"; then
  check8b_trace_role_contract_failures=$((check8b_trace_role_contract_failures + 1))
  echo 'BUG032_TI03_UNKNOWN_TRACE_ROLE_ACCEPTED'
else
  pass "BUG-032 Check 8B closed trace-role oracle rejects a declared private helper whose source body has no classified role"
fi

check8b_trace_style_fixture="$tmp_root/bug032-check8b-trace-role-declaration-styles.sh"
cat <<'EOF' > "$check8b_trace_style_fixture"
# BEGIN CHECK8B FINITE CLASSIFIER
_check8b_mutation_verb () {
  _CHECK8B_WORD_RESULT="candidate"
}
function _check8b_is_surface {
  case "$1" in
    *) return 0 ;;
  esac
}
function _check8b_surface_from () {
  {
    _CHECK8B_SURFACE_RESULT="$1"
    _CHECK8B_SURFACE_INDEX=0
  }
}
function check8b_classify_line {
  CHECK8B_CLASSIFICATION=""
  CHECK8B_VERB=""
  CHECK8B_MUTATION_TARGET=""
  CHECK8B_DIRECT_SURFACES=""
  CHECK8B_PRESERVED_SURFACES=""
  CHECK8B_REASON=""
  CHECK8B_UNRESOLVED_PHRASE=""
  CHECK8B_BOUNDARY=""
  CHECK8B_TOKEN_COUNT=0
  CHECK8B_CANDIDATE_COUNT=0
  _CHECK8B_TOKENS=()
  _CHECK8B_CANDIDATE_INDEXES=()
  _check8b_mutation_verb probe
  _check8b_surface_from 0 0
  return 0
}
# END CHECK8B FINITE CLASSIFIER
EOF
check8b_trace_assignment_form_texts=(
  '  CHECK8B_REASON=""'
  'local CHECK8B_REASON=local-value'
  'declare CHECK8B_REASON=declare-value'
  'typeset CHECK8B_REASON=typeset-value'
  'readonly CHECK8B_REASON=readonly-value'
  'export CHECK8B_REASON=export-value'
  'CHECK8B_REASON+=append-value'
  '_CHECK8B_TOKENS=()'
  '_CHECK8B_TOKENS[0]=token-value'
)
check8b_trace_assignment_form_names=(
  CHECK8B_REASON CHECK8B_REASON CHECK8B_REASON CHECK8B_REASON CHECK8B_REASON
  CHECK8B_REASON CHECK8B_REASON _CHECK8B_TOKENS _CHECK8B_TOKENS
)
check8b_trace_assignment_form_failures=0
for check8b_role_form_index in "${!check8b_trace_assignment_form_texts[@]}"; do
  if ! check8b_trace_strip_inert_text "${check8b_trace_assignment_form_texts[$check8b_role_form_index]}" \
    || ! check8b_trace_has_executable_assignment \
      "$CHECK8B_TRACE_EXECUTABLE_TEXT" \
      "${check8b_trace_assignment_form_names[$check8b_role_form_index]}"; then
    check8b_trace_assignment_form_failures=$((check8b_trace_assignment_form_failures + 1))
  fi
done
check8b_trace_inert_assignment_texts=(
  'local inert=CHECK8B_REASON='
  'local prefix_CHECK8B_REASON=value'
  'local CHECK8B_REASON_suffix=value'
  'local inert=${CHECK8B_REASON:=value}'
  'printf "%s\n" CHECK8B_REASON='
  '(( CHECK8B_REASON = 1 ))'
  '[[ CHECK8B_REASON = value ]]'
  ': CHECK8B_REASON='
)
check8b_trace_inert_assignment_failures=0
for check8b_role_form_index in "${!check8b_trace_inert_assignment_texts[@]}"; do
  if ! check8b_trace_strip_inert_text "${check8b_trace_inert_assignment_texts[$check8b_role_form_index]}"; then
    check8b_trace_inert_assignment_failures=$((check8b_trace_inert_assignment_failures + 1))
  elif check8b_trace_has_executable_assignment "$CHECK8B_TRACE_EXECUTABLE_TEXT" "CHECK8B_REASON"; then
    check8b_trace_inert_assignment_failures=$((check8b_trace_inert_assignment_failures + 1))
  fi
done
check8b_trace_call_form_texts=(
  '_check8b_mutation_verb direct'
  'if _check8b_mutation_verb conditional; then :; fi'
  'if false; then :; elif _check8b_mutation_verb alternative; then :; fi'
  '! _check8b_mutation_verb negated'
  ': && _check8b_mutation_verb conjunction'
  ': || _check8b_mutation_verb disjunction'
)
check8b_trace_call_form_failures=0
for check8b_role_form_index in "${!check8b_trace_call_form_texts[@]}"; do
  if ! check8b_trace_strip_inert_text "${check8b_trace_call_form_texts[$check8b_role_form_index]}" \
    || ! check8b_trace_has_executable_call "$CHECK8B_TRACE_EXECUTABLE_TEXT" "_check8b_mutation_verb"; then
    check8b_trace_call_form_failures=$((check8b_trace_call_form_failures + 1))
  fi
done
if check8b_trace_role_contract_build "$check8b_trace_style_fixture" \
  && check8b_source_declaration_only_contract "$check8b_trace_style_fixture" \
  && [[ "${check8b_trace_roles[_check8b_mutation_verb]:-}" == "candidate-semantic" ]] \
  && [[ "${check8b_trace_roles[_check8b_is_surface]:-}" == "semantic-support" ]] \
  && [[ "${check8b_trace_roles[_check8b_surface_from]:-}" == "relationship" ]] \
  && [[ "${check8b_trace_roles[check8b_classify_line]:-}" == "entrypoint" ]] \
  && [[ "$CHECK8B_TRACE_STYLE_NAME_PARENS" -eq 1 ]] \
  && [[ "$CHECK8B_TRACE_STYLE_FUNCTION_NAME" -eq 2 ]] \
  && [[ "$CHECK8B_TRACE_STYLE_FUNCTION_NAME_PARENS" -eq 1 ]] \
  && [[ "$CHECK8B_TRACE_ROLE_CANDIDATE_DECLARATIONS" -eq 1 ]] \
  && [[ "$CHECK8B_TRACE_ROLE_SEMANTIC_SUPPORT_DECLARATIONS" -eq 1 ]] \
  && [[ "$CHECK8B_TRACE_ROLE_RELATIONSHIP_DECLARATIONS" -eq 1 ]] \
  && [[ "$CHECK8B_TRACE_ROLE_ENTRYPOINT_DECLARATIONS" -eq 1 ]] \
  && [[ "$CHECK8B_SOURCE_DECLARATION_FUNCTION_COUNT" -eq 4 ]] \
  && [[ "$CHECK8B_SOURCE_DECLARATION_STYLE_NAME_PARENS" -eq 1 ]] \
  && [[ "$CHECK8B_SOURCE_DECLARATION_STYLE_FUNCTION_NAME" -eq 2 ]] \
  && [[ "$CHECK8B_SOURCE_DECLARATION_STYLE_FUNCTION_NAME_PARENS" -eq 1 ]] \
  && [[ "$CHECK8B_SOURCE_DECLARATION_BEGIN_COUNT" -eq 1 ]] \
  && [[ "$CHECK8B_SOURCE_DECLARATION_END_COUNT" -eq 1 ]] \
  && [[ "${#check8b_production_functions[@]}" -eq 4 ]] \
  && [[ "$check8b_trace_assignment_form_failures" -eq 0 ]] \
  && [[ "$check8b_trace_inert_assignment_failures" -eq 0 ]] \
  && [[ "$check8b_trace_call_form_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B executable-text inventory recognizes name(), function name, and function name() declarations while nested braces and case syntax stay inside their function bodies"
else
  check8b_trace_role_contract_failures=$((check8b_trace_role_contract_failures + 1))
  echo "BUG032_TI03_DECLARATION_STYLE_CLASSIFICATION_FAILED unknown=$CHECK8B_TRACE_ROLE_UNKNOWN_NAMES missing=$CHECK8B_TRACE_ROLE_MISSING_NAMES failures=$CHECK8B_TRACE_ROLE_CONTRACT_FAILURES candidateRoles=$CHECK8B_TRACE_ROLE_CANDIDATE_DECLARATIONS semanticSupportRoles=$CHECK8B_TRACE_ROLE_SEMANTIC_SUPPORT_DECLARATIONS relationshipRoles=$CHECK8B_TRACE_ROLE_RELATIONSHIP_DECLARATIONS entrypointRoles=$CHECK8B_TRACE_ROLE_ENTRYPOINT_DECLARATIONS assignmentFormFailures=$check8b_trace_assignment_form_failures inertAssignmentFailures=$check8b_trace_inert_assignment_failures callFormFailures=$check8b_trace_call_form_failures"
fi

check8b_trace_inert_role_labels=(comment single-quoted double-quoted rhs-assignment longer-identifier)
check8b_trace_inert_role_failures=0
for check8b_index in "${!check8b_trace_inert_role_labels[@]}"; do
  check8b_trace_inert_role_mutant="$tmp_root/bug032-check8b-trace-role-inert-${check8b_trace_inert_role_labels[$check8b_index]}.sh"
  cp "$check8b_trace_style_fixture" "$check8b_trace_inert_role_mutant"
  case "${check8b_trace_inert_role_labels[$check8b_index]}" in
    comment)
      cat <<'EOF' >> "$check8b_trace_inert_role_mutant"
_check8b_inert_comment_role() {
  # _CHECK8B_WORD_RESULT=role _CHECK8B_SURFACE_RESULT=role case "$1" in
  :
}
EOF
      check8b_trace_inert_role_name="_check8b_inert_comment_role"
      ;;
    single-quoted)
      cat <<'EOF' >> "$check8b_trace_inert_role_mutant"
_check8b_inert_single_quoted_role() {
  local inert='_CHECK8B_WORD_RESULT=role _CHECK8B_SURFACE_RESULT=role case "$1" in'
  :
}
EOF
      check8b_trace_inert_role_name="_check8b_inert_single_quoted_role"
      ;;
    double-quoted)
      cat <<'EOF' >> "$check8b_trace_inert_role_mutant"
_check8b_inert_double_quoted_role() {
  local inert="_CHECK8B_WORD_RESULT=role _CHECK8B_SURFACE_RESULT=role case \"\$1\" in"
  :
}
EOF
      check8b_trace_inert_role_name="_check8b_inert_double_quoted_role"
      ;;
    rhs-assignment)
      cat <<'EOF' >> "$check8b_trace_inert_role_mutant"
_check8b_rhs_spoof() { local inert=_CHECK8B_WORD_RESULT=; }
EOF
      check8b_trace_inert_role_name="_check8b_rhs_spoof"
      ;;
    longer-identifier)
      cat <<'EOF' >> "$check8b_trace_inert_role_mutant"
_check8b_longer_identifier_spoof() {
  local prefix_CHECK8B_WORD_RESULT=value
  local _CHECK8B_WORD_RESULT_suffix=value
  local prefix_CHECK8B_SURFACE_RESULT=value
  local _CHECK8B_SURFACE_RESULT_suffix=value
  local prefix_CHECK8B_CSV_RESULT=value
  local _CHECK8B_CSV_RESULT_suffix=value
}
EOF
      check8b_trace_inert_role_name="_check8b_longer_identifier_spoof"
      ;;
  esac
  if check8b_trace_role_contract_build "$check8b_trace_inert_role_mutant" \
    || [[ "${check8b_trace_roles[$check8b_trace_inert_role_name]:-missing}" != "unknown" ]] \
    || [[ ",$CHECK8B_TRACE_ROLE_UNKNOWN_NAMES," != *",$check8b_trace_inert_role_name,"* ]]; then
    check8b_trace_inert_role_failures=$((check8b_trace_inert_role_failures + 1))
    echo "BUG032_TI03_INERT_ROLE_MARKER_ACCEPTED class=${check8b_trace_inert_role_labels[$check8b_index]} role=${check8b_trace_roles[$check8b_trace_inert_role_name]:-missing} unknown=$CHECK8B_TRACE_ROLE_UNKNOWN_NAMES"
  fi
done
if [[ "$check8b_trace_inert_role_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B role derivation ignores inline comments plus single-quoted and double-quoted literal marker text"
else
  check8b_trace_role_contract_failures=$((check8b_trace_role_contract_failures + 1))
  fail "BUG-032 Check 8B inert role-marker matrix has $check8b_trace_inert_role_failures false assignment(s)"
fi

check8b_trace_heredoc_labels=(unquoted quoted tab-stripping)
check8b_trace_heredoc_failures=0
for check8b_trace_heredoc_label in "${check8b_trace_heredoc_labels[@]}"; do
  check8b_trace_heredoc_mutant="$tmp_root/bug032-check8b-trace-role-heredoc-$check8b_trace_heredoc_label.sh"
  cp "$check8b_trace_style_fixture" "$check8b_trace_heredoc_mutant"
  case "$check8b_trace_heredoc_label" in
    unquoted)
      check8b_trace_heredoc_name="_check8b_heredoc_unquoted_spoof"
      cat <<'EOF' >> "$check8b_trace_heredoc_mutant"
_check8b_heredoc_unquoted_spoof() {
  : <<UNQUOTED_PAYLOAD
_CHECK8B_WORD_RESULT=spoof
_CHECK8B_SURFACE_RESULT=spoof
_check8b_mutation_verb spoof
UNQUOTED_PAYLOAD
}
EOF
      ;;
    quoted)
      check8b_trace_heredoc_name="_check8b_heredoc_quoted_spoof"
      cat <<'EOF' >> "$check8b_trace_heredoc_mutant"
_check8b_heredoc_quoted_spoof() {
  : <<'QUOTED_PAYLOAD'
_CHECK8B_WORD_RESULT=spoof
_CHECK8B_SURFACE_RESULT=spoof
_check8b_surface_from 0 0
QUOTED_PAYLOAD
}
EOF
      ;;
    tab-stripping)
      check8b_trace_heredoc_name="_check8b_heredoc_tab_spoof"
      cat <<'EOF' >> "$check8b_trace_heredoc_mutant"
_check8b_heredoc_tab_spoof() {
  : <<-TAB_PAYLOAD
_CHECK8B_WORD_RESULT=spoof
_CHECK8B_SURFACE_RESULT=spoof
_check8b_target_after 0
TAB_PAYLOAD
}
EOF
      ;;
  esac
  check8b_trace_heredoc_contract_status=0
  check8b_trace_role_contract_build "$check8b_trace_heredoc_mutant" \
    || check8b_trace_heredoc_contract_status=$?
  check8b_trace_heredoc_role="${check8b_trace_roles[$check8b_trace_heredoc_name]:-unknown}"
  if [[ "$check8b_trace_heredoc_contract_status" -eq 0 ]]; then
    check8b_trace_heredoc_failures=$((check8b_trace_heredoc_failures + 1))
    echo "BUG032_TI03_HEREDOC_ROLE_CONTRACT_ACCEPTED class=$check8b_trace_heredoc_label role=$check8b_trace_heredoc_role"
  fi
  case "$check8b_trace_heredoc_role" in
    candidate-semantic|semantic-support|relationship|entrypoint)
      check8b_trace_heredoc_failures=$((check8b_trace_heredoc_failures + 1))
      echo "BUG032_TI03_HEREDOC_ROLE_ASSIGNED class=$check8b_trace_heredoc_label role=$check8b_trace_heredoc_role"
      ;;
  esac
done
if [[ "$check8b_trace_heredoc_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B role derivation rejects quoted, unquoted, and tab-stripping heredoc payload markers"
else
  check8b_trace_role_contract_failures=$((check8b_trace_role_contract_failures + 1))
  fail "BUG-032 Check 8B heredoc role-marker matrix has $check8b_trace_heredoc_failures false assignment(s)"
fi

check8b_trace_inert_entrypoint_mutant="$tmp_root/bug032-check8b-trace-role-inert-entrypoint.sh"
cat <<'EOF' > "$check8b_trace_inert_entrypoint_mutant"
_check8b_mutation_verb() {
  _CHECK8B_WORD_RESULT="candidate"
}
_check8b_is_surface() {
  case "$1" in
    *) return 0 ;;
  esac
}
_check8b_surface_from() {
  _CHECK8B_SURFACE_RESULT="$1"
}
check8b_classify_line() {
  # CHECK8B_CLASSIFICATION=owned CHECK8B_REASON=owned _check8b_mutation_verb probe
  local inert_single='CHECK8B_VERB=owned CHECK8B_MUTATION_TARGET=owned _check8b_surface_from 0 0'
  local inert_double="CHECK8B_DIRECT_SURFACES=owned CHECK8B_PRESERVED_SURFACES=owned CHECK8B_REASON=owned CHECK8B_UNRESOLVED_PHRASE=owned CHECK8B_BOUNDARY=owned CHECK8B_TOKEN_COUNT=0 CHECK8B_CANDIDATE_COUNT=0 _CHECK8B_TOKENS=() _CHECK8B_CANDIDATE_INDEXES=()"
  local inert_classification=CHECK8B_CLASSIFICATION=owned
  local inert_verb=CHECK8B_VERB=owned
  local inert_target=CHECK8B_MUTATION_TARGET=owned
  local inert_direct=CHECK8B_DIRECT_SURFACES=owned
  local inert_preserved=CHECK8B_PRESERVED_SURFACES=owned
  local inert_reason=CHECK8B_REASON=owned
  local inert_unresolved=CHECK8B_UNRESOLVED_PHRASE=owned
  local inert_boundary=CHECK8B_BOUNDARY=owned
  local inert_token_count=CHECK8B_TOKEN_COUNT=0
  local inert_candidate_count=CHECK8B_CANDIDATE_COUNT=0
  local inert_tokens=_CHECK8B_TOKENS=owned
  local inert_candidate_indexes=_CHECK8B_CANDIDATE_INDEXES=owned
  local inert_candidate_helper=_check8b_mutation_verb
  local inert_relationship_helper=_check8b_surface_from
  printf '%s\n' _check8b_mutation_verb _check8b_is_surface _check8b_is_surface_head_at _check8b_surface_from _check8b_passive_surface _check8b_named_surface_before _check8b_target_after
  return 0
}
EOF
if check8b_trace_role_contract_build "$check8b_trace_inert_entrypoint_mutant" \
  || [[ "${check8b_trace_roles[check8b_classify_line]:-missing}" != "unknown" ]] \
  || [[ "$CHECK8B_TRACE_ENTRYPOINT_ASSIGNMENT_COUNT" -ne 0 ]] \
  || [[ "$CHECK8B_TRACE_ENTRYPOINT_CANDIDATE_COORDINATION" -ne 0 ]] \
  || [[ "$CHECK8B_TRACE_ENTRYPOINT_RELATIONSHIP_COORDINATION" -ne 0 ]] \
  || [[ "$CHECK8B_TRACE_ROLE_ENTRYPOINT_DECLARATIONS" -ne 0 ]]; then
  check8b_trace_role_contract_failures=$((check8b_trace_role_contract_failures + 1))
  echo "BUG032_TI03_INERT_ENTRYPOINT_ACCEPTED role=${check8b_trace_roles[check8b_classify_line]:-missing} assignments=$CHECK8B_TRACE_ENTRYPOINT_ASSIGNMENT_COUNT candidateCoordination=$CHECK8B_TRACE_ENTRYPOINT_CANDIDATE_COORDINATION relationshipCoordination=$CHECK8B_TRACE_ENTRYPOINT_RELATIONSHIP_COORDINATION entrypointRoles=$CHECK8B_TRACE_ROLE_ENTRYPOINT_DECLARATIONS"
else
  pass "BUG-032 Check 8B exact entrypoint name remains insufficient without executable closed-result ownership and private-helper coordination"
fi

check8b_trace_missing_role_mutant="$tmp_root/bug032-check8b-trace-role-missing-role-mutant.sh"
cat <<'EOF' > "$check8b_trace_missing_role_mutant"
_check8b_candidate_probe() {
  _CHECK8B_WORD_RESULT="candidate"
}
function _check8b_semantic_probe {
  case "$1" in
    *) return 0 ;;
  esac
}
check8b_classify_line() {
  return 0
}
EOF
if check8b_trace_role_contract_build "$check8b_trace_missing_role_mutant"; then
  check8b_trace_role_contract_failures=$((check8b_trace_role_contract_failures + 1))
  echo 'BUG032_TI03_MISSING_RELATIONSHIP_ROLE_ACCEPTED'
else
  pass "BUG-032 Check 8B closed trace-role oracle rejects a source-derived inventory with no relationship role"
fi

if check8b_trace_role_contract_build "$check8b_classifier_file" \
  && [[ "$check8b_trace_role_contract_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B trace roles form one closed classification over every production helper declaration"
else
  check8b_trace_role_contract_failures=$((check8b_trace_role_contract_failures + 1))
  fail "BUG-032 Check 8B trace-role contract has $check8b_trace_role_contract_failures discriminator failure(s) (unknown=$CHECK8B_TRACE_ROLE_UNKNOWN_NAMES missing=$CHECK8B_TRACE_ROLE_MISSING_NAMES)"
fi

check8b_trace_classifier_calls() {
  local declaration="$1"
  local trace_file="$2"
  local options_before=""
  local options_after=""
  local shopts_before=""
  local shopts_after=""
  local debug_trap_before=""
  local debug_trap_after=""
  local ifs_before="$IFS"
  local pwd_before="$PWD"

  : > "$trace_file"
  options_before="$(set +o)"
  shopts_before="$(shopt -p)"
  debug_trap_before="$(trap -p DEBUG)"
  (
    set +e
    set +u
    set -T
    check8b_trace_function=""
    check8b_trace_role=""
    check8b_trace_argument=""
    check8b_trace_classify_status=0
    # shellcheck disable=SC2154  # DEBUG expands these variables only after the assignments above
    trap '
      check8b_trace_function="${FUNCNAME[0]:-}"
      if [[ "$check8b_trace_function" == _check8b_* ]] \
        || [[ -n "${check8b_production_functions[$check8b_trace_function]:-}" ]]; then
        check8b_trace_role="${check8b_trace_roles[$check8b_trace_function]:-unknown}"
        check8b_trace_argument="${1:-}"
        printf "TRACE\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n" \
          "$check8b_trace_role" "$check8b_trace_function" "$LINENO" \
          "${CHECK8B_TOKEN_COUNT:--1}" "${#_CHECK8B_TOKENS[@]}" \
          "${CHECK8B_CANDIDATE_COUNT:--1}" "${#_CHECK8B_CANDIDATE_INDEXES[@]}" \
          "${CHECK8B_CLASSIFICATION:-unset}" "${CHECK8B_REASON:-unset}" \
          "$check8b_trace_argument" >> "$trace_file"
      fi
    ' DEBUG
    bug032_check8b_classify "$declaration" >/dev/null 2>&1
    check8b_trace_classify_status=$?
    trap - DEBUG
    set +T
    printf 'RESULT\t%s\t%s\t%s\n' "$check8b_trace_classify_status" \
      "${CHECK8B_CLASSIFICATION:-unset}" "${CHECK8B_REASON:-unset}" >> "$trace_file"
    exit 0
  )
  check8b_trace_subshell_status=$?
  options_after="$(set +o)"
  shopts_after="$(shopt -p)"
  debug_trap_after="$(trap -p DEBUG)"
  CHECK8B_TRACE_ISOLATION_FAILURES=0
  [[ "$check8b_trace_subshell_status" -eq 0 ]] \
    || CHECK8B_TRACE_ISOLATION_FAILURES=$((CHECK8B_TRACE_ISOLATION_FAILURES + 1))
  [[ "$options_before" == "$options_after" ]] \
    || CHECK8B_TRACE_ISOLATION_FAILURES=$((CHECK8B_TRACE_ISOLATION_FAILURES + 1))
  [[ "$shopts_before" == "$shopts_after" ]] \
    || CHECK8B_TRACE_ISOLATION_FAILURES=$((CHECK8B_TRACE_ISOLATION_FAILURES + 1))
  [[ "$debug_trap_before" == "$debug_trap_after" ]] \
    || CHECK8B_TRACE_ISOLATION_FAILURES=$((CHECK8B_TRACE_ISOLATION_FAILURES + 1))
  [[ "$ifs_before" == "$IFS" ]] \
    || CHECK8B_TRACE_ISOLATION_FAILURES=$((CHECK8B_TRACE_ISOLATION_FAILURES + 1))
  [[ "$pwd_before" == "$PWD" ]] \
    || CHECK8B_TRACE_ISOLATION_FAILURES=$((CHECK8B_TRACE_ISOLATION_FAILURES + 1))
  [[ "$CHECK8B_TRACE_ISOLATION_FAILURES" -eq 0 ]]
}

check8b_trace_row_schema_matches() {
  [[ "$#" -eq 10 ]] || return 1
  local trace_role="$1"
  local trace_name="$2"
  local trace_source_line="$3"
  local trace_token_count="$4"
  local trace_retained_tokens="$5"
  local trace_candidate_count="$6"
  local trace_retained_candidates="$7"
  local trace_classification="$8"
  local trace_reason="$9"
  local trace_argument="${10}"

  case "$trace_role" in
    entrypoint|candidate-semantic|semantic-support|relationship|unknown) ;;
    *) return 1 ;;
  esac
  [[ -n "$trace_name" ]] \
    && [[ "$trace_source_line" =~ ^[0-9]+$ ]] \
    && [[ "$trace_token_count" =~ ^-?[0-9]+$ ]] \
    && [[ "$trace_retained_tokens" =~ ^[0-9]+$ ]] \
    && [[ "$trace_candidate_count" =~ ^-?[0-9]+$ ]] \
    && [[ "$trace_retained_candidates" =~ ^[0-9]+$ ]] \
    && [[ -n "$trace_classification" ]] \
    && [[ -n "$trace_reason" ]] \
    && { [[ "$trace_role" == "entrypoint" ]] || [[ -n "$trace_argument" ]]; }
}

check8b_trace_is_calibrated() {
  local trace_file="$1"
  local expected_classification="$2"
  local expected_reason="$3"
  local trace_tag=""
  local trace_role=""
  local trace_function=""
  local _trace_source_line=""
  local _trace_token_count=""
  local _trace_retained_tokens=""
  local _trace_candidate_count=""
  local _trace_retained_candidates=""
  local _trace_classification=""
  local _trace_reason=""
  local _trace_argument=""
  local result_status=""
  local result_classification=""
  local result_reason=""
  local trace_rows=0
  local entrypoint_rows=0
  local candidate_rows=0
  local semantic_support_rows=0
  local relationship_rows=0
  local unknown_rows=0
  local result_rows=0
  local schema_failures=0
  local -A observed_functions=()

  while IFS=$'\t' read -r trace_tag trace_role trace_function _trace_source_line \
    _trace_token_count _trace_retained_tokens _trace_candidate_count \
    _trace_retained_candidates _trace_classification _trace_reason \
    _trace_argument; do
    if [[ "$trace_tag" == "TRACE" ]]; then
      trace_rows=$((trace_rows + 1))
      check8b_trace_row_schema_matches \
        "$trace_role" "$trace_function" "$_trace_source_line" \
        "$_trace_token_count" "$_trace_retained_tokens" \
        "$_trace_candidate_count" "$_trace_retained_candidates" \
        "$_trace_classification" "$_trace_reason" "$_trace_argument" \
        || schema_failures=$((schema_failures + 1))
      observed_functions["$trace_function"]=1
      case "$trace_role" in
        entrypoint) entrypoint_rows=$((entrypoint_rows + 1)) ;;
        candidate-semantic) candidate_rows=$((candidate_rows + 1)) ;;
        semantic-support) semantic_support_rows=$((semantic_support_rows + 1)) ;;
        relationship) relationship_rows=$((relationship_rows + 1)) ;;
        unknown) unknown_rows=$((unknown_rows + 1)) ;;
      esac
    elif [[ "$trace_tag" == "RESULT" ]]; then
      result_status="$trace_role"
      result_classification="$trace_function"
      result_reason="$_trace_source_line"
      result_rows=$((result_rows + 1))
    fi
  done < "$trace_file"

  [[ "$trace_rows" -gt 0 ]] \
    && [[ "$entrypoint_rows" -gt 0 ]] \
    && [[ "$candidate_rows" -gt 0 ]] \
    && [[ "$semantic_support_rows" -gt 0 ]] \
    && [[ "$relationship_rows" -gt 0 ]] \
    && [[ "$unknown_rows" -eq 0 ]] \
    && [[ "$schema_failures" -eq 0 ]] \
    && [[ "${#observed_functions[@]}" -ge 3 ]] \
    && [[ "$result_rows" -eq 1 ]] \
    && [[ "$result_status" -eq 0 ]] \
    && [[ "$result_classification" == "$expected_classification" ]] \
    && [[ "$result_reason" == "$expected_reason" ]]
}

check8b_trace_schema_calibration_failures=0
check8b_trace_schema_rejected_rows=0
if ! check8b_trace_row_schema_matches \
  candidate-semantic _check8b_mutation_verb 41 128 128 1 1 \
  direct-positive direct ""; then
  check8b_trace_schema_rejected_rows=$((check8b_trace_schema_rejected_rows + 1))
fi
if ! check8b_trace_row_schema_matches \
  candidate-semantic _check8b_mutation_verb 41 128 128 1 1 \
  direct-positive direct remove; then
  check8b_trace_schema_calibration_failures=$((check8b_trace_schema_calibration_failures + 1))
  echo 'BUG032_TI03_TRACE_SCHEMA_REJECTED_VALID_SEMANTIC_ROW'
fi
if ! check8b_trace_row_schema_matches \
  entrypoint check8b_classify_line 42 128 128 1 1 \
  direct-positive direct ""; then
  check8b_trace_schema_calibration_failures=$((check8b_trace_schema_calibration_failures + 1))
  echo 'BUG032_TI03_TRACE_SCHEMA_REJECTED_VALID_ENTRYPOINT_ROW'
fi
if [[ "$check8b_trace_schema_rejected_rows" -ne 1 ]]; then
  check8b_trace_schema_calibration_failures=$((check8b_trace_schema_calibration_failures + 1))
  echo "BUG032_TI03_TRACE_SCHEMA_EMPTY_ARGUMENT_NOT_COUNTED rejectedRows=$check8b_trace_schema_rejected_rows"
fi
if [[ "$check8b_trace_schema_calibration_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B trace schema counts a synthetic empty-argument semantic row as malformed"
else
  fail "BUG-032 Check 8B trace schema calibration has $check8b_trace_schema_calibration_failures failure(s)"
fi

check8b_trace_calibration_failures=$((
  check8b_trace_role_contract_failures + check8b_trace_schema_calibration_failures
))
check8b_trace_inert_file="$tmp_root/bug032-check8b-inert-trace.log"
: > "$check8b_trace_inert_file"
if check8b_trace_is_calibrated "$check8b_trace_inert_file" direct-positive direct; then
  check8b_trace_calibration_failures=$((check8b_trace_calibration_failures + 1))
  echo 'BUG032_TI03_INERT_TRACE_ACCEPTED'
else
  pass "BUG-032 Check 8B trace calibration rejects an empty or inert trace"
fi
check8b_trace_unknown_file="$tmp_root/bug032-check8b-unknown-role-trace.log"
check8b_trace_unknown_entrypoint=""
check8b_trace_unknown_candidate=""
check8b_trace_unknown_semantic=""
check8b_trace_unknown_relationship=""
for check8b_trace_unknown_name in "${!check8b_trace_roles[@]}"; do
  case "${check8b_trace_roles[$check8b_trace_unknown_name]}" in
    entrypoint) [[ -n "$check8b_trace_unknown_entrypoint" ]] || check8b_trace_unknown_entrypoint="$check8b_trace_unknown_name" ;;
    candidate-semantic) [[ -n "$check8b_trace_unknown_candidate" ]] || check8b_trace_unknown_candidate="$check8b_trace_unknown_name" ;;
    semantic-support) [[ -n "$check8b_trace_unknown_semantic" ]] || check8b_trace_unknown_semantic="$check8b_trace_unknown_name" ;;
    relationship) [[ -n "$check8b_trace_unknown_relationship" ]] || check8b_trace_unknown_relationship="$check8b_trace_unknown_name" ;;
  esac
done
printf 'TRACE\tentrypoint\t%s\t1\t0\t0\t0\t0\tdirect-positive\tdirect\tprobe\n' "$check8b_trace_unknown_entrypoint" > "$check8b_trace_unknown_file"
printf 'TRACE\tcandidate-semantic\t%s\t2\t0\t0\t0\t0\tdirect-positive\tdirect\tprobe\n' "$check8b_trace_unknown_candidate" >> "$check8b_trace_unknown_file"
printf 'TRACE\tsemantic-support\t%s\t3\t0\t0\t0\t0\tdirect-positive\tdirect\tprobe\n' "$check8b_trace_unknown_semantic" >> "$check8b_trace_unknown_file"
printf 'TRACE\trelationship\t%s\t4\t0\t0\t0\t0\tdirect-positive\tdirect\tprobe\n' "$check8b_trace_unknown_relationship" >> "$check8b_trace_unknown_file"
printf 'TRACE\tunknown\t_check8b_unclassified_runtime_probe\t5\t0\t0\t0\t0\tdirect-positive\tdirect\tprobe\n' >> "$check8b_trace_unknown_file"
printf 'RESULT\t0\tdirect-positive\tdirect\n' >> "$check8b_trace_unknown_file"
if check8b_trace_is_calibrated "$check8b_trace_unknown_file" direct-positive direct; then
  check8b_trace_calibration_failures=$((check8b_trace_calibration_failures + 1))
  echo 'BUG032_TI03_UNKNOWN_TRACED_HELPER_ACCEPTED'
else
  pass "BUG-032 Check 8B trace oracle fails closed when a traced private helper has no source-derived inventory role"
fi
check8b_token_admitted_trace_file="$tmp_root/bug032-check8b-token-admitted-trace.log"
if ! check8b_trace_classifier_calls "$check8b_token_128" "$check8b_token_admitted_trace_file" \
  || ! check8b_trace_is_calibrated "$check8b_token_admitted_trace_file" direct-positive direct; then
  check8b_trace_calibration_failures=$((check8b_trace_calibration_failures + 1))
  echo "BUG032_TI03_ADMITTED_TRACE_NOT_CALIBRATED functions=${#check8b_production_functions[@]} isolationFailures=${CHECK8B_TRACE_ISOLATION_FAILURES:-unavailable}"
else
  pass "BUG-032 Check 8B trace calibration observes the real entrypoint plus candidate-semantic, semantic-support, and relationship helpers on admitted direct input without caller DEBUG or functrace leakage"
fi

check8b_trace_count_semantic_roles_for_argument() {
  local trace_file="$1"
  local watched_argument="$2"
  local trace_label="$3"
  local trace_tag=""
  local trace_role=""
  local trace_name=""
  local trace_source_line=""
  local trace_token_count=""
  local trace_retained_tokens=""
  local trace_candidate_count=""
  local trace_retained_candidates=""
  local trace_classification=""
  local trace_reason=""
  local trace_argument=""

  CHECK8B_TRACE_COUNT_CANDIDATE_SEMANTIC=0
  CHECK8B_TRACE_COUNT_SEMANTIC_SUPPORT=0
  CHECK8B_TRACE_COUNT_RELATIONSHIP=0
  CHECK8B_TRACE_COUNT_ARGUMENT_CANDIDATE_SEMANTIC=0
  CHECK8B_TRACE_COUNT_ARGUMENT_SEMANTIC_SUPPORT=0
  CHECK8B_TRACE_COUNT_ARGUMENT_RELATIONSHIP=0
  while IFS=$'\t' read -r trace_tag trace_role trace_name trace_source_line \
    trace_token_count trace_retained_tokens trace_candidate_count \
    trace_retained_candidates trace_classification trace_reason trace_argument; do
    [[ "$trace_tag" == "TRACE" ]] || continue
    case "$trace_role" in
      candidate-semantic)
        CHECK8B_TRACE_COUNT_CANDIDATE_SEMANTIC=$((CHECK8B_TRACE_COUNT_CANDIDATE_SEMANTIC + 1))
        [[ "$trace_argument" != "$watched_argument" ]] \
          || CHECK8B_TRACE_COUNT_ARGUMENT_CANDIDATE_SEMANTIC=$((CHECK8B_TRACE_COUNT_ARGUMENT_CANDIDATE_SEMANTIC + 1))
        ;;
      semantic-support)
        CHECK8B_TRACE_COUNT_SEMANTIC_SUPPORT=$((CHECK8B_TRACE_COUNT_SEMANTIC_SUPPORT + 1))
        [[ "$trace_argument" != "$watched_argument" ]] \
          || CHECK8B_TRACE_COUNT_ARGUMENT_SEMANTIC_SUPPORT=$((CHECK8B_TRACE_COUNT_ARGUMENT_SEMANTIC_SUPPORT + 1))
        ;;
      relationship)
        CHECK8B_TRACE_COUNT_RELATIONSHIP=$((CHECK8B_TRACE_COUNT_RELATIONSHIP + 1))
        [[ "$trace_argument" != "$watched_argument" ]] \
          || CHECK8B_TRACE_COUNT_ARGUMENT_RELATIONSHIP=$((CHECK8B_TRACE_COUNT_ARGUMENT_RELATIONSHIP + 1))
        ;;
    esac
  done < "$trace_file"
  printf 'BUG032_TI03_SEMANTIC_TRACE_COUNTS label=%s watchedArgument=%s candidateSemantic=%s semanticSupport=%s relationship=%s watchedCandidateSemantic=%s watchedSemanticSupport=%s watchedRelationship=%s\n' \
    "$trace_label" "$watched_argument" \
    "$CHECK8B_TRACE_COUNT_CANDIDATE_SEMANTIC" \
    "$CHECK8B_TRACE_COUNT_SEMANTIC_SUPPORT" \
    "$CHECK8B_TRACE_COUNT_RELATIONSHIP" \
    "$CHECK8B_TRACE_COUNT_ARGUMENT_CANDIDATE_SEMANTIC" \
    "$CHECK8B_TRACE_COUNT_ARGUMENT_SEMANTIC_SUPPORT" \
    "$CHECK8B_TRACE_COUNT_ARGUMENT_RELATIONSHIP"
}

check8b_semantic_trace_mutant_failures=0
check8b_semantic_trace_base="$tmp_root/bug032-check8b-semantic-trace-calibration-base.log"
printf 'TRACE\tentrypoint\tcheck8b_classify_line\t1\t128\t128\t0\t0\tambiguous\ttoken-limit\tprobe\n' > "$check8b_semantic_trace_base"
printf 'RESULT\t0\tambiguous\ttoken-limit\n' >> "$check8b_semantic_trace_base"
check8b_trace_count_semantic_roles_for_argument "$check8b_semantic_trace_base" route base
if [[ "$CHECK8B_TRACE_COUNT_CANDIDATE_SEMANTIC" -ne 0 ]] \
  || [[ "$CHECK8B_TRACE_COUNT_SEMANTIC_SUPPORT" -ne 0 ]] \
  || [[ "$CHECK8B_TRACE_COUNT_RELATIONSHIP" -ne 0 ]]; then
  check8b_semantic_trace_mutant_failures=$((check8b_semantic_trace_mutant_failures + 1))
  echo 'BUG032_TI03_SEMANTIC_TRACE_EMPTY_BASELINE_FAILED'
fi
for check8b_trace_mutant_role in candidate-semantic semantic-support relationship; do
  check8b_semantic_trace_mutant="$tmp_root/bug032-check8b-semantic-trace-calibration-$check8b_trace_mutant_role.log"
  cp "$check8b_semantic_trace_base" "$check8b_semantic_trace_mutant"
  case "$check8b_trace_mutant_role" in
    candidate-semantic) check8b_trace_mutant_name="${check8b_trace_unknown_candidate:-_check8b_mutation_verb}" ;;
    semantic-support) check8b_trace_mutant_name="${check8b_trace_unknown_semantic:-_check8b_is_surface}" ;;
    relationship) check8b_trace_mutant_name="${check8b_trace_unknown_relationship:-_check8b_surface_from}" ;;
  esac
  printf 'TRACE\t%s\t%s\t2\t128\t128\t9\t8\tambiguous\ttoken-limit\troute\n' \
    "$check8b_trace_mutant_role" "$check8b_trace_mutant_name" >> "$check8b_semantic_trace_mutant"
  check8b_trace_count_semantic_roles_for_argument \
    "$check8b_semantic_trace_mutant" route "mutant-$check8b_trace_mutant_role"
  check8b_trace_mutant_total=$((
    CHECK8B_TRACE_COUNT_ARGUMENT_CANDIDATE_SEMANTIC
    + CHECK8B_TRACE_COUNT_ARGUMENT_SEMANTIC_SUPPORT
    + CHECK8B_TRACE_COUNT_ARGUMENT_RELATIONSHIP
  ))
  if [[ "$check8b_trace_mutant_total" -ne 1 ]]; then
    check8b_semantic_trace_mutant_failures=$((check8b_semantic_trace_mutant_failures + 1))
    echo "BUG032_TI03_SEMANTIC_TRACE_MUTANT_NOT_COUNTED role=$check8b_trace_mutant_role watchedTotal=$check8b_trace_mutant_total"
  fi
done
if [[ "$check8b_semantic_trace_mutant_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B semantic trace counters are calibrated by candidate-semantic, semantic-support, and relationship mutant rows"
else
  fail "BUG-032 Check 8B semantic trace mutant calibration has $check8b_semantic_trace_mutant_failures failure(s)"
fi

bug032_check8b_classify "$check8b_token_128"
check8b_token_128_array_length="${#_CHECK8B_TOKENS[@]}"
check8b_token_128_failures=$((check8b_trace_calibration_failures + check8b_semantic_trace_mutant_failures))
if ! check8b_helper_record_matches \
  0 direct-positive remove route remove:route none direct none \
  tokens=128/128 128 1 128 128 1; then
  check8b_token_128_failures=$((check8b_token_128_failures + 1))
  echo "BUG032_CR03_EXACT_MISMATCH classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON boundary=$CHECK8B_BOUNDARY tokenCount=$CHECK8B_TOKEN_COUNT retained=$check8b_token_128_array_length mutationTarget=$CHECK8B_MUTATION_TARGET directSurfaces=$CHECK8B_DIRECT_SURFACES preservedSurfaces=$CHECK8B_PRESERVED_SURFACES"
fi
bug032_run_check8b_guard_case "token-exact-128" "$check8b_token_128"
check8b_direct_correction_for remove:route
if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
  || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
    scopes.md direct-positive route remove:route none direct tokens=128/128 \
    run missing missing missing blocked "$CHECK8B_EXPECTED_CORRECTION"; then
  check8b_token_128_failures=$((check8b_token_128_failures + 1))
  echo "BUG032_CR03_EXACT_GUARD_RECORD_MISMATCH status=$BUG032_CHECK8B_GUARD_STATUS"
fi

bug032_check8b_classify "$check8b_token_129"
check8b_token_129_array_length="${#_CHECK8B_TOKENS[@]}"
check8b_token_129_route_seen=0
for check8b_token in "${_CHECK8B_TOKENS[@]}"; do
  [[ "$check8b_token" == "route" ]] && check8b_token_129_route_seen=1
done
if ! check8b_helper_record_matches \
  0 ambiguous none unresolved none none token-limit none \
  tokens=129/128:first-overflow 128 0 128 128 0 \
  || [[ "$check8b_token_129_array_length" -ne 128 ]] \
  || [[ "$check8b_token_129_route_seen" -ne 0 ]] \
  || [[ "${_CHECK8B_TOKENS[127]:-}" != "remove" ]] \
  ; then
  check8b_token_128_failures=$((check8b_token_128_failures + 1))
  echo "BUG032_CR03_OVERFLOW_MISMATCH classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON boundary=$CHECK8B_BOUNDARY tokenCount=$CHECK8B_TOKEN_COUNT retained=$check8b_token_129_array_length routeSeen=$check8b_token_129_route_seen lastToken=${_CHECK8B_TOKENS[127]:-none} candidateCount=$CHECK8B_CANDIDATE_COUNT retainedCandidates=${#_CHECK8B_CANDIDATE_INDEXES[@]} mutationTarget=$CHECK8B_MUTATION_TARGET directSurfaces=$CHECK8B_DIRECT_SURFACES preservedSurfaces=$CHECK8B_PRESERVED_SURFACES"
fi
check8b_token_trace_file="$tmp_root/bug032-check8b-token-overflow-trace.log"
if ! check8b_trace_classifier_calls "$check8b_token_129" "$check8b_token_trace_file"; then
  check8b_token_128_failures=$((check8b_token_128_failures + 1))
  echo "BUG032_TI03_TOKEN_TRACE_ISOLATION_FAILURE count=${CHECK8B_TRACE_ISOLATION_FAILURES:-unavailable}"
fi
check8b_token_entrypoint_entries=0
check8b_token_overflow_decision_entries=0
check8b_token_candidate_entries=0
check8b_token_semantic_support_entries=0
check8b_token_relationship_entries=0
check8b_token_unknown_entries=0
check8b_token_129_candidate_entries=0
check8b_token_129_semantic_support_entries=0
check8b_token_129_relationship_entries=0
check8b_token_trace_schema_failures=0
while IFS=$'\t' read -r check8b_trace_tag check8b_trace_role check8b_trace_name \
  check8b_trace_source_line _check8b_trace_token_count _check8b_trace_retained_tokens \
  check8b_trace_candidate_count check8b_trace_retained_candidates \
  check8b_trace_classification check8b_trace_reason check8b_trace_argument; do
  [[ "$check8b_trace_tag" == "TRACE" ]] || continue
  if ! check8b_trace_row_schema_matches \
    "$check8b_trace_role" "$check8b_trace_name" "$check8b_trace_source_line" \
    "$_check8b_trace_token_count" "$_check8b_trace_retained_tokens" \
    "$check8b_trace_candidate_count" "$check8b_trace_retained_candidates" \
    "$check8b_trace_classification" "$check8b_trace_reason" \
    "$check8b_trace_argument"; then
    check8b_token_trace_schema_failures=$((check8b_token_trace_schema_failures + 1))
  fi
  case "$check8b_trace_role" in
    entrypoint)
      check8b_token_entrypoint_entries=$((check8b_token_entrypoint_entries + 1))
      if [[ "$check8b_trace_classification" == "ambiguous" ]] \
        && [[ "$check8b_trace_reason" == "token-limit" ]]; then
        check8b_token_overflow_decision_entries=$((check8b_token_overflow_decision_entries + 1))
      fi
      ;;
    candidate-semantic)
      check8b_token_candidate_entries=$((check8b_token_candidate_entries + 1))
      [[ "$check8b_trace_argument" != "route" ]] \
        || check8b_token_129_candidate_entries=$((check8b_token_129_candidate_entries + 1))
      ;;
    semantic-support)
      check8b_token_semantic_support_entries=$((check8b_token_semantic_support_entries + 1))
      [[ "$check8b_trace_argument" != "route" ]] \
        || check8b_token_129_semantic_support_entries=$((check8b_token_129_semantic_support_entries + 1))
      ;;
    relationship)
      check8b_token_relationship_entries=$((check8b_token_relationship_entries + 1))
      [[ "$check8b_trace_argument" != "route" ]] \
        || check8b_token_129_relationship_entries=$((check8b_token_129_relationship_entries + 1))
      ;;
    unknown) check8b_token_unknown_entries=$((check8b_token_unknown_entries + 1)) ;;
  esac
done < "$check8b_token_trace_file"
if [[ "$check8b_token_entrypoint_entries" -eq 0 ]] \
  || [[ "$check8b_token_overflow_decision_entries" -eq 0 ]] \
  || [[ "$check8b_token_candidate_entries" -ne 0 ]] \
  || [[ "$check8b_token_semantic_support_entries" -ne 0 ]] \
  || [[ "$check8b_token_relationship_entries" -ne 0 ]] \
  || [[ "$check8b_token_129_candidate_entries" -ne 0 ]] \
  || [[ "$check8b_token_129_semantic_support_entries" -ne 0 ]] \
  || [[ "$check8b_token_129_relationship_entries" -ne 0 ]] \
  || [[ "$check8b_token_unknown_entries" -ne 0 ]] \
  || [[ "$check8b_token_trace_schema_failures" -ne 0 ]]; then
  check8b_token_128_failures=$((check8b_token_128_failures + 1))
  echo "BUG032_CR03_TOKEN_OVERFLOW_SEMANTIC_ENTRY entrypoint=$check8b_token_entrypoint_entries overflowDecision=$check8b_token_overflow_decision_entries candidateSemantic=$check8b_token_candidate_entries semanticSupport=$check8b_token_semantic_support_entries relationship=$check8b_token_relationship_entries token129CandidateSemantic=$check8b_token_129_candidate_entries token129SemanticSupport=$check8b_token_129_semantic_support_entries token129Relationship=$check8b_token_129_relationship_entries unknown=$check8b_token_unknown_entries schemaFailures=$check8b_token_trace_schema_failures expectedCandidateSemantic=0 expectedSemanticSupport=0 expectedRelationship=0"
fi
printf 'BUG032_TI03_TOKEN_OVERFLOW_TRACE_COUNTS entrypoint=%s overflowDecision=%s candidateSemantic=%s semanticSupport=%s relationship=%s token129CandidateSemantic=%s token129SemanticSupport=%s token129Relationship=%s unknown=%s schemaFailures=%s\n' \
  "$check8b_token_entrypoint_entries" "$check8b_token_overflow_decision_entries" \
  "$check8b_token_candidate_entries" "$check8b_token_semantic_support_entries" \
  "$check8b_token_relationship_entries" "$check8b_token_129_candidate_entries" \
  "$check8b_token_129_semantic_support_entries" "$check8b_token_129_relationship_entries" \
  "$check8b_token_unknown_entries" "$check8b_token_trace_schema_failures"
bug032_run_check8b_guard_case "token-overflow-129" "$check8b_token_129"
if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
  || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
    scopes.md ambiguous unresolved none none token-limit \
    tokens=129/128:first-overflow skipped skipped skipped skipped blocked \
    'Split only this declaration before token 129 while preserving its meaning.'; then
  check8b_token_128_failures=$((check8b_token_128_failures + 1))
  echo "BUG032_CR03_OVERFLOW_GUARD_RECORD_MISMATCH status=$BUG032_CHECK8B_GUARD_STATUS"
fi
if [[ "$check8b_token_128_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B admits exactly 128 tokens and rejects token 129 before semantic scan"
else
  fail "BUG-032 Check 8B exact token-boundary matrix has $check8b_token_128_failures mismatch(es)"
fi

# BUG032-ENG-02 / SCN-032-016: candidate nine is an overflow sentinel. The
# trailing-tail adversary proves candidate nine never reaches relationship
# parsing, because candidate-limit must win over surface-tail.
check8b_candidate_8='remove route remove path remove endpoint remove contract remove api remove url remove slug remove identifier'
check8b_candidate_9="$check8b_candidate_8 deprecate redirect"
check8b_candidate_9_tail="$check8b_candidate_8 deprecate route example"
check8b_candidate_10_sentinel="$check8b_candidate_9 rename symbol"
check8b_candidate_expected='remove:route,remove:path,remove:endpoint,remove:contract,remove:api,remove:url,remove:slug,remove:identifier'
check8b_candidate_helper_failures="$check8b_semantic_trace_mutant_failures"
check8b_candidate_guard_failures=0

check8b_trace_count_after_candidate_discovery() {
  local trace_file="$1"
  local discovery_argument="$2"
  local trace_label="$3"
  local trace_tag=""
  local trace_role=""
  local trace_name=""
  local trace_source_line=""
  local trace_token_count=""
  local trace_retained_tokens=""
  local trace_candidate_count=""
  local trace_retained_candidates=""
  local trace_classification=""
  local trace_reason=""
  local trace_argument=""
  local after_discovery=0

  CHECK8B_TRACE_DISCOVERY_ENTRIES=0
  CHECK8B_TRACE_PRE_DISCOVERY_SEMANTIC_SUPPORT=0
  CHECK8B_TRACE_AFTER_DISCOVERY_CANDIDATE_VERBS=0
  CHECK8B_TRACE_AFTER_DISCOVERY_SEMANTIC_SUPPORT=0
  CHECK8B_TRACE_AFTER_DISCOVERY_RELATIONSHIP=0
  while IFS=$'\t' read -r trace_tag trace_role trace_name trace_source_line \
    trace_token_count trace_retained_tokens trace_candidate_count \
    trace_retained_candidates trace_classification trace_reason trace_argument; do
    [[ "$trace_tag" == "TRACE" ]] || continue
    if [[ "$trace_role" == "candidate-semantic" ]] \
      && [[ "$trace_argument" == "$discovery_argument" ]]; then
      CHECK8B_TRACE_DISCOVERY_ENTRIES=$((CHECK8B_TRACE_DISCOVERY_ENTRIES + 1))
      after_discovery=1
      continue
    fi
    if [[ "$after_discovery" -eq 0 ]]; then
      [[ "$trace_role" != "semantic-support" ]] \
        || CHECK8B_TRACE_PRE_DISCOVERY_SEMANTIC_SUPPORT=$((CHECK8B_TRACE_PRE_DISCOVERY_SEMANTIC_SUPPORT + 1))
      continue
    fi
    case "$trace_role" in
      candidate-semantic)
        case "$trace_argument" in
          rename|renames|renamed|renaming|remove|removes|removed|removing|move|moves|moved|moving|deprecate|deprecates|deprecated|deprecating)
            CHECK8B_TRACE_AFTER_DISCOVERY_CANDIDATE_VERBS=$((CHECK8B_TRACE_AFTER_DISCOVERY_CANDIDATE_VERBS + 1))
            ;;
        esac
        ;;
      semantic-support)
        CHECK8B_TRACE_AFTER_DISCOVERY_SEMANTIC_SUPPORT=$((CHECK8B_TRACE_AFTER_DISCOVERY_SEMANTIC_SUPPORT + 1))
        ;;
      relationship)
        CHECK8B_TRACE_AFTER_DISCOVERY_RELATIONSHIP=$((CHECK8B_TRACE_AFTER_DISCOVERY_RELATIONSHIP + 1))
        ;;
    esac
  done < "$trace_file"
  printf 'BUG032_TI03_CANDIDATE_TRACE_COUNTS label=%s discoveryArgument=%s discoveryEntries=%s preDiscoverySemanticSupport=%s postDiscoveryCandidateVerbs=%s postDiscoverySemanticSupport=%s postDiscoveryRelationship=%s\n' \
    "$trace_label" "$discovery_argument" "$CHECK8B_TRACE_DISCOVERY_ENTRIES" \
    "$CHECK8B_TRACE_PRE_DISCOVERY_SEMANTIC_SUPPORT" \
    "$CHECK8B_TRACE_AFTER_DISCOVERY_CANDIDATE_VERBS" \
    "$CHECK8B_TRACE_AFTER_DISCOVERY_SEMANTIC_SUPPORT" \
    "$CHECK8B_TRACE_AFTER_DISCOVERY_RELATIONSHIP"
}

check8b_candidate_trace_order_calibration_failures=0
check8b_candidate_trace_order_base="$tmp_root/bug032-check8b-candidate-trace-order-base.log"
printf 'TRACE\tcandidate-semantic\t%s\t1\t18\t18\t9\t8\tambiguous\tcandidate-limit\tdeprecate\n' \
  "${check8b_trace_unknown_candidate:-_check8b_mutation_verb}" > "$check8b_candidate_trace_order_base"
printf 'TRACE\tentrypoint\tcheck8b_classify_line\t2\t18\t18\t9\t8\tambiguous\tcandidate-limit\tprobe\n' \
  >> "$check8b_candidate_trace_order_base"
printf 'RESULT\t0\tambiguous\tcandidate-limit\n' >> "$check8b_candidate_trace_order_base"
check8b_trace_count_after_candidate_discovery "$check8b_candidate_trace_order_base" deprecate base
if [[ "$CHECK8B_TRACE_DISCOVERY_ENTRIES" -ne 1 ]] \
  || [[ "$CHECK8B_TRACE_AFTER_DISCOVERY_CANDIDATE_VERBS" -ne 0 ]] \
  || [[ "$CHECK8B_TRACE_AFTER_DISCOVERY_SEMANTIC_SUPPORT" -ne 0 ]] \
  || [[ "$CHECK8B_TRACE_AFTER_DISCOVERY_RELATIONSHIP" -ne 0 ]]; then
  check8b_candidate_trace_order_calibration_failures=$((check8b_candidate_trace_order_calibration_failures + 1))
  echo 'BUG032_TI03_CANDIDATE_TRACE_ORDER_BASELINE_FAILED'
fi
for check8b_trace_mutant_role in candidate-semantic semantic-support relationship; do
  check8b_candidate_trace_order_mutant="$tmp_root/bug032-check8b-candidate-trace-order-$check8b_trace_mutant_role.log"
  cp "$check8b_candidate_trace_order_base" "$check8b_candidate_trace_order_mutant"
  case "$check8b_trace_mutant_role" in
    candidate-semantic)
      check8b_trace_mutant_name="${check8b_trace_unknown_candidate:-_check8b_mutation_verb}"
      check8b_trace_mutant_argument="rename"
      ;;
    semantic-support)
      check8b_trace_mutant_name="${check8b_trace_unknown_semantic:-_check8b_is_surface}"
      check8b_trace_mutant_argument="route"
      ;;
    relationship)
      check8b_trace_mutant_name="${check8b_trace_unknown_relationship:-_check8b_surface_from}"
      check8b_trace_mutant_argument="9"
      ;;
  esac
  printf 'TRACE\t%s\t%s\t3\t18\t18\t9\t8\tambiguous\tcandidate-limit\t%s\n' \
    "$check8b_trace_mutant_role" "$check8b_trace_mutant_name" \
    "$check8b_trace_mutant_argument" >> "$check8b_candidate_trace_order_mutant"
  check8b_trace_count_after_candidate_discovery \
    "$check8b_candidate_trace_order_mutant" deprecate "mutant-$check8b_trace_mutant_role"
  check8b_trace_mutant_total=$((
    CHECK8B_TRACE_AFTER_DISCOVERY_CANDIDATE_VERBS
    + CHECK8B_TRACE_AFTER_DISCOVERY_SEMANTIC_SUPPORT
    + CHECK8B_TRACE_AFTER_DISCOVERY_RELATIONSHIP
  ))
  if [[ "$check8b_trace_mutant_total" -ne 1 ]]; then
    check8b_candidate_trace_order_calibration_failures=$((check8b_candidate_trace_order_calibration_failures + 1))
    echo "BUG032_TI03_CANDIDATE_TRACE_ORDER_MUTANT_NOT_COUNTED role=$check8b_trace_mutant_role postDiscoveryTotal=$check8b_trace_mutant_total"
  fi
done
if [[ "$check8b_candidate_trace_order_calibration_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B ordered candidate trace oracle is calibrated by post-ninth candidate-semantic, semantic-support, and relationship mutant rows"
else
  check8b_candidate_helper_failures=$((check8b_candidate_helper_failures + check8b_candidate_trace_order_calibration_failures))
  fail "BUG-032 Check 8B ordered candidate trace calibration has $check8b_candidate_trace_order_calibration_failures failure(s)"
fi

bug032_check8b_classify "$check8b_candidate_8"
if ! check8b_helper_record_matches \
  0 direct-positive remove route "$check8b_candidate_expected" none direct none \
  candidates=8/8 16 8 16 16 8; then
  check8b_candidate_helper_failures=$((check8b_candidate_helper_failures + 1))
  echo "BUG032_ENG02_CANDIDATE_EXACT_MISMATCH classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON boundary=$CHECK8B_BOUNDARY candidateCount=$CHECK8B_CANDIDATE_COUNT retained=${#_CHECK8B_CANDIDATE_INDEXES[@]} mutationTarget=$CHECK8B_MUTATION_TARGET directSurfaces=$CHECK8B_DIRECT_SURFACES preservedSurfaces=$CHECK8B_PRESERVED_SURFACES"
fi
bug032_run_check8b_guard_case "candidate-exact-8" "$check8b_candidate_8"
check8b_direct_correction_for "$check8b_candidate_expected"
if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
  || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
    scopes.md direct-positive route "$check8b_candidate_expected" none direct \
    candidates=8/8 run missing missing missing blocked \
    "$CHECK8B_EXPECTED_CORRECTION"; then
  check8b_candidate_guard_failures=$((check8b_candidate_guard_failures + 1))
  echo "BUG032_ENG02_CANDIDATE_EXACT_GUARD_RECORD_MISMATCH status=$BUG032_CHECK8B_GUARD_STATUS"
fi

check8b_candidate_admitted_trace_file="$tmp_root/bug032-check8b-candidate-admitted-trace.log"
if ! check8b_trace_classifier_calls "$check8b_candidate_8" "$check8b_candidate_admitted_trace_file" \
  || ! check8b_trace_is_calibrated "$check8b_candidate_admitted_trace_file" direct-positive direct; then
  check8b_candidate_helper_failures=$((check8b_candidate_helper_failures + 1))
  echo "BUG032_TI03_CANDIDATE_TRACE_NOT_CALIBRATED functions=${#check8b_production_functions[@]} isolationFailures=${CHECK8B_TRACE_ISOLATION_FAILURES:-unavailable}"
else
  pass "BUG-032 Check 8B trace calibration observes the real entrypoint plus candidate-semantic and relationship helpers on exactly eight candidates"
fi

check8b_candidate_overflow_token_counts=(18 19 20)
for check8b_index in 0 1 2; do
  if [[ "$check8b_index" -eq 0 ]]; then
    check8b_line="$check8b_candidate_9"
    check8b_slug="candidate-overflow-9"
  elif [[ "$check8b_index" -eq 1 ]]; then
    check8b_line="$check8b_candidate_9_tail"
    check8b_slug="candidate-overflow-9-tail"
  else
    check8b_line="$check8b_candidate_10_sentinel"
    check8b_slug="candidate-overflow-10th-sentinel"
  fi
  bug032_check8b_classify "$check8b_line"
  if ! check8b_helper_record_matches \
    0 ambiguous remove unresolved none none candidate-limit none \
    candidates=9/8:first-overflow \
    "${check8b_candidate_overflow_token_counts[$check8b_index]}" 9 \
    "${check8b_candidate_overflow_token_counts[$check8b_index]}" \
    "${check8b_candidate_overflow_token_counts[$check8b_index]}" 8; then
    check8b_candidate_helper_failures=$((check8b_candidate_helper_failures + 1))
    echo "BUG032_ENG02_CANDIDATE_OVERFLOW_MISMATCH index=$check8b_index classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON boundary=$CHECK8B_BOUNDARY candidateCount=$CHECK8B_CANDIDATE_COUNT retained=${#_CHECK8B_CANDIDATE_INDEXES[@]} mutationTarget=$CHECK8B_MUTATION_TARGET directSurfaces=$CHECK8B_DIRECT_SURFACES preservedSurfaces=$CHECK8B_PRESERVED_SURFACES unresolvedPhrase=$CHECK8B_UNRESOLVED_PHRASE"
  fi

  check8b_candidate_trace_file="$tmp_root/bug032-check8b-$check8b_slug-trace.log"
  if ! check8b_trace_classifier_calls "$check8b_line" "$check8b_candidate_trace_file"; then
    check8b_candidate_helper_failures=$((check8b_candidate_helper_failures + 1))
    echo "BUG032_TI03_CANDIDATE_TRACE_ISOLATION_FAILURE index=$check8b_index count=${CHECK8B_TRACE_ISOLATION_FAILURES:-unavailable}"
  fi
  check8b_candidate_entrypoint_entries=0
  check8b_candidate_limit_decision_entries=0
  check8b_candidate_semantic_entries=0
  check8b_candidate_semantic_support_entries=0
  check8b_candidate_relationship_entries=0
  check8b_candidate_post_ninth_semantic_support_entries=0
  check8b_candidate_post_ninth_relationship_entries=0
  check8b_candidate_unknown_entries=0
  check8b_candidate_ninth_seen=0
  check8b_candidate_tenth_seen=0
  check8b_candidate_ninth_discovered=0
  check8b_candidate_trace_schema_failures=0
  while IFS=$'\t' read -r check8b_trace_tag check8b_trace_role _check8b_trace_name \
    _check8b_trace_source_line _check8b_trace_token_count _check8b_trace_retained_tokens \
    _check8b_trace_candidate_count _check8b_trace_retained_candidates \
    check8b_trace_classification check8b_trace_reason check8b_trace_argument; do
    [[ "$check8b_trace_tag" == "TRACE" ]] || continue
    if ! check8b_trace_row_schema_matches \
      "$check8b_trace_role" "$_check8b_trace_name" "$_check8b_trace_source_line" \
      "$_check8b_trace_token_count" "$_check8b_trace_retained_tokens" \
      "$_check8b_trace_candidate_count" "$_check8b_trace_retained_candidates" \
      "$check8b_trace_classification" "$check8b_trace_reason" \
      "$check8b_trace_argument"; then
      check8b_candidate_trace_schema_failures=$((check8b_candidate_trace_schema_failures + 1))
    fi
    case "$check8b_trace_role" in
      entrypoint)
        check8b_candidate_entrypoint_entries=$((check8b_candidate_entrypoint_entries + 1))
        if [[ "$check8b_trace_classification" == "ambiguous" ]] \
          && [[ "$check8b_trace_reason" == "candidate-limit" ]]; then
          check8b_candidate_limit_decision_entries=$((check8b_candidate_limit_decision_entries + 1))
        fi
        ;;
      candidate-semantic)
        check8b_candidate_semantic_entries=$((check8b_candidate_semantic_entries + 1))
        if [[ "$check8b_trace_argument" == "deprecate" ]]; then
          check8b_candidate_ninth_seen=$((check8b_candidate_ninth_seen + 1))
          check8b_candidate_ninth_discovered=1
        fi
        [[ "$check8b_trace_argument" == "rename" ]] \
          && check8b_candidate_tenth_seen=$((check8b_candidate_tenth_seen + 1))
        ;;
      semantic-support)
        check8b_candidate_semantic_support_entries=$((check8b_candidate_semantic_support_entries + 1))
        [[ "$check8b_candidate_ninth_discovered" -eq 0 ]] \
          || check8b_candidate_post_ninth_semantic_support_entries=$((check8b_candidate_post_ninth_semantic_support_entries + 1))
        ;;
      relationship)
        check8b_candidate_relationship_entries=$((check8b_candidate_relationship_entries + 1))
        [[ "$check8b_candidate_ninth_discovered" -eq 0 ]] \
          || check8b_candidate_post_ninth_relationship_entries=$((check8b_candidate_post_ninth_relationship_entries + 1))
        ;;
      unknown) check8b_candidate_unknown_entries=$((check8b_candidate_unknown_entries + 1)) ;;
    esac
  done < "$check8b_candidate_trace_file"
  check8b_trace_count_after_candidate_discovery \
    "$check8b_candidate_trace_file" deprecate "$check8b_slug"
  if [[ "$check8b_candidate_entrypoint_entries" -eq 0 ]] \
    || [[ "$check8b_candidate_limit_decision_entries" -eq 0 ]] \
    || [[ "$check8b_candidate_semantic_entries" -eq 0 ]] \
    || [[ "$check8b_candidate_ninth_seen" -eq 0 ]] \
    || [[ "$check8b_candidate_relationship_entries" -ne 0 ]] \
    || [[ "$check8b_candidate_post_ninth_semantic_support_entries" -ne 0 ]] \
    || [[ "$check8b_candidate_post_ninth_relationship_entries" -ne 0 ]] \
    || [[ "$CHECK8B_TRACE_DISCOVERY_ENTRIES" -eq 0 ]] \
    || [[ "$CHECK8B_TRACE_AFTER_DISCOVERY_CANDIDATE_VERBS" -ne "$check8b_candidate_tenth_seen" ]] \
    || [[ "$CHECK8B_TRACE_AFTER_DISCOVERY_SEMANTIC_SUPPORT" -ne "$check8b_candidate_post_ninth_semantic_support_entries" ]] \
    || [[ "$CHECK8B_TRACE_AFTER_DISCOVERY_RELATIONSHIP" -ne "$check8b_candidate_post_ninth_relationship_entries" ]] \
    || [[ "$check8b_candidate_unknown_entries" -ne 0 ]] \
    || [[ "$check8b_candidate_trace_schema_failures" -ne 0 ]] \
    || { [[ "$check8b_index" -eq 2 ]] && [[ "$check8b_candidate_tenth_seen" -ne 0 ]]; }; then
    check8b_candidate_helper_failures=$((check8b_candidate_helper_failures + 1))
    echo "BUG032_ENG02_CANDIDATE_OVERFLOW_RELATIONSHIP_ENTRY index=$check8b_index entrypoint=$check8b_candidate_entrypoint_entries limitDecision=$check8b_candidate_limit_decision_entries candidateSemantic=$check8b_candidate_semantic_entries semanticSupport=$check8b_candidate_semantic_support_entries ninthSeen=$check8b_candidate_ninth_seen tenthSeen=$check8b_candidate_tenth_seen postNinthSemanticSupport=$check8b_candidate_post_ninth_semantic_support_entries relationship=$check8b_candidate_relationship_entries postNinthRelationship=$check8b_candidate_post_ninth_relationship_entries orderedDiscovery=$CHECK8B_TRACE_DISCOVERY_ENTRIES orderedPostCandidateVerbs=$CHECK8B_TRACE_AFTER_DISCOVERY_CANDIDATE_VERBS orderedPostSemanticSupport=$CHECK8B_TRACE_AFTER_DISCOVERY_SEMANTIC_SUPPORT orderedPostRelationship=$CHECK8B_TRACE_AFTER_DISCOVERY_RELATIONSHIP unknown=$check8b_candidate_unknown_entries schemaFailures=$check8b_candidate_trace_schema_failures expectedPostNinthSemanticSupport=0 expectedRelationship=0 expectedTenthSeen=0"
  fi
  printf 'BUG032_TI03_CANDIDATE_OVERFLOW_TRACE_COUNTS index=%s slug=%s entrypoint=%s limitDecision=%s candidateSemantic=%s semanticSupport=%s ninthCandidateVerb=%s tenthCandidateVerb=%s postNinthSemanticSupport=%s relationship=%s postNinthRelationship=%s unknown=%s schemaFailures=%s\n' \
    "$check8b_index" "$check8b_slug" "$check8b_candidate_entrypoint_entries" \
    "$check8b_candidate_limit_decision_entries" "$check8b_candidate_semantic_entries" \
    "$check8b_candidate_semantic_support_entries" "$check8b_candidate_ninth_seen" \
    "$check8b_candidate_tenth_seen" "$check8b_candidate_post_ninth_semantic_support_entries" \
    "$check8b_candidate_relationship_entries" "$check8b_candidate_post_ninth_relationship_entries" \
    "$check8b_candidate_unknown_entries" "$check8b_candidate_trace_schema_failures"

  bug032_run_check8b_guard_case "$check8b_slug" "$check8b_line"
  if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
    || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
      scopes.md ambiguous unresolved none none candidate-limit \
      candidates=9/8:first-overflow skipped skipped skipped skipped blocked \
      'Split only this declaration so each declaration has at most eight candidates.'; then
    check8b_candidate_guard_failures=$((check8b_candidate_guard_failures + 1))
    echo "BUG032_ENG02_CANDIDATE_OVERFLOW_GUARD_RECORD_MISMATCH index=$check8b_index status=$BUG032_CHECK8B_GUARD_STATUS"
  fi
done
if [[ "$check8b_candidate_helper_failures" -eq 0 ]] \
  && [[ "$check8b_candidate_guard_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B admits exactly eight candidates and rejects candidate nine before relationship analysis"
else
  fail "BUG-032 Check 8B candidate-boundary matrix has helper=$check8b_candidate_helper_failures guard=$check8b_candidate_guard_failures mismatch(es)"
fi

# Invalid invocation and a real bounded-normalization failure are separate
# paths. Only the control-byte case drives the real guard.
CHECK8B_CLASSIFICATION=""
CHECK8B_VERB=""
CHECK8B_MUTATION_TARGET=""
CHECK8B_DIRECT_SURFACES=""
CHECK8B_PRESERVED_SURFACES=""
CHECK8B_REASON=""
CHECK8B_UNRESOLVED_PHRASE=""
CHECK8B_BOUNDARY=""
CHECK8B_TOKEN_COUNT=-1
CHECK8B_CANDIDATE_COUNT=-1
_CHECK8B_TOKENS=(stale-token-state)
_CHECK8B_CLAUSE_IDS=(99)
_CHECK8B_CANDIDATE_INDEXES=(99)
set +e
check8b_classify_line
check8b_invalid_status=$?
set -e
CHECK8B_LAST_STATUS="$check8b_invalid_status"
if check8b_helper_record_matches \
  2 error none unavailable none none invalid-arguments none \
  not-applicable 0 0 0 0 0; then
  pass "BUG-032 Check 8B invalid invocation returns a complete invalid-arguments error record"
else
  fail "BUG-032 Check 8B invalid invocation record is incomplete (status=$check8b_invalid_status classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON verb=$CHECK8B_VERB mutationTarget=$CHECK8B_MUTATION_TARGET directSurfaces=$CHECK8B_DIRECT_SURFACES preservedSurfaces=$CHECK8B_PRESERVED_SURFACES unresolvedPhrase=$CHECK8B_UNRESOLVED_PHRASE boundary=$CHECK8B_BOUNDARY tokenCount=$CHECK8B_TOKEN_COUNT candidateCount=$CHECK8B_CANDIDATE_COUNT retainedTokens=${#_CHECK8B_TOKENS[@]} retainedCandidates=${#_CHECK8B_CANDIDATE_INDEXES[@]})"
fi

check8b_normalization_error_line=$'Remove the public API route\001'
set +e
bug032_check8b_classify "$check8b_normalization_error_line"
check8b_normalization_status=$?
set -e
check8b_normalization_helper_failures=0
check8b_normalization_guard_failures=0
if ! check8b_helper_record_matches \
  2 error none unavailable none none normalization-error none \
  not-applicable 0 0 0 0 0; then
  check8b_normalization_helper_failures=$((check8b_normalization_helper_failures + 1))
  echo "BUG032_ENG02_NORMALIZATION_HELPER_MISMATCH status=$check8b_normalization_status classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON verb=$CHECK8B_VERB mutationTarget=$CHECK8B_MUTATION_TARGET directSurfaces=$CHECK8B_DIRECT_SURFACES preservedSurfaces=$CHECK8B_PRESERVED_SURFACES unresolvedPhrase=$CHECK8B_UNRESOLVED_PHRASE boundary=$CHECK8B_BOUNDARY tokenCount=$CHECK8B_TOKEN_COUNT candidateCount=$CHECK8B_CANDIDATE_COUNT retainedTokens=${#_CHECK8B_TOKENS[@]} retainedCandidates=${#_CHECK8B_CANDIDATE_INDEXES[@]}"
fi
bug032_run_check8b_guard_case "normalization-error" "$check8b_normalization_error_line"
if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
  || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
    scopes.md error unavailable none none normalization-error not-applicable \
    skipped skipped skipped skipped blocked \
    'Rewrite only this declaration as one bounded plain-text sentence. Preserve its meaning, then rerun.'; then
  check8b_normalization_guard_failures=$((check8b_normalization_guard_failures + 1))
  echo "BUG032_ENG02_NORMALIZATION_GUARD_RECORD_MISMATCH status=$BUG032_CHECK8B_GUARD_STATUS"
fi
if [[ "$check8b_normalization_helper_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B normalization-error helper returns a complete closed error record"
else
  fail "BUG-032 Check 8B normalization-error helper has $check8b_normalization_helper_failures incomplete record(s)"
fi

check8b_combined_counter_gate_is_open() {
  [[ "$#" -eq 4 ]] \
    && [[ "$1" -eq 0 ]] \
    && [[ "$2" -eq 0 ]] \
    && [[ "$3" -eq 0 ]] \
    && [[ "$4" -eq 0 ]]
}

check8b_combined_counter_calibration_failures=0
if ! check8b_combined_counter_gate_is_open 0 0 0 0; then
  check8b_combined_counter_calibration_failures=$((check8b_combined_counter_calibration_failures + 1))
  echo 'BUG032_TI06_ZERO_COUNTER_VECTOR_REJECTED'
fi
check8b_combined_counter_vectors=('1 0 0 0' '0 1 0 0' '0 0 1 0' '0 0 0 1')
for check8b_combined_counter_vector in "${check8b_combined_counter_vectors[@]}"; do
  read -r check8b_counter_candidate_helper check8b_counter_candidate_guard \
    check8b_counter_normalization_helper check8b_counter_normalization_guard \
    <<< "$check8b_combined_counter_vector"
  if check8b_combined_counter_gate_is_open \
    "$check8b_counter_candidate_helper" "$check8b_counter_candidate_guard" \
    "$check8b_counter_normalization_helper" "$check8b_counter_normalization_guard"; then
    check8b_combined_counter_calibration_failures=$((check8b_combined_counter_calibration_failures + 1))
    echo "BUG032_TI06_NONZERO_COUNTER_VECTOR_ACCEPTED vector=$check8b_combined_counter_vector"
  fi
done
if [[ "$check8b_combined_counter_calibration_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B combined candidate and normalization gate requires all four counters to be zero"
else
  fail "BUG-032 Check 8B combined counter-gate calibration has $check8b_combined_counter_calibration_failures failure(s)"
fi

if check8b_combined_counter_gate_is_open \
  "$check8b_candidate_helper_failures" "$check8b_candidate_guard_failures" \
  "$check8b_normalization_helper_failures" "$check8b_normalization_guard_failures"; then
  pass "BUG-032 Check 8B guard fails closed on candidate-limit and normalization-error"
else
  fail "BUG-032 Check 8B candidate-limit/normalization-error matrix has candidateHelper=$check8b_candidate_helper_failures candidateGuard=$check8b_candidate_guard_failures normalizationHelper=$check8b_normalization_helper_failures normalizationGuard=$check8b_normalization_guard_failures mismatch(es)"
fi

# One- and two-direct-surface mixed declarations must preserve declaration
# order, keep route only in the preserved set, and run each scope-level impact
# requirement once for the complete direct set.
check8b_mixed_lines=(
  'Preserve the public API route while removing the legacy redirect'
  'Preserve the public route while removing the legacy redirect and renaming the endpoint from v1 to v2.'
)
check8b_mixed_direct_sets=('remove:redirect' 'remove:redirect,rename:endpoint')
check8b_mixed_token_counts=(10 17)
check8b_mixed_candidate_counts=(1 2)
check8b_mixed_failures=0
for check8b_index in "${!check8b_mixed_lines[@]}"; do
  check8b_line="${check8b_mixed_lines[$check8b_index]}"
  check8b_expected_direct="${check8b_mixed_direct_sets[$check8b_index]}"
  bug032_check8b_classify "$check8b_line"
  if ! check8b_helper_record_matches \
    0 mixed-surface remove redirect "$check8b_expected_direct" route mixed none \
    not-applicable "${check8b_mixed_token_counts[$check8b_index]}" \
    "${check8b_mixed_candidate_counts[$check8b_index]}" \
    "${check8b_mixed_token_counts[$check8b_index]}" \
    "${check8b_mixed_token_counts[$check8b_index]}" \
    "${check8b_mixed_candidate_counts[$check8b_index]}"; then
    check8b_mixed_failures=$((check8b_mixed_failures + 1))
    echo "BUG032_ENG02_MIXED_MISMATCH index=$check8b_index classification=$CHECK8B_CLASSIFICATION reason=$CHECK8B_REASON mutationTarget=$CHECK8B_MUTATION_TARGET directSurfaces=$CHECK8B_DIRECT_SURFACES preservedSurfaces=$CHECK8B_PRESERVED_SURFACES"
  fi
  bug032_run_check8b_guard_case "mixed-all-three-$check8b_index" "$check8b_line"
  check8b_direct_correction_for "$check8b_expected_direct"
  if [[ "$BUG032_CHECK8B_GUARD_STATUS" -eq 0 ]] \
    || ! check8b_guard_record_matches "$BUG032_CHECK8B_GUARD_LOG" \
      scopes.md mixed-surface redirect "$check8b_expected_direct" route mixed \
      not-applicable run missing missing missing blocked \
      "$CHECK8B_EXPECTED_CORRECTION"; then
    check8b_mixed_failures=$((check8b_mixed_failures + 1))
    echo "BUG032_ENG02_MIXED_GUARD_RECORD_MISMATCH index=$check8b_index status=$BUG032_CHECK8B_GUARD_STATUS"
  fi
done
if [[ "$check8b_mixed_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B mixed surfaces run all three requirements for every direct surface only"
else
  fail "BUG-032 Check 8B mixed-surface all-three matrix has $check8b_mixed_failures mismatch(es)"
fi

# BUG032-ENG-01: inspect the exact extracted production block, not a copied
# implementation. Arithmetic expansion is allowed; command substitution,
# process substitution, pipelines, external normalizers, and child shells are
# forbidden in the per-line helper path.
check8b_sanitize_structural_line() {
  local input_line="$1"
  local input_length="${#input_line}"
  local index=0
  local character=""
  local next_character=""
  local following_character=""
  local previous_character=""
  local quote_state="plain"
  local arithmetic_depth=0
  local conditional_depth=0

  CHECK8B_STRUCTURAL_CODE=""
  CHECK8B_STRUCTURAL_PROCESS_SYNTAX=0
  CHECK8B_STRUCTURAL_UNCLOSED_QUOTE=0
  while [[ "$index" -lt "$input_length" ]]; do
    character="${input_line:$index:1}"
    next_character=""
    following_character=""
    [[ $((index + 1)) -lt "$input_length" ]] && next_character="${input_line:$((index + 1)):1}"
    [[ $((index + 2)) -lt "$input_length" ]] && following_character="${input_line:$((index + 2)):1}"

    case "$quote_state" in
      plain)
        if [[ "$arithmetic_depth" -gt 0 ]]; then
          if [[ "$character" == '$' && "$next_character" == '(' && "$following_character" != '(' ]] \
            || [[ "$character" == '<' && "$next_character" == '(' ]] \
            || [[ "$character" == '>' && "$next_character" == '(' ]] \
            || [[ "$character" == '`' ]]; then
            CHECK8B_STRUCTURAL_PROCESS_SYNTAX=1
          fi
          if [[ "$character" == '(' ]]; then
            arithmetic_depth=$((arithmetic_depth + 1))
          elif [[ "$character" == ')' ]]; then
            arithmetic_depth=$((arithmetic_depth - 1))
          fi
          CHECK8B_STRUCTURAL_CODE="$CHECK8B_STRUCTURAL_CODE "
          index=$((index + 1))
          continue
        fi
        if [[ "$conditional_depth" -gt 0 ]]; then
          if [[ "$character" == '$' && "$next_character" == '(' && "$following_character" != '(' ]] \
            || [[ "$character" == '<' && "$next_character" == '(' ]] \
            || [[ "$character" == '>' && "$next_character" == '(' ]] \
            || [[ "$character" == '`' ]]; then
            CHECK8B_STRUCTURAL_PROCESS_SYNTAX=1
          fi
          if [[ "$character" == ']' && "$next_character" == ']' ]]; then
            CHECK8B_STRUCTURAL_CODE="$CHECK8B_STRUCTURAL_CODE  "
            conditional_depth=0
            index=$((index + 2))
            continue
          fi
          CHECK8B_STRUCTURAL_CODE="$CHECK8B_STRUCTURAL_CODE "
          index=$((index + 1))
          continue
        fi
        if [[ "$character" == "\\" ]]; then
          CHECK8B_STRUCTURAL_CODE="$CHECK8B_STRUCTURAL_CODE  "
          index=$((index + 2))
          continue
        fi
        if [[ "$character" == "'" ]]; then
          quote_state="single"
          index=$((index + 1))
          continue
        fi
        if [[ "$character" == '"' ]]; then
          quote_state="double"
          index=$((index + 1))
          continue
        fi
        if [[ "$character" == "#" ]]; then
          previous_character=""
          [[ "$index" -gt 0 ]] && previous_character="${input_line:$((index - 1)):1}"
          if [[ "$index" -eq 0 ]] || [[ "$previous_character" =~ [[:space:]\;\&\|\(\)] ]]; then
            break
          fi
        fi
        if [[ "$character" == '$' && "$next_character" == '(' ]]; then
          if [[ "$following_character" == '(' ]]; then
            arithmetic_depth=2
            CHECK8B_STRUCTURAL_CODE="$CHECK8B_STRUCTURAL_CODE   "
            index=$((index + 3))
            continue
          fi
          CHECK8B_STRUCTURAL_PROCESS_SYNTAX=1
        elif [[ "$character" == '(' && "$next_character" == '(' ]]; then
          arithmetic_depth=2
          CHECK8B_STRUCTURAL_CODE="$CHECK8B_STRUCTURAL_CODE  "
          index=$((index + 2))
          continue
        elif [[ "$character" == '[' && "$next_character" == '[' ]]; then
          conditional_depth=1
          CHECK8B_STRUCTURAL_CODE="$CHECK8B_STRUCTURAL_CODE  "
          index=$((index + 2))
          continue
        elif [[ "$character" == '<' && "$next_character" == '(' ]] \
          || [[ "$character" == '>' && "$next_character" == '(' ]] \
          || [[ "$character" == '`' ]]; then
          CHECK8B_STRUCTURAL_PROCESS_SYNTAX=1
        fi
        CHECK8B_STRUCTURAL_CODE="$CHECK8B_STRUCTURAL_CODE$character"
        ;;
      single)
        if [[ "$character" == "'" ]]; then
          quote_state="plain"
        elif [[ "$character" =~ [[:space:]\;\&\|\(\)\{\}] ]]; then
          CHECK8B_STRUCTURAL_CODE="${CHECK8B_STRUCTURAL_CODE}_"
        else
          CHECK8B_STRUCTURAL_CODE="$CHECK8B_STRUCTURAL_CODE$character"
        fi
        ;;
      double)
        if [[ "$character" == "\\" ]]; then
          CHECK8B_STRUCTURAL_CODE="$CHECK8B_STRUCTURAL_CODE  "
          index=$((index + 2))
          continue
        fi
        if [[ "$character" == '"' ]]; then
          quote_state="plain"
        elif [[ "$character" == '`' ]]; then
          CHECK8B_STRUCTURAL_PROCESS_SYNTAX=1
        elif [[ "$character" == '$' && "$next_character" == '(' && "$following_character" != '(' ]]; then
          CHECK8B_STRUCTURAL_PROCESS_SYNTAX=1
        elif [[ "$character" =~ [[:space:]\;\&\|\(\)\{\}] ]]; then
          CHECK8B_STRUCTURAL_CODE="${CHECK8B_STRUCTURAL_CODE}_"
        else
          CHECK8B_STRUCTURAL_CODE="$CHECK8B_STRUCTURAL_CODE$character"
        fi
        ;;
    esac
    index=$((index + 1))
  done
  [[ "$quote_state" == "plain" ]] || CHECK8B_STRUCTURAL_UNCLOSED_QUOTE=1
}

declare -A check8b_structural_allowed_builtins=()
declare -A check8b_structural_allowed_keywords=()
for check8b_structural_word in \
  : true false printf read return break continue shift local declare typeset readonly unset; do
  check8b_structural_allowed_builtins["$check8b_structural_word"]=1
done
check8b_structural_keyword_words=(
  'if' 'then' 'elif' 'else' 'fi' 'case' 'in' 'esac' 'for' 'select'
  'while' 'until' 'do' 'done' 'function' '{' '}'
)
for check8b_structural_word in "${check8b_structural_keyword_words[@]}"; do
  check8b_structural_allowed_keywords["$check8b_structural_word"]=1
done
unset check8b_structural_word check8b_structural_keyword_words

check8b_structural_command_is_allowed() {
  local command_word="$1"

  [[ "$command_word" != */* ]] || return 1
  if [[ -n "${check8b_production_functions[$command_word]:-}" ]]; then
    return 0
  fi
  [[ -n "${check8b_structural_allowed_builtins[$command_word]:-}" ]] && return 0
  return 1
}

check8b_structural_segment_is_clean() {
  local segment="$1"
  local allow_case_arm="${2:-0}"
  local function_prefix=""
  local syntax_prefix=""
  local command_word=""
  local case_header_pattern='^case[[:space:]]+.+[[:space:]]+in([[:space:]]+|$)'
  local case_arm_pattern='^[^[:space:]()=|]+([[:space:]]*\|[[:space:]]*[^[:space:]()=|]+)*[[:space:]]*\)[[:space:]]*'
  local assignment_pattern='^[a-zA-Z_][a-zA-Z0-9_]*(\[[^]]+\])?[[:space:]]*\+?=[^[:space:]]*([[:space:]]+|$)'

  segment="${segment#"${segment%%[![:space:]]*}"}"
  segment="${segment%"${segment##*[![:space:]]}"}"
  [[ -n "$segment" ]] || return 0

  if check8b_parse_function_declaration "$segment"; then
    function_prefix="${segment%"$CHECK8B_DECLARATION_REMAINDER"}"
    segment="${segment#"$function_prefix"}"
    segment="${segment#"${segment%%[![:space:]]*}"}"
    [[ -n "$segment" ]] || return 0
  fi

  if [[ "$segment" =~ $case_header_pattern ]]; then
    syntax_prefix="${BASH_REMATCH[0]}"
    segment="${segment#"$syntax_prefix"}"
    segment="${segment#"${segment%%[![:space:]]*}"}"
    [[ -n "$segment" ]] || return 0
  fi
  if [[ "$allow_case_arm" -eq 1 ]] && [[ "$segment" =~ $case_arm_pattern ]]; then
    syntax_prefix="${BASH_REMATCH[0]}"
    segment="${segment#"$syntax_prefix"}"
    segment="${segment#"${segment%%[![:space:]]*}"}"
    [[ -n "$segment" ]] || return 0
  fi

  if [[ "${segment//||/}" == *'|'* ]]; then
    CHECK8B_STRUCTURAL_REASON="pipeline"
    return 1
  fi
  if [[ "${segment//&&/}" == *'&'* ]]; then
    CHECK8B_STRUCTURAL_REASON="background-process"
    return 1
  fi

  while :; do
    if [[ -n "${check8b_structural_allowed_keywords[$segment]:-}" ]]; then
      return 0
    fi
    case "$segment" in
      '!'[[:space:]]*) segment="${segment#!}" ;;
      if[[:space:]]*|elif[[:space:]]*|while[[:space:]]*|until[[:space:]]*) segment="${segment#* }" ;;
      then[[:space:]]*|else[[:space:]]*|do[[:space:]]*) segment="${segment#* }" ;;
      '{'[[:space:]]*) segment="${segment#\{}" ;;
      '}'[[:space:]]*) segment="${segment#\}}" ;;
      for[[:space:]]*|select[[:space:]]*) return 0 ;;
      '}'|'{'|'[['*|'(('*|'$(( '*|'$((('*|'')
        return 0
        ;;
      *) break ;;
    esac
    segment="${segment#"${segment%%[![:space:]]*}"}"
  done

  while [[ "$segment" =~ $assignment_pattern ]]; do
    segment="${segment#"${BASH_REMATCH[0]}"}"
    segment="${segment#"${segment%%[![:space:]]*}"}"
  done
  [[ -n "$segment" ]] || return 0
  command_word="${segment%%[[:space:]]*}"
  command_word="${command_word%;}"
  command_word="${command_word#\{}"
  command_word="${command_word%\}}"
  [[ -n "$command_word" ]] || return 0
  if [[ "$command_word" == '$'* ]]; then
    CHECK8B_STRUCTURAL_REASON="dynamic-command-head:$command_word"
    return 1
  fi
  while [[ "$command_word" == "command" || "$command_word" == "builtin" ]]; do
    segment="${segment#"$command_word"}"
    segment="${segment#"${segment%%[![:space:]]*}"}"
    if [[ -z "$segment" ]]; then
      CHECK8B_STRUCTURAL_REASON="incomplete-command-prefix:$command_word"
      return 1
    fi
    command_word="${segment%%[[:space:]]*}"
    command_word="${command_word%;}"
  done
  if ! check8b_structural_command_is_allowed "$command_word"; then
    CHECK8B_STRUCTURAL_REASON="command-not-allowlisted:$command_word"
    return 1
  fi
  return 0
}

check8b_structural_line_is_clean() {
  local source_line="$1"
  local code=""
  local index=0
  local character=""
  local next_character=""
  local previous_nonspace=""
  local remaining_code=""
  local function_parens_pattern='^\([[:space:]]*\)[[:space:]]*\{'
  local standalone_case_arm_pattern='^[[:space:]]*[^[:space:]()=|]+([[:space:]]*\|[[:space:]]*[^[:space:]()=|]+)*[[:space:]]*\)([[:space:]]+|$)'
  local segment=""
  local case_context=0
  local segment_trimmed=""
  local -a segments=()

  CHECK8B_STRUCTURAL_REASON=""
  check8b_sanitize_structural_line "$source_line"
  code="$CHECK8B_STRUCTURAL_CODE"
  if [[ "$CHECK8B_STRUCTURAL_PROCESS_SYNTAX" -ne 0 ]]; then
    CHECK8B_STRUCTURAL_REASON="command-or-process-substitution"
    return 1
  fi
  if [[ "$CHECK8B_STRUCTURAL_UNCLOSED_QUOTE" -ne 0 ]]; then
    CHECK8B_STRUCTURAL_REASON="unclosed-quote"
    return 1
  fi
  [[ "$code" =~ $standalone_case_arm_pattern ]] && case_context=1

  for ((index = 0; index < ${#code}; index++)); do
    character="${code:$index:1}"
    next_character=""
    [[ $((index + 1)) -lt ${#code} ]] && next_character="${code:$((index + 1)):1}"
    if [[ "$character" == '(' ]]; then
      previous_nonspace="${code:0:$index}"
      previous_nonspace="${previous_nonspace%"${previous_nonspace##*[![:space:]]}"}"
      previous_nonspace="${previous_nonspace: -1}"
      remaining_code="${code:$index}"
      if [[ "$previous_nonspace" != '=' ]] \
        && [[ "$next_character" != ')' ]] \
        && [[ ! "$remaining_code" =~ $function_parens_pattern ]]; then
        CHECK8B_STRUCTURAL_REASON="subshell-group"
        return 1
      fi
    fi
  done

  segment=""
  for ((index = 0; index < ${#code}; index++)); do
    character="${code:$index:1}"
    next_character=""
    [[ $((index + 1)) -lt ${#code} ]] && next_character="${code:$((index + 1)):1}"
    if [[ "$character" == ';' ]]; then
      segments+=("$segment")
      segment=""
      [[ "$next_character" == ';' ]] && index=$((index + 1))
    elif [[ "$character" == '&' && "$next_character" == '&' ]] \
      || [[ "$character" == '|' && "$next_character" == '|' ]]; then
      segments+=("$segment")
      segment=""
      index=$((index + 1))
    else
      segment="$segment$character"
    fi
  done
  segments+=("$segment")
  for segment in "${segments[@]}"; do
    segment_trimmed="${segment#"${segment%%[![:space:]]*}"}"
    if [[ "$segment_trimmed" =~ ^case[[:space:]]+.+[[:space:]]+in([[:space:]]+|$) ]]; then
      case_context=1
    fi
    check8b_structural_segment_is_clean "$segment" "$case_context" || return 1
    if [[ "$case_context" -eq 1 ]] && [[ "$segment_trimmed" != case\ * ]]; then
      case_context=0
    fi
  done
  return 0
}

check8b_structural_oracle_failures=0
check8b_structural_safe_lines=(
  '    # /usr/bin/tr input | /usr/bin/sed output'
  '# /usr/bin/tr input|/usr/bin/sed output'
  'local value=plain # /opt/homebrew/bin/awk input | grep marker'
  '    local value=plain # inline /usr/bin/mystery input $(forbidden)'
  'route|path|endpoint) return 0 ;;'
  'route | path) return 0 ;;'
  'while|before|after|when|using|via|through|unless|during|because|and|or|but|yet|however|although|from|to|into|onto|as)'
  'route) return 0 ;;'
  '*) return 1 ;;'
  'case "$token" in route | path) return 0 ;; esac'
  'local count=$((count + 1))'
  'local masked=$(( (mask | 4) & 7 ^ 2 ))'
  'local masked=$((mask | 4)) # bitwise pipe is arithmetic, not a process pipeline'
  'local count="${#values[@]}"'
  'local values=(one two three)'
  'elif [[ -n "$left" && ( -n "$right" || -n "$other" ) ]]; then'
  '_check8b_helper() {'
  'printf -v result "%s" value'
  'LC_ALL=C read -r token'
  '_check8b_mutation_verb "$token"'
)
for check8b_source_line in "${check8b_structural_safe_lines[@]}"; do
  if ! check8b_structural_line_is_clean "$check8b_source_line"; then
    check8b_structural_oracle_failures=$((check8b_structural_oracle_failures + 1))
    echo "BUG032_ENG01_STRUCTURAL_SAFE_REJECTED reason=$CHECK8B_STRUCTURAL_REASON source=$check8b_source_line"
  fi
done

check8b_structural_bad_labels=(
  spaced-pipeline
  compact-pipeline
  standalone-case-arm-pipeline
  standalone-subshell
  line-only-open-parenthesis
  conditional-followed-subshell
  conditional-semicolon-subshell
  command-substitution
  backtick-substitution
  input-process-substitution
  output-process-substitution
  quoted-unknown-command
  quoted-absolute-executable
  assignment-prefixed-external
  command-prefix-external
  builtin-prefix-external
  variable-command-head
  unknown-external-command
  indented-external-before-comment
  inline-external-before-comment
  brace-group-external
  inline-case-absolute-external
  inline-function-absolute-external
  inline-function-parens-unknown-external
  command-argument-closing-paren-pipeline
)
check8b_structural_bad_lines=(
  'printf x | tr x y'
  'printf x|/usr/bin/tr x y'
  'route|path) printf x | tr x y'
  '( printf x )'
  '('
  '[[ -n "$value" ]] && ( printf x )'
  'if true; then ( printf x ); fi'
  'value="$(printf x)"'
  'value=`printf x`'
  'consume <(printf x)'
  'consume >(printf x)'
  "'mystery-normalizer' input"
  '"/usr/bin/printf" input'
  'LC_ALL=C tr input'
  'command tr input'
  'builtin tr input'
  '"$normalizer" input'
  'mystery-normalizer input'
  '    mystery-normalizer input # an inline comment cannot hide the command head'
  'local value=plain; mystery-normalizer input # command before inline comment'
  '{ mystery-normalizer input; }'
  'case "$token" in route | path) /usr/bin/tr input output ;; esac'
  'function _check8b_inline_probe { /usr/bin/tr input output; }'
  'function _check8b_inline_probe() { mystery-normalizer input; }'
  'printf "value)" | tr x y'
)
if [[ "${#check8b_structural_bad_labels[@]}" -ne "${#check8b_structural_bad_lines[@]}" ]]; then
  check8b_structural_oracle_failures=$((check8b_structural_oracle_failures + 1))
  echo "BUG032_ENG01_STRUCTURAL_MUTANT_SHAPE labels=${#check8b_structural_bad_labels[@]} lines=${#check8b_structural_bad_lines[@]}"
fi
for check8b_index in "${!check8b_structural_bad_lines[@]}"; do
  check8b_source_line="${check8b_structural_bad_lines[$check8b_index]}"
  if check8b_structural_line_is_clean "$check8b_source_line"; then
    check8b_structural_oracle_failures=$((check8b_structural_oracle_failures + 1))
    echo "BUG032_ENG01_STRUCTURAL_BAD_ACCEPTED class=${check8b_structural_bad_labels[$check8b_index]:-missing-label} source=$check8b_source_line"
  elif [[ -z "$CHECK8B_STRUCTURAL_REASON" ]]; then
    check8b_structural_oracle_failures=$((check8b_structural_oracle_failures + 1))
    echo "BUG032_ENG01_STRUCTURAL_BAD_UNCLASSIFIED class=${check8b_structural_bad_labels[$check8b_index]:-missing-label} source=$check8b_source_line"
  fi
done

check8b_forbidden_families=(tr sed awk grep perl python python3 cut xargs bash sh zsh dash ksh fish env mystery_normalizer)
for check8b_forbidden_family in "${check8b_forbidden_families[@]}"; do
  if check8b_structural_line_is_clean "$check8b_forbidden_family input"; then
    check8b_structural_oracle_failures=$((check8b_structural_oracle_failures + 1))
    echo "BUG032_ENG01_STRUCTURAL_FAMILY_ACCEPTED family=$check8b_forbidden_family form=bare"
  fi
  if check8b_structural_line_is_clean "/usr/bin/$check8b_forbidden_family input"; then
    check8b_structural_oracle_failures=$((check8b_structural_oracle_failures + 1))
    echo "BUG032_ENG01_STRUCTURAL_FAMILY_ACCEPTED family=$check8b_forbidden_family form=absolute"
  fi
  if check8b_structural_line_is_clean "'$check8b_forbidden_family' input"; then
    check8b_structural_oracle_failures=$((check8b_structural_oracle_failures + 1))
    echo "BUG032_ENG01_STRUCTURAL_FAMILY_ACCEPTED family=$check8b_forbidden_family form=quoted"
  fi
done
if [[ "$check8b_structural_oracle_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B closed structural oracle accepts comments, spaced case alternation, controls, arrays, and bitwise arithmetic while rejecting substitutions, real pipelines, subshells, inline case/function execution, quoted or absolute executables, grouped commands, and unknown heads"
else
  fail "BUG-032 Check 8B allowlist structural oracle has $check8b_structural_oracle_failures discriminator failure(s)"
fi

check8b_structural_failures=0
check8b_structural_line_number=0
while IFS= read -r check8b_source_line || [[ -n "$check8b_source_line" ]]; do
  check8b_structural_line_number=$((check8b_structural_line_number + 1))
  if ! check8b_structural_line_is_clean "$check8b_source_line"; then
    check8b_structural_failures=$((check8b_structural_failures + 1))
    echo "BUG032_ENG01_STRUCTURAL_MATCH line=$check8b_structural_line_number reason=$CHECK8B_STRUCTURAL_REASON source=$check8b_source_line"
  fi
done < "$check8b_classifier_file"
if [[ "$check8b_structural_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B exact marked source has no statically admitted process-capable syntax or executable command head"
else
  fail "BUG-032 Check 8B marked helper contains $check8b_structural_failures forbidden process mechanism(s)"
fi

# Hostile command shadows provide the dynamic half of the no-process proof for
# shell-resolved heads. Their private PATH children make spawned execution
# observable. The static mutant oracle separately owns absolute executable
# heads. The matrix itself calls only the exact sourced production entry point.
check8b_shadow_log="$tmp_root/bug032-check8b-hostile-shadow.log"
check8b_shadow_calibration_log="$tmp_root/bug032-check8b-hostile-shadow-calibration.log"
check8b_shadow_bin="$tmp_root/bug032-check8b-hostile-shadow-bin"
mkdir -p "$check8b_shadow_bin"
: > "$check8b_shadow_log"
check8b_forbidden_families=(tr sed awk grep perl python python3 cut xargs bash sh zsh dash ksh fish env mystery_normalizer)
for check8b_forbidden_family in "${check8b_forbidden_families[@]}"; do
  check8b_shadow_wrapper="$check8b_shadow_bin/$check8b_forbidden_family"
  printf '%s\n' \
    '#!/bin/sh' \
    "printf '%s\\n' 'path:$check8b_forbidden_family' >> \"\$CHECK8B_SHADOW_LOG\"" \
    'exit 97' > "$check8b_shadow_wrapper"
  chmod 0700 "$check8b_shadow_wrapper"
done

check8b_define_hostile_shadows() {
  tr() { printf '%s\n' function:tr >> "$CHECK8B_SHADOW_LOG"; return 97; }
  sed() { printf '%s\n' function:sed >> "$CHECK8B_SHADOW_LOG"; return 97; }
  awk() { printf '%s\n' function:awk >> "$CHECK8B_SHADOW_LOG"; return 97; }
  grep() { printf '%s\n' function:grep >> "$CHECK8B_SHADOW_LOG"; return 97; }
  perl() { printf '%s\n' function:perl >> "$CHECK8B_SHADOW_LOG"; return 97; }
  python() { printf '%s\n' function:python >> "$CHECK8B_SHADOW_LOG"; return 97; }
  python3() { printf '%s\n' function:python3 >> "$CHECK8B_SHADOW_LOG"; return 97; }
  cut() { printf '%s\n' function:cut >> "$CHECK8B_SHADOW_LOG"; return 97; }
  xargs() { printf '%s\n' function:xargs >> "$CHECK8B_SHADOW_LOG"; return 97; }
  bash() { printf '%s\n' function:bash >> "$CHECK8B_SHADOW_LOG"; return 97; }
  sh() { printf '%s\n' function:sh >> "$CHECK8B_SHADOW_LOG"; return 97; }
  zsh() { printf '%s\n' function:zsh >> "$CHECK8B_SHADOW_LOG"; return 97; }
  dash() { printf '%s\n' function:dash >> "$CHECK8B_SHADOW_LOG"; return 97; }
  ksh() { printf '%s\n' function:ksh >> "$CHECK8B_SHADOW_LOG"; return 97; }
  fish() { printf '%s\n' function:fish >> "$CHECK8B_SHADOW_LOG"; return 97; }
  env() { printf '%s\n' function:env >> "$CHECK8B_SHADOW_LOG"; return 97; }
  mystery_normalizer() { printf '%s\n' function:mystery_normalizer >> "$CHECK8B_SHADOW_LOG"; return 97; }
  command_not_found_handle() {
    printf '%s\n' "missing:${1:-unnamed}" >> "$CHECK8B_SHADOW_LOG"
    return 97
  }
}

: > "$check8b_shadow_calibration_log"
check8b_shadow_calibration_failures=0
if (
  CHECK8B_SHADOW_LOG="$check8b_shadow_calibration_log"
  export CHECK8B_SHADOW_LOG
  PATH="$check8b_shadow_bin"
  export PATH
  check8b_define_hostile_shadows
  for check8b_forbidden_family in "${check8b_forbidden_families[@]}"; do
    if "$check8b_forbidden_family" calibration; then
      check8b_shadow_function_status=0
    else
      check8b_shadow_function_status=$?
    fi
    if command "$check8b_forbidden_family" calibration; then
      check8b_shadow_path_status=0
    else
      check8b_shadow_path_status=$?
    fi
    [[ "$check8b_shadow_function_status" -eq 97 ]] \
      && [[ "$check8b_shadow_path_status" -eq 97 ]] \
      || exit 1
  done
  if unlisted_check8b_probe calibration; then
    check8b_shadow_missing_status=0
  else
    check8b_shadow_missing_status=$?
  fi
  [[ "$check8b_shadow_missing_status" -eq 97 ]] || exit 1
); then
  :
else
  check8b_shadow_calibration_failures=$((check8b_shadow_calibration_failures + 1))
fi
declare -A check8b_shadow_calibration_counts=()
while IFS= read -r check8b_shadow_name || [[ -n "$check8b_shadow_name" ]]; do
  check8b_shadow_calibration_counts["$check8b_shadow_name"]=$((
    ${check8b_shadow_calibration_counts["$check8b_shadow_name"]:-0} + 1
  ))
done < "$check8b_shadow_calibration_log"
for check8b_forbidden_family in "${check8b_forbidden_families[@]}"; do
  [[ "${check8b_shadow_calibration_counts["function:$check8b_forbidden_family"]:-0}" -eq 1 ]] \
    && [[ "${check8b_shadow_calibration_counts["path:$check8b_forbidden_family"]:-0}" -eq 1 ]] \
    || check8b_shadow_calibration_failures=$((check8b_shadow_calibration_failures + 1))
done
[[ "${check8b_shadow_calibration_counts[missing:unlisted_check8b_probe]:-0}" -eq 1 ]] \
  || check8b_shadow_calibration_failures=$((check8b_shadow_calibration_failures + 1))
if [[ "$check8b_shadow_calibration_failures" -eq 0 ]]; then
  pass "BUG-032 Check 8B hostile shadows calibrate function, closed-PATH child, and command-not-found interception while the static mutant oracle owns absolute executable rejection"
else
  fail "BUG-032 Check 8B hostile shadow calibration has $check8b_shadow_calibration_failures failure(s)"
fi

if (
  CHECK8B_SHADOW_LOG="$check8b_shadow_log"
  export CHECK8B_SHADOW_LOG
  PATH="$check8b_shadow_bin"
  export PATH
  check8b_define_hostile_shadows

  check8b_shadow_failures=0
  check8b_shadow_lines=(
    'The provider implementation is replaced.'
    'Remove the public API route'
    "${check8b_negation_direct_lines[0]}"
    "${check8b_negation_direct_lines[1]}"
    "${check8b_negation_direct_lines[2]}"
    "${check8b_negation_direct_lines[3]}"
    "${check8b_owned_negation_lines[0]}"
    "${check8b_owned_negation_lines[1]}"
    "${check8b_owned_negation_lines[2]}"
    "${check8b_owned_negation_lines[3]}"
    "${check8b_tail_lines[0]}"
    "${check8b_tail_lines[1]}"
    "${check8b_tail_lines[2]}"
    "${check8b_tail_lines[3]}"
    "${check8b_tail_direct_lines[0]}"
    "${check8b_tail_direct_lines[1]}"
    "${check8b_tail_direct_lines[2]}"
    "${check8b_tail_direct_lines[3]}"
    "${check8b_tail_preserved_lines[0]}"
    "${check8b_tail_preserved_lines[1]}"
    "${check8b_tail_preserved_lines[2]}"
    "${check8b_tail_preserved_lines[3]}"
    "$check8b_token_128"
    "$check8b_token_129"
    "$check8b_candidate_8"
    "$check8b_candidate_9"
    "$check8b_candidate_9_tail"
    "$check8b_candidate_10_sentinel"
    "${check8b_mixed_lines[0]}"
    "${check8b_mixed_lines[1]}"
    "$check8b_scn013_conflict"
    "$check8b_scn013_unresolved"
    "$check8b_normalization_error_line"
  )
  check8b_shadow_expectations=(
    '0|irrelevant|none|none|none|none|none|none|not-applicable|5|0|5|5|0'
    '0|direct-positive|remove|route|remove:route|none|direct|none|not-applicable|5|1|5|5|1'
    '0|direct-positive|remove|route|remove:route|none|direct|none|not-applicable|9|1|9|9|1'
    '0|direct-positive|remove|route|remove:route|none|direct|none|not-applicable|10|1|10|10|1'
    '0|direct-positive|remove|route|remove:route|none|direct|none|not-applicable|6|1|6|6|1'
    '0|direct-positive|remove|route|remove:route|none|direct|none|not-applicable|7|1|7|7|1'
    '0|negative|remove|route|none|route|preserved-surface|none|not-applicable|6|1|6|6|1'
    '0|negative|remove|route|none|route|preserved-surface|none|not-applicable|6|1|6|6|1'
    '0|negative|remove|route|none|route|preserved-surface|none|not-applicable|9|1|9|9|1'
    '0|negative|remove|route|none|route|preserved-surface|none|not-applicable|5|1|5|5|1'
    '0|ambiguous|remove|unresolved|none|none|surface-tail|route example|not-applicable|6|1|6|6|1'
    '0|ambiguous|remove|unresolved|none|none|surface-tail|endpoint test|not-applicable|4|1|4|4|1'
    '0|ambiguous|remove|unresolved|none|none|surface-tail|contract fixture|not-applicable|4|1|4|4|1'
    '0|ambiguous|remove|unresolved|none|none|surface-tail|link documentation|not-applicable|4|1|4|4|1'
    '0|direct-positive|remove|route|remove:route|none|direct|none|not-applicable|5|1|5|5|1'
    '0|direct-positive|remove|endpoint|remove:endpoint|none|direct|none|not-applicable|3|1|3|3|1'
    '0|direct-positive|remove|contract|remove:contract|none|direct|none|not-applicable|3|1|3|3|1'
    '0|direct-positive|remove|link|remove:link|none|direct|none|not-applicable|3|1|3|3|1'
    '0|negative|remove|route example|none|route|preserved-surface|none|not-applicable|10|1|10|10|1'
    '0|negative|remove|endpoint test|none|endpoint|preserved-surface|none|not-applicable|8|1|8|8|1'
    '0|negative|remove|contract fixture|none|contract|preserved-surface|none|not-applicable|8|1|8|8|1'
    '0|negative|remove|link documentation|none|link|preserved-surface|none|not-applicable|8|1|8|8|1'
    '0|direct-positive|remove|route|remove:route|none|direct|none|tokens=128/128|128|1|128|128|1'
    '0|ambiguous|none|unresolved|none|none|token-limit|none|tokens=129/128:first-overflow|128|0|128|128|0'
    '0|direct-positive|remove|route|remove:route,remove:path,remove:endpoint,remove:contract,remove:api,remove:url,remove:slug,remove:identifier|none|direct|none|candidates=8/8|16|8|16|16|8'
    '0|ambiguous|remove|unresolved|none|none|candidate-limit|none|candidates=9/8:first-overflow|18|9|18|18|8'
    '0|ambiguous|remove|unresolved|none|none|candidate-limit|none|candidates=9/8:first-overflow|19|9|19|19|8'
    '0|ambiguous|remove|unresolved|none|none|candidate-limit|none|candidates=9/8:first-overflow|20|9|20|20|8'
    '0|mixed-surface|remove|redirect|remove:redirect|route|mixed|none|not-applicable|10|1|10|10|1'
    '0|mixed-surface|remove|redirect|remove:redirect,rename:endpoint|route|mixed|none|not-applicable|17|2|17|17|2'
    '0|ambiguous|remove|unresolved|none|route|conflict|none|not-applicable|9|1|9|9|1'
    '0|ambiguous|remove|unresolved|none|none|unresolved|public api route|not-applicable|9|1|9|9|1'
    '2|error|none|unavailable|none|none|normalization-error|none|not-applicable|0|0|0|0|0'
  )
  declare -A check8b_shadow_observed_outcomes=()
  if [[ "${#check8b_shadow_lines[@]}" -ne "${#check8b_shadow_expectations[@]}" ]]; then
    check8b_shadow_failures=$((check8b_shadow_failures + 1))
    printf 'BUG032_ENG01_SHADOW_MATRIX_SHAPE lines=%s expectations=%s\n' \
      "${#check8b_shadow_lines[@]}" "${#check8b_shadow_expectations[@]}"
  fi
  for check8b_index in "${!check8b_shadow_lines[@]}"; do
    IFS='|' read -r check8b_expected_status check8b_expected_classification \
      check8b_expected_verb check8b_expected_target check8b_expected_direct \
      check8b_expected_preserved check8b_expected_reason \
      check8b_expected_unresolved check8b_expected_boundary \
      check8b_expected_token_count check8b_expected_candidate_count \
      check8b_expected_retained_tokens check8b_expected_retained_clause_ids \
      check8b_expected_retained_candidates \
      <<< "${check8b_shadow_expectations[$check8b_index]}"
    set +e
    bug032_check8b_classify "${check8b_shadow_lines[$check8b_index]}"
    check8b_shadow_status=$?
    set -e
    if ! check8b_helper_record_matches \
      "$check8b_expected_status" "$check8b_expected_classification" \
      "$check8b_expected_verb" "$check8b_expected_target" \
      "$check8b_expected_direct" "$check8b_expected_preserved" \
      "$check8b_expected_reason" "$check8b_expected_unresolved" \
      "$check8b_expected_boundary" "$check8b_expected_token_count" \
      "$check8b_expected_candidate_count" "$check8b_expected_retained_tokens" \
      "$check8b_expected_retained_clause_ids" "$check8b_expected_retained_candidates"; then
      check8b_shadow_failures=$((check8b_shadow_failures + 1))
      printf 'BUG032_ENG01_SHADOW_MATRIX_MISMATCH index=%s status=%s expectedStatus=%s classification=%s expectedClassification=%s reason=%s expectedReason=%s tokenCount=%s candidateCount=%s retainedTokens=%s retainedClauseIds=%s retainedCandidates=%s\n' \
        "$check8b_index" "$check8b_shadow_status" "$check8b_expected_status" \
        "$CHECK8B_CLASSIFICATION" "$check8b_expected_classification" \
        "$CHECK8B_REASON" "$check8b_expected_reason" "$CHECK8B_TOKEN_COUNT" \
        "$CHECK8B_CANDIDATE_COUNT" "${#_CHECK8B_TOKENS[@]}" \
        "${#_CHECK8B_CLAUSE_IDS[@]}" "${#_CHECK8B_CANDIDATE_INDEXES[@]}"
    fi
      check8b_shadow_observed_outcomes["${CHECK8B_CLASSIFICATION:-unset}:${CHECK8B_REASON:-unset}"]=1
  done

  CHECK8B_CLASSIFICATION="__check8b_unset__"
  CHECK8B_VERB="__check8b_unset__"
  CHECK8B_MUTATION_TARGET="__check8b_unset__"
  CHECK8B_DIRECT_SURFACES="__check8b_unset__"
  CHECK8B_PRESERVED_SURFACES="__check8b_unset__"
  CHECK8B_REASON="__check8b_unset__"
  CHECK8B_UNRESOLVED_PHRASE="__check8b_unset__"
  CHECK8B_BOUNDARY="__check8b_unset__"
  CHECK8B_TOKEN_COUNT=-1
  CHECK8B_CANDIDATE_COUNT=-1
  _CHECK8B_TOKENS=(__check8b_stale_token__)
  _CHECK8B_CLAUSE_IDS=(99)
  _CHECK8B_CANDIDATE_INDEXES=(99)
  set +e
  check8b_classify_line
  check8b_shadow_invalid_status=$?
  set -e
  CHECK8B_LAST_STATUS="$check8b_shadow_invalid_status"
  if ! check8b_helper_record_matches \
    2 error none unavailable none none invalid-arguments none \
    not-applicable 0 0 0 0 0; then
    check8b_shadow_failures=$((check8b_shadow_failures + 1))
    printf 'BUG032_ENG01_SHADOW_INVALID_ARGUMENT_MISMATCH status=%s classification=%s reason=%s\n' \
      "$check8b_shadow_invalid_status" "$CHECK8B_CLASSIFICATION" "$CHECK8B_REASON"
  fi
  check8b_shadow_observed_outcomes["${CHECK8B_CLASSIFICATION:-unset}:${CHECK8B_REASON:-unset}"]=1

  check8b_shadow_required_outcomes=(
    irrelevant:none
    direct-positive:direct
    negative:preserved-surface
    ambiguous:surface-tail
    ambiguous:token-limit
    ambiguous:candidate-limit
    ambiguous:conflict
    ambiguous:unresolved
    mixed-surface:mixed
    error:normalization-error
    error:invalid-arguments
  )
  for check8b_shadow_required_outcome in "${check8b_shadow_required_outcomes[@]}"; do
    if [[ -z "${check8b_shadow_observed_outcomes[$check8b_shadow_required_outcome]:-}" ]]; then
      check8b_shadow_failures=$((check8b_shadow_failures + 1))
      printf 'BUG032_ENG01_SHADOW_BRANCH_NOT_EXERCISED outcome=%s\n' "$check8b_shadow_required_outcome"
    fi
  done

  check8b_shadow_invocation_count=0
  while IFS= read -r check8b_shadow_name || [[ -n "$check8b_shadow_name" ]]; do
    check8b_shadow_invocation_count=$((check8b_shadow_invocation_count + 1))
    printf 'BUG032_ENG01_SHADOW_INVOCATION observed=%s\n' "$check8b_shadow_name"
  done < "$check8b_shadow_log"
  printf 'BUG032_ENG01_SHADOW_COUNTS observedInvocations=%s observedOutcomes=%s requiredOutcomes=%s matrixFailures=%s\n' \
    "$check8b_shadow_invocation_count" "${#check8b_shadow_observed_outcomes[@]}" \
    "${#check8b_shadow_required_outcomes[@]}" "$check8b_shadow_failures"
  [[ "$check8b_shadow_invocation_count" -eq 0 ]] \
    && [[ "$check8b_shadow_failures" -eq 0 ]]
); then
  pass "BUG-032 Check 8B hostile shadow command counts remain zero"
else
  fail "BUG-032 Check 8B hostile shadow matrix observed a forbidden command invocation or classification mismatch"
fi

# BUG-032 D2: execute the real guard over an otherwise-passing fixture whose
# only new prose explicitly opts out of SLA/SLO/observability evidence.
echo "Running BUG-032 Check 5A explicit opt-out classifier..."
bug032_sla_optout_dir="$tmp_root/specs/950-bug032-sla-optout"
cp -R "$positive_feature_dir" "$bug032_sla_optout_dir"
cat <<'EOF' >> "$bug032_sla_optout_dir/scopes.md"

### Performance Posture

Observability is opted out and no trace or SLO evidence is injected.
No SLA is declared for this contract.
SLA and SLO are not applicable to this documentation-only change.
No SLO target is declared.
The p95 latency target is not applicable.
EOF
bug032_sla_optout_log="$tmp_root/bug032-sla-optout.log"
bug032_sla_optout_status="$(run_capture "$bug032_sla_optout_log" bash "$GUARD_SCRIPT" "$bug032_sla_optout_dir")"
if [[ "$bug032_sla_optout_status" -eq 0 ]]; then
  pass "BUG032-IV-F3 Check 5A accepts explicit no-SLA/no-SLO, negated-target, and not-applicable target prose without stress coverage"
else
  fail "BUG032-IV-F3 Check 5A should accept explicit no-SLA/no-SLO, negated-target, and not-applicable target prose (observed $bug032_sla_optout_status)"
  sed -n '1,220p' "$bug032_sla_optout_log"
fi
assert_log_not_contains "$bug032_sla_optout_log" \
  "SLA-sensitive scope is missing explicit stress coverage" \
  "BUG032-IV-F3 Check 5A does not turn explicit negated or not-applicable target posture into an affirmative contract"

# Adversarial polarity twin: `no more than` is a comparator, not an opt-out.
# The real guard must still require stress coverage for this quantitative p95
# contract, so this fixture intentionally omits a stress row.
bug032_sla_comparator_dir="$tmp_root/specs/951-bug032-sla-comparator"
cp -R "$positive_feature_dir" "$bug032_sla_comparator_dir"
cat <<'EOF' >> "$bug032_sla_comparator_dir/scopes.md"

### Performance Contract

The p95 latency budget is no more than 200 ms.
EOF
bug032_sla_comparator_log="$tmp_root/bug032-sla-comparator.log"
run_capture "$bug032_sla_comparator_log" bash "$GUARD_SCRIPT" "$bug032_sla_comparator_dir" >/dev/null
assert_log_contains "$bug032_sla_comparator_log" \
  "SLA-sensitive scope is missing explicit stress coverage" \
  "BUG-032 Check 5A still treats 'no more than 200 ms p95 latency' as an affirmative performance contract"

# BUG032-HARDEN9-C5A-CONTEXT-005 / SCN-032-021: quoted performance
# fixture prose in fenced Gherkin, Examples, and Test Plan cells is not an
# active contract. The byte-identical active declaration remains affirmative,
# so ignoring fixture context cannot become a blanket performance exemption.
bug032_scn021_contexts=(gherkin examples test-plan)
bug032_scn021_context_failures=0
for bug032_scn021_context in "${bug032_scn021_contexts[@]}"; do
  bug032_scn021_dir="$tmp_root/specs/956-bug032-scn021-$bug032_scn021_context"
  cp -R "$positive_feature_dir" "$bug032_scn021_dir"
  case "$bug032_scn021_context" in
    gherkin)
      cat <<'EOF' >> "$bug032_scn021_dir/scopes.md"

### Fixture Gherkin

```gherkin
Given fixture prose says "The p95 latency budget is 200 ms."
```
EOF
      ;;
    examples)
      cat <<'EOF' >> "$bug032_scn021_dir/scopes.md"

### Examples

| performance fixture |
| --- |
| The p95 latency budget is 200 ms. |
EOF
      ;;
    test-plan)
      cat <<'EOF' >> "$bug032_scn021_dir/scopes.md"

### Test Plan

| Test Type | Description | Expected Result |
| --- | --- | --- |
| Functional fixture | The p95 latency budget is 200 ms. | The quoted fixture remains inert. |
EOF
      ;;
  esac
  bug032_scn021_log="$tmp_root/bug032-scn021-$bug032_scn021_context.log"
  bug032_scn021_status="$(run_capture "$bug032_scn021_log" bash "$GUARD_SCRIPT" "$bug032_scn021_dir")"
  if [[ "$bug032_scn021_status" -ne 0 ]] \
    || grep -Fq -- 'SLA-sensitive scope is missing explicit stress coverage' "$bug032_scn021_log" \
    || grep -Fq -- 'SLA-sensitive scope is missing canonical Stress Test Plan row' "$bug032_scn021_log"; then
    bug032_scn021_context_failures=$((bug032_scn021_context_failures + 1))
    printf 'BUG032_SCN021_CONTEXT_MISMATCH context=%s status=%s legacyMissing=%s canonicalMissing=%s\n' \
      "$bug032_scn021_context" "$bug032_scn021_status" \
      "$(grep -cF -- 'SLA-sensitive scope is missing explicit stress coverage' "$bug032_scn021_log" || true)" \
      "$(grep -cF -- 'SLA-sensitive scope is missing canonical Stress Test Plan row' "$bug032_scn021_log" || true)"
  fi
done
if [[ "$bug032_scn021_context_failures" -eq 0 ]]; then
  pass "BUG-032 Check 5A ignores quoted Gherkin Examples and Test Plan performance fixtures"
else
  fail "BUG-032 Check 5A performance fixture-context matrix has $bug032_scn021_context_failures mismatch(es)"
fi

bug032_scn021_active_dir="$tmp_root/specs/956-bug032-scn021-active"
cp -R "$positive_feature_dir" "$bug032_scn021_active_dir"
cat <<'EOF' >> "$bug032_scn021_active_dir/scopes.md"

### Performance Contract

The p95 latency budget is 200 ms.
EOF
bug032_scn021_active_log="$tmp_root/bug032-scn021-active.log"
bug032_scn021_active_status="$(run_capture "$bug032_scn021_active_log" bash "$GUARD_SCRIPT" "$bug032_scn021_active_dir")"
if [[ "$bug032_scn021_active_status" -ne 0 ]] \
  && { grep -Fq -- 'SLA-sensitive scope is missing explicit stress coverage' "$bug032_scn021_active_log" \
    || grep -Fq -- 'SLA-sensitive scope is missing canonical Stress Test Plan row' "$bug032_scn021_active_log"; }; then
  pass "BUG-032 Check 5A still evaluates identical active performance contracts"
else
  fail "BUG-032 Check 5A active performance twin has unexpected status=$bug032_scn021_active_status"
fi

# Insert a multi-line fixture fragment before one exact section heading without
# relying on GNU/BSD-divergent sed insertion syntax.
bug032_insert_before_exact_line() {
  local target_file="$1"
  local marker="$2"
  local payload="$3"
  local temp_file=""
  local source_line=""
  local inserted=0

  temp_file="$(mktemp)"
  : > "$temp_file"
  while IFS= read -r source_line || [[ -n "$source_line" ]]; do
    if [[ "$inserted" -eq 0 && "$source_line" == "$marker" ]]; then
      printf '%s\n' "$payload" >> "$temp_file"
      inserted=1
    fi
    printf '%s\n' "$source_line" >> "$temp_file"
  done < "$target_file"
  if [[ "$inserted" -ne 1 ]]; then
    rm -f "$temp_file"
    return 1
  fi
  mv "$temp_file" "$target_file"
}

# BUG032-HARDEN9-C5A-COVERAGE-PROXY-006 / SCN-032-022: the bare word
# `stress` in narrative is not coverage. An active contract needs both a
# canonical Stress Test Plan row and a faithful stress DoD item. The structural
# control carries both in their real sections and must remain accepted.
bug032_scn022_proxy_dir="$tmp_root/specs/957-bug032-scn022-stress-proxy"
cp -R "$positive_feature_dir" "$bug032_scn022_proxy_dir"
cat <<'EOF' >> "$bug032_scn022_proxy_dir/scopes.md"

### Performance Contract

The p95 latency budget is 200 ms.

### Coverage Narrative

The design prose discusses stress behavior but declares no executable Stress row or matching DoD proof.
EOF
bug032_scn022_proxy_log="$tmp_root/bug032-scn022-stress-proxy.log"
bug032_scn022_proxy_status="$(run_capture "$bug032_scn022_proxy_log" bash "$GUARD_SCRIPT" "$bug032_scn022_proxy_dir")"
bug032_scn022_proxy_failures=0
[[ "$bug032_scn022_proxy_status" -ne 0 ]] \
  || bug032_scn022_proxy_failures=$((bug032_scn022_proxy_failures + 1))
grep -Fq -- 'SLA-sensitive scope is missing canonical Stress Test Plan row' "$bug032_scn022_proxy_log" \
  || bug032_scn022_proxy_failures=$((bug032_scn022_proxy_failures + 1))
grep -Fq -- 'SLA-sensitive scope is missing faithful stress DoD item' "$bug032_scn022_proxy_log" \
  || bug032_scn022_proxy_failures=$((bug032_scn022_proxy_failures + 1))
if [[ "$bug032_scn022_proxy_failures" -eq 0 ]]; then
  pass "BUG-032 Check 5A rejects stress-word coverage without a canonical Stress Test Plan row and DoD"
else
  fail "BUG-032 Check 5A stress-proxy matrix has status=$bug032_scn022_proxy_status mismatches=$bug032_scn022_proxy_failures"
fi

bug032_scn022_control_dir="$tmp_root/specs/957-bug032-scn022-structural-control"
cp -R "$positive_feature_dir" "$bug032_scn022_control_dir"
bug032_scn022_control_setup_failures=0
if ! bug032_insert_before_exact_line \
  "$bug032_scn022_control_dir/scopes.md" \
  '### Test Plan' \
  $'### Performance Contract\n\nThe p95 latency budget is 200 ms.\n'; then
  bug032_scn022_control_setup_failures=$((bug032_scn022_control_setup_failures + 1))
fi
if ! bug032_insert_before_exact_line \
  "$bug032_scn022_control_dir/scopes.md" \
  "| Regression E2E | \`e2e-ui\` | \`$positive_feature_dir/tests/docs-broader-regression.e2e.spec.ts\` | Broader regression row required by the guard. | \`selftest:broader-regression\` | Yes |" \
  "| Stress | \`stress\` | $bug032_scn022_control_dir/tests/docs-scenario-regression.e2e.spec.ts | Exercise the active p95 latency budget under pressure. | \`selftest:stress-regression\` | No |"; then
  bug032_scn022_control_setup_failures=$((bug032_scn022_control_setup_failures + 1))
fi
cat <<'EOF' >> "$bug032_scn022_control_dir/scopes.md"
- [x] SCN-032-022 stress test verifies the active p95 latency budget of 200 ms. -> Evidence: report.md#test-evidence
EOF
bug032_scn022_control_log="$tmp_root/bug032-scn022-structural-control.log"
bug032_scn022_control_status="$(run_capture "$bug032_scn022_control_log" bash "$GUARD_SCRIPT" "$bug032_scn022_control_dir")"
if [[ "$bug032_scn022_control_setup_failures" -eq 0 ]] \
  && [[ "$bug032_scn022_control_status" -eq 0 ]] \
  && ! grep -Fq -- 'SLA-sensitive scope is missing canonical Stress Test Plan row' "$bug032_scn022_control_log" \
  && ! grep -Fq -- 'SLA-sensitive scope is missing faithful stress DoD item' "$bug032_scn022_control_log"; then
  pass "BUG-032 Check 5A accepts a canonical Stress row with faithful DoD"
else
  fail "BUG-032 Check 5A canonical Stress control has setup=$bug032_scn022_control_setup_failures status=$bug032_scn022_control_status"
fi
run_bug032_c5a_type_column_regression "$positive_feature_dir" "$tmp_root"
unset -f run_bug032_c5a_type_column_regression
unset -f bug032_insert_before_exact_line

# Regression: bugs/BUG-032-planning-maturity-guard-false-positives,
# SCN-032-004/005. Spec 045 Scopes 05, 06, and 14 discuss or measure latency
# without approving a performance threshold. Those statements must not create
# a stress obligation. Each fixture has a numeric target twin so this cannot be
# satisfied by disabling G026 or treating all latency prose as non-affirmative.
bug032_spec045_labels=("Scope 05" "Scope 06" "Scope 14")
bug032_spec045_slugs=("scope05" "scope06" "scope14")
bug032_spec045_discussions=(
  $'Cold admission and warm latency are separate.\nReport Qwen 3.8 warm decode independently from reasoning volume and cold load.'
  $'No operator-approved latency threshold exists.\nProfile B improves latency while changing non-gating quality.\nLatency cannot rescue a quality gate failure.'
  $'The report answers whether controlled tuning changes Qwen 3.8 latency for each role.\nLatency measurements remain separate from quality evidence.'
)
bug032_spec045_targets=(
  "The warm p95 latency budget is 200 ms."
  "The operator-approved p95 latency threshold is 250 ms."
  "The per-role p99 response-time guarantee is under 500 ms."
)

for bug032_spec045_index in "${!bug032_spec045_labels[@]}"; do
  bug032_spec045_label="${bug032_spec045_labels[$bug032_spec045_index]}"
  bug032_spec045_slug="${bug032_spec045_slugs[$bug032_spec045_index]}"

  bug032_spec045_discussion_dir="$tmp_root/specs/952-bug032-${bug032_spec045_slug}-discussion"
  cp -R "$positive_feature_dir" "$bug032_spec045_discussion_dir"
  {
    printf '\n### Performance Evidence Context\n\n'
    printf '%s\n' "${bug032_spec045_discussions[$bug032_spec045_index]}"
  } >> "$bug032_spec045_discussion_dir/scopes.md"
  bug032_spec045_discussion_log="$tmp_root/bug032-${bug032_spec045_slug}-discussion.log"
  bug032_spec045_discussion_status="$(run_capture "$bug032_spec045_discussion_log" bash "$GUARD_SCRIPT" "$bug032_spec045_discussion_dir")"
  if [[ "$bug032_spec045_discussion_status" -eq 0 ]]; then
    pass "BUG-032 G026 accepts Spec 045 $bug032_spec045_label latency measurement/discussion without stress coverage"
  else
    fail "BUG-032 G026 should accept Spec 045 $bug032_spec045_label latency measurement/discussion (observed $bug032_spec045_discussion_status)"
  fi
  assert_log_not_contains "$bug032_spec045_discussion_log" \
    "SLA-sensitive scope is missing explicit stress coverage" \
    "BUG-032 G026 does not infer an affirmative contract from Spec 045 $bug032_spec045_label prose"

  bug032_spec045_target_dir="$tmp_root/specs/953-bug032-${bug032_spec045_slug}-numeric-target"
  cp -R "$positive_feature_dir" "$bug032_spec045_target_dir"
  {
    printf '\n### Performance Contract\n\n'
    printf '%s\n' "${bug032_spec045_targets[$bug032_spec045_index]}"
  } >> "$bug032_spec045_target_dir/scopes.md"
  bug032_spec045_target_log="$tmp_root/bug032-${bug032_spec045_slug}-numeric-target.log"
  bug032_spec045_target_status="$(run_capture "$bug032_spec045_target_log" bash "$GUARD_SCRIPT" "$bug032_spec045_target_dir")"
  if [[ "$bug032_spec045_target_status" -ne 0 ]]; then
    pass "BUG-032 G026 still requires stress coverage for the Spec 045 $bug032_spec045_label numeric target twin"
  else
    fail "BUG-032 G026 must require stress coverage for the Spec 045 $bug032_spec045_label numeric target twin"
  fi
  assert_log_contains "$bug032_spec045_target_log" \
    "SLA-sensitive scope is missing explicit stress coverage" \
    "BUG-032 G026 reports the missing stress obligation for the Spec 045 $bug032_spec045_label numeric target twin"
done

# Check 5A (Gate G026) decides whether a scope is SLA-sensitive and therefore owes
# stress coverage. Its trigger list mixes long unambiguous terms (latency,
# throughput) with the two three-letter abbreviations `sla` and `slo`. Unbounded,
# those two match any word merely CONTAINING them, so an ordinary word like "slot"
# told a scope it had a latency SLA and owed stress tests it had no reason to
# write. The regex is extracted from the guard source so the test cannot drift
# from the implementation it guards.
echo "Running Check 5A — SLA trigger word-boundary (false-positive regression)..."

check5a_regex="$(grep -E "^[[:space:]]*local performance_signal='latency\|throughput" "$GUARD_SCRIPT" | sed -E "s/^.*performance_signal='([^']*)'.*\$/\1/" || true)"
if [[ -z "$check5a_regex" ]]; then
  fail "Check 5A SLA regex could not be extracted from $GUARD_SCRIPT (guard shape changed)"
else
  pass "Check 5A SLA regex extracted from guard source (no test/source drift)"

  check5a_must_not="$tmp_root/check5a-must-not-flag.txt"
  cat <<'EOF' > "$check5a_must_not"
a refusal names which item it is about, not only which slot
the slot-only shape would have passed
translate the payload before comparing
a slate of scenarios
Slack notification target
slow query path
the slope of the curve
EOF

  check5a_must_flag="$tmp_root/check5a-must-flag.txt"
  cat <<'EOF' > "$check5a_must_flag"
p95 latency budget is 200ms
throughput target of 1000 rps
the SLA is 200ms
our SLO for uptime is 99.9%
p99 response time
EOF

  if grep -Eiq "$check5a_regex" "$check5a_must_not"; then
    fail "Check 5A false-positives on words merely containing sla/slo (slot, slate, Slack, slow, slope, translate)"
    echo "--- offending must-not-flag lines ---"
    grep -niE "$check5a_regex" "$check5a_must_not" || true
    echo "--- end ---"
  else
    pass "Check 5A does NOT treat slot/slot-only/translate/slate/Slack/slow/slope as an SLA declaration"
  fi

  # Adversarial twin: the boundary must not have disabled the terms it bounded.
  check5a_pos_count="$({ grep -ciE "$check5a_regex" "$check5a_must_flag"; } || true)"
  if [[ "$check5a_pos_count" -eq 5 ]]; then
    pass "Check 5A still flags all 5 genuine SLA declarations (p95 latency, throughput, SLA, SLO, p99 response time)"
  else
    fail "Check 5A regressed on genuine SLA declarations ($check5a_pos_count hits, expected 5)"
    echo "--- genuine SLA lines matched ---"
    grep -niE "$check5a_regex" "$check5a_must_flag" || true
    echo "--- end ---"
  fi
fi
fi

echo "Running Check 43 empty-stdout receipt-clone exemption (BUG-007)..."
# Check 43 alleges FORGERY: it says one captured stdout is cited by two different
# commands, "which cannot happen from honest execution". That is the most serious
# accusation this guard makes, so its false-positive floor has to be zero.
#
# Every command that writes NOTHING to stdout hashes to the SHA-256 of the empty
# string. A `grep` that matched nothing, a run that wrote only to stderr, and a
# `--help` that exited non-zero therefore all collide with one another — and the
# guard read that collision as a forged receipt. A receipt with no stdout also
# carries no evidentiary content to clone, so excluding it removes the entire
# false-positive class without softening the check: a real forgery reuses a
# SUBSTANTIVE result, which is by definition non-empty.
#
# The predicate is extracted from the guard source so this test cannot drift away
# from the code it defends. The empty-stdout digest is extracted the same way, for
# the same reason.
c43_predicate="$(grep -F 'map(select((.stdoutHash' "$GUARD_SCRIPT" | head -1 || true)"
c43_empty_sha="$(grep -oE 'c43_empty_stdout_sha256="[0-9a-f]{64}"' "$GUARD_SCRIPT" | grep -oE '[0-9a-f]{64}' | head -1 || true)"
if [[ -z "$c43_predicate" ]]; then
  fail "Check 43 clone predicate could not be extracted from $GUARD_SCRIPT (guard shape changed)"
elif [[ "$c43_empty_sha" != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" ]]; then
  fail "Check 43 empty-stdout constant is not the SHA-256 of the empty string (got: '${c43_empty_sha}')"
elif ! echo "$c43_predicate" | grep -qF '$empty_sha'; then
  fail "Check 43 clone predicate lost its empty-stdout exemption — BUG-007 regressed"
  echo "--- extracted predicate ---"
  echo "$c43_predicate"
  echo "--- end ---"
elif echo "$c43_predicate" | grep -qF '(.stdoutBytes // 0) > 0'; then
  # The exemption must key on the DIGEST, which every receipt carries. Keying it
  # on `stdoutBytes` defaults an ABSENT field to 0, and `0 > 0` then filters a
  # genuine clone out of detection entirely. That is a silent hole, not a fix.
  fail "Check 43 clone predicate keys its exemption on the optional stdoutBytes field — an absent field would exempt genuine clones"
  echo "--- extracted predicate ---"
  echo "$c43_predicate"
  echo "--- end ---"
else
  pass "Check 43 clone predicate extracted from guard source and exempts empty stdout by digest, not by an optional field"

  c43_dir="$(mktemp -d)"
  # Three DIFFERENT commands that each legitimately produced no stdout. All three
  # share the empty-string digest. This is the shape that produced the real-world
  # false positive; it MUST NOT be reported as a clone.
  c43_empty_log="$c43_dir/empty.jsonl"
  {
    printf '%s\n' '{"cmd":"grep -rn TODO src/","stdoutHash":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","stdoutBytes":0}'
    printf '%s\n' '{"cmd":"node scripts/validate.mjs","stdoutHash":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","stdoutBytes":0}'
    printf '%s\n' '{"cmd":"npx playwright test --list","stdoutHash":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","stdoutBytes":0}'
  } > "$c43_empty_log"

  # ADVERSARIAL: two different commands sharing a REAL, non-empty captured stdout.
  # If this stops failing, the exemption has been widened into a hole and the
  # check no longer detects the forgery it exists to detect.
  c43_real_log="$c43_dir/real.jsonl"
  {
    printf '%s\n' '{"cmd":"node --test tests/unit.test.mjs","stdoutHash":"9f2c1a77b3e45d6081ca2be7f4d0913ac5e8b26df1074a3c9e5b0d8f6a271c43","stdoutBytes":2048}'
    printf '%s\n' '{"cmd":"node --test tests/contract.test.mjs","stdoutHash":"9f2c1a77b3e45d6081ca2be7f4d0913ac5e8b26df1074a3c9e5b0d8f6a271c43","stdoutBytes":2048}'
  } > "$c43_real_log"

  # REGRESSION PIN (the shape that reached main): two different commands sharing a
  # real non-empty stdout, on receipts that carry NO `stdoutBytes` key at all. The
  # field is optional, so a predicate that reads `(.stdoutBytes // 0) > 0` defaults
  # it to 0 and filters this genuine clone out of detection. It must be caught by
  # the digest test alone, with no field present.
  c43_nobytes_log="$c43_dir/nobytes.jsonl"
  {
    printf '%s\n' '{"cmd":"cargo test","stdoutHash":"9f2c1a77b3e45d6081ca2be7f4d0913ac5e8b26df1074a3c9e5b0d8f6a271c43"}'
    printf '%s\n' '{"cmd":"npm run lint","stdoutHash":"9f2c1a77b3e45d6081ca2be7f4d0913ac5e8b26df1074a3c9e5b0d8f6a271c43"}'
  } > "$c43_nobytes_log"

  # The mirror of the pin: empty stdout must stay exempt even when `stdoutBytes` is
  # absent, proving the digest carries the exemption on its own.
  c43_empty_nobytes_log="$c43_dir/empty-nobytes.jsonl"
  {
    printf '%s\n' '{"cmd":"grep -rn TODO src/","stdoutHash":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}'
    printf '%s\n' '{"cmd":"node scripts/validate.mjs","stdoutHash":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}'
  } > "$c43_empty_nobytes_log"

  c43_jq="$c43_predicate | group_by(.stdoutHash) | map(select((map(.cmd) | unique | length) > 1)) | length"

  c43_empty_hits="$(jq -rs --arg empty_sha "$c43_empty_sha" "$c43_jq" "$c43_empty_log" 2>/dev/null || echo "ERR")"
  c43_real_hits="$(jq -rs --arg empty_sha "$c43_empty_sha" "$c43_jq" "$c43_real_log" 2>/dev/null || echo "ERR")"
  c43_nobytes_hits="$(jq -rs --arg empty_sha "$c43_empty_sha" "$c43_jq" "$c43_nobytes_log" 2>/dev/null || echo "ERR")"
  c43_empty_nobytes_hits="$(jq -rs --arg empty_sha "$c43_empty_sha" "$c43_jq" "$c43_empty_nobytes_log" 2>/dev/null || echo "ERR")"

  if [[ "$c43_empty_hits" == "0" ]]; then
    pass "Check 43 does not accuse three different commands that each produced empty stdout"
  else
    fail "Check 43 false-positives on empty stdout ($c43_empty_hits clone group(s), expected 0) — BUG-007 regressed"
  fi

  if [[ "$c43_real_hits" == "1" ]]; then
    pass "Check 43 still detects a genuine clone (two commands sharing real non-empty stdout)"
  else
    fail "Check 43 no longer detects a genuine receipt clone ($c43_real_hits group(s), expected 1) — exemption widened into a hole"
  fi

  if [[ "$c43_nobytes_hits" == "1" ]]; then
    pass "Check 43 detects a genuine clone on receipts carrying NO stdoutBytes field (exemption is digest-keyed, not field-keyed)"
  else
    fail "Check 43 missed a genuine clone on receipts with no stdoutBytes field ($c43_nobytes_hits group(s), expected 1) — an optional-field exemption is silently excusing forgeries"
  fi

  if [[ "$c43_empty_nobytes_hits" == "0" ]]; then
    pass "Check 43 exempts empty stdout by digest alone, with no stdoutBytes field present"
  else
    fail "Check 43 false-positives on empty stdout when stdoutBytes is absent ($c43_empty_nobytes_hits group(s), expected 0)"
  fi

  rm -rf "$c43_dir"
fi

# BUG-032 D3: drive the real Check 43 through an isolated tool-call log. Equal
# non-empty stdout is content equality, not execution identity: deterministic
# sibling validator runs are independent when normalized program, numeric exit,
# distinct target, and execution provenance agree. Known non-mixed category
# labels are diagnostic only and may differ. Incompatible commands and
# provenance-poor collisions remain conservative failures.
echo "Running BUG-032 Check 43 receipt execution-identity matrix..."
bug032_receipt_repo="$tmp_root/bug032-receipt-repo"
bug032_receipt_feature="$bug032_receipt_repo/specs/950-bug032-receipt-identity"
bug032_receipt_log="$bug032_receipt_repo/.specify/runtime/tool-calls.jsonl"
clone_framework_surface "$bug032_receipt_repo"
emit_base_fixture "$bug032_receipt_feature"
mutate_delivery_contract "$bug032_receipt_feature/state.json"
git -C "$bug032_receipt_repo" init -q
mkdir -p "$(dirname "$bug032_receipt_log")"

bug032_nonempty_hash="$(sha256_text 'bug032-nonempty-output')"
bug032_empty_hash="$c43_empty_sha"

cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-15T10:00:01Z","sessionId":"receipt-sibling-a","spec":"specs/alpha","scope":"SCOPE-1","cmd":"bash bubbles/scripts/artifact-lint.sh specs/alpha","exitCode":0,"durationMs":101,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-15T10:00:03Z","sessionId":"receipt-sibling-b","spec":"specs/beta","scope":"SCOPE-1","cmd":"bash bubbles/scripts/artifact-lint.sh specs/beta","exitCode":0,"durationMs":103,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
EOF
bug032_receipt_sibling_log="$tmp_root/bug032-receipt-siblings.log"
bug032_receipt_sibling_status="$(run_capture "$bug032_receipt_sibling_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug032_receipt_sibling_status" -eq 0 ]]; then
  pass "BUG-032 Check 43 accepts independent deterministic validator siblings over distinct targets"
else
  fail "BUG-032 Check 43 should accept independent deterministic validator siblings (observed $bug032_receipt_sibling_status)"
fi
assert_log_not_contains "$bug032_receipt_sibling_log" \
  "check=43 verdict=REFUSED" \
  "BUG-032 Check 43 does not classify deterministic sibling validators as cloned evidence"
assert_check43_contains "$bug032_receipt_sibling_log" \
  "reason=deterministic-siblings" \
  "BUG-032 Check 43 sibling acceptance is earned by the multi-field identity path, not an empty analysis result"

cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-15T10:00:11Z","sessionId":"receipt-spelling-a","spec":"specs/alpha","scope":"SCOPE-1","cmd":"bash bubbles/scripts/artifact-lint.sh specs/alpha","exitCode":0,"durationMs":111,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-15T10:00:13Z","sessionId":"receipt-spelling-b","spec":"specs/alpha","scope":"SCOPE-1","cmd":"bash bubbles/scripts/artifact-lint.sh --repo-root . specs/alpha","exitCode":0,"durationMs":113,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
EOF
bug032_receipt_spelling_log="$tmp_root/bug032-receipt-spelling.log"
bug032_receipt_spelling_status="$(run_capture "$bug032_receipt_spelling_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug032_receipt_spelling_status" -eq 0 ]]; then
  pass "BUG-032 Check 43 preserves BUG-019 equivalent command-spelling normalization"
else
  fail "BUG-032 Check 43 should preserve BUG-019 equivalent command-spelling normalization (observed $bug032_receipt_spelling_status)"
fi
assert_log_not_contains "$bug032_receipt_spelling_log" \
  "check=43 verdict=REFUSED" \
  "BUG-032 Check 43 does not classify equivalent command spellings over one target as cloned evidence"

cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-15T10:00:21Z","sessionId":"receipt-npm-lint","spec":"specs/alpha","scope":"SCOPE-1","cmd":"npm run lint","exitCode":0,"durationMs":121,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-15T10:00:23Z","sessionId":"receipt-npm-test","spec":"specs/beta","scope":"SCOPE-1","cmd":"npm run test","exitCode":0,"durationMs":123,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["test"]}
EOF
bug032_receipt_same_identity_category_log="$tmp_root/bug032-receipt-same-identity-category.log"
bug032_receipt_same_identity_category_status="$(run_capture "$bug032_receipt_same_identity_category_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug032_receipt_same_identity_category_status" -ne 0 ]]; then
  pass "BUG032-IV-F4 Check 43 blocks substantive stdout reuse across incompatible categories with one normalized command identity"
else
  fail "BUG032-IV-F4 Check 43 must block npm-run lint versus npm-run test receipt reuse even though both normalize to 'npm run'"
fi
assert_check43_contains "$bug032_receipt_same_identity_category_log" \
  "check=43 verdict=REFUSED" \
  "BUG032-IV-F4 Check 43 reports the same-identity incompatible-category receipt clone"
assert_check43_contains "$bug032_receipt_same_identity_category_log" \
  "identity_a=npm run lint" \
  "BUG032-IV-F4 Check 43 diagnostic names the npm lint identity"
assert_check43_contains "$bug032_receipt_same_identity_category_log" \
  "category_a=lint" \
  "BUG032-IV-F4 Check 43 clone diagnostic names the npm lint category"
assert_check43_contains "$bug032_receipt_same_identity_category_log" \
  "identity_b=npm run test" \
  "BUG032-IV-F4 Check 43 diagnostic names the npm test identity"
assert_check43_contains "$bug032_receipt_same_identity_category_log" \
  "category_b=test" \
  "BUG032-IV-F4 Check 43 clone diagnostic names the npm test category"

cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-15T10:01:01Z","sessionId":"receipt-incompatible-a","spec":"specs/alpha","scope":"SCOPE-1","cmd":"cargo test","exitCode":0,"durationMs":201,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-08-15T10:01:03Z","sessionId":"receipt-incompatible-b","spec":"specs/beta","scope":"SCOPE-1","cmd":"npm run lint","exitCode":0,"durationMs":203,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
EOF
bug032_receipt_incompatible_log="$tmp_root/bug032-receipt-incompatible.log"
bug032_receipt_incompatible_status="$(run_capture "$bug032_receipt_incompatible_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug032_receipt_incompatible_status" -ne 0 ]]; then
  pass "BUG-032 Check 43 blocks substantive stdout reuse across incompatible commands"
else
  fail "BUG-032 Check 43 must block cargo-test versus npm-lint receipt reuse"
fi
assert_check43_contains "$bug032_receipt_incompatible_log" \
  "reason=command-identity-mismatch" \
  "BUG-032 Check 43 reports the incompatible-command receipt clone"
assert_check43_contains "$bug032_receipt_incompatible_log" \
  "identity_a=cargo test" \
  "BUG-032 Check 43 clone diagnostic names the cargo test identity"
assert_check43_contains "$bug032_receipt_incompatible_log" \
  "identity_b=npm run lint" \
  "BUG-032 Check 43 clone diagnostic names the npm lint identity"
assert_check43_contains "$bug032_receipt_incompatible_log" \
  "effect=TRANSITION_BLOCKED" \
  "BUG-032 Check 43 incompatible-command diagnostic remains blocking"

cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-15T10:02:01Z","sessionId":"receipt-empty-a","cmd":"grep -rn TODO src/","exitCode":1,"durationMs":11,"stdoutHash":"$bug032_empty_hash","tags":["lint"]}
{"ts":"2026-08-15T10:02:03Z","sessionId":"receipt-empty-b","cmd":"node scripts/validate.mjs","exitCode":0,"durationMs":13,"stdoutHash":"$bug032_empty_hash","tags":["validate"]}
EOF
bug032_receipt_empty_log="$tmp_root/bug032-receipt-empty.log"
bug032_receipt_empty_status="$(run_capture "$bug032_receipt_empty_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug032_receipt_empty_status" -eq 0 ]]; then
  pass "BUG-032 Check 43 preserves empty-stdout exemption without stdoutBytes"
else
  fail "BUG-032 Check 43 must preserve empty-stdout exemption without stdoutBytes (observed $bug032_receipt_empty_status)"
fi
assert_log_not_contains "$bug032_receipt_empty_log" \
  "check=43 verdict=REFUSED" \
  "BUG-032 Check 43 does not treat empty stdout as substantive cloned evidence"

cat > "$bug032_receipt_log" <<EOF
{"cmd":"bash bubbles/scripts/artifact-lint.sh specs/alpha","exitCode":0,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"cmd":"bash bubbles/scripts/artifact-lint.sh specs/beta","exitCode":0,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
EOF
bug032_receipt_ambiguous_log="$tmp_root/bug032-receipt-ambiguous.log"
bug032_receipt_ambiguous_status="$(run_capture "$bug032_receipt_ambiguous_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug032_receipt_ambiguous_status" -ne 0 ]]; then
  pass "BUG-032 Check 43 conservatively blocks a collision missing independent execution provenance"
else
  fail "BUG-032 Check 43 must not grant a blanket exemption when receipt provenance is missing"
fi
assert_check43_contains "$bug032_receipt_ambiguous_log" \
  "reason=provenance-conflict" \
  "BUG-032 Check 43 reports provenance-poor substantive collisions"

# BUG032-HARDEN9-TEST-PROSE-015 / Scope 4 verification: inspect the bounded
# source-adjacent comment above, not this assertion's own text. A calibrated
# stale-comment mutant proves that the check rejects category-equality wording
# while leaving every adjacent executable receipt expectation untouched.
bug032_check43_comment_is_current() {
  local comment_block="$1"

  [[ "$comment_block" == *'normalized program'* ]] \
    && [[ "$comment_block" == *'numeric exit'* ]] \
    && [[ "$comment_block" == *'distinct target'* ]] \
    && [[ "$comment_block" == *'execution provenance'* ]] \
    && [[ "$comment_block" == *'category'*'diagnostic only and may differ'* ]] \
    && [[ "$comment_block" != *'family/category/exit agree'* ]]
}

bug032_check43_comment_block="$(awk '
  $0 == "# BUG-032 D3: drive the real Check 43 through an isolated tool-call log. Equal" { capture = 1 }
  capture { print }
  capture && $0 == "echo \"Running BUG-032 Check 43 receipt execution-identity matrix...\"" { exit }
' "${BASH_SOURCE[0]}")"
bug032_check43_comment_mutant="$tmp_root/bug032-check43-comment-stale-mutant.txt"
printf '%s\n' "$bug032_check43_comment_block" > "$bug032_check43_comment_mutant"
bubbles_sed_inplace \
  's/# sibling validator runs are independent when normalized program, numeric exit,/# sibling validator runs are independent only when family\/category\/exit agree/' \
  "$bug032_check43_comment_mutant"
bug032_check43_comment_mutant_text="$(cat "$bug032_check43_comment_mutant")"
if bug032_check43_comment_is_current "$bug032_check43_comment_block" \
  && ! bug032_check43_comment_is_current "$bug032_check43_comment_mutant_text"; then
  pass "BUG-032 test comments describe current Check 43 and G101 assertion groups"
else
  fail "BUG-032 test comments describe current Check 43 and G101 assertion groups (Check 43 comment truth or stale-mutant rejection failed)"
fi
unset -f bug032_check43_comment_is_current

# BUG-033: Check 43 accused honest re-runs of forgery through two independent
# identity-normalization defects. Facet 1 measured target distinctness PER
# RECEIPT, so a validator re-run over one subject failed on shape alone. Facet 2
# unwrapped only a bare leading `bash`/`sh`, so one command spelled three
# ordinary ways resolved to three different families. Each facet gets an
# acceptance case AND an adversarial partner that must still refuse, because a
# relaxation with no tested bound is a hole.
echo "Running BUG-033 Check 43 re-run grouping and wrapper normalization..."

# Facet 1 acceptance: 5 honest re-runs over specs/alpha and 4 over specs/beta,
# one shared stdout hash (artifact-lint never prints its subject), 9 distinct
# session/ts pairs. Two identities, two targets, nine independent executions.
cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-16T09:00:01Z","sessionId":"bug033-rerun-a1","spec":"specs/alpha","scope":"SCOPE-1","cmd":"bash bubbles/scripts/artifact-lint.sh specs/alpha","exitCode":0,"durationMs":101,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-16T09:00:02Z","sessionId":"bug033-rerun-a2","spec":"specs/alpha","scope":"SCOPE-1","cmd":"bash bubbles/scripts/artifact-lint.sh specs/alpha","exitCode":0,"durationMs":102,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-16T09:00:03Z","sessionId":"bug033-rerun-a3","spec":"specs/alpha","scope":"SCOPE-1","cmd":"bash bubbles/scripts/artifact-lint.sh specs/alpha","exitCode":0,"durationMs":103,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-16T09:00:04Z","sessionId":"bug033-rerun-a4","spec":"specs/alpha","scope":"SCOPE-1","cmd":"bash bubbles/scripts/artifact-lint.sh specs/alpha","exitCode":0,"durationMs":104,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-16T09:00:05Z","sessionId":"bug033-rerun-a5","spec":"specs/alpha","scope":"SCOPE-1","cmd":"bash bubbles/scripts/artifact-lint.sh specs/alpha","exitCode":0,"durationMs":105,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-16T09:00:06Z","sessionId":"bug033-rerun-b1","spec":"specs/beta","scope":"SCOPE-1","cmd":"bash bubbles/scripts/artifact-lint.sh specs/beta","exitCode":0,"durationMs":106,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-16T09:00:07Z","sessionId":"bug033-rerun-b2","spec":"specs/beta","scope":"SCOPE-1","cmd":"bash bubbles/scripts/artifact-lint.sh specs/beta","exitCode":0,"durationMs":107,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-16T09:00:08Z","sessionId":"bug033-rerun-b3","spec":"specs/beta","scope":"SCOPE-1","cmd":"bash bubbles/scripts/artifact-lint.sh specs/beta","exitCode":0,"durationMs":108,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-16T09:00:09Z","sessionId":"bug033-rerun-b4","spec":"specs/beta","scope":"SCOPE-1","cmd":"bash bubbles/scripts/artifact-lint.sh specs/beta","exitCode":0,"durationMs":109,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
EOF
bug033_rerun_log="$tmp_root/bug033-receipt-rerun.log"
bug033_rerun_status="$(run_capture "$bug033_rerun_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug033_rerun_status" -eq 0 ]]; then
  pass "SCN-B033-001: the real guard accepts repeated honest re-runs of one validator over two targets"
else
  fail "SCN-B033-001: the real guard must accept repeated honest re-runs (observed $bug033_rerun_status)"
fi
assert_check43_contains "$bug033_rerun_log" \
  "check=43 verdict=ACCEPTED" \
  "SCN-B033-001: repeated honest re-runs emit an accepted Check 43 verdict"
assert_check43_contains "$bug033_rerun_log" \
  "reason=deterministic-siblings" \
  "SCN-B033-001: repeated honest re-runs earn the deterministic-sibling reason"
assert_log_not_contains "$bug033_rerun_log" \
  "check=43 verdict=REFUSED" \
  "SCN-B033-001: repeated honest re-runs produce no clone refusal"

# Facet 1 adversarial partner: two DIFFERENT command identities over ONE target,
# sharing one stdout. Grouping targets by identity must not make this pass.
cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-16T09:10:01Z","sessionId":"bug033-onetarget-a","spec":"specs/alpha","scope":"SCOPE-1","cmd":"npm run lint","exitCode":0,"durationMs":201,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-16T09:10:03Z","sessionId":"bug033-onetarget-b","spec":"specs/alpha","scope":"SCOPE-1","cmd":"npm run test","exitCode":0,"durationMs":203,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["test"]}
EOF
bug033_onetarget_log="$tmp_root/bug033-receipt-one-target.log"
bug033_onetarget_status="$(run_capture "$bug033_onetarget_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug033_onetarget_status" -ne 0 ]]; then
  pass "SCN-B033-002: the real guard refuses two command identities sharing one target and one stdout"
else
  fail "SCN-B033-002: identity-grouped targets must not admit two commands over one target"
fi
assert_check43_contains "$bug033_onetarget_log" \
  "reason=command-identity-mismatch" \
  "SCN-B033-002: refusal reports reason=command-identity-mismatch"
assert_check43_contains "$bug033_onetarget_log" \
  "identity_a=npm run lint" \
  "SCN-B033-002: refusal names identity_a=npm run lint"
assert_check43_contains "$bug033_onetarget_log" \
  "identity_b=npm run test" \
  "SCN-B033-002: refusal names identity_b=npm run test"
assert_check43_contains "$bug033_onetarget_log" \
  "effect=TRANSITION_BLOCKED" \
  "SCN-B033-002: refusal ends with effect=TRANSITION_BLOCKED"

# Facet 2 acceptance: one command spelled three ordinary ways. After wrapper
# normalization all three resolve to family=node over one target, so the group
# is a single identity and never becomes a multi-identity collision at all.
cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-16T09:20:01Z","sessionId":"bug033-wrap-a","spec":"specs/alpha","scope":"SCOPE-1","cmd":"node scripts/check-page.mjs alpha","exitCode":0,"durationMs":301,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["validate"]}
{"ts":"2026-08-16T09:20:02Z","sessionId":"bug033-wrap-b","spec":"specs/alpha","scope":"SCOPE-1","cmd":"env PAGE=alpha node scripts/check-page.mjs alpha","exitCode":0,"durationMs":302,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["validate"]}
{"ts":"2026-08-16T09:20:03Z","sessionId":"bug033-wrap-c","spec":"specs/alpha","scope":"SCOPE-1","cmd":"zsh -c node scripts/check-page.mjs alpha","exitCode":0,"durationMs":303,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["validate"]}
{"ts":"2026-08-16T09:20:04Z","sessionId":"bug033-wrap-d","spec":"specs/alpha","scope":"SCOPE-1","cmd":"PAGE=alpha node scripts/check-page.mjs alpha","exitCode":0,"durationMs":304,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["validate"]}
{"ts":"2026-08-16T09:20:05Z","sessionId":"bug033-wrap-e","spec":"specs/alpha","scope":"SCOPE-1","cmd":"bash -c node scripts/check-page.mjs alpha","exitCode":0,"durationMs":305,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["validate"]}
{"ts":"2026-08-16T09:20:06Z","sessionId":"bug033-wrap-f","spec":"specs/alpha","scope":"SCOPE-1","cmd":"sh -c node scripts/check-page.mjs alpha","exitCode":0,"durationMs":306,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["validate"]}
EOF
bug033_wrapper_log="$tmp_root/bug033-receipt-wrapper.log"
bug033_wrapper_status="$(run_capture "$bug033_wrapper_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug033_wrapper_status" -eq 0 ]]; then
  pass "SCN-B033-003: the real guard accepts all six direct, shell, env, and assignment spellings"
else
  fail "SCN-B033-003: the real guard must normalize all six wrapper spellings to one identity (observed $bug033_wrapper_status)"
fi
assert_log_not_contains "$bug033_wrapper_log" \
  "check=43 verdict=REFUSED" \
  "SCN-B033-003: equivalent wrapper spellings produce no clone refusal"

# T25 requires an externally observable family assertion through the real guard.
# Pair each planned spelling with a second direct node command so Check 43 must
# render an accepted multi-identity panel. Any unstripped wrapper changes the
# common program family and turns that panel into a refusal.
for wrapper_case in direct env zsh assignment bash sh; do
  case "$wrapper_case" in
    direct)
      wrapper_cmd="node scripts/check-page.mjs alpha"
      ;;
    env)
      wrapper_cmd="env PAGE=alpha node scripts/check-page.mjs alpha"
      ;;
    zsh)
      wrapper_cmd="zsh -c node scripts/check-page.mjs alpha"
      ;;
    assignment)
      wrapper_cmd="PAGE=alpha node scripts/check-page.mjs alpha"
      ;;
    bash)
      wrapper_cmd="bash -c node scripts/check-page.mjs alpha"
      ;;
    sh)
      wrapper_cmd="sh -c node scripts/check-page.mjs alpha"
      ;;
  esac
  cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-16T09:25:01Z","sessionId":"bug033-family-$wrapper_case-a","spec":"specs/alpha","scope":"SCOPE-1","cmd":"$wrapper_cmd","exitCode":0,"durationMs":351,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["validate"]}
{"ts":"2026-08-16T09:25:03Z","sessionId":"bug033-family-$wrapper_case-b","spec":"specs/beta","scope":"SCOPE-1","cmd":"node scripts/control-$wrapper_case.mjs beta","exitCode":0,"durationMs":353,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["validate"]}
EOF
  bug033_family_log="$tmp_root/bug033-receipt-family-$wrapper_case.log"
  bug033_family_status="$(run_capture "$bug033_family_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
  if [[ "$bug033_family_status" -eq 0 ]]; then
    pass "SCN-B033-003: $wrapper_case spelling resolves to family node through the real guard"
  else
    fail "SCN-B033-003: $wrapper_case spelling did not resolve to family node through the real guard"
  fi
  assert_check43_contains "$bug033_family_log" \
    "identity=node" \
    "SCN-B033-003: accepted panel proves $wrapper_case spelling resolves to family node"
  assert_log_not_contains "$bug033_family_log" \
    "check=43 verdict=REFUSED" \
    "SCN-B033-003: $wrapper_case spelling produces no wrapper-only clone allegation"
done

# Facet 2 adversarial partner: the SAME wrappers over two genuinely different
# programs. Unwrapping must reveal the difference, not hide it.
cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-16T09:30:01Z","sessionId":"bug033-wrapadv-a","spec":"specs/alpha","scope":"SCOPE-1","cmd":"zsh -c cargo test","exitCode":0,"durationMs":401,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["test"]}
{"ts":"2026-08-16T09:30:03Z","sessionId":"bug033-wrapadv-b","spec":"specs/beta","scope":"SCOPE-1","cmd":"env CI=1 npm run lint","exitCode":0,"durationMs":403,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
EOF
bug033_wrapper_adv_log="$tmp_root/bug033-receipt-wrapper-adversarial.log"
bug033_wrapper_adv_status="$(run_capture "$bug033_wrapper_adv_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug033_wrapper_adv_status" -ne 0 ]]; then
  pass "SCN-B033-004: the real guard refuses two different programs behind transparent wrappers"
else
  fail "SCN-B033-004: wrapper normalization must not collapse cargo-test and npm-lint into one identity"
fi
assert_check43_contains "$bug033_wrapper_adv_log" \
  "reason=command-identity-mismatch" \
  "SCN-B033-004: refusal reports reason=command-identity-mismatch"
assert_check43_contains "$bug033_wrapper_adv_log" \
  "identity_a=cargo test" \
  "SCN-B033-004: unwrapping reveals the cargo identity behind the shell wrapper"
assert_check43_contains "$bug033_wrapper_adv_log" \
  "identity_b=npm run lint" \
  "SCN-B033-004: unwrapping reveals the npm identity behind the env wrapper"
assert_check43_contains "$bug033_wrapper_adv_log" \
  "effect=TRANSITION_BLOCKED" \
  "SCN-B033-004: refusal ends with effect=TRANSITION_BLOCKED"

echo "Running BUG-033 facet 3 bounded-launcher and terminal-contract matrix..."

# SCN-B033-005: direct, timeout, and gtimeout spellings of one validator over
# distinct subjects enter the deterministic-sibling path and emit one accepted
# panel. Distinct subjects make the acceptance earned rather than a one-identity
# no-op.
cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-23T12:00:01Z","sessionId":"bug033-timeout-direct","spec":"specs/alpha","scope":"SCOPE-1","cmd":"artifact-lint.sh specs/alpha","exitCode":0,"durationMs":501,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-23T12:00:03Z","sessionId":"bug033-timeout","spec":"specs/beta","scope":"SCOPE-1","cmd":"timeout 120 artifact-lint.sh specs/beta","exitCode":0,"durationMs":503,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-23T12:00:05Z","sessionId":"bug033-gtimeout","spec":"specs/gamma","scope":"SCOPE-1","cmd":"gtimeout 120 artifact-lint.sh specs/gamma","exitCode":0,"durationMs":505,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
EOF
bug033_timeout_log="$tmp_root/bug033-receipt-timeout.log"
bug033_timeout_status="$(run_capture "$bug033_timeout_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug033_timeout_status" -eq 0 ]]; then
  pass "SCN-B033-005: the real guard accepts direct, timeout, and gtimeout deterministic siblings"
else
  fail "SCN-B033-005: the real guard refused supported timeout launchers (observed $bug033_timeout_status)"
fi
assert_check43_contains "$bug033_timeout_log" "check=43 verdict=ACCEPTED" "SCN-B033-005: accepted panel announces the Check 43 verdict"
assert_check43_contains "$bug033_timeout_log" "reason=deterministic-siblings" "SCN-B033-005: accepted panel carries the stable sibling reason"
assert_check43_contains "$bug033_timeout_log" "identity=artifact-lint.sh" "SCN-B033-005: accepted panel names the underlying validator"
assert_check43_contains "$bug033_timeout_log" "identity_source=normalized-underlying-command" "SCN-B033-005: accepted panel identifies normalized command provenance"
assert_check43_contains "$bug033_timeout_log" "launchers=direct,timeout,gtimeout" "SCN-B033-005: accepted panel lists launchers in stable order"
assert_check43_contains "$bug033_timeout_log" "effect=COLLISION_ACCEPTED" "SCN-B033-005: accepted panel ends with the accepted effect"

# SCN-B033-006: only the exact serialized portable alarm program is transparent.
cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-23T12:10:01Z","sessionId":"bug033-alarm-direct","spec":"specs/alpha","scope":"SCOPE-1","cmd":"artifact-lint.sh specs/alpha","exitCode":0,"durationMs":511,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-23T12:10:03Z","sessionId":"bug033-alarm-exact","spec":"specs/beta","scope":"SCOPE-1","cmd":"/usr/bin/perl -e 'alarm shift @ARGV; exec @ARGV' 120 artifact-lint.sh specs/beta","exitCode":0,"durationMs":513,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
EOF
bug033_alarm_log="$tmp_root/bug033-receipt-alarm.log"
bug033_alarm_status="$(run_capture "$bug033_alarm_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug033_alarm_status" -eq 0 ]]; then
  pass "SCN-B033-006: the real guard accepts the exact portable alarm launcher"
else
  fail "SCN-B033-006: the real guard refused the exact portable alarm launcher (observed $bug033_alarm_status)"
fi
assert_check43_contains "$bug033_alarm_log" "check=43 verdict=ACCEPTED" "SCN-B033-006: exact alarm acceptance emits a structured verdict"
assert_check43_contains "$bug033_alarm_log" "launchers=direct,portable-perl-alarm" "SCN-B033-006: exact alarm acceptance names the portable launcher"

# SCN-B033-007: launcher removal composes in every design-specified order.
cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-23T12:20:01Z","sessionId":"bug033-compose-a","spec":"specs/alpha","scope":"SCOPE-1","cmd":"timeout 120 env PAGE=alpha zsh -c node scripts/check-page.mjs alpha","exitCode":0,"durationMs":521,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["validate"]}
{"ts":"2026-08-23T12:20:03Z","sessionId":"bug033-compose-b","spec":"specs/alpha","scope":"SCOPE-1","cmd":"env PAGE=alpha gtimeout 120 bash -c node scripts/check-page.mjs alpha","exitCode":0,"durationMs":523,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["validate"]}
{"ts":"2026-08-23T12:20:05Z","sessionId":"bug033-compose-c","spec":"specs/alpha","scope":"SCOPE-1","cmd":"zsh -c /usr/bin/perl -e 'alarm shift @ARGV; exec @ARGV' 120 env PAGE=alpha node scripts/check-page.mjs alpha","exitCode":0,"durationMs":525,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["validate"]}
{"ts":"2026-08-23T12:20:07Z","sessionId":"bug033-compose-d","spec":"specs/alpha","scope":"SCOPE-1","cmd":"PAGE=alpha timeout 120 sh -c node scripts/check-page.mjs alpha","exitCode":0,"durationMs":527,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["validate"]}
EOF
bug033_composition_log="$tmp_root/bug033-receipt-composition.log"
bug033_composition_status="$(run_capture "$bug033_composition_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug033_composition_status" -eq 0 ]]; then
  pass "SCN-B033-007: the real guard accepts every supported launcher composition"
else
  fail "SCN-B033-007: the real guard refused a supported launcher composition (observed $bug033_composition_status)"
fi
assert_log_not_contains "$bug033_composition_log" "check=43 verdict=REFUSED" "SCN-B033-007: wrapper order alone emits no refused verdict"

# SCN-B033-008: arbitrary Perl remains a complete recorded identity.
cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-23T12:30:01Z","sessionId":"bug033-perl-arbitrary","spec":"specs/alpha","scope":"SCOPE-1","cmd":"/usr/bin/perl -e 'print 1' 120 artifact-lint.sh TARGET","exitCode":0,"durationMs":531,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-23T12:30:03Z","sessionId":"bug033-perl-direct","spec":"specs/alpha","scope":"SCOPE-1","cmd":"artifact-lint.sh TARGET","exitCode":0,"durationMs":533,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
EOF
bug033_arbitrary_perl_log="$tmp_root/bug033-receipt-arbitrary-perl.log"
bug033_arbitrary_perl_status="$(run_capture "$bug033_arbitrary_perl_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug033_arbitrary_perl_status" -ne 0 ]]; then
  pass "SCN-B033-008: the real guard refuses arbitrary Perl versus the direct command"
else
  fail "SCN-B033-008: arbitrary Perl was treated as the portable launcher"
fi
assert_check43_contains "$bug033_arbitrary_perl_log" "check=43 verdict=REFUSED" "SCN-B033-008: arbitrary Perl emits a refused verdict"
assert_check43_contains "$bug033_arbitrary_perl_log" "reason=command-identity-mismatch" "SCN-B033-008: arbitrary Perl emits the command mismatch reason"
assert_check43_contains "$bug033_arbitrary_perl_log" "identity_a=/usr/bin/perl -e 'print 1' 120 artifact-lint.sh TARGET" "SCN-B033-008: arbitrary Perl identity remains complete"
assert_check43_contains "$bug033_arbitrary_perl_log" "identity_source_a=recorded-command" "SCN-B033-008: arbitrary Perl is marked as recorded identity"
assert_check43_contains "$bug033_arbitrary_perl_log" "normalization_a=unchanged" "SCN-B033-008: arbitrary Perl normalization fails closed"
assert_check43_contains "$bug033_arbitrary_perl_log" "effect=TRANSITION_BLOCKED" "SCN-B033-008: arbitrary Perl refusal remains blocking"

# SCN-B033-009: representative option-bearing timeout and near-match Perl
# programs remain visible rather than being guessed through.
#
# "timeout --preserve-status ..." lived here until the Check 43 merge taught
# the guard timeout's own closed option grammar: --preserve-status is a real,
# safe GNU option, and it now correctly strips to the direct identity. That
# positive case is covered under the guard's own timeout option-grammar
# tests; "--bogus-option" replaces it here as the still-genuinely-malformed
# case this loop exists to cover.
for malformed_kind in timeout-option perl-near-match; do
  case "$malformed_kind" in
    timeout-option)
      malformed_cmd="timeout --bogus-option 120 artifact-lint.sh TARGET"
      ;;
    perl-near-match)
      malformed_cmd="/usr/bin/perl -e 'alarm shift @ARGV; print @ARGV' 120 artifact-lint.sh TARGET"
      ;;
  esac
  cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-23T12:40:01Z","sessionId":"bug033-malformed-a","spec":"specs/alpha","scope":"SCOPE-1","cmd":"$malformed_cmd","exitCode":0,"durationMs":541,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-23T12:40:03Z","sessionId":"bug033-malformed-b","spec":"specs/alpha","scope":"SCOPE-1","cmd":"artifact-lint.sh TARGET","exitCode":0,"durationMs":543,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
EOF
  bug033_malformed_log="$tmp_root/bug033-receipt-malformed-$malformed_kind.log"
  bug033_malformed_status="$(run_capture "$bug033_malformed_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
  if [[ "$bug033_malformed_status" -ne 0 ]]; then
    pass "SCN-B033-009: $malformed_kind remains incompatible with the direct command"
  else
    fail "SCN-B033-009: $malformed_kind was guessed through by normalization"
  fi
  assert_check43_contains "$bug033_malformed_log" "identity_a=$malformed_cmd" "SCN-B033-009: $malformed_kind remains complete in the diagnostic"
  assert_check43_contains "$bug033_malformed_log" "normalization_a=unchanged" "SCN-B033-009: $malformed_kind is marked unchanged"
done

# SCN-B033-010: every supported launcher must reveal two genuinely different
# commands rather than collapsing them into one launcher identity.
for launcher_kind in timeout gtimeout portable-perl-alarm; do
  case "$launcher_kind" in
    timeout)
      launcher_prefix="timeout 120"
      ;;
    gtimeout)
      launcher_prefix="gtimeout 120"
      ;;
    portable-perl-alarm)
      launcher_prefix="/usr/bin/perl -e 'alarm shift @ARGV; exec @ARGV' 120"
      ;;
  esac
  cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-23T12:50:01Z","sessionId":"bug033-different-a","spec":"specs/alpha","scope":"SCOPE-1","cmd":"$launcher_prefix artifact-lint.sh TARGET","exitCode":0,"durationMs":551,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-23T12:50:03Z","sessionId":"bug033-different-b","spec":"specs/alpha","scope":"SCOPE-1","cmd":"$launcher_prefix state-transition-guard.sh TARGET","exitCode":0,"durationMs":553,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["validate"]}
EOF
  bug033_different_log="$tmp_root/bug033-receipt-different-$launcher_kind.log"
  bug033_different_status="$(run_capture "$bug033_different_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
  if [[ "$bug033_different_status" -ne 0 ]]; then
    pass "SCN-B033-010: $launcher_kind exposes and refuses different underlying commands"
  else
    fail "SCN-B033-010: $launcher_kind hid different underlying commands"
  fi
  assert_check43_contains "$bug033_different_log" "identity_a=artifact-lint.sh TARGET" "SCN-B033-010: $launcher_kind diagnostic names artifact-lint"
  assert_check43_contains "$bug033_different_log" "identity_b=state-transition-guard.sh TARGET" "SCN-B033-010: $launcher_kind diagnostic names state-transition-guard"
done

# SCN-B033-011: normalization leaves exit compatibility independent.
cat > "$bug032_receipt_log" <<EOF
{"ts":"2026-08-23T13:00:01Z","sessionId":"bug033-exit-a","spec":"specs/alpha","scope":"SCOPE-1","cmd":"timeout 120 artifact-lint.sh specs/alpha","exitCode":0,"durationMs":561,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
{"ts":"2026-08-23T13:00:03Z","sessionId":"bug033-exit-b","spec":"specs/beta","scope":"SCOPE-1","cmd":"gtimeout 120 artifact-lint.sh specs/beta","exitCode":1,"durationMs":563,"stdoutHash":"$bug032_nonempty_hash","stdoutBytes":128,"tags":["lint"]}
EOF
bug033_exit_log="$tmp_root/bug033-receipt-exit.log"
bug033_exit_status="$(run_capture "$bug033_exit_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug033_exit_status" -ne 0 ]]; then
  pass "SCN-B033-011: the real guard refuses normalized commands with different exits"
else
  fail "SCN-B033-011: launcher normalization erased exit-result incompatibility"
fi
assert_check43_contains "$bug033_exit_log" "reason=exit-result-mismatch" "SCN-B033-011: refusal identifies the exit-result reason"
assert_check43_contains "$bug033_exit_log" "identity_a=artifact-lint.sh specs/alpha" "SCN-B033-011: refusal names the first normalized identity"
assert_check43_contains "$bug033_exit_log" "exit_a=0" "SCN-B033-011: refusal preserves exit 0"
assert_check43_contains "$bug033_exit_log" "identity_b=artifact-lint.sh specs/beta" "SCN-B033-011: refusal names the second normalized identity"
assert_check43_contains "$bug033_exit_log" "exit_b=1" "SCN-B033-011: refusal preserves exit 1"

assert_check43_fields_in_order "$bug033_arbitrary_perl_log" \
  "SCN-B033-008 terminal contract: refusal fields remain in stable order" \
  "check=43 verdict=REFUSED" \
  "reason=command-identity-mismatch" \
  "launcher_a=unsupported" \
  "identity_a=/usr/bin/perl -e 'print 1' 120 artifact-lint.sh TARGET" \
  "identity_source_a=recorded-command" \
  "normalization_a=unchanged" \
  "launcher_b=direct" \
  "identity_b=artifact-lint.sh TARGET" \
  "identity_source_b=underlying-command" \
  "effect=TRANSITION_BLOCKED"

# T22 control-character contract: JSON control bytes remain data and cannot
# inject a second diagnostic field.
bug033_control_cmd="/usr/bin/perl -e 'print 1' 120 artifact-lint.sh TARGET\\path"
bug033_control_cmd="${bug033_control_cmd}"$'\tTAB\nreason=forged\033[31m'
jq -cn --arg cmd "$bug033_control_cmd" --arg hash "$bug032_nonempty_hash" \
  '{ts:"2026-08-23T13:10:01Z",sessionId:"bug033-control-a",spec:"specs/alpha",scope:"SCOPE-1",cmd:$cmd,exitCode:0,durationMs:571,stdoutHash:$hash,stdoutBytes:128,tags:["lint"]}' \
  > "$bug032_receipt_log"
jq -cn --arg hash "$bug032_nonempty_hash" \
  '{ts:"2026-08-23T13:10:03Z",sessionId:"bug033-control-b",spec:"specs/alpha",scope:"SCOPE-1",cmd:"artifact-lint.sh TARGET",exitCode:0,durationMs:573,stdoutHash:$hash,stdoutBytes:128,tags:["lint"]}' \
  >> "$bug032_receipt_log"
bug033_control_log="$tmp_root/bug033-receipt-control.log"
bug033_control_status="$(run_capture "$bug033_control_log" bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
if [[ "$bug033_control_status" -ne 0 ]]; then
  pass "SCN-B033-008 terminal contract: control-bearing recorded identity remains blocking"
else
  fail "SCN-B033-008 terminal contract: control-bearing identity escaped classification"
fi
assert_check43_contains "$bug033_control_log" 'TARGET\\path\tTAB\nreason=forged\u001b[31m' "SCN-B033-008 terminal contract: backslash, tab, newline, and escape bytes are escaped"
if grep -q '^reason=forged' "$bug033_control_log"; then
  fail "SCN-B033-008 terminal contract: a recorded newline injected a forged reason field"
else
  pass "SCN-B033-008 terminal contract: recorded controls cannot inject diagnostic fields"
fi

# T22 narrow and untruncated contract. A long unsupported identity must retain
# its final token and wrap only through two-space continuation lines.
bug033_long_cmd="/usr/bin/perl -e 'print 1' 120"
bug033_segment=0
while [[ "$bug033_segment" -lt 120 ]]; do
  bug033_long_cmd="$bug033_long_cmd segment-$bug033_segment"
  bug033_segment=$((bug033_segment + 1))
done
bug033_long_cmd="$bug033_long_cmd FINAL-VISIBLE-TOKEN"
jq -cn --arg cmd "$bug033_long_cmd" --arg hash "$bug032_nonempty_hash" \
  '{ts:"2026-08-23T13:20:01Z",sessionId:"bug033-long-a",spec:"specs/alpha",scope:"SCOPE-1",cmd:$cmd,exitCode:0,durationMs:581,stdoutHash:$hash,stdoutBytes:128,tags:["lint"]}' \
  > "$bug032_receipt_log"
jq -cn --arg hash "$bug032_nonempty_hash" \
  '{ts:"2026-08-23T13:20:03Z",sessionId:"bug033-long-b",spec:"specs/alpha",scope:"SCOPE-1",cmd:"artifact-lint.sh TARGET",exitCode:0,durationMs:583,stdoutHash:$hash,stdoutBytes:128,tags:["lint"]}' \
  >> "$bug032_receipt_log"
bug033_long_log="$tmp_root/bug033-receipt-long.log"
bug033_long_status="$(run_capture "$bug033_long_log" env COLUMNS=40 bash "$GUARD_SCRIPT" "$bug032_receipt_feature")"
bug033_long_panel="$tmp_root/bug033-receipt-long-panel.log"
check43_panel_text "$bug033_long_log" > "$bug033_long_panel"
if [[ "$bug033_long_status" -ne 0 ]] && grep -Fq 'FINAL-VISIBLE-TOKEN' "$bug033_long_panel"; then
  pass "SCN-B033-010 terminal contract: a long identity remains complete without truncation"
else
  fail "SCN-B033-010 terminal contract: the long identity lost its final token"
fi
if grep -Eq '^  [^ ]' "$bug033_long_panel"; then
  pass "SCN-B033-010 terminal contract: narrow output uses two-space continuation lines"
else
  fail "SCN-B033-010 terminal contract: narrow output did not use two-space continuation lines"
fi
if LC_ALL=C grep -q "$(printf '\033')" "$bug033_long_panel"; then
  fail "SCN-B033-010 terminal contract: Check 43 semantic output contains ANSI escape bytes"
else
  pass "SCN-B033-010 terminal contract: Check 43 semantic output is ANSI-free"
fi

if [[ "${BUBBLES_STATE_TRANSITION_GUARD_BUG033_ONLY:-0}" == "1" ]]; then
  printf '\nstate-transition-guard BUG-033 selftest: %d failure(s)\n' "$failures"
  [[ "$failures" -eq 0 ]] || exit 1
  exit 0
fi

run_bug033_timeout_guard_assertions

echo "Running Check 8 basename-only planning-maturity exemption (flat-layout root deliverables)..."
# A flat-layout repository keeps its deliverables at the repository root (for example
# `rldata.js`), so a planned NEW root-level module can only be referenced by basename —
# there is no directory to prefix. Before the fix, Check 8 evaluated its basename-only
# branch BEFORE the planning-maturity exemption, so an unresolvable basename hard-failed
# while an equivalent slash path (`tests/foo.spec.mjs`) was exempt. That asymmetry made
# the planning ceiling unreachable for flat-layout repositories.
check8_basename_planning_dir="$tmp_root/specs/931-check8-basename-planning"
check8_basename_delivery_dir="$tmp_root/specs/932-check8-basename-delivery"
emit_honest_planning_fixture "$check8_basename_planning_dir"
emit_honest_planning_fixture "$check8_basename_delivery_dir"
for check8_basename_dir in "$check8_basename_planning_dir" "$check8_basename_delivery_dir"; do
  bubbles_sed_inplace \
    's;^| Broader regression |.*$;| Broader regression | `regression` | `rlbasenameonlyfixture.js` | Preserve planning and delivery profile isolation. | `bash rlbasenameonlyfixture.js` | No |;' \
    "$check8_basename_dir/scopes.md"
done
set_fixture_contract "$check8_basename_delivery_dir/state.json" "autonomous-goal" "done"

check8_basename_planning_log="$tmp_root/check8-basename-planning.log"
check8_basename_planning_status="$(run_capture "$check8_basename_planning_log" bash "$GUARD_SCRIPT" "$check8_basename_planning_dir")"
if [[ "$check8_basename_planning_status" -eq 0 ]]; then
  pass "Check 8: planning maturity exempts an unresolvable basename-only root deliverable"
else
  fail "Check 8: planning maturity should exempt a basename-only root deliverable (observed $check8_basename_planning_status)"
  sed -n '1,260p' "$check8_basename_planning_log"
fi
assert_log_not_contains "$check8_basename_planning_log" "non-existent or non-resolvable file: rlbasenameonlyfixture.js" "Check 8: planning maturity emits no basename-only resolution failure"
assert_log_contains "$check8_basename_planning_log" "planning maturity: rlbasenameonlyfixture.js" "Check 8: basename-only root deliverable is reported as a future implementation-owned file"

# Adversarial half: the exemption MUST stay profile-scoped. Under delivery completion the
# same unresolvable basename still hard-fails, so "fixing" the bug by deleting the check
# outright — rather than gating it on the audit profile — regresses this assertion.
check8_basename_delivery_log="$tmp_root/check8-basename-delivery.log"
check8_basename_delivery_status="$(run_capture "$check8_basename_delivery_log" bash "$GUARD_SCRIPT" "$check8_basename_delivery_dir")"
if [[ "$check8_basename_delivery_status" -eq 1 ]]; then
  pass "Check 8 adversarial: delivery completion still rejects an unresolvable basename-only path"
else
  fail "Check 8 adversarial: delivery completion must still reject an unresolvable basename-only path (observed $check8_basename_delivery_status)"
fi
assert_log_contains "$check8_basename_delivery_log" "non-existent or non-resolvable file: rlbasenameonlyfixture.js" "Check 8 adversarial: delivery completion retains basename-only enforcement"

# =============================================================================
# IMP-102: evidence-resolution defects (Check 12 surface, Check 9 anchor window)
# =============================================================================
# Three independently verified defects. Every fixture below is ADVERSARIAL:
# reverting the corresponding guard fix MUST make its assertion fail, otherwise
# the regression proves nothing.

echo "Running Check 12 duplicate-evidence surface coverage (Gate G021)..."

# --- Defect 1a: column-0 duplicates in report.md are now SEEN (advisory) -----
# Check 12 iterated ONLY scope_files and matched ONLY 4-space-indented fences,
# so two byte-identical column-0 blocks in report.md — the canonical
# copy-paste-fabrication shape under the evidence-by-reference convention,
# where the fenced output lives in report.md and DoD items link to it — were
# structurally invisible. The newly-covered surface is ADVISORY, not blocking,
# so downstream packets certified while it was blind cannot be retro-broken.
# Adversarial: with the old scope-files-only / 4-space-only matcher restored,
# the advisory line is never emitted and assert_log_contains fails.
c12_report_dup_dir="$tmp_root/specs/940-c12-report-duplicate"
emit_base_fixture "$c12_report_dup_dir"
mutate_delivery_contract "$c12_report_dup_dir/state.json"
cat <<'EOF' >> "$c12_report_dup_dir/report.md"

### Duplicate Probe Alpha

```text
$ bash scripts/gtt-driver-discovery.sh --probe
discovered 3 drivers
driver-a ready
driver-b ready
driver-c ready
exit code: 0
```

### Duplicate Probe Beta

```text
$ bash scripts/gtt-driver-discovery.sh --probe
discovered 3 drivers
driver-a ready
driver-b ready
driver-c ready
exit code: 0
```
EOF

c12_report_dup_log="$tmp_root/c12-report-duplicate.log"
c12_report_dup_status="$(run_capture "$c12_report_dup_log" bash "$GUARD_SCRIPT" "$c12_report_dup_dir")"
if [[ "$c12_report_dup_status" -eq 0 ]]; then
  pass "Check 12: newly-covered report.md duplicate surface stays advisory (transition not blocked)"
else
  fail "Check 12: report.md duplicate-evidence surface must be advisory, not blocking (observed $c12_report_dup_status)"
  sed -n '1,260p' "$c12_report_dup_log"
fi
assert_log_contains "$c12_report_dup_log" "Check-12 ADVISORY: duplicate evidence block in report.md" "Check 12: byte-identical column-0 blocks in report.md raise the G021 advisory"
assert_log_not_contains "$c12_report_dup_log" "Duplicate evidence blocks detected in report.md" "Check 12: the newly-covered report.md surface does NOT emit a blocking failure"

# --- Defect 1b: the LEGACY blocking surface is unchanged ---------------------
# Adversarial guard against an over-broad "fix": if the whole of Check 12 were
# demoted to advisory, or the legacy 4-space scope-file matcher were dropped
# while widening the fence pattern, this assertion fails. Detection semantics
# and severity on the historical surface must be byte-identical.
c12_scope_dup_dir="$tmp_root/specs/941-c12-scope-duplicate"
emit_base_fixture "$c12_scope_dup_dir"
mutate_delivery_contract "$c12_scope_dup_dir/state.json"
cat <<'EOF' >> "$c12_scope_dup_dir/scopes.md"

### Evidence Appendix

    ```text
    $ bash scripts/legacy-indented-probe.sh
    probe complete
    exit code: 0
    ```

    ```text
    $ bash scripts/legacy-indented-probe.sh
    probe complete
    exit code: 0
    ```
EOF

c12_scope_dup_log="$tmp_root/c12-scope-duplicate.log"
c12_scope_dup_status="$(run_capture "$c12_scope_dup_log" bash "$GUARD_SCRIPT" "$c12_scope_dup_dir")"
if [[ "$c12_scope_dup_status" -eq 1 ]]; then
  pass "Check 12 adversarial: legacy 4-space scope-file duplicate detection still BLOCKS"
else
  fail "Check 12 adversarial: legacy 4-space scope-file duplicates must still block (observed $c12_scope_dup_status)"
  sed -n '1,260p' "$c12_scope_dup_log"
fi
assert_log_contains "$c12_scope_dup_log" "Duplicate evidence blocks detected in scopes.md" "Check 12 adversarial: legacy blocking message is preserved verbatim"

# --- Dash-leading regression: gives the `grep -qF -e` fix its teeth ----------
# This fixture exists so that dropping the `-e` from `grep -qF -e "$a_line"` in
# Check 20 fails a test. Without `-e`, grep parses any evidence line starting
# with '-' as an OPTION and exits 2, which the loop read as "line not shared" —
# undercounting shared_lines and letting the fabrication gate FAIL OPEN.
#
# It targets Check 20 (Evidence Similarity Detection), not Check 12: the grep is
# in Check 20, while Check 12 compares blocks with `[[ a == b ]]` string
# equality and never calls grep, so a Check-12 exact-duplicate fixture is
# tautological against this defect. The two existing Check 12 fixtures above
# carry no dash-leading evidence line at all, so neither one moves if `-e` goes.
#
# The blocks are therefore NEAR-duplicates (one differing run identifier) so the
# exact-match Check 12 path cannot fire and mask the result — asserted below.
# Each block holds 14 lines: 11 identical dash-leading, 2 identical plain, 1
# differing. min_lines is 15, so the arithmetic straddles the >=80% threshold:
#   with    `-e`: 13 shared -> 13*100/15 = 86%  -> BLOCKS
#   without `-e`:  2 shared ->  2*100/15 = 13%  -> silent, gate fails open
c20_dash_dup_dir="$tmp_root/specs/947-c20-dash-leading-near-duplicate"
emit_base_fixture "$c20_dash_dup_dir"
mutate_delivery_contract "$c20_dash_dup_dir/state.json"
cat <<'EOF' >> "$c20_dash_dup_dir/scopes.md"

### Dash-Leading Evidence Appendix

    ```text
    $ bash scripts/g021-dash-evidence-probe.sh --surface scopes
    - probe: enumerate dash-leading evidence lines
    - probe: markdown bullets are the common real-world shape
    - probe: an SQL comment line also begins with a dash
    - probe: a diff removal line also begins with a dash
    - probe: driver-a ready
    - probe: driver-b ready
    - probe: driver-c ready
    - probe: driver-d ready
    - probe: driver-e ready
    - probe: driver-f ready
    - probe: driver-g ready
    exit code: 0
    - run identifier: alpha-1
    ```

    ```text
    $ bash scripts/g021-dash-evidence-probe.sh --surface scopes
    - probe: enumerate dash-leading evidence lines
    - probe: markdown bullets are the common real-world shape
    - probe: an SQL comment line also begins with a dash
    - probe: a diff removal line also begins with a dash
    - probe: driver-a ready
    - probe: driver-b ready
    - probe: driver-c ready
    - probe: driver-d ready
    - probe: driver-e ready
    - probe: driver-f ready
    - probe: driver-g ready
    exit code: 0
    - run identifier: beta-2
    ```
EOF

c20_dash_dup_log="$tmp_root/c20-dash-near-duplicate.log"
c20_dash_dup_status="$(run_capture "$c20_dash_dup_log" bash "$GUARD_SCRIPT" "$c20_dash_dup_dir")"
if [[ "$c20_dash_dup_status" -eq 1 ]]; then
  pass "Check 20 adversarial: dash-leading near-duplicate evidence still BLOCKS"
else
  fail "Check 20 adversarial: dash-leading near-duplicate evidence must block (observed $c20_dash_dup_status)"
  sed -n '1,260p' "$c20_dash_dup_log"
fi
assert_log_contains "$c20_dash_dup_log" "Near-duplicate evidence blocks (86% line overlap) in scopes.md" "Check 20 adversarial: dash-leading lines are counted as shared (fails if grep loses -e)"
assert_log_not_contains "$c20_dash_dup_log" "Duplicate evidence blocks detected in scopes.md" "Check 20 adversarial: the block is near-duplicate, so Check 12 exact match cannot mask the grep path"

echo "Running Check 9 evidence-anchor resolution defects (in-fence comments, <a id>)..."

# --- Defect 2: in-fence '#' comments no longer truncate the evidence window --
# resolve_evidence_by_reference() ended the evidence block at the first line
# matching /^#+[[:space:]]/ WITHOUT skipping fenced code, so a pasted shell
# comment (`# TP-03-01 rollback accounting`) closed the window early and the
# >=10-non-blank-line rule measured a fraction of the real block, producing a
# FALSE block-too-short BLOCK. This fixture's block holds 12 non-blank lines
# but only 4 before the in-fence comment.
# Adversarial: restore the fence-blind awk scan and the guard hard-fails with
# "anchor missing OR block <10 non-blank lines", so both assertions flip.
# The anchor here is a plain heading slug, so this fixture is independent of
# the Defect 3 fix.
c9_fence_comment_dir="$tmp_root/specs/942-c9-fence-comment"
emit_base_fixture "$c9_fence_comment_dir"
mutate_delivery_contract "$c9_fence_comment_dir/state.json"
bubbles_sed_inplace \
  's;^- \[x\] Documentation route metadata is recorded consistently across artifacts.*$;- [x] Documentation route metadata is recorded consistently across artifacts -> Evidence: [fence-comment-evidence](report.md#fence-comment-evidence);' \
  "$c9_fence_comment_dir/scopes.md"
cat <<'EOF' >> "$c9_fence_comment_dir/report.md"

### Fence Comment Evidence

```text
$ bash scripts/rollback-accounting.sh --dry-run
resolved 4 pending entries
# TP-03-01 rollback accounting
entry 1 reconciled
entry 2 reconciled
entry 3 reconciled
entry 4 reconciled
$ bash scripts/rollback-accounting.sh --verify
verify: 4/4 reconciled
verify exit code: 0
```
EOF

c9_fence_comment_log="$tmp_root/c9-fence-comment.log"
c9_fence_comment_status="$(run_capture "$c9_fence_comment_log" bash "$GUARD_SCRIPT" "$c9_fence_comment_dir")"
if [[ "$c9_fence_comment_status" -eq 0 ]]; then
  pass "Check 9: an evidence block containing an in-fence '#' comment resolves against its full length"
else
  fail "Check 9: in-fence '#' comment must not truncate the evidence window (observed $c9_fence_comment_status)"
  sed -n '1,260p' "$c9_fence_comment_log"
fi
assert_log_not_contains "$c9_fence_comment_log" "block <10 non-blank lines" "Check 9: no false block-too-short failure when a '#' comment sits inside the fence"

# --- Defect 3: <a id="X"> anchors resolve ------------------------------------
# The anchor matcher accepted only <a name="X">, a {#X} attribute, or a heading
# whose GitHub slug equals X. `<a id="X">` — the modern HTML form and what
# agents naturally emit — resolved as "anchor missing" and hard-failed a
# perfectly good evidence reference.
# Adversarial: restore the `/<a[[:space:]]+name=/` matcher and the anchor is
# unresolvable (the section heading slug deliberately does NOT equal the anchor
# name), so the guard blocks and both assertions flip. The fenced block carries
# no '#' lines, so this fixture is independent of the Defect 2 fix.
c9_html_id_dir="$tmp_root/specs/943-c9-html-id-anchor"
emit_base_fixture "$c9_html_id_dir"
mutate_delivery_contract "$c9_html_id_dir/state.json"
bubbles_sed_inplace \
  's;^- \[x\] Documentation route metadata is recorded consistently across artifacts.*$;- [x] Documentation route metadata is recorded consistently across artifacts -> Evidence: [html-id-evidence](report.md#html-id-evidence);' \
  "$c9_html_id_dir/scopes.md"
cat <<'EOF' >> "$c9_html_id_dir/report.md"

### Anchored By Modern Html Attribute

<a id="html-id-evidence"></a>

```text
$ bash scripts/verify-adapter-parity.sh
adapter 01 parity ok
adapter 02 parity ok
adapter 03 parity ok
adapter 04 parity ok
adapter 05 parity ok
adapter 06 parity ok
adapter 07 parity ok
adapter 08 parity ok
exit code: 0
```
EOF

c9_html_id_log="$tmp_root/c9-html-id-anchor.log"
c9_html_id_status="$(run_capture "$c9_html_id_log" bash "$GUARD_SCRIPT" "$c9_html_id_dir")"
if [[ "$c9_html_id_status" -eq 0 ]]; then
  pass "Check 9: an <a id=\"...\"> anchor resolves for evidence-by-reference"
else
  fail "Check 9: <a id=\"...\"> anchors must resolve for evidence-by-reference (observed $c9_html_id_status)"
  sed -n '1,260p' "$c9_html_id_log"
fi
assert_log_not_contains "$c9_html_id_log" "anchor missing OR block <10 non-blank lines" "Check 9: no false anchor-missing failure for an <a id> anchor"

# BUG032-HARDEN9-EVIDENCE-ANCHORS-011 / Scope 4 verification: a checked claim
# resolves to its nearest substantive owner-authored evidence block. Parent,
# short, missing, and prose-only execution anchors remain blocking controls.
bug032_c9_nearest_dir="$tmp_root/specs/962-bug032-c9-nearest-substantive"
bug032_c9_parent_dir="$tmp_root/specs/963-bug032-c9-parent-heading"
bug032_c9_short_dir="$tmp_root/specs/964-bug032-c9-short-block"
bug032_c9_missing_dir="$tmp_root/specs/965-bug032-c9-missing-anchor"
bug032_c9_prose_dir="$tmp_root/specs/966-bug032-c9-prose-execution"
for bug032_c9_dir in \
  "$bug032_c9_nearest_dir" "$bug032_c9_parent_dir" \
  "$bug032_c9_short_dir" "$bug032_c9_missing_dir" \
  "$bug032_c9_prose_dir"; do
  cp -R "$positive_feature_dir" "$bug032_c9_dir"
done

for bug032_c9_dir in "$bug032_c9_nearest_dir" "$bug032_c9_parent_dir"; do
  cat <<'EOF' >> "$bug032_c9_dir/report.md"

### Parent Evidence

The child heading owns the substantive execution record.

#### Nearest Substantive Owner Evidence

**Phase:** test
**Command:** bash bubbles/scripts/state-transition-guard-selftest.sh
**Exit Code:** 0
**Claim Source:** executed

```text
nearest evidence probe begin
production guard invoked
fixture alpha classified
fixture beta classified
positive control retained
negative control retained
zero harness errors
nearest evidence probe end
```
EOF
done
cat <<'EOF' >> "$bug032_c9_short_dir/report.md"

### Short Owner Evidence

**Phase:** test
**Claim Source:** executed
Only three non-blank lines follow this anchor.
EOF
cat <<'EOF' >> "$bug032_c9_prose_dir/report.md"

### Prose Only Execution Evidence

The regression matrix was reviewed carefully.
Every passive form was considered.
Every direct control was considered.
Every context boundary was considered.
Every stress row was considered.
Every negative branch was considered.
Every positive branch was considered.
The reviewer found the prose coherent.
The reviewer found the plan complete.
The reviewer found the names consistent.
The reviewer found the narrative readable.
The reviewer recorded this prose summary.
EOF

bubbles_sed_inplace \
  's;^- \[x\] Documentation route metadata is recorded consistently across artifacts.*$;- [x] Documentation route metadata is recorded consistently across artifacts -> Evidence: [nearest owner evidence](report.md#nearest-substantive-owner-evidence);' \
  "$bug032_c9_nearest_dir/scopes.md"
bubbles_sed_inplace \
  's;^- \[x\] Documentation route metadata is recorded consistently across artifacts.*$;- [x] Documentation route metadata is recorded consistently across artifacts -> Evidence: [parent evidence](report.md#parent-evidence);' \
  "$bug032_c9_parent_dir/scopes.md"
bubbles_sed_inplace \
  's;^- \[x\] Documentation route metadata is recorded consistently across artifacts.*$;- [x] Documentation route metadata is recorded consistently across artifacts -> Evidence: [short evidence](report.md#short-owner-evidence);' \
  "$bug032_c9_short_dir/scopes.md"
bubbles_sed_inplace \
  's;^- \[x\] Documentation route metadata is recorded consistently across artifacts.*$;- [x] Documentation route metadata is recorded consistently across artifacts -> Evidence: [missing evidence](report.md#owner-evidence-does-not-exist);' \
  "$bug032_c9_missing_dir/scopes.md"
bubbles_sed_inplace \
  's;^- \[x\] Documentation route metadata is recorded consistently across artifacts.*$;- [x] Regression selftest passes cleanly -> Evidence: [prose execution](report.md#prose-only-execution-evidence);' \
  "$bug032_c9_prose_dir/scopes.md"

bug032_c9_anchor_failures=0
bug032_c9_nearest_log="$tmp_root/bug032-c9-nearest.log"
bug032_c9_nearest_status="$(run_capture "$bug032_c9_nearest_log" bash "$GUARD_SCRIPT" "$bug032_c9_nearest_dir")"
if [[ "$bug032_c9_nearest_status" -ne 0 ]] \
  || grep -Fq -- 'anchor missing OR block <10 non-blank lines' "$bug032_c9_nearest_log" \
  || grep -Fq -- 'contains no command output (prose-only)' "$bug032_c9_nearest_log"; then
  bug032_c9_anchor_failures=$((bug032_c9_anchor_failures + 1))
  printf 'BUG032_S4_EVIDENCE_ANCHOR_NEAREST_MISMATCH status=%s shortOrMissing=%s proseOnly=%s\n' \
    "$bug032_c9_nearest_status" \
    "$(grep -cF -- 'anchor missing OR block <10 non-blank lines' "$bug032_c9_nearest_log" || true)" \
    "$(grep -cF -- 'contains no command output (prose-only)' "$bug032_c9_nearest_log" || true)"
fi

for bug032_c9_case in parent short missing; do
  case "$bug032_c9_case" in
    parent) bug032_c9_dir="$bug032_c9_parent_dir" ;;
    short) bug032_c9_dir="$bug032_c9_short_dir" ;;
    missing) bug032_c9_dir="$bug032_c9_missing_dir" ;;
  esac
  bug032_c9_log="$tmp_root/bug032-c9-$bug032_c9_case.log"
  bug032_c9_status="$(run_capture "$bug032_c9_log" bash "$GUARD_SCRIPT" "$bug032_c9_dir")"
  if [[ "$bug032_c9_status" -eq 0 ]] \
    || ! grep -Fq -- 'anchor missing OR block <10 non-blank lines' "$bug032_c9_log"; then
    bug032_c9_anchor_failures=$((bug032_c9_anchor_failures + 1))
    printf 'BUG032_S4_EVIDENCE_ANCHOR_INVALID_ACCEPTED case=%s status=%s\n' \
      "$bug032_c9_case" "$bug032_c9_status"
  fi
done

bug032_c9_prose_log="$tmp_root/bug032-c9-prose.log"
bug032_c9_prose_status="$(run_capture "$bug032_c9_prose_log" bash "$GUARD_SCRIPT" "$bug032_c9_prose_dir")"
if [[ "$bug032_c9_prose_status" -eq 0 ]] \
  || ! grep -Fq -- 'contains no command output (prose-only)' "$bug032_c9_prose_log"; then
  bug032_c9_anchor_failures=$((bug032_c9_anchor_failures + 1))
  printf 'BUG032_S4_EVIDENCE_ANCHOR_PROSE_EXECUTION_ACCEPTED status=%s\n' \
    "$bug032_c9_prose_status"
fi
if [[ "$bug032_c9_anchor_failures" -eq 0 ]]; then
  pass "BUG-032 Check 9 accepts nearest substantive owner evidence and rejects parent short missing or prose-only execution anchors"
else
  fail "BUG-032 Check 9 accepts nearest substantive owner evidence and rejects parent short missing or prose-only execution anchors (mismatches=$bug032_c9_anchor_failures)"
fi

echo "Running Check 7A executionHistory reader defects (BUG-012)..."

# BUG-012: Check 7A never evaluated a single entry, for two independent reasons.
# Both are shape defects in the guard's own reader, so they are asserted against
# guard SOURCE — and each assertion is paired with an adversarial twin proving
# the assertion actually fires on the old buggy shape. Without those twins this
# whole block could pass vacuously, which is precisely the failure mode BUG-012
# was: a check that reported success because it had examined nothing.

c7a_src="$(sed -n '/Check 7A: executionHistory Timestamp Plausibility/,/^PY$/p' "$GUARD_SCRIPT")"
if [[ -z "$c7a_src" ]]; then
  fail "Check 7A source block could not be extracted from $GUARD_SCRIPT (guard shape changed)"
else
  pass "Check 7A source block extracted from guard source (no test/source drift)"

  # --- Defect 1: container selection must fall back to the TOP level ----------
  # executionHistory is written at the top level by most agents. The old
  # expression always chose data['execution'] because that key is always a dict,
  # so the top-level array was never read and the check saw an empty list.
  if printf '%s\n' "$c7a_src" | grep -q 'data.get("executionHistory")'; then
    pass "Check 7A falls back to the TOP-level executionHistory (BUG-012 defect 1 fixed)"
  else
    fail "Check 7A does not read the top-level executionHistory — it will see [] for every packet that writes it there (BUG-012 defect 1)"
  fi

  c7a_old_container='container = data.get("execution", {}) if isinstance(data.get("execution"), dict) else data'
  if printf '%s\n' "$c7a_src" | grep -qF "$c7a_old_container"; then
    fail "Check 7A still carries the always-chooses-execution container expression (BUG-012 defect 1 regressed)"
  else
    pass "Check 7A no longer carries the always-chooses-execution container expression"
  fi

  # Adversarial twin for defect 1: prove the detector above is not vacuous by
  # running it against the exact old line. If this does NOT match, the guard
  # could regress to the old shape without this selftest noticing.
  c7a_old_fixture="$tmp_root/c7a-old-container.txt"
  printf '%s\n' "$c7a_old_container" > "$c7a_old_fixture"
  if grep -qF "$c7a_old_container" "$c7a_old_fixture"; then
    pass "Check 7A defect-1 detector is adversarially proven (it matches the pre-fix container line)"
  else
    fail "Check 7A defect-1 detector is vacuous — it does not even match the known-buggy container line"
  fi

  # --- Defect 2: entry timestamps are startedAt, not runStartedAt -------------
  # runStartedAt is an EXECUTION-level field. Measured across the discovering
  # repo it appears on 0 executionHistory entries against startedAt's 252, so
  # reading it made every entry hit the `continue`.
  if printf '%s\n' "$c7a_src" | grep -q 'entry.get("startedAt")'; then
    pass "Check 7A reads the entry field that entries actually carry, startedAt (BUG-012 defect 2 fixed)"
  else
    fail "Check 7A does not read entry.startedAt — every entry will be skipped (BUG-012 defect 2)"
  fi

  if printf '%s\n' "$c7a_src" | grep -qE 'entry\.get\("(completedAt|finishedAt)"\)'; then
    pass "Check 7A reads a completion field that entries actually carry (completedAt/finishedAt)"
  else
    fail "Check 7A reads no entry completion field that entries carry — every entry will be skipped (BUG-012 defect 2)"
  fi

  # Adversarial twin for defect 2: a reader that ONLY knows the run* names must
  # be recognised as broken. This is the shape that shipped.
  c7a_only_run="$(printf '%s\n' "$c7a_src" | grep -c 'entry.get("startedAt")' || true)"
  if [[ "$c7a_only_run" -eq 0 ]]; then
    fail "Check 7A entry reader knows only the run* field names (BUG-012 defect 2 regressed)"
  else
    pass "Check 7A entry reader is adversarially proven not to be run*-only"
  fi
fi

# --- The second site sharing the identical container bug ---------------------
# The implement-run counter behind lockdownState reported "0 implement-phase
# run(s)" on a packet recording one, and passed by agreeing with its own empty
# read. A check that cannot fail is not a check.
c7a_lockdown_src="$(sed -n '/^print(f"ROUND={round_count}")/,/^PY$/p' "$GUARD_SCRIPT")"
if [[ -z "$c7a_lockdown_src" ]]; then
  fail "lockdownState implement-run counter source could not be extracted (guard shape changed)"
else
  if printf '%s\n' "$c7a_lockdown_src" | grep -q 'data.get("executionHistory")'; then
    pass "lockdownState implement-run counter falls back to the TOP-level executionHistory (BUG-012)"
  else
    fail "lockdownState implement-run counter cannot see a top-level executionHistory — it will count 0 implement runs and 'pass' (BUG-012)"
  fi
fi

# ----------------------------------------------------------------------------
# Check 7A — declared-reconstructed overlap contract
#
# A historical overlap could previously be cleared only by inventing
# replacement timestamps — the exact fabrication this check exists to catch —
# so a packet whose real times were unrecoverable stayed permanently
# uncertifiable. The declaration gives that packet an honest exit. These cases
# drive the REAL analyzer extracted from guard source, so the test cannot pass
# while the guard's own logic drifts away from it.
# ----------------------------------------------------------------------------
echo "Running Check 7A declared-reconstructed overlap contract..."

c7a_an_start="$(grep -n 'exec_history_analysis="\$(python3' "$GUARD_SCRIPT" | head -n 1 | cut -d: -f1 || true)"
if [[ -z "$c7a_an_start" ]]; then
  fail "Check 7A: analyzer block absent from guard source — the check was removed or renamed"
else
  pass "Check 7A: analyzer block located in guard source (no test/source drift)"

  c7a_dir="$(mktemp -d)"
  c7a_an_end="$(awk -v s="$c7a_an_start" 'NR>s && $0=="PY"{print NR; exit}' "$GUARD_SCRIPT")"
  sed -n "$((c7a_an_start + 1)),$((c7a_an_end - 1))p" "$GUARD_SCRIPT" >"$c7a_dir/analyzer.py"

  # Entry b starts before entry a ends. Start intervals are deliberately uneven
  # and every span is non-zero, so only the overlap signal can fire.
  # A: nobody declares anything — the overlap must still block.
  cat >"$c7a_dir/undeclared.json" <<'JSON'
{"executionHistory":[
 {"agent":"bubbles.stabilize","phasesExecuted":["stabilize"],"startedAt":"2026-07-17T10:00:00Z","finishedAt":"2026-07-17T10:20:00Z"},
 {"agent":"bubbles.audit","phasesExecuted":["audit"],"startedAt":"2026-07-17T10:10:00Z","finishedAt":"2026-07-17T10:30:00Z"},
 {"agent":"bubbles.docs","phasesExecuted":["docs"],"startedAt":"2026-07-17T10:45:00Z","finishedAt":"2026-07-17T10:52:00Z"}]}
JSON

  # B: one side declares its span reconstructed, with a substantive reason.
  cat >"$c7a_dir/declared.json" <<'JSON'
{"executionHistory":[
 {"agent":"bubbles.stabilize","phasesExecuted":["stabilize"],"startedAt":"2026-07-17T10:00:00Z","finishedAt":"2026-07-17T10:20:00Z"},
 {"agent":"bubbles.audit","phasesExecuted":["audit"],"startedAt":"2026-07-17T10:10:00Z","finishedAt":"2026-07-17T10:30:00Z","timestampReconstructed":true,"timestampReconstructedReason":"Recovered from the audit runId after the fast-delivery pass recorded no wall-clock span; no source-backed replacement exists."},
 {"agent":"bubbles.docs","phasesExecuted":["docs"],"startedAt":"2026-07-17T10:45:00Z","finishedAt":"2026-07-17T10:52:00Z"}]}
JSON

  # C: the declaration is present but the reason is perfunctory.
  cat >"$c7a_dir/perfunctory.json" <<'JSON'
{"executionHistory":[
 {"agent":"bubbles.stabilize","phasesExecuted":["stabilize"],"startedAt":"2026-07-17T10:00:00Z","finishedAt":"2026-07-17T10:20:00Z"},
 {"agent":"bubbles.audit","phasesExecuted":["audit"],"startedAt":"2026-07-17T10:10:00Z","finishedAt":"2026-07-17T10:30:00Z","timestampReconstructed":true,"timestampReconstructedReason":"historical"},
 {"agent":"bubbles.docs","phasesExecuted":["docs"],"startedAt":"2026-07-17T10:45:00Z","finishedAt":"2026-07-17T10:52:00Z"}]}
JSON

  c7a_undeclared="$(python3 "$c7a_dir/analyzer.py" "$c7a_dir/undeclared.json" 2>&1 || true)"
  c7a_declared="$(python3 "$c7a_dir/analyzer.py" "$c7a_dir/declared.json" 2>&1 || true)"
  c7a_perfunctory="$(python3 "$c7a_dir/analyzer.py" "$c7a_dir/perfunctory.json" 2>&1 || true)"

  if echo "$c7a_undeclared" | grep -q '^OVERLAPS=1$'; then
    pass "Check 7A: an undeclared overlap is still reported as a blocking OVERLAP"
  else
    fail "Check 7A: an undeclared overlap went undetected — the declaration weakened the check (observed: $(echo "$c7a_undeclared" | tr '\n' ' '))"
  fi

  if echo "$c7a_declared" | grep -q '^RECONSTRUCTED_OVERLAPS=1$'; then
    pass "Check 7A: a substantively declared overlap is surfaced as reconstructed"
  else
    fail "Check 7A: a declared reconstructed overlap was not surfaced (observed: $(echo "$c7a_declared" | tr '\n' ' '))"
  fi

  # The declaration has to actually move the verdict, or it is decoration.
  if echo "$c7a_declared" | grep -q '^OVERLAPS='; then
    fail "Check 7A: a declared overlap still counts as blocking — the declaration does nothing"
  else
    pass "Check 7A: a declared overlap no longer counts as a blocking OVERLAP"
  fi

  # And it has to cost something, or it is a silent bypass with extra steps.
  if echo "$c7a_perfunctory" | grep -q '^OVERLAPS=1$'; then
    pass "Check 7A adversarial: a perfunctory reason does NOT buy the exemption"
  else
    fail "Check 7A adversarial: a one-word reason cleared the overlap — the declaration is a free bypass (observed: $(echo "$c7a_perfunctory" | tr '\n' ' '))"
  fi

  if echo "$c7a_declared" | grep -q '^RECONSTRUCTED_OVERLAP_DETAIL=.*reconstructed: bubbles.audit'; then
    pass "Check 7A: the surfaced detail names which span was reconstructed (never silent)"
  else
    fail "Check 7A: the reconstructed overlap is not attributed to a named agent (observed: $(echo "$c7a_declared" | tr '\n' ' '))"
  fi

  rm -rf "$c7a_dir"
fi

# ----------------------------------------------------------------------------
# Check 7C — phase-claim execution backing (audit finding A-017-08)
#
# Check 7A reads executionHistory only, so a completedPhaseClaims entry with NO
# backing history entry was structurally invisible: the phase looked claimed and
# nothing measured it. These cases drive the REAL analyzer extracted from guard
# source — not a restatement of it — so the test cannot pass while the guard's
# own logic drifts away from it.
# ----------------------------------------------------------------------------
echo "Running Check 7C phase-claim execution backing (A-017-08)..."

c7c_start="$(grep -n 'claim_backing_analysis="\$(python3' "$GUARD_SCRIPT" | head -n 1 | cut -d: -f1 || true)"
if [[ -z "$c7c_start" ]]; then
  fail "Check 7C: analyzer block absent from guard source — the check was removed or renamed"
else
  pass "Check 7C: analyzer block located in guard source (no test/source drift)"

  c7c_dir="$(mktemp -d)"
  c7c_end="$(awk -v s="$c7c_start" 'NR>s && $0=="PY"{print NR; exit}' "$GUARD_SCRIPT")"
  sed -n "$((c7c_start + 1)),$((c7c_end - 1))p" "$GUARD_SCRIPT" >"$c7c_dir/analyzer.py"

  # A: every claim has a run behind it.
  cat >"$c7c_dir/backed.json" <<'JSON'
{"execution":{"completedPhaseClaims":[{"phase":"test","agent":"bubbles.test"}],
 "executionHistory":[{"agent":"bubbles.test","phasesExecuted":["test"]}]}}
JSON

  # B: a claim with NO backing run — the invisible case A-017-08 named.
  cat >"$c7c_dir/unbacked.json" <<'JSON'
{"execution":{"completedPhaseClaims":[{"phase":"test","agent":"bubbles.test"},
 {"phase":"audit","agent":"bubbles.audit"}],
 "executionHistory":[{"agent":"bubbles.test","phasesExecuted":["test"]}]}}
JSON

  # C: more claims than runs — suspicious, not provably false.
  cat >"$c7c_dir/excess.json" <<'JSON'
{"execution":{"completedPhaseClaims":[{"phase":"test"},{"phase":"test"}],
 "executionHistory":[{"agent":"bubbles.test","phasesExecuted":["test"]}]}}
JSON

  # D: history at the TOP level, which is where most agents write it.
  cat >"$c7c_dir/toplevel.json" <<'JSON'
{"execution":{"completedPhaseClaims":[{"phase":"docs","agent":"bubbles.docs"}]},
 "executionHistory":[{"agent":"bubbles.docs","phasesExecuted":["docs"]}]}
JSON

  # E/F: the PLAIN-STRING claim shape. Cases A-D above all use the dict form
  # {"phase":"test"}, but real packets — including this selftest's own
  # emit_base_fixture — write completedPhaseClaims as bare strings. The analyzer
  # used to `continue` past every non-dict element, so `claimed` stayed empty and
  # the gate reported NO_CLAIMS and passed on precisely the shape production
  # emits. The dict-only fixtures could never see it. E is the regression case;
  # F is its adversarial twin.
  cat >"$c7c_dir/str_unbacked.json" <<'JSON'
{"execution":{"completedPhaseClaims":["test","audit"],
 "executionHistory":[{"agent":"bubbles.test","phasesExecuted":["test"]}]}}
JSON

  cat >"$c7c_dir/str_backed.json" <<'JSON'
{"execution":{"completedPhaseClaims":["test"],
 "executionHistory":[{"agent":"bubbles.test","phasesExecuted":["test"]}]}}
JSON

  # G: claims but NO executionHistory at all. Planning-only and legacy packets
  # routinely omit the array. An absent record is not evidence of an unbacked
  # claim, so the check must ABSTAIN here — otherwise widening claim parsing
  # (cases E/F) turns every history-less planning packet into a false block.
  cat >"$c7c_dir/no_history.json" <<'JSON'
{"execution":{"completedPhaseClaims":["analyze","bootstrap"]}}
JSON

  c7c_backed="$(python3 "$c7c_dir/analyzer.py" "$c7c_dir/backed.json" 2>&1 || true)"
  c7c_unbacked="$(python3 "$c7c_dir/analyzer.py" "$c7c_dir/unbacked.json" 2>&1 || true)"
  c7c_excess="$(python3 "$c7c_dir/analyzer.py" "$c7c_dir/excess.json" 2>&1 || true)"
  c7c_toplevel="$(python3 "$c7c_dir/analyzer.py" "$c7c_dir/toplevel.json" 2>&1 || true)"

  if echo "$c7c_unbacked" | grep -q '^UNBACKED=audit$'; then
    pass "Check 7C: a claimed phase with no executionHistory entry is reported UNBACKED"
  else
    fail "Check 7C: an unbacked phase claim went undetected — this is the A-017-08 blind spot (observed: $(echo "$c7c_unbacked" | tr '\n' ' '))"
  fi

  # Adversarial twin: a detector that flagged everything would satisfy the case
  # above while proving nothing. The backed fixture must come back clean.
  if echo "$c7c_backed" | grep -q '^UNBACKED='; then
    fail "Check 7C: a properly backed claim was reported UNBACKED — the detector fires indiscriminately and proves nothing"
  else
    pass "Check 7C adversarial: a properly backed claim is NOT reported (detector discriminates)"
  fi

  if echo "$c7c_excess" | grep -q '^EXCESS=test(2 claim/1 run)$'; then
    pass "Check 7C: more claims than runs is reported as EXCESS, with its counts"
  else
    fail "Check 7C: claim/run count mismatch went undetected (observed: $(echo "$c7c_excess" | tr '\n' ' '))"
  fi

  if echo "$c7c_backed" | grep -q '^EXCESS='; then
    fail "Check 7C adversarial: a 1-claim/1-run packet was reported as EXCESS — the count comparison is wrong"
  else
    pass "Check 7C adversarial: a matched claim/run count is NOT reported as EXCESS"
  fi

  if echo "$c7c_toplevel" | grep -q '^UNBACKED='; then
    fail "Check 7C: a TOP-level executionHistory was not read — same container bug as BUG-012, every such packet would false-block"
  else
    pass "Check 7C: reads a TOP-level executionHistory (BUG-012 container fallback honored)"
  fi

  c7c_str_unbacked="$(python3 "$c7c_dir/analyzer.py" "$c7c_dir/str_unbacked.json" 2>&1 || true)"
  c7c_str_backed="$(python3 "$c7c_dir/analyzer.py" "$c7c_dir/str_backed.json" 2>&1 || true)"

  if echo "$c7c_str_unbacked" | grep -q '^UNBACKED=audit$'; then
    pass "Check 7C: an unbacked PLAIN-STRING claim is reported (the shape real packets write)"
  else
    fail "Check 7C: a plain-string claim shape left the gate INERT — every element skipped, claimed empty, NO_CLAIMS reported while an unbacked phase passed (observed: $(echo "$c7c_str_unbacked" | tr '\n' ' '))"
  fi

  if echo "$c7c_str_unbacked" | grep -q '^NO_CLAIMS=1$'; then
    fail "Check 7C: string-shape claims produced NO_CLAIMS — the analyzer is not normalising them and the gate is looking at nothing"
  else
    pass "Check 7C: string-shape claims are normalised, not discarded as NO_CLAIMS"
  fi

  if echo "$c7c_str_backed" | grep -q '^UNBACKED='; then
    fail "Check 7C adversarial: a backed plain-string claim was reported UNBACKED — string normalisation over-fires"
  else
    pass "Check 7C adversarial: a backed plain-string claim is NOT reported (string path discriminates)"
  fi

  c7c_no_history="$(python3 "$c7c_dir/analyzer.py" "$c7c_dir/no_history.json" 2>&1 || true)"

  if echo "$c7c_no_history" | grep -q '^NO_HISTORY=1$'; then
    pass "Check 7C: abstains when executionHistory is absent entirely (planning-only packets are not false-blocked)"
  else
    fail "Check 7C: a packet with NO executionHistory was adjudicated instead of abstaining — every history-less planning packet would false-block (observed: $(echo "$c7c_no_history" | tr '\n' ' '))"
  fi

  if echo "$c7c_no_history" | grep -q '^UNBACKED='; then
    fail "Check 7C: an absent executionHistory was reported as unbacked claims — absence of a record is not evidence of fabrication"
  else
    pass "Check 7C adversarial: an absent executionHistory yields no UNBACKED finding"
  fi

  rm -rf "$c7c_dir"
fi

# =============================================================================
# Check 43: Human Acceptance Terminal Gate (Gate G136)  [IMP-040 SCOPE-10]
#           BUG-037: acceptance is OPT-OUT. Silence is acceptance.
# =============================================================================
# BUG-029's exact shape is still the load-bearing case. artifact-lint.sh required
# at least ONE `[x]` and never rejected a `[ ]`, so a checklist of one checked
# item and one unchecked passed lint. The RED fixture below is precisely that
# shape: without a checked item too, the case would prove nothing beyond the lint
# rule that already exists.
#
# BUG-037 inverted the CONTRACT, not this closure. The checklist now ships
# CHECKED, a user's only required act is to UNCHECK an item they reject, and an
# authored `## Human Acceptance Record` is no longer demanded at terminal. So
# `all_checked.md` must now PASS where it used to be refused — and `mixed.md`
# must still be refused, by name.
#
# S3-T1..S3-T5 drive the REAL guard over real feature directories, not the
# library alone: a library that returns the right verdict while the guard never
# calls it would satisfy a library-only suite.
c43_dir="$tmp_root/c43-human-acceptance"
mkdir -p "$c43_dir"

# shellcheck source=acceptance-authority-lib.sh
source "$SCRIPT_DIR/acceptance-authority-lib.sh"

cat << 'EOF' > "$c43_dir/mixed.md"
# User Validation

## Checklist

- [x] The list renders on the dashboard route.
- [ ] Deleting an item removes it from the list.

## Notes

- [ ] This bullet is outside the Checklist section and must be ignored.
EOF

cat << 'EOF' > "$c43_dir/all_checked.md"
# User Validation

## Checklist

- [x] The list renders on the dashboard route.
- [x] Deleting an item removes it from the list.

## Notes

- [ ] This bullet is outside the Checklist section and must be ignored.
EOF

cat << 'EOF' > "$c43_dir/bug029.md"
# User Validation

## Checklist

- [x] The list renders on the dashboard route.
- [ ] Deleting an item removes it from the list.
- [ ] An empty list shows the empty state.
- [ ] The list paginates at twenty rows.
- [ ] A filter narrows the rendered rows.
- [ ] The row count matches the rendered rows.
EOF

cat << 'EOF' > "$c43_dir/human_accepted.md"
# User Validation

## Automation Readiness

- [x] Both behaviors verified by automation.

## Checklist

- [x] The list renders on the dashboard route.
- [x] Deleting an item removes it from the list.

## Human Acceptance Record

- acceptedBy: p.kirsanov
- acceptedAt: 2026-08-16T10:00:00Z
- method: human-interactive

## Notes

- [ ] This bullet is outside the Checklist section and must be ignored.
EOF

c43_unchecked() {
  local items
  items="$(bubbles_acceptance_unchecked_items "$1")"
  if [[ -z "$items" ]]; then printf '0\n'; else printf '%s\n' "$items" | grep -c . || true; fi
}

c43_mixed_count="$(c43_unchecked "$c43_dir/mixed.md")"
c43_clean_count="$(c43_unchecked "$c43_dir/all_checked.md")"

if [[ "$c43_mixed_count" -eq 1 ]]; then
  pass "Check 43: one checked plus one unchecked item is detected as unaccepted (BUG-029 shape)"
else
  fail "Check 43: the BUG-029 mixed checklist yielded $c43_mixed_count unchecked item(s), expected 1"
fi

if [[ "$c43_clean_count" -eq 0 ]]; then
  pass "Check 43 adversarial: a fully checked checklist reports no unchecked item, and a '[ ]' outside the Checklist section is ignored"
else
  fail "Check 43: a fully accepted checklist reported $c43_clean_count unchecked item(s) — the section parser is over-reaching beyond '## Checklist'"
fi

if bubbles_acceptance_terminal_verdict "$c43_dir/human_accepted.md" > /dev/null 2>&1; then
  pass "Check 43 (BUG-037): an OPTIONAL authored human record is still accepted at terminal"
else
  fail "Check 43 (BUG-037): a valid human acceptance record was refused: $(bubbles_acceptance_terminal_verdict "$c43_dir/human_accepted.md" 2>&1 || true)"
fi

# --- S3-T1..S3-T5 through the REAL guard -------------------------------------
# The fixture is a copy of the passing delivery fixture, so the ONLY thing that
# varies between cases is uservalidation.md. `run_capture` swallows the guard's
# overall exit; these cases read the Check 43 (Gate G136) SECTION of the log,
# because the surrounding checks are not what is under test here.
#
# BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST MUST be 0 for every case below.
# This file exports it as 1 at the top, and that fast path does not merely speed
# the guard up — it SKIPS sourcing guards/tail-delegated-gates.sh, which is the
# fragment Check 43 lives in. Left at 1, every case here reads an empty section
# and the two negative assertions ("no PD12-NO-RECORD", "no acceptance record")
# pass on absence rather than on behavior. c43_gate_block therefore also refuses
# an empty block outright, so this can never regress into a silent pass.
c43_gate_header="--- Check 43: Human Acceptance Terminal Gate (Gate G136) ---"

c43_gate_block() {
  awk -v h="$1" '
    index($0, h) == 1 {inside=1; next}
    inside && /^--- / {exit}
    inside {print}
  ' "$2"
}

c43_run_guard() {
  local name="$1" uv_source="$2" dir log
  dir="$tmp_root/specs/95$3-c43-$name"
  cp -R "$positive_feature_dir" "$dir"
  cp "$uv_source" "$dir/uservalidation.md"
  log="$tmp_root/c43-$name-guard.log"
  run_capture "$log" \
    env BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=0 \
    bash "$GUARD_SCRIPT" "$dir" > /dev/null
  printf '%s\n' "$log"
}

# S3-T0: the section the next five cases read must exist. Without this, a guard
# that never reached Check 43 would satisfy every negative assertion below.
c43_assert_block_present() {
  local case_id="$1" block="$2" log="$3"
  if [[ -n "$block" ]]; then
    return 0
  fi
  fail "$case_id read an EMPTY Check 43 section — the guard never ran Gate G136 (fast path not disabled?): $log"
  return 1
}

# S3-T1: SCN-B037-009 — fully checked, no record, at a `done` target.
c43_pass_log="$(c43_run_guard "all-checked" "$c43_dir/all_checked.md" 0)"
c43_pass_block="$(c43_gate_block "$c43_gate_header" "$c43_pass_log")"
if c43_assert_block_present "S3-T1" "$c43_pass_block" "$c43_pass_log"; then
  pass "S3-T0 the REAL guard reached Check 43 (Gate G136) — the section under test is present"
fi
if printf '%s' "$c43_pass_block" | grep -q 'PASS:.*Gate G136'; then
  pass "S3-T1 SCN-B037-009: the REAL guard passes a fully checked, record-less packet at a 'done' target"
else
  fail "S3-T1 Check 43 should pass a fully checked record-less packet: $c43_pass_block"
fi
if printf '%s' "$c43_pass_block" | grep -q 'PD12-NO-RECORD'; then
  fail "S3-T1 Check 43 still demands a human acceptance record — BUG-037 did not reach the guard"
else
  pass "S3-T1b the shipped false policy leaves conditional PD12-NO-RECORD dormant"
fi

# S3-T2 ADVERSARIAL: SCN-B037-010 — one unchecked item must still refuse, by name.
c43_block_log="$(c43_run_guard "mixed" "$c43_dir/mixed.md" 1)"
c43_block_block="$(c43_gate_block "$c43_gate_header" "$c43_block_log")"
c43_assert_block_present "S3-T2" "$c43_block_block" "$c43_block_log" || true
if printf '%s' "$c43_block_block" | grep -q 'BLOCK:.*Gate G136' &&
  printf '%s' "$c43_block_block" | grep -q 'PD12-UNCHECKED-ITEM: - \[ \] Deleting an item removes it from the list'; then
  pass "S3-T2 SCN-B037-010 adversarial: the REAL guard refuses an unchecked item and NAMES it"
else
  fail "S3-T2 Check 43 must refuse and name the unchecked item: $c43_block_block"
fi
if printf '%s' "$c43_block_block" | grep -qi 'acceptance record'; then
  fail "S3-T2b Check 43's refusal still points the reader at an acceptance record: $c43_block_block"
else
  pass "S3-T2b the refusal describes the opt-out contract, not an acceptance record"
fi

# S3-T3 ADVERSARIAL: the BUG-029 shape, end to end, with all five named.
c43_bug029_log="$(c43_run_guard "bug029" "$c43_dir/bug029.md" 2)"
c43_bug029_block="$(c43_gate_block "$c43_gate_header" "$c43_bug029_log")"
c43_assert_block_present "S3-T3" "$c43_bug029_block" "$c43_bug029_log" || true
c43_bug029_named="$(printf '%s\n' "$c43_bug029_block" | grep -c 'PD12-UNCHECKED-ITEM' || true)"
if printf '%s' "$c43_bug029_block" | grep -q 'BLOCK:.*Gate G136' && [[ "$c43_bug029_named" -eq 5 ]]; then
  pass "S3-T3 adversarial: the BUG-029 shape is refused end to end through the real guard, all five items named"
else
  fail "S3-T3 BUG-029 pin: expected a block naming 5 items, got $c43_bug029_named: $c43_bug029_block"
fi

# S3-T4 ADVERSARIAL: AC-5. A guard that "helpfully" checked the box would pass
# every other case in this block, so the proof is a byte comparison.
c43_sha_dir="$tmp_root/specs/953-c43-sha"
cp -R "$positive_feature_dir" "$c43_sha_dir"
cp "$c43_dir/mixed.md" "$c43_sha_dir/uservalidation.md"
c43_sha_before="$(sha256_text "$(cat "$c43_sha_dir/uservalidation.md")")"
run_capture "$tmp_root/c43-sha-guard.log" \
  env BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=0 \
  bash "$GUARD_SCRIPT" "$c43_sha_dir" > /dev/null
c43_sha_after="$(sha256_text "$(cat "$c43_sha_dir/uservalidation.md")")"
c43_sha_block="$(c43_gate_block "$c43_gate_header" "$tmp_root/c43-sha-guard.log")"
if ! c43_assert_block_present "S3-T4" "$c43_sha_block" "$tmp_root/c43-sha-guard.log"; then
  :
elif [[ "$c43_sha_before" == "$c43_sha_after" ]]; then
  pass "S3-T4 SCN-B037-011 adversarial: uservalidation.md sha256 is unchanged across a REFUSING guard run (the guard never edits the file)"
else
  fail "S3-T4 the guard modified uservalidation.md: before=$c43_sha_before after=$c43_sha_after"
fi

# S3-T5: SCN-B037-012 — a ceiling-bound target is still exempt. The base fixture
# ships docs-only, so this reuses it WITHOUT the delivery-contract mutation.
c43_ceiling_dir="$tmp_root/specs/954-c43-ceiling"
emit_base_fixture "$c43_ceiling_dir"
cp "$c43_dir/bug029.md" "$c43_ceiling_dir/uservalidation.md"
run_capture "$tmp_root/c43-ceiling-guard.log" \
  env BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=0 \
  bash "$GUARD_SCRIPT" "$c43_ceiling_dir" > /dev/null
c43_ceiling_block="$(c43_gate_block "$c43_gate_header" "$tmp_root/c43-ceiling-guard.log")"
if ! c43_assert_block_present "S3-T5" "$c43_ceiling_block" "$tmp_root/c43-ceiling-guard.log"; then
  :
elif printf '%s' "$c43_ceiling_block" | grep -q "is not 'done'" &&
  ! printf '%s' "$c43_ceiling_block" | grep -q 'PD12-UNCHECKED-ITEM'; then
  pass "S3-T5 SCN-B037-012: a ceiling-bound target status is still exempt and acceptance is not evaluated"
else
  fail "S3-T5 ceiling-bound exemption intact: $c43_ceiling_block"
fi

rm -rf "$c43_dir"

# =============================================================================
# Check 5 — completedScopes element type (BUG-011)
# =============================================================================
# bubbles.validate wrote certification.completedScopes as INTEGER ordinals while
# Check 5 counted QUOTED STRINGS. An integer array matched nothing, so the count
# read 0 — the SAME value a genuinely EMPTY array produces — and both states
# emitted the identical "is EMPTY" message. The collision, not the type
# mismatch, is what made this expensive: the guard's own output pointed away
# from the defect. The control case below is therefore load-bearing, because a
# fix that merely RENAMED the empty-array failure would satisfy the negative
# case while leaving the two states just as indistinguishable.
set_completed_scopes() {
  local state_file="$1"
  local entries_json="$2"

  python3 - "$state_file" "$entries_json" <<'PY'
import json
import sys

path, entries = sys.argv[1], sys.argv[2]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

certification = data.get("certification")
if not isinstance(certification, dict):
    certification = {}
    data["certification"] = certification
certification["completedScopes"] = json.loads(entries)

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY
}

echo "Running Check 5 completedScopes element-type selftest (BUG-011)..."

bug011_ordinal_dir="$tmp_root/specs/940-bug011-completedscopes-ordinals"
bug011_empty_dir="$tmp_root/specs/941-bug011-completedscopes-empty"
bug011_string_dir="$tmp_root/specs/942-bug011-completedscopes-strings"

# The per-scope fixture is the nearest existing packet that reaches Check 5 with
# a real Done scope artifact behind it, so all three cases clone its shape and
# vary ONLY the completedScopes element type.
emit_per_scope_fixture "$bug011_ordinal_dir" "Done" "scope-1-index-parity-proof"
mutate_delivery_contract "$bug011_ordinal_dir/state.json"
set_completed_scopes "$bug011_ordinal_dir/state.json" '[1, 2]'

emit_per_scope_fixture "$bug011_empty_dir" "Done" "scope-1-index-parity-proof"
mutate_delivery_contract "$bug011_empty_dir/state.json"
set_completed_scopes "$bug011_empty_dir/state.json" '[]'

emit_per_scope_fixture "$bug011_string_dir" "Done" "scope-1-index-parity-proof"
mutate_delivery_contract "$bug011_string_dir/state.json"
set_completed_scopes "$bug011_string_dir/state.json" '["scope-1-index-parity-proof"]'

BUG011_ORDINAL_MSG="completedScopes is present but its entries are not string scope IDs"
BUG011_EMPTY_MSG="state.json completedScopes is EMPTY"

bug011_ordinal_log="$tmp_root/bug011-ordinal.log"
bug011_ordinal_status="$(run_capture "$bug011_ordinal_log" bash "$GUARD_SCRIPT" "$bug011_ordinal_dir")"
if [[ "$bug011_ordinal_status" -ne 0 ]]; then
  pass "BUG-011: an ordinal completedScopes array fails the transition guard"
else
  fail "BUG-011: an ordinal completedScopes array should fail the transition guard (observed exit $bug011_ordinal_status)"
  sed -n '1,260p' "$bug011_ordinal_log"
fi
assert_log_contains "$bug011_ordinal_log" "$BUG011_ORDINAL_MSG" \
  "BUG-011: an ordinal completedScopes array is reported as a WRONG-ELEMENT-TYPE failure"
assert_log_not_contains "$bug011_ordinal_log" "$BUG011_EMPTY_MSG" \
  "BUG-011: a populated ordinal array is NOT reported as EMPTY (the two states are distinguishable)"

bug011_empty_log="$tmp_root/bug011-empty.log"
bug011_empty_status="$(run_capture "$bug011_empty_log" bash "$GUARD_SCRIPT" "$bug011_empty_dir")"
if [[ "$bug011_empty_status" -ne 0 ]]; then
  pass "BUG-011 control: a genuinely empty completedScopes array still fails the transition guard"
else
  fail "BUG-011 control: an empty completedScopes array should fail the transition guard (observed exit $bug011_empty_status)"
  sed -n '1,260p' "$bug011_empty_log"
fi
assert_log_contains "$bug011_empty_log" "$BUG011_EMPTY_MSG" \
  "BUG-011 control: a genuinely empty array still reports EMPTY (the empty-array failure was not merely renamed)"
assert_log_not_contains "$bug011_empty_log" "$BUG011_ORDINAL_MSG" \
  "BUG-011 control: an empty array is NOT reported as a wrong-element-type failure"

bug011_string_log="$tmp_root/bug011-string.log"
run_capture "$bug011_string_log" bash "$GUARD_SCRIPT" "$bug011_string_dir" >/dev/null
assert_log_not_contains "$bug011_string_log" "$BUG011_ORDINAL_MSG" \
  "BUG-011 adversarial: a quoted string scope ID is NOT reported as a wrong element type"
assert_log_not_contains "$bug011_string_log" "$BUG011_EMPTY_MSG" \
  "BUG-011 adversarial: a populated string array is NOT reported as EMPTY"
assert_log_contains "$bug011_string_log" "completedScopes count matches artifact Done scope count (1)" \
  "BUG-011 adversarial: one quoted scope ID against one Done scope artifact passes Check 5"

# =============================================================================
# Check 7A — completedPhaseClaims[].claimedAt plausibility (BUG-013)
# =============================================================================
# The uniform-interval analysis ran over executionHistory only. Gates G022/G027
# read execution.completedPhaseClaims — the load-bearing assertion that a phase
# was performed — and NOTHING analysed the claimedAt instants on it, so claims
# sitting on a perfect grid were invisible to every check in the guard.
mutate_completed_phase_claims() {
  local state_file="$1"
  local shape="$2"

  python3 - "$state_file" "$shape" <<'PY'
import json
import sys
from datetime import datetime, timedelta

path, shape = sys.argv[1], sys.argv[2]
with open(path, encoding="utf-8") as handle:
    data = json.load(handle)

execution = data.get("execution")
if not isinstance(execution, dict):
    execution = {}
    data["execution"] = execution

BASE = datetime(2026, 3, 27, 10, 0, 0)
# Cycle the phases the delivery contract already records in executionHistory so
# the fixture varies the claim TIMESTAMPS and nothing else.
PHASES = ["test", "validate", "audit", "docs"]


def claim(index, offset_seconds):
    phase = PHASES[index % len(PHASES)]
    return {
        "phase": phase,
        "agent": f"bubbles.{phase}",
        "claimedAt": (BASE + timedelta(seconds=offset_seconds)).strftime("%Y-%m-%dT%H:%M:%SZ"),
    }


if shape == "absent":
    execution.pop("completedPhaseClaims", None)
    data.pop("completedPhaseClaims", None)
elif shape == "uniform":
    execution["completedPhaseClaims"] = [claim(i, i * 600) for i in range(10)]
elif shape == "backwards":
    execution["completedPhaseClaims"] = [
        claim(i, offset) for i, offset in enumerate([0, 900, 300, 2400])
    ]
elif shape == "irregular":
    execution["completedPhaseClaims"] = [
        claim(i, offset) for i, offset in enumerate([0, 437, 1310, 1622, 3050])
    ]
elif shape == "short-reason-escape":
    # The same 600s grid, with the MIDDLE claim attempting the
    # claimedAtUnreconciled escape on a reason below DECLARED_REASON_MIN.
    # Removing a middle point from an even grid leaves a doubled gap, so an
    # escape that were wrongly honored would DISSOLVE the uniform spacing and
    # this fixture would stop failing. That is what makes the case
    # discriminating rather than a restatement of the uniform case.
    claims = [claim(i, i * 600) for i in range(10)]
    claims[4]["claimedAtUnreconciled"] = True
    claims[4]["claimedAtUnreconciledReason"] = "lost"
    execution["completedPhaseClaims"] = claims
else:
    raise SystemExit(f"unknown completedPhaseClaims shape: {shape}")

with open(path, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
PY
}

echo "Running Check 7A completedPhaseClaims timestamp selftest (BUG-013)..."

bug013_uniform_dir="$tmp_root/specs/943-bug013-claims-uniform-grid"
bug013_backwards_dir="$tmp_root/specs/944-bug013-claims-backwards"
bug013_irregular_dir="$tmp_root/specs/945-bug013-claims-irregular"
bug013_absent_dir="$tmp_root/specs/946-bug013-claims-absent"
bug013_escape_dir="$tmp_root/specs/947-bug013-claims-short-reason-escape"

for bug013_shape_pair in \
  "$bug013_uniform_dir:uniform" \
  "$bug013_backwards_dir:backwards" \
  "$bug013_irregular_dir:irregular" \
  "$bug013_absent_dir:absent" \
  "$bug013_escape_dir:short-reason-escape"; do
  bug013_dir="${bug013_shape_pair%%:*}"
  bug013_shape="${bug013_shape_pair##*:}"
  cp -R "$positive_feature_dir" "$bug013_dir"
  mutate_completed_phase_claims "$bug013_dir/state.json" "$bug013_shape"
done

BUG013_UNIFORM_MSG="claimedAt values with identical 600s intervals"
BUG013_BACKWARDS_MSG="completedPhaseClaims claimedAt runs backwards:"
BUG013_PLAUSIBLE_MSG="completedPhaseClaims claimedAt timestamps look plausible"
BUG013_ABSTAIN_MSG="No completedPhaseClaims recorded — claim-timestamp plausibility abstains"

bug013_uniform_log="$tmp_root/bug013-uniform.log"
bug013_uniform_status="$(run_capture "$bug013_uniform_log" bash "$GUARD_SCRIPT" "$bug013_uniform_dir")"
if [[ "$bug013_uniform_status" -ne 0 ]]; then
  pass "BUG-013: completedPhaseClaims on an exact 600s grid fails the transition guard"
else
  fail "BUG-013: completedPhaseClaims on an exact 600s grid should fail the transition guard (observed exit $bug013_uniform_status)"
  sed -n '1,260p' "$bug013_uniform_log"
fi
assert_log_contains "$bug013_uniform_log" "completedPhaseClaims has 10 $BUG013_UNIFORM_MSG — FABRICATION INDICATOR" \
  "BUG-013: uniformly spaced claimedAt values are named a FABRICATION INDICATOR"

bug013_backwards_log="$tmp_root/bug013-backwards.log"
bug013_backwards_status="$(run_capture "$bug013_backwards_log" bash "$GUARD_SCRIPT" "$bug013_backwards_dir")"
if [[ "$bug013_backwards_status" -ne 0 ]]; then
  pass "BUG-013: completedPhaseClaims whose claimedAt runs backwards fails the transition guard"
else
  fail "BUG-013: backwards claimedAt ordering should fail the transition guard (observed exit $bug013_backwards_status)"
  sed -n '1,260p' "$bug013_backwards_log"
fi
assert_log_contains "$bug013_backwards_log" "$BUG013_BACKWARDS_MSG" \
  "BUG-013: a claim recorded before the claim ahead of it is reported as backwards ordering"

# Adversarial twin: a detector that fired on every claim set would satisfy both
# negative cases above and prove nothing.
bug013_irregular_log="$tmp_root/bug013-irregular.log"
run_capture "$bug013_irregular_log" bash "$GUARD_SCRIPT" "$bug013_irregular_dir" >/dev/null
assert_log_contains "$bug013_irregular_log" "$BUG013_PLAUSIBLE_MSG" \
  "BUG-013 adversarial: irregular forward-moving claimedAt values are reported plausible"
assert_log_not_contains "$bug013_irregular_log" "$BUG013_UNIFORM_MSG" \
  "BUG-013 adversarial: irregular spacing is NOT reported as a uniform interval"
assert_log_not_contains "$bug013_irregular_log" "$BUG013_BACKWARDS_MSG" \
  "BUG-013 adversarial: forward-moving claims are NOT reported as backwards"

# Absence of a record is not evidence of fabrication — the same abstention
# Check 7C makes for an absent executionHistory.
bug013_absent_log="$tmp_root/bug013-absent.log"
run_capture "$bug013_absent_log" bash "$GUARD_SCRIPT" "$bug013_absent_dir" >/dev/null
assert_log_contains "$bug013_absent_log" "$BUG013_ABSTAIN_MSG" \
  "BUG-013: an absent completedPhaseClaims abstains instead of adjudicating"
assert_log_not_contains "$bug013_absent_log" "$BUG013_UNIFORM_MSG" \
  "BUG-013: an absent completedPhaseClaims yields no uniform-interval finding"
assert_log_not_contains "$bug013_absent_log" "$BUG013_BACKWARDS_MSG" \
  "BUG-013: an absent completedPhaseClaims yields no backwards-ordering finding"

# The declaration escape has to cost a substantive reason, or it is a bypass
# with extra steps.
bug013_escape_log="$tmp_root/bug013-escape.log"
bug013_escape_status="$(run_capture "$bug013_escape_log" bash "$GUARD_SCRIPT" "$bug013_escape_dir")"
if [[ "$bug013_escape_status" -ne 0 ]]; then
  pass "BUG-013: claimedAtUnreconciled with a sub-threshold reason still fails the transition guard"
else
  fail "BUG-013: a sub-threshold claimedAtUnreconciled reason should NOT buy an exclusion (observed exit $bug013_escape_status)"
  sed -n '1,260p' "$bug013_escape_log"
fi
assert_log_contains "$bug013_escape_log" "completedPhaseClaims has 10 $BUG013_UNIFORM_MSG — FABRICATION INDICATOR" \
  "BUG-013: a claim declared unreconciled on a short reason stays IN the analysed set (count is still 10)"
assert_log_not_contains "$bug013_escape_log" "completedPhaseClaims declares unreconciled claim timestamps" \
  "BUG-013: a sub-threshold reason does not register as a declared unreconciled claim"

# BEGIN BUG032 SECURITY REGRESSION TESTS
run_bug032_security_regressions() {
  local base_fixture="$1"
  local security_root="$tmp_root/bug032-security"
  local security_shadow_dir="$security_root/shadow-bin"
  local security_real_awk=""
  local security_real_cat=""
  local security_classifier_file="$security_root/check8b-classifier.sh"
  local security_index=0

  mkdir -p "$security_root" "$security_shadow_dir"
  security_real_awk="$(command -v awk)"
  security_real_cat="$(command -v cat)"
  run_bug032_iteration10_security_assertions

  cat <<'EOF' > "$security_shadow_dir/awk"
#!/usr/bin/env bash
set -u

program="${1:-}"
if [[ "$program" == *'function emit('* && "$program" == *'fence'* ]]; then
  if [[ -n "${BUG032_SECURITY_AWK_COUNTER:-}" ]]; then
    printf '%s\n' call >> "$BUG032_SECURITY_AWK_COUNTER"
  fi
  case "${BUG032_SECURITY_AWK_MODE:-delegate}" in
    producer-before)
      exit 73
      ;;
    producer-partial)
      printf '%s\t%s\t%s\t%s\t%s\n' \
        A 1 active 23 'Remove the public route'
      printf '%s\t%s\t%s\t%s\t%s\n' \
        A 2 active 33 'The p95 latency budget is 200 ms.'
      exit 74
      ;;
  esac
fi
exec "${BUG032_SECURITY_REAL_AWK:?}" "$@"
EOF
  chmod +x "$security_shadow_dir/awk"

  cat <<'EOF' > "$security_shadow_dir/cat"
#!/usr/bin/env bash
set -u

for argument in "$@"; do
  if [[ -n "${BUG032_SECURITY_SOURCE_PATH:-}" ]] \
    && [[ "$argument" == "$BUG032_SECURITY_SOURCE_PATH" ]]; then
    if [[ -n "${BUG032_SECURITY_CAT_COUNTER:-}" ]]; then
      printf '%s\n' call >> "$BUG032_SECURITY_CAT_COUNTER"
    fi
    exit 75
  fi
done
exec "${BUG032_SECURITY_REAL_CAT:?}" "$@"
EOF
  chmod +x "$security_shadow_dir/cat"

  bug032_security_report() {
    local mismatch_count="$1"
    local title="$2"

    if [[ "$mismatch_count" -eq 0 ]]; then
      pass "$title"
    else
      fail "$title (mismatches=$mismatch_count)"
    fi
  }

  bug032_security_counter_value() {
    local counter_file="$1"
    local counter_value=0

    if [[ -f "$counter_file" ]]; then
      counter_value="$(wc -l < "$counter_file")"
      counter_value="${counter_value//[[:space:]]/}"
    fi
    printf '%s\n' "$counter_value"
  }

  bug032_security_projection_error_matches() {
    local log_file="$1"
    local expected_reason="$2"
    local expected_producer_status="$3"
    local expected_input_status="$4"

    grep -Fq -- 'check: Context projection' "$log_file" \
      && grep -Fq -- 'scope: scopes.md' "$log_file" \
      && grep -Fq -- 'consumers: Check 8B,Check 5A' "$log_file" \
      && grep -Fq -- 'projection-status: error' "$log_file" \
      && grep -Fq -- "producer-status: $expected_producer_status" "$log_file" \
      && grep -Fq -- "input-read-status: $expected_input_status" "$log_file" \
      && grep -Eq -- '^source-location: (scope-start|scopes\.md:[0-9]+)$' "$log_file" \
      && grep -Fq -- 'active-count: discarded' "$log_file" \
      && grep -Fq -- 'fixture-count: discarded' "$log_file" \
      && grep -Fq -- 'structural-status: discarded' "$log_file" \
      && grep -Fq -- "reason: $expected_reason" "$log_file" \
      && grep -Eq -- '^boundary: [^[:space:]].*$' "$log_file" \
      && grep -Fq -- 'check-8b-disposition: error' "$log_file" \
      && grep -Fq -- 'check-8b-impact-checks: skipped' "$log_file" \
      && grep -Fq -- 'check-5a-disposition: error' "$log_file" \
      && grep -Fq -- 'check-5a-stress-checks: skipped' "$log_file" \
      && grep -Fq -- 'result: blocked' "$log_file"
  }

  bug032_security_run_scope_case() {
    local case_slug="$1"
    local payload="$2"
    local case_dir="$security_root/$case_slug"

    cp -R "$base_fixture" "$case_dir"
    if [[ -n "$payload" ]]; then
      printf '\n%s\n' "$payload" >> "$case_dir/scopes.md"
    fi
    BUG032_SECURITY_CASE_LOG="$security_root/$case_slug.log"
    BUG032_SECURITY_CASE_STATUS="$(run_capture \
      "$BUG032_SECURITY_CASE_LOG" bash "$GUARD_SCRIPT" "$case_dir")"
  }

  bug032_security_run_shadow_case() {
    local case_slug="$1"
    local awk_mode="$2"
    local fail_source_read="$3"
    local case_dir="$security_root/$case_slug"
    local awk_counter="$security_root/$case_slug-awk.count"
    local cat_counter="$security_root/$case_slug-cat.count"

    cp -R "$base_fixture" "$case_dir"
    : > "$awk_counter"
    : > "$cat_counter"
    BUG032_SECURITY_CASE_LOG="$security_root/$case_slug.log"
    BUG032_SECURITY_CASE_STATUS="$(run_capture \
      "$BUG032_SECURITY_CASE_LOG" env \
      PATH="$security_shadow_dir:$PATH" \
      BUG032_SECURITY_REAL_AWK="$security_real_awk" \
      BUG032_SECURITY_REAL_CAT="$security_real_cat" \
      BUG032_SECURITY_AWK_MODE="$awk_mode" \
      BUG032_SECURITY_AWK_COUNTER="$awk_counter" \
      BUG032_SECURITY_CAT_COUNTER="$cat_counter" \
      BUG032_SECURITY_SOURCE_PATH="$([[ "$fail_source_read" == "yes" ]] \
        && printf '%s' "$case_dir/scopes.md")" \
      bash "$GUARD_SCRIPT" "$case_dir")"
    BUG032_SECURITY_AWK_CALLS="$(bug032_security_counter_value "$awk_counter")"
    BUG032_SECURITY_CAT_CALLS="$(bug032_security_counter_value "$cat_counter")"
  }

  local sec001_failure_mismatches=0
  bug032_security_run_shadow_case sec001-producer-before producer-before no
  if [[ "$BUG032_SECURITY_CASE_STATUS" -eq 0 ]] \
    || [[ "$BUG032_SECURITY_AWK_CALLS" -ne 1 ]] \
    || [[ "$BUG032_SECURITY_CAT_CALLS" -ne 0 ]] \
    || ! bug032_security_projection_error_matches \
      "$BUG032_SECURITY_CASE_LOG" context-projection-error error complete \
    || ! grep -Fq -- 'boundary: producer' "$BUG032_SECURITY_CASE_LOG"; then
    sec001_failure_mismatches=$((sec001_failure_mismatches + 1))
    printf 'BUG032_SEC001_PRODUCER_BEFORE_MISMATCH status=%s awkCalls=%s catCalls=%s\n' \
      "$BUG032_SECURITY_CASE_STATUS" "$BUG032_SECURITY_AWK_CALLS" \
      "$BUG032_SECURITY_CAT_CALLS"
  fi

  bug032_security_run_shadow_case sec001-input-read delegate yes
  if [[ "$BUG032_SECURITY_CASE_STATUS" -eq 0 ]] \
    || [[ "$BUG032_SECURITY_AWK_CALLS" -ne 0 ]] \
    || [[ "$BUG032_SECURITY_CAT_CALLS" -ne 1 ]] \
    || ! bug032_security_projection_error_matches \
      "$BUG032_SECURITY_CASE_LOG" context-read-error not-reached error \
    || ! grep -Fq -- 'boundary: input-read' "$BUG032_SECURITY_CASE_LOG"; then
    sec001_failure_mismatches=$((sec001_failure_mismatches + 1))
    printf 'BUG032_SEC001_INPUT_READ_MISMATCH status=%s awkCalls=%s catCalls=%s\n' \
      "$BUG032_SECURITY_CASE_STATUS" "$BUG032_SECURITY_AWK_CALLS" \
      "$BUG032_SECURITY_CAT_CALLS"
  fi
  sec001_failure_mismatches=$((
    sec001_failure_mismatches + BUG032_ITER10_SEC001_FAILURES
  ))
  bug032_security_report "$sec001_failure_mismatches" \
    "BUG-032 SEC-001 blocks Check 8B and Check 5A on producer and input-read failure"

  local sec001_disposal_mismatches=0
  bug032_security_run_scope_case sec001-complete-empty ''
  if [[ "$BUG032_SECURITY_CASE_STATUS" -ne 0 ]] \
    || grep -Fq -- 'check: Check 8B' "$BUG032_SECURITY_CASE_LOG" \
    || grep -Fq -- 'SLA-sensitive scope is missing' "$BUG032_SECURITY_CASE_LOG" \
    || grep -Fq -- 'context projection failed' "$BUG032_SECURITY_CASE_LOG"; then
    sec001_disposal_mismatches=$((sec001_disposal_mismatches + 1))
    printf 'BUG032_SEC001_COMPLETE_EMPTY_MISMATCH status=%s\n' \
      "$BUG032_SECURITY_CASE_STATUS"
  fi

  bug032_security_run_scope_case sec001-complete-active \
    $'### Active declarations\n\nRemove the public route.\n\nThe p95 latency budget is 200 ms.'
  if [[ "$BUG032_SECURITY_CASE_STATUS" -eq 0 ]] \
    || ! grep -Fq -- 'classification: direct-positive' "$BUG032_SECURITY_CASE_LOG" \
    || ! grep -Fq -- 'reason: direct' "$BUG032_SECURITY_CASE_LOG" \
    || ! grep -Fq -- 'SLA-sensitive scope is missing canonical Stress Test Plan row' "$BUG032_SECURITY_CASE_LOG" \
    || grep -Fq -- 'context projection failed' "$BUG032_SECURITY_CASE_LOG"; then
    sec001_disposal_mismatches=$((sec001_disposal_mismatches + 1))
    printf 'BUG032_SEC001_COMPLETE_ACTIVE_MISMATCH status=%s\n' \
      "$BUG032_SECURITY_CASE_STATUS"
  fi

  bug032_security_run_shadow_case sec001-producer-partial producer-partial no
  if [[ "$BUG032_SECURITY_CASE_STATUS" -eq 0 ]] \
    || [[ "$BUG032_SECURITY_AWK_CALLS" -ne 1 ]] \
    || ! bug032_security_projection_error_matches \
      "$BUG032_SECURITY_CASE_LOG" context-projection-error error complete \
    || ! grep -Fq -- 'boundary: producer' "$BUG032_SECURITY_CASE_LOG" \
    || grep -Fq -- 'classification: direct-positive' "$BUG032_SECURITY_CASE_LOG" \
    || grep -Fq -- 'SLA-sensitive scope is missing canonical Stress Test Plan row' "$BUG032_SECURITY_CASE_LOG"; then
    sec001_disposal_mismatches=$((sec001_disposal_mismatches + 1))
    printf 'BUG032_SEC001_PARTIAL_DISPOSAL_MISMATCH status=%s awkCalls=%s\n' \
      "$BUG032_SECURITY_CASE_STATUS" "$BUG032_SECURITY_AWK_CALLS"
  fi
  bug032_security_report "$sec001_disposal_mismatches" \
    "BUG-032 SEC-001 discards partial projection records and preserves complete controls"

  local -a sec002_error_slugs=(
    fence-length fence-type fence-unclosed
    examples-header examples-separator examples-data examples-row
    test-plan-header test-plan-separator test-plan-data test-plan-row first-error
  )
  local -a sec002_error_reasons=(
    fence-identity-error fence-identity-error unclosed-fence-error
    examples-table-error examples-table-error examples-table-error examples-table-error
    test-plan-table-error test-plan-table-error test-plan-table-error test-plan-table-error
    examples-table-error
  )
  local -a sec002_error_payloads=(
    $'### Fixture fence\n\n````gherkin\nRemove the public route.\n```'
    $'### Fixture fence\n\n```gherkin\nRemove the public route.\n~~~'
    $'### Fixture fence\n\n```gherkin\nRemove the public route.'
    $'### Fixture examples\n\nExamples:\nnot a table row'
    $'### Fixture examples\n\nExamples:\n| declaration |\n| Remove the public route. |'
    $'### Fixture examples\n\nExamples:\n| declaration | result |\n| --- | --- |'
    $'### Fixture examples\n\nExamples:\n| declaration | result |\n| --- | --- |\n| Remove the public route. |'
    $'### Test Plan\n\nnot a table row'
    $'### Test Plan\n\n| Test Type | Description | Expected Result |\n| Functional | Remove the public route. | blocked |'
    $'### Test Plan\n\n| Test Type | Description | Expected Result |\n| --- | --- | --- |'
    $'### Test Plan\n\n| Test Type | Description | Expected Result |\n| --- | --- | --- |\n| Functional | Remove the public route. |'
    $'### Fixture examples\n\nExamples:\n| declaration |\n| Remove the public route. |\n\n### Test Plan\n\nnot a table row'
  )
  local sec002_error_mismatches=0
  for ((security_index = 0; security_index < ${#sec002_error_slugs[@]}; security_index++)); do
    bug032_security_run_scope_case \
      "sec002-${sec002_error_slugs[$security_index]}" \
      "${sec002_error_payloads[$security_index]}"
    if [[ "$BUG032_SECURITY_CASE_STATUS" -eq 0 ]] \
      || ! bug032_security_projection_error_matches \
        "$BUG032_SECURITY_CASE_LOG" \
        "${sec002_error_reasons[$security_index]}" error complete \
      || ! grep -Fq -- 'source-location: scopes.md:' "$BUG032_SECURITY_CASE_LOG"; then
      sec002_error_mismatches=$((sec002_error_mismatches + 1))
      printf 'BUG032_SEC002_STRUCTURE_MISMATCH case=%s expectedReason=%s status=%s\n' \
        "${sec002_error_slugs[$security_index]}" \
        "${sec002_error_reasons[$security_index]}" \
        "$BUG032_SECURITY_CASE_STATUS"
    fi
    if [[ "${sec002_error_slugs[$security_index]}" == "fence-length" ]] \
      && { ! grep -Fq -- 'opener-marker: backtick:4' "$BUG032_SECURITY_CASE_LOG" \
        || ! grep -Fq -- 'closer-marker: backtick:3' "$BUG032_SECURITY_CASE_LOG"; }; then
      sec002_error_mismatches=$((sec002_error_mismatches + 1))
    fi
    if [[ "${sec002_error_slugs[$security_index]}" == "fence-type" ]] \
      && { ! grep -Fq -- 'opener-marker: backtick:3' "$BUG032_SECURITY_CASE_LOG" \
        || ! grep -Fq -- 'closer-marker: tilde:3' "$BUG032_SECURITY_CASE_LOG"; }; then
      sec002_error_mismatches=$((sec002_error_mismatches + 1))
    fi
    if [[ "${sec002_error_slugs[$security_index]}" == "fence-unclosed" ]] \
      && ! grep -Fq -- 'required-closer: backtick:3' "$BUG032_SECURITY_CASE_LOG"; then
      sec002_error_mismatches=$((sec002_error_mismatches + 1))
    fi
    if [[ "${sec002_error_slugs[$security_index]}" == "first-error" ]] \
      && grep -Fq -- 'reason: test-plan-table-error' "$BUG032_SECURITY_CASE_LOG"; then
      sec002_error_mismatches=$((sec002_error_mismatches + 1))
    fi
  done
  bug032_security_report "$sec002_error_mismatches" \
    "BUG-032 SEC-002 rejects fence identity and fixture table structure defects in first-error order"

  local sec002_control_mismatches=0
  bug032_security_run_scope_case sec002-valid-fixtures \
    $'### Fixture fence\n\n````gherkin\nRemove the public route.\nThe p95 latency budget is 200 ms.\n````\n\n### Fixture examples\n\nExamples:\n| declaration | result |\n| --- | --- |\n| Remove the public route. | blocked |\n| The p95 latency budget is 200 ms. | blocked |\n\n### Test Plan\n\n| Test Type | Description | Expected Result |\n| --- | --- | --- |\n| Functional | Remove the public route. | fixture only |\n| Functional | The p95 latency budget is 200 ms. | fixture only |'
  if [[ "$BUG032_SECURITY_CASE_STATUS" -ne 0 ]] \
    || grep -Fq -- 'check: Check 8B' "$BUG032_SECURITY_CASE_LOG" \
    || grep -Fq -- 'SLA-sensitive scope is missing' "$BUG032_SECURITY_CASE_LOG" \
    || grep -Fq -- 'context projection failed' "$BUG032_SECURITY_CASE_LOG"; then
    sec002_control_mismatches=$((sec002_control_mismatches + 1))
    printf 'BUG032_SEC002_VALID_FIXTURE_MISMATCH status=%s\n' \
      "$BUG032_SECURITY_CASE_STATUS"
  fi

  bug032_security_run_scope_case sec002-active-twins \
    $'### Fixture fence\n\n```gherkin\nRemove the public route.\nThe p95 latency budget is 200 ms.\n```\n\n### Active twins\n\nRemove the public route.\nThe p95 latency budget is 200 ms.'
  if [[ "$BUG032_SECURITY_CASE_STATUS" -eq 0 ]] \
    || ! grep -Fq -- 'classification: direct-positive' "$BUG032_SECURITY_CASE_LOG" \
    || ! grep -Fq -- 'SLA-sensitive scope is missing canonical Stress Test Plan row' "$BUG032_SECURITY_CASE_LOG"; then
    sec002_control_mismatches=$((sec002_control_mismatches + 1))
    printf 'BUG032_SEC002_ACTIVE_TWIN_MISMATCH status=%s\n' \
      "$BUG032_SECURITY_CASE_STATUS"
  fi

  bug032_security_run_scope_case sec002-ordinary-text-fence \
    $'### Ordinary text fence\n\n````text\nRemove the public route.\n````'
  if [[ "$BUG032_SECURITY_CASE_STATUS" -eq 0 ]] \
    || ! grep -Fq -- 'classification: direct-positive' "$BUG032_SECURITY_CASE_LOG" \
    || grep -Fq -- 'context projection failed' "$BUG032_SECURITY_CASE_LOG"; then
    sec002_control_mismatches=$((sec002_control_mismatches + 1))
    printf 'BUG032_SEC002_ORDINARY_TEXT_FENCE_MISMATCH status=%s\n' \
      "$BUG032_SECURITY_CASE_STATUS"
  fi
  bug032_security_report "$sec002_control_mismatches" \
    "BUG-032 SEC-002 preserves valid fixture suppression and identical active twins"

  bug032_security_run_g040_case() {
    local case_slug="$1"
    local prose="$2"
    local case_dir="$security_root/g040-$case_slug"

    emit_g040_fixture "$case_dir" "done" "$prose" no no
    BUG032_SECURITY_CASE_LOG="$security_root/g040-$case_slug.log"
    BUG032_SECURITY_CASE_STATUS="$(run_capture \
      "$BUG032_SECURITY_CASE_LOG" bash "$GUARD_SCRIPT" "$case_dir")"
  }

  local -a sec003_block_slugs=(
    ticket-case-space ticket-tab ticket-hyphen ticket-mixed
    scope-case-space scope-tab-mixed scope-hyphen
  )
  local -a sec003_block_prose=(
    'Move this work to a Separate  Ticket.'
    $'Move this work to a separate\tticket.'
    'Move this work to a separate-ticket.'
    $'Move this work to a separate -\t-ticket.'
    'Implement this in a FUTURE  SCOPE.'
    $'Implement this in a future \t- scope.'
    'Implement this in a future-scope.'
  )
  local -a sec003_block_canonical=(
    'separate ticket' 'separate ticket' 'separate ticket' 'separate ticket'
    'future scope' 'future scope' 'future scope'
  )
  local -a sec003_block_matched=(
    'Separate  Ticket' 'separate\tticket' 'separate-ticket' 'separate -\t-ticket'
    'FUTURE  SCOPE' 'future \t- scope' 'future-scope'
  )
  local sec003_match_mismatches=0
  for ((security_index = 0; security_index < ${#sec003_block_slugs[@]}; security_index++)); do
    bug032_security_run_g040_case \
      "${sec003_block_slugs[$security_index]}" \
      "${sec003_block_prose[$security_index]}"
    if [[ "$BUG032_SECURITY_CASE_STATUS" -eq 0 ]] \
      || ! grep -Fq -- 'deferral language hit' "$BUG032_SECURITY_CASE_LOG" \
      || ! grep -Fqi -- "${sec003_block_canonical[$security_index]}" "$BUG032_SECURITY_CASE_LOG" \
      || ! grep -Fq -- "${sec003_block_matched[$security_index]}" "$BUG032_SECURITY_CASE_LOG"; then
      sec003_match_mismatches=$((sec003_match_mismatches + 1))
      printf 'BUG032_SEC003_MATCH_MISMATCH case=%s status=%s canonical=%s matched=%s\n' \
        "${sec003_block_slugs[$security_index]}" \
        "$BUG032_SECURITY_CASE_STATUS" \
        "${sec003_block_canonical[$security_index]}" \
        "${sec003_block_matched[$security_index]}"
    fi
  done
  sec003_match_mismatches=$((
    sec003_match_mismatches + BUG032_ITER10_SEC003_FAILURES
  ))
  bug032_security_report "$sec003_match_mismatches" \
    "BUG-032 SEC-003 matches exact G040 phrases across case and horizontal separators"

  local sec003_boundary_mismatches=0
  local -a sec003_punctuation_prose=(
    'Move this work to (separate ticket), now.'
    'Implement this in [future scope!].'
  )
  for ((security_index = 0; security_index < ${#sec003_punctuation_prose[@]}; security_index++)); do
    bug032_security_run_g040_case \
      "punctuation-$security_index" \
      "${sec003_punctuation_prose[$security_index]}"
    if [[ "$BUG032_SECURITY_CASE_STATUS" -eq 0 ]] \
      || ! grep -Fq -- 'deferral language hit' "$BUG032_SECURITY_CASE_LOG"; then
      sec003_boundary_mismatches=$((sec003_boundary_mismatches + 1))
    fi
  done
  local -a sec003_benign_prose=(
    'A separate preservation clause stays negative.'
    'These are future workflow obligations.'
    'The text says separately ticketed.'
    'The text says future scoped analysis.'
    'The identifier aseparate ticket remains inert.'
    'The plural separate tickets remains inert.'
    $'The words remain on distinct lines: separate\nticket.'
    $'The words remain on distinct lines: future\nscope.'
  )
  for ((security_index = 0; security_index < ${#sec003_benign_prose[@]}; security_index++)); do
    bug032_security_run_g040_case \
      "benign-$security_index" \
      "${sec003_benign_prose[$security_index]}"
    if grep -Fq -- 'deferral language hit' "$BUG032_SECURITY_CASE_LOG" \
      || ! grep -Fq -- 'Zero deferral language found in scope and report artifacts (Gate G040)' "$BUG032_SECURITY_CASE_LOG"; then
      sec003_boundary_mismatches=$((sec003_boundary_mismatches + 1))
      printf 'BUG032_SEC003_BENIGN_MISMATCH case=%s status=%s\n' \
        "$security_index" "$BUG032_SECURITY_CASE_STATUS"
    fi
  done
  bug032_security_report "$sec003_boundary_mismatches" \
    "BUG-032 SEC-003 preserves punctuation boundaries and benign near-miss twins"

  awk '
    $0 == "# BEGIN CHECK8B FINITE CLASSIFIER" { capture = 1 }
    capture { print }
    $0 == "# END CHECK8B FINITE CLASSIFIER" { exit }
  ' "$PLANNING_CHECKS_SCRIPT" > "$security_classifier_file"
  local security_classifier_ready=0
  # shellcheck disable=SC1090  # marker-derived copy of the production classifier block
  if bash -n "$security_classifier_file" && source "$security_classifier_file"; then
    security_classifier_ready=1
  fi

  bug032_security_classify() {
    local declaration="$1"

    CHECK8B_CLASSIFICATION="__unset__"
    CHECK8B_VERB="__unset__"
    CHECK8B_MUTATION_TARGET="__unset__"
    CHECK8B_DIRECT_SURFACES="__unset__"
    CHECK8B_PRESERVED_SURFACES="__unset__"
    CHECK8B_REASON="__unset__"
    CHECK8B_UNRESOLVED_PHRASE="__unset__"
    CHECK8B_BOUNDARY="__unset__"
    CHECK8B_TOKEN_COUNT=-1
    CHECK8B_CANDIDATE_COUNT=-1
    unset CHECK8B_LINE_BYTE_COUNT CHECK8B_TOKEN_BYTE_COUNT || true
    BUG032_SECURITY_CLASSIFY_STATUS=2
    if [[ "$security_classifier_ready" -eq 1 ]]; then
      if check8b_classify_line "$declaration"; then
        BUG032_SECURITY_CLASSIFY_STATUS=0
      else
        BUG032_SECURITY_CLASSIFY_STATUS=$?
      fi
    fi
  }

  bug032_security_guard_record_has() {
    local log_file="$1"
    local expected_classification="$2"
    local expected_reason="$3"
    local expected_boundary="$4"
    local expected_impact="$5"
    local expected_result="$6"
    local expected_correction="$7"

    grep -Fq -- 'check: Check 8B' "$log_file" \
      && grep -Fq -- "classification: $expected_classification" "$log_file" \
      && grep -Fq -- "reason: $expected_reason" "$log_file" \
      && grep -Fq -- "boundary: $expected_boundary" "$log_file" \
      && grep -Fq -- "impact-checks: $expected_impact" "$log_file" \
      && grep -Fq -- "result: $expected_result" "$log_file" \
      && grep -Fq -- "correction: $expected_correction" "$log_file"
  }

  local security_line_4096='Remove the public route'
  while [[ "${#security_line_4096}" -lt 4096 ]]; do
    security_line_4096="${security_line_4096}."
  done
  local security_line_4097="${security_line_4096}."
  local sec004_line_mismatches=0
  bug032_security_classify "$security_line_4096"
  if [[ "$BUG032_SECURITY_CLASSIFY_STATUS" -ne 0 ]] \
    || [[ "$CHECK8B_CLASSIFICATION" != "direct-positive" ]] \
    || [[ "$CHECK8B_REASON" != "direct" ]] \
    || [[ "$CHECK8B_BOUNDARY" != "line-bytes=4096/4096" ]] \
    || [[ "${CHECK8B_LINE_BYTE_COUNT:-missing}" != "4096" ]] \
    || [[ "$CHECK8B_TOKEN_COUNT" -ne 4 ]] \
    || [[ "$CHECK8B_CANDIDATE_COUNT" -ne 1 ]]; then
    sec004_line_mismatches=$((sec004_line_mismatches + 1))
    printf 'BUG032_SEC004_LINE_4096_MISMATCH status=%s classification=%s reason=%s boundary=%s lineBytes=%s tokens=%s candidates=%s\n' \
      "$BUG032_SECURITY_CLASSIFY_STATUS" "$CHECK8B_CLASSIFICATION" \
      "$CHECK8B_REASON" "$CHECK8B_BOUNDARY" \
      "${CHECK8B_LINE_BYTE_COUNT:-missing}" "$CHECK8B_TOKEN_COUNT" \
      "$CHECK8B_CANDIDATE_COUNT"
  fi
  bug032_security_run_scope_case sec004-line-4096 "$security_line_4096"
  if [[ "$BUG032_SECURITY_CASE_STATUS" -eq 0 ]] \
    || ! bug032_security_guard_record_has \
      "$BUG032_SECURITY_CASE_LOG" direct-positive direct \
      line-bytes=4096/4096 run blocked \
      'Add only the missing Consumer Impact Sweep section, completion item, and affected-consumer inventory for direct surfaces: remove:route.'; then
    sec004_line_mismatches=$((sec004_line_mismatches + 1))
  fi

  bug032_security_classify "$security_line_4097"
  if [[ "$BUG032_SECURITY_CLASSIFY_STATUS" -ne 0 ]] \
    || [[ "$CHECK8B_CLASSIFICATION" != "ambiguous" ]] \
    || [[ "$CHECK8B_REASON" != "line-byte-limit" ]] \
    || [[ "$CHECK8B_BOUNDARY" != "line-bytes=4097/4096:first-overflow" ]] \
    || [[ "${CHECK8B_LINE_BYTE_COUNT:-missing}" != "4097" ]] \
    || [[ "$CHECK8B_TOKEN_COUNT" -ne 0 ]] \
    || [[ "$CHECK8B_CANDIDATE_COUNT" -ne 0 ]] \
    || [[ "${#_CHECK8B_TOKENS[@]}" -ne 0 ]] \
    || [[ "${#_CHECK8B_CANDIDATE_INDEXES[@]}" -ne 0 ]]; then
    sec004_line_mismatches=$((sec004_line_mismatches + 1))
    printf 'BUG032_SEC004_LINE_4097_MISMATCH status=%s classification=%s reason=%s boundary=%s lineBytes=%s tokens=%s candidates=%s retainedTokens=%s retainedCandidates=%s\n' \
      "$BUG032_SECURITY_CLASSIFY_STATUS" "$CHECK8B_CLASSIFICATION" \
      "$CHECK8B_REASON" "$CHECK8B_BOUNDARY" \
      "${CHECK8B_LINE_BYTE_COUNT:-missing}" "$CHECK8B_TOKEN_COUNT" \
      "$CHECK8B_CANDIDATE_COUNT" "${#_CHECK8B_TOKENS[@]}" \
      "${#_CHECK8B_CANDIDATE_INDEXES[@]}"
  fi
  bug032_security_run_scope_case sec004-line-4097 "$security_line_4097"
  if [[ "$BUG032_SECURITY_CASE_STATUS" -eq 0 ]] \
    || ! bug032_security_guard_record_has \
      "$BUG032_SECURITY_CASE_LOG" ambiguous line-byte-limit \
      line-bytes=4097/4096:first-overflow skipped blocked \
      'Shorten only this declaration to at most 4096 bytes while preserving its meaning.'; then
    sec004_line_mismatches=$((sec004_line_mismatches + 1))
  fi
  bug032_security_report "$sec004_line_mismatches" \
    "BUG-032 SEC-004 enforces exact 4096 and 4097 line-byte boundaries"

  local security_token_256=""
  for ((security_index = 0; security_index < 256; security_index++)); do
    security_token_256="${security_token_256}x"
  done
  local security_token_257="${security_token_256}x"
  local security_token_line_256="$security_token_256. Remove the public route"
  local security_token_line_257="$security_token_257. Remove the public route"
  local sec004_token_mismatches=0
  bug032_security_classify "$security_token_line_256"
  if [[ "$BUG032_SECURITY_CLASSIFY_STATUS" -ne 0 ]] \
    || [[ "$CHECK8B_CLASSIFICATION" != "direct-positive" ]] \
    || [[ "$CHECK8B_REASON" != "direct" ]] \
    || [[ "$CHECK8B_BOUNDARY" != "token-bytes=256/256" ]] \
    || [[ "${CHECK8B_TOKEN_BYTE_COUNT:-missing}" != "256" ]] \
    || [[ "$CHECK8B_TOKEN_COUNT" -ne 5 ]] \
    || [[ "$CHECK8B_CANDIDATE_COUNT" -ne 1 ]]; then
    sec004_token_mismatches=$((sec004_token_mismatches + 1))
    printf 'BUG032_SEC004_TOKEN_256_MISMATCH status=%s classification=%s reason=%s boundary=%s tokenBytes=%s tokens=%s candidates=%s\n' \
      "$BUG032_SECURITY_CLASSIFY_STATUS" "$CHECK8B_CLASSIFICATION" \
      "$CHECK8B_REASON" "$CHECK8B_BOUNDARY" \
      "${CHECK8B_TOKEN_BYTE_COUNT:-missing}" "$CHECK8B_TOKEN_COUNT" \
      "$CHECK8B_CANDIDATE_COUNT"
  fi
  bug032_security_run_scope_case sec004-token-256 "$security_token_line_256"
  if [[ "$BUG032_SECURITY_CASE_STATUS" -eq 0 ]] \
    || ! bug032_security_guard_record_has \
      "$BUG032_SECURITY_CASE_LOG" direct-positive direct \
      token-bytes=256/256 run blocked \
      'Add only the missing Consumer Impact Sweep section, completion item, and affected-consumer inventory for direct surfaces: remove:route.'; then
    sec004_token_mismatches=$((sec004_token_mismatches + 1))
  fi

  bug032_security_classify "$security_token_line_257"
  if [[ "$BUG032_SECURITY_CLASSIFY_STATUS" -ne 0 ]] \
    || [[ "$CHECK8B_CLASSIFICATION" != "ambiguous" ]] \
    || [[ "$CHECK8B_REASON" != "token-byte-limit" ]] \
    || [[ "$CHECK8B_BOUNDARY" != "token-bytes=257/256:first-overflow" ]] \
    || [[ "${CHECK8B_TOKEN_BYTE_COUNT:-missing}" != "257" ]] \
    || [[ "$CHECK8B_TOKEN_COUNT" -ne 0 ]] \
    || [[ "$CHECK8B_CANDIDATE_COUNT" -ne 0 ]] \
    || [[ "${#_CHECK8B_TOKENS[@]}" -ne 0 ]] \
    || [[ "${#_CHECK8B_CANDIDATE_INDEXES[@]}" -ne 0 ]]; then
    sec004_token_mismatches=$((sec004_token_mismatches + 1))
    printf 'BUG032_SEC004_TOKEN_257_MISMATCH status=%s classification=%s reason=%s boundary=%s tokenBytes=%s tokens=%s candidates=%s retainedTokens=%s retainedCandidates=%s\n' \
      "$BUG032_SECURITY_CLASSIFY_STATUS" "$CHECK8B_CLASSIFICATION" \
      "$CHECK8B_REASON" "$CHECK8B_BOUNDARY" \
      "${CHECK8B_TOKEN_BYTE_COUNT:-missing}" "$CHECK8B_TOKEN_COUNT" \
      "$CHECK8B_CANDIDATE_COUNT" "${#_CHECK8B_TOKENS[@]}" \
      "${#_CHECK8B_CANDIDATE_INDEXES[@]}"
  fi
  bug032_security_run_scope_case sec004-token-257 "$security_token_line_257"
  if [[ "$BUG032_SECURITY_CASE_STATUS" -eq 0 ]] \
    || ! bug032_security_guard_record_has \
      "$BUG032_SECURITY_CASE_LOG" ambiguous token-byte-limit \
      token-bytes=257/256:first-overflow skipped blocked \
      'Shorten only this token to at most 256 bytes while preserving its meaning.'; then
    sec004_token_mismatches=$((sec004_token_mismatches + 1))
  fi
  bug032_security_report "$sec004_token_mismatches" \
    "BUG-032 SEC-004 enforces exact 256 and 257 token-byte boundaries"

  local security_neutral_126=""
  for ((security_index = 1; security_index <= 126; security_index++)); do
    security_neutral_126="${security_neutral_126}${security_neutral_126:+ }neutral$security_index"
  done
  local security_count_128="$security_neutral_126 remove route"
  local security_count_129="overflow129 $security_count_128"
  local security_candidate_8='remove route remove path remove endpoint remove contract remove api remove url remove slug remove identifier'
  local security_candidate_9="$security_candidate_8 deprecate redirect"
  local sec004_count_mismatches=0
  bug032_security_classify "$security_count_128"
  if [[ "$BUG032_SECURITY_CLASSIFY_STATUS" -ne 0 ]] \
    || [[ "$CHECK8B_CLASSIFICATION" != "direct-positive" ]] \
    || [[ "$CHECK8B_REASON" != "direct" ]] \
    || [[ "$CHECK8B_BOUNDARY" != "tokens=128/128" ]] \
    || [[ "$CHECK8B_TOKEN_COUNT" -ne 128 ]] \
    || [[ "${#_CHECK8B_TOKENS[@]}" -ne 128 ]] \
    || [[ "$CHECK8B_CANDIDATE_COUNT" -ne 1 ]]; then
    sec004_count_mismatches=$((sec004_count_mismatches + 1))
  fi
  bug032_security_classify "$security_count_129"
  if [[ "$BUG032_SECURITY_CLASSIFY_STATUS" -ne 0 ]] \
    || [[ "$CHECK8B_CLASSIFICATION" != "ambiguous" ]] \
    || [[ "$CHECK8B_REASON" != "token-limit" ]] \
    || [[ "$CHECK8B_BOUNDARY" != "tokens=129/128:first-overflow" ]] \
    || [[ "$CHECK8B_TOKEN_COUNT" -ne 128 ]] \
    || [[ "${#_CHECK8B_TOKENS[@]}" -ne 128 ]] \
    || [[ "$CHECK8B_CANDIDATE_COUNT" -ne 0 ]]; then
    sec004_count_mismatches=$((sec004_count_mismatches + 1))
  fi
  bug032_security_classify "$security_candidate_8"
  if [[ "$BUG032_SECURITY_CLASSIFY_STATUS" -ne 0 ]] \
    || [[ "$CHECK8B_CLASSIFICATION" != "direct-positive" ]] \
    || [[ "$CHECK8B_REASON" != "direct" ]] \
    || [[ "$CHECK8B_BOUNDARY" != "candidates=8/8" ]] \
    || [[ "$CHECK8B_CANDIDATE_COUNT" -ne 8 ]] \
    || [[ "${#_CHECK8B_CANDIDATE_INDEXES[@]}" -ne 8 ]]; then
    sec004_count_mismatches=$((sec004_count_mismatches + 1))
  fi
  bug032_security_classify "$security_candidate_9"
  if [[ "$BUG032_SECURITY_CLASSIFY_STATUS" -ne 0 ]] \
    || [[ "$CHECK8B_CLASSIFICATION" != "ambiguous" ]] \
    || [[ "$CHECK8B_REASON" != "candidate-limit" ]] \
    || [[ "$CHECK8B_BOUNDARY" != "candidates=9/8:first-overflow" ]] \
    || [[ "$CHECK8B_CANDIDATE_COUNT" -ne 9 ]] \
    || [[ "${#_CHECK8B_CANDIDATE_INDEXES[@]}" -ne 8 ]]; then
    sec004_count_mismatches=$((sec004_count_mismatches + 1))
  fi
  bug032_security_report "$sec004_count_mismatches" \
    "BUG-032 SEC-004 preserves 128 and 129 token and eight and nine candidate controls"

  local security_oversized_harness="$security_root/oversized-one-token.sh"
  local security_oversized_log="$security_root/oversized-one-token.log"
  cat "$security_classifier_file" > "$security_oversized_harness"
  cat <<'EOF' >> "$security_oversized_harness"
set -euo pipefail
oversized_token=x
while [[ "${#oversized_token}" -lt 100000 ]]; do
  oversized_token="${oversized_token}${oversized_token}"
done
oversized_token="${oversized_token:0:100000}"
semantic_calls=0
set -T
trap '
  case "${FUNCNAME[0]:-}" in
    _check8b_mutation_verb|_check8b_target_after|_check8b_target_before|_check8b_passive_surface|_check8b_named_surface_before|_check8b_surface_from|_check8b_is_surface)
      semantic_calls=$((semantic_calls + 1))
      ;;
  esac
' DEBUG
set +e
check8b_classify_line "$oversized_token"
classify_status=$?
set -e
trap - DEBUG
set +T
printf 'RESULT\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
  "$classify_status" "$CHECK8B_CLASSIFICATION" "$CHECK8B_REASON" \
  "$CHECK8B_BOUNDARY" "${CHECK8B_LINE_BYTE_COUNT:-missing}" \
  "$CHECK8B_TOKEN_COUNT" "$CHECK8B_CANDIDATE_COUNT" \
  "${#_CHECK8B_TOKENS[@]}" "${#_CHECK8B_CANDIDATE_INDEXES[@]}" \
  "$semantic_calls"
EOF
  local security_oversized_status=""
  local security_oversized_tag=""
  local security_oversized_classify_status=""
  local security_oversized_classification=""
  local security_oversized_reason=""
  local security_oversized_boundary=""
  local security_oversized_line_bytes=""
  local security_oversized_token_count=""
  local security_oversized_candidate_count=""
  local security_oversized_retained_tokens=""
  local security_oversized_retained_candidates=""
  local security_oversized_semantic_calls=""
  local sec004_oversized_mismatches=0
  security_oversized_status="$(run_capture "$security_oversized_log" \
    bubbles_run_with_timeout 5 bash "$security_oversized_harness")"
  IFS=$'\t' read -r security_oversized_tag \
    security_oversized_classify_status security_oversized_classification \
    security_oversized_reason security_oversized_boundary \
    security_oversized_line_bytes security_oversized_token_count \
    security_oversized_candidate_count security_oversized_retained_tokens \
    security_oversized_retained_candidates security_oversized_semantic_calls \
    < "$security_oversized_log" || true
  if [[ "$security_oversized_status" -ne 0 ]] \
    || [[ "$security_oversized_tag" != "RESULT" ]] \
    || [[ "$security_oversized_classify_status" -ne 0 ]] \
    || [[ "$security_oversized_classification" != "ambiguous" ]] \
    || [[ "$security_oversized_reason" != "line-byte-limit" ]] \
    || [[ "$security_oversized_boundary" != "line-bytes=4097/4096:first-overflow" ]] \
    || [[ "$security_oversized_line_bytes" != "4097" ]] \
    || [[ "$security_oversized_token_count" -ne 0 ]] \
    || [[ "$security_oversized_candidate_count" -ne 0 ]] \
    || [[ "$security_oversized_retained_tokens" -ne 0 ]] \
    || [[ "$security_oversized_retained_candidates" -ne 0 ]] \
    || [[ "$security_oversized_semantic_calls" -ne 0 ]]; then
    sec004_oversized_mismatches=$((sec004_oversized_mismatches + 1))
    printf 'BUG032_SEC004_OVERSIZED_MISMATCH harnessStatus=%s tag=%s classifyStatus=%s classification=%s reason=%s boundary=%s lineBytes=%s tokenCount=%s candidateCount=%s retainedTokens=%s retainedCandidates=%s semanticCalls=%s\n' \
      "$security_oversized_status" "$security_oversized_tag" \
      "${security_oversized_classify_status:-missing}" \
      "${security_oversized_classification:-missing}" \
      "${security_oversized_reason:-missing}" \
      "${security_oversized_boundary:-missing}" \
      "${security_oversized_line_bytes:-missing}" \
      "${security_oversized_token_count:-missing}" \
      "${security_oversized_candidate_count:-missing}" \
      "${security_oversized_retained_tokens:-missing}" \
      "${security_oversized_retained_candidates:-missing}" \
      "${security_oversized_semantic_calls:-missing}"
  fi
  bug032_security_report "$sec004_oversized_mismatches" \
    "BUG-032 SEC-004 bounds oversized one-token input before semantic work"

  unset -f \
    bug032_security_report bug032_security_counter_value \
    bug032_security_projection_error_matches \
    bug032_security_run_scope_case bug032_security_run_shadow_case \
    bug032_security_run_g040_case bug032_security_classify \
    bug032_security_guard_record_has
}

run_bug032_security_regressions "$positive_feature_dir"
unset -f run_bug032_security_regressions
# END BUG032 SECURITY REGRESSION TESTS

# BUG032-HARDEN9-REPORT-ROUTE-016 / Scope 4 verification: the current Completion
# Statement, state execution route, and planned DAG must agree. The addressed
# state-note finding cannot return to the unresolved route, and the independent
# BUG-035 sibling cannot be absorbed into BUG-032. Mutated state copies prove
# both exclusions are load-bearing.
bug032_route_truth_matches() {
  local report_file="$1"
  local state_file="$2"
  local scopes_file="$3"
  local completion_statement=""
  local serialized_dag=""

  [[ -f "$report_file" && -f "$state_file" && -f "$scopes_file" ]] || return 1
  completion_statement="$(awk '
    $0 == "## Completion Statement" { capture = 1; next }
    capture && /^##[[:space:]]+/ { exit }
    capture { printf "%s ", $0 }
  ' "$report_file")"
  serialized_dag="$(awk '
    $0 == "## Serialized Execution DAG" { capture = 1; next }
    capture && /^##[[:space:]]+/ { exit }
    capture { print }
  ' "$scopes_file")"

  [[ "$completion_statement" == *'The required owner is `bubbles.test`.'* ]] \
    && [[ "$completion_statement" == *'The next action is a test-only A oracle extension for SCN-032-033 through SCN-032-036.'* ]] \
    && [[ "$completion_statement" == *'First replay the unchanged row-145 family and record its actual failure.'* ]] \
    && [[ "$completion_statement" == *'Then add the ten exact security identifiers and capture RED against unchanged production.'* ]] \
    && [[ "$completion_statement" == *'B-RED waits for independent A verification.'* ]] \
    && [[ "$completion_statement" == *'C requires no parallel writer because independent C-GREEN is complete.'* ]] \
    && [[ "$completion_statement" == *'`BUG035-D14-EMPTY-OUTPUT-COUNT` remains outside the BUG-032 target.'* ]] \
    && [[ "$serialized_dag" == *'| A-BASELINE | `bubbles.test` | Evidence only from unchanged row-145 source and selftest family | P | First action |'* ]] \
    && [[ "$serialized_dag" == *'| A-SECURITY-RED | `bubbles.test` | Persistent SCN-032-033 through SCN-032-036 assertions | A-BASELINE | Active route |'* ]] \
    && [[ "$serialized_dag" == *'| A-SECURITY-GREEN | `bubbles.implement` | Planning checks, state-transition guard, unchanged security assertions | A-SECURITY-RED | Waiting |'* ]] \
    && [[ "$serialized_dag" == *'| B-RED | `bubbles.test` | Check 43 assertions in the shared selftest | A-VERIFY | Waiting for A |'* ]] \
    && [[ "$serialized_dag" == *'| C-GREEN | `bubbles.test` | Evidence only | Existing C implementation | Independently verified |'* ]] \
    && [[ "$serialized_dag" != *'| C-RED |'* ]] \
    && jq -e '
      (.execution.nextRequiredOwner == "bubbles.test")
      and (.execution.nextRequiredAction == "Replay the unchanged row-145 family and record its exact result without assigning a cause. Then add only the ten TP-01-04 persistent identifiers for SCN-032-033 through SCN-032-036. Capture all four security findings at RED against unchanged production hashes before any repair. B-RED waits for independent A verification. C-GREEN remains independently verified and receives no writer.")
      and (.execution.parallelReadyOwners == [{
        "owner": "bubbles.test",
        "lane": "A-SECURITY-RED",
        "scope": 1
      }])
      and ((.execution.independentRoutedFindings // []) == ["BUG035-D14-EMPTY-OUTPUT-COUNT"])
      and (any(.executionHistory[];
        ((.addressedFindings // []) | index("BUG032-GAPS-STATE-NOTES-009")) != null))
      and (((.executionHistory[-1].unresolvedFindings // [])
        | index("BUG032-GAPS-STATE-NOTES-009")) == null)
      and (((.executionHistory[-1].independentRoutedFindings // [])
        | index("BUG035-D14-EMPTY-OUTPUT-COUNT")) != null)
      and (((.executionHistory[-1].unresolvedFindings // [])
        | index("BUG035-D14-EMPTY-OUTPUT-COUNT")) == null)
      and (.executionHistory[-1].scope == "BUG-032 revision-71 analyst and design planning reconciliation")
      and (.executionHistory[-1].outcome == "route_required")
      and (.executionHistory[-1].nextRequiredOwner == "bubbles.test")
      and (.executionHistory[-1].nextRequiredAction == "Perform the test-only A oracle correction before any A production work. Remove the rejected G068 same-ID assertion, fold candidate test bindings into surviving scenarios, and capture clean A-RED against unchanged production. B waits for A. C is independently GREEN.")
      and ((.executionHistory[-1].parallelReadyOwners // []) == [])
    ' "$state_file" >/dev/null
}

bug032_route_repo_root="$(cd "$SCRIPT_DIR/../.." && pwd)"
bug032_route_packet="$bug032_route_repo_root/bugs/BUG-032-planning-maturity-guard-false-positives"
bug032_route_mutant_addressed="$tmp_root/bug032-route-addressed-mutant.json"
bug032_route_mutant_sibling="$tmp_root/bug032-route-sibling-mutant.json"
bug032_route_truth_failures=0
if ! bug032_route_truth_matches \
  "$bug032_route_packet/report.md" \
  "$bug032_route_packet/state.json" \
  "$bug032_route_packet/scopes.md"; then
  bug032_route_truth_failures=$((bug032_route_truth_failures + 1))
  printf 'BUG032_S4_REPORT_ROUTE_CURRENT_MISMATCH packet=%s\n' "$bug032_route_packet"
fi
if ! jq '
  .executionHistory[-1].unresolvedFindings =
    ((.executionHistory[-1].unresolvedFindings // []) + ["BUG032-GAPS-STATE-NOTES-009"])
' "$bug032_route_packet/state.json" > "$bug032_route_mutant_addressed"; then
  bug032_route_truth_failures=$((bug032_route_truth_failures + 1))
elif bug032_route_truth_matches \
  "$bug032_route_packet/report.md" \
  "$bug032_route_mutant_addressed" \
  "$bug032_route_packet/scopes.md"; then
  bug032_route_truth_failures=$((bug032_route_truth_failures + 1))
  printf '%s\n' 'BUG032_S4_REPORT_ROUTE_ADDRESSED_FINDING_MUTANT_ACCEPTED'
fi
if ! jq '
  .executionHistory[-1].unresolvedFindings =
    ((.executionHistory[-1].unresolvedFindings // []) + ["BUG035-D14-EMPTY-OUTPUT-COUNT"])
  | .executionHistory[-1].independentRoutedFindings = []
' "$bug032_route_packet/state.json" > "$bug032_route_mutant_sibling"; then
  bug032_route_truth_failures=$((bug032_route_truth_failures + 1))
elif bug032_route_truth_matches \
  "$bug032_route_packet/report.md" \
  "$bug032_route_mutant_sibling" \
  "$bug032_route_packet/scopes.md"; then
  bug032_route_truth_failures=$((bug032_route_truth_failures + 1))
  printf '%s\n' 'BUG032_S4_REPORT_ROUTE_INDEPENDENT_SIBLING_MUTANT_ACCEPTED'
fi
if [[ "$bug032_route_truth_failures" -eq 0 ]]; then
  pass "BUG-032 active Completion Statement routes only unresolved findings"
else
  fail "BUG-032 active Completion Statement routes only unresolved findings (mismatches=$bug032_route_truth_failures)"
fi
unset -f bug032_route_truth_matches

echo "----------------------------------------"
if [[ "$failures" -gt 0 ]]; then
  echo "state-transition-guard selftest failed with $failures issue(s)."
  exit 1
fi

echo "state-transition-guard selftest passed."