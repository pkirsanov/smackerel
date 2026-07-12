#!/usr/bin/env bash
#
# Spec 052 Scope 3 — T-052-013 / SCN-052-S06 / BS-052-006
#
# Long-lived BS-052-006 regression for spec 051 FR-051-005 on the
# env-override path.
#
# Background:
#   Spec 052 Scope 2 reordered the FR-051-005 dev-default-Postgres-password
#   gate inside scripts/commands/config.sh so that placeholder mode for
#   production-class targets (self-hosted) short-circuits the yaml-default
#   path BEFORE the dev-default check fires. The reorder is correct for
#   the placeholder-mode happy path (Scope 2 BS-052-005) but it MUST NOT
#   weaken the original FR-051-005 contract on the env-override path:
#   when the operator explicitly overrides POSTGRES_PASSWORD in the
#   environment, the literal value beats the yaml AND skips placeholder
#   mode AND must still pass the dev-default rejection.
#
# Spec source:
#   - specs/052-bundle-secret-injection-contract/spec.md FR-052-010
#   - specs/052-bundle-secret-injection-contract/design.md §4 step 3
#     ("short-circuit preservation")
#   - specs/052-bundle-secret-injection-contract/design.md §8 Test Plan
#     row 5 alternative
#   - specs/052-bundle-secret-injection-contract/scopes.md Scope 3
#     T-052-013 row + DoD A3/B7
#   - specs/051-deployment-secret-auth-contract/spec.md FR-051-005
#     (dev-default rejection invariant)
#   - specs/051-deployment-secret-auth-contract/spec.md FR-051-007
#     (redaction contract — never echo the literal value)
#
# Adversarial proof:
#   Reverting Scope 2's reorder so the placeholder-mode short-circuit
#   ALSO covers env-override values would still PASS Sub-test 1 (because
#   the env-override would silently be replaced by a placeholder in the
#   bundle), so Sub-test 1 alone would be tautological. Sub-test 1 here
#   is paired with the existing scripts/commands/config_secret_rejection_test.sh
#   Sub-test 1 (which uses the same env-override drive) and with the
#   bundle absence assertion below: if a bundle ever materialized at the
#   expected path under POSTGRES_PASSWORD=smackerel TARGET_ENV=self-hosted,
#   that proves the dev-default gate was bypassed (the loader is
#   contracted to fail BEFORE any bundle bytes are written).
#
# Sub-tests:
#   1. POSTGRES_PASSWORD=smackerel TARGET_ENV=self-hosted against the live
#      smackerel.yaml MUST exit non-zero. The loader output MUST name
#      "infrastructure.postgres.password" AND reference "spec 051" AND
#      MUST NOT echo the literal value "smackerel" as a password value
#      (FR-051-007 redaction).
#   2. The loader MUST NOT have written a bundle tarball at the expected
#      output path (additional defense-in-depth: the env-override path
#      fails BEFORE any file is written).
#
# Output isolation:
#   The smackerel.sh CLI's `config generate` writes
#   $REPO_ROOT/config/generated/${TARGET_ENV}.env unconditionally. To
#   keep the operator's working state untouched, this script backs up any
#   pre-existing dev.env / test.env / self-hosted.env files BEFORE running
#   and restores them after, mirroring the same isolation strategy used
#   by config_self_hosted_runtime_env_test.sh.
#
# This script is invoked by `./smackerel.sh test unit` (via the bash
# unit harness) so it runs in the standard unit-tier pipeline as well as
# standalone for ad-hoc adversarial verification.

set -uo pipefail

# REPO_ROOT can be overridden by a Go test harness; falls back to the
# path-from-this-file computation when invoked standalone.
if [[ -z "${REPO_ROOT:-}" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

SMACKEREL_SH="$REPO_ROOT/smackerel.sh"
GENERATED_DIR="$REPO_ROOT/config/generated"

if [[ ! -x "$SMACKEREL_SH" ]] && [[ ! -f "$SMACKEREL_SH" ]]; then
  echo "FATAL: cannot locate smackerel.sh under $REPO_ROOT" >&2
  exit 1
fi

# -----------------------------------------------------------------------
# Backup pre-existing generated env files so the operator's state is
# never disturbed by this regression run.
# -----------------------------------------------------------------------
BACKUP_DIR="$(mktemp -d)"
for env_name in dev test self-hosted; do
  src="$GENERATED_DIR/${env_name}.env"
  if [[ -f "$src" ]]; then
    cp "$src" "$BACKUP_DIR/${env_name}.env"
  fi
done

# Use a temp output dir for the bundle tarball so the assertion that NO
# bundle was written can use a clean reference directory.
BUNDLE_OUT_DIR="$(mktemp -d)"

restore_generated() {
  local rc=$?
  for env_name in dev test self-hosted; do
    src="$BACKUP_DIR/${env_name}.env"
    dst="$GENERATED_DIR/${env_name}.env"
    if [[ -f "$src" ]]; then
      cp "$src" "$dst"
    else
      # The file did not exist before our run; remove any artifact we
      # may have created. (The env-override path is contracted to fail
      # BEFORE any output file is written, but harden against partial
      # writes if a regression breaks that contract.)
      rm -f "$dst"
    fi
  done
  rm -rf "$BACKUP_DIR"
  rm -rf "$BUNDLE_OUT_DIR"
  exit "$rc"
}
trap restore_generated EXIT

failures=0

# -----------------------------------------------------------------------
# Sub-test 1: env-override dev-default value rejected for self-hosted AND
# error names infrastructure.postgres.password AND spec 051 AND does
# NOT echo the literal value "smackerel" as a password value.
# -----------------------------------------------------------------------
echo "--- Sub-test 1 (T-052-013 / SCN-052-S06 / BS-052-006): env-override dev-default rejected for self-hosted ---"

override_output="$(POSTGRES_PASSWORD=smackerel bash "$SMACKEREL_SH" config generate \
  --env self-hosted --bundle --output-dir "$BUNDLE_OUT_DIR" 2>&1)"
override_exit=$?

if [[ "$override_exit" -eq 0 ]]; then
  echo "FAIL: SST loader returned exit 0 for POSTGRES_PASSWORD=smackerel + TARGET_ENV=self-hosted (expected non-zero)"
  echo "----- captured output -----"
  echo "$override_output"
  echo "----- end output -----"
  failures=$((failures + 1))
else
  echo "PASS: SST loader refused env-override dev-default with exit code $override_exit"
fi

if ! echo "$override_output" | grep -q "infrastructure.postgres.password"; then
  echo "FAIL: SST loader output does NOT name infrastructure.postgres.password"
  failures=$((failures + 1))
else
  echo "PASS: SST loader output names infrastructure.postgres.password"
fi

if ! echo "$override_output" | grep -q "spec 051"; then
  echo "FAIL: SST loader output does NOT reference spec 051"
  failures=$((failures + 1))
else
  echo "PASS: SST loader output references spec 051"
fi

# Redaction assertion (FR-051-007): the output MUST NOT contain the
# literal value "smackerel" as a free-standing password value. The
# message is allowed to mention the project name "smackerel" in
# attribution / branding (e.g. "smackerel-core") — but never as a
# credential. The pattern below mirrors config_secret_rejection_test.sh
# Sub-test 1's redaction grep so both regressions enforce the same
# contract.
if echo "$override_output" | grep -qE '(POSTGRES_PASSWORD|password)[[:space:]=:]+["'\''[:space:]]*smackerel[[:space:]"'\''$]'; then
  echo "FAIL: SST loader output echoes dev-default value 'smackerel' as a password value (FR-051-007 redaction violation)"
  failures=$((failures + 1))
else
  echo "PASS: SST loader output does not echo 'smackerel' as a password value (FR-051-007 preserved)"
fi

# -----------------------------------------------------------------------
# Sub-test 2: defense-in-depth — assert NO bundle tarball was written.
# The dev-default gate is contracted to fail BEFORE any output file is
# created. If a tarball materializes at the expected path, the loader
# wrote bundle bytes despite the rejection — a regression class above
# and beyond the exit-code contract.
# -----------------------------------------------------------------------
echo "--- Sub-test 2 (T-052-013 defense-in-depth): no bundle tarball written on rejection ---"

# The bundle filename pattern is config-bundle-<env>-<source-sha>.tar.gz.
# The source-sha defaults to a git lookup when --source-sha is not
# provided; use a glob to catch any sha. Zero matches is the contract.
shopt -s nullglob
written_bundles=("$BUNDLE_OUT_DIR"/config-bundle-self-hosted-*.tar.gz)
shopt -u nullglob

if [[ "${#written_bundles[@]}" -eq 0 ]]; then
  echo "PASS: no bundle tarball was written to $BUNDLE_OUT_DIR (FR-051-005 fail-before-write preserved)"
else
  echo "FAIL: ${#written_bundles[@]} bundle tarball(s) written to $BUNDLE_OUT_DIR despite rejection:"
  for f in "${written_bundles[@]}"; do
    echo "       $f"
  done
  failures=$((failures + 1))
fi

# -----------------------------------------------------------------------
# Result.
# -----------------------------------------------------------------------
echo
if [[ "$failures" -eq 0 ]]; then
  echo "RESULT: PASS — spec 052 T-052-013 / SCN-052-S06 / BS-052-006 regression intact"
  exit 0
else
  echo "RESULT: FAIL — $failures assertion(s) failed; spec 052 T-052-013 / SCN-052-S06 / BS-052-006 regression broken"
  exit 1
fi
