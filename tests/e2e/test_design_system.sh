#!/usr/bin/env bash
# E2E test: Design system CSS themes and responsiveness
# Scenarios: SCN-001-005, SCN-001-006, SCN-001-007, SCN-001-008, SCN-001-009
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/helpers.sh"

trap e2e_cleanup EXIT

echo "=== Design System E2E Tests ==="
e2e_start

BODY=$(curl -sf --max-time 15 "$CORE_URL/" 2>/dev/null || true)

# --- SCN-001-005 / SCN-001-006: Light and dark themes ---
echo "Test: CSS custom properties for theming..."
if echo "$BODY" | grep -q -- "--bg\|--fg\|--border"; then
  e2e_pass "SCN-001-005/006: CSS custom properties present for theming"
else
  e2e_fail "SCN-001-005/006: CSS custom properties (--bg, --fg, --border) not found in response"
fi

# --- SCN-001-008: System fonts ---
echo "Test: System font stack..."
if echo "$BODY" | grep -q "system-ui\|-apple-system\|BlinkMacSystemFont"; then
  e2e_pass "SCN-001-008: System font stack used"
else
  e2e_fail "SCN-001-008: System font stack (system-ui, -apple-system, BlinkMacSystemFont) not found in CSS"
fi

# --- SCN-001-009: No accent colors ---
echo "Test: No accent colors..."
ACCENT_FOUND=0
for COLOR in "#0000ff" "#0066cc" "color: blue" "color:blue" "badge-primary" "btn-primary"; do
  if echo "$BODY" | grep -qi "$COLOR"; then
    echo "  WARNING: Found accent color pattern: $COLOR"
    ACCENT_FOUND=1
  fi
done
if [ "$ACCENT_FOUND" -eq 0 ]; then
  e2e_pass "SCN-001-009: No accent colors in web UI"
else
  e2e_fail "SCN-001-009: Non-monochrome accent colors found in web UI (violates monochrome palette)"
fi

# --- Responsive meta tag ---
echo "Test: Responsive viewport..."
if echo "$BODY" | grep -q "viewport"; then
  e2e_pass "SCN-001-007: Viewport meta tag present for responsive layout"
else
  e2e_fail "SCN-001-007: Missing viewport meta tag"
fi

echo ""
echo "=== Design System E2E tests passed ==="
