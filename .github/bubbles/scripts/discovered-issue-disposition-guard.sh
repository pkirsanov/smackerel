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

# Helper: scan a file for forbidden phrases not paired with disposition refs
scan_file() {
  local file="$1"
  local context="$2"
  [[ -f "$file" ]] || return 0

  # Read the file paragraph by paragraph (blank-line separated)
  local current_para=""
  local para_line=0
  local line_num=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_num=$((line_num + 1))
    if [[ -z "$line" ]]; then
      check_paragraph "$current_para" "$file" "$para_line" "$context"
      current_para=""
      para_line=0
    else
      if [[ -z "$current_para" ]]; then
        para_line=$line_num
      fi
      current_para="${current_para}${line}
"
    fi
  done < "$file"
  # Final paragraph
  [[ -n "$current_para" ]] && check_paragraph "$current_para" "$file" "$para_line" "$context"
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
    if ! echo "$para" | grep -qiE "$disposition_re"; then
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
  # Strip the Discovered Issues section so we don't flag the table headers
  tmp_report="$(mktemp)"
  awk '
    /^## Discovered Issues/ { skip=1; next }
    /^## / && skip { skip=0 }
    !skip { print }
  ' "$report_md" > "$tmp_report"
  scan_file "$tmp_report" "report.md"
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
