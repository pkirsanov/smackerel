#!/usr/bin/env bash
#
# scan-lib.sh — shared, sourceable helpers for Bubbles guards/lints that SCAN
# files or prose (IMP-009). Centralizes the three recurring scan mistakes that
# produced false-positive findings and forced agents to reword legitimate
# artifacts:
#
#   1. a scan that matches its OWN selftest fixtures (the G115 env-pollution
#      self-match, 2026-06-09);
#   2. a code-evidence grep that matches a mechanism name in a COMMENT, not code;
#   3. a `**Status:**` scan that matches a header/summary BLOCKQUOTE as if it were
#      a canonical per-scope status (BUG-006).
#
# Provides (sourced, not executed):
#   bubbles_scan_files <root> <name-glob>   print files under <root> matching the
#                                           find -name <name-glob>, EXCLUDING
#                                           *selftest* fixtures + generated dirs.
#   bubbles_strip_comments                  filter stdin → drop pure-comment lines.
#   bubbles_status_lines <file>             print canonical '**Status:**' lines,
#                                           EXCLUDING '>'-prefixed blockquotes.
#
# Idempotent: guarded against double-source.

[[ -n "${_BUBBLES_SCAN_LIB_SOURCED:-}" ]] && return 0
_BUBBLES_SCAN_LIB_SOURCED=1

# Enumerate files under <root> matching <name-glob> (a find -name pattern), while
# excluding (a) the guard's OWN selftest fixtures (`*selftest*`), so a scan never
# flags the deliberate fixtures inside its own selftest, and (b) generated/vendor
# dirs that no scan should ever traverse. One path per line.
#
# Mistake #1 prevention: a tree-scanning lint MUST enumerate via this helper (or
# replicate its exclusions) so it cannot self-match its selftest fixtures.
bubbles_scan_files() {
  local root="$1"
  local name_glob="$2"
  [[ -d "$root" ]] || return 0
  find "$root" \
    \( -path '*/.git' -o -path '*/node_modules' -o -path '*/target' \
    -o -path '*/dist' -o -path '*/build' -o -path '*/vendor' \
    -o -path '*/.bubbles-cache' \) -prune -o \
    -type f -name "$name_glob" \
    ! -name '*selftest*' ! -name '*-selftest.sh' \
    -print
}

# Drop pure-comment lines from stdin so a subsequent code-evidence grep does not
# count a mechanism name that appears only in a comment (mistake #2). Strips lines
# whose first non-space token is a common comment marker: // # * /* -- <!-- ;;
bubbles_strip_comments() {
  grep -vE '^[[:space:]]*(//|#|\*|/\*|--|<!--|;;)' || true
}

# Print the canonical '**Status:**' declaration lines from a scope artifact,
# EXCLUDING '>'-prefixed Markdown blockquote lines. A blockquote `**Status:**`
# (e.g. a top-of-file rollup `> **Status:** all scopes Not Started …`) is
# human-readable summary prose, NOT a per-scope status declaration, so it must
# never be read as a scope status or counted in the scope tally (mistake #3 /
# BUG-006). Centralizes the exclusion so Check 4B and Check 5 stay in lockstep.
bubbles_status_lines() {
  local file="$1"
  [[ -f "$file" ]] || return 0
  grep -E '\*\*Status:\*\*' "$file" 2>/dev/null | grep -vE '^[[:space:]]*>' || true
}
