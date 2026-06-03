#!/usr/bin/env bash
# Bubbles cheatsheet drift selftest (v5.0.1 / H7).
#
# Diff-only check: every workflow-mode name and every TPB vocabulary entry
# present in docs/CHEATSHEET.md MUST also appear in docs/its-not-rocket-appliances.html.
# Catches the v5.0 drift class where mode/vocab updates landed in MD but not HTML.
#
# Phase: diff-only. v6 replaces this with a generator (S5 in modernization plan).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
MD="$REPO_ROOT/docs/CHEATSHEET.md"
HTML="$REPO_ROOT/docs/its-not-rocket-appliances.html"

if [[ ! -f "$MD" || ! -f "$HTML" ]]; then
  echo "cheatsheet-drift-selftest: SKIP (cheatsheet files missing)"
  exit 0
fi

failures=0
pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; failures=$((failures + 1)); }

# Extract workflow-mode names from the MD's Workflow Modes table.
# Pattern: lines like `| \`mode-name\` | alias | description |` inside that section.
md_modes="$(awk '
  /^## .*Workflow Modes/ { in_section = 1; next }
  in_section && /^## / { in_section = 0 }
  in_section && /^\| `[a-z][a-z0-9-]*[ `]/ {
    match($0, /`[a-z][a-z0-9-]*`/)
    if (RSTART > 0) {
      name = substr($0, RSTART + 1, RLENGTH - 2)
      print name
    }
  }
' "$MD" | sort -u)"

if [[ -z "$md_modes" ]]; then
  fail "cheatsheet-drift: no workflow modes parsed from CHEATSHEET.md (parser broken?)"
fi

md_mode_count="$(echo "$md_modes" | grep -c .)"

missing_in_html=""
while IFS= read -r mode; do
  [[ -z "$mode" ]] && continue
  if ! grep -Fq ">$mode<" "$HTML" 2>/dev/null \
     && ! grep -Fq "\"$mode\"" "$HTML" 2>/dev/null \
     && ! grep -Fq ">$mode</" "$HTML" 2>/dev/null \
     && ! grep -Fq "<code>$mode</code>" "$HTML" 2>/dev/null; then
    missing_in_html+="$mode"$'\n'
  fi
done <<< "$md_modes"

if [[ -n "$missing_in_html" ]]; then
  fail "Workflow modes present in CHEATSHEET.md but missing from HTML cheatsheet:"
  echo "$missing_in_html" | sed 's/^/  /'
else
  pass "All $md_mode_count workflow-mode names from CHEATSHEET.md appear in HTML cheatsheet"
fi

# Extract TPB Vocabulary terms from the MD's TPB Vocabulary table.
md_vocab="$(awk '
  /^## .*TPB Vocabulary/ { in_section = 1; next }
  in_section && /^## / { in_section = 0 }
  in_section && /^\| `[^`]+` \|/ {
    match($0, /`[^`]+`/)
    if (RSTART > 0) {
      term = substr($0, RSTART + 1, RLENGTH - 2)
      print term
    }
  }
' "$MD" | sort -u)"

md_vocab_count="$(echo "$md_vocab" | grep -c . || true)"

missing_vocab=""
while IFS= read -r term; do
  [[ -z "$term" ]] && continue
  # The HTML stores vocab terms in <div class="wf-name">term</div>.
  if ! grep -Fq ">$term<" "$HTML" 2>/dev/null; then
    missing_vocab+="$term"$'\n'
  fi
done <<< "$md_vocab"

if [[ -n "$missing_vocab" ]]; then
  fail "TPB Vocabulary terms present in CHEATSHEET.md but missing from HTML cheatsheet:"
  echo "$missing_vocab" | sed 's/^/  /'
else
  pass "All $md_vocab_count TPB Vocabulary terms from CHEATSHEET.md appear in HTML cheatsheet"
fi

if [[ "$failures" -gt 0 ]]; then
  echo "cheatsheet-drift-selftest: FAIL ($failures issue(s))"
  exit 1
fi

echo "cheatsheet-drift-selftest: PASS"
exit 0
