#!/usr/bin/env bash
# Bubbles cheatsheet generator selftest (v6.0 / B7).
#
# Runs after every framework-validate to keep the cheatsheet honest:
#   1. The current docs/CHEATSHEET.md and docs/its-not-rocket-appliances.html
#      MUST match what the generator would emit from the registry. Stale
#      cheatsheets fail.
#   2. Registry validation rejects unknown workflow modes, dangling
#      maps_to values, duplicate aliases, and duplicate vocabulary terms.
#   3. A fixture with a deliberately broken registry exits non-zero.
#
# Replaces the v5.0.1 cheatsheet-drift-selftest.sh because the generator
# makes MD/HTML drift structurally impossible.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
GENERATOR="$SCRIPT_DIR/generate-cheatsheet.sh"
REGISTRY_DIR="$REPO_ROOT/bubbles/cheatsheet"

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; failures=$((failures + 1)); }

if [[ ! -x "$GENERATOR" ]]; then
  echo "generate-cheatsheet-selftest: missing or non-executable generator at $GENERATOR" >&2
  exit 2
fi

# 1. Registry JSON parses.
if jq -e . "$REGISTRY_DIR/modes.json" >/dev/null 2>&1; then
  pass "modes.json parses"
else
  fail "modes.json does not parse"
fi

if jq -e . "$REGISTRY_DIR/aliases.json" >/dev/null 2>&1; then
  pass "aliases.json parses"
else
  fail "aliases.json does not parse"
fi

if jq -e . "$REGISTRY_DIR/vocabulary.json" >/dev/null 2>&1; then
  pass "vocabulary.json parses"
else
  fail "vocabulary.json does not parse"
fi

# 2. Generator runs in --check mode against the current tree.
check_log="$(mktemp)"
if bash "$GENERATOR" --check >"$check_log" 2>&1; then
  pass "generator --check matches current cheatsheets"
else
  fail "generator --check reports drift; rerun: bash bubbles/scripts/generate-cheatsheet.sh"
  sed -n '1,40p' "$check_log" | sed 's/^/    /' >&2
fi
rm -f "$check_log"

# 3. Both cheatsheets carry the GENERATED markers (drift-by-deletion guard).
required_markers=(
  "docs/CHEATSHEET.md:GENERATED:CHEATSHEET_ALIASES_START"
  "docs/CHEATSHEET.md:GENERATED:CHEATSHEET_ALIASES_END"
  "docs/CHEATSHEET.md:GENERATED:CHEATSHEET_MODES_START"
  "docs/CHEATSHEET.md:GENERATED:CHEATSHEET_MODES_END"
  "docs/CHEATSHEET.md:GENERATED:CHEATSHEET_VOCABULARY_START"
  "docs/CHEATSHEET.md:GENERATED:CHEATSHEET_VOCABULARY_END"
  "docs/its-not-rocket-appliances.html:GENERATED:HTML_ALIASES_TABLE_START"
  "docs/its-not-rocket-appliances.html:GENERATED:HTML_ALIASES_TABLE_END"
  "docs/its-not-rocket-appliances.html:GENERATED:HTML_MODES_CARDS_START"
  "docs/its-not-rocket-appliances.html:GENERATED:HTML_MODES_CARDS_END"
  "docs/its-not-rocket-appliances.html:GENERATED:HTML_VOCABULARY_CARDS_START"
  "docs/its-not-rocket-appliances.html:GENERATED:HTML_VOCABULARY_CARDS_END"
)
for entry in "${required_markers[@]}"; do
  file="${entry%%:*}"
  marker="${entry#*:}"
  if grep -Fq "$marker" "$REPO_ROOT/$file" 2>/dev/null; then
    pass "$file has $marker marker"
  else
    fail "$file is missing $marker marker"
  fi
done

# 4. Adversarial: broken fixture must fail validation.
fixture_root="$(mktemp -d)"
trap 'rm -rf "$fixture_root"' EXIT

mkdir -p "$fixture_root/bubbles/cheatsheet"
mkdir -p "$fixture_root/bubbles/scripts"
mkdir -p "$fixture_root/docs"

cp "$REPO_ROOT/docs/CHEATSHEET.md" "$fixture_root/docs/CHEATSHEET.md"
cp "$REPO_ROOT/docs/its-not-rocket-appliances.html" "$fixture_root/docs/its-not-rocket-appliances.html"
cp "$REPO_ROOT/bubbles/workflows.yaml" "$fixture_root/bubbles/workflows.yaml"
cp "$REPO_ROOT/bubbles/cheatsheet/vocabulary.json" "$fixture_root/bubbles/cheatsheet/vocabulary.json"
cp "$REPO_ROOT/bubbles/cheatsheet/aliases.json" "$fixture_root/bubbles/cheatsheet/aliases.json"
# Inject a phantom workflow-mode reference.
jq '. + [{"name": "this-mode-does-not-exist", "alias": "phantom", "description": "should fail"}]' \
  "$REPO_ROOT/bubbles/cheatsheet/modes.json" > "$fixture_root/bubbles/cheatsheet/modes.json"

# The generator resolves paths relative to its own directory, so we have to
# install it inside the fixture tree to point at the fixture registry.
cp "$GENERATOR" "$fixture_root/bubbles/scripts/generate-cheatsheet.sh"

if bash "$fixture_root/bubbles/scripts/generate-cheatsheet.sh" --check >/dev/null 2>&1; then
  fail "generator accepted phantom workflow mode (should have rejected)"
else
  pass "generator rejects phantom workflow mode"
fi

# Adversarial 2: duplicate aliases.
jq '. + [{"alias": "pull-the-strings", "maps_to": "bubbles.workflow", "quote": "duplicate"}]' \
  "$REPO_ROOT/bubbles/cheatsheet/aliases.json" > "$fixture_root/bubbles/cheatsheet/aliases.json"
cp "$REPO_ROOT/bubbles/cheatsheet/modes.json" "$fixture_root/bubbles/cheatsheet/modes.json"

if bash "$fixture_root/bubbles/scripts/generate-cheatsheet.sh" --check >/dev/null 2>&1; then
  fail "generator accepted duplicate sunnyvale alias (should have rejected)"
else
  pass "generator rejects duplicate sunnyvale alias"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "generate-cheatsheet-selftest: FAIL ($failures issue(s))"
  exit 1
fi

echo "generate-cheatsheet-selftest: PASS"
exit 0
