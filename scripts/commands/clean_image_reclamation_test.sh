#!/usr/bin/env bash
#
# ─────────────────────────────────────────────────────────────────────────────
# spec 103 — Age-Bounded Project-Scoped Unused-Image Reclamation (Smart Cleanup)
# Docker-free unit harness for the cleanup image-reclamation stage.
# ─────────────────────────────────────────────────────────────────────────────
#
# Invoked via the repo CLI (terminal-discipline, full unfiltered output):
#
#     ./smackerel.sh --env dev clean test
#
# The clean) dispatch intercepts `test` BEFORE require_docker /
# smackerel_generate_config (Scope 4), so this harness runs with NO Docker
# daemon and NO generated env. It sources scripts/lib/runtime.sh (for
# smackerel_is_truthy) and scripts/lib/cleanup-image-reclamation.sh directly —
# so the pure argv builder and the dev-plane guard are exercised in isolation,
# WITHOUT running the smackerel.sh monolith (whose bottom `case "$COMMAND"`
# dispatch runs on `source`, making it non-sourceable in a unit test).
#
# Test convention mirrors scripts/commands/config_self_hosted_runtime_env_test.sh:
#   set -uo pipefail; compute REPO_ROOT; SMACKEREL_GENERATED_DIR isolation for
#   config-generation tests; echo PASS/FAIL per assertion; non-zero exit on any
#   failure. Fail-loud helper functions (assert_dev_plane / the argv builder on
#   invalid input) `exit 1`, so those cases are exercised in a subshell and the
#   exit code captured — never letting a fail-loud path abort the harness.

# File-wide shellcheck directives (intentional, documented):
#   SC2016 — several assertions grep smackerel.sh for LITERAL shell text such as
#            'smackerel_run_down "$TARGET_ENV" false'; the single quotes are
#            deliberate (we match the text as it appears in the file, no expansion).
#   SC2329 — the docker() stub in test_dry_run_no_exec is defined precisely to
#            PROVE non-invocation under DRY_RUN; shellcheck cannot see the
#            indirect (non-)call.
# shellcheck disable=SC2016,SC2329
set -uo pipefail

# REPO_ROOT may be injected by a Go driver; fall back to the path-from-this-file
# computation when invoked standalone or via `./smackerel.sh --env dev clean test`.
if [[ -z "${REPO_ROOT:-}" ]]; then
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
fi

HELPER="$REPO_ROOT/scripts/lib/cleanup-image-reclamation.sh"
DOCKERFILE_CORE="$REPO_ROOT/Dockerfile"
DOCKERFILE_ML="$REPO_ROOT/ml/Dockerfile"
CONFIG_YAML="$REPO_ROOT/config/smackerel.yaml"
CONFIG_SH="$REPO_ROOT/scripts/commands/config.sh"
SMACKEREL_SH="$REPO_ROOT/smackerel.sh"

# Canonical project-scope identity label (the single logical identity added to
# both Dockerfiles in Scope 1 and consumed by the argv builder in Scope 3).
OWNER_LABEL="io.smackerel.lifecycle.owner=smackerel"

FAIL=0
pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  FAIL=$((FAIL + 1))
}

# Source scripts/lib/runtime.sh (for smackerel_is_truthy, used by the executor)
# then the helper under test. runtime.sh enables `set -e`; the harness manages
# exit codes explicitly (fail-loud helper paths `exit 1` are exercised in
# subshells), so disable -e afterward — matching the config-test convention
# (set -uo pipefail). The helper is guarded so the Scope 3 RED run (helper
# absent) fails on undefined functions rather than aborting at source time.
# shellcheck source=/dev/null
source "$REPO_ROOT/scripts/lib/runtime.sh"
set +e
if [[ -f "$HELPER" ]]; then
  # shellcheck source=/dev/null
  source "$HELPER"
fi

# ─────────────────────────────────────────────────────────────────────────────
# Scope 1 — label-add prerequisite (Dockerfile + ml/Dockerfile) + label finding
# ─────────────────────────────────────────────────────────────────────────────

# AC-10 / FR-012 — both runtime stages carry the owner identity label.
test_owner_label_added() {
  local literal='LABEL io.smackerel.lifecycle.owner="smackerel"'
  if grep -qF "$literal" "$DOCKERFILE_CORE"; then
    pass "test_owner_label_added: Dockerfile (core) carries $literal"
  else
    fail "test_owner_label_added: Dockerfile (core) MISSING $literal"
  fi
  if grep -qF "$literal" "$DOCKERFILE_ML"; then
    pass "test_owner_label_added: ml/Dockerfile carries $literal"
  else
    fail "test_owner_label_added: ml/Dockerfile MISSING $literal"
  fi
}

# FR-002 / FR-012 — the Dockerfile LABEL literal matches the canonical owner
# label string; once the Scope 3 helper exists, its SMACKEREL_IMAGE_OWNER_LABEL
# constant is additionally asserted to match (single logical identity). The
# Dockerfile-vs-canonical assertion is real in every scope; the helper-constant
# clause activates as soon as the helper is present (Scope 3+).
test_owner_label_parity() {
  local literal='LABEL io.smackerel.lifecycle.owner="smackerel"'
  local canonical_from_literal='io.smackerel.lifecycle.owner=smackerel'
  if [[ "$canonical_from_literal" != "$OWNER_LABEL" ]]; then
    fail "test_owner_label_parity: Dockerfile literal owner label != canonical $OWNER_LABEL"
    return
  fi
  # Both Dockerfiles must use the identical literal (no drift between images).
  if grep -qF "$literal" "$DOCKERFILE_CORE" && grep -qF "$literal" "$DOCKERFILE_ML"; then
    pass "test_owner_label_parity: both Dockerfiles use the canonical literal $OWNER_LABEL"
  else
    fail "test_owner_label_parity: Dockerfile literal owner label absent/inconsistent"
    return
  fi
  if [[ -f "$HELPER" ]]; then
    local constant
    constant="$(
      # shellcheck disable=SC1090
      source "$HELPER" >/dev/null 2>&1
      printf '%s' "${SMACKEREL_IMAGE_OWNER_LABEL:-}"
    )"
    if [[ "$constant" == "$OWNER_LABEL" ]]; then
      pass "test_owner_label_parity: helper SMACKEREL_IMAGE_OWNER_LABEL matches the Dockerfile literal"
    else
      fail "test_owner_label_parity: helper SMACKEREL_IMAGE_OWNER_LABEL='$constant' != '$OWNER_LABEL'"
    fi
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Scope 2 — SST config keys + fail-loud loader/emit extension
# ─────────────────────────────────────────────────────────────────────────────

# Run config.sh --env dev against $1 (yaml path) with generated-dir isolation.
# Combined stdout+stderr goes to $2 (outfile); the function's return code is
# config.sh's exit code. Called DIRECTLY (never in $(...)) so the rc propagates.
_run_config_gen() {
  local yaml="$1" gen_dir="$2" outfile="$3"
  SMACKEREL_GENERATED_DIR="$gen_dir" SMACKEREL_HARDWARE_TIER=cpu \
    bash "$CONFIG_SH" --env dev --config "$yaml" >"$outfile" 2>&1
}

# FR-001 — the 3 keys exist under cleanup: in the SST yaml, and config.sh reads
# + emits them (SST wiring present).
test_config_keys_present() {
  local ok=1 k
  for k in remove_unused_images unused_image_min_age_hours unused_image_scope; do
    grep -qE "^  ${k}:" "$CONFIG_YAML" ||
      { fail "test_config_keys_present: config/smackerel.yaml missing cleanup.${k}"; ok=0; }
    grep -qF "required_value cleanup.${k}" "$CONFIG_SH" ||
      { fail "test_config_keys_present: config.sh missing 'required_value cleanup.${k}'"; ok=0; }
  done
  grep -qF 'validate_unused_image_policy' "$CONFIG_SH" ||
    { fail "test_config_keys_present: config.sh missing validate_unused_image_policy"; ok=0; }
  for e in CLEANUP_REMOVE_UNUSED_IMAGES CLEANUP_UNUSED_IMAGE_MIN_AGE_HOURS CLEANUP_UNUSED_IMAGE_SCOPE; do
    grep -qF "${e}=\${${e}}" "$CONFIG_SH" ||
      { fail "test_config_keys_present: config.sh missing emit line ${e}=\${${e}}"; ok=0; }
  done
  [[ "$ok" -eq 1 ]] && pass "test_config_keys_present: 3 cleanup keys + reads + validate + emits present"
}

# AC-6 / FR-001 — a missing key aborts non-zero with the canonical message.
test_fail_loud_missing_key() {
  local tmp_yaml gen_dir outfile
  tmp_yaml="$(mktemp)"; gen_dir="$(mktemp -d)"; outfile="$(mktemp)"
  awk '!/^[[:space:]]*unused_image_min_age_hours:[[:space:]]/' "$CONFIG_YAML" >"$tmp_yaml"
  _run_config_gen "$tmp_yaml" "$gen_dir" "$outfile"
  local rc=$?
  if [[ "$rc" -ne 0 ]] && grep -qF "Missing config key: cleanup.unused_image_min_age_hours" "$outfile"; then
    pass "test_fail_loud_missing_key: aborts non-zero with 'Missing config key: cleanup.unused_image_min_age_hours' (rc=$rc)"
  else
    fail "test_fail_loud_missing_key: expected non-zero + missing-key message (rc=$rc); output:"
    sed 's/^/    | /' "$outfile"
  fi
  rm -rf "$tmp_yaml" "$gen_dir" "$outfile"
}

# FR-001 — an invalid scope value aborts non-zero via validate_unused_image_policy.
test_fail_loud_invalid_scope() {
  local tmp_yaml gen_dir outfile
  tmp_yaml="$(mktemp)"; gen_dir="$(mktemp -d)"; outfile="$(mktemp)"
  awk '{ if ($0 ~ /^  unused_image_scope:[[:space:]]/) print "  unused_image_scope: everything"; else print }' \
    "$CONFIG_YAML" >"$tmp_yaml"
  _run_config_gen "$tmp_yaml" "$gen_dir" "$outfile"
  local rc=$?
  if [[ "$rc" -ne 0 ]] && grep -qF "cleanup.unused_image_scope must be project|all" "$outfile"; then
    pass "test_fail_loud_invalid_scope: aborts non-zero on unused_image_scope=everything (rc=$rc)"
  else
    fail "test_fail_loud_invalid_scope: expected non-zero + scope validation message (rc=$rc); output:"
    sed 's/^/    | /' "$outfile"
  fi
  rm -rf "$tmp_yaml" "$gen_dir" "$outfile"
}

# FR-001 — a non-positive-integer age aborts non-zero.
test_fail_loud_invalid_age() {
  local tmp_yaml gen_dir outfile
  tmp_yaml="$(mktemp)"; gen_dir="$(mktemp -d)"; outfile="$(mktemp)"
  awk '{ if ($0 ~ /^  unused_image_min_age_hours:[[:space:]]/) print "  unused_image_min_age_hours: 0"; else print }' \
    "$CONFIG_YAML" >"$tmp_yaml"
  _run_config_gen "$tmp_yaml" "$gen_dir" "$outfile"
  local rc=$?
  if [[ "$rc" -ne 0 ]] && grep -qF "cleanup.unused_image_min_age_hours must be a positive integer" "$outfile"; then
    pass "test_fail_loud_invalid_age: aborts non-zero on unused_image_min_age_hours=0 (rc=$rc)"
  else
    fail "test_fail_loud_invalid_age: expected non-zero + age validation message (rc=$rc); output:"
    sed 's/^/    | /' "$outfile"
  fi
  rm -rf "$tmp_yaml" "$gen_dir" "$outfile"
}

# FR-001 — a non-boolean remove_unused_images aborts non-zero.
test_fail_loud_invalid_bool() {
  local tmp_yaml gen_dir outfile
  tmp_yaml="$(mktemp)"; gen_dir="$(mktemp -d)"; outfile="$(mktemp)"
  awk '{ if ($0 ~ /^  remove_unused_images:[[:space:]]/) print "  remove_unused_images: maybe"; else print }' \
    "$CONFIG_YAML" >"$tmp_yaml"
  _run_config_gen "$tmp_yaml" "$gen_dir" "$outfile"
  local rc=$?
  if [[ "$rc" -ne 0 ]] && grep -qF "cleanup.remove_unused_images must be true|false" "$outfile"; then
    pass "test_fail_loud_invalid_bool: aborts non-zero on remove_unused_images=maybe (rc=$rc)"
  else
    fail "test_fail_loud_invalid_bool: expected non-zero + bool validation message (rc=$rc); output:"
    sed 's/^/    | /' "$outfile"
  fi
  rm -rf "$tmp_yaml" "$gen_dir" "$outfile"
}

# FR-001 — a valid generation emits the 3 CLEANUP_* values into the env file.
test_generated_env_carries_cleanup() {
  local gen_dir outfile
  gen_dir="$(mktemp -d)"; outfile="$(mktemp)"
  SMACKEREL_GENERATED_DIR="$gen_dir" SMACKEREL_HARDWARE_TIER=cpu \
    bash "$CONFIG_SH" --env dev >"$outfile" 2>&1
  local rc=$?
  if [[ "$rc" -eq 0 ]] &&
    grep -q '^CLEANUP_REMOVE_UNUSED_IMAGES=true$' "$gen_dir/dev.env" &&
    grep -q '^CLEANUP_UNUSED_IMAGE_MIN_AGE_HOURS=48$' "$gen_dir/dev.env" &&
    grep -q '^CLEANUP_UNUSED_IMAGE_SCOPE=project$' "$gen_dir/dev.env"; then
    pass "test_generated_env_carries_cleanup: dev.env carries CLEANUP_REMOVE_UNUSED_IMAGES/MIN_AGE_HOURS/SCOPE (rc=$rc)"
  else
    fail "test_generated_env_carries_cleanup: rc=$rc; CLEANUP_* emitted: $(grep -E '^CLEANUP_' "$gen_dir/dev.env" 2>/dev/null | tr '\n' ' ' || echo none)"
  fi
  rm -rf "$gen_dir" "$outfile"
}

# ─────────────────────────────────────────────────────────────────────────────# Scope 3 — reclamation helper: pure argv builder + env=prod guard + executor
# ────────────────────────────────────────────────────────────────────────────────────

EXPECT_PROJECT="image prune -a -f --filter until=48h --filter label=io.smackerel.lifecycle.owner=smackerel --filter label!=io.smackerel.environment=prod"
EXPECT_ALL="image prune -a -f --filter until=48h --filter label!=io.smackerel.environment=prod"

# AC-1, AC-9 / FR-002 — project argv is exact incl age + owner label + env=prod exclude.
test_argv_project() {
  local out
  out="$(build_unused_image_prune_argv 48 project 2>&1)"
  if [[ "$out" == "$EXPECT_PROJECT" ]]; then
    pass "test_argv_project: project argv is exact"
  else
    fail "test_argv_project: got [$out] expected [$EXPECT_PROJECT]"
  fi
}

# FR-002 — all argv omits the owner label but keeps age + env=prod exclude.
test_argv_all() {
  local out
  out="$(build_unused_image_prune_argv 48 all 2>&1)"
  if [[ "$out" == "$EXPECT_ALL" ]]; then
    pass "test_argv_all: all argv is exact (no owner label)"
  else
    fail "test_argv_all: got [$out] expected [$EXPECT_ALL]"
  fi
}

# AC-2 / FR-007 — changing the age changes ONLY the until token.
test_argv_min_age_applied() {
  local out expect
  out="$(build_unused_image_prune_argv 72 project 2>&1)"
  expect="${EXPECT_PROJECT/until=48h/until=72h}"
  if [[ "$out" == "$expect" ]]; then
    pass "test_argv_min_age_applied: min-age drives only the until token (until=72h)"
  else
    fail "test_argv_min_age_applied: got [$out] expected [$expect]"
  fi
}

# AC-9 / FR-009 — both scopes' argv contain the env=prod exclusion.
test_argv_env_prod_excluded() {
  local p a marker='--filter label!=io.smackerel.environment=prod'
  p="$(build_unused_image_prune_argv 48 project 2>&1)"
  a="$(build_unused_image_prune_argv 48 all 2>&1)"
  if [[ "$p" == *"$marker"* && "$a" == *"$marker"* ]]; then
    pass "test_argv_env_prod_excluded: both scopes exclude env=prod"
  else
    fail "test_argv_env_prod_excluded: project=[$p] all=[$a]"
  fi
}

# FR-002 — an invalid scope aborts non-zero.
test_argv_invalid_scope_fails() {
  local rc
  (build_unused_image_prune_argv 48 nonsense) >/dev/null 2>&1
  rc=$?
  if [[ "$rc" -ne 0 ]]; then
    pass "test_argv_invalid_scope_fails: scope=nonsense aborts non-zero (rc=$rc)"
  else
    fail "test_argv_invalid_scope_fails: expected non-zero for scope=nonsense (rc=$rc)"
  fi
}

# FR-002 — a non-positive-integer age aborts non-zero.
test_argv_invalid_age_fails() {
  local rc1 rc2
  (build_unused_image_prune_argv 0 project) >/dev/null 2>&1
  rc1=$?
  (build_unused_image_prune_argv abc project) >/dev/null 2>&1
  rc2=$?
  if [[ "$rc1" -ne 0 && "$rc2" -ne 0 ]]; then
    pass "test_argv_invalid_age_fails: age=0 and age=abc abort non-zero (rc=$rc1,$rc2)"
  else
    fail "test_argv_invalid_age_fails: expected non-zero for both (rc=$rc1,$rc2)"
  fi
}

# AC-8 / FR-008 — the guard refuses non-dev planes (production/staging/empty).
test_guard_refuses_nondev() {
  local ok=1 v rc out
  for v in production staging ""; do
    out="$(assert_dev_plane "$v" 2>&1)"
    rc=$?
    if [[ "$rc" -eq 0 ]]; then
      fail "test_guard_refuses_nondev: assert_dev_plane '$v' returned 0 (should refuse)"
      ok=0
    fi
  done
  out="$(assert_dev_plane production 2>&1)"
  if [[ "$out" != *"refusing dev-plane image reclamation on a non-dev plane (SMACKEREL_ENV=production)"* ]]; then
    fail "test_guard_refuses_nondev: production message missing the contract text; got [$out]"
    ok=0
  fi
  [[ "$ok" -eq 1 ]] && pass "test_guard_refuses_nondev: production/staging/empty refuse with the contract message"
}

# FR-008 — the guard allows the dev-safe planes {development, test}.
test_guard_allows_dev() {
  local rc1 rc2
  (assert_dev_plane development) >/dev/null 2>&1
  rc1=$?
  (assert_dev_plane test) >/dev/null 2>&1
  rc2=$?
  if [[ "$rc1" -eq 0 && "$rc2" -eq 0 ]]; then
    pass "test_guard_allows_dev: development and test return 0"
  else
    fail "test_guard_allows_dev: expected 0/0 (rc=$rc1,$rc2)"
  fi
}

# AC-5 / FR-004 — DRY_RUN previews the plan and executes nothing (a shadowed
# docker stub proves non-invocation).
test_dry_run_no_exec() {
  local out
  docker() { echo "DOCKER_STUB_INVOKED $*"; }
  out="$(SMACKEREL_ENV=development DRY_RUN=true prune_unused_images_aged 48 project 2>&1)"
  unset -f docker
  if [[ "$out" == *"[DRY-RUN] Would execute: docker image prune -a -f --filter until=48h"* &&
    "$out" != *"DOCKER_STUB_INVOKED"* ]]; then
    pass "test_dry_run_no_exec: DRY_RUN previews the plan and never invokes docker"
  else
    fail "test_dry_run_no_exec: output=[$out]"
  fi
}

# AC-11 / FR-005 — the helper file references no volume-pruning token.
test_no_volume_tokens() {
  if grep -nE 'docker[[:space:]]+volume|--volumes' "$HELPER"; then
    fail "test_no_volume_tokens: helper references a volume-prune token (above)"
  else
    pass "test_no_volume_tokens: helper is grep-clean of volume-prune tokens"
  fi
}

# AC-11 / FR-006 — the helper file references no container-removal token.
test_no_container_tokens() {
  if grep -nE 'docker[[:space:]]+container|docker[[:space:]]+rm' "$HELPER"; then
    fail "test_no_container_tokens: helper references a container-removal token (above)"
  else
    pass "test_no_container_tokens: helper is grep-clean of container-removal tokens"
  fi
}

# ─────────────────────────────────────────────────────────────────────────────
# Scope 4 — wire into clean smart (gated) + clean test entrypoint; other unchanged
# ─────────────────────────────────────────────────────────────────────────────

# Extract a single 6-space-indented arm of the clean) SUBCOMMAND case (body up
# to the arm's 8-space ;;). bash-3.2-safe (awk only).
_clean_arm() { awk -v a="$1" '$0 ~ ("^      " a "\\)") {f=1} f{print} f && /^        ;;/{exit}' "$SMACKEREL_SH"; }

# AC-1 / FR-010 — smart) calls prune_unused_images_aged AFTER the teardown, gated.
test_smart_wires_stage() {
  local block ok=1 order
  block="$(_clean_arm smart)"
  printf '%s\n' "$block" | grep -qF 'smackerel_run_down "$TARGET_ENV" false' ||
    { fail "test_smart_wires_stage: smart) lost the volume-preserving teardown"; ok=0; }
  printf '%s\n' "$block" | grep -qF 'CLEANUP_REMOVE_UNUSED_IMAGES' ||
    { fail "test_smart_wires_stage: smart) not gated on CLEANUP_REMOVE_UNUSED_IMAGES"; ok=0; }
  printf '%s\n' "$block" | grep -qF 'prune_unused_images_aged' ||
    { fail "test_smart_wires_stage: smart) does not call prune_unused_images_aged"; ok=0; }
  order="$(printf '%s\n' "$block" | awk '/smackerel_run_down/{rd=NR} /prune_unused_images_aged/{if(!pr)pr=NR} END{if(rd&&pr&&pr>rd)print "ok"; else print "bad"}')"
  [[ "$order" == "ok" ]] || { fail "test_smart_wires_stage: prune not positioned after the teardown"; ok=0; }
  [[ "$ok" -eq 1 ]] && pass "test_smart_wires_stage: smart) calls prune_unused_images_aged after teardown, gated on CLEANUP_REMOVE_UNUSED_IMAGES"
}

# AC-4 / FR-010 — the disabled path logs the exact 'disabled' line behind the gate.
test_smart_gated_off() {
  local block ok=1
  block="$(_clean_arm smart)"
  printf '%s\n' "$block" | grep -qF 'Aged unused-image reclamation disabled (cleanup.remove_unused_images=false)' ||
    { fail "test_smart_gated_off: missing the disabled log line"; ok=0; }
  printf '%s\n' "$block" | grep -qF '== "true"' ||
    { fail "test_smart_gated_off: gate condition (== \"true\") not found"; ok=0; }
  [[ "$ok" -eq 1 ]] && pass "test_smart_gated_off: disabled path logs 'Aged unused-image reclamation disabled (...)' behind the gate"
}

# AC-7 / FR-011 — full) unchanged: down -v (true), never prunes.
test_full_unchanged() {
  local block ok=1
  block="$(_clean_arm full)"
  printf '%s\n' "$block" | grep -qF 'smackerel_run_down "$TARGET_ENV" true' ||
    { fail "test_full_unchanged: full) no longer runs smackerel_run_down true"; ok=0; }
  if printf '%s\n' "$block" | grep -qF 'prune_unused_images_aged'; then
    fail "test_full_unchanged: full) must NOT call prune_unused_images_aged"; ok=0
  fi
  [[ "$ok" -eq 1 ]] && pass "test_full_unchanged: full) still runs smackerel_run_down true and never prunes images"
}

# AC-7 / FR-011 — status)/measure) unchanged, never prune.
test_status_measure_unchanged() {
  local sblock mblock ok=1
  sblock="$(_clean_arm status)"
  mblock="$(_clean_arm measure)"
  printf '%s\n' "$sblock" | grep -qF 'smackerel_compose "$TARGET_ENV" ps -a' ||
    { fail "test_status_measure_unchanged: status) changed"; ok=0; }
  printf '%s\n' "$mblock" | grep -qF 'docker system df' ||
    { fail "test_status_measure_unchanged: measure) changed"; ok=0; }
  if printf '%s\n' "$sblock" "$mblock" | grep -qF 'prune_unused_images_aged'; then
    fail "test_status_measure_unchanged: status/measure must not prune"; ok=0
  fi
  [[ "$ok" -eq 1 ]] && pass "test_status_measure_unchanged: status) ps -a + measure) system df unchanged; neither prunes"
}

# FR-015 — clean) intercepts `test` BEFORE require_docker (Docker-free) + usage lists it.
test_clean_test_entrypoint() {
  local ok=1 order
  grep -qF 'exec bash "$SCRIPT_DIR/scripts/commands/clean_image_reclamation_test.sh"' "$SMACKEREL_SH" ||
    { fail "test_clean_test_entrypoint: clean) test intercept missing"; ok=0; }
  order="$(awk '
    /^  clean\)/ { inarm=1 }
    inarm && /clean_image_reclamation_test\.sh/ && !iset { itcpt=NR; iset=1 }
    inarm && /^[[:space:]]*require_docker$/ && !rset { reqd=NR; rset=1 }
    inarm && /^  [a-z]/ && !/^  clean\)/ { inarm=0 }
    END { if (iset && rset && itcpt < reqd) print "ok"; else print "bad" }
  ' "$SMACKEREL_SH")"
  [[ "$order" == "ok" ]] || { fail "test_clean_test_entrypoint: intercept not before require_docker in clean) arm"; ok=0; }
  grep -qE '^[[:space:]]+clean test[[:space:]]' "$SMACKEREL_SH" ||
    { fail "test_clean_test_entrypoint: usage missing a 'clean test' help line"; ok=0; }
  [[ "$ok" -eq 1 ]] && pass "test_clean_test_entrypoint: clean) intercepts test before require_docker + usage lists 'clean test'"
}

# ─────────────────────────────────────────────────────────────────────────────
# Run all tests (explicit ordered list — bash 3.2 safe, no mapfile).
# ─────────────────────────────────────────────────────────────────────────────
echo "═══ spec 103 cleanup image-reclamation unit harness ═══"
echo "--- Scope 1: label-add prerequisite ---"
test_owner_label_added
test_owner_label_parity

echo "--- Scope 2: SST config keys (fail-loud) ---"
test_config_keys_present
test_fail_loud_missing_key
test_fail_loud_invalid_scope
test_fail_loud_invalid_age
test_fail_loud_invalid_bool
test_generated_env_carries_cleanup

echo "--- Scope 3: reclamation helper (argv + guard + executor) ---"
test_argv_project
test_argv_all
test_argv_min_age_applied
test_argv_env_prod_excluded
test_argv_invalid_scope_fails
test_argv_invalid_age_fails
test_guard_refuses_nondev
test_guard_allows_dev
test_dry_run_no_exec
test_no_volume_tokens
test_no_container_tokens

echo "--- Scope 4: wiring + clean test entrypoint (other levels unchanged) ---"
test_smart_wires_stage
test_smart_gated_off
test_full_unchanged
test_status_measure_unchanged
test_clean_test_entrypoint

echo ""
if [[ "$FAIL" -ne 0 ]]; then
  echo "RESULT: $FAIL assertion(s) FAILED"
  exit 1
fi
echo "RESULT: all assertions passed"
exit 0
