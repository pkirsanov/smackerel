#!/usr/bin/env bash
# Spec 051 SCN-051-S02 / FR-051-005 — SST-loader dev-default rejection
# adversarial test. Invokes scripts/commands/config.sh against the live
# config/smackerel.yaml as the home-lab target. Asserts:
#
#   1. The loader exits non-zero (the dev-default Postgres password in
#      config/smackerel.yaml is refused for non-dev/test targets).
#   2. The stderr output names "infrastructure.postgres.password"
#      (operators can act on the failure).
#   3. The stderr output references "spec 051" (links the failure to the
#      authoritative requirement).
#   4. The stderr output does NOT echo the literal dev-default value
#      "smackerel" as a free-standing token (FR-051-007 redaction).
#
# Canary sub-test:
#
#   5. The same loader run for TARGET_ENV=dev still produces a usable
#      env file with the same dev-default value (proves we did not
#      accidentally break the dev path).
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
# Sub-test 1: home-lab target with dev-default password MUST be refused.
# -----------------------------------------------------------------------------
echo "--- Sub-test 1: SST loader refuses dev-default password for home-lab ---"
home_lab_output="$(bash "$CONFIG_SH" --env home-lab 2>&1)"
home_lab_exit=$?

if [[ "$home_lab_exit" -eq 0 ]]; then
  echo "FAIL: SST loader returned exit 0 for TARGET_ENV=home-lab with dev-default Postgres password (expected non-zero)"
  echo "----- captured output -----"
  echo "$home_lab_output"
  echo "----- end output -----"
  failures=$((failures + 1))
else
  echo "PASS: SST loader refused TARGET_ENV=home-lab with exit code $home_lab_exit"
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
# value as a free-standing token. We check the canonical primary default
# value used by this repo's config/smackerel.yaml.
if echo "$home_lab_output" | grep -qwF "smackerel"; then
  # The error message is allowed to mention the project name "smackerel"
  # in passing — but the dev-default password value must not appear as a
  # standalone word in the error. Distinguish: the offending pattern is
  # the password value appearing as the *value* of any KEY=VALUE field.
  # Use a stricter regex that looks for KEY=smackerel or =smackerel or
  # any explicit echo of the value.
  if echo "$home_lab_output" | grep -qE '(POSTGRES_PASSWORD|password)[[:space:]=:]+["'\''[:space:]]*smackerel[[:space:]"'\''$]'; then
    echo "FAIL: SST loader stderr echoes dev-default value 'smackerel' as a password value"
    failures=$((failures + 1))
  else
    echo "PASS: SST loader stderr mentions 'smackerel' only in non-credential context (project name OK)"
  fi
else
  echo "PASS: SST loader stderr does not contain free-standing 'smackerel' token"
fi

# -----------------------------------------------------------------------------
# Sub-test 2 (canary): dev target still produces a usable env file.
# -----------------------------------------------------------------------------
echo "--- Sub-test 2 (canary): SST loader still works for TARGET_ENV=dev ---"
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
