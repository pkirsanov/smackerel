#!/usr/bin/env bash
# File: framework-validate-changed-only-selftest.sh
#
# Hermetic selftest for --changed-only surface resolution. Uses the
# `--list-tier` DRY-LIST mode (no checks execute) so it is fast and
# non-circular, and drives a disposable clone so the real repository's git
# state can never change the verdict.
#
# Proves:
#   1. an UNCOMMITTED change to a subject script keeps ITS selftest in the run
#      set and drops an unrelated one;
#   2. once that change is COMMITTED the working-tree diff is empty, and the
#      not-yet-upstream commit range keeps the same check in the run set --
#      without this a pre-push run would degrade to the full suite;
#   3. with no working-tree change and no upstream to compare against, NOTHING
#      is skipped. An undeterminable change set must run everything, never
#      skip everything.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# macOS /tmp and /var are symlinks, and the repository-binding rules refuse a
# symlinked root, so canonicalize the fixture path rather than relaxing them.
TMP_ROOT="$(cd "$(mktemp -d)" && pwd -P)"
cleanup() { rm -rf "$TMP_ROOT"; }
trap cleanup EXIT

failures=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  failures=$((failures + 1))
}

# Both are registered unconditionally, so the listing is deterministic.
TOUCHED_SUBJECT="bubbles/scripts/tool-log.sh"
TOUCHED_LABEL="Tool-log selftest"
UNTOUCHED_LABEL="Diff-evidence guard selftest"

export BUBBLES_FRAMEWORK_VALIDATE_MODE=source

echo "Running framework-validate --changed-only selftest..."

FIXTURE="$TMP_ROOT/fixture"
source_head="$(git -C "$REPO_ROOT" rev-parse HEAD)"
git init -q "$FIXTURE"
git -C "$FIXTURE" remote add origin "$REPO_ROOT"
git -C "$FIXTURE" fetch -q origin "$source_head"
git -C "$FIXTURE" checkout -q -b main FETCH_HEAD
git -C "$FIXTURE" update-ref refs/remotes/origin/main "$source_head"
git -C "$FIXTURE" branch --set-upstream-to=origin/main main >/dev/null

if [[ "$(git -C "$FIXTURE" rev-parse HEAD)" == "$source_head" ]]; then
  pass "fixture baseline matches the exact source HEAD"
else
  fail "fixture baseline must match the exact source HEAD"
fi

# A clone carries only committed state, so the fixture would exercise the
# PREVIOUS framework-validate.sh. Replay the current worktree onto it (the same
# approach trust-doctor-selftest uses) and commit that as the fixture baseline,
# so every case below starts from a clean tree running the code under test.
if ! git -C "$REPO_ROOT" diff --cached --quiet -- .; then
  git -C "$REPO_ROOT" diff --cached --binary -- . | git -C "$FIXTURE" apply --binary
fi
if ! git -C "$REPO_ROOT" diff --quiet -- .; then
  git -C "$REPO_ROOT" diff --binary -- . | git -C "$FIXTURE" apply --binary
fi
git -C "$FIXTURE" add -A
if ! git -C "$FIXTURE" diff --cached --quiet; then
  git -C "$FIXTURE" -c user.email=selftest@example.invalid -c user.name=selftest \
    commit -qm "test: changed-only fixture baseline"
fi

list_changed_only() {
  (
    cd "$FIXTURE"
    set +e
    bash bubbles/scripts/framework-validate.sh --list-tier=full --changed-only 2>&1
  )
}

# --- 1. uncommitted change to a subject script --------------------------------
printf '\n# framework-validate changed-only selftest marker\n' >>"$FIXTURE/$TOUCHED_SUBJECT"
dirty_list="$(list_changed_only)"

if grep -qE "^WOULD-RUN:.*${TOUCHED_LABEL}" <<<"$dirty_list"; then
  pass "a modified subject script keeps its own selftest in the run set"
else
  fail "the selftest owning $TOUCHED_SUBJECT should run when that script is modified"
fi

if grep -qE "^WOULD-SKIP \(--changed-only\):.*${UNTOUCHED_LABEL}" <<<"$dirty_list"; then
  pass "an unrelated selftest is skipped when its surface is untouched"
else
  fail "an unrelated selftest should be skipped under --changed-only"
fi

# --- 2. the same change, COMMITTED (the pre-push shape) -----------------------
git -C "$FIXTURE" -c user.email=selftest@example.invalid -c user.name=selftest \
  commit -qam "test: changed-only fixture commit"

if [[ -z "$(git -C "$FIXTURE" status --porcelain=v1)" ]]; then
  pass "fixture working tree is clean after committing"
else
  fail "fixture working tree should be clean after committing"
fi

committed_list="$(list_changed_only)"

if grep -qE "^WOULD-RUN:.*${TOUCHED_LABEL}" <<<"$committed_list"; then
  pass "a committed, not-yet-pushed change still selects its selftest"
else
  fail "a committed change should be resolved from the upstream range"
fi

if grep -qE "^WOULD-SKIP \(--changed-only\):.*${UNTOUCHED_LABEL}" <<<"$committed_list"; then
  pass "a clean tree still narrows the run set instead of running everything"
else
  fail "a clean tree with an upstream should still narrow the run set"
fi

# --- 3. nothing to compare against → run everything ---------------------------
git -C "$FIXTURE" branch --unset-upstream
undeterminable_list="$(list_changed_only)"

if grep -qE '^WOULD-SKIP \(--changed-only\)' <<<"$undeterminable_list"; then
  fail "an undeterminable change set must not skip any check"
else
  pass "an undeterminable change set runs everything rather than skipping"
fi

if grep -qE "^WOULD-RUN:.*${UNTOUCHED_LABEL}" <<<"$undeterminable_list"; then
  pass "the previously-skipped check runs once the change set is undeterminable"
else
  fail "the previously-skipped check should run when no change set is resolvable"
fi

# ---------------------------------------------------------------------------
# IMP-058 SCOPE-4 / PERF-13: a bare invocation (no flags at all) now defaults
# to the same --changed-only behavior. Re-arm a determinable change set (an
# upstream to compare against, matching case 2 above) and prove the DEFAULT
# narrows the run set identically to the explicit flag, that --no-changed-only
# forces the full run back, and that BUBBLES_VALIDATE_CHANGED_ONLY=false does
# too. This is the exact property release-check.sh and pre-push.sh's
# full-validation branch depend on for their own explicit override to work.
# ---------------------------------------------------------------------------
git -C "$FIXTURE" branch --set-upstream-to=origin/main main >/dev/null

list_bare() {
  ( cd "$FIXTURE"; set +e; bash bubbles/scripts/framework-validate.sh --list-tier=full 2>&1 )
}
list_no_changed_only() {
  ( cd "$FIXTURE"; set +e; bash bubbles/scripts/framework-validate.sh --list-tier=full --no-changed-only 2>&1 )
}
list_env_override() {
  ( cd "$FIXTURE"; set +e; BUBBLES_VALIDATE_CHANGED_ONLY=false bash bubbles/scripts/framework-validate.sh --list-tier=full 2>&1 )
}

bare_list="$(list_bare)"
if grep -qE "^WOULD-SKIP \(--changed-only\):.*${UNTOUCHED_LABEL}" <<<"$bare_list"; then
  pass "a bare invocation (no flags) defaults to --changed-only narrowing"
else
  fail "a bare invocation should narrow the run set exactly like --changed-only does"
fi
if grep -qE "^WOULD-RUN:.*${TOUCHED_LABEL}" <<<"$bare_list"; then
  pass "a bare invocation still runs the check owning the actually-changed surface"
else
  fail "a bare invocation dropped the check owning the actually-changed surface"
fi

# --list-tier itself never runs a check, so this proves flag PARSING wires
# --no-changed-only correctly, not the full 3743s suite -- --list-tier=full
# already lists what WOULD run under each mode without executing anything.
no_changed_only_list="$(list_no_changed_only)"
if grep -qE '^WOULD-SKIP \(--changed-only\)' <<<"$no_changed_only_list"; then
  fail "--no-changed-only must force the full run; something is still being skipped"
else
  pass "--no-changed-only forces the full run, overriding the new default"
fi

env_override_list="$(list_env_override)"
if grep -qE '^WOULD-SKIP \(--changed-only\)' <<<"$env_override_list"; then
  fail "BUBBLES_VALIDATE_CHANGED_ONLY=false must force the full run; something is still being skipped"
else
  pass "BUBBLES_VALIDATE_CHANGED_ONLY=false forces the full run, overriding the new default"
fi

# ---------------------------------------------------------------------------
# Regression pin: the two release-critical callers must protect themselves
# EXPLICITLY rather than relying on flag absence, so neither can silently
# inherit a future default change again. This is a static source check, not a
# behavioral one -- it is a promise about the CALLERS, not the flag mechanism
# already proven above.
# ---------------------------------------------------------------------------
if grep -qE 'framework-validate\.sh"[^"]*--no-changed-only' "$REPO_ROOT/bubbles/scripts/release-check.sh"; then
  pass "release-check.sh explicitly passes --no-changed-only to framework-validate.sh"
else
  fail "release-check.sh does not explicitly force --no-changed-only -- a release could silently narrow its own validation"
fi
if grep -qE 'framework-validate\.sh"[^"]*--no-changed-only' "$REPO_ROOT/bubbles/scripts/hooks/pre-push.sh"; then
  pass "pre-push.sh's full-validation fallback explicitly passes --no-changed-only"
else
  fail "pre-push.sh's fallback does not explicitly force --no-changed-only -- a push could silently narrow its own validation"
fi

echo
if [[ "$failures" -eq 0 ]]; then
  echo "framework-validate changed-only selftest passed."
  exit 0
fi
echo "framework-validate changed-only selftest FAILED ($failures failure(s))."
exit 1
