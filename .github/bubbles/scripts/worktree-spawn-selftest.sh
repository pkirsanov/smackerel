#!/usr/bin/env bash
# worktree-spawn-selftest.sh (IMP-107 / SCOPE-5 — gap WT-HARNESS)
# ---------------------------------------------------------------------------
# Hermetic selftest for worktree-spawn.sh (the canonical supported spawn path)
# and the marker recognition added to worktree-hygiene-report.sh. Synthesizes a
# throwaway git repo and proves, with ADVERSARIAL / non-tautological fixtures:
#   (a) --help exits 0;
#   (b) a missing REQUIRED arg (--path or --branch, or none) exits 2 (fail-loud);
#   (c) spawn creates the worktree + branch AND stamps a `.bubbles-worktree`
#       marker recording the EXACT passed { runId, mode, sessionId } plus the
#       resolved baseSha (== `git rev-parse <base>`) and an ISO8601-UTC createdAt;
#   (d) the marker is VALID JSON (checked via jq/python3 when available);
#   (e) --experiment ALSO stamps a `.design-experiment` marker;
#   (f) worktree-hygiene-report.sh RECOGNIZES the marker: the spawned worktree is
#       tagged `framework-created=yes` while a HAND-created (unmarked) worktree is
#       tagged `framework-created=no` (the negative case makes it non-tautological);
#   (g) marker recognition did NOT perturb --porcelain (reaper contract intact).
# Portable to bash 3.2 (macOS) + GNU/BSD git; uses only git + POSIX text tools.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPAWN_SH="$SCRIPT_DIR/worktree-spawn.sh"
REPORT_SH="$SCRIPT_DIR/worktree-hygiene-report.sh"

FAILURES=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILURES=$((FAILURES + 1)); }

for dep in "$SPAWN_SH" "$REPORT_SH"; do
  if [[ ! -f "$dep" ]]; then
    echo "SETUP-ABORT: required script not found: $dep" >&2
    exit 1
  fi
done
if ! command -v git >/dev/null 2>&1; then
  echo "SETUP-ABORT: git not available" >&2
  exit 1
fi

# Canonicalize (pwd -P) so the repo path matches git's physical worktree record
# on macOS too (where mktemp -d returns a /var -> /private/var symlink).
TMP_ROOT="$(cd "$(mktemp -d)" && pwd -P)"
trap 'rm -rf "$TMP_ROOT"' EXIT INT TERM
REPO="$TMP_ROOT/repo"

setup() { if ! "$@"; then echo "SETUP-ABORT: $*" >&2; exit 1; fi; }

echo "Running worktree-spawn selftest (IMP-107 / SCOPE-5 — WT-HARNESS)..."

# --- synthesize a repo with an initial commit pinned to `main` ---------------
setup git init -q "$REPO"
setup git -C "$REPO" config user.email "selftest@bubbles.local"
setup git -C "$REPO" config user.name "Bubbles Selftest"
setup git -C "$REPO" config commit.gpgsign false
setup git -C "$REPO" symbolic-ref HEAD refs/heads/main
printf 'base\n' > "$REPO/base.txt"
setup git -C "$REPO" add -A
setup git -C "$REPO" commit -qm base

MAIN_SHA="$(git -C "$REPO" rev-parse --verify main)"

# =====================================================================
# (a) --help exits 0
# =====================================================================
BUBBLES_REPO_ROOT="$REPO" bash "$SPAWN_SH" --help >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 0 ]]; then pass "a1 --help exits 0"; else fail "a1 --help exit $rc (expected 0)"; fi

# =====================================================================
# (b) missing REQUIRED args fail-loud with exit 2 (non-tautological: three
#     distinct missing-arg shapes, each must be a usage failure)
# =====================================================================
BUBBLES_REPO_ROOT="$REPO" bash "$SPAWN_SH" --path "$TMP_ROOT/wt-nobranch" >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 2 ]]; then pass "b1 missing --branch exits 2"; else fail "b1 missing --branch exit $rc (expected 2)"; fi

BUBBLES_REPO_ROOT="$REPO" bash "$SPAWN_SH" --branch orphan-branch >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 2 ]]; then pass "b2 missing --path exits 2"; else fail "b2 missing --path exit $rc (expected 2)"; fi

BUBBLES_REPO_ROOT="$REPO" bash "$SPAWN_SH" >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 2 ]]; then pass "b3 no args exits 2"; else fail "b3 no args exit $rc (expected 2)"; fi

# The failed spawns must NOT have created a branch (fail-loud = no side effects).
if git -C "$REPO" show-ref --verify --quiet refs/heads/orphan-branch; then
  fail "b4 a fail-loud spawn wrongly created a branch"
else
  pass "b4 fail-loud spawn created no branch (no side effects)"
fi

# =====================================================================
# (c) spawn creates worktree + branch + a valid marker with the recorded fields
# =====================================================================
WT_SPAWN="$TMP_ROOT/wt-spawn"
RUN_ID="RUN-5-abc123"
MODE_VAL="full-delivery"
SESSION_ID="sess-xyz-987"

spawn_out="$(BUBBLES_REPO_ROOT="$REPO" bash "$SPAWN_SH" \
  --path "$WT_SPAWN" --branch task-wt --base main \
  --mode "$MODE_VAL" --run-id "$RUN_ID" --session-id "$SESSION_ID" 2>&1)"
rc=$?
if [[ "$rc" -eq 0 ]]; then pass "c0 spawn exits 0"; else fail "c0 spawn exit $rc: $spawn_out"; fi

if [[ -d "$WT_SPAWN" ]]; then pass "c1 worktree directory created"; else fail "c1 worktree directory missing"; fi

if git -C "$REPO" show-ref --verify --quiet refs/heads/task-wt; then
  pass "c2 branch task-wt created"
else
  fail "c2 branch task-wt not created"
fi

MARKER="$WT_SPAWN/.bubbles-worktree"
if [[ -f "$MARKER" ]]; then pass "c3 .bubbles-worktree marker stamped"; else fail "c3 marker missing"; fi

if grep -q "\"runId\": \"$RUN_ID\"" "$MARKER" 2>/dev/null; then
  pass "c4 marker records passed runId"
else
  fail "c4 marker runId mismatch: $(cat "$MARKER" 2>/dev/null)"
fi
if grep -q "\"mode\": \"$MODE_VAL\"" "$MARKER" 2>/dev/null; then
  pass "c5 marker records passed mode"
else
  fail "c5 marker mode mismatch"
fi
if grep -q "\"sessionId\": \"$SESSION_ID\"" "$MARKER" 2>/dev/null; then
  pass "c6 marker records passed sessionId"
else
  fail "c6 marker sessionId mismatch"
fi
if grep -q "\"baseSha\": \"$MAIN_SHA\"" "$MARKER" 2>/dev/null; then
  pass "c7 marker baseSha == git rev-parse main"
else
  fail "c7 marker baseSha mismatch (expected $MAIN_SHA)"
fi
created_at="$(sed -nE 's/.*"createdAt": "([^"]*)".*/\1/p' "$MARKER" 2>/dev/null | head -1)"
if printf '%s\n' "$created_at" | grep -qE '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$'; then
  pass "c8 marker createdAt is ISO8601 UTC ($created_at)"
else
  fail "c8 marker createdAt not ISO8601 UTC: '${created_at:-<none>}'"
fi

# =====================================================================
# (d) marker is VALID JSON (checked when a JSON parser is available)
# =====================================================================
if command -v jq >/dev/null 2>&1; then
  if jq -e . "$MARKER" >/dev/null 2>&1; then pass "d1 marker is valid JSON (jq)"; else fail "d1 marker is NOT valid JSON (jq)"; fi
elif command -v python3 >/dev/null 2>&1; then
  if python3 -c 'import json,sys; json.load(open(sys.argv[1]))' "$MARKER" >/dev/null 2>&1; then
    pass "d1 marker is valid JSON (python3)"
  else
    fail "d1 marker is NOT valid JSON (python3)"
  fi
else
  echo "NOTE: d1 skipped — no jq/python3 available to validate marker JSON"
fi

# =====================================================================
# (e) --experiment ALSO stamps a `.design-experiment` marker
# =====================================================================
WT_EXP="$TMP_ROOT/wt-exp"
BUBBLES_REPO_ROOT="$REPO" bash "$SPAWN_SH" \
  --path "$WT_EXP" --branch exp-wt --base main --experiment >/dev/null 2>&1
rc=$?
if [[ "$rc" -eq 0 ]]; then pass "e0 experiment spawn exits 0"; else fail "e0 experiment spawn exit $rc"; fi
if [[ -f "$WT_EXP/.bubbles-worktree" ]]; then pass "e1 experiment worktree carries .bubbles-worktree"; else fail "e1 .bubbles-worktree missing on experiment"; fi
if [[ -f "$WT_EXP/.design-experiment" ]]; then pass "e2 --experiment also stamped .design-experiment"; else fail "e2 .design-experiment not stamped"; fi

# =====================================================================
# (f) the hygiene report RECOGNIZES the marker — non-tautological: a spawned
#     (marked) worktree is framework-created=yes; a HAND-created (unmarked)
#     worktree is framework-created=no.
# =====================================================================
WT_PLAIN="$TMP_ROOT/wt-plain"
setup git -C "$REPO" worktree add -q -b plain-wt "$WT_PLAIN" main

rep="$(BUBBLES_REPO_ROOT="$REPO" bash "$REPORT_SH" 2>/dev/null || true)"

spawn_line="$(printf '%s\n' "$rep" | grep -F -- "$WT_SPAWN" | head -1)"
if printf '%s\n' "$spawn_line" | grep -q 'framework-created=yes'; then
  pass "f1 spawned worktree tagged framework-created=yes"
else
  fail "f1 spawned worktree not tagged framework-created=yes: '${spawn_line:-<no line>}'"
fi

plain_line="$(printf '%s\n' "$rep" | grep -F -- "$WT_PLAIN" | head -1)"
if printf '%s\n' "$plain_line" | grep -q 'framework-created=no'; then
  pass "f2 hand-created worktree tagged framework-created=no (non-tautological)"
else
  fail "f2 hand-created worktree not tagged framework-created=no: '${plain_line:-<no line>}'"
fi

# =====================================================================
# (g) --porcelain (the reaper's input contract) is NOT perturbed by the marker:
#     it must NOT leak a framework-created tag into the machine lines.
# =====================================================================
porc="$(BUBBLES_REPO_ROOT="$REPO" bash "$REPORT_SH" --porcelain 2>/dev/null || true)"
if printf '%s\n' "$porc" | grep -qi 'framework-created'; then
  fail "g1 --porcelain leaked a framework-created tag (reaper contract perturbed)"
else
  pass "g1 --porcelain free of framework-created tag (reaper contract intact)"
fi

echo
if [[ "$FAILURES" -eq 0 ]]; then
  echo "all cases passed."
  exit 0
else
  echo "$FAILURES case(s) failed."
  exit 1
fi
