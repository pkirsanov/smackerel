#!/usr/bin/env bash
# scripts/runtime/web-e2e-ui.sh
#
# Spec 077 — Compose-Project Lane wrapper for the PWA browser
# end-to-end UI test harness.
#
# SCOPE-1a shipped the dispatcher + lane wrapper skeleton (Compose
# project name `smackerel-test-e2e-ui`, `--print-compose-project`
# introspection, fail-loud "runner not yet wired" stub).
#
# SCOPE-1b added the `run_node_tooling` helper that locates the Node
# tooling, runs `npx playwright test`, and propagates the exit code
# (SCN-077-A10 / TP-077-01-03).
#
# SCOPE-1c (this scope) wires the disposable-stack lifecycle around
# `run_node_tooling`: generate test SST → bring up the default compose
# stack under the dedicated project name `smackerel-test-e2e-ui` →
# export `SMACKEREL_BASE_URL` derived from the SST `CORE_EXTERNAL_URL`
# → invoke Playwright → teardown via trap on success/failure/signal.
# Anchors SCN-077-A01 (proof-of-life) and SCN-077-A07 (dev-stack
# isolation).
#
# The lane uses docker compose's `--project-name` flag (which wins over
# the env-file `COMPOSE_PROJECT` value) so the wrapper can reuse the
# repo `docker-compose.yml` + SST env file without colliding with the
# `smackerel-test` project owned by `./smackerel.sh test integration`
# and `./smackerel.sh test e2e`.

set -euo pipefail

# Stable, dedicated Compose project for the disposable e2e-ui test stack.
# Distinct from `smackerel-test` (Go integration/e2e/stress lane) so the
# two lanes cannot collide on networks, container names, or volumes.
SMACKEREL_E2E_UI_COMPOSE_PROJECT="smackerel-test-e2e-ui"

# Repo-relative location of the PWA Playwright workspace.
SMACKEREL_E2E_UI_PWA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/web/pwa"

# run_node_tooling — invoke `npx playwright test` against the PWA
# workspace. Propagates the exit code to the caller. Callers MUST have
# exported `SMACKEREL_BASE_URL` (the fail-loud SST consumer in
# `web/pwa/tests/_support/env.ts` will throw if it is missing). Any
# additional arguments are forwarded verbatim to `playwright test`.
#
# Designed to be sourced and called by the spec-077 unit test that
# asserts exit-code propagation (TP-077-01-03). When the env var
# `SMACKEREL_E2E_UI_NPX` is set, it is used in place of `npx` — the unit
# test uses this to inject a stub binary that exits with a configurable
# code.
run_node_tooling() {
  local npx_bin="${SMACKEREL_E2E_UI_NPX:-npx}"
  if ! command -v "$npx_bin" >/dev/null 2>&1; then
    echo "ERROR: '$npx_bin' is required to run the spec 077 PWA e2e-ui harness but is not on PATH." >&2
    return 127
  fi
  if [[ ! -d "$SMACKEREL_E2E_UI_PWA_DIR" ]]; then
    echo "ERROR: PWA workspace not found at $SMACKEREL_E2E_UI_PWA_DIR" >&2
    return 1
  fi
  (
    cd "$SMACKEREL_E2E_UI_PWA_DIR"
    "$npx_bin" playwright test "$@"
  )
}

# Allow callers (e.g. the dispatcher canary) to introspect the project name
# without bringing up any stack or invoking the Node runner.
if [[ "${1:-}" == "--print-compose-project" ]]; then
  printf '%s\n' "$SMACKEREL_E2E_UI_COMPOSE_PROJECT"
  exit 0
fi

# When sourced (e.g. by the spec-077 unit test), do not execute the
# default action — only the functions and constants above are exposed.
if [[ "${BASH_SOURCE[0]}" != "${0}" ]]; then
  return 0
fi

# ------------------------------------------------------------------
# SCOPE-1c lifecycle wiring
# ------------------------------------------------------------------
#
# bring_up_test_stack — generate SST → bring up the default compose
# stack under project name `smackerel-test-e2e-ui`. Uses docker compose
# directly so the `--project-name` flag overrides the env-file
# `COMPOSE_PROJECT` value (= `smackerel-test`) and isolates this lane
# from the integration/e2e/stress lanes.
#
# tear_down_test_stack — invoked from the EXIT/INT/TERM trap. MUST run
# on success, failure, and signal interruption. Removes volumes +
# orphans for the dedicated project only — the persistent dev stack
# (default Compose project `smackerel`) and the integration/e2e test
# stack (`smackerel-test`) are NOT touched because docker compose
# scopes all operations to `--project-name`.

# Resolve repo root + SST helpers exactly once.
SMACKEREL_E2E_UI_REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/lib/runtime.sh
source "$SMACKEREL_E2E_UI_REPO_ROOT/scripts/lib/runtime.sh"

e2e_ui_compose() {
  # All compose invocations for this lane go through this helper so the
  # project-name + env-file + repo compose file are applied uniformly
  # and the integration test can grep for the contract.
  docker compose \
    --project-name "$SMACKEREL_E2E_UI_COMPOSE_PROJECT" \
    --env-file "$SMACKEREL_E2E_UI_ENV_FILE" \
    -f "$SMACKEREL_E2E_UI_REPO_ROOT/docker-compose.yml" \
    "$@"
}

tear_down_test_stack() {
  # Idempotent: safe to call multiple times. `down --remove-orphans
  # --volumes` removes only resources labeled with this project name.
  if [[ -n "${SMACKEREL_E2E_UI_ENV_FILE:-}" && -f "$SMACKEREL_E2E_UI_ENV_FILE" ]]; then
    echo "[web-e2e-ui] Tearing down disposable test stack (project ${SMACKEREL_E2E_UI_COMPOSE_PROJECT})..." >&2
    e2e_ui_compose down --remove-orphans --volumes --timeout 60 >&2 || true
  fi
}

bring_up_test_stack() {
  echo "[web-e2e-ui] Generating SST test env..." >&2
  smackerel_generate_config test >/dev/null

  SMACKEREL_E2E_UI_ENV_FILE="$(smackerel_require_env_file test)"

  local core_url
  local wait_timeout_s
  core_url="$(smackerel_env_value "$SMACKEREL_E2E_UI_ENV_FILE" "CORE_EXTERNAL_URL")"
  wait_timeout_s="$(smackerel_env_value "$SMACKEREL_E2E_UI_ENV_FILE" "COMPOSE_WAIT_TIMEOUT_S")"

  if [[ -z "$core_url" ]]; then
    echo "ERROR: CORE_EXTERNAL_URL missing from $SMACKEREL_E2E_UI_ENV_FILE; cannot derive SMACKEREL_BASE_URL." >&2
    return 1
  fi
  if [[ -z "$wait_timeout_s" ]]; then
    echo "ERROR: COMPOSE_WAIT_TIMEOUT_S missing from $SMACKEREL_E2E_UI_ENV_FILE." >&2
    return 1
  fi

  # Fail-loud SST consumer: Playwright config requires SMACKEREL_BASE_URL
  # (web/pwa/tests/_support/env.ts). The disposable test stack derives it
  # from CORE_EXTERNAL_URL — no silent default, no hardcoded localhost.
  export SMACKEREL_BASE_URL="$core_url"

  # Install the teardown trap BEFORE bringing the stack up so a failed
  # `up` still triggers cleanup.
  trap 'tear_down_test_stack' EXIT
  trap 'tear_down_test_stack; trap - INT;  kill -INT  $$' INT
  trap 'tear_down_test_stack; trap - TERM; kill -TERM $$' TERM

  echo "[web-e2e-ui] Bringing up disposable test stack (project ${SMACKEREL_E2E_UI_COMPOSE_PROJECT}, wait ${wait_timeout_s}s)..." >&2
  # Pre-clean any leftover lane state from a prior aborted run so a
  # restart cannot inherit a stale container/volume set.
  e2e_ui_compose down --remove-orphans --volumes --timeout 60 >&2 || true
  e2e_ui_compose up -d --wait --wait-timeout "$wait_timeout_s"
}

# Default action: bring up the disposable test stack under the
# dedicated Compose project, invoke the Playwright runner against it,
# and tear the stack down on exit.
#
# Seam handoff with the SCOPE-1a dispatcher canary (TP-077-01-04):
# when `SMACKEREL_E2E_UI_NPX` is set, the caller has injected a stub
# binary in place of `npx` (no docker, no network) — skip the live
# stack lifecycle so the canary can still assert exit-code propagation
# without bringing up the real stack. The lifecycle is exercised
# end-to-end by the SCOPE-1c proof-of-life suite (TP-077-01-01 +
# TP-077-01-01R) under `./smackerel.sh test e2e-ui` in CI.
# bootstrap_pwa_tooling — ensures the PWA workspace has its npm
# dependencies and the Playwright Chromium browser installed before
# `run_node_tooling` invokes `npx playwright test`. A fresh clone (or
# a freshly-cleaned `node_modules`) would otherwise fail with "Cannot
# find module '@playwright/test'" or "Executable doesn't exist at
# .../chromium-*/chrome-linux/chrome". Idempotent — fast no-op on a
# warm workspace. Skipped when the dispatcher canary injects
# `SMACKEREL_E2E_UI_NPX` (no real Node tooling on the path).
bootstrap_pwa_tooling() {
  if [[ -n "${SMACKEREL_E2E_UI_NPX:-}" ]]; then
    return 0
  fi
  if [[ ! -d "$SMACKEREL_E2E_UI_PWA_DIR" ]]; then
    return 0
  fi
  local need_npm_ci=0
  local need_browser_install=0
  if [[ ! -d "$SMACKEREL_E2E_UI_PWA_DIR/node_modules" ]]; then
    need_npm_ci=1
  fi
  local browser_cache="${PLAYWRIGHT_BROWSERS_PATH:-$HOME/.cache/ms-playwright}"
  if ! compgen -G "$browser_cache/chromium-*" >/dev/null; then
    need_browser_install=1
  fi
  if (( need_npm_ci == 0 && need_browser_install == 0 )); then
    return 0
  fi
  (
    cd "$SMACKEREL_E2E_UI_PWA_DIR"
    if (( need_npm_ci == 1 )); then
      echo "[web-e2e-ui] Bootstrapping web/pwa npm dependencies (npm ci)..." >&2
      npm ci
    fi
    if (( need_browser_install == 1 )); then
      echo "[web-e2e-ui] Installing Playwright chromium browser..." >&2
      npx playwright install chromium
    fi
  )
}

if [[ -z "${SMACKEREL_E2E_UI_NPX:-}" ]]; then
  bootstrap_pwa_tooling
  bring_up_test_stack
fi
run_node_tooling "$@"
