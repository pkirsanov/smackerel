#!/usr/bin/env bash
# tests/unit/cli/spec_077_bootstrap_pwa_tooling_test.sh
#
# Spec 077 PWA browser-test harness — verification-enabler for spec 100
# SCOPE-05. Locks the OS-correct Playwright browser-cache detection in
# scripts/runtime/web-e2e-ui.sh.
#
# Regression origin: bootstrap_pwa_tooling() hardcoded the Linux cache
# path `$HOME/.cache/ms-playwright`. On macOS Playwright caches under
# `$HOME/Library/Caches/ms-playwright`, so the warm-cache compgen probe
# NEVER matched, need_browser_install was always 1, and `npx playwright
# install` re-ran on every invocation — which on the macOS Docker-Desktop
# host reliably deadlocks (hung oopDownloadBrowserMain) and, even when it
# completes, installed only `chromium` (not `chromium-headless-shell`,
# which the headless tests actually launch). See spec 100 report.md
# "SCOPE-05 verification-enabler".
#
# Every assertion is pure — no docker, no network, no real browser
# install (npx/npm are stubbed; caches are fake fixtures).
#
#   A. resolve_playwright_browser_cache Darwin  -> ~/Library/Caches/ms-playwright
#   B. resolve_playwright_browser_cache Linux   -> ~/.cache/ms-playwright
#   C. resolve_playwright_browser_cache <other> -> ~/.cache/ms-playwright (default)
#   D. explicit PLAYWRIGHT_BROWSERS_PATH override wins even on Darwin
#   E. no-arg auto-detect agrees with `uname -s` on the real host
#   F. bootstrap_pwa_tooling SKIPS install on a warm cache (all three
#      components present at their EXACT pinned revision) — npx is NEVER
#      invoked (the no-hang guarantee)
#   G. bootstrap_pwa_tooling INSTALLS the combined target when a component
#      (headless-shell / ffmpeg) dir is missing, forwarding EXACTLY
#      `playwright install chromium chromium-headless-shell ffmpeg`
#   G2. a STALE wrong-revision ffmpeg dir (ffmpeg-1011 vs pinned 1010) does
#      NOT satisfy the probe — the combined install still triggers
#      (F-100-OPT-01 revision-blind-glob regression lock)
#   H. static regression lock on the runner source (incl. revision-aware probe)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
WRAPPER="$REPO_ROOT/scripts/runtime/web-e2e-ui.sh"

[[ -f "$WRAPPER" ]] || {
  echo "FAIL: wrapper $WRAPPER missing" >&2
  exit 1
}

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# ------------------------------------------------------------------
# Source the wrapper. The sourced-guard returns 0 before the live-stack
# lifecycle wiring, exposing only the pure helpers exercised here.
# ------------------------------------------------------------------
# shellcheck source=/dev/null
source "$WRAPPER"

declare -F resolve_playwright_browser_cache >/dev/null \
  || fail "sourcing $WRAPPER did not expose resolve_playwright_browser_cache()"
declare -F bootstrap_pwa_tooling >/dev/null \
  || fail "sourcing $WRAPPER did not expose bootstrap_pwa_tooling()"

#############################################
# A/B/C. OS-correct default cache paths.
#############################################
# Clear any inherited override so the OS branch is exercised.
unset PLAYWRIGHT_BROWSERS_PATH

got="$(resolve_playwright_browser_cache Darwin)"
[[ "$got" == "$HOME/Library/Caches/ms-playwright" ]] \
  || fail "Darwin cache path = '$got' (want '$HOME/Library/Caches/ms-playwright')"

got="$(resolve_playwright_browser_cache Linux)"
[[ "$got" == "$HOME/.cache/ms-playwright" ]] \
  || fail "Linux cache path = '$got' (want '$HOME/.cache/ms-playwright')"

got="$(resolve_playwright_browser_cache FreeBSD)"
[[ "$got" == "$HOME/.cache/ms-playwright" ]] \
  || fail "non-Darwin OS should default to the Linux cache path (got '$got')"

#############################################
# D. Explicit PLAYWRIGHT_BROWSERS_PATH override wins (Playwright's own
#    precedence), even when the detected OS is Darwin.
#############################################
got="$(PLAYWRIGHT_BROWSERS_PATH=/custom/pw/cache resolve_playwright_browser_cache Darwin)"
[[ "$got" == "/custom/pw/cache" ]] \
  || fail "explicit PLAYWRIGHT_BROWSERS_PATH override ignored (got '$got')"

#############################################
# E. No-arg auto-detect agrees with `uname -s` on the real host.
#############################################
host_os="$(uname -s)"
case "$host_os" in
  Darwin) want="$HOME/Library/Caches/ms-playwright" ;;
  *) want="$HOME/.cache/ms-playwright" ;;
esac
got="$(resolve_playwright_browser_cache)"
[[ "$got" == "$want" ]] \
  || fail "auto-detect on '$host_os' = '$got' (want '$want')"

#############################################
# Shared fixture for F/G: a stub PWA dir with node_modules present (so
# npm ci is never needed) and stub npx/npm on PATH that record any
# invocation to $NPX_CALL_LOG.
#############################################
STUB_PWA="$TMP/pwa"
mkdir -p "$STUB_PWA/node_modules/playwright-core"
# Pinned-revision source the bootstrap probe reads
# (resolve_pinned_playwright_revision). The warm/cold/skew fixtures below use
# these exact revisions so the probe is exercised revision-EXACTLY: a stale
# different-revision dir must NOT satisfy the requirement (F-100-OPT-01).
cat >"$STUB_PWA/node_modules/playwright-core/browsers.json" <<'BROWSERS_JSON'
{
  "browsers": [
    { "name": "chromium", "revision": "1148" },
    { "name": "chromium-headless-shell", "revision": "1148" },
    { "name": "ffmpeg", "revision": "1010" }
  ]
}
BROWSERS_JSON
# Consumed by the sourced bootstrap_pwa_tooling(); shellcheck cannot see
# through `source` so it reports it unused.
# shellcheck disable=SC2034
SMACKEREL_E2E_UI_PWA_DIR="$STUB_PWA" # override the sourced global

STUB_BIN="$TMP/bin"
mkdir -p "$STUB_BIN"
NPX_CALL_LOG="$TMP/npx-called.log"
cat >"$STUB_BIN/npx" <<STUB
#!/usr/bin/env bash
echo "\$*" >>"$NPX_CALL_LOG"
exit 0
STUB
cat >"$STUB_BIN/npm" <<STUB
#!/usr/bin/env bash
echo "npm \$*" >>"$NPX_CALL_LOG"
exit 0
STUB
chmod +x "$STUB_BIN/npx" "$STUB_BIN/npm"

#############################################
# F. Warm cache (all three component dirs present: chromium,
#    chromium-headless-shell, ffmpeg) -> install SKIPPED, npx never
#    invoked (the no-hang, fast-no-op guarantee the fix restores).
#############################################
WARM="$TMP/warm-cache"
mkdir -p "$WARM/chromium-1148" "$WARM/chromium_headless_shell-1148" "$WARM/ffmpeg-1010"
: >"$NPX_CALL_LOG"
rc=0
# Subshell env isolation is deliberate: PATH/PLAYWRIGHT_BROWSERS_PATH must
# NOT leak into the next assertion. SC2030/SC2031 flag exactly that
# (intended) locality.
# shellcheck disable=SC2030,SC2031
(
  export PATH="$STUB_BIN:$PATH"
  export PLAYWRIGHT_BROWSERS_PATH="$WARM"
  unset SMACKEREL_E2E_UI_NPX
  bootstrap_pwa_tooling
) || rc=$?
[[ "$rc" -eq 0 ]] || fail "bootstrap_pwa_tooling on a warm cache exited $rc (want 0)"
[[ ! -s "$NPX_CALL_LOG" ]] \
  || fail "warm cache should SKIP install but npx was invoked:
$(cat "$NPX_CALL_LOG")"

#############################################
# G. Missing ffmpeg (a warm chromium+headless-shell cache, but no ffmpeg
#    dir) -> install TRIGGERED, forwarding EXACTLY the combined target so
#    the three components cannot GC-evict each other. Mirrors the real
#    F-100-OPT-01 host state where chromium/headless-shell were warm but
#    ffmpeg was absent, breaking every browser newPage.
#############################################
COLD="$TMP/cold-cache"
mkdir -p "$COLD/chromium-1148" "$COLD/chromium_headless_shell-1148" # ffmpeg dir intentionally absent
: >"$NPX_CALL_LOG"
rc=0
# shellcheck disable=SC2030,SC2031
(
  export PATH="$STUB_BIN:$PATH"
  export PLAYWRIGHT_BROWSERS_PATH="$COLD"
  unset SMACKEREL_E2E_UI_NPX
  bootstrap_pwa_tooling
) || rc=$?
[[ "$rc" -eq 0 ]] || fail "bootstrap_pwa_tooling (cold ffmpeg) exited $rc (want 0)"
grep -qx 'playwright install chromium chromium-headless-shell ffmpeg' "$NPX_CALL_LOG" \
  || fail "combined browser+ffmpeg install not forwarded; npx log was:
$(cat "$NPX_CALL_LOG")"
if grep -q '^npm ' "$NPX_CALL_LOG"; then
  fail "npm ci ran even though node_modules was present:
$(cat "$NPX_CALL_LOG")"
fi

#############################################
# G2. Stale wrong-revision ffmpeg (F-100-OPT-01 regression lock): a warm
#     chromium+headless-shell cache PLUS a leftover ffmpeg-1011 dir that does
#     NOT match the pinned ffmpeg revision (1010, per the stub browsers.json).
#     A revision-blind `ffmpeg-*` glob was fooled by this and SKIPPED the
#     install, leaving the lane red with "Executable doesn't exist at
#     .../ffmpeg-1010/ffmpeg-<os>". The revision-EXACT probe MUST still
#     trigger the combined install.
#############################################
SKEW="$TMP/skew-cache"
# ffmpeg-1010 (pinned) absent; stale ffmpeg-1011 present.
mkdir -p "$SKEW/chromium-1148" "$SKEW/chromium_headless_shell-1148" "$SKEW/ffmpeg-1011"
: >"$NPX_CALL_LOG"
rc=0
# shellcheck disable=SC2030,SC2031
(
  export PATH="$STUB_BIN:$PATH"
  export PLAYWRIGHT_BROWSERS_PATH="$SKEW"
  unset SMACKEREL_E2E_UI_NPX
  bootstrap_pwa_tooling
) || rc=$?
[[ "$rc" -eq 0 ]] || fail "bootstrap_pwa_tooling (stale ffmpeg-1011) exited $rc (want 0)"
grep -qx 'playwright install chromium chromium-headless-shell ffmpeg' "$NPX_CALL_LOG" \
  || fail "stale wrong-revision ffmpeg must STILL trigger the combined install (revision-blind glob regression); npx log was:
$(cat "$NPX_CALL_LOG")"

#############################################
# H. Static regression lock on the runner source.
#############################################
# Fixed-string search for the retired Linux-only default literal; the
# `${...}` is intentionally NOT expanded (grep -qF).
# shellcheck disable=SC2016
if grep -qF '${PLAYWRIGHT_BROWSERS_PATH:-$HOME/.cache/ms-playwright}' "$WRAPPER"; then
  fail "web-e2e-ui.sh still hardcodes the Linux-only cache default (macOS regression)"
fi
grep -qF 'playwright install chromium chromium-headless-shell ffmpeg' "$WRAPPER" \
  || fail "web-e2e-ui.sh no longer installs the combined chromium + chromium-headless-shell + ffmpeg target"
grep -qF 'resolve_pinned_playwright_revision' "$WRAPPER" \
  || fail "web-e2e-ui.sh no longer resolves the pinned Playwright revision (revision-blind probe regression)"
grep -qF 'playwright-core/browsers.json' "$WRAPPER" \
  || fail "web-e2e-ui.sh no longer reads playwright-core/browsers.json for the pinned revisions"
grep -qF 'Library/Caches/ms-playwright' "$WRAPPER" \
  || fail "web-e2e-ui.sh missing the macOS Library/Caches cache-path branch"

#############################################
# I. Spec 100 F-100-OPT-02/03 — no-ML e2e-ui lane + lowered `ui` preflight
#    floor. Static locks (no docker, no stack): the override profile-gates the
#    2 GB ml sidecar OFF, and the dispatcher gates the lane on the `ui` floor.
#############################################
OVERRIDE="$REPO_ROOT/docker-compose.e2e-ui.override.yml"
DISPATCH="$REPO_ROOT/smackerel.sh"
[[ -f "$OVERRIDE" ]] || fail "e2e-ui override $OVERRIDE missing"
[[ -f "$DISPATCH" ]] || fail "dispatcher $DISPATCH missing"

# Capture the smackerel-ml override block (decl to the next top-level 2-space
# service key, or EOF). smackerel-ml is currently the last service; the reset
# guard keeps this correct if another service is appended later.
ML_BLOCK="$(awk '
  $1 == "smackerel-ml:" { in_ml = 1; next }
  in_ml && /^  [^[:space:]#]/ { in_ml = 0 }
  in_ml { print }
' "$OVERRIDE")"

[[ -n "$ML_BLOCK" ]] \
  || fail "docker-compose.e2e-ui.override.yml has no smackerel-ml override block (F-100-OPT-03 no-ML lane missing)"

# F-100-OPT-03 — ml is DROPPED by profile-gating it behind the inert `ml`
# profile (COMPOSE_PROFILES for the test env is only ever ollama,searxng
# [,monitoring] — never `ml`), so the 2 GB sidecar never boots in this lane.
printf '%s\n' "$ML_BLOCK" | grep -qE '^[[:space:]]*-[[:space:]]+ml[[:space:]]*$' \
  || fail "override no longer profile-gates smackerel-ml behind the inert 'ml' profile (F-100-OPT-03 regression):
$ML_BLOCK"

# The drop MUST NOT be silently converted back into a running/stub service: no
# re-declared image or memory limit under smackerel-ml (prose mentions in the
# comments are excluded — this matches only leading-whitespace YAML keys).
if printf '%s\n' "$ML_BLOCK" | grep -qE '^[[:space:]]+(memory|image):'; then
  fail "override re-declares an image/memory limit under smackerel-ml — the lane must DROP ml, not resize or stub it:
$ML_BLOCK"
fi

# F-100-OPT-02 — the e2e-ui dispatch gates on the LOWER `ui` preflight profile,
# not the 6000 MB heavy wrapper (smackerel_assert_host_resources test).
grep -qF 'smackerel_assert_host_resources_profile test ui' "$DISPATCH" \
  || fail "smackerel.sh e2e-ui lane no longer selects the 'ui' preflight profile (F-100-OPT-02 lowered-floor regression)"

echo "PASS: spec_077_e2e_ui_no_ml_and_ui_preflight_floor (F-100-OPT-02/03 lock)"

echo "PASS: spec_077_bootstrap_pwa_tooling_test (macOS browser-cache OS-path lock)"
