#!/usr/bin/env bash
# Gate G095: discovered_issue_disposition_gate
#
# Enforces operating-baseline.md → "Discovered-Issue Disposition":
# Every issue an agent observes during work MUST have an explicit
# disposition (fixed-in-session, bug-filed, spec-filed, ops-filed, routed).
# Saying "pre-existing", "unrelated", "out of scope", "known issue",
# "skipping", "will fix later", "not my session" without filing a
# corresponding artifact is forbidden and counts as fabrication.
#
# Scanning is NARRATIVE-ONLY: fenced code blocks (```) are blanked before the
# paragraph walk, because the Execution Evidence Standard requires verbatim
# terminal captures in report.md and a quoted guard/grep line is evidence, not
# a deferral. An unterminated fence is scanned verbatim (fail-safe toward
# detection) rather than swallowing the rest of the file.
#
# Usage:
#   bash discovered-issue-disposition-guard.sh <spec-dir>
#   bash discovered-issue-disposition-guard.sh <spec-dir> --envelope <envelope-file>
#
# Exit codes:
#   0  clean (no forbidden phrases, OR all forbidden phrases properly dispositioned)
#   1  G095 finding (forbidden phrase without disposition)
#   2  runtime error / malformed input
#
set -uo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <spec-dir> [--envelope <envelope-file>]" >&2
  exit 2
fi

spec_dir="$1"
envelope_file=""
shift
while [[ $# -gt 0 ]]; do
  case "$1" in
    --envelope) envelope_file="$2"; shift 2 ;;
    *) echo "Unknown arg: $1" >&2; exit 2 ;;
  esac
done

if [[ ! -d "$spec_dir" ]]; then
  echo "G095 ERROR: spec dir does not exist: $spec_dir" >&2
  exit 2
fi

report_md="$spec_dir/report.md"

# Forbidden deferral phrases (case-insensitive). Each is a regex.
forbidden_phrases=(
  'pre-existing[^.]*unrelated'
  'unrelated[^.]*pre-existing'
  'out of scope'
  'known issue'
  '\bskipping\b'
  'will (fix|file|address|create) later'
  'not my session'
  'fix in a follow[- ]?up'
  'leave for (now|later)'
  'recommend(ed|s|ing)? (filing|creating a bug|tracking as a bug)'
  'should be filed'
  'await(ing)? (user|operator|your) (approval|authorization|confirmation|decision|sign-?off|go-?ahead)'
  'pending (user|operator|your) (approval|authorization|confirmation|decision|sign-?off)'
  'defer(red|ring)? filing'
  'defer(red|ring)? to a (future|later|separate) (workflow|delivery|run|session|cycle)'
)

# Artifact-reference patterns that count as a valid disposition citation
# in the same paragraph as a forbidden phrase.
disposition_refs=(
  'specs/[^[:space:]]+/bug\.md'
  'specs/[^[:space:]]+/spec\.md'
  'BUG-[0-9]+'
  'TR-[0-9]+'
  'transitionRequests'
  'routedTo'
  'https?://[^[:space:]]+'
  'ops-filed'
  'bug-filed'
  'spec-filed'
  'routed'
  'fixed-in-session'
)

# Build alternation patterns
forbidden_re="$(IFS='|'; echo "${forbidden_phrases[*]}")"
disposition_re="$(IFS='|'; echo "${disposition_refs[*]}")"

today="$(date -u +%Y-%m-%d)"
findings=0

# Helper: check if report.md has a Discovered Issues row with today's date
report_has_today_disposition() {
  [[ -f "$report_md" ]] || return 1
  # Find the ## Discovered Issues section and check for today's date in a row
  awk -v today="$today" '
    /^## Discovered Issues/ { in_section=1; next }
    /^## / && in_section { in_section=0 }
    in_section && $0 ~ today { found=1 }
    END { exit !found }
  ' "$report_md"
}

# Helper: blank out fenced code blocks (```), LINE-COUNT-PRESERVINGLY, so that
# verbatim terminal captures — which Bubbles' own Execution Evidence Standard
# REQUIRES in report.md — are never misread as narrative deferral. Without this
# the guard re-flags its own quoted output: documenting a G095 finding would
# manufacture the next one, and the more disciplined the evidence, the more
# violations are invented.
#
# Fence delimiter lines are blanked too, so a fence is a hard paragraph break
# (markdown-correct) rather than glue joining the narrative on either side of it.
# The delimiter pattern matches the certifying-window helper below, including its
# indented-fence tolerance.
#
# UNBALANCED FENCE (odd delimiter count → still open at EOF): fail SAFE toward
# DETECTION. The unterminated region is emitted VERBATIM so the remainder of the
# file is still scanned; the caller warns on stderr. A stray fence must never
# silently disable the gate — a false negative here is far worse than the false
# positive this helper exists to remove. Signalled via exit status 3.
strip_fenced_blocks() {
  awk '
    function emit_blanks(n,   i) { for (i = 0; i < n; i++) print "" }
    /^[[:space:]]*```/ {
      if (in_fence) { emit_blanks(bn + 1); bn = 0; in_fence = 0 }
      else          { in_fence = 1; bn = 1; buf[1] = $0 }
      next
    }
    in_fence { buf[++bn] = $0; next }
    { print }
    END {
      if (in_fence) {
        for (i = 1; i <= bn; i++) print buf[i]
        exit 3
      }
    }
  ' "$1" > "$2"
}

# Helper: scan a file for forbidden phrases not paired with disposition refs
scan_file() {
  local file="$1"
  local context="$2"
  # Path cited in findings. Line numbers below are 1:1 with this file, because
  # every suppression upstream blanks lines instead of deleting them.
  local display_file="${3:-$1}"
  [[ -f "$file" ]] || return 0

  local scan_src
  local strip_rc=0
  scan_src="$(mktemp)"
  strip_fenced_blocks "$file" "$scan_src" || strip_rc=$?
  if [[ "$strip_rc" -eq 3 ]]; then
    echo "G095 WARNING: unbalanced code fence in $display_file — the unterminated region is scanned VERBATIM (fail-safe: a stray fence must not disable G095)." >&2
  fi

  # Read the file paragraph by paragraph (blank-line separated)
  local current_para=""
  local para_line=0
  local line_num=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_num=$((line_num + 1))
    if [[ -z "$line" ]]; then
      check_paragraph "$current_para" "$display_file" "$para_line" "$context"
      current_para=""
      para_line=0
    else
      if [[ -z "$current_para" ]]; then
        para_line=$line_num
      fi
      current_para="${current_para}${line}
"
    fi
  done < "$scan_src"
  # Final paragraph
  [[ -n "$current_para" ]] && check_paragraph "$current_para" "$display_file" "$para_line" "$context"
  rm -f "$scan_src"
  return 0
}

check_paragraph() {
  local para="$1"
  local file="$2"
  local line_num="$3"
  local context="$4"

  # Find each forbidden phrase in the paragraph
  while IFS= read -r match; do
    [[ -z "$match" ]] && continue
    # Check if same paragraph contains a disposition reference
    if ! grep -qiE "$disposition_re" <<< "$para"; then
      # No inline disposition. Check report.md for today's row.
      if ! report_has_today_disposition; then
        echo "🔴 G095 BLOCK: $context $file:$line_num — forbidden deferral phrase '$match' without disposition citation and no '## Discovered Issues' row for $today in $report_md"
        findings=$((findings + 1))
      fi
    fi
  done < <(echo "$para" | grep -oiE "$forbidden_re" | sort -u)
}

# Scan envelope file if provided (highest priority — most recent narrative)
if [[ -n "$envelope_file" ]]; then
  if [[ ! -f "$envelope_file" ]]; then
    echo "G095 ERROR: envelope file does not exist: $envelope_file" >&2
    exit 2
  fi
  scan_file "$envelope_file" "RESULT-ENVELOPE"
fi

# Scan report.md narrative (excluding the Discovered Issues table itself)
if [[ -f "$report_md" ]]; then
  # Certifying-window boundary (report.md, opt-in, at most ONE), mirroring
  # artifact-lint.sh Check 3: a single out-of-fence
  # <!-- bubbles:certifying-window-begin --> marker freezes the prior-window
  # history region (every line BEFORE it), which the paragraph walk below must
  # NOT re-adjudicate. INTEGRITY: opt-in per file (a marker-less report.md is
  # scanned in full); >1 marker is malformed/ambiguous and fails loud (exit 2).
  cw_count="$(grep -cF -- '<!-- bubbles:certifying-window-begin -->' "$report_md" 2>/dev/null || true)"
  if [[ "$cw_count" -gt 1 ]]; then
    echo "G095 ERROR: multiple certifying-window markers ($cw_count) in $report_md — at most one is allowed (it marks the single current certifying-window start)" >&2
    exit 2
  fi
  # Locate the certifying-window marker, fence-aware (the marker is honored only
  # out-of-fence, matching artifact-lint.sh Check 3). Emits its line number.
  cw_line="$(awk '
    /^[[:space:]]*```/ { in_fence = !in_fence; next }
    !in_fence && /<!-- bubbles:certifying-window-begin -->/ { print NR; exit }
  ' "$report_md")"
  if [[ "$cw_count" -eq 1 && -z "$cw_line" ]]; then
    # The marker exists but is fence-shadowed, so the window start is
    # unresolvable. Honoring it would blank the WHOLE file and silently disable
    # G095 — the catastrophic false negative. Scan-on-doubt instead.
    echo "G095 WARNING: certifying-window marker in $report_md is inside a code fence — window ignored, the report is scanned in FULL (fail-safe)." >&2
  fi
  # Suppress the frozen prior-window history region (every line up to and
  # including the marker) and the Discovered Issues table itself. Suppression
  # BLANKS lines rather than deleting them, so finding line numbers stay 1:1
  # with the real report.md and remain actionable.
  tmp_report="$(mktemp)"
  awk -v cwline="${cw_line:-0}" '
    NR <= cwline { print ""; next }
    /^## Discovered Issues/ { skip=1; print ""; next }
    /^## / && skip { skip=0 }
    skip { print ""; next }
    { print }
  ' "$report_md" > "$tmp_report"
  scan_file "$tmp_report" "report.md" "$report_md"
  rm -f "$tmp_report"
fi

if [[ "$findings" -gt 0 ]]; then
  echo ""
  echo "G095: $findings discovered-issue disposition violation(s)."
  echo "Remediation: for each forbidden phrase, either"
  echo "  (a) cite a concrete artifact reference (BUG-NNN, TR-NNN, spec path, ops URL) in the same paragraph, OR"
  echo "  (b) add a row to '## Discovered Issues' in $report_md dated $today with disposition + reference."
  echo ""
  echo "See agents/bubbles_shared/operating-baseline.md → 'Discovered-Issue Disposition' for the disposition table."
  exit 1
fi

echo "✅ G095: discovered-issue disposition clean (no unfiled deferrals)"
exit 0
