#!/usr/bin/env bash
# case-collision-guard-selftest.sh
#
# Hermetic selftest for case-collision-guard.sh. Stages throwaway git repos
# under a temp dir and asserts the guard's contract:
#
#   - clean fixture    -> exit 0, "no case-insensitive duplicate" reported
#   - collision fixture-> exit 1, both colliding paths listed
#   - non-git fixture  -> exit 0, "not a git work tree" reported
#
# The collision fixture injects a SECOND index entry that differs only by case
# (Foo.md alongside foo.md) via `git update-index --cacheinfo`. That is the only
# way to reproduce the bug on a case-insensitive filesystem (macOS/Windows),
# where two case-variant names cannot both exist as physical files — exactly the
# real-world shape (templates/AGENTS.md.tmpl vs templates/agents.md.tmpl) this
# guard exists to catch.
#
# Cleans up on exit. Exit 0 = all assertions pass; exit 1 = one or more failed.
#
# Origin: IMP-017 (template case-collision fix + prevention).

set -euo pipefail

# Hermetic git: ignore the operator's global/system config + never prompt.
export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_SYSTEM=/dev/null
export GIT_TERMINAL_PROMPT=0
unset GIT_DIR GIT_WORK_TREE 2>/dev/null || true

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/case-collision-guard.sh"

if [[ ! -f "$TARGET" ]]; then
  echo "[selftest case-collision-guard] FAIL: target script missing at $TARGET" >&2
  exit 1
fi

if ! command -v git >/dev/null 2>&1; then
  echo "[selftest case-collision-guard] SKIP (git not installed)"
  exit 0
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

failures=0
pass() { echo "  PASS: $1"; }
fail() {
  echo "  FAIL: $1"
  failures=$((failures + 1))
}

# Initialize a throwaway git repo (never committed — the guard reads the index
# via `git ls-files`, so staging is enough).
seed_git_repo() {
  local root="$1"
  mkdir -p "$root"
  git -C "$root" init -q
  git -C "$root" config user.email "selftest@example.invalid"
  git -C "$root" config user.name "case-collision-selftest"
}

# --- Clean fixture --------------------------------------------------------

clean_root="$TMPDIR/repo-clean"
seed_git_repo "$clean_root"
mkdir -p "$clean_root/src" "$clean_root/docs"
printf 'readme\n' >"$clean_root/README.md"
printf 'main\n' >"$clean_root/src/main.sh"
printf 'guide\n' >"$clean_root/docs/GUIDE.md"
git -C "$clean_root" add -A

set +e
clean_log="$TMPDIR/clean.log"
bash "$TARGET" --repo-root "$clean_root" >"$clean_log" 2>&1
clean_rc=$?
set -e

if [[ "$clean_rc" -eq 0 ]]; then
  pass "clean fixture exits 0"
else
  fail "clean fixture expected exit 0, got $clean_rc"
  sed -n '1,40p' "$clean_log"
fi

if grep -Fq "no case-insensitive duplicate" "$clean_log"; then
  pass "clean fixture reports no duplicates"
else
  fail "clean fixture missing 'no case-insensitive duplicate'"
  sed -n '1,40p' "$clean_log"
fi

# --- Collision fixture ----------------------------------------------------

dup_root="$TMPDIR/repo-dup"
seed_git_repo "$dup_root"
printf 'shared blob\n' >"$dup_root/foo.md"
git -C "$dup_root" add foo.md
dup_blob="$(git -C "$dup_root" rev-parse :foo.md)"
# Inject a case-variant index entry pointing at the SAME blob, with ignorecase
# forced off so git records the distinct-case path (no second physical file
# needed — impossible on a case-insensitive FS anyway).
git -C "$dup_root" -c core.ignorecase=false update-index --add --cacheinfo 100644 "$dup_blob" Foo.md

set +e
dup_log="$TMPDIR/dup.log"
bash "$TARGET" --repo-root "$dup_root" >"$dup_log" 2>&1
dup_rc=$?
set -e

if [[ "$dup_rc" -eq 1 ]]; then
  pass "collision fixture exits 1"
else
  fail "collision fixture expected exit 1, got $dup_rc"
  sed -n '1,40p' "$dup_log"
fi

if grep -Fq "case-insensitive duplicate (2 tracked paths" "$dup_log" &&
  grep -Fq "foo.md" "$dup_log" && grep -Fq "Foo.md" "$dup_log"; then
  pass "collision fixture lists both colliding paths"
else
  fail "collision fixture missing the Foo.md/foo.md collision group"
  sed -n '1,40p' "$dup_log"
fi

# --- Non-git fixture ------------------------------------------------------

nongit_root="$TMPDIR/not-a-repo"
mkdir -p "$nongit_root"
printf 'x\n' >"$nongit_root/file.txt"

set +e
nongit_log="$TMPDIR/nongit.log"
bash "$TARGET" --repo-root "$nongit_root" >"$nongit_log" 2>&1
nongit_rc=$?
set -e

if [[ "$nongit_rc" -eq 0 ]]; then
  pass "non-git fixture exits 0 (nothing to scan)"
else
  fail "non-git fixture expected exit 0, got $nongit_rc"
  sed -n '1,40p' "$nongit_log"
fi

if grep -Fq "not a git work tree" "$nongit_log"; then
  pass "non-git fixture reports it is not a work tree"
else
  fail "non-git fixture missing 'not a git work tree'"
  sed -n '1,40p' "$nongit_log"
fi

# --- Summary --------------------------------------------------------------

echo
if [[ "$failures" -eq 0 ]]; then
  echo "[selftest case-collision-guard] OK — all assertions passed."
  exit 0
fi
echo "[selftest case-collision-guard] FAIL — $failures assertion(s) failed." >&2
exit 1
