#!/usr/bin/env bash
set -euo pipefail

# Package browser extension for distribution.
# Creates .zip archives for Chrome and Firefox in dist/extension/.
# Usage: ./smackerel.sh package extension

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
EXT_DIR="$REPO_ROOT/web/extension"
DIST_DIR="$REPO_ROOT/dist/extension"

if [ ! -d "$EXT_DIR" ]; then
  echo "ERROR: Extension directory not found: $EXT_DIR" >&2
  exit 1
fi

# Read version from Chrome manifest
VERSION=$(python3 -c "import json; print(json.load(open('$EXT_DIR/manifest.json'))['version'])")
echo "Extension version: $VERSION"

mkdir -p "$DIST_DIR"

# --- Chrome extension ---
echo "Packaging Chrome extension..."
CHROME_ZIP="$DIST_DIR/smackerel-chrome-${VERSION}.zip"
rm -f "$CHROME_ZIP"
(
  cd "$EXT_DIR"
  zip -r "$CHROME_ZIP" \
    manifest.json \
    background.js \
    popup/ \
    icons/ \
    lib/queue.js \
    -x "manifest.firefox.json" \
    -x "lib/browser-polyfill.js"
)
echo "  Created: $CHROME_ZIP"

# --- Firefox extension ---
echo "Packaging Firefox extension..."
FIREFOX_ZIP="$DIST_DIR/smackerel-firefox-${VERSION}.zip"
rm -f "$FIREFOX_ZIP"

# Firefox needs manifest.firefox.json renamed to manifest.json
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

cp -r "$EXT_DIR/popup" "$TMPDIR/popup"
cp -r "$EXT_DIR/icons" "$TMPDIR/icons"
mkdir -p "$TMPDIR/lib"
cp "$EXT_DIR/lib/queue.js" "$TMPDIR/lib/queue.js"
cp "$EXT_DIR/lib/browser-polyfill.js" "$TMPDIR/lib/browser-polyfill.js"
cp "$EXT_DIR/background.js" "$TMPDIR/background.js"
cp "$EXT_DIR/manifest.firefox.json" "$TMPDIR/manifest.json"

(
  cd "$TMPDIR"
  zip -r "$FIREFOX_ZIP" .
)
echo "  Created: $FIREFOX_ZIP"

echo ""
echo "Extension packages ready in dist/extension/"
ls -lh "$DIST_DIR"/smackerel-*-"${VERSION}".zip
