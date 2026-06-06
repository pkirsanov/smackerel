#!/usr/bin/env bash
#
# eval-harness-selftest.sh — hermetic selftest for the v6.1 (R11) golden-task
# eval harness. Proves the scorer DISCRIMINATES: a known-good output scores
# above threshold (PASS / exit 0); a known-bad output scores below threshold
# (FAIL / exit 1). Without discrimination, the harness would be theater.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
HARNESS="$SCRIPT_DIR/eval-harness.sh"
TASKS="$REPO_ROOT/bubbles/eval/tasks"

if ! command -v python3 >/dev/null 2>&1; then
  echo "eval-harness-selftest: SKIP (python3 not installed)"
  exit 0
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

pass_count=0
fail_count=0
pass() { echo "  PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "  FAIL: $1"; fail_count=$((fail_count + 1)); }

# --- GOOD output: satisfies both golden rubrics -----------------------------
GOOD="$TMPDIR/good"; mkdir -p "$GOOD"
cat > "$GOOD/report.md" <<'EOF'
# Report

## Test Evidence
Command: bash run-tests.sh
Exit Code: 0
... 12 passed ...

## Adversarial Regression
Added an adversarial regression test that fails if the bug returns: the fixture
uses input WITHOUT the field the broken filter relied on, proving the fix.
EOF
cat > "$GOOD/spec.md" <<'EOF'
# Spec

## Outcome Contract
Intent: deliver X. Success Signal: endpoint returns 200 with the row persisted.
Hard Constraints: no data loss. Failure Condition: silent drop.

## Scenarios
Given a user, When they submit, Then the record is stored.
EOF

# --- BAD output: deferral + placeholder, thin evidence ----------------------
BAD="$TMPDIR/bad"; mkdir -p "$BAD"
cat > "$BAD/report.md" <<'EOF'
# Report

Work mostly done. TODO: add tests later. The regression coverage was deferred
to a future scope; out of scope for this session.
EOF
cat > "$BAD/spec.md" <<'EOF'
# Spec

PLACEHOLDER — TBD. Will fill in the outcome and scenarios later. lorem ipsum.
EOF

score_ratio() { printf '%s' "$1" | python3 -c 'import json,sys; print(json.load(sys.stdin)["ratio"])'; }

# --- bugfix task: GOOD passes ------------------------------------------------
rc=0; good_out="$(bash "$HARNESS" score --task "$TASKS/golden-bugfix-001.json" --output "$GOOD")" || rc=$?
gr="$(score_ratio "$good_out")"
if [[ "$rc" -eq 0 ]]; then pass "bugfix GOOD passes (exit 0, ratio=$gr)"; else fail "bugfix GOOD should pass (exit $rc, ratio=$gr)"; fi

# --- bugfix task: BAD fails --------------------------------------------------
rc=0; bad_out="$(bash "$HARNESS" score --task "$TASKS/golden-bugfix-001.json" --output "$BAD")" || rc=$?
br="$(score_ratio "$bad_out")"
if [[ "$rc" -eq 1 ]]; then pass "bugfix BAD fails (exit 1, ratio=$br)"; else fail "bugfix BAD should fail (exit $rc, ratio=$br)"; fi

# --- discrimination: GOOD ratio strictly greater than BAD --------------------
if python3 -c "import sys; sys.exit(0 if float('$gr') > float('$br') else 1)"; then
  pass "scorer discriminates: GOOD ratio ($gr) > BAD ratio ($br)"
else
  fail "scorer does not discriminate: GOOD ($gr) !> BAD ($br)"
fi

# --- feature task: GOOD passes, BAD fails ------------------------------------
rc=0; bash "$HARNESS" score --task "$TASKS/golden-feature-001.json" --output "$GOOD" >/dev/null 2>&1 || rc=$?
if [[ "$rc" -eq 0 ]]; then pass "feature GOOD passes"; else fail "feature GOOD should pass (exit $rc)"; fi
rc=0; bash "$HARNESS" score --task "$TASKS/golden-feature-001.json" --output "$BAD" >/dev/null 2>&1 || rc=$?
if [[ "$rc" -eq 1 ]]; then pass "feature BAD fails"; else fail "feature BAD should fail (exit $rc)"; fi

# --- suite run aggregates ----------------------------------------------------
rc=0; suite_out="$(bash "$HARNESS" run --suite "$TASKS" --output "$GOOD")" || rc=$?
tcount="$(printf '%s' "$suite_out" | python3 -c 'import json,sys; print(json.load(sys.stdin)["taskCount"])')"
if [[ "$rc" -eq 0 && "$tcount" -ge 2 ]]; then pass "suite run aggregates ($tcount tasks, all pass on GOOD)"; else fail "suite run unexpected (exit $rc, tasks $tcount)"; fi

echo ""
echo "[eval-harness-selftest] $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "[eval-harness-selftest] OK"
exit 0
