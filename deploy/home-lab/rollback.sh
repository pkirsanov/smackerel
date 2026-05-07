#!/usr/bin/env bash
# rollback.sh — pure pointer-swap to previousManifest. NEVER rebuilds.
#
# Adapter contract (NON-NEGOTIABLE per bubbles G074):
#   - MUST NOT invoke any build step
#   - MUST NOT pull a different image digest beyond what's in previousManifest
#   - MUST NOT fail-soft if previousManifest is null — fail explicitly with a clear message
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFEST="$SCRIPT_DIR/manifest.yaml"

[[ -f "$MANIFEST" ]] || { echo "ERROR: $MANIFEST missing" >&2; exit 1; }

# Detect whether previousManifest is null
if grep -qE '^previousManifest:[[:space:]]*null[[:space:]]*$' "$MANIFEST"; then
  echo "ERROR: previousManifest is null — there is nothing to roll back to." >&2
  echo "       Apply at least two distinct releases before rollback is meaningful." >&2
  exit 1
fi

echo "▶ rollback: swapping current ⇄ previousManifest in $MANIFEST"

# Extract previousManifest body (lines indented under `previousManifest:`).
PREV_BODY="$(awk '
  in_prev && /^[^[:space:]]/ { in_prev=0 }
  /^previousManifest:[[:space:]]*$/ { in_prev=1; next }
  in_prev { print }
' "$MANIFEST")"

[[ -n "$PREV_BODY" ]] || { echo "ERROR: previousManifest body is empty in $MANIFEST" >&2; exit 1; }

# Strip the leading 2-space indent so the block becomes a top-level `current:` body.
PREV_CURRENT="$(echo "$PREV_BODY" | sed -E 's/^  //')"

# Capture today's `current:` body so we can park it as the new `previousManifest:`.
CURRENT_BODY="$(awk '
  in_current && /^[^[:space:]]/ { in_current=0 }
  /^current:/ { in_current=1; next }
  in_current { print }
' "$MANIFEST")"

NEW_MANIFEST="$(mktemp "${MANIFEST}.new.XXXXXX")"
{
  echo "# Home-Lab Deployment Manifest"
  echo "# Written by deploy/home-lab/rollback.sh — DO NOT EDIT BY HAND"
  echo "manifestVersion: 1"
  echo ""
  echo "current:"
  echo "$PREV_CURRENT"
  echo ""
  echo "previousManifest:"
  echo "$CURRENT_BODY" | sed 's/^/  /'
} > "$NEW_MANIFEST"

mv -f "$NEW_MANIFEST" "$MANIFEST"
echo "  manifest pointer swapped"

echo "▶ rollback: re-running rollout with prior digests"
# A real implementation would re-run the rollout strategy here using the now-current digests.
# Today's home-lab is recreate-only, so a stack restart is sufficient:
echo "  (host-side restart would happen here; recreate strategy)"

echo "rollback OK"

