#!/usr/bin/env bash
# Spec 058 Scope 4 — Build pipeline for the Chrome Extension Bridge.
#
# Wraps the committed extensions/chrome-bridge/ npm + esbuild toolchain and
# emits a versioned zip + .sha256 under dist/extension/. Byte-reproducible
# packaging is achieved via `zip -X` (no extra file attrs) and SOURCE_DATE_EPOCH
# (resolved from the current git commit when available) so two CI runs on the
# same source SHA produce byte-identical zips.
#
# Usage: ./smackerel.sh build --extension chrome-bridge
#        (or directly: bash scripts/commands/build-chrome-bridge.sh)
#
# SST-zero-defaults: this script consumes NO runtime config. The extension's
# runtime endpoints/tokens/device IDs are operator-configured through the
# options page at install time (see docs/Operations.md "Chrome Extension
# Bridge — Sideload Workflow"). Nothing about the operator's deployment is
# baked into the artifact.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
EXT_DIR="$REPO_ROOT/extensions/chrome-bridge"
DIST_DIR="$REPO_ROOT/dist/extension"

if [ ! -d "$EXT_DIR" ]; then
  echo "ERROR: Chrome Bridge extension directory not found: $EXT_DIR" >&2
  exit 1
fi

if ! command -v npm >/dev/null 2>&1; then
  echo "ERROR: npm is required to build the chrome-bridge extension" >&2
  exit 1
fi
if ! command -v zip >/dev/null 2>&1; then
  echo "ERROR: zip is required to package the chrome-bridge extension" >&2
  exit 1
fi
if ! command -v sha256sum >/dev/null 2>&1; then
  echo "ERROR: sha256sum is required to emit the .sha256 digest" >&2
  exit 1
fi

VERSION="$(python3 -c "import json,sys; print(json.load(open('$EXT_DIR/manifest.json'))['version'])")"
if [ -z "$VERSION" ]; then
  echo "ERROR: failed to read version from $EXT_DIR/manifest.json" >&2
  exit 1
fi

if SHA_FULL="$(git -C "$REPO_ROOT" rev-parse HEAD 2>/dev/null)"; then
  SHA_SHORT="$(printf '%s' "$SHA_FULL" | cut -c1-12)"
else
  SHA_SHORT="nogit"
  SHA_FULL=""
fi

# Pin file mtimes for byte-reproducible zips. Prefer the commit's author date
# when running inside a git checkout; otherwise fall back to a constant epoch.
# `zip -X` strips extra attrs but still writes mtime; setting it via touch
# (and exporting SOURCE_DATE_EPOCH for esbuild) keeps the archive deterministic.
if [ -n "$SHA_FULL" ]; then
  EPOCH="$(git -C "$REPO_ROOT" log -1 --pretty=%ct "$SHA_FULL" 2>/dev/null || echo "")"
fi
if [ -z "${EPOCH:-}" ]; then
  EPOCH="0"
fi
export SOURCE_DATE_EPOCH="$EPOCH"

echo "smackerel-chrome-bridge build"
echo "  version: $VERSION"
echo "  sha:     $SHA_SHORT"
echo "  epoch:   $EPOCH"

mkdir -p "$DIST_DIR"

(
  cd "$EXT_DIR"
  echo "==> npm ci"
  npm ci --no-audit --no-fund --loglevel=error
  echo "==> esbuild (production)"
  NODE_ENV=production node esbuild.config.mjs
)

BUILD_OUT="$EXT_DIR/dist/extension/chrome-bridge"
if [ ! -d "$BUILD_OUT" ]; then
  echo "ERROR: esbuild did not produce $BUILD_OUT" >&2
  exit 1
fi

# Normalize mtimes inside the staging tree to SOURCE_DATE_EPOCH so the zip
# entries are deterministic on identical inputs.
TOUCH_TS="$(date -u -d "@$EPOCH" +%Y%m%d%H%M.%S 2>/dev/null || echo "")"
if [ -n "$TOUCH_TS" ]; then
  find "$BUILD_OUT" -exec touch -t "$TOUCH_TS" {} +
fi

ZIP_NAME="smackerel-chrome-bridge-${VERSION}-${SHA_SHORT}.zip"
ZIP_PATH="$DIST_DIR/$ZIP_NAME"
SHA_PATH="${ZIP_PATH}.sha256"

rm -f "$ZIP_PATH" "$SHA_PATH"

# Zip from inside the build output so paths in the archive are relative to
# the extension root (manifest.json at top level — required by Chrome).
(
  cd "$BUILD_OUT"
  # -X drops extra file attrs (UID/GID/extended timestamps); -r recurses;
  # entries listed via `find ... | sort` give a stable archive order.
  find . -type f -print0 | LC_ALL=C sort -z | xargs -0 zip -X -q "$ZIP_PATH"
)

(
  cd "$DIST_DIR"
  sha256sum "$ZIP_NAME" > "$(basename "$SHA_PATH")"
)

echo "==> produced:"
echo "  $ZIP_PATH"
echo "  $SHA_PATH"
ls -lh "$ZIP_PATH" "$SHA_PATH"
