#!/usr/bin/env bash
#
# state-transition-guard-perf-selftest.sh — BUG-001 reliability regression
# (review R1, v6.1). Proves the guard's reliability helpers bound work that
# previously could hang Check 3G / whole-repo find walks on large repos.
#
# Asserts:
#   1. bubbles_run_with_timeout returns 124 when a command exceeds the limit.
#   2. bubbles_run_with_timeout preserves a fast command's own exit code.
#   3. bubbles_pruned_find does NOT descend into .git / node_modules / target,
#      so a synthetic repo with a huge node_modules is walked quickly.
#   4. The walk completes well within budget (proves the exclusion works).

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LIB="$SCRIPT_DIR/guard-lib.sh"

if [[ ! -f "$LIB" ]]; then
  echo "state-transition-guard-perf-selftest: SKIP (guard-lib.sh missing)"
  exit 0
fi
# shellcheck disable=SC1090
source "$LIB"

selftest_tmp_base="${TMPDIR:-$HOME/.cache}"
mkdir -p "$selftest_tmp_base"
TMPDIR="$(mktemp -d "$selftest_tmp_base/bubbles-guard-perf.XXXXXX")"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

pass_count=0
fail_count=0
pass() { echo "  PASS: $1"; pass_count=$((pass_count + 1)); }
fail() { echo "  FAIL: $1"; fail_count=$((fail_count + 1)); }

guard_perf_budget_milliseconds=30000
guard_perf_timeout_seconds=30

guard_perf_sample_pass() {
  local wall_milliseconds="$1"

  [[ "$wall_milliseconds" -lt "$guard_perf_budget_milliseconds" ]]
}

guard_perf_samples_pass() {
  local first_wall_milliseconds="$1"
  local first_cpu_milliseconds="$2"
  local retry_wall_milliseconds="${3:-}"
  local retry_cpu_milliseconds="${4:-}"

  if guard_perf_sample_pass "$first_wall_milliseconds" "$first_cpu_milliseconds"; then
    return 0
  fi
  [[ -n "$retry_wall_milliseconds" && -n "$retry_cpu_milliseconds" ]] || return 1
  guard_perf_sample_pass "$retry_wall_milliseconds" "$retry_cpu_milliseconds"
}

if guard_perf_samples_pass 51000 1 51000 1; then
  fail "low-CPU wall stalls must not satisfy the performance contract"
else
  pass "low-CPU wall stalls fail closed (wall=51s CPU=1ms on both attempts)"
fi

if guard_perf_samples_pass 51000 51000 51000 51000; then
  fail "two over-budget samples must preserve the performance failure"
else
  pass "two wall-and-CPU over-budget samples preserve the performance failure (51s/51s, 51s/51s)"
fi

if guard_perf_samples_pass 12000 51000; then
  pass "sub-budget wall time passes even when CPU diagnostics exceed budget"
else
  fail "CPU diagnostics must not reject a sub-budget wall sample"
fi

if guard_perf_samples_pass 51000 51000; then
  fail "an over-budget first sample without retry data must fail closed"
else
  pass "missing retry data cannot turn an over-budget sample into a pass"
fi

# --- 1. timeout fires --------------------------------------------------------
rc=0; bubbles_run_with_timeout 1 sleep 5 >/dev/null 2>&1 || rc=$?
# portable-ok: assertion prose only; execution uses bubbles_run_with_timeout
if [[ "$rc" -eq 124 ]]; then pass "timeout fires (124) on a hanging command"; else fail "timeout expected 124, got $rc"; fi

# --- 2. exit code preserved --------------------------------------------------
rc=0; bubbles_run_with_timeout 5 bash -c 'exit 3' >/dev/null 2>&1 || rc=$?
if [[ "$rc" -eq 3 ]]; then pass "fast command exit code preserved (3)"; else fail "exit code expected 3, got $rc"; fi

# --- 3 + 4. pruned find skips generated dirs and is fast ---------------------
# Build a synthetic repo: a real target file at the top, plus large generated
# dirs that an unbounded find would crawl.
mkdir -p "$TMPDIR/repo/specs/001-x"
echo "real" > "$TMPDIR/repo/specs/001-x/wanted_test.rs"
# Heavy generated trees (the BUG-001 hang source). Keep counts modest but
# structured so a naive walk would do real work.
for d in .git node_modules target build .venv; do
  mkdir -p "$TMPDIR/repo/$d"
  for i in $(seq 1 50); do
    mkdir -p "$TMPDIR/repo/$d/sub$i"
    : > "$TMPDIR/repo/$d/sub$i/wanted_test.rs"   # decoys with the same name
  done
done

# pruned find for the target name should match ONLY the real file, not decoys.
matches="$(bubbles_pruned_find "$TMPDIR/repo" -type f -name wanted_test.rs -print 2>/dev/null | grep -c . || true)"
if [[ "$matches" -eq 1 ]]; then
  pass "pruned find returns only the real file (1), excludes generated-dir decoys"
else
  fail "pruned find matched $matches files (expected 1 — decoys in .git/node_modules/target leaked)"
fi

# Confirm no pruned-dir path appears in the output.
leak="$(bubbles_pruned_find "$TMPDIR/repo" -type f -name wanted_test.rs -print 2>/dev/null | grep -E '/(\.git|node_modules|target|build|\.venv)/' || true)"
if [[ -z "$leak" ]]; then
  pass "no generated-directory paths leak through the prune"
else
  fail "generated-directory path leaked: $leak"
fi

# Budget: the pruned walk must finish quickly (proves exclusion). 10s is a very
# loose ceiling; the real point is it does not crawl the heavy trees.
start=$(date +%s)
bubbles_pruned_find "$TMPDIR/repo" -type f -name wanted_test.rs -print >/dev/null 2>&1 || true
elapsed=$(( $(date +%s) - start ))
if [[ "$elapsed" -le 10 ]]; then
  pass "pruned walk completes within budget (${elapsed}s <= 10s)"
else
  fail "pruned walk took ${elapsed}s (> 10s budget)"
fi

# =============================================================================
# BUG-005: Check 11 evidence-block legitimacy must be O(builtins), not O(forks).
# =============================================================================
# A ~5000-line report.md previously took ~126s in Check 11 because the inline
# loop forked a subshell per line (echo|grep fence test) and 8x per closed code
# block (echo "$block_content" | grep). After the bash-builtin conversion the
# whole guard must complete in seconds. This section ALSO proves the 8-category
# "count of DISTINCT matching categories, threshold >=2" verdict survived the
# collapse — i.e. it did NOT regress to naive matching-LINE counting.
GUARD="$SCRIPT_DIR/state-transition-guard.sh"
if [[ ! -f "$GUARD" ]]; then
  echo "  SKIP: state-transition-guard.sh not found; BUG-005 perf section skipped"
else
  b5_feature="$TMPDIR/bug005/specs/005-perf-fixture"
  mkdir -p "$b5_feature/tests"

  cat > "$b5_feature/spec.md" <<'EOF'
# BUG-005 Perf Fixture Spec

## Purpose

Synthetic fixture whose oversized report.md exercises the transition guard's
Check 11 evidence-block legitimacy scan at scale.
EOF

  cat > "$b5_feature/design.md" <<'EOF'
# BUG-005 Perf Fixture Design

## Approach

Drive the real state-transition guard against a ~5000-line report.md and time
it; assert Check 11 is now O(bash builtins) instead of O(subshell forks).
EOF

  cat > "$b5_feature/uservalidation.md" <<'EOF'
# User Validation

## Checklist

- [x] Perf fixture available for the Check 11 fork-storm regression.
EOF

  cat > "$b5_feature/scopes.md" <<'EOF'
# Scope 01: Perf Fixture

**Status:** Done

### Definition of Done

- [x] Guard runs Check 11 over a large report.md -> Evidence: report.md#test-evidence
EOF

  cat > "$b5_feature/state.json" <<'EOF'
{
  "version": 3,
  "status": "in_progress",
  "workflowMode": "bugfix-fastlane",
  "execution": { "completedPhaseClaims": ["docs"] },
  "certification": {
    "certifiedCompletedPhases": ["docs"],
    "completedScopes": ["01-perf-fixture"],
    "scopeProgress": [],
    "lockdownState": { "mode": "off", "lockedScenarioIds": [] },
    "status": "in_progress"
  },
  "policySnapshot": {
    "grill": { "mode": "off", "source": "repo-default" },
    "tdd": { "mode": "off", "source": "repo-default" },
    "autoCommit": { "mode": "off", "source": "repo-default" },
    "lockdown": { "mode": "off", "source": "repo-default" },
    "regression": { "mode": "protect-existing-scenarios", "source": "repo-default" },
    "validation": { "mode": "required", "source": "workflow-forced" },
    "workflowMode": "bugfix-fastlane"
  },
  "transitionRequests": [],
  "reworkQueue": [],
  "executionHistory": [ { "phase": "docs", "completedAt": "2026-06-14T00:00:00Z" } ],
  "lastUpdatedAt": "2026-06-14T00:00:01Z"
}
EOF

  # Build a ~5000-line report.md:
  #   * required sections (Summary has >=10 non-blank lines, Completion, Test Evidence)
  #   * ~1000 LEGITIMATE filler blocks — line 1 ("$ cargo test") alone matches
  #     categories v (cargo ) + viii (^$ ), so every filler has >=2 distinct
  #     categories regardless of the other lines.
  #   * Block A — EXACTLY 2 distinct categories {v (cargo ), vi (N warnings)} ->
  #     LEGITIMATE (proves the >=2 boundary still passes a real 2-category block).
  #   * Block B — ONLY category vi ("N errors") repeated over 5 lines -> 1
  #     distinct category -> ILLEGITIMATE. A naive matching-LINE count would see
  #     5 matches (>=2) and WRONGLY pass it; distinct-category counting fails it.
  {
    printf '# Report\n\n### Summary\n\n'
    for i in $(seq 1 12); do
      printf 'Summary prose line %s for the BUG-005 Check 11 perf fixture.\n' "$i"
    done
    printf '\n### Completion Statement\n\nFixture complete.\n\n### Test Evidence\n\n'
    for i in $(seq 1 1000); do
      printf '```text\n'
      printf '$ cargo test\n'
      printf 'test result: ok. %s passed; 0 failed\n' "$i"
      printf '   Finished in 0.%02ds\n' "$((i % 100))"
      printf '```\n\n'
    done
    # Block A — exactly 2 distinct categories {v, vi} -> LEGITIMATE
    printf '```text\n'
    printf 'cargo build --release\n'
    printf '5 warnings\n'
    printf '9 warnings\n'
    printf '```\n\n'
    # Block B — one category (vi) repeated -> ILLEGITIMATE
    printf '```text\n'
    printf '5 errors\n'
    printf '12 errors\n'
    printf '7 errors\n'
    printf '3 errors\n'
    printf '9 errors\n'
    printf '```\n'
  } > "$b5_feature/report.md"

  b5_lines="$(wc -l < "$b5_feature/report.md")"
  duration_milliseconds() {
    awk -v seconds="$1" 'BEGIN { printf "%.0f\n", seconds * 1000 }'
  }

  run_guard_perf_sample() {
    local sample_log="$1"
    local timing_log="$2"
    local real_seconds user_seconds system_seconds sample_exit=0

    TIMEFORMAT='%3R %3U %3S'
    {
      time BUBBLES_STATE_TRANSITION_GUARD_SELFTEST_FAST=1 \
        bubbles_run_with_timeout "$guard_perf_timeout_seconds" \
        bash "$GUARD" "$b5_feature" > "$sample_log" 2>&1 || sample_exit=$?
    } 2> "$timing_log"
    if ! read -r real_seconds user_seconds system_seconds < "$timing_log"; then
      fail "guard timing output is unreadable"
      real_seconds=999
      user_seconds=999
      system_seconds=999
    fi
    sample_wall_seconds="$real_seconds"
    sample_cpu_seconds="$(awk -v user_value="$user_seconds" -v system_value="$system_seconds" \
      'BEGIN { printf "%.3f\n", user_value + system_value }')"
    if [[ ! "$sample_wall_seconds" =~ ^[0-9]+([.][0-9]+)?$ ]] ||
      [[ ! "$sample_cpu_seconds" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
      fail "guard timing output is not numeric"
      sample_wall_seconds=999
      sample_cpu_seconds=999
    fi
    sample_wall_milliseconds="$(duration_milliseconds "$sample_wall_seconds")"
    sample_cpu_milliseconds="$(duration_milliseconds "$sample_cpu_seconds")"
    if [[ "$sample_exit" -eq 124 ]]; then
      sample_wall_milliseconds=999000
      sample_cpu_milliseconds=999000
    fi
  }

  b5_log="$TMPDIR/bug005-guard.log"
  b5_time_log="$TMPDIR/bug005-guard.time"
  run_guard_perf_sample "$b5_log" "$b5_time_log"
  b5_first_wall_seconds="$sample_wall_seconds"
  b5_first_cpu_seconds="$sample_cpu_seconds"
  b5_first_wall_milliseconds="$sample_wall_milliseconds"
  b5_first_cpu_milliseconds="$sample_cpu_milliseconds"
  b5_retry_wall_seconds=""
  b5_retry_cpu_seconds=""
  b5_retry_wall_milliseconds=""
  b5_retry_cpu_milliseconds=""

  # Retry one wall-clock breach to distinguish a transient host outlier. CPU is
  # diagnostic only and can never turn an over-budget wall sample into a pass.
  if ! guard_perf_sample_pass "$b5_first_wall_milliseconds" "$b5_first_cpu_milliseconds"; then
    b5_retry_log="$TMPDIR/bug005-guard-retry.log"
    b5_retry_time_log="$TMPDIR/bug005-guard-retry.time"
    run_guard_perf_sample "$b5_retry_log" "$b5_retry_time_log"
    b5_retry_wall_seconds="$sample_wall_seconds"
    b5_retry_cpu_seconds="$sample_cpu_seconds"
    b5_retry_wall_milliseconds="$sample_wall_milliseconds"
    b5_retry_cpu_milliseconds="$sample_cpu_milliseconds"
    if [[ "$b5_retry_wall_milliseconds" -lt "$b5_first_wall_milliseconds" ]]; then
      b5_log="$b5_retry_log"
    fi
  fi

  # Perf: the whole guard (Check 11 dominated; other checks do O(1) greps on the
  # report) must be seconds, not minutes. The fork-storm version took ~126s for
  # Check 11 alone on a 4888-line report; 30s is a deliberately loose ceiling.
  if guard_perf_samples_pass \
    "$b5_first_wall_milliseconds" "$b5_first_cpu_milliseconds" \
    "$b5_retry_wall_milliseconds" "$b5_retry_cpu_milliseconds"; then
    pass "guard over ${b5_lines}-line report.md stays within the 30s wall budget (first wall=${b5_first_wall_seconds}s CPU=${b5_first_cpu_seconds}s; retry wall=${b5_retry_wall_seconds:-not-run} CPU=${b5_retry_cpu_seconds:-not-run}; fork-storm was ~126s)"
  else
    fail "guard over ${b5_lines}-line report.md exceeded the wall budget (first wall=${b5_first_wall_seconds}s CPU=${b5_first_cpu_seconds}s; retry wall=${b5_retry_wall_seconds:-not-run} CPU=${b5_retry_cpu_seconds:-not-run}) — Check 11 fork-storm may have regressed"
    sed -n '1,40p' "$b5_log"
  fi

  # Correctness: EXACTLY ONE illegitimate block (Block B) must be detected. This
  # proves (a) the >=2-distinct-category blocks (1000 fillers + Block A) are all
  # judged legitimate, and (b) Block B's single category repeated across 5 lines
  # is judged ILLEGITIMATE — i.e. distinct-category counting survived. A
  # regression to matching-LINE counting would judge Block B legitimate and the
  # guard would instead print "All N evidence blocks ... legitimate", failing
  # this assertion.
  if grep -Eq 'has 1 of [0-9]+ evidence blocks that lack terminal output signals' "$b5_log"; then
    pass "Check 11 distinct-category semantics preserved (exactly 1 illegitimate block detected)"
  else
    fail "Check 11 verdict changed — expected exactly 1 illegitimate block (Block B)"
    grep -nE 'evidence blocks|--- Check 11' "$b5_log" || true
  fi

  # G093 changed-path collection must remain linear for a dirty repository.
  # Four hundred tracked paths deliberately appear in all four git evidence
  # streams (HEAD diff, cached diff, worktree diff, status), while 1,200 paths
  # appear only as untracked status entries. Event counts retain duplicates;
  # classification stores each normalized path once in first-seen order.
  g093_root="$TMPDIR/g093-path-scale"
  g093_feature="$g093_root/specs/093-path-scale"
  mkdir -p "$g093_root/bubbles/workflows" "$g093_feature" \
    "$g093_root/scripts/duplicate-events" "$g093_root/services/unique-events"
  cp "$SCRIPT_DIR/../workflows.yaml" "$g093_root/bubbles/workflows.yaml"
  cp "$SCRIPT_DIR/../workflows/modes.yaml" "$g093_root/bubbles/workflows/modes.yaml"
  cat > "$g093_feature/state.json" <<'EOF'
{
  "version": 3,
  "status": "in_progress",
  "workflowMode": "autonomous-goal"
}
EOF
  for i in $(seq 1 400); do
    printf '#!/usr/bin/env bash\nprintf "baseline-%s\\n"\n' "$i" \
      > "$g093_root/scripts/duplicate-events/path-$i.sh"
  done
  git -C "$g093_root" init -q
  git -C "$g093_root" add bubbles specs scripts
  git -C "$g093_root" -c user.name='Bubbles Selftest' \
    -c user.email='bubbles-selftest@example.invalid' commit -q -m 'seed G093 path fixture'
  for i in $(seq 1 400); do
    printf '# staged-%s\n' "$i" >> "$g093_root/scripts/duplicate-events/path-$i.sh"
  done
  git -C "$g093_root" add scripts
  for i in $(seq 1 400); do
    printf '# unstaged-%s\n' "$i" >> "$g093_root/scripts/duplicate-events/path-$i.sh"
  done
  for i in $(seq 1 1200); do
    printf 'pub const PATH_%s: usize = %s;\n' "$i" "$i" \
      > "$g093_root/services/unique-events/path-$i.rs"
  done

  g093_log="$TMPDIR/g093-path-scale.log"
  g093_time_log="$TMPDIR/g093-path-scale.time"
  g093_exit=0
  TIMEFORMAT='%3R %3U %3S'
  {
    time BUBBLES_REPO_ROOT="$g093_root" \
      bubbles_run_with_timeout "$guard_perf_timeout_seconds" \
      bash "$SCRIPT_DIR/delivery-implementation-delta-guard.sh" "$g093_feature" \
      > "$g093_log" 2>&1 || g093_exit=$?
  } 2> "$g093_time_log"
  if read -r g093_wall_seconds g093_user_seconds g093_system_seconds < "$g093_time_log"; then
    g093_wall_milliseconds="$(duration_milliseconds "$g093_wall_seconds")"
    g093_cpu_seconds="$(awk -v user_value="$g093_user_seconds" -v system_value="$g093_system_seconds" \
      'BEGIN { printf "%.3f\n", user_value + system_value }')"
    g093_cpu_milliseconds="$(duration_milliseconds "$g093_cpu_seconds")"
    if [[ "$g093_exit" -ne 124 ]] && guard_perf_sample_pass "$g093_wall_milliseconds" "$g093_cpu_milliseconds"; then
      pass "G093 classifies 2,800 path events within the 30s wall budget (wall=${g093_wall_seconds}s CPU=${g093_cpu_seconds}s)"
    else
      fail "G093 path collection exceeded or timed out at the wall budget (wall=${g093_wall_seconds}s CPU=${g093_cpu_seconds}s exit=${g093_exit})"
    fi
  else
    fail "G093 path-scale timing output is unreadable"
  fi
  if grep -Fq 'evidenceSources: git=2800 report=0 reportCodeDiffSections=0' "$g093_log"; then
    pass "G093 preserves all 2,800 git events, including 1,200 duplicate observations"
  else
    fail "G093 changed the duplicate-inclusive git event count"
  fi
  if grep -Fq 'source (1200):' "$g093_log" && grep -Fq 'runtime (400):' "$g093_log"; then
    pass "G093 deduplicates to exact source/runtime family totals (1,200 source, 400 runtime)"
  else
    fail "G093 unique path totals or family assignments changed"
  fi
fi

echo ""
echo "[state-transition-guard-perf-selftest] $pass_count passed, $fail_count failed"
[[ "$fail_count" -eq 0 ]] || exit 1
echo "[state-transition-guard-perf-selftest] OK"
exit 0
