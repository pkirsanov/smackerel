#!/usr/bin/env bash
#
# ─────────────────────────────────────────────────────────────────────────────
# spec 103 — Age-Bounded Project-Scoped Unused-Image Reclamation (Smart Cleanup)
# ─────────────────────────────────────────────────────────────────────────────
#
# A focused, SOURCEABLE helper holding the three NEW cleanup functions plus their
# module constants. smackerel.sh sources this file once (right after the existing
# `source scripts/lib/runtime.sh`); the unit harness
# (scripts/commands/clean_image_reclamation_test.sh) sources it directly — so the
# pure argv builder and the dev-plane guard are genuinely unit-testable WITHOUT
# Docker and WITHOUT running the smackerel.sh monolith (whose bottom
# `case "$COMMAND"` dispatch runs on `source`, making it non-sourceable).
#
# This stage is IMAGE-ONLY. It never references or removes persistent named data
# stores and never references any running workload. `docker image prune -a`
# inherently refuses image layers backing a live workload; `-f` only skips the
# interactive prompt. smackerel's persistent stores (the postgres/pgvector data
# store and any NATS jetstream store) are structurally out of reach of an
# image-only prune.
#
# This file is a sourced function library: it deliberately does NOT set shell
# options (no `set -euo pipefail`) so it inherits the caller's options — under
# smackerel.sh it runs with the monolith's `set -e` (fail-loud exit on the guard
# / invalid input); under the harness it runs with the harness's exit-code
# management.
#
# Consumed-as-existing helper (defined by runtime.sh, sourced by both callers):
# smackerel_is_truthy.

# Project-scope identity label. FIXED build-identity constant that MUST match the
# `LABEL io.smackerel.lifecycle.owner="smackerel"` literal added to Dockerfile +
# ml/Dockerfile (spec 103 Scope 1); test_owner_label_parity asserts the match.
# This is NOT a runtime-config fallback (smackerel-no-defaults governs runtime
# VALUES read from config/smackerel.yaml — this pairs 1:1 with the Dockerfile
# literal, so a plain assignment is correct, not a `${VAR:-default}`).
SMACKEREL_IMAGE_OWNER_LABEL="io.smackerel.lifecycle.owner=smackerel"
SMACKEREL_ENV_LABEL_KEY="io.smackerel.environment" # runtime env classification label
SMACKEREL_PROD_ENV_EXCLUDE="prod"                  # env=prod image exclusion token
SMACKEREL_DEV_SAFE_ENVS="development test"          # dev-plane allow-list (fail-safe)

# assert_dev_plane <smackerel_env> — PROD-SAFETY guard (FR-008).
# $1 is the resolved SMACKEREL_ENV (read by the caller from the generated
# config/generated/<env>.env, or injected by the unit harness). Fail-safe: only
# proceed on a known dev-safe plane; anything else — production, empty, unknown —
# aborts BEFORE any prune. This is the real prod-leak guard: `--env dev` combined
# with `runtime.environment: production` in smackerel.yaml emits
# SMACKEREL_ENV=production (config.sh overrides only test/self-hosted, so dev
# passes production straight through), which this guard catches.
assert_dev_plane() {
  local env="${1:-}"
  local safe
  # shellcheck disable=SC2086
  for safe in $SMACKEREL_DEV_SAFE_ENVS; do
    if [[ "$env" == "$safe" ]]; then
      return 0
    fi
  done
  echo "ERROR: refusing dev-plane image reclamation on a non-dev plane (SMACKEREL_ENV=${env:-<unset>}); this stage runs only on the developer/CI dev Docker daemon" >&2
  exit 1
}

# build_unused_image_prune_argv <min_age_hours> <scope> — PURE, testable (FR-002).
# Echoes the docker argv (space-joined, WITHOUT the leading `docker`), no side
# effects. Validates inputs (positive-int age; scope ∈ {project, all}); fails
# loud on invalid input. `-a` (remove unused, not just dangling) + `until=<N>h`
# (age bound) + `label=<owner>` (project scope) + `label!=<env>=prod` (env=prod
# exclusion) compose with AND semantics in `docker image prune`.
#   project -> image prune -a -f --filter until=<N>h \
#              --filter label=<owner> --filter label!=<env>=prod
#   all     -> image prune -a -f --filter until=<N>h \
#              --filter label!=<env>=prod
build_unused_image_prune_argv() {
  local min_age_hours="$1"
  local scope="$2"

  if ! [[ "$min_age_hours" =~ ^[1-9][0-9]*$ ]]; then
    echo "ERROR: build_unused_image_prune_argv: min_age_hours must be a positive integer (got: ${min_age_hours})" >&2
    exit 1
  fi

  local argv="image prune -a -f --filter until=${min_age_hours}h"
  case "$scope" in
    project)
      # Project scope: only owner-labeled smackerel images are eligible;
      # peer-product images (no owner label) are structurally skipped. The owner
      # label comes from the module constant, never re-hardcoded inline (FR-002).
      argv+=" --filter label=${SMACKEREL_IMAGE_OWNER_LABEL}"
      ;;
    all)
      : # system-wide age-bounded prune (opt-in breadth escape hatch)
      ;;
    *)
      echo "ERROR: build_unused_image_prune_argv: scope must be project|all (got: ${scope})" >&2
      exit 1
      ;;
  esac
  # env=prod exclusion applies in BOTH scopes (FR-009).
  argv+=" --filter label!=${SMACKEREL_ENV_LABEL_KEY}=${SMACKEREL_PROD_ENV_EXCLUDE}"

  printf '%s\n' "$argv"
}

# prune_unused_images_aged <min_age_hours> <scope> — executor (FR-004/005/006/007).
# Calls assert_dev_plane FIRST (prod-safety), honors the DRY_RUN convention via
# smackerel_is_truthy ([DRY-RUN] Would execute: …, execute nothing), and on the
# real path prints the unfiltered `docker image prune` output (which ends with
# the reclaimed-space summary) plus a completion line. It NEVER references or
# removes persistent data stores and NEVER references running workloads.
prune_unused_images_aged() {
  local min_age_hours="$1"
  local scope="$2"

  # Resolve the plane signal. In production smackerel.sh reads SMACKEREL_ENV from
  # the generated config/generated/<env>.env (FR-008) and passes it in via the
  # environment for this single call; the Docker-free unit harness injects it
  # directly. An empty/unset value is caught by assert_dev_plane (fail-safe).
  local smackerel_env="${SMACKEREL_ENV:-}"
  assert_dev_plane "$smackerel_env"

  local argv_str
  argv_str="$(build_unused_image_prune_argv "$min_age_hours" "$scope")"

  echo "Reclaiming aged unused smackerel images (scope=${scope}, min age ${min_age_hours}h)..."

  if smackerel_is_truthy "${DRY_RUN:-}"; then
    echo "[DRY-RUN] Would execute: docker ${argv_str}"
    return 0
  fi

  local -a argv
  read -r -a argv <<<"$argv_str"
  docker "${argv[@]}"
  echo "Aged unused-image reclamation complete (scope=${scope})"
}
