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

# resolve_playwright_browser_cache — echo the directory Playwright uses to
# cache downloaded browser builds, per OS. Honors an explicit
# PLAYWRIGHT_BROWSERS_PATH override first (Playwright's own precedence),
# then falls back to the correct per-OS default:
#   * macOS (Darwin): $HOME/Library/Caches/ms-playwright
#   * Linux / other:  $HOME/.cache/ms-playwright
# Using the Linux default on macOS makes the warm-cache probe in
# bootstrap_pwa_tooling never match, which forces a needless — and on some
# Docker-Desktop hosts, deadlock-prone — `npx playwright install` on EVERY
# invocation. Detection uses `uname -s`; an optional first argument
# overrides the detected OS so the spec-077 shell unit can lock the path
# logic deterministically (WSL/macOS portability per the repo convention).
resolve_playwright_browser_cache() {
  if [[ -n "${PLAYWRIGHT_BROWSERS_PATH:-}" ]]; then
    printf '%s\n' "$PLAYWRIGHT_BROWSERS_PATH"
    return 0
  fi
  local os_name="${1:-$(uname -s 2>/dev/null || printf 'Linux')}"
  case "$os_name" in
    Darwin) printf '%s\n' "$HOME/Library/Caches/ms-playwright" ;;
    *) printf '%s\n' "$HOME/.cache/ms-playwright" ;;
  esac
}

# bootstrap_pwa_tooling — ensures the PWA workspace has its npm
# dependencies and the Playwright browsers installed before
# `run_node_tooling` invokes `npx playwright test`. A fresh clone (or a
# freshly-cleaned `node_modules`) would otherwise fail with "Cannot find
# module '@playwright/test'" or "Executable doesn't exist at
# .../chromium-*/chrome-*/...". Idempotent — a warm cache is a fast no-op
# that never invokes `npx` at all. Skipped when the dispatcher canary
# injects `SMACKEREL_E2E_UI_NPX` (no real Node tooling on the path).
#
# The PWA suite launches BOTH the full `chromium` build and the
# `chromium-headless-shell` build (the tests run headless), AND
# playwright.config.ts sets `video: "retain-on-failure"`, so Playwright
# starts its bundled `ffmpeg` binary when a browser context is created —
# a missing ffmpeg makes EVERY `newPage`/`newContext` throw "Executable
# doesn't exist at .../ffmpeg-<rev>/ffmpeg-<os>" (spec 100 F-100-OPT-01,
# discovered once the ollama de-weighting let the stack come up). The
# warm-cache probe is REVISION-EXACT: Playwright launches the precise
# revision it pins in playwright-core/browsers.json, so a stale
# different-revision dir (e.g. a leftover `ffmpeg-1011` when Playwright
# pins `ffmpeg-1010`) must NOT count as present — a revision-blind
# `ffmpeg-*` glob is fooled by it, skips the install, and leaves the lane
# red. Any of the three pinned-revision dirs missing triggers a single
# COMBINED install (already-correct components are a fast no-op, so only
# the missing revision is fetched). `npx playwright install chromium
# chromium-headless-shell` does NOT pull ffmpeg on recent Playwright — it
# is a separately-named component — which is why it is listed explicitly
# below. Defined (with resolve_pinned_playwright_revision) above the
# sourced-guard so the spec-077 shell unit can source this file and lock
# the probe logic without bringing up a stack.

# resolve_pinned_playwright_revision — echo the EXACT revision Playwright
# pins for a component (chromium / chromium-headless-shell / ffmpeg), read
# from node_modules/playwright-core/browsers.json. Empty when browsers.json
# is absent (fresh clone before `npm ci`) or the component is unlisted;
# callers treat empty as "install needed". Pure awk (no node / python) so
# the spec-077 shell unit can source + drive it hermetically on any host
# (wsl-macos-compatibility). browsers.json is machine-generated and stable:
# each component object lists "name" immediately before "revision".
resolve_pinned_playwright_revision() {
  local component="$1"
  local browsers_json="$SMACKEREL_E2E_UI_PWA_DIR/node_modules/playwright-core/browsers.json"
  [[ -r "$browsers_json" ]] || return 0
  awk -v want="$component" '
    $0 ~ ("\"name\"[[:space:]]*:[[:space:]]*\"" want "\"") { found = 1 }
    found && /"revision"[[:space:]]*:/ { gsub(/[^0-9]/, ""); print; exit }
  ' "$browsers_json"
}

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
  local browser_cache chromium_rev headless_rev ffmpeg_rev
  browser_cache="$(resolve_playwright_browser_cache)"
  chromium_rev="$(resolve_pinned_playwright_revision chromium)"
  headless_rev="$(resolve_pinned_playwright_revision chromium-headless-shell)"
  ffmpeg_rev="$(resolve_pinned_playwright_revision ffmpeg)"
  # Warm cache requires all three components at their EXACT pinned revision:
  # the full chromium build, the chromium-headless-shell build (headless
  # tests), AND the ffmpeg binary (playwright.config.ts
  # `video: retain-on-failure` needs it at newPage). Revision-EXACT, NOT a
  # `<component>-*` glob: a stale different-revision dir (e.g. a leftover
  # ffmpeg-1011 when Playwright pins ffmpeg-1010) must NOT count as present,
  # or every browser newPage throws "Executable doesn't exist at
  # .../<component>-<pinned>/..." (F-100-OPT-01). An unresolved revision (no
  # node_modules yet) also forces the install; it runs after `npm ci`. The
  # cache dir names the headless-shell component with underscores.
  if [[ -z "$chromium_rev" || -z "$headless_rev" || -z "$ffmpeg_rev" ]] \
    || [[ ! -d "$browser_cache/chromium-$chromium_rev" ]] \
    || [[ ! -d "$browser_cache/chromium_headless_shell-$headless_rev" ]] \
    || [[ ! -d "$browser_cache/ffmpeg-$ffmpeg_rev" ]]; then
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
      echo "[web-e2e-ui] Installing Playwright chromium + chromium-headless-shell + ffmpeg..." >&2
      # Single combined install so all three components are fetched together
      # and Playwright's cache GC cannot evict one while installing another.
      # ffmpeg is listed explicitly: recent Playwright does NOT pull it as a
      # side effect of a browser install, and playwright.config.ts's
      # `video: retain-on-failure` needs it at browser-context creation
      # (F-100-OPT-01). Already-present components are a fast no-op.
      npx playwright install chromium chromium-headless-shell ffmpeg
    fi
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
  #
    # Spec 100 F-100-OPT-01 + F-100-OPT-03 — the base docker-compose.yml is
    # layered with a TEST-ONLY override (docker-compose.e2e-ui.override.yml)
    # that (a) swaps the `ollama` service for a tiny nginx:alpine stub
    # (F-100-OPT-01) and (b) profile-gates the 2 GB `smackerel-ml` sidecar OFF
    # (F-100-OPT-03). The shared SST test env emits COMPOSE_PROFILES=ollama
    # (environments.test.ollama_enabled=true), which --env-file activates
    # natively, so without the override this lane would pull the ~3 GB
    # heavyweight ollama image and stall `up --wait` on a macOS Docker host. The
    # browser UI journeys never run GPU inference or ML embedding (J5 is
    # ENV-CONSTRAINED); core only needs the ollama endpoint REACHABLE at boot
    # and does NOT boot-depend on ml (depends_on = postgres+nats; /api/health
    # excludes ml; ML-readiness is a background goroutine with text fallback).
  # The override is loaded ONLY here — the prod stack (deploy/compose.deploy.yml)
  # and the dev/integration/e2e lanes (smackerel_compose) are untouched.
  docker compose \
    --project-name "$SMACKEREL_E2E_UI_COMPOSE_PROJECT" \
    --env-file "$SMACKEREL_E2E_UI_ENV_FILE" \
    -f "$SMACKEREL_E2E_UI_REPO_ROOT/docker-compose.yml" \
    -f "$SMACKEREL_E2E_UI_REPO_ROOT/docker-compose.e2e-ui.override.yml" \
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

  # Spec 077 SCOPE-3 — the auth_login.spec.ts test suite needs the
  # shared dev token to POST /v1/web/login through the disposable test
  # stack (AuthConfig.Enabled=false → constant-time compare against
  # SMACKEREL_AUTH_TOKEN). Sourced from the same SST env file; fail
  # loud if missing so we never silently skip login coverage.
  local auth_token
  auth_token="$(smackerel_env_value "$SMACKEREL_E2E_UI_ENV_FILE" "SMACKEREL_AUTH_TOKEN")"
  if [[ -z "$auth_token" ]]; then
    echo "ERROR: SMACKEREL_AUTH_TOKEN missing from $SMACKEREL_E2E_UI_ENV_FILE; cannot drive spec 077 SCOPE-3 login tests." >&2
    return 1
  fi
  export SMACKEREL_AUTH_TOKEN="$auth_token"

  # Install the teardown trap BEFORE bringing the stack up so a failed
  # `up` still triggers cleanup.
  trap 'tear_down_test_stack' EXIT
  trap 'tear_down_test_stack; trap - INT;  kill -INT  $$' INT
  trap 'tear_down_test_stack; trap - TERM; kill -TERM $$' TERM

  echo "[web-e2e-ui] Bringing up disposable test stack (project ${SMACKEREL_E2E_UI_COMPOSE_PROJECT}, wait ${wait_timeout_s}s)..." >&2
  # Pre-clean any leftover lane state from a prior aborted run so a
  # restart cannot inherit a stale container/volume set.
  e2e_ui_compose down --remove-orphans --volumes --timeout 60 >&2 || true
  # Build-fresh before starting so a run ALWAYS reflects current source. `--build`
  # rebuilds any service image whose build context changed (here: smackerel-core
  # from ./Dockerfile) and reuses the layer cache when nothing changed. Without it
  # `up` silently reuses a stale prebuilt smackerel-core image, so a green run can
  # mask an unbuilt change (and a red run can reflect stale code) — the correctness
  # hazard this lane previously hit. This mirrors the explicit build->up freshness
  # convention the Go live-stack lanes use in
  # tests/integration/test_runtime_health.sh (`smackerel.sh --env test build` then
  # `up`); `up --build` is the profile-faithful equivalent for this lane because it
  # builds exactly the services it starts — the 2 GB smackerel-ml sidecar stays
  # profile-gated OFF (docker-compose.e2e-ui.override.yml F-100-OPT-03) and is
  # never built, and the nginx/postgres/nats images are pulls, not builds.
  e2e_ui_compose up -d --wait --wait-timeout "$wait_timeout_s" --build
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
# `bootstrap_pwa_tooling` and `resolve_playwright_browser_cache` are
# defined ABOVE the sourced-guard (near `run_node_tooling`) so the
# spec-077 shell unit can source this file and lock the OS-correct
# browser-cache path logic without bringing up a stack. On the happy path
# (npx not stubbed) they run here before the live stack comes up.
if [[ -z "${SMACKEREL_E2E_UI_NPX:-}" ]]; then
  bootstrap_pwa_tooling
  bring_up_test_stack
fi
run_node_tooling "$@"
