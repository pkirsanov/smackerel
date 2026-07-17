#!/usr/bin/env bash
# traceability-guard-selftest.sh
#
# Hermetic selftest for traceability-guard.sh.
#
# Stages a minimal feature dir with scopes.md (Gherkin scenarios + Test
# Plan + DoD), report.md, scenario-manifest.json, and a concrete test
# file under tests/. Then invokes the guard and asserts:
#   - A scope where every Gherkin scenario maps to a Test Plan row,
#     concrete test file, DoD item, and report evidence reference
#     exits 0.
#   - A scope where a Gherkin scenario has no matching Test Plan row
#     exits non-zero with "no traceable Test Plan row" in the output.
#
# Cleans up the temp tree on exit via trap.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/traceability-guard.sh"

if [[ ! -f "$GUARD" ]]; then
  echo "[selftest traceability-guard] FAIL: target script missing at $GUARD" >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "  PASS: $1"; }
fail() { echo "  FAIL: $1"; failures=$((failures + 1)); }

build_clean_feature() {
  local feature_dir="$1"
  local test_plan_heading="${2:-### Test Plan}"
  mkdir -p "$feature_dir/tests"

  cat > "$feature_dir/tests/widget-render.e2e.spec.ts" <<'EOF'
export const widgetRender = true;
EOF

  cat > "$feature_dir/spec.md" <<'EOF'
# Spec — Widget Render
EOF

  cat > "$feature_dir/design.md" <<'EOF'
# Design — Widget Render
EOF

  cat > "$feature_dir/scopes.md" <<EOF
# Scope 01: Widget Render

**Status:** In Progress

### Gherkin

  Scenario: Widget renders with provided label
    Given a label "Hello"
    When the widget mounts
    Then the rendered output displays "Hello"

$test_plan_heading

| Test Type | Category | File/Location | Description | Command | Live System |
| --------- | -------- | ------------- | ----------- | ------- | ----------- |
| E2E       | e2e-ui   | tests/widget-render.e2e.spec.ts | Widget renders with provided label and displays it | selftest:widget-render | Yes |

### Definition of Done

- [x] Widget renders with provided label and displays the rendered output -> Evidence: report.md#test-evidence
EOF

  cat > "$feature_dir/report.md" <<'EOF'
# Report

### Test Evidence

```
$ run tests/widget-render.e2e.spec.ts
PASS tests/widget-render.e2e.spec.ts
```
EOF

  cat > "$feature_dir/scenario-manifest.json" <<'EOF'
{
  "scenarios": [
    {
      "scenarioId": "SCN-01-widget-render",
      "scope": "01-widget-render",
      "title": "Widget renders with provided label",
      "linkedTests": [
        { "file": "tests/widget-render.e2e.spec.ts" }
      ],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF

  cat > "$feature_dir/state.json" <<'EOF'
{
  "version": 3,
  "status": "in_progress",
  "scopeLayout": "single-file"
}
EOF
}

CASE_OUTPUT=""
CASE_STATUS=0
CASE_INDEX=0

run_trace_case() {
  local feature_dir="$1"
  local case_label="$2"
  local case_log

  CASE_INDEX=$((CASE_INDEX + 1))
  case_log="$TMPDIR/bug018-case-${CASE_INDEX}.log"
  CASE_STATUS=0
  if bash "$GUARD" "$feature_dir" >"$case_log" 2>&1; then
    CASE_STATUS=0
  else
    CASE_STATUS=$?
  fi
  CASE_OUTPUT="$(cat "$case_log")"
  echo "[selftest traceability-guard] $case_label (exit $CASE_STATUS)"
}

run_trace_case_system_bash() {
  local feature_dir="$1"
  local case_label="$2"
  local case_log

  CASE_INDEX=$((CASE_INDEX + 1))
  case_log="$TMPDIR/bug018-case-${CASE_INDEX}.log"
  CASE_STATUS=0
  if env -i HOME="$HOME" PATH=/usr/bin:/bin:/usr/sbin:/sbin \
    /bin/bash "$GUARD" "$feature_dir" >"$case_log" 2>&1; then
    CASE_STATUS=0
  else
    CASE_STATUS=$?
  fi
  CASE_OUTPUT="$(cat "$case_log")"
  echo "[selftest traceability-guard] $case_label (system Bash $(/bin/bash -c 'printf "%s" "$BASH_VERSION"'), exit $CASE_STATUS)"
}

run_trace_case_awk_failure() {
  local feature_dir="$1"
  local case_label="$2"
  local case_log
  local shim_dir="$TMPDIR/bug018-awk-failure-bin"
  local real_awk

  real_awk="$(command -v awk)"
  mkdir -p "$shim_dir"
  cat > "$shim_dir/awk" <<'SHIM'
#!/usr/bin/env bash
set -u

: "${BUG018_REAL_AWK:?missing real awk path}"
case "${1:-}" in
  *without_html_comments*) exit 42 ;;
esac
exec "$BUG018_REAL_AWK" "$@"
SHIM
  chmod +x "$shim_dir/awk"

  CASE_INDEX=$((CASE_INDEX + 1))
  case_log="$TMPDIR/bug018-case-${CASE_INDEX}.log"
  CASE_STATUS=0
  if PATH="$shim_dir:$PATH" BUG018_REAL_AWK="$real_awk" \
    bash "$GUARD" "$feature_dir" >"$case_log" 2>&1; then
    CASE_STATUS=0
  else
    CASE_STATUS=$?
  fi
  CASE_OUTPUT="$(cat "$case_log")"
  echo "[selftest traceability-guard] $case_label (exit $CASE_STATUS)"
}

assert_case_status() {
  local expected="$1"
  local label="$2"

  if [[ "$CASE_STATUS" -eq "$expected" ]]; then
    pass "$label"
  else
    fail "$label (expected exit $expected, got $CASE_STATUS)"
    sed -n '1,200p' <<< "$CASE_OUTPUT"
  fi
}

assert_case_contains() {
  local expected="$1"
  local label="$2"

  if grep -Fq -- "$expected" <<< "$CASE_OUTPUT"; then
    pass "$label"
  else
    fail "$label (missing: $expected)"
    sed -n '1,200p' <<< "$CASE_OUTPUT"
  fi
}

assert_case_not_contains() {
  local forbidden="$1"
  local label="$2"

  if grep -Fq -- "$forbidden" <<< "$CASE_OUTPUT"; then
    fail "$label (unexpected: $forbidden)"
    sed -n '1,200p' <<< "$CASE_OUTPUT"
  else
    pass "$label"
  fi
}

assert_case_occurrences() {
  local expected="$1"
  local needle="$2"
  local label="$3"
  local actual=0

  if actual="$(grep -Fc -- "$needle" <<< "$CASE_OUTPUT")"; then
    :
  fi
  if [[ "$actual" -eq "$expected" ]]; then
    pass "$label"
  else
    fail "$label (expected $expected occurrence(s), got $actual: $needle)"
    sed -n '1,200p' <<< "$CASE_OUTPUT"
  fi
}

write_invalid_scope() {
  local feature_dir="$1"
  local test_plan_content="$2"

  cat > "$feature_dir/scopes.md" <<EOF
# Scope 01: Invalid Test Plan

**Status:** In Progress

### Gherkin

  Scenario: Widget renders with provided label
    Given a label "Hello"
    When the widget mounts
    Then the rendered output displays "Hello"

$test_plan_content

### Definition of Done

- [x] Widget renders with provided label and displays the rendered output -> Evidence: report.md#test-evidence
EOF
}

write_false_heading_scope() {
  local feature_dir="$1"

  cat > "$feature_dir/scopes.md" <<'EOF'
# Scope 01: False Test Plan Headings

**Status:** In Progress

### Gherkin

  Scenario: Widget renders with provided label
    Given a label "Hello"
    When the widget mounts
    Then the rendered output displays "Hello"

#### Test Plan

| Test Type | File/Location | Description |
| --- | --- | --- |
| E2E | tests/widget-render.e2e.spec.ts | Widget renders with provided label |

### Test Planning

| Test Type | File/Location | Description |
| --- | --- | --- |
| E2E | tests/widget-render.e2e.spec.ts | Widget renders with provided label |

```text
## Test Plan
| E2E | tests/widget-render.e2e.spec.ts | Widget renders with provided label |
```

<!--
### Test Plan
| E2E | tests/widget-render.e2e.spec.ts | Widget renders with provided label |
-->

### Definition of Done

- [x] Widget renders with provided label and displays the rendered output -> Evidence: report.md#test-evidence
EOF
}

write_boundary_scope() {
  local feature_dir="$1"
  local test_plan_heading="$2"
  local nested_heading="$3"
  local sibling_heading="$4"

  cat > "$feature_dir/scopes.md" <<EOF
# Scope 01: Test Plan Boundary

**Status:** In Progress

### Gherkin

  Scenario: Widget renders with provided label
    Given a label "Hello"
    When the widget mounts
    Then the rendered output displays "Hello"

$test_plan_heading

$nested_heading

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| E2E | e2e-ui | tests/widget-render.e2e.spec.ts | Widget renders with provided label and displays it | selftest:nested | Yes |

$sibling_heading

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| E2E | e2e-ui | tests/must-not-leak.e2e.spec.ts | Unrelated later sibling row | selftest:sibling | Yes |

#### Definition of Done

- [x] Widget renders with provided label and displays the rendered output -> Evidence: report.md#test-evidence
EOF
}

write_no_scenario_scope() {
  local feature_dir="$1"

  cat > "$feature_dir/scopes.md" <<'EOF'
# Scope 01: No Scenario

**Status:** In Progress

### Gherkin

This fixture intentionally contains no executable Scenario line.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| E2E | e2e-ui | tests/widget-render.e2e.spec.ts | Expected no-match reaches diagnostic | selftest:no-scenario | Yes |

### Definition of Done

- [x] Expected no-scenario input reaches its explicit diagnostic
EOF
}

# --- Case 1: clean feature dir → exit 0 ---
clean_feature="$TMPDIR/specs/100-clean-feature"
build_clean_feature "$clean_feature"

echo "[selftest traceability-guard] Case 1: clean feature → exit 0"
log1="$TMPDIR/log1.txt"
set +e
bash "$GUARD" "$clean_feature" >"$log1" 2>&1
status1=$?
set -e
if [[ "$status1" -eq 0 ]]; then
  pass "clean feature exits 0 (got $status1)"
else
  fail "clean feature should exit 0 (got $status1)"
  sed -n '1,120p' "$log1"
fi
if grep -Fq 'scenario mapped to Test Plan row' "$log1"; then
  pass "output reports scenario→row mapping"
else
  fail "expected 'scenario mapped to Test Plan row' line"
  sed -n '1,120p' "$log1"
fi
if grep -Fq 'scenario→row match confidence: inferred' "$log1"; then
  pass "Case 1 reports inferred edge confidence (no trace id)"
else
  fail "expected 'scenario→row match confidence: inferred' in Case 1 log"
  sed -n '1,120p' "$log1"
fi

# --- Case 2: scenario without matching Test Plan row → exit non-zero ---
broken_feature="$TMPDIR/specs/200-broken-feature"
build_clean_feature "$broken_feature"

# Replace the scopes.md so the Gherkin scenario describes a totally
# different behavior than the Test Plan row, breaking the trace.
cat > "$broken_feature/scopes.md" <<'EOF'
# Scope 01: Detached Widget

**Status:** In Progress

### Gherkin

  Scenario: Submit form persists customer billing address to server
    Given a customer billing address form
    When the operator submits the address
    Then the address persists on the server

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --------- | -------- | ------------- | ----------- | ------- | ----------- |
| E2E       | e2e-ui   | tests/widget-render.e2e.spec.ts | Widget renders with provided label and displays it | selftest:widget-render | Yes |

### Definition of Done

- [x] Widget renders with provided label and displays the rendered output -> Evidence: report.md#test-evidence
EOF

# Update scenario-manifest.json so it still matches scope-defined scenario count.
cat > "$broken_feature/scenario-manifest.json" <<'EOF'
{
  "scenarios": [
    {
      "scenarioId": "SCN-01-detached",
      "scope": "01-detached-widget",
      "title": "Submit form persists customer billing address to server",
      "linkedTests": [
        { "file": "tests/widget-render.e2e.spec.ts" }
      ],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF

echo "[selftest traceability-guard] Case 2: untraceable scenario → exit non-zero"
log2="$TMPDIR/log2.txt"
set +e
bash "$GUARD" "$broken_feature" >"$log2" 2>&1
status2=$?
set -e
if [[ "$status2" -ne 0 ]]; then
  pass "untraceable scenario exits non-zero (got $status2)"
else
  fail "untraceable scenario should exit non-zero (got $status2)"
  sed -n '1,160p' "$log2"
fi
if grep -Fq 'no traceable Test Plan row' "$log2" \
  || grep -Fq 'no faithful DoD item' "$log2"; then
  pass "output surfaces traceability or DoD-fidelity violation"
else
  fail "expected 'no traceable Test Plan row' or 'no faithful DoD item' in output"
  sed -n '1,160p' "$log2"
fi

# --- Case 3: scenario + row share a trace id → declared edge ---
declared_feature="$TMPDIR/specs/300-declared-feature"
build_clean_feature "$declared_feature"

cat > "$declared_feature/scopes.md" <<'EOF'
# Scope 01: Declared Trace

**Status:** In Progress

### Gherkin

  Scenario: SCN-07-declared user sees confirmation
    Given a submitted form
    When the server responds
    Then the user sees a confirmation message

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --------- | -------- | ------------- | ----------- | ------- | ----------- |
| E2E       | e2e-ui   | tests/widget-render.e2e.spec.ts | SCN-07-declared user sees confirmation message | selftest:declared | Yes |

### Definition of Done

- [x] SCN-07-declared user sees confirmation message -> Evidence: report.md#test-evidence
EOF

echo "[selftest traceability-guard] Case 3: shared trace id → declared edge"
log3="$TMPDIR/log3.txt"
set +e
bash "$GUARD" "$declared_feature" >"$log3" 2>&1
status3=$?
set -e
if [[ "$status3" -eq 0 ]]; then
  pass "declared-edge feature exits 0 (got $status3)"
else
  fail "declared-edge feature should exit 0 (got $status3)"
  sed -n '1,160p' "$log3"
fi
if grep -Fq 'match confidence: declared' "$log3"; then
  pass "Case 3 reports declared edge confidence (shared trace id)"
else
  fail "expected 'match confidence: declared' in Case 3 log"
  sed -n '1,160p' "$log3"
fi

# --- Case 4: scenario fuzzy-matches two rows → ambiguous edge ---
ambiguous_feature="$TMPDIR/specs/400-ambiguous-feature"
build_clean_feature "$ambiguous_feature"

cat > "$ambiguous_feature/scopes.md" <<'EOF'
# Scope 01: Ambiguous Trace

**Status:** In Progress

### Gherkin

  Scenario: dashboard renders provided label correctly
    Given a dashboard label
    When the dashboard mounts
    Then the dashboard renders the provided label

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --------- | -------- | ------------- | ----------- | ------- | ----------- |
| E2E       | e2e-ui   | tests/widget-render.e2e.spec.ts | dashboard renders provided label promptly | selftest:ambiguous-a | Yes |
| E2E       | e2e-ui   | tests/widget-render.e2e.spec.ts | dashboard renders provided label smoothly | selftest:ambiguous-b | Yes |

### Definition of Done

- [x] dashboard renders provided label promptly -> Evidence: report.md#test-evidence
EOF

echo "[selftest traceability-guard] Case 4: two fuzzy row matches → ambiguous edge"
log4="$TMPDIR/log4.txt"
set +e
bash "$GUARD" "$ambiguous_feature" >"$log4" 2>&1
status4=$?
set -e
if [[ "$status4" -eq 0 ]]; then
  pass "ambiguous-edge feature exits 0 (got $status4)"
else
  fail "ambiguous-edge feature should exit 0 (got $status4)"
  sed -n '1,160p' "$log4"
fi
if grep -Fq 'scenario→row match confidence: ambiguous' "$log4"; then
  pass "Case 4 reports ambiguous edge confidence (two fuzzy row matches)"
else
  fail "expected 'scenario→row match confidence: ambiguous' in Case 4 log"
  sed -n '1,160p' "$log4"
fi

# --- BUG-018: exact level-2/level-3 heading equivalence ---
level2_feature="$TMPDIR/specs/500-level2-feature"
build_clean_feature "$level2_feature" "## Test Plan"
run_trace_case "$level2_feature" "Case 5: exact level-2 Test Plan"
assert_case_status 0 "Case 5 level-2 feature exits 0"
assert_case_contains 'scenario mapped to Test Plan row' "Case 5 level-2 scenario maps"
assert_case_contains 'RESULT: PASSED (0 warnings)' "Case 5 level-2 reaches final summary"
level2_log="$TMPDIR/bug018-level2.log"
printf '%s\n' "$CASE_OUTPUT" > "$level2_log"

level2_mappings=""
level3_mappings=""
if level2_mappings="$(grep -F 'scenario mapped to Test Plan row:' "$level2_log" | sed -E 's/^.*scenario mapped to Test Plan row:[[:space:]]*//' | LC_ALL=C sort)"; then
  :
fi
if level3_mappings="$(grep -F 'scenario mapped to Test Plan row:' "$log1" | sed -E 's/^.*scenario mapped to Test Plan row:[[:space:]]*//' | LC_ALL=C sort)"; then
  :
fi
if [[ -n "$level2_mappings" && "$level2_mappings" == "$level3_mappings" ]]; then
  pass "Case 5 level-2 mapping set equals the existing level-3 mapping set"
else
  fail "Case 5 level-2 and level-3 mapping sets differ"
fi

# --- BUG-018: missing and lookalike headings are distinct from rowless ---
missing_feature="$TMPDIR/specs/600-missing-heading"
build_clean_feature "$missing_feature"
write_invalid_scope "$missing_feature" "This scope has no Test Plan heading."
run_trace_case "$missing_feature" "Case 6: missing exact Test Plan"
assert_case_status 1 "Case 6 missing heading exits 1"
assert_case_occurrences 1 'has no recognized Test Plan section (expected exact ## Test Plan or ### Test Plan)' "Case 6 missing heading reports once"
assert_case_not_contains 'has no concrete Test Plan rows to trace' "Case 6 missing heading is not rowless"
assert_case_contains 'RESULT: FAILED (1 failures, 0 warnings)' "Case 6 missing heading reaches final summary"

false_heading_feature="$TMPDIR/specs/610-false-headings"
build_clean_feature "$false_heading_feature"
write_false_heading_scope "$false_heading_feature"
run_trace_case "$false_heading_feature" "Case 6: unsupported heading lookalikes"
assert_case_status 1 "Case 6 unsupported headings exit 1"
assert_case_occurrences 1 'has no recognized Test Plan section (expected exact ## Test Plan or ### Test Plan)' "Case 6 depth-four, Test Planning, fenced, and commented headings remain unrecognized"
assert_case_contains 'RESULT: FAILED (1 failures, 0 warnings)' "Case 6 unsupported headings reach final summary"

# --- BUG-018: recognized empty/header/separator-only sections are rowless ---
empty_feature="$TMPDIR/specs/700-empty-test-plan"
build_clean_feature "$empty_feature"
write_invalid_scope "$empty_feature" "### Test Plan"
run_trace_case "$empty_feature" "Case 7: empty recognized Test Plan"
assert_case_status 1 "Case 7 empty section exits 1"
assert_case_occurrences 1 'has no concrete Test Plan rows to trace' "Case 7 empty section reports rowless once"
assert_case_not_contains 'has no recognized Test Plan section' "Case 7 empty section remains recognized"
assert_case_contains 'RESULT: FAILED (1 failures, 0 warnings)' "Case 7 empty section reaches final summary"

separator_feature="$TMPDIR/specs/710-separator-test-plan"
build_clean_feature "$separator_feature"
write_invalid_scope "$separator_feature" $'### Test Plan\n\n| --- | --- | --- |'
run_trace_case "$separator_feature" "Case 7: separator-only Test Plan"
assert_case_status 1 "Case 7 separator-only section exits 1"
assert_case_occurrences 1 'has no concrete Test Plan rows to trace' "Case 7 separator-only section reports rowless once"
assert_case_contains 'RESULT: FAILED (1 failures, 0 warnings)' "Case 7 separator-only section reaches final summary"

header_feature="$TMPDIR/specs/720-header-test-plan"
build_clean_feature "$header_feature"
write_invalid_scope "$header_feature" $'### Test Plan\n\n| Test Type | File/Location | Description |\n| --- | --- | --- |'
run_trace_case "$header_feature" "Case 7: header-only Test Plan"
assert_case_status 1 "Case 7 header-only section exits 1"
assert_case_occurrences 1 'has no concrete Test Plan rows to trace' "Case 7 header-only section reports rowless once"
assert_case_contains 'RESULT: FAILED (1 failures, 0 warnings)' "Case 7 header-only section reaches final summary"

run_trace_case_awk_failure "$clean_feature" "Case 7: Test Plan extractor failure"
assert_case_status 1 "Case 7 extractor failure exits 1"
assert_case_occurrences 1 'Test Plan extraction failed' "Case 7 extractor failure reports distinctly once"
assert_case_not_contains 'has no recognized Test Plan section' "Case 7 extractor failure is not missing"
assert_case_not_contains 'has no concrete Test Plan rows to trace' "Case 7 extractor failure is not rowless"
assert_case_contains 'RESULT: FAILED (1 failures, 0 warnings)' "Case 7 extractor failure reaches final summary"

# --- BUG-018: deeper content remains and same-depth siblings stop extraction ---
boundary2_feature="$TMPDIR/specs/800-level2-boundary"
build_clean_feature "$boundary2_feature"
write_boundary_scope "$boundary2_feature" "## Test Plan" "### Nested Cases" "## Later Same-Depth Section"
run_trace_case "$boundary2_feature" "Case 8: level-2 depth boundary"
assert_case_status 0 "Case 8 level-2 boundary exits 0"
assert_case_contains 'scenario mapped to Test Plan row' "Case 8 level-2 nested row remains eligible"
assert_case_contains 'summary: scenarios=1 test_rows=1' "Case 8 level-2 same-depth sibling is excluded"
assert_case_not_contains 'must-not-leak.e2e.spec.ts' "Case 8 level-2 sibling path remains inert"

boundary3_feature="$TMPDIR/specs/810-level3-boundary"
build_clean_feature "$boundary3_feature"
write_boundary_scope "$boundary3_feature" "### Test Plan" "#### Nested Cases" "### Later Same-Depth Section"
run_trace_case "$boundary3_feature" "Case 8: level-3 depth boundary"
assert_case_status 0 "Case 8 level-3 boundary exits 0"
assert_case_contains 'scenario mapped to Test Plan row' "Case 8 level-3 nested row remains eligible"
assert_case_contains 'summary: scenarios=1 test_rows=1' "Case 8 level-3 same-depth sibling is excluded"
assert_case_not_contains 'must-not-leak.e2e.spec.ts' "Case 8 level-3 sibling path remains inert"

# --- BUG-018: expected scenario no-match reaches the existing diagnostic ---
no_scenario_feature="$TMPDIR/specs/900-no-scenario"
build_clean_feature "$no_scenario_feature"
write_no_scenario_scope "$no_scenario_feature"
run_trace_case "$no_scenario_feature" "Case 9: expected no-scenario no-match"
assert_case_status 1 "Case 9 no-scenario feature exits 1"
assert_case_occurrences 1 'has no Gherkin scenarios to trace' "Case 9 no-scenario diagnostic appears once"
assert_case_contains 'RESULT: FAILED (1 failures, 0 warnings)' "Case 9 no-scenario feature reaches final summary"

# --- BUG-018: optional fun mode cannot block macOS system Bash 3.2 ---
run_trace_case_system_bash "$clean_feature" "Case 10: system Bash startup"
assert_case_status 0 "Case 10 system Bash exits 0"
assert_case_contains 'RESULT: PASSED (0 warnings)' "Case 10 system Bash reaches final summary"
assert_case_not_contains 'unbound variable' "Case 10 optional fun mode does not break startup"

if [[ "$failures" -eq 0 ]]; then
  echo "[selftest traceability-guard] PASS"
  exit 0
fi

echo "[selftest traceability-guard] FAIL: $failures assertion(s)"
exit 1
