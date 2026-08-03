#!/usr/bin/env bash
# closeout-report-selftest.sh (IMP-033 / SCOPE-4 — gap WIP-3)
# ---------------------------------------------------------------------------
# Asserts the closeout safety contract against hermetic git fixtures.
#
# The load-bearing case is (e2): a writer-lease acquired AFTER the report is
# produced but BEFORE `--apply` runs. That is the ONLY case that distinguishes
# an action-time lease check from a snapshot-time one, and the snapshot-time
# version of this bug is the one that eats a concurrent session's work.
#
# The refusal cases would be vacuous without their positive control (d1): if
# `--apply` never deleted anything, every "it refused" assertion would pass for
# the wrong reason. d1 proves the delete path is live before the refusals are
# claimed to mean something.
#
# Exit 0 all cases pass | 1 a case failed.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLOSEOUT="$SCRIPT_DIR/closeout-report.sh"
LEASES="$SCRIPT_DIR/runtime-leases.sh"

PASS=0
FAIL=0
pass() { PASS=$((PASS + 1)); echo "PASS: $1"; }
fail() { FAIL=$((FAIL + 1)); echo "FAIL: $1"; }

TMP_ROOT="$(mktemp -d 2>/dev/null || mktemp -d -t closeout)"
TMP_ROOT="$(cd "$TMP_ROOT" && pwd -P)"
trap 'rm -rf "$TMP_ROOT"' EXIT

echo "Running closeout report selftest (IMP-033 SCOPE-4)..."

g() { fixture "$1"; git -C "$1" -c user.email=t@example.com -c user.name=Test "${@:2}"; }

# EVERY git command and every closeout invocation in this file goes through a
# path that has been proven to live inside $TMP_ROOT first. A harness bug that
# leaves a fixture path empty must abort here, because `git -C ""` is a no-op
# that silently operates on the CURRENT repository — which is how a test that
# believes it is sandboxed commits to, branches, and mutates the real checkout.
fixture() {
  case "${1:-}" in
    "") echo "HARNESS BUG: empty fixture path — refusing to run git against the ambient repo" >&2; exit 1 ;;
    "$TMP_ROOT"/*) return 0 ;;
    *) echo "HARNESS BUG: fixture path '$1' is outside $TMP_ROOT — refusing" >&2; exit 1 ;;
  esac
}

# A repo with a real upstream, so "M unpushed commits" is a real measurement
# rather than a fabricated one. The bare repo's HEAD is pinned to main because
# `git init --bare` follows the HOST's init.defaultBranch, which silently makes
# trunk detection resolve `master` on some machines.
#
# The declarations below are DELIBERATELY separate statements. In a single
# `local a=1 b=$a`, bash declares every name before evaluating any assignment,
# so `$a` is unbound under `set -u` and `b` silently becomes the wrong value.
mk_repo() {
  local name="$1"
  local repo="$TMP_ROOT/$name"
  local bare="$TMP_ROOT/$name.git"
  git init -q --bare "$bare"
  git -C "$bare" symbolic-ref HEAD refs/heads/main
  git init -q "$repo"
  git -C "$repo" symbolic-ref HEAD refs/heads/main
  mkdir -p "$repo/.specify/memory"
  printf '.specify/runtime/\n' > "$repo/.gitignore"
  printf 'seed\n' > "$repo/README.md"
  g "$repo" add -A
  g "$repo" commit -q -m "seed"
  g "$repo" remote add origin "$bare"
  g "$repo" push -q -u origin main
  printf '%s' "$repo"
}

report() { fixture "$1"; bash "$CLOSEOUT" --repo-root "$1" "${@:2}" 2>&1; }

# ---------------------------------------------------------------------------
# (a) It reports exactly what is there
# ---------------------------------------------------------------------------
A="$(mk_repo a)"
printf 'edit\n' >> "$A/README.md"
printf 'new1\n' > "$A/loose1.txt"
printf 'new2\n' > "$A/loose2.txt"
g "$A" commit -q -am "local change 1"
printf 'edit2\n' >> "$A/README.md"
g "$A" commit -q -am "local change 2"
printf 'dirty\n' >> "$A/README.md"

OUT_A="$(report "$A")"

if printf '%s' "$OUT_A" | grep -q "1 dirty files (primary), 2 untracked"; then
  pass "a1 reports exactly the dirty and untracked counts it was given"
else
  fail "a1 dirty/untracked counts wrong: $(printf '%s' "$OUT_A" | grep 'hygiene-local')"
fi

if printf '%s' "$OUT_A" | grep -q "2 ahead / 0 behind origin/main"; then
  pass "a2 reports exactly the unpushed commit count"
else
  fail "a2 unpushed count wrong: $(printf '%s' "$OUT_A" | grep 'hygiene-local')"
fi

if printf '%s' "$OUT_A" | grep -q "residue | loose1.txt" \
   && printf '%s' "$OUT_A" | grep -q "residue | loose2.txt"; then
  pass "a3 proposes a residue row for each path that maps to no open artifact"
else
  fail "a3 residue rows missing for untracked paths"
fi

if printf '%s' "$OUT_A" | grep -qE '^  \| residue-[0-9]+ \|.*\| open \|'; then
  pass "a4 the proposed row is register-shaped, so it can be pasted straight in"
else
  fail "a4 proposed residue row is not in register table form"
fi

# ---------------------------------------------------------------------------
# (b) The safety surface: no bypass flags anywhere
#
# Asserted by PARSER REJECTION, not by scanning the help text. The help text
# necessarily contains the words "--force", "--skip", and "--ignore" in order
# to state that they do not exist, so a text scan would fail on the very
# sentence that documents the guarantee.
# ---------------------------------------------------------------------------
HELP="$(bash "$CLOSEOUT" --help 2>&1)"

bypass_rejected=true
for flag in --force --skip --ignore; do
  if bash "$CLOSEOUT" --repo-root "$A" "$flag" >/dev/null 2>&1; then
    bypass_rejected=false
    fail "b1 $flag was ACCEPTED"
  fi
done
[[ "$bypass_rejected" == true ]] && pass "b1 --force, --skip, and --ignore are all rejected (there is no bypass layer)"

if printf '%s' "$HELP" | grep -qE '^ *--(force|skip|ignore)\b'; then
  fail "b2 --help offers a bypass flag in its Args list"
else
  pass "b2 --help offers no bypass flag in its Args list"
fi

if printf '%s' "$HELP" | grep -q "absence of \`--apply\` is"; then
  pass "b3 --help states which flag governs execution, so --dry-run is not read as the mechanism"
else
  fail "b3 --help does not say that absence of --apply is the mechanism"
fi

# ---------------------------------------------------------------------------
# (c) Refusals name their remediation
# ---------------------------------------------------------------------------
C="$(mk_repo c)"
g "$C" checkout -q -b feature/unmerged
printf 'unique\n' > "$C/unique.txt"
g "$C" add -A
g "$C" commit -q -m "unique commit"
g "$C" checkout -q main
printf 'stashme\n' >> "$C/README.md"
g "$C" stash -q
OUT_C="$(report "$C")"

if printf '%s' "$OUT_C" | grep -q "has-unique-commits  feature/unmerged"; then
  pass "c1 an unmerged branch is classified has-unique-commits, not merge-able"
else
  fail "c1 unmerged branch misclassified"
fi

if printf '%s' "$OUT_C" | grep -A1 "has-unique-commits  feature/unmerged" | grep -q "remediation:"; then
  pass "c2 the unmerged-branch refusal names its remediation"
else
  fail "c2 unmerged-branch refusal has no remediation line"
fi

if printf '%s' "$OUT_C" | grep -q "closeout NEVER drops a stash"; then
  pass "c3 a stash is reported and explicitly never dropped"
else
  fail "c3 stash not reported as never-dropped"
fi

report "$C" --apply >/dev/null 2>&1
if [[ "$(g "$C" stash list | wc -l | tr -d ' ')" == "1" ]]; then
  pass "c4 --apply left the stash untouched"
else
  fail "c4 --apply disturbed the stash"
fi
if g "$C" show-ref --verify --quiet refs/heads/feature/unmerged; then
  pass "c5 --apply left the unmerged branch intact"
else
  fail "c5 --apply deleted an unmerged branch"
fi

# ---------------------------------------------------------------------------
# (d) Positive control: the delete path is live
# ---------------------------------------------------------------------------
D="$(mk_repo d)"
g "$D" branch feature/merged
OUT_D="$(report "$D")"
if printf '%s' "$OUT_D" | grep -q "merge-able          feature/merged"; then
  pass "d0 a fully-merged branch is classified merge-able"
else
  fail "d0 merged branch misclassified"
fi
report "$D" --apply >/dev/null 2>&1
if g "$D" show-ref --verify --quiet refs/heads/feature/merged; then
  fail "d1 --apply did NOT delete a merge-able branch (every refusal case below would be vacuous)"
else
  pass "d1 --apply deletes a merge-able branch (positive control for the refusals)"
fi

D2="$(mk_repo d2)"
g "$D2" branch feature/merged
report "$D2" >/dev/null 2>&1
report "$D2" --dry-run >/dev/null 2>&1
if g "$D2" show-ref --verify --quiet refs/heads/feature/merged; then
  pass "d2 the default and --dry-run both change nothing (--dry-run is a synonym, not a mode)"
else
  fail "d2 a run without --apply deleted a branch"
fi

# ---------------------------------------------------------------------------
# (e) The writer-lease, re-checked at ACTION TIME
# ---------------------------------------------------------------------------
E="$(mk_repo e)"
g "$E" branch feature/merged
BUBBLES_REPO_ROOT="$E" bash "$LEASES" acquire --purpose closeout-selftest >/dev/null 2>&1

OUT_E="$(report "$E")"
if [[ -n "$OUT_E" ]] && printf '%s' "$OUT_E" | grep -q "Session closeout"; then
  pass "e1a report mode succeeds under a live lease (reading state is safe)"
else
  fail "e1a report mode failed under a live lease"
fi
# Output is CAPTURED before it is searched. `report ... | grep -q` would let
# grep close the pipe on its first match, the producer would take SIGPIPE, and
# `pipefail` would mark the whole pipeline failed — turning a real refusal into
# a reported failure, and a real failure-to-refuse into a false pass.
APPLY_E="$(report "$E" --apply)"
if printf '%s' "$APPLY_E" | grep -q "SKIP  feature/merged"; then
  pass "e1b --apply refuses while a lease is live"
else
  fail "e1b --apply did not refuse under a live lease"
fi
if g "$E" show-ref --verify --quiet refs/heads/feature/merged; then
  pass "e1c the lease-covered branch survived --apply"
else
  fail "e1c --apply deleted a branch while a lease was live"
fi

# The case that separates an action-time check from a snapshot-time one: the
# report is produced with NO lease (so the snapshot says "safe to delete"), the
# lease is taken afterwards, and only then does apply run.
E2="$(mk_repo e2)"
g "$E2" branch feature/merged
OUT_E2="$(report "$E2")"
if printf '%s' "$OUT_E2" | grep -q "merge-able          feature/merged"; then
  pass "e2a the report saw NO lease and classified the branch merge-able"
else
  fail "e2a fixture setup wrong — the report did not see a clean, deletable branch"
fi

BUBBLES_REPO_ROOT="$E2" bash "$LEASES" acquire --purpose closeout-selftest >/dev/null 2>&1
APPLY_E2="$(report "$E2" --apply)"
if printf '%s' "$APPLY_E2" | grep -q "SKIP  feature/merged"; then
  pass "e2b --apply refuses a lease acquired AFTER the report (the check is at action time)"
else
  fail "e2b --apply used the stale snapshot and ignored a lease taken after the report"
fi
if g "$E2" show-ref --verify --quiet refs/heads/feature/merged; then
  pass "e2c the branch survived, so a concurrent session's work was not disturbed"
else
  fail "e2c --apply deleted a branch a concurrent session had just leased"
fi

# ---------------------------------------------------------------------------
# (f) The session archive: closing the reader-without-writer path
# ---------------------------------------------------------------------------
F="$(mk_repo f)"
printf '{"sessionId":"sess-closeout-test","agent":"bubbles.goal","status":"in_progress"}\n' \
  > "$F/.specify/memory/bubbles.session.json"
report "$F" >/dev/null 2>&1
if [[ -f "$F/.specify/memory/sessions/sess-closeout-test.json" ]]; then
  fail "f1 the report wrote a session archive (report mode must not mutate)"
else
  pass "f1 report mode writes no session archive"
fi
report "$F" --apply >/dev/null 2>&1
if [[ -f "$F/.specify/memory/sessions/sess-closeout-test.json" ]]; then
  pass "f2 --apply archives the session where trajectory-inspector.sh already looks"
else
  fail "f2 --apply did not write .specify/memory/sessions/<id>.json"
fi
if command -v jq >/dev/null 2>&1; then
  if [[ "$(jq -r '.sessionId' "$F/.specify/memory/sessions/sess-closeout-test.json" 2>/dev/null)" == "sess-closeout-test" ]]; then
    pass "f3 the archive carries the sessionId the inspector matches on"
  else
    fail "f3 the archive is not readable by the inspector's sessionId lookup"
  fi
fi

# ---------------------------------------------------------------------------
# (g) Degradation
# ---------------------------------------------------------------------------
NG="$TMP_ROOT/not-a-repo"
mkdir -p "$NG"
if bash "$CLOSEOUT" --repo-root "$NG" 2>&1 | grep -q "not a git repository"; then
  pass "g1 a non-git root is named, not silently treated as clean"
else
  fail "g1 a non-git root was not reported"
fi
if bash "$CLOSEOUT" --repo-root "$NG" >/dev/null 2>&1; then
  pass "g2 a non-git root still exits 0 (this is a report, not a gate)"
else
  fail "g2 a non-git root produced a non-zero exit"
fi

# An empty --repo-root MUST be a usage error. Falling back to the walk-upward
# default would bind a MUTATING command to whatever repository the caller
# happens to be standing in — which is exactly how this selftest, in an earlier
# form, silently committed to and branched the framework's own checkout.
if bash "$CLOSEOUT" --repo-root "" >/dev/null 2>&1; then
  fail "g3 an empty --repo-root was accepted and fell back to the ambient repository"
else
  pass "g3 an empty --repo-root is a usage error, never a silent bind to the ambient repository"
fi

echo
if [[ "$FAIL" -gt 0 ]]; then
  echo "closeout-report-selftest: $FAIL of $((PASS + FAIL)) cases FAILED."
  exit 1
fi
echo "closeout-report-selftest: all $PASS cases passed."
exit 0
