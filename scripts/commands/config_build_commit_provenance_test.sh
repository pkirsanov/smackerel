#!/usr/bin/env bash
#
# redteam F6 / BUG-029-008 — build-commit provenance regression.
#
# Background:
#   scripts/commands/config.sh resolves SMACKEREL_COMMIT into the generated
#   config/generated/<env>.env, which docker-compose.yml passes as the
#   COMMIT_HASH build-arg (→ OCI org.opencontainers.image.revision + the core
#   `commitHash` ldflag → the app's reported commit_hash). Before this fix the
#   resolution was:
#
#       if [[ -z "${SMACKEREL_COMMIT+set}" ]]; then SMACKEREL_COMMIT="unknown"; fi
#
#   CI exports SMACKEREL_COMMIT, so CI images were fine. But the local-operator
#   (evo-x2) build path builds ON the host with no CI env, so SMACKEREL_COMMIT
#   fell through to the literal "unknown" — which is exactly what the redteam
#   observed on the LIVE prod images (revision=unknown, commit_hash=unknown).
#
# Fix:
#   When SMACKEREL_COMMIT is unset, config.sh now derives the source SHA from
#   the git working tree (`git rev-parse --short=12 HEAD`, `-dirty` suffix when
#   the tree is dirty) so a locally-built image is self-identifying. It falls
#   back to "unknown" ONLY outside a git checkout. CI's shell-env export still
#   wins (the `[[ -z ... ]]` arm runs only when unset).
#
# Sub-tests:
#   1. (core) SMACKEREL_COMMIT UNSET, TARGET_ENV=dev against the live yaml
#      emits a REAL 12-hex SHA (optionally `-dirty`) into dev.env — never the
#      literal "unknown".
#   2. (precedence) SMACKEREL_COMMIT=<sentinel> exported is passed through
#      verbatim (CI/shell-env override preserved).
#
# Adversarial proof: reverting the git-derivation arm in
# scripts/commands/config.sh (back to SMACKEREL_COMMIT="unknown") makes
# Sub-test 1 fail because dev.env reverts to SMACKEREL_COMMIT=unknown.
#
# Output isolation: config.sh honors SMACKEREL_GENERATED_DIR for the env file;
# this test points it at a private temp dir so it never touches the operator's
# working state and never races with other config-generating tests under
# `go test ./...`.
#
# This script is invoked by
# internal/config/sst_loader_build_commit_provenance_test.go under
# `./smackerel.sh test unit --go` so it runs in the standard unit-tier Go test
# pipeline as well as standalone.

set -uo pipefail

# REPO_ROOT is set by the Go driver; fall back to the path-from-this-file
# computation when invoked standalone from anywhere.
if [[ -z "${REPO_ROOT:-}" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

LIVE_YAML="$REPO_ROOT/config/smackerel.yaml"
CONFIG_SH="$REPO_ROOT/scripts/commands/config.sh"

if [[ ! -f "$LIVE_YAML" ]]; then
  echo "FATAL: $LIVE_YAML not found" >&2
  exit 1
fi
if [[ ! -f "$CONFIG_SH" ]]; then
  echo "FATAL: $CONFIG_SH not found" >&2
  exit 1
fi

GENERATED_DIR="$(mktemp -d)"
export SMACKEREL_GENERATED_DIR="$GENERATED_DIR"

cleanup() {
  local rc=$?
  rm -rf "$GENERATED_DIR"
  exit "$rc"
}
trap cleanup EXIT INT TERM

FAIL=0

echo "--- Sub-test 1 (core): SMACKEREL_COMMIT unset -> dev.env carries a real git SHA ---"
# env -u guarantees SMACKEREL_COMMIT is unset for this generation regardless of
# the ambient shell / CI env, isolating the git-derivation arm.
SUB1_OUT="$(env -u SMACKEREL_COMMIT bash "$CONFIG_SH" --env dev --config "$LIVE_YAML" 2>&1 || true)"
SUB1_COMMIT="$(grep '^SMACKEREL_COMMIT=' "$GENERATED_DIR/dev.env" 2>/dev/null | head -1 | cut -d= -f2-)"
if [[ "$SUB1_COMMIT" =~ ^[0-9a-f]{12}(-dirty)?$ ]]; then
  echo "PASS: dev.env SMACKEREL_COMMIT=$SUB1_COMMIT (real source SHA, self-identifying image)"
else
  echo "FAIL: dev.env SMACKEREL_COMMIT is not a real git SHA — actual: '${SUB1_COMMIT:-<none emitted>}'"
  echo "      redteam F6 / BUG-029-008 reintroduced: the git-derivation arm in scripts/commands/config.sh"
  echo "      is missing or broken, so a locally-built (local-operator / evo-x2) image is stamped the opaque"
  echo "      'unknown' for org.opencontainers.image.revision + commit_hash instead of its source SHA."
  echo "      --- config.sh output ---"
  echo "$SUB1_OUT" | sed 's/^/        /'
  FAIL=$((FAIL + 1))
fi

echo "--- Sub-test 2 (precedence): exported SMACKEREL_COMMIT wins (CI/shell-env override) ---"
SENTINEL="cafef00dba11"
SUB2_OUT="$(SMACKEREL_COMMIT="$SENTINEL" bash "$CONFIG_SH" --env dev --config "$LIVE_YAML" 2>&1 || true)"
SUB2_COMMIT="$(grep '^SMACKEREL_COMMIT=' "$GENERATED_DIR/dev.env" 2>/dev/null | head -1 | cut -d= -f2-)"
if [[ "$SUB2_COMMIT" == "$SENTINEL" ]]; then
  echo "PASS: dev.env SMACKEREL_COMMIT=$SUB2_COMMIT (shell-env / CI override preserved)"
else
  echo "FAIL: exported SMACKEREL_COMMIT override was not preserved — expected '$SENTINEL', got '${SUB2_COMMIT:-<none emitted>}'"
  echo "      The git-derivation arm must run ONLY when SMACKEREL_COMMIT is unset; a CI export MUST win."
  echo "      --- config.sh output ---"
  echo "$SUB2_OUT" | sed 's/^/        /'
  FAIL=$((FAIL + 1))
fi

echo ""
if [[ $FAIL -gt 0 ]]; then
  echo "FAILURES: $FAIL sub-test(s) failed"
  exit 1
fi

echo "All BUG-029-008 build-commit provenance sub-tests passed"
exit 0
