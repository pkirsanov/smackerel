#!/usr/bin/env bash
# multi-root-honesty-selftest.sh (IMP-033 / SCOPE-7 — gap WIP-3)
#
# The operator's real workspace spans several repositories. The repository-
# binding contract is deliberately ONE repository per command, and
# repository-binding-host-context.sh fails closed on a declared workspace root
# that is not a Git worktree, with no bypass.
#
# This selftest defends BOTH halves of that arrangement, and the second half is
# the one that matters most:
#
#   (a) The refusal is now actionable — it names the offending root AND prints
#       the reduced-root command. A refusal the operator cannot act on is the
#       moment they abandon the tool and work unbound instead, which is a worse
#       outcome than the one the refusal was protecting against.
#   (b) The refusal is STILL a refusal. Exit code and fail-closed behaviour are
#       unchanged. Making an error message helpful is the classic route by which
#       a hard check quietly becomes a warning, so (b) exists specifically to
#       catch that drift.
#   (c) `closeout` prints one invocation per OTHER declared root and reads NO
#       cross-root state. Sweeping every root would be the easy answer and the
#       wrong one: it would make a MUTATING command's blast radius depend on
#       host workspace configuration rather than on an argument the operator
#       typed.
#
# Exit: 0 all cases pass, 1 any case fails.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HOST_CTX="$SCRIPT_DIR/repository-binding-host-context.sh"
CLOSEOUT="$SCRIPT_DIR/closeout-report.sh"

# Canonicalized, because both scripts under test resolve roots with `pwd -P`.
# On macOS $TMPDIR lives under /var, which is a symlink to /private/var, so an
# uncanonicalized fixture path would never match the output and every assertion
# would fail for a reason that has nothing to do with the behaviour being tested.
TMP_ROOT="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$TMP_ROOT"' EXIT
mkdir -p "$TMP_ROOT/control-home"
chmod 700 "$TMP_ROOT/control-home"
export BUBBLES_SESSION_CONTROL_HOME="$TMP_ROOT/control-home"

failures=0
pass() { printf 'PASS: %s\n' "$1"; }
fail() { printf 'FAIL: %s\n' "$1"; failures=$((failures + 1)); }

echo "Running multi-root honesty selftest (IMP-033 SCOPE-7)..."

# ---------------------------------------------------------------------------
# Fixture: several git roots plus ONE non-git root, which is exactly the shape
# that produced the unactionable refusal in the field.
# ---------------------------------------------------------------------------
mk_git_root() {
  local name="$1"
  local root="$TMP_ROOT/$name"
  git init -q "$root"
  git -C "$root" symbolic-ref HEAD refs/heads/main
  printf '.specify/runtime/\n' > "$root/.gitignore"
  printf 'seed\n' > "$root/README.md"
  git -C "$root" add -A
  git -C "$root" -c user.email=t@example.com -c user.name=Test commit -q -m seed
}

mk_git_root alpha
mk_git_root beta
mk_git_root gamma
NOT_GIT="$TMP_ROOT/plain-folder"
mkdir -p "$NOT_GIT"

SESSION_LOG="$TMP_ROOT/session.log"
printf 'session\n' > "$SESSION_LOG"

# ---------------------------------------------------------------------------
# (a) The refusal is actionable
# ---------------------------------------------------------------------------
REFUSAL="$(bash "$HOST_CTX" \
  --session-log "$SESSION_LOG" \
  --workspace-root "$TMP_ROOT/alpha" \
  --workspace-root "$NOT_GIT" \
  --workspace-root "$TMP_ROOT/beta" 2>&1)"
REFUSAL_RC=$?

if printf '%s' "$REFUSAL" | grep -Fq "$NOT_GIT"; then
  pass "a1 the refusal names the offending root"
else
  fail "a1 the refusal does not name which root was the problem"
fi

if printf '%s' "$REFUSAL" | grep -q 'repository-binding-host-context.sh --session-log'; then
  pass "a2 the refusal prints a runnable reduced-root command"
else
  fail "a2 the refusal prints no command the operator can run"
fi

# The reduced command must EXCLUDE the offending root and RETAIN the good ones,
# otherwise it is a command that fails again or one that silently narrows the
# session's authority further than the operator asked.
CMD_LINE="$(printf '%s\n' "$REFUSAL" | grep 'repository-binding-host-context.sh --session-log' | tail -1)"
if printf '%s' "$CMD_LINE" | grep -Fq "$NOT_GIT"; then
  fail "a3 the reduced command still contains the offending root"
else
  pass "a3 the reduced command drops the offending root"
fi

if printf '%s' "$CMD_LINE" | grep -Fq "$TMP_ROOT/alpha" && \
   printf '%s' "$CMD_LINE" | grep -Fq "$TMP_ROOT/beta"; then
  pass "a4 the reduced command retains every root that WAS a Git worktree"
else
  fail "a4 the reduced command dropped roots the operator never asked to drop"
fi

# A single bad root has no reduced form. Saying so is better than printing a
# command with no roots, which would fail on a different error entirely.
SOLO="$(bash "$HOST_CTX" --session-log "$SESSION_LOG" --workspace-root "$NOT_GIT" 2>&1)"
if printf '%s' "$SOLO" | grep -q 'only declared root'; then
  pass "a5 a sole bad root is reported as having no reduced form, not given an empty command"
else
  fail "a5 a sole bad root produced a misleading or empty reduced command"
fi

# ---------------------------------------------------------------------------
# (b) It is still fail-closed — the whole point of making it friendlier
# ---------------------------------------------------------------------------
if [[ "$REFUSAL_RC" -eq 2 ]]; then
  pass "b1 the refusal still exits 2 (actionable, not permissive)"
else
  fail "b1 the refusal exited $REFUSAL_RC, not 2 — the hard check softened"
fi

if printf '%s' "$REFUSAL" | grep -q '"workspaceRoots"'; then
  fail "b2 the refusal emitted a host-context payload despite refusing"
else
  pass "b2 the refusal emits NO host-context payload, so nothing downstream can proceed"
fi

# The refusal must not offer a way past itself. A --force here would defeat the
# entire binding contract, not just this check.
if bash "$HOST_CTX" --session-log "$SESSION_LOG" --workspace-root "$NOT_GIT" --force >/dev/null 2>&1; then
  fail "b3 a bypass flag was accepted by authority resolution"
else
  pass "b3 no bypass flag exists in authority resolution"
fi

# Positive control: without the bad root the SAME invocation must succeed.
# Without this, every assertion above would also pass if the script were simply
# broken and refused everything.
if bash "$HOST_CTX" --session-log "$SESSION_LOG" \
     --workspace-root "$TMP_ROOT/alpha" --workspace-root "$TMP_ROOT/beta" >/dev/null 2>&1; then
  pass "b4 the same invocation succeeds once the non-git root is dropped (positive control)"
else
  fail "b4 the reduced invocation also fails — the refusal is not specific to the bad root"
fi

# ---------------------------------------------------------------------------
# (c) closeout points at the other roots without touching them
# ---------------------------------------------------------------------------
OUT="$(BUBBLES_WORKSPACE_ROOTS="$TMP_ROOT/alpha:$TMP_ROOT/beta:$TMP_ROOT/gamma" \
  bash "$CLOSEOUT" --repo-root "$TMP_ROOT/alpha" 2>&1)"

if printf '%s' "$OUT" | grep -q 'Other declared roots'; then
  pass "c1 closeout ends with the other declared roots"
else
  fail "c1 closeout printed no other-roots section"
fi

for other in beta gamma; do
  if printf '%s' "$OUT" | grep -Fq "closeout --repo-root $TMP_ROOT/$other"; then
    pass "c2-$other a bounded per-repository invocation is printed for $other"
  else
    fail "c2-$other no invocation printed for $other"
  fi
done

# The current repository must not be offered back to the operator.
if printf '%s' "$OUT" | grep -Fq "closeout --repo-root $TMP_ROOT/alpha"; then
  fail "c3 closeout offered the repository it is already bound to"
else
  pass "c3 closeout excludes the repository it is already bound to"
fi

# The load-bearing assertion: NO cross-root state is read. beta is given a
# dirty tree and a stash that would be impossible to miss if closeout inspected
# it. Seeing any of it here would mean the one-repository-per-command contract
# had been broken by a convenience feature.
printf 'unmistakable-cross-root-string\n' > "$TMP_ROOT/beta/LEAKED.txt"
git -C "$TMP_ROOT/beta" checkout -q -b cross-root-branch-name
OUT2="$(BUBBLES_WORKSPACE_ROOTS="$TMP_ROOT/alpha:$TMP_ROOT/beta" \
  bash "$CLOSEOUT" --repo-root "$TMP_ROOT/alpha" 2>&1)"

if printf '%s' "$OUT2" | grep -q 'unmistakable-cross-root-string\|LEAKED.txt\|cross-root-branch-name'; then
  fail "c4 closeout read state from another root — the binding contract is broken"
else
  pass "c4 closeout reads NO state from other roots (they are echoed, not inspected)"
fi

# With no declared roots the section must be absent rather than empty, so a
# single-repository operator never sees a heading with nothing under it.
OUT3="$(bash "$CLOSEOUT" --repo-root "$TMP_ROOT/alpha" 2>&1)"
if printf '%s' "$OUT3" | grep -q 'Other declared roots'; then
  fail "c5 an other-roots heading was printed with no other roots declared"
else
  pass "c5 no other-roots section when the host declared none"
fi

echo ""
if [[ "$failures" -eq 0 ]]; then
  echo "multi-root-honesty-selftest: all 14 cases passed."
  exit 0
fi
echo "multi-root-honesty-selftest: $failures of 14 cases FAILED."
exit 1
