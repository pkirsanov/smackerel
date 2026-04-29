#!/usr/bin/env bash
#
# BUG-031-001 / SCN-031-BUG001-A1 — `--volumes` is honored when placed after
# the command token.
#
# This regression exercises the smackerel.sh argv loop end-to-end against the
# real Docker volume surface. On pre-fix HEAD the parser only consumed flags
# BEFORE the command token, so `./smackerel.sh --env test down --volumes`
# silently dropped the post-command `--volumes`, leaving the named test
# PostgreSQL volume on disk after teardown. After the fix, flags must be
# accepted in any position relative to the command token.
#
# Adversarial input (the broken position): `--volumes` placed AFTER `down`.
# This script MUST FAIL on pre-fix HEAD and PASS post-fix.
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

TEST_VOLUME="smackerel-test-postgres-data"

# Pre-clean: ensure no stale volume from a prior run influences the assertion.
docker volume rm -f "$TEST_VOLUME" >/dev/null 2>&1 || true
timeout 60 "$REPO_DIR/smackerel.sh" --env test down --volumes >/dev/null 2>&1 || true

# Bring the test stack up. This creates the named test postgres volume.
"$REPO_DIR/smackerel.sh" --env test up

# Confirm the volume exists before we try to remove it (otherwise the
# assertion below would be tautological).
if ! docker volume ls --format '{{.Name}}' | grep -qx "$TEST_VOLUME"; then
    echo "FAIL: precondition not met — $TEST_VOLUME was not created by 'up'" >&2
    exit 1
fi

# Adversarial invocation: `--volumes` placed AFTER the command token.
# Pre-fix HEAD: parser breaks on `down`, never sees `--volumes`, volume survives.
# Post-fix:     parser is positional-agnostic, --volumes is honored, volume removed.
"$REPO_DIR/smackerel.sh" --env test down --volumes

# Assert the volume was actually removed.
if docker volume ls --format '{{.Name}}' | grep -qx "$TEST_VOLUME"; then
    echo "FAIL: $TEST_VOLUME still present after 'down --volumes' (post-command form)" >&2
    docker volume ls | grep -E "smackerel-test|$TEST_VOLUME" >&2 || true
    exit 1
fi

echo "PASS: post-command --volumes removed $TEST_VOLUME"
