#!/usr/bin/env bash
set -euo pipefail

# pre-existing-deferral-guard-selftest.sh
#
# Hermetic selftest for `bubbles/scripts/pre-existing-deferral-guard.sh`
# (Gate G084 — pre_existing_deferral_block_gate).
#
# Stages disposable spec-tree fixtures under a `mktemp -d` workspace
# and asserts the exit-code / stdout / stderr contract for every
# Gherkin scenario plus the supporting fail-fast and exemption
# variants required by the SCOPE-3 DoD.
#
# Scenarios:
#   S0  Missing specDir argument               → exit 2 + stderr usage
#   S0b Non-existent specDir path              → exit 2 + stderr diag
#   S1  Active "pre-existing failure — out of  → exit 1 + stderr G084 +
#       session scope" phrase in report.md       file path + line number
#   S1b Active `TODO:` marker in scope.md      → exit 1 + stderr G084
#   S2  Same phrase inside `## Superseded      → exit 0 (exempt H2)
#       Decisions` H2 subsection
#   S2b `FIXME:` marker inside `## Historical  → exit 0 (exempt H2)
#       Notes` H2 subsection
#   S2c Marker inside `## Out of Scope` H2     → exit 0 (exempt H2)
#       subsection
#   S3  Clean tree, zero forbidden phrases     → exit 0 + PASS stdout
#   S4  Phrase wrapped in inline `...`         → exit 0 (inline backtick
#       backticks                                exemption)
#   S5  Phrase inside ```text fenced block     → exit 0 (fence exemption)
#   S6  Phrase inside INDENTED fenced block    → exit 0 (indented-fence
#       (nested under list-item bullet)          exemption — matches
#                                                DoD inline-evidence shape)
#
# Reference:
#   docs/Framework_Convergence_Health.md

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD_SCRIPT="$SCRIPT_DIR/pre-existing-deferral-guard.sh"

if [[ ! -x "$GUARD_SCRIPT" ]]; then
  echo "selftest: guard script not executable: $GUARD_SCRIPT" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-g084-selftest-XXXXXXXX)"
trap 'rm -rf "$WORKSPACE"' EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
FAILED_SCENARIOS=()

# -----------------------------------------------------------------------
# Helpers
# -----------------------------------------------------------------------

# Stage a fresh spec dir under the workspace and emit its absolute path.
new_spec_dir() {
  local name="$1"
  local d="$WORKSPACE/$name"
  mkdir -p "$d/scopes/01-only-scope"
  printf "# %s\n\n## Intent\n\nClean scaffold for selftest.\n" "$name" \
    > "$d/scopes/01-only-scope/scope.md"
  echo "$d"
}

# Run the guard, capturing exit code + stdout + stderr to known paths.
# Usage: run_guard <spec_dir> [extra-args...]
run_guard() {
  local spec_dir="$1"
  shift || true
  set +e
  bash "$GUARD_SCRIPT" "$spec_dir" "$@" \
    > "$WORKSPACE/stdout.last" \
    2> "$WORKSPACE/stderr.last"
  local rc=$?
  set -e
  echo "$rc" > "$WORKSPACE/exit.last"
}

# Run the guard with raw argv (no spec_dir prepended).
run_guard_raw() {
  set +e
  bash "$GUARD_SCRIPT" "$@" \
    > "$WORKSPACE/stdout.last" \
    2> "$WORKSPACE/stderr.last"
  local rc=$?
  set -e
  echo "$rc" > "$WORKSPACE/exit.last"
}

assert_exit() {
  local scenario="$1"
  local expected="$2"
  local actual
  actual="$(cat "$WORKSPACE/exit.last")"
  if [[ "$actual" == "$expected" ]]; then
    PASS_COUNT=$((PASS_COUNT + 1))
    echo "  PASS [$scenario] exit code = $expected"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    FAILED_SCENARIOS+=("$scenario:exit:expected=$expected:actual=$actual")
    echo "  FAIL [$scenario] expected exit $expected, got $actual"
    echo "    --- stdout (last) ---"
    sed 's/^/    /' "$WORKSPACE/stdout.last" || true
    echo "    --- stderr (last) ---"
    sed 's/^/    /' "$WORKSPACE/stderr.last" || true
  fi
}

assert_stdout_contains() {
  local scenario="$1"
  local needle="$2"
  if grep -qF "$needle" "$WORKSPACE/stdout.last" 2>/dev/null; then
    PASS_COUNT=$((PASS_COUNT + 1))
    echo "  PASS [$scenario] stdout contains: $needle"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    FAILED_SCENARIOS+=("$scenario:stdout-missing:$needle")
    echo "  FAIL [$scenario] stdout missing: $needle"
    echo "    --- stdout (last) ---"
    sed 's/^/    /' "$WORKSPACE/stdout.last" || true
  fi
}

assert_stderr_contains() {
  local scenario="$1"
  local needle="$2"
  if grep -qF "$needle" "$WORKSPACE/stderr.last" 2>/dev/null; then
    PASS_COUNT=$((PASS_COUNT + 1))
    echo "  PASS [$scenario] stderr contains: $needle"
  else
    FAIL_COUNT=$((FAIL_COUNT + 1))
    FAILED_SCENARIOS+=("$scenario:stderr-missing:$needle")
    echo "  FAIL [$scenario] stderr missing: $needle"
    echo "    --- stderr (last) ---"
    sed 's/^/    /' "$WORKSPACE/stderr.last" || true
  fi
}

# -----------------------------------------------------------------------
# S0 — Missing argument
# -----------------------------------------------------------------------
echo "[S0] Missing argument should exit 2 with usage on stderr"
run_guard_raw
assert_exit "S0" 2
assert_stderr_contains "S0" "Usage: bash bubbles/scripts/pre-existing-deferral-guard.sh"

# -----------------------------------------------------------------------
# S0b — Non-existent specDir
# -----------------------------------------------------------------------
echo "[S0b] Non-existent specDir should exit 2 with diagnostic"
run_guard_raw "$WORKSPACE/does-not-exist-$$"
assert_exit "S0b" 2
assert_stderr_contains "S0b" "specDir not found or not a directory"

# -----------------------------------------------------------------------
# S1 — Active "pre-existing failure — out of session scope" phrase
# -----------------------------------------------------------------------
echo "[S1] Active 'pre-existing failure — out of session scope' phrase should exit 1"
S1_DIR="$(new_spec_dir s1-active-phrase)"
cat > "$S1_DIR/report.md" <<'EOF'
# Report

## Test Evidence

The scope is otherwise complete but suffers from a pre-existing failure — out of session scope.

## Notes

None.
EOF
run_guard "$S1_DIR"
assert_exit "S1" 1
assert_stderr_contains "S1" "G084"
assert_stderr_contains "S1" "pre_existing_deferral_block_gate"
assert_stderr_contains "S1" "$S1_DIR/report.md"
assert_stderr_contains "S1" "pre-existing failure"

# -----------------------------------------------------------------------
# S1b — Active `TODO:` marker in scope.md (no exempt section)
# -----------------------------------------------------------------------
echo "[S1b] Active 'TODO:' marker in scope.md should exit 1"
S1B_DIR="$(new_spec_dir s1b-active-todo)"
cat > "$S1B_DIR/scopes/01-only-scope/scope.md" <<'EOF'
# SCOPE-1: only scope

## Intent

Demo intent.

## Implementation Plan

1. TODO: implement the missing path.
EOF
run_guard "$S1B_DIR"
assert_exit "S1b" 1
assert_stderr_contains "S1b" "G084"
assert_stderr_contains "S1b" "TODO:"
assert_stderr_contains "S1b" "$S1B_DIR/scopes/01-only-scope/scope.md"

# -----------------------------------------------------------------------
# S2 — Same phrase inside `## Superseded Decisions` H2 subsection
# -----------------------------------------------------------------------
echo "[S2] Phrase inside '## Superseded Decisions' should exit 0 (exempt)"
S2_DIR="$(new_spec_dir s2-superseded-exempt)"
cat > "$S2_DIR/report.md" <<'EOF'
# Report

## Summary

All scopes complete.

## Superseded Decisions

An earlier session claimed a pre-existing failure — out of session scope.
That claim is superseded by the current convergence-cap and compaction
discipline gates; the rationale is retained for historical traceability.
EOF
run_guard "$S2_DIR"
assert_exit "S2" 0
assert_stdout_contains "S2" "PASS Gate G084"

# -----------------------------------------------------------------------
# S2b — `FIXME:` inside `## Historical Notes`
# -----------------------------------------------------------------------
echo "[S2b] 'FIXME:' inside '## Historical Notes' should exit 0 (exempt)"
S2B_DIR="$(new_spec_dir s2b-historical-exempt)"
cat > "$S2B_DIR/report.md" <<'EOF'
# Report

## Summary

All scopes complete.

## Historical Notes

Original draft contained FIXME: investigate flaky test. Resolved in
session 2 by deleting the flaky test in favor of a deterministic
fixture. Kept here for historical traceability.
EOF
run_guard "$S2B_DIR"
assert_exit "S2b" 0
assert_stdout_contains "S2b" "PASS Gate G084"

# -----------------------------------------------------------------------
# S2c — `HACK:` inside `## Out of Scope`
# -----------------------------------------------------------------------
echo "[S2c] 'HACK:' inside '## Out of Scope' should exit 0 (exempt)"
S2C_DIR="$(new_spec_dir s2c-out-of-scope-exempt)"
cat > "$S2C_DIR/scopes/01-only-scope/scope.md" <<'EOF'
# SCOPE-1

## Intent

Narrow scope.

## Out of Scope

- HACK: the legacy fallback path that the broader refactor will address.
- Anything outside the convergence cap surface.
EOF
run_guard "$S2C_DIR"
assert_exit "S2c" 0
assert_stdout_contains "S2c" "PASS Gate G084"

# -----------------------------------------------------------------------
# S3 — Clean tree
# -----------------------------------------------------------------------
echo "[S3] Clean tree with zero forbidden phrases should exit 0"
S3_DIR="$(new_spec_dir s3-clean)"
cat > "$S3_DIR/report.md" <<'EOF'
# Report

## Summary

All scopes complete. Build green. No outstanding action items.
EOF
cat > "$S3_DIR/scopes/01-only-scope/scope.md" <<'EOF'
# SCOPE-1

## Intent

Clean scope.

## Implementation Plan

1. Implement the feature.
2. Add tests.
EOF
run_guard "$S3_DIR"
assert_exit "S3" 0
assert_stdout_contains "S3" "PASS Gate G084"
assert_stdout_contains "S3" "violations=0"

# -----------------------------------------------------------------------
# S4 — Phrase inside inline `...` backticks (active section)
# -----------------------------------------------------------------------
echo "[S4] Phrase inside inline backticks should exit 0 (backtick exemption)"
S4_DIR="$(new_spec_dir s4-inline-backticks)"
cat > "$S4_DIR/report.md" <<'EOF'
# Report

## Summary

This document enumerates forbidden phrases for reference: the guard
rejects strings such as `pre-existing failure`, `carried forward`,
and `out of session scope` outside exempt subsections.
EOF
run_guard "$S4_DIR"
assert_exit "S4" 0
assert_stdout_contains "S4" "PASS Gate G084"

# -----------------------------------------------------------------------
# S5 — Phrase inside ```text fenced code block (active section)
# -----------------------------------------------------------------------
echo "[S5] Phrase inside fenced code block should exit 0 (fence exemption)"
S5_DIR="$(new_spec_dir s5-fenced-block)"
cat > "$S5_DIR/report.md" <<'EOF'
# Report

## Test Evidence

```text
$ run-tests
FAIL legacy-flaky: pre-existing failure — out of session scope
PASS new-coverage
```

The legacy line above is captured raw terminal output and is exempt
from the guard's scan because it lives inside a fenced code block.
EOF
run_guard "$S5_DIR"
assert_exit "S5" 0
assert_stdout_contains "S5" "PASS Gate G084"

# -----------------------------------------------------------------------
# S6 — Phrase inside INDENTED fenced code block (nested in list-item
# evidence). Mirrors how DoD inline evidence is rendered in bubbles
# scope.md files (the fence opener is indented under the bullet).
# -----------------------------------------------------------------------
echo "[S6] Phrase inside INDENTED fenced code block should exit 0 (indented-fence exemption)"
S6_DIR="$(new_spec_dir s6-indented-fence)"
cat > "$S6_DIR/scopes/01-only-scope/scope.md" <<'EOF'
# SCOPE-1

## Definition of Done

- [x] Guard handles indented fences inside list-item evidence blocks

  **Phase:** implement

  ```text
  $ run-tests
  FAIL legacy-flaky: pre-existing failure — out of session scope
  PASS new-coverage
  TODO: drop the legacy adapter once the rewrite lands
  ```

  The fenced block above is indented two spaces under the DoD bullet
  but MUST still be treated as exempt by the guard.
EOF
run_guard "$S6_DIR"
assert_exit "S6" 0
assert_stdout_contains "S6" "PASS Gate G084"

# -----------------------------------------------------------------------
# Verdict
# -----------------------------------------------------------------------

echo ""
echo "============================================================"
echo "pre-existing-deferral-guard selftest verdict"
echo "  passed assertions: $PASS_COUNT"
echo "  failed assertions: $FAIL_COUNT"
if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo "  failed scenarios:"
  for s in "${FAILED_SCENARIOS[@]}"; do
    echo "    - $s"
  done
  echo "============================================================"
  exit 1
fi
echo "  result: PASS"
echo "============================================================"
exit 0
