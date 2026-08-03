#!/usr/bin/env bash
# Hermetic selftest for the IMP-033 SCOPE-2 doctor hygiene surface (gap EV-5).
# ---------------------------------------------------------------------------
# `doctor` consumes worktree-hygiene-report.sh. Before SCOPE-2 it read the FIRST
# summary line only: it computed the stale-branch + stash line and then threw it
# away, and it printed a GREEN TICK over a repository holding uncommitted work
# and unpushed commits — states the first line is structurally incapable of
# observing. This selftest pins the corrected contract:
#
#   t1  a genuinely clean fixture DOES earn the green tick — the anchor that
#       makes every "no tick" assertion below non-tautological;
#   t2  uncommitted work suppresses the tick and renders as a NOTE, not a
#       warning (mid-session dirt is normal and must not cry wolf);
#   t3  unpushed commits + a non-trunk branch + a stash suppress the tick, and
#       BOTH the branch/stash line (previously discarded) and the primary-
#       worktree line reach the operator with the command that resolves them;
#   t4  a state the detector could not inspect (detached HEAD) is never
#       certified clean.
#
# The fixture is injected through BUBBLES_REPO_ROOT, which worktree-hygiene-
# report.sh honours; `doctor`'s remaining checks still run against the real
# repository, so this exercises the CONSUMER logic exactly as shipped. Each
# assertion greps `doctor`'s own stdout — no internal function is reached into.
# Exit 0 = all cases passed. Exit 1 = at least one case failed.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLI_SH="$SCRIPT_DIR/cli.sh"

FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }

# macOS mktemp -d sits under the /var symlink; canonicalize the fixture root.
TMP_ROOT="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM

# Abort setup loudly (a setup failure is a real error, not an assertion miss).
setup() { if ! "$@"; then echo "SETUP-ABORT: $*" >&2; exit 1; fi; }

echo "Running doctor hygiene surface selftest (IMP-033 SCOPE-2)..."

if [[ ! -f "$CLI_SH" ]]; then
  fail "cli.sh not found at $CLI_SH"
  exit 1
fi

# Synthesize a repo pinned to `main` regardless of the host init.defaultBranch —
# trunk detection reads the local branch name, so a `master` default would make
# every downstream assertion measure the wrong thing. `.specify/` is ignored
# because `doctor` spawns children (runtime-leases.sh) that materialize
# `.specify/runtime/` scratch inside whatever BUBBLES_REPO_ROOT names; a real
# repository ignores that scratch, and counting it as the operator's untracked
# work would make every fixture below measure framework noise instead.
mk_repo() {
  local d="$1"
  setup git init -q "$d"
  setup git -C "$d" config user.email "selftest@bubbles.local"
  setup git -C "$d" config user.name "Bubbles Selftest"
  setup git -C "$d" config commit.gpgsign false
  setup git -C "$d" symbolic-ref HEAD refs/heads/main
  printf '.specify/\n' > "$d/.gitignore"
  printf 'base\n' > "$d/base.txt"
  setup git -C "$d" add -A
  setup git -C "$d" commit -qm "base"
}

# `doctor` writes colour codes only on a TTY, but strip them anyway so a future
# forced-colour mode cannot silently break every match below.
strip_ansi() { sed "s/$(printf '\033')\[[0-9;]*m//g"; }

# Only the three hygiene rows; the rest of doctor's report is another surface's
# business and must not influence these assertions.
hygiene_rows() {
  BUBBLES_REPO_ROOT="$1" bash "$CLI_SH" doctor 2>/dev/null \
    | strip_ansi \
    | grep -E 'Worktree hygiene:|Local branches/stashes:|Primary worktree:' \
    || true
}

assert_contains() {
  local label="$1" needle="$2" haystack="$3"
  if printf '%s\n' "$haystack" | grep -qF -- "$needle"; then
    pass "$label"
  else
    fail "$label — expected to find '$needle' in:
$haystack"
  fi
}

assert_absent() {
  local label="$1" needle="$2" haystack="$3"
  if printf '%s\n' "$haystack" | grep -qF -- "$needle"; then
    fail "$label — did NOT expect '$needle' in:
$haystack"
  else
    pass "$label"
  fi
}

# =====================================================================
# t1 — ANCHOR. A genuinely clean checkout (clean tree, no remote, no
#      non-trunk branch, no stash, no linked worktree) still earns the
#      green tick. Without this, "no tick" below would pass even if the
#      tick were deleted outright.
# =====================================================================
F_CLEAN="$TMP_ROOT/clean"
mk_repo "$F_CLEAN"
ROWS_CLEAN="$(hygiene_rows "$F_CLEAN")"
assert_contains "t1 clean checkout earns the green hygiene tick (anchor)" \
  "✅ Worktree hygiene: nothing outstanding" "$ROWS_CLEAN"

# =====================================================================
# t2 — Uncommitted work suppresses the tick. Ranking: mid-session dirt is
#      a NOTE, not a warning, so this surface does not cry wolf on the
#      normal case.
# =====================================================================
F_DIRTY="$TMP_ROOT/dirty"
mk_repo "$F_DIRTY"
printf 'edited\n' > "$F_DIRTY/base.txt"
ROWS_DIRTY="$(hygiene_rows "$F_DIRTY")"
assert_absent "t2a uncommitted work forfeits the green tick" \
  "✅ Worktree hygiene: nothing outstanding" "$ROWS_DIRTY"
assert_contains "t2b uncommitted work renders as a note, naming the primary worktree" \
  "ℹ️  Primary worktree: 1 dirty files (primary)" "$ROWS_DIRTY"
assert_absent "t2c dirty-only does not escalate to a warning (no false alarm)" \
  "⚠️  Primary worktree:" "$ROWS_DIRTY"

# =====================================================================
# t3 — Unpushed commits + a non-trunk branch + a stash. All three are work
#      that no remote holds. Asserts the tick is gone, the branch/stash
#      line (which doctor used to discard entirely) is printed, and the
#      primary-worktree row escalates to a warning naming `closeout`.
# =====================================================================
F_BARE="$TMP_ROOT/bare.git"
setup git init -q --bare "$F_BARE"
F_RISK="$TMP_ROOT/risk"
mk_repo "$F_RISK"
setup git -C "$F_RISK" remote add origin "$F_BARE"
setup git -C "$F_RISK" push -q origin main
# Two commits the remote does not have.
printf 'one\n' > "$F_RISK/one.txt"
setup git -C "$F_RISK" add -A
setup git -C "$F_RISK" commit -qm "unpushed one"
printf 'two\n' > "$F_RISK/two.txt"
setup git -C "$F_RISK" add -A
setup git -C "$F_RISK" commit -qm "unpushed two"
# A non-trunk local branch.
setup git -C "$F_RISK" branch side-work
# A real stash, leaving the tracked tree clean so this fixture isolates
# "unpushed/stashed", not "dirty".
printf 'wip\n' > "$F_RISK/base.txt"
setup git -C "$F_RISK" stash push -q -m "selftest-wip"
ROWS_RISK="$(hygiene_rows "$F_RISK")"
assert_absent "t3a unpushed work forfeits the green tick" \
  "✅ Worktree hygiene: nothing outstanding" "$ROWS_RISK"
assert_contains "t3b branch/stash line reaches the operator (no longer discarded)" \
  "Local branches/stashes:" "$ROWS_RISK"
assert_contains "t3c stash is counted on that line" \
  "1 stashes" "$ROWS_RISK"
assert_contains "t3d unpushed commits escalate the primary-worktree row to a warning" \
  "⚠️  Primary worktree:" "$ROWS_RISK"
assert_contains "t3e the warning names the command that resolves it" \
  "closeout" "$ROWS_RISK"
assert_contains "t3f ahead count is reported, not transposed" \
  "2 ahead / 0 behind" "$ROWS_RISK"
assert_contains "t3g the non-trunk branch is counted" \
  "1 non-trunk local branches" "$ROWS_RISK"

# =====================================================================
# t4 — A state the detector could not inspect is never certified clean.
#      Detached HEAD makes the remote comparison impossible; every counter
#      is nonetheless zero, so a counters-only rule would wrongly tick.
# =====================================================================
F_DET="$TMP_ROOT/detached"
mk_repo "$F_DET"
setup git -C "$F_DET" checkout -q --detach
ROWS_DET="$(hygiene_rows "$F_DET")"
assert_absent "t4a an unobservable remote state forfeits the green tick" \
  "✅ Worktree hygiene: nothing outstanding" "$ROWS_DET"
assert_contains "t4b doctor says plainly that the comparison could not be made" \
  "the remote comparison could not be made" "$ROWS_DET"
assert_contains "t4c the detached HEAD is named, not hidden behind a zero" \
  "detached-HEAD" "$ROWS_DET"

# =====================================================================
# t5 — doctor's exit code is unchanged by this advisory surface. The
#      hygiene rows are informational; they must never flip a run.
# =====================================================================
BUBBLES_REPO_ROOT="$F_RISK" bash "$CLI_SH" doctor >/dev/null 2>&1
rc_risk=$?
BUBBLES_REPO_ROOT="$F_CLEAN" bash "$CLI_SH" doctor >/dev/null 2>&1
rc_clean=$?
if [[ "$rc_risk" -eq "$rc_clean" ]]; then
  pass "t5 doctor exit code identical for a dirty/unpushed and a clean fixture (advisory only)"
else
  fail "t5 hygiene state changed doctor's exit code (risk=$rc_risk clean=$rc_clean)"
fi

echo
if [[ "$FAILURES" -gt 0 ]]; then
  echo "doctor-hygiene-surface-selftest FAILED with $FAILURES issue(s)."
  exit 1
fi
echo "doctor-hygiene-surface-selftest: all cases passed."
