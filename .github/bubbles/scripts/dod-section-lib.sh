#!/usr/bin/env bash
# dod-section-lib.sh — shared Definition-of-Done section parser (BUG-026).
#
# Sourceable and Bash-3.2-safe. Owns ONE lexical contract for DoD section
# boundaries so traceability G068 and state-transition Check 4A/22 can consume
# identical parse results instead of three independently-drifting parsers.
#
# Correct tiered-DoD boundary (fixes BUG026-F002): a DoD section starts at a
# heading of depth 1-4 whose title contains "Definition of Done" or "DoD". The
# section RETAINS nested tier subheadings (depth greater than the start depth,
# through depth 6) and ends ONLY at the next heading whose depth is less than or
# equal to the start depth. Headings inside fenced code blocks (``` or ~~~) or
# HTML comments (<!-- ... -->) are inert. Depths 5 and 6 never START a section.
#
# Checkbox grammar matches the owning guards exactly: `- [x] ` / `- [ ] ` with a
# single trailing space (no `- [X]`); the depth-aware boundary is the only
# behavior the guards did not already have.
#
# Emitted record protocol (tab-delimited, internal/versioned; consumers MUST
# require exactly one terminal STATUS record and reject malformed output):
#   SECTION  <start-line>  <depth>          <visible-title>
#   CHECKBOX <line>        checked|unchecked <item-text>
#   LIST     <line>        checkbox|non-checkbox <verbatim-line>
#   STATUS   rows|rowless|missing|ambiguous|read_error|parse_error <detail>
#
# Protocol version: 1

if [ -n "${_DOD_SECTION_LIB_SOURCED:-}" ]; then
  return 0 2>/dev/null || exit 0
fi
_DOD_SECTION_LIB_SOURCED=1

_DOD_SECTION_LIB_PROTOCOL_VERSION=1

# dod_section_parse <markdown-file>
# Writes the record protocol to stdout. Always emits exactly one terminal
# STATUS record. Returns 0 on a completed parse (the outcome is in STATUS);
# returns 2 only on a usage error (no file argument).
dod_section_parse() {
  local _dod_file="${1:-}"
  if [ -z "$_dod_file" ]; then
    printf 'STATUS\tparse_error\tno file argument\n'
    return 2
  fi
  if [ ! -f "$_dod_file" ] || [ ! -r "$_dod_file" ]; then
    printf 'STATUS\tread_error\t%s\n' "$_dod_file"
    return 0
  fi

  awk '
    BEGIN {
      in_fence = 0; in_comment = 0; in_dod = 0;
      dod_depth = 0; section_count = 0; row_count = 0;
    }
    {
      raw = $0

      # --- HTML comment span (line-oriented) ---
      if (in_comment) {
        if (index(raw, "-->") > 0) { in_comment = 0 }
        next
      }
      if (index(raw, "<!--") > 0 && index(raw, "-->") == 0) {
        in_comment = 1
        next
      }

      # --- fenced code block toggle (``` or ~~~) ---
      if (raw ~ /^[[:space:]]*(```|~~~)/) {
        in_fence = (in_fence ? 0 : 1)
        next
      }
      if (in_fence) { next }

      # --- ATX heading ---
      if (raw ~ /^#+[[:space:]]/) {
        d = 0
        while (substr(raw, d + 1, 1) == "#") { d++ }
        lower = tolower(raw)
        is_dod = (d >= 1 && d <= 4 && (lower ~ /definition of done/ || lower ~ /dod/))

        if (in_dod && d <= dod_depth) {
          # boundary: the open section ends at a same-or-shallower heading
          in_dod = 0
        }
        # a qualifying heading (that is not a nested subheading of an open
        # section) opens a new section
        if (is_dod && !in_dod) {
          in_dod = 1
          dod_depth = d
          section_count++
          title = raw
          sub(/^#+[[:space:]]+/, "", title)
          printf "SECTION\t%d\t%d\t%s\n", NR, d, title
        }
        next
      }

      # --- column-zero list content inside an open DoD section ---
      if (in_dod && raw ~ /^- /) {
        if (raw ~ /^- \[[ x]\] /) {
          if (raw ~ /^- \[x\] /) { checked = "checked" } else { checked = "unchecked" }
          item = raw
          sub(/^- \[[ x]\] /, "", item)
          printf "CHECKBOX\t%d\t%s\t%s\n", NR, checked, item
          printf "LIST\t%d\tcheckbox\t%s\n", NR, raw
          row_count++
        } else {
          printf "LIST\t%d\tnon-checkbox\t%s\n", NR, raw
          row_count++
        }
      }
    }
    END {
      if (in_comment) {
        printf "STATUS\tparse_error\tunterminated HTML comment\n"
      } else if (in_fence) {
        printf "STATUS\tparse_error\tunterminated code fence\n"
      } else if (section_count == 0) {
        printf "STATUS\tmissing\tno Definition of Done / DoD section\n"
      } else if (row_count == 0) {
        printf "STATUS\trowless\t%d section(s), no column-zero list rows\n", section_count
      } else {
        printf "STATUS\trows\t%d row(s) across %d section(s)\n", row_count, section_count
      }
    }
  ' "$_dod_file"
}

# Standalone CLI shim: `bash dod-section-lib.sh <file>` parses and prints.
if [ "${BASH_SOURCE[0]:-$0}" = "$0" ]; then
  dod_section_parse "${1:-}"
  exit $?
fi
