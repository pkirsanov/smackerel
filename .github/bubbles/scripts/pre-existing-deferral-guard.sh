#!/usr/bin/env bash
set -euo pipefail

# pre-existing-deferral-guard.sh
#
# Gate G084 — pre_existing_deferral_block_gate.
#
# Mechanically enforces the "no pre-existing-failure deferrals" rule for
# downstream/fixture specs (any spec at `specs/NNN-*/`). Blocks a spec
# from reaching `status: done` while its `report.md`, per-scope
# `report.md`, or per-scope `scope.md` contains pre-existing-failure
# deferral phrases in any ACTIVE (non-exempt) section.
#
# Files scanned (relative to <specDir>):
#   - report.md
#   - scopes/*/report.md
#   - scopes/*/scope.md
#
# Forbidden phrases (case-insensitive substring match):
#   - pre-existing failure
#   - pre-existing test failure
#   - carried forward
#   - out of session scope
#   - previous-session failure
#   - not introduced by this spec
#
# Forbidden deferral markers (case-sensitive, conventional colon form
# only — bare ALL-CAPS prose like "TODO/FIXME" in a Test Plan row is
# NOT flagged; only the actionable `MARKER:` form is treated as a
# deferral hit):
#   - TODO:
#   - FIXME:
#   - HACK:
#   - STUB:
#
# Exempt H2 subsections (boundary ends at next H2 header):
#   - ## Superseded Decisions
#   - ## Historical Notes
#   - ## Out of Scope
#
# Exempt content (within active sections):
#   - Lines inside ``` ... ``` fenced code blocks (with optional leading
#     whitespace, so fences nested inside list-item evidence blocks are
#     still detected as exempt)
#   - Content inside `inline backticks` (replaced with whitespace before
#     phrase matching so legitimate documentation enumerations such as
#     `pre-existing failure` are not flagged)
#
# Exit codes:
#   0  no forbidden phrase found in any active, non-fenced, non-backticked
#      span of the scanned files
#   1  at least one forbidden phrase found; stderr cites G084,
#      pre_existing_deferral_block_gate, and the offending
#      <file>:<line>:<phrase> for every hit
#   2  malformed / missing inputs (specDir missing, not a directory,
#      missing required arguments) — diagnostic on stderr
#
# Usage:
#   bash bubbles/scripts/pre-existing-deferral-guard.sh <specDir> [--quiet]
#
# Inputs:
#   <specDir>   Path to the spec directory (e.g.
#               specs/900-convergence-fixture). May be absolute
#               or repo-relative.
#   --quiet     Suppress informational stdout on success.
#
# Dependencies:
#   - awk (POSIX)
#
# Reference:
#   docs/Framework_Convergence_Health.md

QUIET="false"
SPEC_DIR=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/pre-existing-deferral-guard.sh <specDir> [--quiet]

Required:
  <specDir>   Spec directory whose pre-existing-failure deferral
              language is inspected (e.g.
              specs/900-convergence-fixture).

Optional:
  --quiet     Suppress informational stdout; the final PASS or VIOLATION
              line is still emitted (stdout on pass, stderr on fail).
  -h, --help  Print this usage and exit.

Exit codes:
  0 = no forbidden phrase found in any active section
  1 = forbidden phrase found (Gate G084 violation)
  2 = malformed inputs or missing arguments
EOF
}

# --- Argument parsing ----------------------------------------------------

if [[ $# -eq 0 ]]; then
  usage >&2
  exit 2
fi

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --quiet)
      QUIET="true"
      shift
      ;;
    --*)
      echo "pre-existing-deferral-guard: unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -z "$SPEC_DIR" ]]; then
        SPEC_DIR="$1"
      else
        echo "pre-existing-deferral-guard: unexpected positional argument: $1" >&2
        usage >&2
        exit 2
      fi
      shift
      ;;
  esac
done

if [[ -z "$SPEC_DIR" ]]; then
  echo "pre-existing-deferral-guard: <specDir> is required" >&2
  usage >&2
  exit 2
fi

if [[ ! -d "$SPEC_DIR" ]]; then
  echo "pre-existing-deferral-guard: specDir not found or not a directory: $SPEC_DIR" >&2
  exit 2
fi

info() {
  if [[ "$QUIET" != "true" ]]; then
    echo "pre-existing-deferral-guard: $*"
  fi
}

# --- awk dependency check ------------------------------------------------

if ! command -v awk >/dev/null 2>&1; then
  echo "pre-existing-deferral-guard: awk is required but not found in PATH" >&2
  exit 2
fi

NORMALIZED_SPEC="${SPEC_DIR%/}"

# --- Collect target files ------------------------------------------------
#
# We scan exactly three file patterns under the specDir:
#   1. <specDir>/report.md
#   2. <specDir>/scopes/*/report.md
#   3. <specDir>/scopes/*/scope.md
#
# Missing files are silently skipped (a spec may legitimately have no
# top-level report.md, or no per-scope dirs in single-file mode).

TARGETS=()

if [[ -f "$NORMALIZED_SPEC/report.md" ]]; then
  TARGETS+=("$NORMALIZED_SPEC/report.md")
fi

if [[ -d "$NORMALIZED_SPEC/scopes" ]]; then
  # Sort for deterministic ordering across invocations.
  while IFS= read -r -d '' f; do
    TARGETS+=("$f")
  done < <(find "$NORMALIZED_SPEC/scopes" -mindepth 2 -maxdepth 2 \
             -type f \( -name 'report.md' -o -name 'scope.md' \) \
             -print0 | sort -z)
fi

if [[ ${#TARGETS[@]} -eq 0 ]]; then
  info "no target files (report.md / scopes/*/report.md / scopes/*/scope.md) found under $NORMALIZED_SPEC"
  echo "PASS Gate G084 (pre_existing_deferral_block_gate) — no target files, specDir=$NORMALIZED_SPEC"
  exit 0
fi

# --- Scan each target file with awk -------------------------------------
#
# The awk program tracks two stateful flags per file:
#   - in_exempt: currently inside an exempt H2 section (Superseded
#     Decisions, Historical Notes, Out of Scope). Reset to 0 on any
#     other H2 header.
#   - in_fence:  currently inside a ``` ... ``` fenced code block.
#     Toggled on each line that begins with three backticks.
#
# When BOTH flags are 0 for a given line, the line is "active" content.
# We strip inline `...` backtick spans (content replaced with single
# space) and then check the remaining text against the forbidden-phrase
# list. Substring matches are case-insensitive; deferral markers
# (TODO/FIXME/HACK/STUB) are matched with surrounding non-letter
# context to avoid hits inside camelCase identifiers.
#
# Each hit is printed as a tab-separated record:
#   <file>\t<lineNo>\t<phrase>
# Exit status from awk: 0 = no hits, 1 = >=1 hit.

VIOLATIONS_FILE="$(mktemp -t bubbles-g084-violations-XXXXXXXX)"
trap 'rm -f "$VIOLATIONS_FILE"' EXIT INT TERM

AWK_PROG='
BEGIN {
  # Substring patterns (lowercased before match).
  sp_count = 0
  sp[++sp_count] = "pre-existing failure"
  sp[++sp_count] = "pre-existing test failure"
  sp[++sp_count] = "carried forward"
  sp[++sp_count] = "out of session scope"
  sp[++sp_count] = "previous-session failure"
  sp[++sp_count] = "not introduced by this spec"

  # Colon-anchored markers (matched case-sensitively in the ORIGINAL
  # line, after stripping inline backticks). We deliberately require
  # the trailing `:` so the rule fires only on conventional deferral
  # markers (e.g. `TODO: refactor this`) and not on incidental
  # prose like "an active TODO/FIXME deferral" in a Test Plan row.
  wp_count = 0
  wp[++wp_count] = "TODO:"
  wp[++wp_count] = "FIXME:"
  wp[++wp_count] = "HACK:"
  wp[++wp_count] = "STUB:"
}

function reset_state() {
  in_exempt = 0
  in_fence = 0
}

# Awk built-in NR resets per-file when using ENDFILE handler. We
# instead reset on every FNR == 1.
FNR == 1 { reset_state() }

# H2 header detection. Anything starting with "## " is an H2; check
# whether it is an exempt section header.
/^## / {
  # Extract the header text after "## " and trim trailing whitespace.
  header = substr($0, 4)
  sub(/[[:space:]]+$/, "", header)
  # Case-insensitive comparison against the three exempt names.
  hlc = tolower(header)
  if (hlc == "superseded decisions" || hlc == "historical notes" || hlc == "out of scope") {
    in_exempt = 1
  } else {
    in_exempt = 0
  }
  next
}

# Code-fence detection. Lines that begin with three backticks (with
# optional leading whitespace, so fences nested inside list-item
# evidence blocks are still detected) toggle the fence state. The
# fence delimiter itself is never scanned.
/^[[:space:]]*```/ {
  in_fence = !in_fence
  next
}

# Skip exempt or fenced lines entirely.
in_exempt || in_fence { next }

# Active line: strip inline `...` backtick spans (replace each match
# with a single space so word boundaries are preserved).
{
  line = $0
  while (match(line, /`[^`]*`/)) {
    line = substr(line, 1, RSTART - 1) " " substr(line, RSTART + RLENGTH)
  }

  # Substring phrase check (case-insensitive).
  lower = tolower(line)
  for (i = 1; i <= sp_count; i++) {
    if (index(lower, sp[i]) > 0) {
      printf "%s\t%d\t%s\n", FILENAME, FNR, sp[i]
      hits++
    }
  }

  # Colon-anchored marker check (case-sensitive). We look for the
  # literal `TODO:` / `FIXME:` / `HACK:` / `STUB:` and require the
  # character immediately preceding the marker to NOT be an identifier
  # character (letter/digit/underscore) so we never match inside a
  # CamelCase identifier such as "MyTODO:".
  padded = " " line " "
  for (j = 1; j <= wp_count; j++) {
    marker = wp[j]
    mlen = length(marker)
    pos = 1
    while ((idx = index(substr(padded, pos), marker)) > 0) {
      abs = pos + idx - 1
      before = substr(padded, abs - 1, 1)
      if (before !~ /[A-Za-z0-9_]/) {
        printf "%s\t%d\t%s\n", FILENAME, FNR, marker
        hits++
      }
      pos = abs + mlen
    }
  }
}

END {
  if (hits > 0) {
    exit 1
  }
  exit 0
}
'

# Run awk over all targets at once so we get a unified violations stream.
# We capture the exit code separately to avoid losing it under set -e.
set +e
awk "$AWK_PROG" "${TARGETS[@]}" > "$VIOLATIONS_FILE" 2>/dev/null
AWK_RC=$?
set -e

# Defensive: awk should return 0 or 1; any other code is a malformed
# scan and we surface it as exit 2.
if [[ "$AWK_RC" -ne 0 && "$AWK_RC" -ne 1 ]]; then
  echo "pre-existing-deferral-guard: awk scan failed with unexpected exit code $AWK_RC" >&2
  exit 2
fi

VIOLATION_COUNT=0
if [[ -s "$VIOLATIONS_FILE" ]]; then
  VIOLATION_COUNT="$(wc -l < "$VIOLATIONS_FILE" | tr -d ' ')"
fi

# --- Decision -----------------------------------------------------------

if [[ "$VIOLATION_COUNT" -gt 0 ]]; then
  {
    echo "G084 pre_existing_deferral_block_gate violation"
    echo "  specDir:           $NORMALIZED_SPEC"
    echo "  scanned files:     ${#TARGETS[@]}"
    echo "  violations found:  $VIOLATION_COUNT"
    echo "  hits (file:line:phrase):"
    while IFS=$'\t' read -r vf vl vp; do
      printf '    %s:%d: forbidden phrase "%s"\n' "$vf" "$vl" "$vp"
    done < "$VIOLATIONS_FILE"
    echo "  remediation:       fix the pre-existing failure inline (do NOT defer to a follow-up session). If the language is intentionally historical, move it under '## Superseded Decisions', '## Historical Notes', or '## Out of Scope', or wrap the phrase in inline backticks if it is enumeration prose."
  } >&2
  exit 1
fi

info "specDir=$NORMALIZED_SPEC scannedFiles=${#TARGETS[@]} violations=0"
echo "PASS Gate G084 (pre_existing_deferral_block_gate) — scannedFiles=${#TARGETS[@]} violations=0, specDir=$NORMALIZED_SPEC"
exit 0
