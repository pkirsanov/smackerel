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
#   F. bootstrap_pwa_tooling SKIPS install on a warm cache (both browser
#      dirs present) — npx is NEVER invoked (the no-hang guarantee)
#   G. bootstrap_pwa_tooling INSTALLS both browsers when the headless-shell
#      dir is missing, forwarding EXACTLY
#      `playwright install chromium chromium-headless-shell`
#   H. static regression lock on the runner source

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
mkdir -p "$STUB_PWA/node_modules"
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
# F. Warm cache (BOTH browser dirs present) -> install SKIPPED, npx never
#    invoked (the no-hang, fast-no-op guarantee the fix restores).
#############################################
WARM="$TMP/warm-cache"
mkdir -p "$WARM/chromium-1148" "$WARM/chromium_headless_shell-1148"
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
# G. Missing chromium-headless-shell -> install TRIGGERED, forwarding
#    EXACTLY the combined target so the two builds cannot GC-evict each
#    other. (Old code checked only chromium-* and installed only chromium.)
#############################################
COLD="$TMP/cold-cache"
mkdir -p "$COLD/chromium-1148" # headless-shell dir intentionally absent
: >"$NPX_CALL_LOG"
rc=0
# shellcheck disable=SC2030,SC2031
(
  export PATH="$STUB_BIN:$PATH"
  export PLAYWRIGHT_BROWSERS_PATH="$COLD"
  unset SMACKEREL_E2E_UI_NPX
  bootstrap_pwa_tooling
) || rc=$?
[[ "$rc" -eq 0 ]] || fail "bootstrap_pwa_tooling (cold headless-shell) exited $rc (want 0)"
grep -qx 'playwright install chromium chromium-headless-shell' "$NPX_CALL_LOG" \
  || fail "combined browser install not forwarded; npx log was:
$(cat "$NPX_CALL_LOG")"
if grep -q '^npm ' "$NPX_CALL_LOG"; then
  fail "npm ci ran even though node_modules was present:
$(cat "$NPX_CALL_LOG")"
fi

#############################################
# H. Static regression lock on the runner source.
#############################################
# Fixed-string search for the retired Linux-only default literal; the
# `${...}` is intentionally NOT expanded (grep -qF).
# shellcheck disable=SC2016
if grep -qF '${PLAYWRIGHT_BROWSERS_PATH:-$HOME/.cache/ms-playwright}' "$WRAPPER"; then
  fail "web-e2e-ui.sh still hardcodes the Linux-only cache default (macOS regression)"
fi
grep -qF 'playwright install chromium chromium-headless-shell' "$WRAPPER" \
  || fail "web-e2e-ui.sh no longer installs the combined chromium + chromium-headless-shell target"
grep -qF 'Library/Caches/ms-playwright' "$WRAPPER" \
  || fail "web-e2e-ui.sh missing the macOS Library/Caches cache-path branch"

echo "PASS: spec_077_bootstrap_pwa_tooling_test (macOS browser-cache OS-path lock)"
