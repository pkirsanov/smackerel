#!/usr/bin/env bash
set -euo pipefail

# Validates web assets: PWA and browser extension manifests, JS syntax.
# Called by ./smackerel.sh lint and CI.

cd /workspace

errors=0

# --- JSON manifest validation ---
echo "=== Validating web manifests ==="

validate_json() {
  local file="$1"
  if ! python3 -c "import json, sys; json.load(open(sys.argv[1]))" "$file" 2>/dev/null; then
    echo "FAIL: $file is not valid JSON"
    errors=$((errors + 1))
    return 1
  fi
  echo "  OK: $file"
  return 0
}

# PWA manifest
if [ -f "web/pwa/manifest.json" ]; then
  validate_json "web/pwa/manifest.json"
  # Check required PWA fields
  python3 -c "
import json, sys
m = json.load(open('web/pwa/manifest.json'))
required = ['name', 'short_name', 'start_url', 'display', 'icons', 'share_target']
missing = [f for f in required if f not in m]
if missing:
    print(f'FAIL: web/pwa/manifest.json missing required fields: {missing}', file=sys.stderr)
    sys.exit(1)
st = m.get('share_target', {})
st_required = ['action', 'method', 'params']
st_missing = [f for f in st_required if f not in st]
if st_missing:
    print(f'FAIL: web/pwa/manifest.json share_target missing: {st_missing}', file=sys.stderr)
    sys.exit(1)
print('  OK: PWA manifest has required fields')
" || errors=$((errors + 1))
else
  echo "SKIP: web/pwa/manifest.json not found"
fi

# Chrome extension manifest
if [ -f "web/extension/manifest.json" ]; then
  validate_json "web/extension/manifest.json"
  python3 -c "
import json, sys
m = json.load(open('web/extension/manifest.json'))
required = ['manifest_version', 'name', 'version', 'permissions', 'background']
missing = [f for f in required if f not in m]
if missing:
    print(f'FAIL: web/extension/manifest.json missing required fields: {missing}', file=sys.stderr)
    sys.exit(1)
if m.get('manifest_version') != 3:
    print(f'FAIL: web/extension/manifest.json manifest_version should be 3', file=sys.stderr)
    sys.exit(1)
print('  OK: Chrome extension manifest has required fields (MV3)')
" || errors=$((errors + 1))
else
  echo "SKIP: web/extension/manifest.json not found"
fi

# Firefox extension manifest
if [ -f "web/extension/manifest.firefox.json" ]; then
  validate_json "web/extension/manifest.firefox.json"
  python3 -c "
import json, sys
m = json.load(open('web/extension/manifest.firefox.json'))
required = ['manifest_version', 'name', 'version', 'permissions', 'browser_specific_settings']
missing = [f for f in required if f not in m]
if missing:
    print(f'FAIL: web/extension/manifest.firefox.json missing required fields: {missing}', file=sys.stderr)
    sys.exit(1)
bss = m.get('browser_specific_settings', {}).get('gecko', {})
if 'id' not in bss:
    print('FAIL: web/extension/manifest.firefox.json missing gecko.id', file=sys.stderr)
    sys.exit(1)
print('  OK: Firefox extension manifest has required fields (MV2 + gecko)')
" || errors=$((errors + 1))
else
  echo "SKIP: web/extension/manifest.firefox.json not found"
fi

# --- JS syntax validation (lightweight, no node required) ---
echo ""
echo "=== Validating JS syntax ==="

# Use python3 to check for common JS syntax issues (balanced braces/parens)
# This is a lightweight structural check, not a full parser.
for jsfile in \
  web/pwa/app.js \
  web/pwa/sw.js \
  web/pwa/lib/queue.js \
  web/extension/background.js \
  web/extension/popup/popup.js \
  web/extension/lib/queue.js \
  web/extension/lib/browser-polyfill.js; do
  if [ -f "$jsfile" ]; then
    python3 -c "
import sys
content = open(sys.argv[1]).read()
# Check balanced braces
opens = content.count('{')
closes = content.count('}')
if opens != closes:
    print(f'FAIL: {sys.argv[1]} has unbalanced braces ({{ {opens} vs }} {closes})', file=sys.stderr)
    sys.exit(1)
# Check balanced parentheses
opens_p = content.count('(')
closes_p = content.count(')')
if opens_p != closes_p:
    print(f'FAIL: {sys.argv[1]} has unbalanced parentheses (( {opens_p} vs ) {closes_p})', file=sys.stderr)
    sys.exit(1)
print(f'  OK: {sys.argv[1]}')
" "$jsfile" || errors=$((errors + 1))
  fi
done

# --- Extension version consistency ---
echo ""
echo "=== Checking extension version consistency ==="

if [ -f "web/extension/manifest.json" ] && [ -f "web/extension/manifest.firefox.json" ]; then
  python3 -c "
import json, sys
chrome = json.load(open('web/extension/manifest.json'))
firefox = json.load(open('web/extension/manifest.firefox.json'))
cv = chrome.get('version', '')
fv = firefox.get('version', '')
if cv != fv:
    print(f'FAIL: Extension version mismatch — Chrome: {cv}, Firefox: {fv}', file=sys.stderr)
    sys.exit(1)
print(f'  OK: Extension versions match ({cv})')
" || errors=$((errors + 1))
fi

echo ""
if [ "$errors" -gt 0 ]; then
  echo "Web validation FAILED with $errors error(s)"
  exit 1
fi
echo "Web validation passed"
