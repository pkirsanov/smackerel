#!/usr/bin/env bash
#
# BUG-051-001 / SCN-051-001-A through SCN-051-001-D — SST loader regression
# for the home-lab runtime-env-mode bypass (SEC-HL-001).
#
# Background:
#   Before BUG-051-001, scripts/commands/config.sh resolved SMACKEREL_ENV
#   from smackerel.yaml's runtime.environment (which is "development" in the
#   SST) and only overrode it for TARGET_ENV=test. The home-lab
#   generator-side guard at lines ~415-433 rejected the dev-default
#   Postgres password, but the resulting bundle still emitted
#   SMACKEREL_ENV=development. That silently disabled:
#     - internal/auth/startup.go::ValidateRuntimeAuthStartup (returns nil
#       unless environment=="production"),
#     - internal/config/config.go production-mode auth + DB-password
#       fail-fast (gated on cfg.Environment=="production"),
#     - the spec 044 production-mode signing-material requirements,
#     - the spec 051 FR-051-005 dev-default Postgres password rejection at
#       runtime (the generator-side guard at lines ~415-433 still fires,
#       but the runtime-side guard becomes a no-op).
#
# Fix:
#   The per-target case in config.sh now adds a home-lab arm that overrides
#   SMACKEREL_ENV to "production" so the runtime defense-in-depth fires on
#   the home-lab tailnet bundle (matches the spec 044 + spec 051 contract).
#
# Sub-tests:
#   1. TARGET_ENV=home-lab against a temp smackerel.yaml (with a non-default
#      Postgres password so FR-051-005 does NOT block) emits
#      SMACKEREL_ENV=production into the generated env file.
#   2. (canary) TARGET_ENV=dev against the same temp yaml still emits
#      SMACKEREL_ENV=development.
#   3. (canary) TARGET_ENV=test against the same temp yaml still emits
#      SMACKEREL_ENV=test.
#   4. (defense-in-depth) TARGET_ENV=home-lab against the unpatched live
#      yaml (which still has the dev-default Postgres password "smackerel")
#      is still rejected by the FR-051-005 generator-side guard with the
#      spec 051 attribution.
#
# Adversarial proof: reverting the home-lab arm of the per-target case in
# scripts/commands/config.sh makes Sub-test 1 fail because the home-lab
# bundle reverts to SMACKEREL_ENV=development.
#
# Output isolation: config.sh hardcodes its output path to
# $REPO_ROOT/config/generated/${TARGET_ENV}.env, so this script backs up any
# pre-existing dev.env / test.env / home-lab.env files BEFORE running and
# restores them after, leaving the operator's working state untouched.
#
# This script is invoked by
# internal/config/sst_loader_home_lab_runtime_env_test.go under
# `./smackerel.sh test unit --go` so it runs in the standard unit-tier Go
# test pipeline as well as standalone.

set -uo pipefail

# REPO_ROOT is set by the Go driver; fall back to the path-from-this-file
# computation when invoked standalone from anywhere.
if [[ -z "${REPO_ROOT:-}" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

LIVE_YAML="$REPO_ROOT/config/smackerel.yaml"
CONFIG_SH="$REPO_ROOT/scripts/commands/config.sh"
GENERATED_DIR="$REPO_ROOT/config/generated"

if [[ ! -f "$LIVE_YAML" ]]; then
  echo "FATAL: $LIVE_YAML not found" >&2
  exit 1
fi
if [[ ! -f "$CONFIG_SH" ]]; then
  echo "FATAL: $CONFIG_SH not found" >&2
  exit 1
fi

# -----------------------------------------------------------------------
# Backup pre-existing generated env files so we can restore them at exit.
# -----------------------------------------------------------------------
BACKUP_DIR="$(mktemp -d)"
for env_name in dev test home-lab; do
  src="$GENERATED_DIR/${env_name}.env"
  if [[ -f "$src" ]]; then
    cp "$src" "$BACKUP_DIR/${env_name}.env"
  fi
done

TMP_YAML="$(mktemp)"

restore_generated() {
  local rc=$?
  for env_name in dev test home-lab; do
    src="$BACKUP_DIR/${env_name}.env"
    dst="$GENERATED_DIR/${env_name}.env"
    if [[ -f "$src" ]]; then
      cp "$src" "$dst"
    else
      # The file did not exist before our run; remove any artifact we created.
      rm -f "$dst"
    fi
  done
  rm -rf "$BACKUP_DIR"
  rm -f "$TMP_YAML"
  exit "$rc"
}
trap restore_generated EXIT INT TERM

# -----------------------------------------------------------------------
# Build the temp yaml with a non-default Postgres password so Sub-tests
# 1/2/3 isolate the SMACKEREL_ENV override behavior from the orthogonal
# FR-051-005 dev-default rejection.
# -----------------------------------------------------------------------
# Patch only the infrastructure.postgres.password line (4-space indent under
# infrastructure: → postgres: → password:). Other 'password:' or 'smackerel'
# tokens elsewhere in the yaml MUST NOT be rewritten.
awk '
  /^infrastructure:[[:space:]]*$/ { in_infra = 1; print; next }
  in_infra && /^[^[:space:]]/  { in_infra = 0; in_pg = 0 }
  in_infra && /^  postgres:[[:space:]]*$/ { in_pg = 1; print; next }
  in_infra && /^  [^[:space:]]/ && !/^  postgres:/ { in_pg = 0 }
  in_infra && in_pg && /^    password:[[:space:]]+smackerel[[:space:]]*$/ {
    print "    password: bug051001-strong-test-password-not-in-allowlist"
    next
  }
  { print }
' "$LIVE_YAML" > "$TMP_YAML"

# Sanity-check the patch applied. If awk failed (e.g. yaml structure
# changed), the test must abort rather than mis-attribute a downstream
# failure.
if ! grep -q '^    password: bug051001-strong-test-password-not-in-allowlist$' "$TMP_YAML"; then
  echo "FATAL: awk patch on infrastructure.postgres.password did not apply — yaml shape may have changed" >&2
  exit 1
fi

# Helper: invoke config.sh against a target env using the supplied yaml.
# Output always goes to $GENERATED_DIR/${TARGET_ENV}.env (config.sh contract).
run_generator() {
  local target_env="$1"
  local yaml_path="$2"

  bash "$CONFIG_SH" --env "$target_env" --config "$yaml_path" 2>&1
}

FAIL=0

echo "--- Sub-test 1: TARGET_ENV=home-lab emits SMACKEREL_ENV=production ---"
SUB1_OUT="$(run_generator home-lab "$TMP_YAML" || true)"
if [[ -f "$GENERATED_DIR/home-lab.env" ]] && grep -q '^SMACKEREL_ENV=production$' "$GENERATED_DIR/home-lab.env"; then
  echo "PASS: home-lab.env contains SMACKEREL_ENV=production"
else
  ACTUAL="$(grep '^SMACKEREL_ENV=' "$GENERATED_DIR/home-lab.env" 2>/dev/null || echo 'no SMACKEREL_ENV emitted')"
  echo "FAIL: home-lab.env does NOT contain SMACKEREL_ENV=production — actual: $ACTUAL"
  echo "      BUG-051-001 reintroduced: the home-lab arm of the per-target case in config.sh is missing or broken."
  echo "      Without SMACKEREL_ENV=production for home-lab, internal/auth/startup.go::ValidateRuntimeAuthStartup"
  echo "      returns nil unconditionally and internal/config/config.go production-mode auth + DB-password checks"
  echo "      are silently skipped, collapsing spec 044 + spec 051 defense-in-depth to bundle-generator-only."
  FAIL=$((FAIL+1))
fi

echo "--- Sub-test 2 (canary): TARGET_ENV=dev emits SMACKEREL_ENV=development ---"
SUB2_OUT="$(run_generator dev "$TMP_YAML" || true)"
if [[ -f "$GENERATED_DIR/dev.env" ]] && grep -q '^SMACKEREL_ENV=development$' "$GENERATED_DIR/dev.env"; then
  echo "PASS: dev.env contains SMACKEREL_ENV=development"
else
  ACTUAL="$(grep '^SMACKEREL_ENV=' "$GENERATED_DIR/dev.env" 2>/dev/null || echo 'no SMACKEREL_ENV emitted')"
  echo "FAIL: dev.env does NOT contain SMACKEREL_ENV=development — actual: $ACTUAL"
  FAIL=$((FAIL+1))
fi

echo "--- Sub-test 3 (canary): TARGET_ENV=test emits SMACKEREL_ENV=test ---"
SUB3_OUT="$(run_generator test "$TMP_YAML" || true)"
if [[ -f "$GENERATED_DIR/test.env" ]] && grep -q '^SMACKEREL_ENV=test$' "$GENERATED_DIR/test.env"; then
  echo "PASS: test.env contains SMACKEREL_ENV=test"
else
  ACTUAL="$(grep '^SMACKEREL_ENV=' "$GENERATED_DIR/test.env" 2>/dev/null || echo 'no SMACKEREL_ENV emitted')"
  echo "FAIL: test.env does NOT contain SMACKEREL_ENV=test — actual: $ACTUAL"
  FAIL=$((FAIL+1))
fi

echo "--- Sub-test 4: FR-051-005 generator-side Postgres dev-default check still fires for home-lab ---"
# Spec 052 evolution: under Scope 2, TARGET_ENV=home-lab now emits a
# placeholder marker for POSTGRES_PASSWORD when the resolved value would
# come from the yaml. To preserve SCN-051-S04 coverage, drive the
# FR-051-005 check via the env-override path (POSTGRES_PASSWORD=smackerel
# in the environment beats the yaml AND skips placeholder mode AND must
# pass the dev-default gate per BS-052-006).
SUB4_OUT="$(POSTGRES_PASSWORD=smackerel run_generator home-lab "$LIVE_YAML")"
SUB4_RC=$?
if [[ $SUB4_RC -ne 0 ]] && \
   echo "$SUB4_OUT" | grep -qi 'spec 051\|FR-051-005\|dev-default\|password' ; then
  echo "PASS: FR-051-005 generator-side guard still fires for home-lab via env-override (refused with spec 051 attribution)"
else
  echo "FAIL: FR-051-005 generator-side guard did NOT fire for home-lab via env-override (rc=$SUB4_RC)"
  echo "      Captured output:"
  echo "$SUB4_OUT" | sed 's/^/        /'
  FAIL=$((FAIL+1))
fi

echo ""
if [[ $FAIL -gt 0 ]]; then
  echo "FAILURES: $FAIL sub-test(s) failed"
  exit 1
fi

echo "All BUG-051-001 sub-tests passed"
exit 0
