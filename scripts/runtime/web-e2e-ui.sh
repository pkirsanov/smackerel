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
  #
  # F-080-04-LANE — optional THIRD compose file. A lane phase MAY layer one
  # extra file on top of the two above by exporting
  # SMACKEREL_E2E_UI_EXTRA_COMPOSE_FILE; it is appended LAST so its keys win
  # over both the base file and the e2e-ui override. This is a HARNESS PHASE
  # SELECTOR, not runtime configuration: it names a repo-local compose file,
  # carries no service value, and has NO default — when the variable is unset
  # the compose argv is byte-for-byte what it was before, so the pre-existing
  # single-phase lane is untouched. When it IS set it must resolve to a real
  # file; a bad path fails loud HERE rather than silently degrading to the
  # base-only stack (smackerel-no-defaults / Gate G028).
  #
  # This mirrors the SMACKEREL_COMPOSE_OVERRIDE_FILE hook in
  # scripts/lib/runtime.sh::smackerel_compose, which this lane cannot reuse
  # because it invokes docker compose directly (it must pin --project-name to
  # `smackerel-test-e2e-ui` rather than inherit the env-file COMPOSE_PROJECT).
  # A distinct variable name keeps the two hooks from cross-triggering.
  #
  # Because BOTH `up` and `down` funnel through this one helper, a phase that
  # exports the variable gets a symmetric bring-up/teardown pair for free.
  local -a extra_compose_file_args=()
  if [[ -n "${SMACKEREL_E2E_UI_EXTRA_COMPOSE_FILE:-}" ]]; then
    if [[ ! -f "$SMACKEREL_E2E_UI_EXTRA_COMPOSE_FILE" ]]; then
      echo "ERROR: SMACKEREL_E2E_UI_EXTRA_COMPOSE_FILE is set but is not a readable file: $SMACKEREL_E2E_UI_EXTRA_COMPOSE_FILE" >&2
      return 1
    fi
    extra_compose_file_args=(-f "$SMACKEREL_E2E_UI_EXTRA_COMPOSE_FILE")
  fi

  docker compose \
    --project-name "$SMACKEREL_E2E_UI_COMPOSE_PROJECT" \
    --env-file "$SMACKEREL_E2E_UI_ENV_FILE" \
    -f "$SMACKEREL_E2E_UI_REPO_ROOT/docker-compose.yml" \
    -f "$SMACKEREL_E2E_UI_REPO_ROOT/docker-compose.e2e-ui.override.yml" \
    ${extra_compose_file_args[@]+"${extra_compose_file_args[@]}"} \
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

# ------------------------------------------------------------------
# true-empty phase — "an authorized graph that legitimately holds nothing"
# ------------------------------------------------------------------
#
# web/pwa/tests/graph-activation.spec.ts carries a STATE_TRUE_EMPTY arm whose
# assertions are strong and specific — zero rows, the exact copy "Nothing has
# been synthesized into your knowledge graph yet", an action href of
# /pwa/connectors.html (a capture/source next step, NOT a retry), no
# unavailable/error/failed/retry text, and role="status" rather than "alert".
# None of it had ever executed. cmd/core/services.go unconditionally calls
# graph.SeedHospitalityTopics on EVERY startup, so the topics family is never
# empty on a booted stack and the arm was unreachable (F-080-05-SEED).
#
# SCN-080-001-05 is a CONDITIONAL: *when* every authorized family read succeeds
# with zero records, the user sees the true-empty state. Creating that
# condition inside a DISPOSABLE lane stack is ordinary test-fixture setup, not
# a product change. It does NOT resolve F-080-05-SEED — whether a
# seeded-but-unlinked taxonomy should count as user content remains an open
# product question — it proves the UI contract holds when the condition occurs.
#
# Placement is load-bearing. This phase runs immediately after
# bring_up_test_stack and BEFORE the full suite, because that is the only
# moment the stack is guaranteed fresh: on a just-booted stack `people` and
# `places` are ALREADY empty (no artifacts, no location clusters), so clearing
# the boot-seeded `topics` is the single remaining step to an all-family empty
# graph. Later in the lane that no longer holds.
#
# Scoping is by Compose project, through the same e2e_ui_compose helper every
# other lane operation uses, so only `smackerel-test-e2e-ui`'s containers are
# touched. Credentials are read from the postgres container's OWN environment
# and never cross the host shell or reach the transcript.

# clear_seeded_taxonomy — delete the boot-seeded hospitality topics from THIS
# lane's postgres. `topics.parent_id` is the only FK into the table and it is
# self-referential, so a single whole-table DELETE satisfies referential
# integrity at end-of-statement; the seeded rows carry no parent anyway.
clear_seeded_taxonomy() {
  local deleted status
  set +e
  # SC2016 is deliberate here, not an oversight: the single quotes keep the
  # HOST shell from expanding $POSTGRES_PASSWORD / $POSTGRES_USER / $POSTGRES_DB
  # so the credentials are read inside the container from its OWN environment
  # and never traverse this shell, its argv, or the session transcript.
  #
  # S-6: bounded SERVER-SIDE. lock_timeout caps waiting on a lock the boot seed
  # may still hold; statement_timeout caps the DELETE itself. Both surface as a
  # loud psql error instead of the silent unbounded hang this call used to risk.
  # Deliberately NOT wrapped in `timeout`: e2e_ui_compose is a bash function, and
  # timeout(1) execs a binary, so wrapping it fails with exit 126 rather than
  # bounding anything.
  # shellcheck disable=SC2016
  deleted="$(e2e_ui_compose exec -T postgres sh -c \
    'PGPASSWORD="$POSTGRES_PASSWORD" psql -qtAX -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -c "SET lock_timeout = '"'"'10s'"'"'; SET statement_timeout = '"'"'30s'"'"'; DELETE FROM topics;"' 2>&1)"
  status=$?
  set -e
  if [[ "$status" -ne 0 ]]; then
    echo "ERROR: [web-e2e-ui] true-empty phase could not clear the boot-seeded taxonomy (exit ${status})." >&2
    echo "       psql reported: ${deleted}" >&2
    return "$status"
  fi
  if [[ -n "$deleted" ]]; then
    echo "[web-e2e-ui] true-empty phase: cleared the boot-seeded taxonomy (psql: ${deleted})" >&2
  else
    # -q suppresses the DELETE command tag, so silence here is success, not a
    # no-op. Say that outright rather than printing an empty field a reader
    # would have to interpret — the family guard below is the actual proof.
    echo "[web-e2e-ui] true-empty phase: cleared the boot-seeded taxonomy (psql exited 0; -q prints no command tag)" >&2
  fi
  return 0
}

# assert_all_graph_families_true_empty — refuse to run the specs unless ALL
# THREE authorized family reads answer HTTP 200 with an empty items array.
#
# This guard is the entire point of the phase, not a nicety.
# graph-activation.spec.ts is state-adaptive by design: it reads whatever the
# stack publishes and asserts the matching arm. That is what keeps it honest on
# any stack — and is exactly what would let this phase quietly degrade into a
# second READY run, report green, and prove nothing. Reading the same family
# routes the browser reads, and refusing unless every one of them is an empty
# 200, is what makes a green phase mean the true-empty arm actually executed.
#
# 200-with-empty-items specifically, never a 503 or a 5xx: web/pwa/wiki_state.js
# resolves zero items under CODE_OK to STATE_TRUE_EMPTY (emptyPermitted defaults
# true), whereas a fault code resolves to a fault state and would send the spec
# down a different branch.
#
# The routes are behind `knowledge-graph:read`, so the probe presents the same
# lane token the spec's session cookie carries — an unauthenticated probe would
# see 401 and could never observe the condition under test.
assert_all_graph_families_true_empty() {
  local label="$1"
  if ! command -v curl >/dev/null 2>&1; then
    echo "ERROR: [web-e2e-ui] true-empty phase needs 'curl' to prove the graph is empty but it is not on PATH." >&2
    return 127
  fi

  local family response http_code body
  for family in topics people places; do
    # No --fail: a non-200 is exactly what this guard must be able to REPORT,
    # and --fail would discard the body that identifies it. --write-out appends
    # the status on its own final line so one call yields both.
    if ! response="$(curl --silent --show-error --max-time 15 \
      --write-out '\n%{http_code}' \
      --header "Authorization: Bearer $SMACKEREL_AUTH_TOKEN" \
      --header 'Accept: application/json' \
      "$SMACKEREL_BASE_URL/api/${family}?limit=5" 2>/dev/null)"; then
      echo "ERROR: [web-e2e-ui] true-empty phase (${label}) could not reach /api/${family}." >&2
      echo "       curl transport failure; observed: ${response}" >&2
      return 1
    fi
    http_code="${response##*$'\n'}"
    body="${response%$'\n'*}"
    if [[ "$http_code" != "200" ]] || ! printf '%s' "$body" | grep -qF '"items":[]'; then
      echo "ERROR: [web-e2e-ui] true-empty phase (${label}) did NOT observe an all-family empty graph." >&2
      echo "       Required: every authorized family answers HTTP 200 with a body carrying \"items\":[]." >&2
      echo "       Observed /api/${family}?limit=5: HTTP ${http_code} body: ${body}" >&2
      echo "       graph-activation.spec.ts would exercise a different arm and prove nothing about SCN-080-001-05." >&2
      return 1
    fi
    echo "[web-e2e-ui] true-empty phase (${label}): GET /api/${family}?limit=5 answers HTTP ${http_code} ${body}" >&2
  done
  return 0
}

# await_core_healthy — bounded wait for THIS lane's core container to report a
# healthy Docker healthcheck again after a restart. Bounded by the SST's own
# COMPOSE_WAIT_TIMEOUT_S (the same budget `up --wait` is given), read fail-loud
# rather than guessed: no default is invented here (Gate G028).
await_core_healthy() {
  local wait_timeout_s core_cid attempts attempt health
  wait_timeout_s="$(smackerel_env_value "$SMACKEREL_E2E_UI_ENV_FILE" "COMPOSE_WAIT_TIMEOUT_S")"
  if [[ -z "$wait_timeout_s" ]]; then
    echo "ERROR: [web-e2e-ui] COMPOSE_WAIT_TIMEOUT_S missing from $SMACKEREL_E2E_UI_ENV_FILE; refusing to guess a health-wait bound." >&2
    return 1
  fi
  core_cid="$(e2e_ui_compose ps -q smackerel-core)"
  if [[ -z "$core_cid" ]]; then
    echo "ERROR: [web-e2e-ui] true-empty phase could not resolve the smackerel-core container for project ${SMACKEREL_E2E_UI_COMPOSE_PROJECT}." >&2
    return 1
  fi

  attempts=$((wait_timeout_s / 2))
  ((attempts > 0)) || attempts=1
  attempt=0
  health="not-yet-probed"
  while ((attempt < attempts)); do
    attempt=$((attempt + 1))
    health="$(docker inspect --format '{{.State.Health.Status}}' "$core_cid" 2>/dev/null || printf 'inspect-failed')"
    if [[ "$health" == "healthy" ]]; then
      echo "[web-e2e-ui] true-empty phase: smackerel-core reports healthy again (after ${attempt} probe(s))." >&2
      return 0
    fi
    sleep 2
  done

  echo "ERROR: [web-e2e-ui] smackerel-core did not become healthy within ${wait_timeout_s}s of the restore restart." >&2
  echo "       Last observed container health status: ${health}" >&2
  return 1
}

# assert_seeded_taxonomy_restored — prove the boot seed actually came back.
# "Container healthy" alone would not: it only says /api/health answers. This
# reads the same family route the next phase's tests depend on and requires it
# to be NON-empty, which is the direct observable of SeedHospitalityTopics
# having re-run. Bounded, never an unbounded loop.
assert_seeded_taxonomy_restored() {
  local attempt=0 response http_code body
  http_code="not-yet-probed"
  body=""
  while ((attempt < 30)); do
    attempt=$((attempt + 1))
    if response="$(curl --silent --show-error --max-time 15 \
      --write-out '\n%{http_code}' \
      --header "Authorization: Bearer $SMACKEREL_AUTH_TOKEN" \
      --header 'Accept: application/json' \
      "$SMACKEREL_BASE_URL/api/topics?limit=5" 2>/dev/null)"; then
      http_code="${response##*$'\n'}"
      body="${response%$'\n'*}"
      if [[ "$http_code" == "200" ]] && ! printf '%s' "$body" | grep -qF '"items":[]'; then
        echo "[web-e2e-ui] true-empty phase: boot seed RESTORED — GET /api/topics?limit=5 answers HTTP ${http_code} ${body}" >&2
        return 0
      fi
    else
      http_code="transport-failure"
      body="$response"
    fi
    sleep 2
  done

  echo "ERROR: [web-e2e-ui] the boot-seeded taxonomy did NOT come back after restarting smackerel-core." >&2
  echo "       Last observed /api/topics?limit=5: HTTP ${http_code} body: ${body}" >&2
  return 1
}

# restore_seeded_taxonomy — put the stack back exactly as the full suite
# expects to find it. Restarting core re-runs SetupServices, which calls the
# explicitly idempotent SeedHospitalityTopics (ON CONFLICT (name) DO NOTHING)
# before the HTTP listener binds. The wait is what makes this safe to hand on:
# the full suite runs next and must not inherit a de-seeded database.
restore_seeded_taxonomy() {
  local status
  echo "[web-e2e-ui] true-empty phase: restarting smackerel-core so the boot seed re-runs..." >&2
  set +e
  e2e_ui_compose restart --timeout 30 smackerel-core >&2
  status=$?
  set -e
  if [[ "$status" -ne 0 ]]; then
    echo "ERROR: [web-e2e-ui] true-empty phase could not restart smackerel-core (exit ${status})." >&2
    return "$status"
  fi
  await_core_healthy || return $?
  assert_seeded_taxonomy_restored || return $?
  return 0
}

# true_empty_phase_applies — gate the phase on the caller's own Playwright
# filter. Identical predicate to the two phases below (all three run exactly
# the one activation-dependent spec), named separately so the gates read
# independently and can diverge without a rename.
true_empty_phase_applies() {
  graph_disabled_phase_applies "$@"
}

run_true_empty_phase() {
  local status=0
  local restore_status=0
  # `SECONDS` counts from shell start, so a snapshot here yields this phase's
  # own wall-clock cost — the lane's added price is measurable, not estimated.
  local phase_start="$SECONDS"

  echo "" >&2
  echo "[web-e2e-ui] true-empty phase: emptying every authorized graph family on the FRESH stack (project ${SMACKEREL_E2E_UI_COMPOSE_PROJECT})..." >&2

  set +e
  clear_seeded_taxonomy
  status=$?
  set -e

  if [[ "$status" -eq 0 ]]; then
    set +e
    assert_all_graph_families_true_empty before-specs
    status=$?
    set -e
  fi

  if [[ "$status" -eq 0 ]]; then
    echo "[web-e2e-ui] true-empty phase: running graph-activation.spec.ts against the ALL-FAMILY-EMPTY stack..." >&2
    set +e
    # Scoped to the ONE spec whose assertions are graph-state dependent.
    # Re-running the whole suite here would cost minutes and blur the phases'
    # result summaries together.
    run_node_tooling graph-activation.spec.ts
    status=$?
    set -e
  fi

  # Re-prove emptiness AFTER the specs ran. The guard above establishes the
  # condition at one instant; this closes the window, so the graph is known to
  # have been empty for the WHOLE run rather than only at its start. That
  # matters because the spec adapts to whatever it observes: had rows appeared
  # mid-run the browser would have painted the ready view and the phase would
  # still have reported green.
  if [[ "$status" -eq 0 ]]; then
    set +e
    assert_all_graph_families_true_empty after-specs
    status=$?
    set -e
    if [[ "$status" -ne 0 ]]; then
      echo "ERROR: [web-e2e-ui] the graph refilled during the true-empty phase; the specs may have exercised a populated arm." >&2
    fi
  fi

  # Diagnostic only — the bracket above already decided the outcome, so a
  # missing extraction here can neither pass nor fail the phase. The
  # store-exclusivity evidence line names the state the browser actually
  # painted on the topics index, which the TTY list reporter overwrites in the
  # live transcript but the JSON reporter records verbatim. In this phase it
  # should read `painted=true-empty`.
  local painted_arm=""
  if [[ -r "$SMACKEREL_E2E_UI_PWA_DIR/test-results/results.json" ]]; then
    painted_arm="$(grep -o 'GRAPH-EV [^"]*' "$SMACKEREL_E2E_UI_PWA_DIR/test-results/results.json" || true)"
  fi
  if [[ -n "$painted_arm" ]]; then
    echo "[web-e2e-ui] true-empty phase: browser painted $painted_arm" >&2
  fi

  # RESTORE — on EVERY exit path above, including a failure before the specs
  # ran. The full suite runs next on this same stack and must not inherit a
  # de-seeded database.
  set +e
  restore_seeded_taxonomy
  restore_status=$?
  set -e
  if [[ "$restore_status" -ne 0 ]]; then
    echo "ERROR: [web-e2e-ui] true-empty phase FAILED TO RESTORE the boot seed — the full suite that runs next would inherit a de-seeded database." >&2
    # S-1: tell the CALLER the fixture is untrustworthy, not merely that the
    # phase failed. Without this the lane recorded the failure and then ran the
    # full suite anyway against the de-seeded database; because the specs are
    # state-adaptive they would paint the true-empty arm and PASS, printing a
    # green suite result beside this red line.
    TRUE_EMPTY_FIXTURE_UNTRUSTWORTHY=1
    # First failure wins within the phase: a spec failure stays the reported
    # cause, but a restore failure can never pass silently.
    if [[ "$status" -eq 0 ]]; then
      status="$restore_status"
    fi
  fi

  local phase_seconds=$((SECONDS - phase_start))
  if [[ "$status" -eq 0 ]]; then
    echo "[web-e2e-ui] true-empty phase: PASS (${phase_seconds}s)" >&2
  else
    echo "[web-e2e-ui] true-empty phase: FAIL (exit=${status}, ${phase_seconds}s)" >&2
  fi
  return "$status"
}

# ------------------------------------------------------------------
# store-unavailable phase — "the database is down"
# ------------------------------------------------------------------
#
# Losing the graph store is a real, user-facing failure mode with its own
# exclusive UI state (store-unavailable) and its own copy, and until now it
# had no live-container proof at all: the lane's stack always boots a healthy
# postgres, so the store arm of web/pwa/tests/graph-activation.spec.ts never
# executed against a real outage.
#
# It is induced by STOPPING this lane's own postgres service on the stack the
# full suite just finished using. That costs no rebuild and no boot cycle —
# roughly the spec's own runtime — and it is a truthful outage rather than a
# simulated one: core keeps running (that IS the fail-soft contract), the
# family reads answer a typed 503, and the browser sees exactly what a user
# would see if the database fell over mid-session.
#
# Scoping is by Compose project, through the same e2e_ui_compose helper every
# other lane operation uses, so only `smackerel-test-e2e-ui`'s postgres is
# touched. The dev stack and the `smackerel-test` integration stack are
# unaffected. The service declares `restart: unless-stopped`, so an explicit
# `stop` stays stopped for the duration of the phase.

# assert_graph_store_unavailable — refuse to run the specs unless the store
# really is unavailable.
#
# This guard is the entire point of the phase, not a nicety.
# graph-activation.spec.ts is state-adaptive by design: it reads whatever the
# stack publishes and asserts the matching arm. That is what makes it honest
# on any stack — and it is also exactly what would let this phase quietly
# degrade into a second HEALTHY run, report green, and prove nothing. Reading
# the same family route the browser reads, and refusing unless it answers a
# typed 503 store_unavailable, is what makes a green phase mean the store arm
# actually executed.
#
# The route is behind `knowledge-graph:read`, so the probe presents the same
# lane token the spec's session cookie carries — an unauthenticated probe
# would see 401 and could never observe the condition under test.
assert_graph_store_unavailable() {
  local label="$1"
  if ! command -v curl >/dev/null 2>&1; then
    echo "ERROR: [web-e2e-ui] store-unavailable phase needs 'curl' to prove the store is down but it is not on PATH." >&2
    return 127
  fi

  # Stopping a container kills its connections, but the core's pool may take a
  # moment to observe that, so poll for the induced condition rather than
  # sampling once. Bounded at 30 x 2s: this waits for a condition to become
  # OBSERVABLE, and still fails loudly if it never does.
  local attempt=0
  local response="" http_code="" body=""
  while (( attempt < 30 )); do
    attempt=$((attempt + 1))
    # No --fail: 503 is the EXPECTED answer here, and --fail would discard the
    # very body that proves which 503 this is. --write-out appends the status
    # on its own final line so one call yields both.
    if response="$(curl --silent --show-error --max-time 15 \
      --write-out '\n%{http_code}' \
      --header "Authorization: Bearer $SMACKEREL_AUTH_TOKEN" \
      --header 'Accept: application/json' \
      "$SMACKEREL_BASE_URL/api/topics?limit=5" 2>/dev/null)"; then
      http_code="${response##*$'\n'}"
      body="${response%$'\n'*}"
      if [[ "$http_code" == "503" ]] && printf '%s' "$body" | grep -q '"code":"store_unavailable"'; then
        echo "[web-e2e-ui] store-unavailable phase (${label}): GET /api/topics?limit=5 answers HTTP $http_code $body" >&2
        return 0
      fi
    else
      http_code="transport-failure"
      body="$response"
    fi
    sleep 2
  done

  echo "ERROR: [web-e2e-ui] store-unavailable phase (${label}) did NOT observe an unavailable graph store." >&2
  echo "       Required: HTTP 503 whose body carries \"code\":\"store_unavailable\"." >&2
  echo "       Observed after ${attempt} attempts: HTTP ${http_code} body: ${body}" >&2
  echo "       graph-activation.spec.ts would exercise a different arm and prove nothing about this failure mode." >&2
  return 1
}

# store_unavailable_phase_applies — gate the phase on the caller's own
# Playwright filter. Identical predicate to graph_disabled_phase_applies (both
# phases run exactly the one activation-dependent spec), named separately so
# the two gates read independently and can diverge without a rename.
store_unavailable_phase_applies() {
  graph_disabled_phase_applies "$@"
}

run_store_unavailable_phase() {
  local status=0
  # `SECONDS` counts from shell start, so a snapshot here yields this phase's
  # own wall-clock cost — the lane's added price is measurable, not estimated.
  local phase_start="$SECONDS"

  echo "" >&2
  echo "[web-e2e-ui] store-unavailable phase: stopping the graph store on the running stack (project ${SMACKEREL_E2E_UI_COMPOSE_PROJECT})..." >&2

  set +e
  e2e_ui_compose stop --timeout 30 postgres >&2
  status=$?
  set -e
  if [[ "$status" -ne 0 ]]; then
    echo "ERROR: [web-e2e-ui] store-unavailable phase could not stop the postgres service (exit ${status})." >&2
  fi

  if [[ "$status" -eq 0 ]]; then
    set +e
    assert_graph_store_unavailable before-specs
    status=$?
    set -e
  fi

  if [[ "$status" -eq 0 ]]; then
    echo "[web-e2e-ui] store-unavailable phase: running graph-activation.spec.ts against the STORE-DOWN stack..." >&2
    set +e
    # Scoped to the ONE spec whose assertions are store-state dependent.
    # Re-running the whole suite here would cost minutes and blur the phases'
    # result summaries together.
    run_node_tooling graph-activation.spec.ts
    status=$?
    set -e
  fi

  # Re-prove the outage AFTER the specs ran. The guard above establishes the
  # condition at one instant; this closes the window, so the store is known to
  # have been down for the WHOLE run rather than only at its start. That
  # matters because the spec adapts to whatever it observes: had the store
  # come back mid-run the browser would have painted a healthy view and the
  # phase would still have reported green. Bracketing the run is what makes
  # the arm the specs took deterministic, and unlike the console evidence the
  # spec emits, it survives the reporter's terminal redraw.
  if [[ "$status" -eq 0 ]]; then
    set +e
    assert_graph_store_unavailable after-specs
    status=$?
    set -e
    if [[ "$status" -ne 0 ]]; then
      echo "ERROR: [web-e2e-ui] the graph store recovered during the store-unavailable phase; the specs may have exercised a healthy arm." >&2
    fi
  fi

  # Diagnostic only — the bracket above already decided the outcome, so a
  # missing extraction here can neither pass nor fail the phase. It surfaces
  # the arm the browser actually painted, which the TTY list reporter
  # overwrites in the live transcript but the JSON reporter records verbatim.
  local painted_arm=""
  if [[ -r "$SMACKEREL_E2E_UI_PWA_DIR/test-results/results.json" ]]; then
    painted_arm="$(grep -o 'GRAPH-EV store[^"]*' "$SMACKEREL_E2E_UI_PWA_DIR/test-results/results.json" || true)"
  fi
  if [[ -n "$painted_arm" ]]; then
    echo "[web-e2e-ui] store-unavailable phase: browser painted $painted_arm" >&2
  fi

  # Never hand a half-stopped stack to whatever runs next, on ANY exit path
  # above — including a failure before the specs ran. The teardown is
  # idempotent and project-scoped, the graph-disabled phase opens with its own
  # (which then finds nothing to do and boots fresh), and the EXIT/INT/TERM
  # traps remain the outer safety net.
  tear_down_test_stack

  local phase_seconds=$((SECONDS - phase_start))
  if [[ "$status" -eq 0 ]]; then
    echo "[web-e2e-ui] store-unavailable phase: PASS (${phase_seconds}s)" >&2
  else
    echo "[web-e2e-ui] store-unavailable phase: FAIL (exit=${status}, ${phase_seconds}s)" >&2
  fi
  return "$status"
}

# ------------------------------------------------------------------
# F-080-04-LANE — graph-DISABLED second phase
# ------------------------------------------------------------------
#
# The first phase can only ever boot an ENABLED graph core: the shared SST
# test env always emits a NON-EMPTY KNOWLEDGE_GRAPH_API_CURSOR_SECRET and
# docker-compose.yml sources it via `env_file:` with no per-run override. So
# the activation-dependent assertions in web/pwa/tests/graph-activation.spec.ts
# only ever exercised their ENABLED arm, and the DISABLED arm — the one the
# BUG-080-001 SCOPE-04 rows are about — had no live-container proof.
#
# This second phase boots a FRESH stack with that enabler explicitly empty
# (docker-compose.graph-disabled.override.yml -> SecretEmpty ->
# ActivationDisabled -> policy_disabled) so the disabled arm runs against a
# REAL container over REAL HTTP. The spec file is already state-adaptive and
# needs no edit; nothing here mocks or intercepts anything.
#
# Exactly ONE core is alive at any moment: the enabled stack is torn down
# COMPLETELY before the disabled one starts. The pipeline subscribers in
# cmd/core/services.go use no queue groups, so two concurrent cores would
# double-consume and pollute shared state.

# assert_graph_activation_disabled — refuse to continue unless the freshly
# booted stack actually PUBLISHES the disabled aggregate.
#
# This is the phase's precondition, not decoration. Every assertion in
# graph-activation.spec.ts reads the published aggregate and then asserts
# whichever arm matches, which is what makes the spec honest on either stack —
# and is also exactly what would let this phase silently degrade into a
# duplicate ENABLED run if the overlay ever stopped applying, reporting green
# while proving nothing. Reading the same aggregate the spec reads, and
# refusing unless it says policy_disabled, is what makes a green phase mean
# the disabled branch really executed.
assert_graph_activation_disabled() {
  if ! command -v curl >/dev/null 2>&1; then
    echo "ERROR: [web-e2e-ui] graph-disabled phase needs 'curl' to read the published graph aggregate but it is not on PATH." >&2
    return 127
  fi

  # The `graph` section lives inside the AUTHENTICATED branch of /api/health
  # (capability detail is withheld from anonymous callers, CWE-200), so the
  # probe presents the same lane token the spec's session cookie carries.
  local health
  if ! health="$(curl --silent --show-error --fail --max-time 15 \
    --header "Authorization: Bearer $SMACKEREL_AUTH_TOKEN" \
    --header 'Accept: application/json' \
    "$SMACKEREL_BASE_URL/api/health")"; then
    echo "ERROR: [web-e2e-ui] graph-disabled phase could not read /api/health from the disabled stack." >&2
    return 1
  fi

  if ! printf '%s' "$health" | grep -q '"state":"policy_disabled"'; then
    echo "ERROR: [web-e2e-ui] graph-disabled phase booted a stack that does NOT publish the disabled aggregate." >&2
    echo "       The overlay did not take effect, so graph-activation.spec.ts would exercise its ENABLED arm and prove nothing." >&2
    echo "       observed /api/health body: $health" >&2
    return 1
  fi

  # Diagnostic only — the assertion above already decided the outcome, so a
  # non-matching extraction here can neither pass nor fail the phase.
  local observed
  observed="$(printf '%s' "$health" | grep -o '"activation":"[^"]*","state":"[^"]*","code":"[^"]*"' || true)"
  if [[ -z "$observed" ]]; then
    observed="$health"
  fi
  echo "[web-e2e-ui] graph-disabled phase: stack publishes $observed" >&2
}

# graph_disabled_phase_applies — gate the extra stack cycle on the caller's
# own Playwright filter, mirroring `e2e_graph_disabled_phase_applies` in the
# `./smackerel.sh test e2e` lane. The full lane (no filter) always proves the
# disabled state; a caller who narrowed the run to some other spec should not
# pay minutes for a stack cycle whose one spec they excluded.
graph_disabled_phase_applies() {
  if [[ "$#" -eq 0 ]]; then
    return 0
  fi
  local arg
  for arg in "$@"; do
    if [[ "$arg" == *graph-activation* ]]; then
      return 0
    fi
  done
  return 1
}

run_graph_disabled_phase() {
  local status=0
  # `SECONDS` counts from shell start, so a snapshot here yields this phase's
  # own wall-clock cost — the lane's added price is then measurable rather
  # than estimated.
  local phase_start="$SECONDS"

  echo "" >&2
  echo "[web-e2e-ui] graph-disabled phase: recycling the stack with the graph activation enabler explicitly empty..." >&2

  # Exactly ONE core alive at a time — the enabled stack must be fully gone
  # before the disabled one starts. tear_down_test_stack is idempotent and
  # project-scoped (`down --remove-orphans --volumes`).
  tear_down_test_stack

  # Exported so BOTH the `up` inside bring_up_test_stack and the matching
  # `down` after the phase resolve the SAME compose file set through
  # e2e_ui_compose.
  export SMACKEREL_E2E_UI_EXTRA_COMPOSE_FILE="$SMACKEREL_E2E_UI_REPO_ROOT/docker-compose.graph-disabled.override.yml"

  set +e
  bring_up_test_stack
  status=$?
  set -e
  if [[ "$status" -ne 0 ]]; then
    echo "ERROR: [web-e2e-ui] graph-disabled phase stack failed to start (exit ${status})." >&2
  fi

  if [[ "$status" -eq 0 ]]; then
    set +e
    assert_graph_activation_disabled
    status=$?
    set -e
  fi

  if [[ "$status" -eq 0 ]]; then
    echo "[web-e2e-ui] graph-disabled phase: running graph-activation.spec.ts against the DISABLED stack..." >&2
    set +e
    # Scoped to the ONE spec whose assertions are activation-dependent.
    # Re-running the whole suite here would cost minutes and blur the two
    # phases' result summaries together.
    run_node_tooling graph-activation.spec.ts
    status=$?
    set -e
  fi

  # Symmetric teardown — the overlay is still exported here, so `down`
  # resolves the same compose file set that `up` did. Runs on failure too,
  # and the EXIT/INT/TERM traps remain installed as the outer safety net.
  tear_down_test_stack
  unset SMACKEREL_E2E_UI_EXTRA_COMPOSE_FILE

  local phase_seconds=$((SECONDS - phase_start))
  if [[ "$status" -eq 0 ]]; then
    echo "[web-e2e-ui] graph-disabled phase: PASS (${phase_seconds}s)" >&2
  else
    echo "[web-e2e-ui] graph-disabled phase: FAIL (exit=${status}, ${phase_seconds}s)" >&2
  fi
  return "$status"
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

# Seeded so the phases below can fold in with first-failure-wins. On the
# SMACKEREL_E2E_UI_NPX canary path no stack is brought up, every phase gate is
# false, and the lane exit stays the full suite's own code verbatim — the
# exit-code-propagation contract the canary asserts is unchanged.
lane_status=0
# S-1: set by run_true_empty_phase when the boot seed could not be restored.
# Phases 2 and 3 REUSE that stack, so their results would be meaningless.
TRUE_EMPTY_FIXTURE_UNTRUSTWORTHY=0

# Phase 1 — true-empty. MUST run here: the stack is freshly booted, so `people`
# and `places` are already empty and clearing the boot-seeded `topics` is all
# that stands between this run and an all-family empty graph. It restores the
# seed (and waits for core to be healthy again) before handing the same stack
# to the full suite. Live path only.
if [[ -z "${SMACKEREL_E2E_UI_NPX:-}" ]] && true_empty_phase_applies "$@"; then
  true_empty_status=0
  run_true_empty_phase || true_empty_status=$?
  # First failure wins, as with the phases below. The phase is NOT advisory:
  # a red true-empty phase fails the lane.
  if [[ "$lane_status" -eq 0 && "$true_empty_status" -ne 0 ]]; then
    lane_status="$true_empty_status"
  fi
else
  # S-5: say so. A silent skip is indistinguishable from a phase that ran, so
  # detecting a filtered run required noticing an ABSENCE in the transcript.
  echo "[web-e2e-ui] true-empty phase: SKIPPED (gate false — no true-empty state proof in this run)" >&2
fi

# Phase 2 — the full suite against the default, graph-ENABLED stack.
# Behavior is unchanged: the exit code still propagates verbatim, including
# on the SMACKEREL_E2E_UI_NPX canary path where no stack is brought up.
if [[ "$TRUE_EMPTY_FIXTURE_UNTRUSTWORTHY" -ne 0 ]]; then
  # S-1: the true-empty phase could not restore the boot seed, so this stack is
  # de-seeded. Running the suite here would print a green result that proves
  # nothing -- the specs are state-adaptive and would simply paint the
  # true-empty arm. Refusing to run is the honest outcome; the lane is already
  # failing on the restore error.
  echo "ERROR: [web-e2e-ui] full suite SKIPPED — the true-empty phase left the fixture unrestored, so any result here would be untrustworthy." >&2
  full_suite_status=0
else
  set +e
  run_node_tooling "$@"
  full_suite_status=$?
  set -e
fi
if [[ "$lane_status" -eq 0 && "$full_suite_status" -ne 0 ]]; then
  lane_status="$full_suite_status"
fi

# Phase 3 — store-unavailable. Runs on the stack phase 2 JUST finished with,
# still up: stopping one container is all the induction needs, so this phase
# costs roughly the spec's runtime instead of a whole rebuild/boot cycle. It
# must run BEFORE the graph-disabled phase, which recycles the stack. Live
# path only: the canary injects a stub npx and brings up no stack.
if [[ -z "${SMACKEREL_E2E_UI_NPX:-}" && "$TRUE_EMPTY_FIXTURE_UNTRUSTWORTHY" -eq 0 ]] && store_unavailable_phase_applies "$@"; then
  store_unavailable_status=0
  run_store_unavailable_phase || store_unavailable_status=$?
  # First failure wins, as with the phase below. The phase is NOT advisory:
  # a red store-unavailable phase fails the lane.
  if [[ "$lane_status" -eq 0 && "$store_unavailable_status" -ne 0 ]]; then
    lane_status="$store_unavailable_status"
  fi
else
  echo "[web-e2e-ui] store-unavailable phase: SKIPPED (gate false — no store-unavailable state proof in this run)" >&2
fi

# Phase 4 — F-080-04-LANE. Live-stack path only: the canary injects a stub
# npx and brings up no stack, so there is nothing to recycle there.
if [[ -z "${SMACKEREL_E2E_UI_NPX:-}" ]] && graph_disabled_phase_applies "$@"; then
  graph_disabled_status=0
  run_graph_disabled_phase || graph_disabled_status=$?
  # First failure wins, mirroring the `./smackerel.sh test e2e` lane. The
  # phase is NOT advisory: a red second phase fails the lane.
  if [[ "$lane_status" -eq 0 && "$graph_disabled_status" -ne 0 ]]; then
    lane_status="$graph_disabled_status"
  fi
else
  echo "[web-e2e-ui] graph-disabled phase: SKIPPED (gate false — no disabled state proof in this run)" >&2
fi

exit "$lane_status"
