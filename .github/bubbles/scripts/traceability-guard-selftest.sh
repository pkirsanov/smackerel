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

  cat > "$feature_dir/scopes.md" <<'EOF'
# Scope 01: Widget Render

**Status:** In Progress

### Gherkin

  Scenario: Widget renders with provided label
    Given a label "Hello"
    When the widget mounts
    Then the rendered output displays "Hello"

### Test Plan

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

if [[ "$failures" -eq 0 ]]; then
  echo "[selftest traceability-guard] PASS"
  exit 0
fi

echo "[selftest traceability-guard] FAIL: $failures assertion(s)"
exit 1
