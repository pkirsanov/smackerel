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
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=guard-lib.sh
source "$SCRIPT_DIR/guard-lib.sh"

if [[ ! -f "$GUARD" ]]; then
  echo "[selftest traceability-guard] FAIL: target script missing at $GUARD" >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
REPO_FIXTURE_ROOT="$(mktemp -d "$REPO_ROOT/.traceability-guard-selftest.XXXXXX")"
trap 'rm -rf "$TMPDIR" "$REPO_FIXTURE_ROOT"' EXIT INT TERM

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
  "schemaVersion": 1,
  "scenarios": [
    {
      "scenarioId": "SCN-WIDGET-01",
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

build_bug039_feature028_shape() {
  local feature_dir="$1"
  local scope_number row_number row_id scenario_id scenario_title

  mkdir -p "$feature_dir/tests"
  git init -q "$feature_dir"
  cat > "$feature_dir/tests/traceability.spec.ts" <<'EOF'
export const traceabilityFixture = true;
EOF
  cat > "$feature_dir/spec.md" <<'EOF'
# Spec — Feature 028 Shape
EOF
  cat > "$feature_dir/design.md" <<'EOF'
# Design — Feature 028 Shape
EOF
  cat > "$feature_dir/report.md" <<'EOF'
# Report

### Test Evidence

tests/traceability.spec.ts is the persistent regression surface.
EOF
  cat > "$feature_dir/state.json" <<'EOF'
{
  "version": 3,
  "status": "in_progress",
  "scopeLayout": "single-file"
}
EOF
  cat > "$feature_dir/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "scenarios": [
    {"id":"SCN-028-A-01","title":"Scope 1 primary behavior","linkedTests":["tests/traceability.spec.ts"],"evidenceRefs":["report.md#test-evidence"]},
    {"id":"SCN-028-B-01","title":"Scope 2 primary behavior","linkedTests":["tests/traceability.spec.ts"],"evidenceRefs":["report.md#test-evidence"]},
    {"id":"SCN-028-C-01","title":"Scope 3 primary behavior","linkedTests":["tests/traceability.spec.ts"],"evidenceRefs":["report.md#test-evidence"]},
    {"id":"SCN-028-D-01","title":"Scope 4 primary behavior","linkedTests":["tests/traceability.spec.ts"],"evidenceRefs":["report.md#test-evidence"]}
  ]
}
EOF

  : > "$feature_dir/scopes.md"
  printf '# Scopes — Feature 028 Shape\n\n' >> "$feature_dir/scopes.md"
  scope_number=1
  while [[ "$scope_number" -le 4 ]]; do
    case "$scope_number" in
      1) scenario_id="SCN-028-A-01" ;;
      2) scenario_id="SCN-028-B-01" ;;
      3) scenario_id="SCN-028-C-01" ;;
      *) scenario_id="SCN-028-D-01" ;;
    esac
    scenario_title="Scope $scope_number primary behavior"
    cat >> "$feature_dir/scopes.md" <<EOF
## Scope $scope_number: Exact Table $scope_number

**Status:** Not Started

### $scenario_id
  Scenario: $scenario_title

### Test Plan

| ID | Type | Scenario | Persistent file and exact title | Command | Required behavior |
| --- | --- | --- | --- | --- | --- |
EOF
    row_number=1
    while [[ "$row_number" -le 6 ]]; do
      row_id=$(((scope_number - 1) * 6 + row_number))
      if [[ "$row_number" -eq 1 ]]; then
        printf '| TP-028-%02d | regression | %s | `tests/traceability.spec.ts` — %s | selftest:traceability | Maps the primary behavior |\n' \
          "$row_id" "$scenario_id" "$scenario_title" >> "$feature_dir/scopes.md"
      else
        printf '| TP-028-%02d | unit | SCN-028-%d%02d | `tests/traceability.spec.ts` — Auxiliary behavior %d | selftest:traceability | Covers one auxiliary path |\n' \
          "$row_id" "$scope_number" "$row_number" "$row_id" >> "$feature_dir/scopes.md"
      fi
      row_number=$((row_number + 1))
    done
    cat >> "$feature_dir/scopes.md" <<EOF

### Definition of Done

- [ ] $scenario_id $scenario_title -> Evidence: report.md#test-evidence

EOF
    scope_number=$((scope_number + 1))
  done
}

CASE_OUTPUT=""
CASE_STATUS=0
CASE_INDEX=0

run_trace_case() {
  local feature_dir="$1"
  local case_label="$2"
  local case_log
  local feature_arg="$feature_dir"
  local run_root="$PWD"
  shift 2

  if [[ "$feature_dir" == "$TMPDIR/"* || "$feature_dir" == "$REPO_FIXTURE_ROOT/"* ]]; then
    feature_arg="."
    run_root="$feature_dir"
  fi

  CASE_INDEX=$((CASE_INDEX + 1))
  case_log="$TMPDIR/bug018-case-${CASE_INDEX}.log"
  CASE_STATUS=0
  if (cd "$run_root" && bash "$GUARD" "$feature_arg" "$@") >"$case_log" 2>&1; then
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
  local feature_arg="$feature_dir"
  local run_root="$PWD"

  if [[ "$feature_dir" == "$TMPDIR/"* ]]; then
    feature_arg="."
    run_root="$feature_dir"
  fi

  CASE_INDEX=$((CASE_INDEX + 1))
  case_log="$TMPDIR/bug018-case-${CASE_INDEX}.log"
  CASE_STATUS=0
  if (cd "$run_root" && /usr/bin/env -i HOME="$HOME" PATH="$PATH" \
    /bin/bash "$GUARD" "$feature_arg") >"$case_log" 2>&1; then
    CASE_STATUS=0
  else
    CASE_STATUS=$?
  fi
  CASE_OUTPUT="$(cat "$case_log")"
  echo "[selftest traceability-guard] $case_label (system Bash $(/bin/bash -c 'printf "%s" "$BASH_VERSION"'), exit $CASE_STATUS)"
}

run_trace_case_parser_failure() {
  local feature_dir="$1"
  local case_label="$2"
  local case_log
  local shim_dir="$TMPDIR/bug018-parser-failure-bin"
  local real_python
  local feature_arg="$feature_dir"
  local run_root="$PWD"

  if [[ "$feature_dir" == "$TMPDIR/"* ]]; then
    feature_arg="."
    run_root="$feature_dir"
  fi

  real_python="$(command -v python3)"
  mkdir -p "$shim_dir"
  cat > "$shim_dir/python3" <<'SHIM'
#!/usr/bin/env bash
set -u

: "${BUG018_REAL_PYTHON:?missing real python path}"
if [[ "${1:-}" == "-" && "${2:-}" == */scopes.md ]]; then
  exit 42
fi
exec "$BUG018_REAL_PYTHON" "$@"
SHIM
  chmod +x "$shim_dir/python3"

  CASE_INDEX=$((CASE_INDEX + 1))
  case_log="$TMPDIR/bug018-case-${CASE_INDEX}.log"
  CASE_STATUS=0
  if (cd "$run_root" && PATH="$shim_dir:$PATH" BUG018_REAL_PYTHON="$real_python" \
    bash "$GUARD" "$feature_arg") >"$case_log" 2>&1; then
    CASE_STATUS=0
  else
    CASE_STATUS=$?
  fi
  CASE_OUTPUT="$(cat "$case_log")"
  echo "[selftest traceability-guard] $case_label (exit $CASE_STATUS)"
}

run_trace_case_with_parser_status() {
  local feature_dir="$1"
  local case_label="$2"
  local case_log
  local shim_dir="$TMPDIR/bug039-parser-status-bin"
  local real_python
  local feature_arg="$feature_dir"
  local run_root="$PWD"
  shift 2

  if [[ "$feature_dir" == "$TMPDIR/"* || "$feature_dir" == "$REPO_FIXTURE_ROOT/"* ]]; then
    feature_arg="."
    run_root="$feature_dir"
  fi

  real_python="$(command -v python3)"
  mkdir -p "$shim_dir"
  cat > "$shim_dir/python3" <<'SHIM'
#!/usr/bin/env bash
set -u

: "${TRACEABILITY_REAL_PYTHON:?missing real python path}"
stdout_capture="$(mktemp)"
stderr_capture="$(mktemp)"
  script_capture="$(mktemp)"
  trap 'rm -f "$stdout_capture" "$stderr_capture" "$script_capture"' EXIT INT TERM
status=0
  parser_invocation=0
  if [[ "${1:-}" == "-" ]]; then
    cat >"$script_capture"
    shift
    if grep -Fq 'def fail_structure(message, line_number):' "$script_capture"; then
      parser_invocation=1
    fi
    "$TRACEABILITY_REAL_PYTHON" "$script_capture" "$@" >"$stdout_capture" 2>"$stderr_capture" || status=$?
  else
    "$TRACEABILITY_REAL_PYTHON" "$@" >"$stdout_capture" 2>"$stderr_capture" || status=$?
  fi
cat "$stdout_capture"
cat "$stderr_capture" >&2
  if [[ "$parser_invocation" -eq 1 ]]; then
  printf 'SELFTEST_PARSER_STATUS=%s\n' "$status" >&2
fi
exit "$status"
SHIM
  chmod +x "$shim_dir/python3"

  CASE_INDEX=$((CASE_INDEX + 1))
  case_log="$TMPDIR/bug039-parser-status-${CASE_INDEX}.log"
  CASE_STATUS=0
  if (cd "$run_root" && PATH="$shim_dir:$PATH" TRACEABILITY_REAL_PYTHON="$real_python" \
    bash "$GUARD" "$feature_arg" "$@") >"$case_log" 2>&1; then
    CASE_STATUS=0
  else
    CASE_STATUS=$?
  fi
  CASE_OUTPUT="$(cat "$case_log")"
  echo "[selftest traceability-guard] $case_label (exit $CASE_STATUS)"
}

run_trace_case_identity_rewrite() {
  local feature_dir="$1"
  local case_label="$2"
  local case_log
  local shim_dir="$TMPDIR/identity-rewrite-bin"
  local real_python

  real_python="$(command -v python3)"
  mkdir -p "$shim_dir"
  cat > "$shim_dir/python3" <<'SHIM'
#!/usr/bin/env bash
set -u
: "${IDENTITY_REAL_PYTHON:?missing real python path}"
: "${IDENTITY_TARGET:?missing rewrite target}"
capture="$(mktemp)"
trap 'rm -f "$capture"' EXIT INT TERM
if "$IDENTITY_REAL_PYTHON" "$@" >"$capture"; then
  status=0
else
  status=$?
fi
if [[ "$status" -eq 0 && "${1:-}" == "-" && "${4:-}" == "tests/widget-render.e2e.spec.ts" ]]; then
  printf '\nidentity rewrite\n' >> "$IDENTITY_TARGET"
fi
cat "$capture"
exit "$status"
SHIM
  chmod +x "$shim_dir/python3"

  CASE_INDEX=$((CASE_INDEX + 1))
  case_log="$TMPDIR/bug018-case-${CASE_INDEX}.log"
  CASE_STATUS=0
  if (cd "$feature_dir" && PATH="$shim_dir:$PATH" IDENTITY_REAL_PYTHON="$real_python" \
    IDENTITY_TARGET="$feature_dir/tests/widget-render.e2e.spec.ts" \
    bash "$GUARD" ".") >"$case_log" 2>&1; then
    CASE_STATUS=0
  else
    CASE_STATUS=$?
  fi
  CASE_OUTPUT="$(cat "$case_log")"
  echo "[selftest traceability-guard] $case_label (exit $CASE_STATUS)"
}

run_trace_case_mode_operation() {
  local feature_dir="$1"
  local case_label="$2"
  local operation="$3"
  local case_log
  local shim_dir="$TMPDIR/identity-mode-bin"
  local real_python

  real_python="$(command -v python3)"
  mkdir -p "$shim_dir"
  cat > "$shim_dir/python3" <<'SHIM'
#!/usr/bin/env bash
set -u
: "${IDENTITY_REAL_PYTHON:?missing real python path}"
: "${IDENTITY_TARGET:?missing identity target}"
: "${IDENTITY_OPERATION:?missing identity operation}"
capture="$(mktemp)"
trap 'rm -f "$capture"' EXIT INT TERM
status=0
"$IDENTITY_REAL_PYTHON" "$@" >"$capture" || status=$?
if [[ "$status" -eq 0 && "${1:-}" == */scenario-reference-reader.py ]]; then
  current_mode="$(stat -c '%a' "$IDENTITY_TARGET" 2>/dev/null || stat -f '%Lp' "$IDENTITY_TARGET")"
  if [[ "$IDENTITY_OPERATION" == "change" ]]; then chmod 600 "$IDENTITY_TARGET"; else chmod "$current_mode" "$IDENTITY_TARGET"; fi
fi
cat "$capture"
exit "$status"
SHIM
  chmod +x "$shim_dir/python3"

  CASE_INDEX=$((CASE_INDEX + 1))
  case_log="$TMPDIR/identity-mode-case-${CASE_INDEX}.log"
  CASE_STATUS=0
  if (cd "$feature_dir" && PATH="$shim_dir:$PATH" IDENTITY_REAL_PYTHON="$real_python" \
    IDENTITY_TARGET="$feature_dir/tests/widget-render.e2e.spec.ts" IDENTITY_OPERATION="$operation" \
    bash "$GUARD" ".") >"$case_log" 2>&1; then CASE_STATUS=0; else CASE_STATUS=$?; fi
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

B046_CASE_LOG=""

build_b046_feature() {
  local repo_dir="$1"
  local feature_rel="$2"
  local feature_dir="$repo_dir/$feature_rel"

  mkdir -p "$repo_dir/tests" "$feature_dir/tests"

  cat > "$repo_dir/tests/repository-control.spec.ts" <<'EOF'
export const repositoryControl = true;
EOF
  cat > "$feature_dir/tests/feature-control.spec.ts" <<'EOF'
export const featureControl = true;
EOF
  cat > "$feature_dir/spec.md" <<'EOF'
# Spec - Contained Linked Test Fixture
EOF
  cat > "$feature_dir/design.md" <<'EOF'
# Design - Contained Linked Test Fixture
EOF
  cat > "$feature_dir/scopes.md" <<'EOF'
# Scope 01: Contained Linked Test Fixture

**Status:** In Progress

### Gherkin

#### SCN-B046-FIXTURE-001

Scenario: Linked test reference validation remains contained
  Given a repository-controlled linked test reference
  When the traceability guard evaluates the reference
  Then only a contained regular test file can satisfy the edge

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| Functional | functional | tests/feature-control.spec.ts | SCN-B046-FIXTURE-001 linked test reference validation remains contained | selftest:contained-reference | No |

### Definition of Done

- [x] SCN-B046-FIXTURE-001 only a contained regular test file satisfies the linked-test edge -> Evidence: report.md#test-evidence
EOF
  cat > "$feature_dir/report.md" <<'EOF'
# Report

### Test Evidence

tests/feature-control.spec.ts passed.
EOF
  cat > "$feature_dir/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-B046-FIXTURE-001",
      "title": "Linked test reference validation remains contained",
      "linkedTests": ["tests/feature-control.spec.ts"],
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

run_b046_trace_case() {
  local repo_dir="$1"
  local feature_rel="$2"
  local case_label="$3"
  local scope_mode="${4:-}"
  local selected_guard="${5:-$GUARD}"
  local case_log

  CASE_INDEX=$((CASE_INDEX + 1))
  case_log="$TMPDIR/b046-case-${CASE_INDEX}.log"
  CASE_STATUS=0
  if [[ -n "$scope_mode" ]]; then
    if (cd "$repo_dir" && bash "$selected_guard" "$feature_rel" "$scope_mode") >"$case_log" 2>&1; then
      CASE_STATUS=0
    else
      CASE_STATUS=$?
    fi
  elif (cd "$repo_dir" && bash "$selected_guard" "$feature_rel") >"$case_log" 2>&1; then
    CASE_STATUS=0
  else
    CASE_STATUS=$?
  fi
  B046_CASE_LOG="$case_log"
  CASE_OUTPUT="$(cat "$case_log")"
  echo "[selftest traceability-guard] $case_label (exit $CASE_STATUS)"
}

run_b046_trace_case_system_bash() {
  local repo_dir="$1"
  local feature_rel="$2"
  local case_label="$3"
  local case_log

  CASE_INDEX=$((CASE_INDEX + 1))
  case_log="$TMPDIR/b046-case-${CASE_INDEX}.log"
  CASE_STATUS=0
  if (cd "$repo_dir" && /bin/bash "$GUARD" "$feature_rel") >"$case_log" 2>&1; then
    CASE_STATUS=0
  else
    CASE_STATUS=$?
  fi
  B046_CASE_LOG="$case_log"
  CASE_OUTPUT="$(cat "$case_log")"
  echo "[selftest traceability-guard] $case_label (system Bash $(/bin/bash -c 'printf "%s" "$BASH_VERSION"'), exit $CASE_STATUS)"
}

assert_b046_log_has_no_active_control_bytes() {
  local label="$1"
  local bad_bytes

  bad_bytes="$(LC_ALL=C od -An -tu1 "$B046_CASE_LOG" | awk '
    {
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^[0-9]+$/ && (($i < 32 && $i != 10) || $i == 127)) print $i
      }
    }
  ')"
  if [[ -z "$bad_bytes" ]]; then
    pass "$label"
  else
    fail "$label (captured output contains active control byte(s): ${bad_bytes//$'\n'/,})"
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
(cd "$clean_feature" && bash "$GUARD" ".") >"$log1" 2>&1
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

# --- Case 1b: canonical manifest id + string linkedTests are first-class -----
canonical_feature="$TMPDIR/specs/101-canonical-manifest"
build_clean_feature "$canonical_feature"
cat > "$canonical_feature/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-CANON-001",
      "title": "Widget renders with provided label",
      "requiredTestType": "e2e-ui",
      "linkedTests": ["tests/widget-render.e2e.spec.ts"],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_trace_case "$canonical_feature" "canonical manifest id and string linked test"
assert_case_status 0 "canonical scenario-manifest envelope exits 0"
assert_case_contains "scenario-manifest.json covers 1 scenario contract(s)" \
  "canonical id is counted as a scenario contract"
assert_case_contains "scenario-manifest.json linked test exists: tests/widget-render.e2e.spec.ts" \
  "canonical string linkedTests path is validated"
assert_case_contains "All 1 authored linked test reference(s) from scenario-manifest.json exist" \
  "canonical authored link emits aggregate success only after one reference resolves"
assert_case_contains "scenario-manifest.json records evidenceRefs for all 1 scenario contract(s)" \
  "canonical evidenceRefs array is recognized"

# --- BUG-045: evidenceRefs requires positive array cardinality ---------------
# These assertions are mutation-sensitive: restoring the former type-only jq
# selector makes both malformed fixtures exit 0 and emit the all-covered line,
# while the canonical one-member control above remains green.
empty_evidence_feature="$TMPDIR/specs/101-empty-evidence-refs"
build_clean_feature "$empty_evidence_feature"
bubbles_sed_inplace \
  's/"evidenceRefs": \["report.md#test-evidence"\]/"evidenceRefs": []/' \
  "$empty_evidence_feature/scenario-manifest.json"
run_trace_case "$empty_evidence_feature" "BUG-045 empty evidenceRefs array"
assert_case_status 1 "BUG-045 empty array: guard exits 1"
assert_case_contains "scenario-manifest.json records evidenceRefs for only 0 of 1 scenario contract(s)" \
  "BUG-045 empty array: exact uncovered count is reported"
assert_case_not_contains "scenario-manifest.json records evidenceRefs for all 1 scenario contract(s)" \
  "BUG-045 empty array: complete coverage is not reported"

mixed_evidence_feature="$TMPDIR/specs/101-mixed-evidence-refs"
build_clean_feature "$mixed_evidence_feature"
cat > "$mixed_evidence_feature/scopes.md" <<'EOF'
# Scope 01: Mixed Evidence References

**Status:** In Progress

### Gherkin

#### SCN-B045-MIXED-001

  Scenario: Primary widget renders with provided label
    Given a primary widget label
    When the primary widget mounts
    Then the primary widget displays the label

#### SCN-B045-MIXED-002

  Scenario: Secondary widget retains its accessible name
    Given a secondary widget name
    When the secondary widget mounts
    Then the secondary widget retains its accessible name

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --------- | -------- | ------------- | ----------- | ------- | ----------- |
| E2E | e2e-ui | tests/widget-render.e2e.spec.ts | SCN-B045-MIXED-001 primary widget renders with provided label | selftest:mixed-primary | Yes |
| E2E | e2e-ui | tests/widget-render.e2e.spec.ts | SCN-B045-MIXED-002 secondary widget retains its accessible name | selftest:mixed-secondary | Yes |

### Definition of Done

- [x] SCN-B045-MIXED-001 primary widget renders with provided label -> Evidence: report.md#test-evidence
- [x] SCN-B045-MIXED-002 secondary widget retains its accessible name -> Evidence: report.md#test-evidence
EOF
cat > "$mixed_evidence_feature/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-B045-MIXED-001",
      "title": "Primary widget renders with provided label",
      "requiredTestType": "e2e-ui",
      "linkedTests": ["tests/widget-render.e2e.spec.ts"],
      "evidenceRefs": ["report.md#test-evidence"]
    },
    {
      "id": "SCN-B045-MIXED-002",
      "title": "Secondary widget retains its accessible name",
      "requiredTestType": "e2e-ui",
      "linkedTests": ["tests/widget-render.e2e.spec.ts"],
      "evidenceRefs": []
    }
  ]
}
EOF
run_trace_case "$mixed_evidence_feature" "BUG-045 mixed evidenceRefs arrays"
assert_case_status 1 "BUG-045 mixed arrays: guard exits 1"
assert_case_contains "scenario-manifest.json records evidenceRefs for only 1 of 2 scenario contract(s)" \
  "BUG-045 mixed arrays: exact covered count is reported"
assert_case_not_contains "scenario-manifest.json records evidenceRefs for all 2 scenario contract(s)" \
  "BUG-045 mixed arrays: complete coverage is not reported"

# --- Case 1bb: equivalent id aliases still describe one scenario object -----
dual_id_feature="$TMPDIR/specs/101b-dual-id-manifest"
build_clean_feature "$dual_id_feature"
cat > "$dual_id_feature/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-DUAL-001",
      "scenarioId": "SCN-DUAL-001",
      "title": "Widget renders with provided label",
      "requiredTestType": "e2e-ui",
      "linkedTests": ["tests/widget-render.e2e.spec.ts"],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_trace_case "$dual_id_feature" "equivalent canonical and legacy ids"
assert_case_status 0 "equivalent id aliases exit zero"
assert_case_contains "scenario-manifest.json covers 1 scenario contract(s)" \
  "equivalent id aliases count one scenario object"
assert_case_contains "All 1 authored linked test reference(s) from scenario-manifest.json exist" \
  "equivalent id aliases resolve one authored reference"
assert_case_not_contains "covers 2 scenario contract(s)" \
  "equivalent id aliases never double-count the scenario object"

# --- CONTRACT-PATCH-001/STAB-004: identified exact + legacy cardinality ----
stable_id_feature="$TMPDIR/specs/101c-stable-id-manifest"
build_clean_feature "$stable_id_feature"
bubbles_sed_inplace \
  '/### Gherkin/a\
\
#### SCN-WIDGET-01' \
  "$stable_id_feature/scopes.md"

substituted_id_feature="$TMPDIR/specs/101d-substituted-id-manifest"
cp -R "$stable_id_feature" "$substituted_id_feature"
bubbles_sed_inplace 's/SCN-WIDGET-01/SCN-SUBSTITUTED-01/' \
  "$substituted_id_feature/scenario-manifest.json"
run_trace_case "$substituted_id_feature" "equal counts with a substituted stable scenario id"
assert_case_status 1 "CR-01 substituted stable id exits nonzero despite equal counts"
assert_case_contains "identified-subset exact matching failed for known stable scenario id: SCN-WIDGET-01 expected=1 actual=0" \
  "CR-01 substituted stable id is diagnosed as identified-subset exact-match drift"

surplus_scenario_feature="$TMPDIR/specs/101e-surplus-scenario-manifest"
cp -R "$stable_id_feature" "$surplus_scenario_feature"
python3 - "$surplus_scenario_feature/scenario-manifest.json" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    document = json.load(handle)
extra = dict(document["scenarios"][0])
extra["scenarioId"] = "SCN-SURPLUS-01"
document["scenarios"].append(extra)
with open(path, "w", encoding="utf-8") as handle:
    json.dump(document, handle, indent=2)
    handle.write("\n")
PY
run_trace_case "$surplus_scenario_feature" "surplus manifest scenario record"
assert_case_status 1 "CR-01 surplus manifest record exits nonzero"
assert_case_contains "legacy residual cardinality differs after identified-subset exact matching: expected=0 actual=1" \
  "CR-01 all-identified surplus is diagnosed as a nonzero legacy manifest residual"

mixed_stable_legacy_feature="$TMPDIR/specs/101f-mixed-stable-legacy"
cp -R "$stable_id_feature" "$mixed_stable_legacy_feature"
cat >> "$mixed_stable_legacy_feature/scopes.md" <<'EOF'

### Legacy scenario without stable ID

Scenario: Widget renders with provided label
Given a legacy heading without an identifier
When the widget mounts
Then the rendered output displays the provided label
EOF
python3 - "$mixed_stable_legacy_feature/scenario-manifest.json" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    document = json.load(handle)
extra = dict(document["scenarios"][0])
extra["scenarioId"] = "SCN-LEGACY-RESIDUAL-01"
document["scenarios"].append(extra)
with open(path, "w", encoding="utf-8") as handle:
    json.dump(document, handle, indent=2)
    handle.write("\n")
PY
run_trace_case "$mixed_stable_legacy_feature" "positive mixed stable and legacy residual"
assert_case_status 0 "mixed stable plus legacy residual exits zero"
assert_case_contains "scenario-manifest.json covers 2 scenario contract(s)" \
  "mixed stable plus legacy residual preserves total cardinality without inferred legacy identity"

undercount_feature="$TMPDIR/specs/101g-legacy-undercount"
cp -R "$mixed_stable_legacy_feature" "$undercount_feature"
python3 - "$undercount_feature/scenario-manifest.json" <<'PY'
import json
import sys
path = sys.argv[1]
with open(path, encoding="utf-8") as handle: document = json.load(handle)
document["scenarios"].pop()
with open(path, "w", encoding="utf-8") as handle:
    json.dump(document, handle, indent=2); handle.write("\n")
PY
run_trace_case "$undercount_feature" "legacy residual undercount"
assert_case_status 1 "legacy residual undercount exits nonzero"
assert_case_contains "legacy residual cardinality differs after identified-subset exact matching: expected=1 actual=0" \
  "legacy residual undercount is explicit"

duplicate_scope_feature="$TMPDIR/specs/101h-duplicate-scope-known-id"
cp -R "$stable_id_feature" "$duplicate_scope_feature"
cat >> "$duplicate_scope_feature/scopes.md" <<'EOF'

#### SCN-WIDGET-01 duplicate scope identity

Scenario: Duplicate known identity is rejected
Given two scope scenarios claim one stable identifier
When identified-subset multiplicity is reconciled
Then the duplicate scope identity fails
EOF
run_trace_case "$duplicate_scope_feature" "duplicate scope known ID"
assert_case_status 1 "duplicate scope known ID exits nonzero"
assert_case_contains "identified-subset exact matching failed: duplicate known stable scenario id in resolved scope scenarios: SCN-WIDGET-01" \
  "duplicate scope known ID is explicit"

duplicate_manifest_feature="$TMPDIR/specs/101i-duplicate-manifest-known-id"
cp -R "$stable_id_feature" "$duplicate_manifest_feature"
python3 - "$duplicate_manifest_feature/scenario-manifest.json" <<'PY'
import json
import sys
path = sys.argv[1]
with open(path, encoding="utf-8") as handle: document = json.load(handle)
document["scenarios"].append(dict(document["scenarios"][0]))
with open(path, "w", encoding="utf-8") as handle:
    json.dump(document, handle, indent=2); handle.write("\n")
PY
run_trace_case "$duplicate_manifest_feature" "duplicate manifest known ID"
assert_case_status 1 "duplicate manifest known ID exits nonzero"
assert_case_contains "duplicate effective scenario id" \
  "duplicate manifest known ID is rejected by canonical projection"

all_unidentified_feature="$TMPDIR/specs/101j-all-unidentified-equal"
build_clean_feature "$all_unidentified_feature"
run_trace_case "$all_unidentified_feature" "all-unidentified equal count"
assert_case_status 0 "all-unidentified equal count exits zero"
assert_case_contains "scenario-manifest.json covers 1 scenario contract(s)" \
  "all-unidentified equal count passes without inferred identity"

# --- Case 1c: canonical string linkedTests missing path remains blocking ------

canonical_missing_feature="$TMPDIR/specs/102-canonical-missing-test"
build_clean_feature "$canonical_missing_feature"
cat > "$canonical_missing_feature/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-CANON-002",
      "title": "Widget renders with provided label",
      "requiredTestType": "e2e-ui",
      "linkedTests": ["tests/missing-widget.e2e.spec.ts"],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_trace_case "$canonical_missing_feature" "canonical manifest missing linked test"
if [[ "$CASE_STATUS" -ne 0 ]]; then
  pass "canonical missing linked test exits nonzero"
else
  fail "canonical missing linked test should fail"
fi
assert_case_contains "scenario SCN-CANON-002 linkedTests[0]: authored path is not an existing stable regular file" \
  "canonical missing string linkedTests path is named"
assert_case_not_contains "All 1 authored linked test reference(s) from scenario-manifest.json exist" \
  "missing authored link never emits aggregate success"

# --- H022-006: planned tests are not reported as authored successes ---------
planned_only_feature="$TMPDIR/specs/103-planned-only"
build_clean_feature "$planned_only_feature"
cat > "$planned_only_feature/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-PLANNED-001",
      "title": "Widget renders with provided label",
      "requiredTestType": "e2e-ui",
      "linkedTests": [],
      "plannedTests": [
        {
          "path": "tests/widget-render.future.e2e.spec.ts",
          "title": "Widget renders in a future scope",
          "type": "e2e-ui"
        }
      ],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_trace_case "$planned_only_feature" "planned-only manifest links remain unauthored"
assert_case_status 0 "Planned-only links retain planning-time exit semantics"
assert_case_contains "Authored linked-test resolution NOT_APPLICABLE: 0 authored reference(s); 1 planned test reference(s) remain unauthored" \
  "Planned-only links are explicitly classified as unauthored"
assert_case_not_contains "All linked tests from scenario-manifest.json exist" \
  "Planned-only links never emit the legacy universal success"
assert_case_not_contains "authored linked test reference(s) from scenario-manifest.json exist" \
  "Planned-only links never emit authored-link success"

legacy_planned_feature="$TMPDIR/specs/103-legacy-planned"
build_clean_feature "$legacy_planned_feature"
cat > "$legacy_planned_feature/scenario-manifest.json" <<'EOF'
{"schemaVersion":1,"scenarios":[{"id":"SCN-PLANNED-002","title":"Legacy planning metadata remains compatible","requiredTestType":"e2e-ui","linkedTests":[{"file":"tests/legacy-future.spec.ts","testState":"planned-not-authored"}],"evidenceRefs":["report.md#test-evidence"]}]}
EOF
run_trace_case "$legacy_planned_feature" "legacy testState remains planned"
assert_case_status 0 "Legacy testState planned reference retains planning-time exit semantics"
assert_case_contains "0 authored reference(s); 1 planned test reference(s) remain unauthored" \
  "Legacy testState is counted as planned"
assert_case_not_contains "references missing linked test file: tests/legacy-future.spec.ts" \
  "Legacy testState is never resolved as authored"

mixed_reference_feature="$TMPDIR/specs/103-mixed-references"
build_clean_feature "$mixed_reference_feature"
cat > "$mixed_reference_feature/scenario-manifest.json" <<'EOF'
{"schemaVersion":1,"scenarios":[{"id":"SCN-PLANNED-003","title":"Mixed test references retain separate states","requiredTestType":"e2e-ui","linkedTests":["tests/widget-render.e2e.spec.ts"],"plannedTests":[{"path":"tests/mixed-future.spec.ts","title":"Future mixed behavior","type":"e2e-ui"}],"evidenceRefs":["report.md#test-evidence"]}]}
EOF
run_trace_case "$mixed_reference_feature" "mixed authored and planned references"
assert_case_status 0 "Mixed authored/planned references exit zero when authored files resolve"
assert_case_contains "scenario-manifest.json linked test exists: tests/widget-render.e2e.spec.ts" \
  "Mixed references resolve the authored file"
assert_case_not_contains "references missing linked test file: tests/mixed-future.spec.ts" \
  "Mixed references do not require the planned file"

planned_traversal_feature="$TMPDIR/specs/103-planned-traversal"
build_clean_feature "$planned_traversal_feature"
cat > "$planned_traversal_feature/scenario-manifest.json" <<'EOF'
{"schemaVersion":1,"scenarios":[{"id":"SCN-PLANNED-004","title":"Unsafe planned path","requiredTestType":"e2e-ui","linkedTests":[],"plannedTests":[{"path":"tests/../future.spec.ts","title":"Unsafe future","type":"e2e-ui"}],"evidenceRefs":["report.md#test-evidence"]}]}
EOF
run_trace_case "$planned_traversal_feature" "planned traversal is refused"
assert_case_status 1 "Planned lexical traversal exits nonzero"
assert_case_contains "scenario SCN-PLANNED-004 plannedTests[0]: path contains lexical traversal" \
  "Planned lexical traversal is diagnosed"

authored_traversal_feature="$TMPDIR/specs/103-authored-traversal"
build_clean_feature "$authored_traversal_feature"
cat > "$authored_traversal_feature/scenario-manifest.json" <<'EOF'
{"schemaVersion":1,"scenarios":[{"id":"SCN-AUTHORED-004","title":"Unsafe authored path","requiredTestType":"e2e-ui","linkedTests":["tests/../tests/widget-render.e2e.spec.ts"],"evidenceRefs":["report.md#test-evidence"]}]}
EOF
run_trace_case "$authored_traversal_feature" "authored traversal is refused"
assert_case_status 1 "Authored lexical traversal exits nonzero"
assert_case_contains "scenario SCN-AUTHORED-004 linkedTests[0]: path contains lexical traversal" \
  "Authored lexical traversal is diagnosed"

symlink_escape_feature="$TMPDIR/specs/103-symlink-escape"
build_clean_feature "$symlink_escape_feature"
printf 'outside repository\n' > "$TMPDIR/outside-widget.spec.ts"
ln -s "$TMPDIR/outside-widget.spec.ts" "$symlink_escape_feature/tests/escape.spec.ts"
cat > "$symlink_escape_feature/scenario-manifest.json" <<'EOF'
{"schemaVersion":1,"scenarios":[{"id":"SCN-AUTHORED-005","title":"Escaping symlink","requiredTestType":"e2e-ui","linkedTests":["tests/escape.spec.ts"],"evidenceRefs":["report.md#test-evidence"]}]}
EOF
run_trace_case "$symlink_escape_feature" "authored symlink escape is refused"
assert_case_status 1 "Authored symlink escape exits nonzero"
assert_case_contains "scenario SCN-AUTHORED-005 linkedTests[0]: authored path is not an existing stable regular file" \
  "Escaping authored symlink is refused as unstable"

internal_symlink_feature="$TMPDIR/specs/103-internal-symlink"
build_clean_feature "$internal_symlink_feature"
ln -s widget-render.e2e.spec.ts "$internal_symlink_feature/tests/internal-link.spec.ts"
cat > "$internal_symlink_feature/scenario-manifest.json" <<'EOF'
{"schemaVersion":1,"scenarios":[{"id":"SCN-AUTHORED-006","title":"Contained symlink","requiredTestType":"e2e-ui","linkedTests":["tests/internal-link.spec.ts"],"evidenceRefs":["report.md#test-evidence"]}]}
EOF
run_trace_case "$internal_symlink_feature" "contained authored symlink is refused"
assert_case_status 1 "Contained authored symlink exits nonzero"
assert_case_contains "scenario SCN-AUTHORED-006 linkedTests[0]: authored path is not an existing stable regular file" \
  "Contained authored symlink is refused as unstable"

# --- H022-006: Command column paths cannot impersonate test files -----------
command_path_feature="$TMPDIR/specs/104-command-path-is-not-test"
build_clean_feature "$command_path_feature"
cat > "$command_path_feature/ozhiva.sh" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$command_path_feature/ozhiva.sh"
cat > "$command_path_feature/scopes.md" <<'EOF'
# Scope 01: Command Path Is Not A Test

**Status:** In Progress

### Gherkin

  Scenario: Widget renders with provided label
    Given a label "Hello"
    When the widget mounts
    Then the rendered output displays "Hello"

### Test Plan

| Test Type | Category | Description | Command | Live System |
| --- | --- | --- | --- | --- |
| E2E | e2e-ui | Widget renders with provided label and displays it | ./ozhiva.sh test | Yes |

### Definition of Done

- [x] Widget renders with provided label and displays the rendered output -> Evidence: report.md#test-evidence
EOF
run_trace_case "$command_path_feature" "missing File/Location cannot select an existing CLI command"
assert_case_status 1 "Missing File/Location remains a concrete-path failure"
assert_case_contains "Test Plan extraction failed" \
  "Missing File/Location fails closed during Test Plan extraction"
assert_case_not_contains "scenario maps to concrete test file: ./ozhiva.sh" \
  "Existing CLI path in Command is never selected as a test"

# --- H022-006: required Test Plan headers are order-independent and unique --
reordered_header_feature="$TMPDIR/specs/105-reordered-headers"
build_clean_feature "$reordered_header_feature"
cat > "$reordered_header_feature/scopes.md" <<'EOF'
# Scope 01: Reordered Headers

**Status:** In Progress

### Gherkin

  Scenario: Widget renders with provided label

### Test Plan

| Description | File / Location | Live System | Test Type | Command |
| --- | --- | --- | --- | --- |
| Widget renders with provided label and displays it | tests/widget-render.e2e.spec.ts | Yes | E2E | selftest:widget-render |

### Definition of Done

- [x] Widget renders with provided label and displays the rendered output -> Evidence: report.md#test-evidence
EOF
run_trace_case "$reordered_header_feature" "reordered required headers"
assert_case_status 0 "Reordered Test Plan headers exit zero"
assert_case_contains "scenario maps to concrete test file: tests/widget-render.e2e.spec.ts" \
  "Reordered File/Location column supplies the concrete path"

# --- H022-006: coverage policy is per-scenario and option-order independent -
planned_policy_feature="$TMPDIR/specs/105-planned-policy"
build_clean_feature "$planned_policy_feature"
cat > "$planned_policy_feature/scenario-manifest.json" <<'EOF'
{"schemaVersion":1,"scenarios":[{"scenarioId":"SCN-PLANNED-01","title":"Widget renders with provided label","plannedTests":[{"path":"tests/future-widget.spec.ts","title":"future widget behavior","type":"e2e-ui"}],"evidenceRefs":["report.md#test-evidence"]}]}
EOF
run_trace_case "$planned_policy_feature" "planning policy accepts canonical planned coverage" --coverage-policy=planning --all-scopes
assert_case_status 0 "Planning policy accepts one canonical planned reference"
run_trace_case "$planned_policy_feature" "authored policy refuses planned-only coverage" --all-scopes --coverage-policy=authored
assert_case_status 1 "Authored policy refuses a scenario with only planned coverage"
assert_case_contains "lacks authored coverage required by --coverage-policy=authored: SCN-PLANNED-01" \
  "Authored policy identifies the uncovered scenario"

unclassified_policy_feature="$TMPDIR/specs/105-unclassified-policy"
build_clean_feature "$unclassified_policy_feature"
cat > "$unclassified_policy_feature/scenario-manifest.json" <<'EOF'
{"schemaVersion":1,"scenarios":[{"scenarioId":"SCN-UNCLASSIFIED-01","title":"Widget renders with provided label","linkedTests":[],"plannedTests":[],"evidenceRefs":["report.md#test-evidence"]}]}
EOF
run_trace_case "$unclassified_policy_feature" "planning policy refuses unclassified scenario" --coverage-policy=planning
assert_case_status 1 "Planning policy refuses a scenario with no authored or planned reference"
assert_case_contains "lacks authored or planned coverage required by --coverage-policy=planning: SCN-UNCLASSIFIED-01" \
  "Planning policy identifies the uncovered scenario"

run_trace_case "$reordered_header_feature" "coverage and scope options reversed" --coverage-policy=authored --all-scopes
assert_case_status 0 "Coverage policy may precede the scope selector"
run_trace_case "$reordered_header_feature" "scope and coverage options canonical order" --all-scopes --coverage-policy=authored
assert_case_status 0 "Scope selector may precede the coverage policy"
run_trace_case "$reordered_header_feature" "invalid coverage policy" --coverage-policy=unknown
assert_case_status 2 "Unknown coverage policy is a usage refusal"
run_trace_case "$reordered_header_feature" "valueless coverage policy" --coverage-policy
assert_case_status 2 "Valueless coverage policy is a usage refusal"
run_trace_case "$reordered_header_feature" "duplicate coverage policy" --coverage-policy=planning --coverage-policy=authored
assert_case_status 2 "Duplicate coverage policy is a usage refusal"
run_trace_case "$reordered_header_feature" "conflicting scope selectors" --all-scopes --current-scope
assert_case_status 2 "Multiple scope selectors are a usage refusal"
run_trace_case "$reordered_header_feature" "duplicate scope selector" --all-scopes --all-scopes
assert_case_status 2 "Repeated scope selector is a usage refusal"

# --- H022-006: File/Location and Command cannot impersonate semantics -------
semantic_impersonation_feature="$TMPDIR/specs/105-semantic-impersonation"
build_clean_feature "$semantic_impersonation_feature"
mv "$semantic_impersonation_feature/tests/widget-render.e2e.spec.ts" \
  "$semantic_impersonation_feature/tests/SCN-SEMANTIC-01.spec.ts"
cat > "$semantic_impersonation_feature/scopes.md" <<'EOF'
# Scope 01: Semantic Isolation

**Status:** In Progress

### SCN-SEMANTIC-01
  Scenario: Customer billing address persists after submission

### Test Plan

| Test Type | File/Location | Description | Command |
| --- | --- | --- | --- |
| E2E | tests/SCN-SEMANTIC-01.spec.ts | Widget smoke check | ./runner customer billing address persists after submission |

### Definition of Done

- [x] SCN-SEMANTIC-01 Customer billing address persists after submission -> Evidence: report.md#test-evidence
EOF
cat > "$semantic_impersonation_feature/scenario-manifest.json" <<'EOF'
{"schemaVersion":1,"scenarios":[{"scenarioId":"SCN-SEMANTIC-01","title":"Customer billing address persists after submission","linkedTests":["tests/SCN-SEMANTIC-01.spec.ts"],"evidenceRefs":["report.md#test-evidence"]}]}
EOF
run_trace_case "$semantic_impersonation_feature" "path and command semantic impersonation"
assert_case_status 1 "Path and Command cells cannot establish a scenario-to-row match"
assert_case_contains "scenario has no traceable Test Plan row" \
  "Semantic isolation reports the unmapped scenario"

duplicate_header_feature="$TMPDIR/specs/106-duplicate-headers"
build_clean_feature "$duplicate_header_feature"
cat > "$duplicate_header_feature/scopes.md" <<'EOF'
# Scope 01: Duplicate Headers

**Status:** In Progress

### Gherkin

  Scenario: Widget renders with provided label

### Test Plan

| Test Type | File/Location | Test Type | Description |
| --- | --- | --- | --- |
| E2E | tests/widget-render.e2e.spec.ts | e2e-ui | Widget renders with provided label |

### Definition of Done

- [x] Widget renders with provided label -> Evidence: report.md#test-evidence
EOF
run_trace_case "$duplicate_header_feature" "duplicate required Test Plan header"
assert_case_status 1 "Duplicate Test Type header exits nonzero"
assert_case_contains "Test Plan extraction failed" \
  "Duplicate required header is an extraction failure"

unrelated_table_feature="$TMPDIR/specs/107-unrelated-table"
build_clean_feature "$unrelated_table_feature"
cat > "$unrelated_table_feature/scopes.md" <<'EOF'
# Scope 01: Unrelated Table

**Status:** In Progress

### Gherkin

  Scenario: Widget renders with provided label

### Test Plan

| Owner | Review State |
| --- | --- |
| test-team | ready |

| Description | File/Location | Test Type |
| --- | --- | --- |
| Widget renders with provided label and displays it | tests/widget-render.e2e.spec.ts | E2E |

### Definition of Done

- [x] Widget renders with provided label and displays the rendered output -> Evidence: report.md#test-evidence
EOF
run_trace_case "$unrelated_table_feature" "unrelated table before Test Plan table"
assert_case_status 0 "Unrelated table is ignored and the valid Test Plan table exits zero"
assert_case_contains "summary: scenarios=1 test_rows=1" \
  "Unrelated table contributes no test rows"

# --- BUG-039: structural rows never inflate canonical ID/Type tables -------
bug039_feature="$REPO_FIXTURE_ROOT/specs/108-bug039-feature028-shape"
build_bug039_feature028_shape "$bug039_feature"
run_trace_case "$bug039_feature" "BUG-039 four canonical ID/Type Test Plan tables" --all-scopes
assert_case_status 0 "BUG-039 canonical ID/Type tables remain fully traceable"
assert_case_occurrences 4 "summary: scenarios=1 test_rows=6" \
  "BUG-039 each six-row table excludes its header and separator"
assert_case_contains "Test rows checked: 24" \
  "BUG-039 four Test Plan tables aggregate exactly 24 genuine rows"
assert_case_not_contains "Test Plan extraction failed" \
  "BUG-039 canonical table extraction has no hidden parser failure"

bug039_malformed_feature="$REPO_FIXTURE_ROOT/specs/109-bug039-malformed-row"
cp -R "$bug039_feature" "$bug039_malformed_feature"
python3 - "$bug039_malformed_feature/scopes.md" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
text = path.read_text(encoding="utf-8")
needle = "| TP-028-24 | unit | SCN-028-406 | `tests/traceability.spec.ts` — Auxiliary behavior 24 | selftest:traceability | Covers one auxiliary path |"
replacement = "| TP-028-24 | unit |"
if needle not in text:
    raise SystemExit("BUG-039 selftest fixture row not found")
path.write_text(text.replace(needle, replacement, 1), encoding="utf-8")
PY
run_trace_case "$bug039_malformed_feature" "BUG-039 malformed canonical data row" --all-scopes
assert_case_status 1 "BUG-039 malformed canonical row fails closed"
assert_case_contains "malformed Test Plan row" \
  "BUG-039 malformed row emits an explicit parser diagnostic"
assert_case_contains "Test Plan extraction failed" \
  "BUG-039 malformed row propagates extraction failure to the guard"

make_bug039_variant() {
  local destination="$1"
  local variant="$2"
  cp -R "$bug039_feature" "$destination"
  python3 - "$destination/scopes.md" "$variant" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
variant = sys.argv[2]
text = path.read_text(encoding="utf-8")
header = "| ID | Type | Scenario | Persistent file and exact title | Command | Required behavior |"
separator = "| --- | --- | --- | --- | --- | --- |"
first_row = "| TP-028-01 | regression | SCN-028-A-01 | `tests/traceability.spec.ts` — Scope 1 primary behavior | selftest:traceability | Maps the primary behavior |"
sixth_row = "| TP-028-06 | unit | SCN-028-106 | `tests/traceability.spec.ts` — Auxiliary behavior 6 | selftest:traceability | Covers one auxiliary path |"

if variant == "rowless-legacy":
  legacy_header = "| Test Type | Scenario | File/Location | Command | Required behavior | Notes |"
  text = text.replace(header, legacy_header, 1)
  header = legacy_header

if variant in ("rowless", "rowless-legacy", "rowless-prose", "rowless-deeper-heading", "rowless-section-boundary"):
  table_start = text.index(header)
  data_start = text.index("\n", text.index(separator, table_start)) + 1
  data_end = text.index("\n\n", data_start)
  replacement = {
      "rowless": text[data_end:],
      "rowless-legacy": text[data_end:],
      "rowless-prose": "Boundary prose" + text[data_end:],
      "rowless-deeper-heading": "#### Nested boundary" + text[data_end:],
      "rowless-section-boundary": "### Later Section" + text[data_end:],
  }[variant]
  path.write_text(text[:data_start] + replacement, encoding="utf-8")
  raise SystemExit(0)

if variant == "rowless-eof":
  table_start = text.index(header)
  separator_end = text.index("\n", text.index(separator, table_start)) + 1
  path.write_text(text[:separator_end], encoding="utf-8")
  raise SystemExit(0)

replacements = {
    "missing-separator": (separator + "\n", "", 1),
    "empty-separator-cell": (separator, "| --- | --- | --- |  | --- | --- |", 1),
    "invalid-separator-cell": (separator, "| --- | --- | -- | --- | --- | --- |", 1),
    "narrow-separator": (separator, "| --- | --- | --- | --- | --- |", 1),
    "wide-separator": (separator, "| --- | --- | --- | --- | --- | --- | --- |", 1),
    "delayed-separator": (header + "\n" + separator, header + "\nprose delay\n" + separator, 1),
    "unsupported-path": ("Persistent file and exact title", "Test Path", 1),
    "duplicate-path": (header, "| ID | Type | Scenario | File / Surface | File/Location | Required behavior |", 1),
    "duplicate-canonical-header": (header, "| ID | ID | Type | Persistent file and exact title | Command | Required behavior |", 1),
    "duplicate-legacy-header": (header, "| Test Type | Test Type | Scenario | File/Location | Command | Required behavior |", 1),
    "partial-header": (header, "| ID | Category | Scenario | Persistent file and exact title | Command | Required behavior |", 1),
    "mixed-family": (header, "| ID | Type | Test Type | Persistent file and exact title | Command | Required behavior |", 1),
    "empty-required-cell": (first_row, "|  | regression | SCN-028-A-01 | `tests/traceability.spec.ts` — Scope 1 primary behavior | selftest:traceability | Maps the primary behavior |", 1),
    "narrow-data-row": (first_row, "| TP-028-01 | regression |", 1),
    "wide-data-row": (first_row, first_row[:-1] + "| Extra cell |", 1),
    "second-table-done": (
        sixth_row,
        sixth_row + "\n\nBoundary prose\n\n| Test Type | File/Location | Description |\n| --- | --- | --- |\n| E2E | tests/second.spec.ts | Must be refused |",
        1,
    ),
    "second-table-read-rows": (
      sixth_row,
      sixth_row + "\n| Test Type | File/Location | Description |\n| --- | --- | --- |\n| E2E | tests/second.spec.ts | Must be refused |",
      1,
    ),
}

if variant not in replacements:
    raise SystemExit(f"unknown BUG-039 fixture variant: {variant}")
needle, replacement, count = replacements[variant]
if needle not in text:
    raise SystemExit(f"BUG-039 fixture needle not found for {variant}")
path.write_text(text.replace(needle, replacement, count), encoding="utf-8")
PY
}

for bug039_variant in \
  missing-separator empty-separator-cell invalid-separator-cell delayed-separator \
  narrow-separator wide-separator unsupported-path duplicate-path \
  duplicate-canonical-header duplicate-legacy-header partial-header mixed-family \
  empty-required-cell narrow-data-row wide-data-row rowless rowless-legacy \
  rowless-prose rowless-deeper-heading \
  rowless-section-boundary rowless-eof second-table-read-rows second-table-done; do
  bug039_variant_feature="$REPO_FIXTURE_ROOT/specs/109-bug039-${bug039_variant}"
  make_bug039_variant "$bug039_variant_feature" "$bug039_variant"
  run_trace_case_with_parser_status "$bug039_variant_feature" \
    "BUG-039 ${bug039_variant}" --all-scopes
  assert_case_status 1 "BUG-039 ${bug039_variant} fails closed"
  assert_case_contains "SELFTEST_PARSER_STATUS=4" \
    "BUG-039 ${bug039_variant} returns parser status 4"
done

assert_bug039_diagnostic() {
  local variant="$1"
  local expected="$2"

  run_trace_case_with_parser_status "$REPO_FIXTURE_ROOT/specs/109-bug039-${variant}" \
    "BUG-039 ${variant} exact diagnostic" --all-scopes
  assert_case_contains "$expected" "BUG-039 ${variant} emits its exact parser diagnostic"
  assert_case_contains "Test Plan extraction failed" \
    "BUG-039 ${variant} remains distinct from its outer extraction failure"
  assert_case_not_contains "has no concrete Test Plan rows to trace" \
    "BUG-039 ${variant} never falls through to the absent-header row message"
}

assert_bug039_diagnostic "missing-separator" \
  "ERROR: invalid or empty Test Plan separator cell at visible line 11"
assert_bug039_diagnostic "empty-separator-cell" \
  "ERROR: invalid or empty Test Plan separator cell at visible line 11"
assert_bug039_diagnostic "invalid-separator-cell" \
  "ERROR: invalid or empty Test Plan separator cell at visible line 11"
assert_bug039_diagnostic "delayed-separator" \
  "ERROR: missing or delayed Test Plan separator at visible line 11"
assert_bug039_diagnostic "narrow-separator" \
  "ERROR: wrong-width Test Plan separator: expected 6 cells, got 5 at visible line 11"
assert_bug039_diagnostic "wide-separator" \
  "ERROR: wrong-width Test Plan separator: expected 6 cells, got 7 at visible line 11"
for bug039_header_variant in \
  unsupported-path duplicate-path duplicate-canonical-header duplicate-legacy-header \
  partial-header mixed-family; do
  assert_bug039_diagnostic "$bug039_header_variant" \
    "ERROR: malformed Test Plan header at visible line 10"
done
assert_bug039_diagnostic "empty-required-cell" \
  "ERROR: required Test Plan cell is empty at visible line 12"
assert_bug039_diagnostic "narrow-data-row" \
  "ERROR: malformed Test Plan row: expected 6 cells, got 2 at visible line 12"
assert_bug039_diagnostic "wide-data-row" \
  "ERROR: malformed Test Plan row: expected 6 cells, got 7 at visible line 12"
for bug039_rowless_variant in \
  rowless rowless-legacy rowless-prose rowless-deeper-heading rowless-section-boundary rowless-eof; do
  assert_bug039_diagnostic "$bug039_rowless_variant" \
    "ERROR: rowless recognized Test Plan table at visible line 12"
done
assert_bug039_diagnostic "second-table-read-rows" \
  "ERROR: second Markdown table in Test Plan section at visible line 18"
assert_bug039_diagnostic "second-table-done" \
  "ERROR: second Markdown table in Test Plan section at visible line 21"

run_trace_case_with_parser_status "$REPO_FIXTURE_ROOT/specs/109-bug039-rowless" \
  "BUG-039 rowless table caller diagnostic" --all-scopes
assert_case_contains "SELFTEST_PARSER_STATUS=4" \
  "BUG-039 rowless recognized table returns parser status 4"
assert_case_contains "ERROR: rowless recognized Test Plan table at visible line 12" \
  "BUG-039 rowless recognized table reports its exact boundary line"
assert_case_not_contains "has no concrete Test Plan rows to trace" \
  "BUG-039 rowless recognized table is distinct from an absent supported header"

bug039_surface_alias_feature="$REPO_FIXTURE_ROOT/specs/109-bug039-file-surface-alias"
cp -R "$bug039_feature" "$bug039_surface_alias_feature"
bubbles_sed_inplace '0,/Persistent file and exact title/s//File \/ Surface/' \
  "$bug039_surface_alias_feature/scopes.md"
run_trace_case "$bug039_surface_alias_feature" \
  "BUG-039 canonical File / Surface compatibility" --all-scopes
assert_case_status 0 "BUG-039 canonical File / Surface alias remains accepted"
assert_case_contains "summary: scenarios=1 test_rows=6" \
  "BUG-039 File / Surface table emits its six genuine rows"

bug039_status4_occurrences=0
if bug039_status4_occurrences="$(grep -Fc 'raise SystemExit(4)' "$GUARD")"; then
  :
fi
if [[ "$bug039_status4_occurrences" -eq 1 ]]; then
  pass "BUG-039 structural parser has one status-4 exit in the diagnostic helper"
else
  fail "BUG-039 structural parser must have exactly one status-4 exit in the diagnostic helper (got $bug039_status4_occurrences)"
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
  "schemaVersion": 1,
  "scenarios": [
    {
      "scenarioId": "SCN-DETACHED-01",
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
(cd "$broken_feature" && bash "$GUARD" ".") >"$log2" 2>&1
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

  Scenario: SCN-07-001 user sees confirmation
    Given a submitted form
    When the server responds
    Then the user sees a confirmation message

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --------- | -------- | ------------- | ----------- | ------- | ----------- |
| E2E       | e2e-ui   | tests/widget-render.e2e.spec.ts | SCN-07-001 user sees confirmation message | selftest:declared | Yes |

### Definition of Done

- [x] SCN-07-001 user sees confirmation message -> Evidence: report.md#test-evidence
EOF
python3 - "$declared_feature/scenario-manifest.json" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
  manifest = json.load(handle)
manifest["scenarios"][0]["scenarioId"] = "SCN-07-001"
with open(path, "w", encoding="utf-8") as handle:
  json.dump(manifest, handle, indent=2)
  handle.write("\n")
PY

echo "[selftest traceability-guard] Case 3: shared trace id → declared edge"
log3="$TMPDIR/log3.txt"
set +e
(cd "$declared_feature" && bash "$GUARD" ".") >"$log3" 2>&1
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

# --- Case 4: scenario fuzzy-matches two rows → blocking ambiguity ---
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
(cd "$ambiguous_feature" && bash "$GUARD" ".") >"$log4" 2>&1
status4=$?
set -e
if [[ "$status4" -eq 1 ]]; then
  pass "ambiguous-edge feature exits 1 (got $status4)"
else
  fail "ambiguous-edge feature should exit 1 (got $status4)"
  sed -n '1,160p' "$log4"
fi
if grep -Fq 'scenario has ambiguous inferred Test Plan bindings' "$log4"; then
  pass "Case 4 blocks ambiguous inferred Test Plan bindings"
else
  fail "expected blocking ambiguous inferred Test Plan binding finding in Case 4 log"
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

# --- CR-02: multiple visible exact sections are structurally ambiguous ------
duplicate_section_feature="$TMPDIR/specs/620-duplicate-sections"
build_clean_feature "$duplicate_section_feature" "## Test Plan"
cat >>"$duplicate_section_feature/scopes.md" <<'EOF'

### Test Plan

| Test Type | File/Location | Description |
| --- | --- | --- |
| E2E | tests/widget-render.e2e.spec.ts | Widget renders with provided label |
EOF
run_trace_case "$duplicate_section_feature" "Case 6: duplicate visible level-2/level-3 Test Plans"
assert_case_status 1 "CR-02 duplicate visible exact Test Plan sections exit 1"
assert_case_occurrences 1 'has multiple visible exact Test Plan sections; exactly one ## Test Plan or ### Test Plan is applicable' \
  "CR-02 duplicate visible exact sections report structural ambiguity once"

hidden_duplicate_feature="$TMPDIR/specs/630-hidden-duplicate-sections"
build_clean_feature "$hidden_duplicate_feature" "## Test Plan"
cat >>"$hidden_duplicate_feature/scopes.md" <<'EOF'

```markdown
### Test Plan
```

<!--
## Test Plan
-->

#### Test Plan
EOF
run_trace_case "$hidden_duplicate_feature" "Case 6: hidden and unsupported duplicate lookalikes"
assert_case_status 0 "CR-02 fenced, commented, and depth-four headings do not count as duplicates"
assert_case_not_contains 'multiple visible exact Test Plan sections' \
  "CR-02 hidden duplicate lookalikes remain inert"

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
run_trace_case_with_parser_status "$header_feature" "Case 7: header-only Test Plan"
assert_case_status 1 "Case 7 header-only section exits 1"
assert_case_contains 'SELFTEST_PARSER_STATUS=4' "Case 7 header-only section returns parser status 4"
assert_case_contains 'ERROR: rowless recognized Test Plan table at visible line 16' \
  "Case 7 header-only section reports the EOF line"
assert_case_occurrences 1 'Test Plan extraction failed' \
  "Case 7 header-only section reports outer extraction failure once"
assert_case_not_contains 'has no concrete Test Plan rows to trace' \
  "Case 7 header-only section does not use absent-supported-header diagnostics"
assert_case_contains 'RESULT: FAILED (1 failures, 0 warnings)' "Case 7 header-only section reaches final summary"

run_trace_case_parser_failure "$clean_feature" "Case 7: Test Plan extractor failure"
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

# --- G068: singular/plural tolerance in the lexical DoD matcher ---
# Regression lock for the observed false positive: scenario "JSON request
# rejected" scored 2 against DoD "JSON requests rejected with 415", below the
# >=3 floor, purely because "request" != "requests". Negative controls assert
# the tolerance did not become a general substring match.
# BUG-004: the tolerance moved into the shared scenario-match-lib.sh, so this
# sources the lib instead of slicing the function body out of the guard. The
# assertions are unchanged — they now exercise the single implementation both
# G068 call paths use.
# shellcheck source=/dev/null
. "$SCRIPT_DIR/scenario-match-lib.sh"
plural_dod="json requests rejected with 415 protobuf only middleware"

for probe in json request rejected; do
  if bubbles_scenario_word_matches_text "$probe" "$plural_dod"; then
    pass "G068 plural tolerance matches '$probe'"
  else
    fail "G068 plural tolerance failed to match '$probe'"
  fi
done

if bubbles_scenario_word_matches_text "requests" "json request rejected"; then
  pass "G068 tolerance also matches plural scenario word against singular DoD"
else
  fail "G068 tolerance missed plural-scenario/singular-DoD direction"
fi

# Inflection/derivation cases observed sinking real DoD items below the floor.
inflect_dod="post api trips group handler implemented with protobuf decode postgresql persist"
if bubbles_scenario_word_matches_text "persisted" "$inflect_dod"; then
  pass "G068 matches inflected scenario word 'persisted' against DoD 'persist'"
else
  fail "G068 failed to match 'persisted' against 'persist'"
fi

derive_dod="staleness warning amber displayed when bundled data published at 90 days"
if bubbles_scenario_word_matches_text "stale" "$derive_dod"; then
  pass "G068 matches derived DoD word 'staleness' from scenario 'stale'"
else
  fail "G068 failed to match 'stale' against 'staleness'"
fi

# The prefix rule is floored at 5 chars so short roots cannot collide.
if bubbles_scenario_word_matches_text "test" "testament of unrelated things"; then
  fail "G068 prefix rule over-matched a 4-char root ('test' vs 'testament')"
else
  pass "G068 prefix rule correctly refuses a 4-char root ('test' vs 'testament')"
fi

for probe in authentication websocket participant; do
  if bubbles_scenario_word_matches_text "$probe" "$plural_dod"; then
    fail "G068 tolerance over-matched unrelated word '$probe'"
  else
    pass "G068 tolerance correctly rejects unrelated word '$probe'"
  fi
done

# ---------------------------------------------------------------------------
# Declared trace-id mapping.
#
# A DoD item or Test Plan row that names the scenario's own SCN- id is the
# strongest available evidence of an intended mapping, but the id previously
# never reached the comparison: extract_scenarios strips everything before
# "Scenario:", while the id lives on the heading above it. The guard therefore
# reported declared=0 on every packet and failed DoD items that cited their
# scenario outright. Titles here share almost no words with their DoD item, so
# only the id can carry the match — word overlap cannot rescue these cases.
# ---------------------------------------------------------------------------
write_declared_id_scope() {
  local feature_dir="$1"
  local dod_id="$2"
  local row_id="$3"

  build_clean_feature "$feature_dir"

  cat > "$feature_dir/scopes.md" <<EOF
# Scope 01: Widget Render

**Status:** In Progress

### Gherkin

#### SCN-ALPHA-77 - governing heading carries the identifier

  Scenario: Zebra telemetry quiesces beneath a lunar eclipse
    Given an orbital sensor
    When the umbra passes
    Then the telemetry quiesces

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --------- | -------- | ------------- | ----------- | ------- | ----------- |
| E2E       | e2e-ui   | tests/widget-render.e2e.spec.ts | ${row_id} orbital regression | selftest:widget-render | Yes |

### Definition of Done

- [x] ${dod_id} evidence proves the orbital regression holds -> Evidence: report.md#test-evidence
EOF

  cat > "$feature_dir/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "scenarioId": "SCN-ALPHA-77",
      "scope": "01-widget-render",
      "title": "Zebra telemetry quiesces beneath a lunar eclipse",
      "linkedTests": [
        { "file": "tests/widget-render.e2e.spec.ts" }
      ],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
}

declared_dir="$TMPDIR/declared-id"
write_declared_id_scope "$declared_dir" "SCN-ALPHA-77" "SCN-ALPHA-77"
run_trace_case "$declared_dir" "declared trace id establishes the mapping"
assert_case_status 0 "Declared id: a DoD item citing the scenario id maps it despite near-zero word overlap"
assert_case_not_contains "no faithful DoD item preserving its behavioral claim" \
  "Declared id: the cited scenario is not reported as an unfaithful DoD item"
assert_case_not_contains "has no traceable Test Plan row" \
  "Declared id: the cited scenario is not reported as row-less"
assert_case_contains "declared" "Declared id: the match is counted as declared, not inferred"

# A path-bearing fuzzy match may appear before the exact-id row. Explicit
# identity must win before prose scoring or the first row steals the scenario.
exact_second_dir="$TMPDIR/declared-id-exact-second"
build_clean_feature "$exact_second_dir"
cat > "$exact_second_dir/tests/fuzzy-first.e2e.spec.ts" <<'EOF'
export const fuzzyFirst = true;
EOF
cat > "$exact_second_dir/scopes.md" <<'EOF'
# Scope 01: Exact Identity Before Fuzzy Prose

**Status:** In Progress

### Gherkin

  Scenario: SCN-99-001 Widget renders with provided label
    Given a label "Hello"
    When the widget mounts
    Then the rendered output displays "Hello"

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --------- | -------- | ------------- | ----------- | ------- | ----------- |
| E2E | e2e-ui | tests/fuzzy-first.e2e.spec.ts | Widget renders with provided label | selftest:fuzzy | Yes |
| E2E | e2e-ui | tests/widget-render.e2e.spec.ts | SCN-99-001 exact identity row | selftest:exact | Yes |

### Definition of Done

- [x] SCN-99-001 exact identity behavior -> Evidence: report.md#test-evidence
EOF
cat > "$exact_second_dir/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-99-001",
      "title": "SCN-99-001 Widget renders with provided label",
      "requiredTestType": "e2e-ui",
      "linkedTests": ["tests/widget-render.e2e.spec.ts"],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_trace_case "$exact_second_dir" "exact-id row follows a path-bearing fuzzy row"
assert_case_status 0 "Exact-id precedence: later exact row wins over earlier fuzzy path row"
assert_case_contains "scenario maps to concrete test file: tests/widget-render.e2e.spec.ts" \
  "Exact-id precedence: selected path belongs to the exact row"
assert_case_not_contains "report is missing evidence reference for concrete test file: tests/fuzzy-first.e2e.spec.ts" \
  "Exact-id precedence: fuzzy-first row does not steal evidence ownership"

# Adversarial twin: if any id satisfied the check, the fix would be a blanket
# pass rather than a mapping. A DoD item naming a DIFFERENT scenario must still
# fail, otherwise every scenario in a packet would match every DoD item.
mismatch_dir="$TMPDIR/declared-id-mismatch"
write_declared_id_scope "$mismatch_dir" "SCN-OMEGA-77" "SCN-ALPHA-77"
run_trace_case "$mismatch_dir" "a different trace id does not establish the mapping"
assert_case_status 1 "Declared id adversarial: a DoD item citing a DIFFERENT scenario id does not map it"
assert_case_contains "no faithful DoD item preserving its behavioral claim" \
  "Declared id adversarial: the mismatched id is still reported unmapped"

# An id-less heading must not blanket-match either, or a packet that simply omits
# identifiers would silently pass the fidelity check it is meant to fail.
idless_dir="$TMPDIR/declared-id-absent"
write_declared_id_scope "$idless_dir" "SCN-ALPHA-77" "SCN-ALPHA-77"
bubbles_sed_inplace \
  's/^#### SCN-ALPHA-77 - governing heading carries the identifier$/#### governing heading carries no identifier/' \
  "$idless_dir/scopes.md"
run_trace_case "$idless_dir" "an id-less heading cannot blanket-match"
assert_case_status 1 "Declared id adversarial: with no id on the heading the unrelated DoD item is still unmapped"

# One scenario is legitimately covered by more than one row: a page-integrity row
# naming the page under test, plus the e2e row naming the spec that exercises it.
# Row order is arbitrary, so matching the first row and stopping made the
# concrete-path check depend on authoring order. Here the path-less row is listed
# FIRST, so a first-match-wins implementation fails this fixture.
multirow_dir="$TMPDIR/multirow-path"
build_clean_feature "$multirow_dir"
cat > "$multirow_dir/scopes.md" <<'EOF'
# Scope 01: Widget Render

**Status:** In Progress

### Gherkin

#### SCN-MULTI-88 - governing heading carries the identifier

  Scenario: Widget renders with provided label
    Given a label "Hello"
    When the widget mounts
    Then the rendered output displays "Hello"

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --------- | -------- | ------------- | ----------- | ------- | ----------- |
| Page integrity | functional | widget.html | SCN-MULTI-88 page parse with no directory prefix | selftest:page | No |
| E2E       | e2e-ui   | tests/widget-render.e2e.spec.ts | SCN-MULTI-88 widget renders with provided label and displays it | selftest:widget-render | Yes |

### Definition of Done

- [x] SCN-MULTI-88 widget renders with provided label and displays the rendered output -> Evidence: report.md#test-evidence
EOF
cat > "$multirow_dir/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "scenarioId": "SCN-MULTI-88",
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
run_trace_case "$multirow_dir" "multiple explicit rows are ambiguous"
assert_case_status 1 "Multi-row: multiple explicit bindings block instead of selecting one"
assert_case_contains "ambiguous explicit Test Plan bindings" \
  "Multi-row: explicit ambiguity is reported"

# Explicit and inferred DoD ambiguity must block before any arbitrary choice.
duplicate_explicit_dod_dir="$TMPDIR/duplicate-explicit-dod"
write_declared_id_scope "$duplicate_explicit_dod_dir" "SCN-ALPHA-77" "SCN-ALPHA-77"
cat >> "$duplicate_explicit_dod_dir/scopes.md" <<'EOF'
- [x] SCN-ALPHA-77 second explicit completion claim -> Evidence: report.md#test-evidence
EOF
run_trace_case "$duplicate_explicit_dod_dir" "multiple explicit DoD bindings are ambiguous"
assert_case_status 1 "Explicit DoD ambiguity exits 1"
assert_case_contains "ambiguous explicit DoD bindings" \
  "Explicit DoD ambiguity is reported before selection"

duplicate_inferred_dod_dir="$TMPDIR/duplicate-inferred-dod"
build_clean_feature "$duplicate_inferred_dod_dir"
cat >> "$duplicate_inferred_dod_dir/scopes.md" <<'EOF'
- [x] Widget renders provided label and displays rendered output in browser -> Evidence: report.md#test-evidence
EOF
run_trace_case "$duplicate_inferred_dod_dir" "multiple inferred DoD bindings are ambiguous"
assert_case_status 1 "Inferred DoD ambiguity exits 1"
assert_case_contains "ambiguous inferred DoD bindings" \
  "Inferred DoD ambiguity is blocking rather than a confidence label"

identity_rewrite_dir="$TMPDIR/identity-in-place-rewrite"
build_clean_feature "$identity_rewrite_dir"
run_trace_case_identity_rewrite "$identity_rewrite_dir" "test file rewritten in place after path resolution"
assert_case_status 1 "In-place test rewrite exits 1"
assert_case_contains "mapped test file identity changed before evidence use" \
  "Size/mtime identity revalidation catches an in-place rewrite"

identity_mode_dir="$TMPDIR/identity-mode-change"
build_clean_feature "$identity_mode_dir"
run_trace_case_mode_operation "$identity_mode_dir" "test file mode changes after shared projection" change
assert_case_status 1 "Chmod-only test identity change exits 1"
assert_case_contains "linked test identity changed before content-sensitive use" \
  "Permission-mode identity revalidation catches chmod-only mutation"

identity_mode_same_dir="$TMPDIR/identity-mode-unchanged"
build_clean_feature "$identity_mode_same_dir"
run_trace_case_mode_operation "$identity_mode_same_dir" "unchanged test file mode after shared projection" same
assert_case_status 0 "Unchanged-mode operation remains accepted"

# --- Not Started scopes defer report evidence, started scopes do not ----------
# A scope that has not run cannot have produced evidence. Reporting that as a
# defect describes the framework's own sequential execution model as a failure
# and buries the findings belonging to scopes actually under way. The twin below
# is what keeps this from degrading into a blanket exemption.
build_evidenceless_feature() {
  local feature_dir="$1"
  local scope_status="$2"
  build_clean_feature "$feature_dir"
  cat > "$feature_dir/report.md" <<'EOF'
# Report

### Test Evidence

```
$ no run has happened yet
```
EOF
  bubbles_sed_inplace \
    "s/^\*\*Status:\*\* In Progress\$/**Status:** $scope_status/" \
    "$feature_dir/scopes.md"
}

notstarted_feature="$TMPDIR/specs/810-notstarted-evidence"
build_evidenceless_feature "$notstarted_feature" "Not Started"
run_trace_case "$notstarted_feature" "Not Started scope defers report evidence"
assert_case_contains 'report evidence DEFERRED (scope is Not Started' "Not Started: the deferral is reported, never silent"
assert_case_not_contains 'report is missing evidence reference' "Not Started: no missing-evidence failure is raised"
assert_case_contains 'Report evidence DEFERRED to their own execution (Not Started scopes): 1' "Not Started: the deferral is counted in the summary"

# Adversarial twin: the identical fixture with a started status MUST still fail,
# or the deferral is a blanket exemption rather than a status-scoped one.
started_feature="$TMPDIR/specs/811-started-evidence"
build_evidenceless_feature "$started_feature" "In Progress"
run_trace_case "$started_feature" "In Progress scope still requires report evidence"
assert_case_status 1 "Adversarial twin: an In Progress scope exits 1 on missing evidence"
assert_case_contains 'report is missing evidence reference' "Adversarial twin: a started scope still fails on missing evidence"
assert_case_not_contains 'report evidence DEFERRED' "Adversarial twin: a started scope is never deferred"

# --- Current-scope manifest projection preserves sequential execution ---------
# Scope 03 declares a planned test file that does not exist yet. Scope 02 is the
# current scope and depends on completed Scope 01. --current-scope must omit the
# Not Started descendant from BOTH the scope-file checks and manifest file
# checks; default --all-scopes remains strict and must fail on the same fixture.
build_current_scope_projection_feature() {
  local feature_dir="$1"
  mkdir -p \
    "$feature_dir/scopes/01-prerequisite" \
    "$feature_dir/scopes/02-current" \
    "$feature_dir/scopes/03-future" \
    "$feature_dir/tests"

  cat > "$feature_dir/tests/prerequisite.spec.ts" <<'EOF'
export const prerequisite = true;
EOF
  cat > "$feature_dir/tests/current.spec.ts" <<'EOF'
export const current = true;
EOF
  mkdir -p \
    "$feature_dir/scopes/01-prerequisite/tests" \
    "$feature_dir/scopes/02-current/tests"
  cp "$feature_dir/tests/prerequisite.spec.ts" "$feature_dir/scopes/01-prerequisite/tests/prerequisite.spec.ts"
  cp "$feature_dir/tests/current.spec.ts" "$feature_dir/scopes/02-current/tests/current.spec.ts"

  cat > "$feature_dir/scopes/01-prerequisite/scope.md" <<'EOF'
# Scope 01: Prerequisite

**Status:** Done

### Gherkin

#### SCN-812-001

Scenario: Prerequisite behavior remains available
  Given a completed prerequisite
  When the current scope starts
  Then prerequisite behavior remains available

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| E2E | e2e-ui | tests/prerequisite.spec.ts | SCN-812-001 prerequisite behavior remains available | selftest:prerequisite | Yes |

### Definition of Done

- [x] SCN-812-001 prerequisite behavior remains available -> Evidence: report.md#test-evidence
EOF
  cat > "$feature_dir/scopes/01-prerequisite/report.md" <<'EOF'
# Report

### Test Evidence

tests/prerequisite.spec.ts passed.
EOF

  cat > "$feature_dir/scopes/02-current/scope.md" <<'EOF'
# Scope 02: Current

**Status:** In Progress

### Gherkin

#### SCN-812-002

Scenario: Current behavior executes before future work
  Given the prerequisite is complete
  When current behavior executes
  Then future work is not required yet

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| E2E | e2e-ui | tests/current.spec.ts | SCN-812-002 current behavior executes before future work | selftest:current | Yes |

### Definition of Done

- [x] SCN-812-002 current behavior executes before future work -> Evidence: report.md#test-evidence
EOF
  cat > "$feature_dir/scopes/02-current/report.md" <<'EOF'
# Report

### Test Evidence

tests/current.spec.ts passed.
EOF

  cat > "$feature_dir/scopes/03-future/scope.md" <<'EOF'
# Scope 03: Future

**Status:** Not Started

### Gherkin

#### SCN-812-003

Scenario: Future behavior runs after current completion
  Given the current scope is complete
  When future behavior runs
  Then its own test proves the result

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
| --- | --- | --- | --- | --- | --- |
| E2E | e2e-ui | tests/future-not-authored.spec.ts | SCN-812-003 future behavior runs after current completion | selftest:future | Yes |

### Definition of Done

- [ ] SCN-812-003 future behavior runs after current completion -> Evidence: report.md#test-evidence
EOF
  cat > "$feature_dir/scopes/03-future/report.md" <<'EOF'
# Report

### Test Evidence

No future execution has occurred.
EOF

  cat > "$feature_dir/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "scenarioId": "SCN-812-001",
      "scopeRef": "scopes/01-prerequisite/scope.md",
      "title": "Prerequisite behavior remains available",
      "linkedTests": [{"file": "tests/prerequisite.spec.ts"}],
      "evidenceRefs": ["scopes/01-prerequisite/report.md#test-evidence"]
    },
    {
      "scenarioId": "SCN-812-002",
      "scopeRef": "scopes/02-current/scope.md",
      "title": "Current behavior executes before future work",
      "linkedTests": [{"file": "tests/current.spec.ts"}],
      "evidenceRefs": ["scopes/02-current/report.md#test-evidence"]
    },
    {
      "scenarioId": "SCN-812-003",
      "scopeRef": "scopes/03-future/scope.md",
      "title": "Future behavior runs after current completion",
      "linkedTests": [{"file": "tests/future-not-authored.spec.ts"}],
      "evidenceRefs": ["scopes/03-future/report.md#test-evidence"]
    }
  ]
}
EOF

  cat > "$feature_dir/state.json" <<'EOF'
{
  "version": 3,
  "status": "in_progress",
  "scopeLayout": "per-scope-directory",
  "execution": {
    "currentPhase": "implement",
    "currentScope": 2,
    "scopeProgress": [
      {"scope": 1, "scopeId": "01-prerequisite", "scopeDir": "scopes/01-prerequisite", "status": "done", "dependsOn": []},
      {"scope": 2, "scopeId": "02-current", "scopeDir": "scopes/02-current", "status": "in_progress", "dependsOn": ["01-prerequisite"]},
      {"scope": 3, "scopeId": "03-future", "scopeDir": "scopes/03-future", "status": "not_started", "dependsOn": ["02-current"]}
    ]
  },
  "certification": {
    "status": "in_progress",
    "scopeProgress": [
      {"scope": 1, "scopeId": "01-prerequisite", "scopeDir": "scopes/01-prerequisite", "status": "done", "dependsOn": []},
      {"scope": 2, "scopeId": "02-current", "scopeDir": "scopes/02-current", "status": "in_progress", "dependsOn": ["01-prerequisite"]},
      {"scope": 3, "scopeId": "03-future", "scopeDir": "scopes/03-future", "status": "not_started", "dependsOn": ["02-current"]}
    ]
  }
}
EOF
}

current_scope_feature="$TMPDIR/specs/812-current-scope-projection"
build_current_scope_projection_feature "$current_scope_feature"
cat > "$current_scope_feature/tests/future-not-authored.spec.ts" <<'EOF'
test('future placeholder fixture is outside current-scope projection', () => {});
EOF
bubbles_sed_inplace \
  's#"file": "tests/current.spec.ts"#"file": "  tests/current.spec.ts  "#' \
  "$current_scope_feature/scenario-manifest.json"
run_trace_case "$current_scope_feature" "current scope omits a future planned manifest test" "--current-scope"
assert_case_status 0 "Current-scope projection: a future Not Started test does not block the current scope"
assert_case_contains 'scenario-manifest.json linked test exists: tests/current.spec.ts' \
  "Current-scope projection: the current manifest binding is still validated"
assert_case_not_contains 'future-not-authored.spec.ts' \
  "Current-scope projection: the future descendant is absent from the applicable manifest universe"
bubbles_sed_inplace \
  's#"file": "  tests/current.spec.ts  "#"file": "tests/current.spec.ts"#' \
  "$current_scope_feature/scenario-manifest.json"

rm "$current_scope_feature/tests/future-not-authored.spec.ts"
run_trace_case "$current_scope_feature" "all scopes remain strict about a future missing test" "--all-scopes"
assert_case_status 1 "All-scope adversarial: the same future missing test still fails terminal/all-scope validation"
assert_case_contains 'scenario SCN-812-003 linkedTests[0]: authored path is not an existing stable regular file' \
  "All-scope adversarial: the missing future manifest binding is named"

mv "$current_scope_feature/tests/current.spec.ts" "$current_scope_feature/tests/current.spec.ts.saved"
run_trace_case "$current_scope_feature" "current scope still fails on its own missing test" "--current-scope"
assert_case_status 1 "Current-scope adversarial: a missing current test is never deferred"
assert_case_contains 'scenario SCN-812-002 linkedTests[0]: authored path is not an existing stable regular file' \
  "Current-scope adversarial: the missing current manifest binding is named"
mv "$current_scope_feature/tests/current.spec.ts.saved" "$current_scope_feature/tests/current.spec.ts"

unknown_scope_feature="$TMPDIR/specs/813-current-scope-unknown-reference"
build_current_scope_projection_feature "$unknown_scope_feature"
bubbles_sed_inplace \
  's#scopes/03-future/scope.md#scopes/99-unknown/scope.md#' \
  "$unknown_scope_feature/scenario-manifest.json"
run_trace_case "$unknown_scope_feature" "current scope fails closed on an unknown manifest scope" "--current-scope"
assert_case_status 1 "Current-scope fail-closed: an unknown scenario scope reference is refused"
assert_case_contains 'linked-test scope projection failed' \
  "Current-scope fail-closed: the projection failure is explicit"

integer_scope_feature="$TMPDIR/specs/814-current-scope-integer-reference"
build_current_scope_projection_feature "$integer_scope_feature"
cat > "$integer_scope_feature/tests/future-not-authored.spec.ts" <<'EOF'
test('future fixture isolates integer scopeRef resolution', () => {});
EOF
bubbles_sed_inplace 's#"scopeRef": "scopes/02-current/scope.md"#"scopeRef": 2#' "$integer_scope_feature/scenario-manifest.json"
run_trace_case "$integer_scope_feature" "current scope accepts reader-projected v1 integer scopeRef" "--current-scope"
assert_case_status 0 "CR-01: v1 positive integer scopeRef is normalized by the reader and resolves through the emitted decimal alias"
assert_case_contains 'scenario-manifest.json linked test exists: tests/current.spec.ts' \
  "CR-01: reader-projected integer current-scope manifest binding is validated"

string_scope_feature="$TMPDIR/specs/815-current-scope-migrated-string-reference"
build_current_scope_projection_feature "$string_scope_feature"
cat > "$string_scope_feature/tests/future-not-authored.spec.ts" <<'EOF'
test('future fixture isolates migrated decimal string scopeRef resolution', () => {});
EOF
bubbles_sed_inplace 's#"scopeRef": "scopes/02-current/scope.md"#"scopeRef": "2"#' "$string_scope_feature/scenario-manifest.json"
run_trace_case "$string_scope_feature" "current scope accepts migrated decimal string scopeRef" "--current-scope"
assert_case_status 0 "CR-01: migrated decimal string resolves through the same resolver-owned alias"

symbolic_scope_feature="$TMPDIR/specs/816-current-scope-symbolic-reference"
build_current_scope_projection_feature "$symbolic_scope_feature"
cat > "$symbolic_scope_feature/tests/future-not-authored.spec.ts" <<'EOF'
test('future fixture isolates symbolic scopeRef resolution', () => {});
EOF
bubbles_sed_inplace 's#"scopeId": "02-current"#"scopeId": "payments-core"#g' "$symbolic_scope_feature/state.json"
bubbles_sed_inplace 's#"currentScope": 2#"currentScope": "payments-core"#' "$symbolic_scope_feature/state.json"
bubbles_sed_inplace 's#"dependsOn": \["02-current"\]#"dependsOn": ["payments-core"]#g' "$symbolic_scope_feature/state.json"
bubbles_sed_inplace 's#"scopeRef": "scopes/02-current/scope.md"#"scopeRef": "payments-core"#' "$symbolic_scope_feature/scenario-manifest.json"
run_trace_case "$symbolic_scope_feature" "current scope resolves a symbolic identity distinct from directory basename" "--current-scope"
assert_case_status 0 "CR-01: symbolic canonical identity resolves through resolver-owned aliases"
assert_case_contains 'scenario-manifest.json linked test exists: tests/current.spec.ts' \
  "CR-01: symbolic identity maps to its unique physical scope"

unknown_alias_feature="$TMPDIR/specs/817-current-scope-unknown-alias"
build_current_scope_projection_feature "$unknown_alias_feature"
cat > "$unknown_alias_feature/tests/future-not-authored.spec.ts" <<'EOF'
test('future fixture isolates unknown scopeRef resolution', () => {});
EOF
bubbles_sed_inplace 's#"scopeRef": "scopes/03-future/scope.md"#"scopeRef": "SCOPE-03"#' "$unknown_alias_feature/scenario-manifest.json"
run_trace_case "$unknown_alias_feature" "current scope refuses a consumer-invented SCOPE alias" "--current-scope"
assert_case_status 1 "CR-01: an alias absent from resolver RECORD field 8 is refused"
assert_case_contains 'scenario SCN-812-003 scope reference resolves to 0 physical scopes' \
  "CR-01: consumer-created SCOPE alias is not synthesized"

ambiguous_alias_feature="$TMPDIR/specs/818-current-scope-ambiguous-alias"
build_current_scope_projection_feature "$ambiguous_alias_feature"
cat > "$ambiguous_alias_feature/tests/future-not-authored.spec.ts" <<'EOF'
test('future fixture isolates ambiguous resolver alias resolution', () => {});
EOF
bubbles_sed_inplace 's#"scope": 2, "scopeId": "02-current"#"scope": "shared", "scopeId": "02-current"#g' "$ambiguous_alias_feature/state.json"
bubbles_sed_inplace 's#"scope": 3, "scopeId": "03-future"#"scope": "shared", "scopeId": "03-future"#g' "$ambiguous_alias_feature/state.json"
bubbles_sed_inplace 's#"currentScope": 2#"currentScope": "shared"#' "$ambiguous_alias_feature/state.json"
bubbles_sed_inplace 's#"scopeRef": "scopes/02-current/scope.md"#"scopeRef": "shared"#' "$ambiguous_alias_feature/scenario-manifest.json"
run_trace_case "$ambiguous_alias_feature" "current scope refuses an ambiguous resolver-owned alias" "--current-scope"
assert_case_status 2 "CR-01: resolver alias collision is refused before manifest projection"
assert_case_contains "scope-universe-resolver: ambiguous scope alias 'shared' identifies canonical scopes: 02-current, 03-future" \
  "CR-01: resolver reports alias ambiguity deterministically"

boolean_scope_feature="$TMPDIR/specs/819-current-scope-boolean-reference"
build_current_scope_projection_feature "$boolean_scope_feature"
bubbles_sed_inplace 's#"scopeRef": "scopes/02-current/scope.md"#"scopeRef": true#' "$boolean_scope_feature/scenario-manifest.json"
run_trace_case "$boolean_scope_feature" "shared reader refuses boolean scopeRef" "--current-scope"
assert_case_status 1 "CR-01: boolean scopeRef remains invalid"
assert_case_contains "scope alias 'scopeRef' must be a nonblank string or positive integer" \
  "CR-01: boolean rejection remains owned by the shared reader"

for invalid_scope_ref in 0 -1; do
  invalid_scope_feature="$TMPDIR/specs/820-current-scope-invalid-${invalid_scope_ref#-}-reference"
  build_current_scope_projection_feature "$invalid_scope_feature"
  bubbles_sed_inplace "s#\"scopeRef\": \"scopes/02-current/scope.md\"#\"scopeRef\": $invalid_scope_ref#" "$invalid_scope_feature/scenario-manifest.json"
  run_trace_case "$invalid_scope_feature" "shared reader refuses invalid numeric scopeRef $invalid_scope_ref" "--current-scope"
  assert_case_status 1 "CR-01: numeric scopeRef $invalid_scope_ref remains invalid"
  assert_case_contains "scope alias 'scopeRef' must be a nonblank string or positive integer" \
    "CR-01: numeric scopeRef $invalid_scope_ref rejection remains reader-owned"
done

physical_alias_duplicate_feature="$TMPDIR/specs/821-current-scope-physical-alias-duplicate"
build_current_scope_projection_feature "$physical_alias_duplicate_feature"
cat > "$physical_alias_duplicate_feature/tests/future-not-authored.spec.ts" <<'EOF'
test('future fixture isolates physical alias duplicate reconciliation', () => {});
EOF
python3 - "$physical_alias_duplicate_feature/scenario-manifest.json" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    document = json.load(handle)
current = next(item for item in document["scenarios"] if item["scenarioId"] == "SCN-812-002")
duplicate = dict(current)
duplicate["scopeRef"] = 2
document["scenarios"].append(duplicate)
with open(path, "w", encoding="utf-8") as handle:
    json.dump(document, handle, indent=2)
    handle.write("\n")
PY
run_trace_case "$physical_alias_duplicate_feature" \
  "current scope rejects duplicate scenario records through physical-scope aliases" "--current-scope"
assert_case_status 1 "CR-01 physical-scope aliases cannot hide duplicate scenario records"
assert_case_contains "scenario SCN-812-002: duplicate effective scenario id" \
  "CR-01 canonical id and legacy scenarioId exact-once semantics reject the duplicate before alias projection"

# Regression: bugs/BUG-046-traceability-linked-test-path-containment/
# --- BUG-046: linked-test paths are inert, canonical, and repository-bound ---

# SCN-B046-001: preserve both existing candidate roots and fragment behavior.
b046_valid_repo="$TMPDIR/b046-valid-repo"
build_b046_feature "$b046_valid_repo" "specs/046-valid"
cat > "$b046_valid_repo/specs/046-valid/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-B046-FIXTURE-001",
      "title": "Linked test reference validation remains contained",
      "linkedTests": [
        "tests/repository-control.spec.ts#repository-fragment",
        {"file": "tests/feature-control.spec.ts"}
      ],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_b046_trace_case "$b046_valid_repo" "specs/046-valid" \
  "BUG-046 accepts repository-relative and feature-relative regular files without rewrites"
assert_case_status 0 "BUG-046 valid roots: repository-relative and feature-relative references both remain accepted"
assert_case_contains 'linked test exists: tests/repository-control.spec.ts' \
  "BUG-046 valid roots: string fragment extraction preserves the repository candidate"
assert_case_contains 'linked test exists: tests/feature-control.spec.ts' \
  "BUG-046 valid roots: object file form preserves the feature candidate"

b046_missing_repo="$TMPDIR/b046-missing-repo"
build_b046_feature "$b046_missing_repo" "specs/046-missing"
bubbles_sed_inplace \
  's#tests/feature-control.spec.ts#tests/missing-sibling.spec.ts#' \
  "$b046_missing_repo/specs/046-missing/scenario-manifest.json"
run_b046_trace_case "$b046_missing_repo" "specs/046-missing" \
  "BUG-046 valid-root negative control uses a missing sibling"
assert_case_status 1 "BUG-046 valid roots: a missing sibling cannot satisfy the edge"

# SCN-B046-002: present and absent traversal targets share one lexical refusal.
cat > "$TMPDIR/b046-external-present.spec.ts" <<'EOF'
export const externalTraversalTarget = true;
EOF
b046_traversal_present_repo="$TMPDIR/b046-traversal-present-repo"
build_b046_feature "$b046_traversal_present_repo" "specs/046-traversal-present"
cat > "$b046_traversal_present_repo/specs/046-traversal-present/scenario-manifest.json" <<'EOF'
{
  "scenarios": [
    {
      "id": "SCN-B046-FIXTURE-001",
      "title": "Linked test reference validation remains contained",
      "linkedTests": ["../b046-external-present.spec.ts"],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_b046_trace_case "$b046_traversal_present_repo" "specs/046-traversal-present" \
  "BUG-046 rejects a present parent traversal target"
assert_case_status 1 "BUG-046 traversal present: an external regular file cannot satisfy the edge"
assert_case_contains 'parent-traversal' "BUG-046 traversal present: the lexical rejection class is stable"
assert_case_not_contains 'b046-external-present.spec.ts' \
  "BUG-046 traversal present: the unsafe raw reference is not disclosed"

b046_traversal_absent_repo="$TMPDIR/b046-traversal-absent-repo"
build_b046_feature "$b046_traversal_absent_repo" "specs/046-traversal-absent"
cat > "$b046_traversal_absent_repo/specs/046-traversal-absent/scenario-manifest.json" <<'EOF'
{
  "scenarios": [
    {
      "id": "SCN-B046-FIXTURE-001",
      "title": "Linked test reference validation remains contained",
      "linkedTests": ["../b046-external-absent.spec.ts"],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_b046_trace_case "$b046_traversal_absent_repo" "specs/046-traversal-absent" \
  "BUG-046 rejects an absent parent traversal target"
assert_case_status 1 "BUG-046 traversal absent: a missing external target cannot satisfy the edge"
assert_case_contains 'parent-traversal' "BUG-046 traversal absent: the rejection does not depend on existence"
assert_case_not_contains 'b046-external-absent.spec.ts' \
  "BUG-046 traversal absent: the unsafe raw reference is not disclosed"

# SCN-B046-003: host-independent absolute forms reject even when matching
# literal fixture files would make an existence-only check pass.
b046_absolute_repo="$TMPDIR/b046-absolute-repo"
build_b046_feature "$b046_absolute_repo" "specs/046-absolute"
mkdir -p "$b046_absolute_repo/etc"
cat > "$b046_absolute_repo/etc/hosts" <<'EOF'
fixture hosts content must remain unread
EOF
b046_drive_prefix='C:'
b046_drive_name="${b046_drive_prefix}"'\fixture.spec.ts'
b046_unc_name='\\server\share.spec.ts'
cat > "$b046_absolute_repo/$b046_drive_name" <<'EOF'
export const driveQualifiedFixture = true;
EOF
cat > "$b046_absolute_repo/$b046_unc_name" <<'EOF'
export const uncFixture = true;
EOF
b046_drive_json_prefix='C:'
b046_drive_json="${b046_drive_json_prefix}"'\\fixture.spec.ts'
b046_unc_json='\\\\server\\share.spec.ts'
cat > "$b046_absolute_repo/specs/046-absolute/scenario-manifest.json" <<EOF
{
  "scenarios": [
    {
      "id": "SCN-B046-FIXTURE-001",
      "title": "Linked test reference validation remains contained",
      "linkedTests": [
        "/etc/hosts",
        "${b046_drive_json}",
        "${b046_unc_json}"
      ],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_b046_trace_case "$b046_absolute_repo" "specs/046-absolute" \
  "BUG-046 rejects POSIX drive-qualified and UNC absolute forms"
assert_case_status 1 "BUG-046 absolute forms: every host-independent absolute class fails"
assert_case_occurrences 3 'absolute-reference' \
  "BUG-046 absolute forms: POSIX drive-qualified and UNC records share the lexical class"
assert_case_not_contains 'fixture hosts content must remain unread' \
  "BUG-046 absolute forms: no candidate content reaches diagnostics"

run_b046_trace_case_system_bash "$b046_absolute_repo" "specs/046-absolute" \
  "BUG-046 absolute classification under macOS system Bash"
assert_case_status 1 "BUG-046 portability: system Bash rejects every absolute form"
assert_case_occurrences 3 'absolute-reference' \
  "BUG-046 portability: system Bash preserves the host-independent classification"

# SCN-B046-004: present and absent external symlink targets use the same class.
cat > "$TMPDIR/B046_EXTERNAL_SECRET_PRESENT.spec.ts" <<'EOF'
EXTERNAL_SECRET_CONTENT_MUST_NOT_APPEAR
EOF
b046_symlink_repo="$TMPDIR/b046-symlink-repo"
build_b046_feature "$b046_symlink_repo" "specs/046-symlink"
mkdir -p "$b046_symlink_repo-sibling"
cat > "$b046_symlink_repo-sibling/prefix-target.spec.ts" <<'EOF'
PREFIX_SIBLING_CONTENT_MUST_NOT_APPEAR
EOF
ln -s '../../B046_EXTERNAL_SECRET_PRESENT.spec.ts' "$b046_symlink_repo/tests/external-present-link.spec.ts"
ln -s '../../B046_EXTERNAL_SECRET_ABSENT.spec.ts' "$b046_symlink_repo/tests/external-absent-link.spec.ts"
ln -s '../../b046-symlink-repo-sibling/prefix-target.spec.ts' "$b046_symlink_repo/tests/external-prefix-link.spec.ts"
cat > "$b046_symlink_repo/specs/046-symlink/scenario-manifest.json" <<'EOF'
{
  "scenarios": [
    {
      "id": "SCN-B046-FIXTURE-001",
      "title": "Linked test reference validation remains contained",
      "linkedTests": [
        "tests/external-present-link.spec.ts",
        "tests/external-absent-link.spec.ts",
        "tests/external-prefix-link.spec.ts"
      ],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_b046_trace_case "$b046_symlink_repo" "specs/046-symlink" \
  "BUG-046 rejects present and absent external symlink targets without target disclosure"
assert_case_status 1 "BUG-046 external symlinks: neither target can satisfy the edge"
assert_case_occurrences 3 'outside-repository' \
  "BUG-046 external symlinks: present absent and prefix-sibling targets share one rejection class"
assert_case_not_contains 'B046_EXTERNAL_SECRET_PRESENT' \
  "BUG-046 external symlinks: the present external target path is not disclosed"
assert_case_not_contains 'B046_EXTERNAL_SECRET_ABSENT' \
  "BUG-046 external symlinks: the absent external target path is not disclosed"
assert_case_not_contains 'EXTERNAL_SECRET_CONTENT_MUST_NOT_APPEAR' \
  "BUG-046 external symlinks: external contents are never disclosed"
assert_case_not_contains 'PREFIX_SIBLING_CONTENT_MUST_NOT_APPEAR' \
  "BUG-046 external symlinks: a textual repository prefix cannot establish containment"

# SCN-B046-005: an internal regular-file link remains eligible, while an
# internal directory link reaches the contained non-regular refusal.
b046_internal_link_repo="$TMPDIR/b046-internal-link-repo"
build_b046_feature "$b046_internal_link_repo" "specs/046-internal-link"
cat > "$b046_internal_link_repo/tests/internal-target.spec.ts" <<'EOF'
export const internalTarget = true;
EOF
mkdir -p "$b046_internal_link_repo/tests/internal-directory"
ln -s 'internal-target.spec.ts' "$b046_internal_link_repo/tests/internal-file-link.spec.ts"
ln -s 'internal-directory' "$b046_internal_link_repo/tests/internal-directory-link.spec.ts"
ln -s 'loop-b.spec.ts' "$b046_internal_link_repo/tests/loop-a.spec.ts"
ln -s 'loop-a.spec.ts' "$b046_internal_link_repo/tests/loop-b.spec.ts"
cat > "$b046_internal_link_repo/specs/046-internal-link/scenario-manifest.json" <<'EOF'
{
  "scenarios": [
    {
      "id": "SCN-B046-FIXTURE-001",
      "title": "Linked test reference validation remains contained",
      "linkedTests": [
        "tests/internal-file-link.spec.ts",
        "tests/internal-directory-link.spec.ts",
        "tests/loop-a.spec.ts"
      ],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_b046_trace_case "$b046_internal_link_repo" "specs/046-internal-link" \
  "BUG-046 accepts a contained regular-file symlink and rejects an internal directory link"
assert_case_status 1 "BUG-046 internal symlinks: the non-regular sibling keeps the packet failing"
assert_case_contains 'linked test exists: tests/internal-file-link.spec.ts' \
  "BUG-046 internal symlinks: a contained regular-file target remains accepted"
assert_case_contains 'non-regular-target' \
  "BUG-046 internal symlinks: the contained directory target is classified as non-regular"
assert_case_contains 'unstable-target' \
  "BUG-046 internal symlinks: a cycle fails at the bounded symlink walk"

# SCN-B046-006: unsafe JSON text is classified before raw shell extraction.
b046_control_repo="$TMPDIR/b046-control-repo"
build_b046_feature "$b046_control_repo" "specs/046-controls"
cat > "$b046_control_repo/specs/046-controls/scenario-manifest.json" <<'EOF'
{
  "scenarios": [
    {
      "id": "SCN-B046-FIXTURE-001",
      "title": "Linked test reference validation remains contained",
      "linkedTests": [
        "",
        "   ",
        "nul\u0000reference",
        "line\nreference",
        "tab\treference",
        "escape\u001breference",
        "delete\u007freference"
      ],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_b046_trace_case "$b046_control_repo" "specs/046-controls" \
  "BUG-046 rejects empty whitespace and control-bearing references without terminal control output"
assert_case_status 1 "BUG-046 controls: every empty or control-bearing reference fails closed"
assert_case_occurrences 2 'empty-reference' \
  "BUG-046 controls: empty and printable-whitespace values share the empty class"
assert_case_occurrences 5 'control-character' \
  "BUG-046 controls: NUL newline tab escape and delete stay JSON-escaped until rejection"
assert_b046_log_has_no_active_control_bytes \
  "BUG-046 controls: captured diagnostics contain no active control byte"

# SCN-B046-007: final acceptance retains the regular-file predicate.
b046_nonregular_repo="$TMPDIR/b046-nonregular-repo"
build_b046_feature "$b046_nonregular_repo" "specs/046-nonregular"
mkdir -p "$b046_nonregular_repo/tests/nonregular-directory"
mkfifo "$b046_nonregular_repo/tests/nonregular-fifo"
python3 - "$b046_nonregular_repo/s" <<'PY'
import socket
import sys

server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
server.bind(sys.argv[1])
server.close()
PY
cat > "$b046_nonregular_repo/specs/046-nonregular/scenario-manifest.json" <<'EOF'
{
  "scenarios": [
    {
      "id": "SCN-B046-FIXTURE-001",
      "title": "Linked test reference validation remains contained",
      "linkedTests": [
        "tests/missing-target.spec.ts",
        "tests/nonregular-directory",
        "tests/nonregular-fifo",
        "s"
      ],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_b046_trace_case "$b046_nonregular_repo" "specs/046-nonregular" \
  "BUG-046 rejects missing directory FIFO and Unix-socket targets"
assert_case_status 1 "BUG-046 non-regular targets: the packet fails"
assert_case_occurrences 1 'missing-target' \
  "BUG-046 non-regular targets: the missing target retains its distinct class"
assert_case_occurrences 3 'non-regular-target' \
  "BUG-046 non-regular targets: directory FIFO and socket share the contained non-regular class"

# SCN-B046-008: path-shaped shell syntax remains data and creates no sentinel.
b046_inert_repo="$TMPDIR/b046-inert-repo"
build_b046_feature "$b046_inert_repo" "specs/046-inert"
cat > "$b046_inert_repo/specs/046-inert/scenario-manifest.json" <<'EOF'
{
  "scenarios": [
    {
      "id": "SCN-B046-FIXTURE-001",
      "title": "Linked test reference validation remains contained",
      "linkedTests": [
        "$(touch b046-command-sentinel)",
        "`touch b046-backtick-sentinel`",
        "${B046_INERT_SENTINEL}",
        "tests/*.spec.ts",
        "tests/nope;touch b046-separator-sentinel"
      ],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_b046_trace_case "$b046_inert_repo" "specs/046-inert" \
  "BUG-046 keeps substitution backtick variable wildcard and separator text inert"
assert_case_status 1 "BUG-046 inert text: command-shaped missing paths keep the packet failing"
assert_case_occurrences 5 'missing-target' \
  "BUG-046 inert text: each printable command-shaped value remains an ordinary missing path"
for b046_sentinel in \
  b046-command-sentinel \
  b046-backtick-sentinel \
  b046-variable-sentinel \
  b046-separator-sentinel; do
  if [[ ! -e "$b046_inert_repo/$b046_sentinel" ]] \
    && [[ ! -e "$b046_inert_repo/specs/046-inert/$b046_sentinel" ]]; then
    pass "BUG-046 inert text: $b046_sentinel was not created"
  else
    fail "BUG-046 inert text: $b046_sentinel was created"
  fi
done

# SCN-B046-009: string, file, path, fragment, envelope, candidate-root, and
# current-scope forms all reach the same validator without manifest rewrites.
b046_projection_repo="$TMPDIR/b046-projection-repo"
build_b046_feature "$b046_projection_repo" "specs/046-projection"
cat > "$b046_projection_repo/specs/046-projection/tests/path-form.spec.ts" <<'EOF'
export const pathForm = true;
EOF
cat > "$b046_projection_repo/specs/046-projection/scenario-manifest.json" <<'EOF'
{
  "schemaVersion": 1,
  "scenarios": [
    {
      "id": "SCN-B046-FIXTURE-001",
      "title": "Linked test reference validation remains contained",
      "linkedTests": [
        "tests/repository-control.spec.ts#fragment",
        {"file": "tests/feature-control.spec.ts"},
        {"path": "tests/path-form.spec.ts"},
        {"file": "tests/feature-control.spec.ts", "path": "tests/missing-lower-precedence.spec.ts"}
      ],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
run_b046_trace_case "$b046_projection_repo" "specs/046-projection" \
  "BUG-046 preserves object-envelope reference forms and candidate roots"
assert_case_status 0 "BUG-046 projection: object envelope and supported forms remain accepted"
assert_case_contains 'linked test exists: tests/repository-control.spec.ts' \
  "BUG-046 projection: string fragments retain repository-root resolution"
assert_case_contains 'linked test exists: tests/path-form.spec.ts' \
  "BUG-046 projection: object path form retains feature-root resolution"
assert_case_not_contains 'missing-lower-precedence.spec.ts' \
  "BUG-046 projection: object file retains precedence over path"

cat > "$b046_projection_repo/specs/046-projection/scenario-manifest.json" <<'EOF'
[
  {
    "scenarioId": "SCN-B046-FIXTURE-001",
    "title": "Linked test reference validation remains contained",
    "linkedTestContracts": [
      {"path": "tests/feature-control.spec.ts"}
    ],
    "evidenceRefs": ["report.md#test-evidence"]
  }
]
EOF
run_b046_trace_case "$b046_projection_repo" "specs/046-projection" \
  "BUG-046 preserves the legacy manifest envelope and linkedTestContracts path form"
assert_case_status 0 "BUG-046 projection: legacy envelope remains accepted"
assert_case_contains 'linked test exists: tests/feature-control.spec.ts' \
  "BUG-046 projection: linkedTestContracts path form reaches the common validator"

run_b046_trace_case "$TMPDIR" "specs/812-current-scope-projection" \
  "BUG-046 preserves current-scope projection through the common validator" "--current-scope"
assert_case_status 0 "BUG-046 projection: current-scope mode retains its applicable scenario universe"
assert_case_contains 'linked test exists: tests/current.spec.ts' \
  "BUG-046 projection: current-scope object file form remains accepted"
assert_case_not_contains 'future-not-authored.spec.ts' \
  "BUG-046 projection: current-scope mode still omits the future descendant"

# TP-B046-011: replace the common acceptance function with the former
# existence-only decision. Traversal and external-symlink fixtures must then
# false-pass, while a missing target proves the final regular-file check remains.
b046_mutation_root="$TMPDIR/b046-containment-mutation"
mkdir -p "$b046_mutation_root/bubbles/scripts"
cp \
  "$SCRIPT_DIR/traceability-guard.sh" \
  "$SCRIPT_DIR/dod-section-lib.sh" \
  "$SCRIPT_DIR/scenario-match-lib.sh" \
  "$SCRIPT_DIR/fun-mode.sh" \
  "$b046_mutation_root/bubbles/scripts/"
b046_mutant="$b046_mutation_root/bubbles/scripts/traceability-guard.sh"
b046_mutant_tmp="$b046_mutant.tmp"
if awk '
  BEGIN { replaced = 0; skipping = 0 }
  /^linked_test_reference_is_acceptable\(\) \{$/ {
    print "linked_test_reference_is_acceptable() {"
    print "  local record=\"$1\""
    print "  local scope_dir=\"$2\""
    print "  local candidate"
    print "  linked_test_reference_ordinal=\"$(jq -r '\'' .ordinal '\'' <<< \"$record\")\""
    print "  candidate=\"$(jq -r '\'' .path '\'' <<< \"$record\")\""
    print "  linked_test_reference_path=\"$candidate\""
    print "  if [[ -f \"$repo_root/$candidate\" || -f \"$scope_dir/$candidate\" ]]; then"
    print "    linked_test_reference_status=\"ok\""
    print "    return 0"
    print "  fi"
    print "  linked_test_reference_status=\"missing-target\""
    print "  return 1"
    print "}"
    replaced = 1
    skipping = 1
    next
  }
  skipping && /^}$/ { skipping = 0; next }
  !skipping { print }
  END { if (replaced != 1) exit 42 }
' "$b046_mutant" > "$b046_mutant_tmp"; then
  mv "$b046_mutant_tmp" "$b046_mutant"
else
  rm -f "$b046_mutant_tmp"
  fail "BUG-046 mutation: common acceptance function seam was not found exactly once"
fi

if [[ -f "$b046_mutant" ]]; then
  run_b046_trace_case "$b046_traversal_present_repo" "specs/046-traversal-present" \
    "BUG-046 containment-disabled mutation traversal witness" "" "$b046_mutant"
  assert_case_status 0 \
    "BUG-046 mutation: disabling containment restores the present traversal false pass"

  b046_symlink_mutation_repo="$TMPDIR/b046-symlink-mutation-repo"
  cp -R "$b046_symlink_repo" "$b046_symlink_mutation_repo"
  cat > "$b046_symlink_mutation_repo/specs/046-symlink/scenario-manifest.json" <<'EOF'
{
  "scenarios": [
    {
      "id": "SCN-B046-FIXTURE-001",
      "title": "Linked test reference validation remains contained",
      "linkedTests": ["tests/external-present-link.spec.ts"],
      "evidenceRefs": ["report.md#test-evidence"]
    }
  ]
}
EOF
  run_b046_trace_case "$b046_symlink_mutation_repo" "specs/046-symlink" \
    "BUG-046 containment-disabled mutation external-symlink witness" "" "$b046_mutant"
  assert_case_status 0 \
    "BUG-046 mutation: disabling containment restores the external-symlink false pass"

  run_b046_trace_case "$b046_traversal_absent_repo" "specs/046-traversal-absent" \
    "BUG-046 containment-disabled mutation regular-file predicate control" "" "$b046_mutant"
  assert_case_status 1 \
    "BUG-046 mutation: the missing target still fails because the regular-file check remains active"
fi

if [[ "$failures" -eq 0 ]]; then
  echo "[selftest traceability-guard] PASS"
  exit 0
fi

echo "[selftest traceability-guard] FAIL: $failures assertion(s)"
exit 1
