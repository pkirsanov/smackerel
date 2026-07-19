#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
source "$REPO_ROOT/scripts/lib/runtime.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

mkdir -p "$TMP/repo"
git -C "$TMP/repo" init -q
git -C "$TMP/repo" config user.name "Smackerel Test"
git -C "$TMP/repo" config user.email "smackerel-test@example.invalid"
printf '%s\n' "fixture" >"$TMP/repo/fixture.txt"
git -C "$TMP/repo" add fixture.txt
git -C "$TMP/repo" commit -q -m "fixture"
git -C "$TMP/repo" worktree add -q -b linked-fixture "$TMP/linked"

smackerel_prepare_tooling_git_mount_args "$TMP/repo"
[[ ${#SMACKEREL_TOOLING_GIT_MOUNT_ARGS[@]} -eq 0 ]] \
  || fail "primary worktree unexpectedly requested an extra Git metadata mount"

smackerel_prepare_tooling_git_mount_args "$TMP/linked"
[[ ${#SMACKEREL_TOOLING_GIT_MOUNT_ARGS[@]} -eq 6 ]] \
  || fail "linked worktree mount argv length=${#SMACKEREL_TOOLING_GIT_MOUNT_ARGS[@]} (want 6)"

git_common_dir="$(git -C "$TMP/linked" rev-parse --git-common-dir)"
[[ "${SMACKEREL_TOOLING_GIT_MOUNT_ARGS[0]}" == "-v" ]] \
  || fail "linked worktree mount is missing the -v flag"
[[ "${SMACKEREL_TOOLING_GIT_MOUNT_ARGS[1]}" == "$git_common_dir:$git_common_dir:ro" ]] \
  || fail "linked worktree common Git directory is not mounted read-only at its original path"
[[ "${SMACKEREL_TOOLING_GIT_MOUNT_ARGS[2]}" == "-v" ]] \
  || fail "linked worktree reciprocal Git file is missing the mount flag"
[[ "${SMACKEREL_TOOLING_GIT_MOUNT_ARGS[3]}" == "$TMP/linked/.git:$TMP/linked/.git:ro" ]] \
  || fail "linked worktree reciprocal Git file is not mounted read-only at its original path"
[[ "${SMACKEREL_TOOLING_GIT_MOUNT_ARGS[4]}" == "-e" ]] \
  || fail "linked worktree mount is missing the environment flag"
[[ "${SMACKEREL_TOOLING_GIT_MOUNT_ARGS[5]}" == "GIT_OPTIONAL_LOCKS=0" ]] \
  || fail "linked worktree tooling does not disable optional Git locks"

echo "PASS: linked worktree tooling mounts common Git metadata read-only"
