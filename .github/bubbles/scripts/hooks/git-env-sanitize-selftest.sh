#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SANITIZER="$SCRIPT_DIR/git-env-sanitize.sh"
PRE_PUSH="$SCRIPT_DIR/pre-push.sh"

[[ -f "$SANITIZER" ]] || { echo "git-env-sanitize-selftest: missing $SANITIZER" >&2; exit 2; }
[[ -f "$PRE_PUSH" ]] || { echo "git-env-sanitize-selftest: missing $PRE_PUSH" >&2; exit 2; }

grep -Fq 'source "$SCRIPT_DIR/hooks/git-env-sanitize.sh"' "$PRE_PUSH" \
  || { echo "FAIL: pre-push hook does not source the Git environment sanitizer" >&2; exit 1; }
grep -Fq 'bubbles_unset_git_local_env' "$PRE_PUSH" \
  || { echo "FAIL: pre-push hook does not invoke Git environment sanitization" >&2; exit 1; }

TEST_ROOT_BASE="${HOME}/.cache/bubbles-git-env-sanitize-selftest"
mkdir -p "$TEST_ROOT_BASE"
TEST_ROOT="$(mktemp -d "$TEST_ROOT_BASE/run.XXXXXX")"
trap 'rm -rf "$TEST_ROOT"' EXIT INT TERM

OUTER_REPO="$TEST_ROOT/outer"
NESTED_REPO="$TEST_ROOT/nested"
mkdir -p "$OUTER_REPO" "$NESTED_REPO"
git -C "$OUTER_REPO" init -q
git -C "$NESTED_REPO" init -q

EXPECTED_ROOT="$(git -C "$NESTED_REPO" rev-parse --show-toplevel)"
ACTUAL_ROOT="$({
  cd "$NESTED_REPO"
  GIT_DIR="$OUTER_REPO/.git" \
  GIT_WORK_TREE="$OUTER_REPO" \
  GIT_INDEX_FILE="$OUTER_REPO/.git/index" \
    bash -c '
      set -euo pipefail
      source "$1"
      bubbles_unset_git_local_env
      [[ -z "${GIT_DIR+x}" ]]
      [[ -z "${GIT_WORK_TREE+x}" ]]
      [[ -z "${GIT_INDEX_FILE+x}" ]]
      git rev-parse --show-toplevel
    ' bash "$SANITIZER"
})"

if [[ "$ACTUAL_ROOT" != "$EXPECTED_ROOT" ]]; then
  echo "FAIL: nested Git command resolved '$ACTUAL_ROOT', expected '$EXPECTED_ROOT'" >&2
  exit 1
fi

echo "PASS: source pre-push hook invokes the Git environment sanitizer"
echo "PASS: hook-local Git variables are removed before nested fixture commands"
echo "PASS: nested fixture resolves its own repository after sanitization"
echo "git-env-sanitize-selftest: PASS"
