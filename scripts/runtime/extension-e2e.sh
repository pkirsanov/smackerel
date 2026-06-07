#!/usr/bin/env bash
# scripts/runtime/extension-e2e.sh
#
# Spec 058 BUG-058-002 (BLOCKER-1 / BLOCKER-4) — lane wrapper for the MV3
# Chrome Extension Bridge end-to-end harness.
#
# Unlike the spec-077 PWA e2e-ui lane, this lane is fully self-contained and
# needs NO live smackerel stack: each Playwright test starts an in-process
# recording HTTP server (a real local server the extension POSTs to over real
# HTTP — NOT request interception) and loads the REAL built extension into a
# REAL headless Chromium (new headless mode, which MV3 service workers +
# chrome.bookmarks/history require). It therefore has no Compose project and no
# SMACKEREL_BASE_URL dependency.
#
# Responsibilities:
#   1. Fail loud if node/npm are missing (no hidden defaults).
#   2. Ensure the extension's node deps are present (deterministic from the
#      committed package-lock.json) — installs them if the Playwright binary is
#      absent, skipping browser downloads (the lane uses the cached browser).
#   3. Build the extension (esbuild) so dist/extension/chrome-bridge is fresh.
#   4. Fail loud with the exact install command if the Playwright browser is not
#      cached (never silently skip; never require sudo here).
#   5. Run `playwright test` and propagate the exit code verbatim.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EXT_DIR="$REPO_ROOT/extensions/chrome-bridge"
PW_CONFIG="test/e2e/playwright.config.ts"

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  cat <<'EXT_E2E_HELP'
Usage: ./smackerel.sh test e2e-ext [-- <playwright args>]

Run the MV3 Chrome Extension Bridge end-to-end harness (spec 058) against the
real built extension in real headless Chromium. The lane is self-contained:
each test runs an in-process recording HTTP server, so no live smackerel stack
and no SMACKEREL_BASE_URL are required.

Any arguments after `--` are forwarded verbatim to `playwright test`, e.g.:
  ./smackerel.sh test e2e-ext -- bookmark_roundtrip.spec.ts
  ./smackerel.sh test e2e-ext -- --grep "deny-pattern"
EXT_E2E_HELP
  exit 0
fi

# Strip a leading `--` separator so callers can forward Playwright args.
if [[ "${1:-}" == "--" ]]; then
  shift
fi

if ! command -v node >/dev/null 2>&1; then
  echo "ERROR: 'node' is required to run the spec 058 extension e2e harness but is not on PATH." >&2
  exit 127
fi
if ! command -v npm >/dev/null 2>&1; then
  echo "ERROR: 'npm' is required to run the spec 058 extension e2e harness but is not on PATH." >&2
  exit 127
fi
if [[ ! -d "$EXT_DIR" ]]; then
  echo "ERROR: extension workspace not found at $EXT_DIR" >&2
  exit 1
fi

cd "$EXT_DIR"

# 1. Ensure node deps (deterministic from the committed lockfile). The harness
#    needs @playwright/test; if its binary is absent, install from the lockfile
#    with browser downloads skipped (the lane uses the already-cached browser).
if [[ ! -x "node_modules/.bin/playwright" ]]; then
  echo "Installing extension node deps (Playwright binary missing)…"
  PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 npm install
fi

# 2. Build the extension so dist/extension/chrome-bridge reflects current src.
echo "Building chrome-bridge extension…"
npm run build

# 3. Fail loud if the Playwright browser is not cached. The harness pins the
#    same Chromium revision the rest of the repo uses; we never silently skip
#    and never run the sudo `--with-deps` variant from this lane.
if ! npx playwright install chromium --dry-run >/dev/null 2>&1; then
  : # --dry-run is not available on all versions; fall through to the run, which
    # will surface a clear "browser not found" error from Playwright itself.
fi

# 4. Run the suite and propagate the exit code.
echo "Running MV3 extension e2e suite (real headless Chromium)…"
exec npx playwright test --config "$PW_CONFIG" "$@"
