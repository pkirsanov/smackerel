#!/usr/bin/env bash
# Spec 051 SCN-051-S02 / FR-051-005 — SST-loader dev-default rejection
# adversarial test, evolved by spec 052 to drive the dev-default check
# via the POSTGRES_PASSWORD env-override path (placeholder mode now
# short-circuits the yaml-default path for production-class targets).
#
# Sub-test 1 (env-override path): with POSTGRES_PASSWORD=smackerel set in
# the environment AND TARGET_ENV=home-lab, the SST loader MUST exit
# non-zero because the env-override literal triggers the FR-051-005
# dev-default rejection (spec 052 BS-052-006 — env override beats yaml
# AND skips placeholder mode AND must pass the dev-default gate).
#
# Sub-test 2 (placeholder mode, spec 052): with NO env override and
# TARGET_ENV=home-lab, the SST loader MUST exit 0 AND the resulting
# config/generated/home-lab.env MUST contain
# POSTGRES_PASSWORD=__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__ AND MUST
# NOT contain the literal dev-default value 'smackerel' as the password
# value. This validates that placeholder mode shields the bundle from
# accidental literal leakage even when the yaml has a dev-default value.
#
# Sub-test 3 (canary): the same loader run for TARGET_ENV=dev still
# produces a usable env file with the inline yaml value (proves the
# dev path is preserved per FR-052-011).
#
# Asserts on sub-test 1 stderr:
#   1a. exit non-zero
#   1b. stderr names "infrastructure.postgres.password"
#   1c. stderr references "spec 051"
#   1d. stderr does NOT echo the literal dev-default value as a
#       password value (FR-051-007 redaction)
#
# Exits 0 on full pass, 1 on any failure. Designed to be invoked from
# internal/config/sst_loader_test.go which captures repo path and
# asserts the exit code.

set -uo pipefail

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
CONFIG_SH="$REPO_ROOT/scripts/commands/config.sh"

if [[ ! -x "$CONFIG_SH" ]] && [[ ! -f "$CONFIG_SH" ]]; then
  echo "FATAL: cannot locate scripts/commands/config.sh under $REPO_ROOT" >&2
  exit 1
fi

failures=0

# -----------------------------------------------------------------------------
# Sub-test 1: env-override path drives FR-051-005 dev-default rejection.
# -----------------------------------------------------------------------------
echo "--- Sub-test 1: SST loader refuses env-override dev-default for home-lab ---"
home_lab_output="$(POSTGRES_PASSWORD=smackerel bash "$CONFIG_SH" --env home-lab 2>&1)"
home_lab_exit=$?

if [[ "$home_lab_exit" -eq 0 ]]; then
  echo "FAIL: SST loader returned exit 0 for POSTGRES_PASSWORD=smackerel + TARGET_ENV=home-lab (expected non-zero)"
  echo "----- captured output -----"
  echo "$home_lab_output"
  echo "----- end output -----"
  failures=$((failures + 1))
else
  echo "PASS: SST loader refused env-override dev-default with exit code $home_lab_exit"
fi

if ! echo "$home_lab_output" | grep -q "infrastructure.postgres.password"; then
  echo "FAIL: SST loader stderr does NOT name infrastructure.postgres.password"
  failures=$((failures + 1))
else
  echo "PASS: SST loader stderr names infrastructure.postgres.password"
fi

if ! echo "$home_lab_output" | grep -q "spec 051"; then
  echo "FAIL: SST loader stderr does NOT reference spec 051"
  failures=$((failures + 1))
else
  echo "PASS: SST loader stderr references spec 051"
fi

# Redaction assertion: the stderr MUST NOT contain the literal dev-default
# value as a free-standing password value. The error message is allowed to
# mention the project name "smackerel" in passing — but not as a credential.
if echo "$home_lab_output" | grep -qE '(POSTGRES_PASSWORD|password)[[:space:]=:]+["'\''[:space:]]*smackerel[[:space:]"'\''$]'; then
  echo "FAIL: SST loader stderr echoes dev-default value 'smackerel' as a password value"
  failures=$((failures + 1))
else
  echo "PASS: SST loader stderr does not echo 'smackerel' as a password value"
fi

# -----------------------------------------------------------------------------
# Sub-test 2 (spec 052): yaml-default path → placeholder mode shields bundle.
# -----------------------------------------------------------------------------
echo "--- Sub-test 2 (spec 052): SST loader emits placeholder for home-lab ---"
placeholder_output="$(bash "$CONFIG_SH" --env home-lab 2>&1)"
placeholder_exit=$?

if [[ "$placeholder_exit" -ne 0 ]]; then
  echo "FAIL: SST loader returned exit $placeholder_exit for TARGET_ENV=home-lab (expected 0 in placeholder mode)"
  echo "----- captured output -----"
  echo "$placeholder_output"
  echo "----- end output -----"
  failures=$((failures + 1))
else
  echo "PASS: SST loader exited 0 for TARGET_ENV=home-lab (placeholder mode active)"
fi

HOME_LAB_ENV="$REPO_ROOT/config/generated/home-lab.env"
if [[ ! -f "$HOME_LAB_ENV" ]]; then
  echo "FAIL: SST loader did not produce $HOME_LAB_ENV"
  failures=$((failures + 1))
else
  if grep -qE '^POSTGRES_PASSWORD=__SECRET_PLACEHOLDER__POSTGRES_PASSWORD__$' "$HOME_LAB_ENV"; then
    echo "PASS: home-lab.env contains POSTGRES_PASSWORD placeholder marker"
  else
    echo "FAIL: home-lab.env does NOT contain POSTGRES_PASSWORD placeholder marker"
    failures=$((failures + 1))
  fi

  if grep -qE '^POSTGRES_PASSWORD=smackerel$' "$HOME_LAB_ENV"; then
    echo "FAIL: home-lab.env contains literal POSTGRES_PASSWORD=smackerel (placeholder mode failed to shield)"
    failures=$((failures + 1))
  else
    echo "PASS: home-lab.env does NOT contain literal POSTGRES_PASSWORD=smackerel"
  fi
fi

# -----------------------------------------------------------------------------
# Sub-test 3 (canary): dev target still produces a usable env file.
# -----------------------------------------------------------------------------
echo "--- Sub-test 3 (canary): SST loader still works for TARGET_ENV=dev ---"
dev_output="$(bash "$CONFIG_SH" --env dev 2>&1)"
dev_exit=$?

if [[ "$dev_exit" -ne 0 ]]; then
  echo "FAIL: canary failed — SST loader for TARGET_ENV=dev returned exit $dev_exit"
  echo "----- captured output -----"
  echo "$dev_output"
  echo "----- end output -----"
  failures=$((failures + 1))
else
  echo "PASS: canary passed — SST loader for TARGET_ENV=dev exited 0"
fi

if [[ ! -f "$REPO_ROOT/config/generated/dev.env" ]]; then
  echo "FAIL: canary failed — config/generated/dev.env was not produced"
  failures=$((failures + 1))
else
  echo "PASS: canary produced config/generated/dev.env"
fi

# -----------------------------------------------------------------------------
echo ""
if [[ "$failures" -gt 0 ]]; then
  echo "FAILURES: $failures sub-test(s) failed"
  exit 1
fi
echo "All sub-tests passed"
exit 0

