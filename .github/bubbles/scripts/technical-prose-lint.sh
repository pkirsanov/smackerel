#!/usr/bin/env bash
# technical-prose-lint.sh — REPORT-ONLY form check for generated technical prose.
#
# Reports mechanically detectable form defects in the CALLER-SUPPLIED markdown
# surface, per skills/bubbles-technical-prose/SKILL.md. It never blocks.
#
# REPORT-ONLY BY DESIGN. This tool always exits 0 on a readable surface. There
# is no threshold, no failure mode, and no bypass flag, because there is nothing
# to bypass. The precedent is bubbles/scripts/skill-description-load.sh, whose
# load report stays advisory "until calibration evidence justifies one".
#
# Registering a blocking gate on top of this output is a separate, conditional
# decision that has NOT been taken. If calibration shows the lint cannot
# separate real form defects from evidence blocks, generated tables, and quoted
# output, the correct outcome is that it stays report-only permanently. A
# blocking gate over an untrustworthy signal is worse than no gate.
#
# REUSABLE BY DESIGN — the scan surface is an argument (files and/or
# directories). There is NO default surface, following the
# macos-portability-guard.sh precedent: the tool is never implicitly pointed at
# a whole repository.
#
# ---------------------------------------------------------------------------
# WHAT IS CHECKED (priority order)
#
#   over-long-sentence   a prose sentence longer than 25 words
#   prose-semicolon      a semicolon in prose (rule 8.1 excludes ONLY this mark)
#   term-spelling        a registry term written in a split form the registry
#                        does not use (for example "sub-agent" against the
#                        registered "subagent")
#   phrasal-verb         a phrasal verb where one verb exists
#   nominalisation       a verb turned into a noun plus a weak carrier verb
#
# WHY TERM *DRIFT* IS NOT CHECKED
#
#   The registry records notInsteadOf clusters such as gate / guard / check.
#   Those terms are DELIBERATELY distinct and legitimately co-occur in the same
#   sentence, so counting their uses measures vocabulary size, not drift. A rule
#   built that way reports hundreds of correct uses as findings, and a
#   report-only lint that cries wolf gets ignored. Only the mechanical half —
#   a term written in a form the registry does not carry — is checked here.
#
# WHAT IS DELIBERATELY *NOT* CHECKED
#
#   em dashes            PERMITTED. The source standard's rule 8.1 reads "You
#                        can use all standard English punctuation marks except
#                        the semicolon (;)." A no-em-dash rule is a folk
#                        addition the standard does not contain, and enforcing
#                        it would put thousands of conforming sentences into a
#                        false-positive backlog.
#
#   marketing adjectives NOT BUILDABLE HONESTLY. Words such as "seamless",
#                        "robust", "leverage" appear ZERO times in the standard.
#                        They are excluded by a positive allowlist of approved
#                        words, which is copyrighted and cannot be shipped. Any
#                        implementation here would degenerate into exactly the
#                        hand-maintained blacklist the original argument objects
#                        to. It MUST NOT be built.
#
# ---------------------------------------------------------------------------
# EXCLUSIONS ARE MANDATORY AND NON-NEGOTIABLE
#
# The following are stripped before any rule runs:
#
#   * fenced code blocks (``` and ~~~), including the fence lines
#   * markdown tables (any line whose trimmed form starts with |)
#   * inline code spans (`...`)
#   * blockquoted evidence (any line whose trimmed form starts with >)
#   * link targets and image targets
#   * headings, list markers, and HTML comments
#
# The evidence exclusion is the highest-severity requirement in this tool.
# Bubbles report.md evidence is VERBATIM terminal output under
# bubbles-evidence-capture and bubbles-anti-fabrication. A prose lint that
# induced an agent to reword captured output would corrupt evidence integrity —
# a far worse defect than any form problem it could ever find.
#
# ---------------------------------------------------------------------------
# Portable BY DESIGN: bash 3.2 + BSD userland and Linux/WSL. No GNU-only
# sed -i / date -d / grep -P. Passes macos-portability-guard.sh and shellcheck.
#
# Exit codes:
#   0  report printed (defects found or not — this tool does not fail)
#   2  usage error (no surface given, unreadable path, -h)
#
# There is NO --skip, --force, or --ignore flag.
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
  cat <<'USAGE'
usage: technical-prose-lint.sh [--detail] [--vocabulary FILE] [--max-words N] PATH [PATH...]

  PATH              one or more markdown files, or directories to scan for *.md
  --detail          list every finding, not just the per-rule counts
  --vocabulary FILE term registry (default: sibling cheatsheet/vocabulary.json)
  --max-words N     descriptive sentence ceiling (default 25)

Report-only. Always exits 0 on a readable surface. Exits 2 on usage error.
USAGE
}

MAX_WORDS=25
VOCAB=""
SURFACE=""
DETAIL=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage; exit 2 ;;
    --detail) DETAIL=1; shift ;;
    --max-words) MAX_WORDS="${2:-}"; shift 2 ;;
    --max-words=*) MAX_WORDS="${1#*=}"; shift ;;
    --vocabulary) VOCAB="${2:-}"; shift 2 ;;
    --vocabulary=*) VOCAB="${1#*=}"; shift ;;
    --skip|--force|--ignore|--skip=*|--force=*|--ignore=*)
      printf '[technical-prose][USAGE] %s is not a flag this tool has. It is report-only; there is nothing to bypass.\n' "${1%%=*}" >&2
      exit 2 ;;
    -*)
      printf '[technical-prose][USAGE] unknown argument: %s\n' "$1" >&2
      exit 2 ;;
    *)
      if [[ ! -e "$1" ]]; then
        printf '[technical-prose][USAGE] no such path: %s\n' "$1" >&2
        exit 2
      fi
      SURFACE="${SURFACE}${1}"$'\n'
      shift ;;
  esac
done

if [[ -z "$SURFACE" ]]; then
  printf '[technical-prose][USAGE] no scan surface given. This tool has no default surface.\n' >&2
  usage >&2
  exit 2
fi

case "$MAX_WORDS" in
  ''|*[!0-9]*)
    printf '[technical-prose][USAGE] --max-words needs a number, got: %s\n' "$MAX_WORDS" >&2
    exit 2 ;;
esac

if [[ -z "$VOCAB" ]]; then
  for candidate in \
    "$SCRIPT_DIR/../cheatsheet/vocabulary.json" \
    "$SCRIPT_DIR/../../bubbles/cheatsheet/vocabulary.json"; do
    if [[ -f "$candidate" ]]; then VOCAB="$candidate"; break; fi
  done
fi

work_dir="$(mktemp -d "${TMPDIR:-/tmp}/technical-prose.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT

# ---------------------------------------------------------------------------
# Collect the file list.
file_list="$work_dir/files"
: > "$file_list"
while IFS= read -r entry; do
  [[ -n "$entry" ]] || continue
  if [[ -d "$entry" ]]; then
    find "$entry" -type f -name '*.md' 2>/dev/null | sort >> "$file_list"
  else
    printf '%s\n' "$entry" >> "$file_list"
  fi
done < <(printf '%s' "$SURFACE")

if [[ ! -s "$file_list" ]]; then
  printf '[technical-prose] no markdown files in the given surface — nothing to report.\n'
  exit 0
fi

# ---------------------------------------------------------------------------
# Build the term-spelling table from the registry, if one is available.
# One registered term per line. Only single-word terms carrying no hyphen are
# usable, because the check looks for a hyphen-split variant of that exact word.
term_table="$work_dir/terms"
: > "$term_table"
if [[ -n "$VOCAB" && -f "$VOCAB" ]]; then
  awk '
    /"term"[[:space:]]*:/ {
      line = $0
      sub(/^.*"term"[[:space:]]*:[[:space:]]*"/, "", line)
      sub(/".*$/, "", line)
      if (line ~ /^[a-z][a-z]+$/ && length(line) >= 6) { print line }
    }
  ' "$VOCAB" > "$term_table" 2>/dev/null || : > "$term_table"
fi

# ---------------------------------------------------------------------------
# Shortlists. Deliberately small and conservative: a form lint that cries wolf
# gets ignored, and an ignored report-only lint is worse than none.
phrasal_list="kick off|look into|call off|carry out|set up the|put in place|come up with|figure out|take care of"
nominal_list="perform [a-z]+ation|carry out [a-z]+ation|make use of|provide support for|give consideration to|do a review of|conduct an analysis of"

total_files=0
total_sentences=0
findings_file="$work_dir/findings"
: > "$findings_file"

while IFS= read -r md; do
  [[ -f "$md" ]] || continue
  [[ -r "$md" ]] || continue
  total_files=$((total_files + 1))

  # Strip every excluded surface, preserving line numbers by blanking lines.
  prose="$work_dir/prose"
  awk '
    BEGIN { fenced = 0 }
    {
      line = $0
      trimmed = line
      sub(/^[[:space:]]+/, "", trimmed)

      # Fenced code blocks — the fence lines themselves are excluded too.
      if (trimmed ~ /^(```|~~~)/) { fenced = !fenced; print ""; next }
      if (fenced) { print ""; next }

      # Tables, blockquoted evidence, headings, HTML comments.
      if (trimmed ~ /^\|/)   { print ""; next }
      if (trimmed ~ /^>/)    { print ""; next }
      if (trimmed ~ /^#/)    { print ""; next }
      if (trimmed ~ /^<!--/) { print ""; next }
      if (trimmed ~ /^(---|===|\*\*\*)/) { print ""; next }

      # YAML frontmatter delimiters are handled by the --- rule above.

      # Inline code spans, link targets, image targets.
      gsub(/`[^`]*`/, " ", line)
      gsub(/\]\([^)]*\)/, "] ", line)
      gsub(/https?:\/\/[^ )]*/, " ", line)

      # List markers are not prose.
      sub(/^[[:space:]]*([-*+]|[0-9]+\.)[[:space:]]+/, "", line)

      print line
    }
  ' "$md" > "$prose"

  # ---- over-long sentences -------------------------------------------------
  awk -v file="$md" -v maxw="$MAX_WORDS" '
    {
      buf = buf " " $0
    }
    END {
      gsub(/[[:space:]]+/, " ", buf)
      n = split(buf, parts, /[.!?](["\x27)]*)( |$)/)
      for (i = 1; i <= n; i++) {
        s = parts[i]
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", s)
        if (s == "") continue
        wc = split(s, w, /[[:space:]]+/)
        total++
        if (wc > maxw) {
          short = substr(s, 1, 60)
          printf "over-long-sentence\t%s\t%d words\t%s...\n", file, wc, short
        }
      }
      printf "__COUNT__\t%d\n", total
    }
  ' "$prose" >> "$findings_file"

  # ---- prose semicolons ----------------------------------------------------
  awk -v file="$md" '
    /;/ {
      line = $0
      # HTML entities are not prose semicolons.
      gsub(/&[a-zA-Z#0-9]+;/, " ", line)
      if (line ~ /;/) {
        short = line
        gsub(/^[[:space:]]+/, "", short)
        printf "prose-semicolon\t%s\tline %d\t%s\n", file, NR, substr(short, 1, 60)
      }
    }
  ' "$prose" >> "$findings_file"

  # ---- phrasal verbs -------------------------------------------------------
  awk -v file="$md" -v pat="$phrasal_list" '
    BEGIN { n = split(pat, p, "|") }
    {
      low = tolower($0)
      for (i = 1; i <= n; i++) {
        if (index(low, p[i]) > 0) {
          printf "phrasal-verb\t%s\tline %d\t%s\n", file, NR, p[i]
        }
      }
    }
  ' "$prose" >> "$findings_file"

  # ---- nominalisations -----------------------------------------------------
  awk -v file="$md" -v pat="$nominal_list" '
    BEGIN { n = split(pat, p, "|") }
    {
      low = tolower($0)
      for (i = 1; i <= n; i++) {
        if (low ~ p[i]) {
          printf "nominalisation\t%s\tline %d\t%s\n", file, NR, p[i]
        }
      }
    }
  ' "$prose" >> "$findings_file"

  # ---- registry term spelling ---------------------------------------------
  if [[ -s "$term_table" ]]; then
    awk -v file="$md" '
      NR == FNR {
        term = $1
        # Every hyphen-split variant of the registered term.
        for (i = 2; i < length(term); i++) {
          variant = substr(term, 1, i) "-" substr(term, i + 1)
          split_of[variant] = term
        }
        next
      }
      {
        low = tolower($0)
        for (v in split_of) {
          if (index(low, v) > 0) { seen[v]++ }
        }
      }
      END {
        for (v in seen) {
          printf "term-spelling\t%s\t%d use(s)\t%s -- the registry carries %s\n", file, seen[v], v, split_of[v]
        }
      }
    ' "$term_table" "$prose" >> "$findings_file"
  fi
done < "$file_list"

# ---------------------------------------------------------------------------
# Report.
sentence_total="$(awk -F'\t' '$1 == "__COUNT__" { s += $2 } END { printf "%d", s + 0 }' "$findings_file")"
total_sentences="$sentence_total"

printf '\n=== Technical prose report (IMP-030 SCOPE-3) ===\n'
printf 'files scanned:        %d\n' "$total_files"
printf 'prose sentences:      %d\n' "$total_sentences"

for rule in over-long-sentence prose-semicolon phrasal-verb nominalisation term-spelling; do
  count="$(awk -F'\t' -v r="$rule" '$1 == r { n++ } END { printf "%d", n + 0 }' "$findings_file")"
  printf '%-22s %d\n' "$rule:" "$count"
done

finding_total="$(awk -F'\t' '$1 != "__COUNT__" { n++ } END { printf "%d", n + 0 }' "$findings_file")"
if [[ "$DETAIL" -eq 1 && "$finding_total" != "0" ]]; then
  printf '\n--- detail ---\n'
  awk -F'\t' '$1 != "__COUNT__" { printf "  [%s] %s (%s) %s\n", $1, $2, $3, $4 }' "$findings_file"
elif [[ "$finding_total" != "0" ]]; then
  printf '\nre-run with --detail to list all %s finding(s).\n' "$finding_total"
fi

printf '\nform is Tier 2. It never outranks an honest finding — see analytical-rigor.md.\n'
printf '[technical-prose] report-only, exit 0.\n'
exit 0
