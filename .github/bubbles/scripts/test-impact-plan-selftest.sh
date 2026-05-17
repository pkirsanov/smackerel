#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLANNER="$SCRIPT_DIR/test-impact-plan.sh"
TMP_BASE="${TMPDIR:-$HOME/.cache}"
mkdir -p "$TMP_BASE"
TMP_DIR="$(mktemp -d -p "$TMP_BASE" bubbles-test-impact-plan.XXXXXX)"
failures=0

cleanup() {
  if (( failures == 0 )) && [[ "${KEEP_SELFTEST_TMP:-0}" != "1" ]]; then
    rm -rf "$TMP_DIR"
  else
    echo "Preserving selftest workspace: $TMP_DIR" >&2
  fi
}
trap cleanup EXIT

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1" >&2; failures=$((failures + 1)); }

config_file="$TMP_DIR/bubbles-project.yaml"
changed_file_list="$TMP_DIR/changed-files.txt"

cat > "$config_file" <<'YAML'
testImpact:
  alwaysRun:
    - lint
  fullSuiteTriggers:
    - "proto/**"
  components:
    api:
      paths:
        - "backend/api/**"
        - "internal/http/**"
      testCategories:
        - unit
        - integration
      alwaysRun:
        - contract
    web:
      paths:
        - "web/**"
      testCategories:
        - ui-unit
        - e2e-ui
YAML

cat > "$changed_file_list" <<'EOF'
backend/api/routes.go
web/src/App.tsx
EOF

text_output="$TMP_DIR/text-output.txt"
json_output="$TMP_DIR/json-output.txt"
missing_config_output="$TMP_DIR/missing-config.txt"
full_suite_output="$TMP_DIR/full-suite.txt"

if "$PLANNER" --config "$config_file" --changed-file-list "$changed_file_list" > "$text_output"; then
  if grep -Fq -- '- api' "$text_output" && grep -Fq -- '- web' "$text_output" && grep -Fq -- '- integration' "$text_output" && grep -Fq -- '- e2e-ui' "$text_output" && grep -Fq -- '- lint' "$text_output" && grep -Fq -- '- contract' "$text_output"; then
    pass "text output maps changed files to components, categories, and always-run checks"
  else
    fail "text output maps changed files to components, categories, and always-run checks"
    cat "$text_output" >&2
  fi
else
  fail "planner exits successfully for mapped changes"
fi

if "$PLANNER" --format json --config "$config_file" backend/api/routes.go > "$json_output"; then
  if grep -Fq '"matchedComponents": ["api"]' "$json_output" && grep -Fq '"testCategories": ["unit", "integration"]' "$json_output"; then
    pass "json output exposes stable impact arrays"
  else
    fail "json output exposes stable impact arrays"
    cat "$json_output" >&2
  fi
else
  fail "planner json mode exits successfully"
fi

if "$PLANNER" --config "$config_file" proto/service.proto > "$full_suite_output"; then
  if grep -Fq 'Full suite required: true' "$full_suite_output" && grep -Fq 'proto/service.proto matches proto/**' "$full_suite_output"; then
    pass "full-suite trigger is detected"
  else
    fail "full-suite trigger is detected"
    cat "$full_suite_output" >&2
  fi
else
  fail "planner exits successfully for full-suite trigger"
fi

if "$PLANNER" --config "$TMP_DIR/no-such.yaml" docs/readme.md > "$missing_config_output"; then
  if grep -Fq 'Configured: false' "$missing_config_output"; then
    pass "missing optional map is non-blocking without --require-config"
  else
    fail "missing optional map is non-blocking without --require-config"
    cat "$missing_config_output" >&2
  fi
else
  fail "missing optional map is non-blocking without --require-config"
fi

if "$PLANNER" --require-config --config "$TMP_DIR/no-such.yaml" docs/readme.md >/dev/null 2>&1; then
  fail "--require-config fails when the map is absent"
else
  pass "--require-config fails when the map is absent"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "test-impact-plan selftest failed with $failures issue(s)."
  exit 1
fi

echo "test-impact-plan selftest passed."
