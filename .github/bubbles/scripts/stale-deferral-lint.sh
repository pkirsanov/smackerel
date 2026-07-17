#!/usr/bin/env bash
# stale-deferral-lint.sh — flags lapsed forward-references in live surfaces.
#
# A "deferred to vX.Y" note is a PROMISE to ship something in a future release.
# Once the current VERSION reaches or passes vX.Y that promise has come due: the
# text is either a lapsed promise (the thing never shipped) or stale wording (it
# shipped but the deferral note was never removed). Either way it misleads
# readers and must be reconciled.
#
# This guard scans LIVE operator/agent/code surfaces for `deferred to vX.Y`
# (also `deferred until vX.Y`) where X.Y <= the current VERSION's MAJOR.MINOR,
# and FAILS. It is the mechanical backstop for the framework's own
# "no silent deferrals" discipline — the class of drift that let MCP templated
# resources sit deferred several releases past their stated target.
#
# EXCLUDED — files that legitimately preserve a PAST promise as a historical
# record (mirrors the migrate-modes-v5-to-v6 exclusion set):
#   - CHANGELOG.md           (per-release history; records what each version said)
#   - docs/v6-mcp-design.md  (frozen design doc; preserves v6.0-era decisions)
#   - docs/v5.2-design.md    (frozen design doc)
#   - docs/decisions/*       (ADRs record the decision context as it was)
#   - this script's own selftest (it embeds lapsed-deferral strings as test
#     fixtures, exactly like migrate-modes-v5-to-v6-selftest embeds v5 names)
#
# Usage:
#   stale-deferral-lint.sh [REPO_ROOT]   # default REPO_ROOT=.
#
# Exit 0 = clean. Exit 1 = at least one lapsed deferral. No --skip / --force.

set -euo pipefail

REPO_ROOT="${1:-.}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
VERSION_FILE="$REPO_ROOT/VERSION"

err() { echo "[stale-deferral-lint][ERROR] $*" >&2; }
info() { echo "[stale-deferral-lint] $*"; }

[[ -f "$VERSION_FILE" ]] || { err "VERSION not found at $VERSION_FILE"; exit 1; }
CURRENT_VERSION="$(tr -d '[:space:]' < "$VERSION_FILE")"
[[ "$CURRENT_VERSION" =~ ^[0-9]+\.[0-9]+ ]] || { err "VERSION '$CURRENT_VERSION' is not semver"; exit 1; }
CUR_MAJOR="${CURRENT_VERSION%%.*}"
_rest="${CURRENT_VERSION#*.}"
CUR_MINOR="${_rest%%.*}"
cur_key=$(( CUR_MAJOR * 1000 + CUR_MINOR ))

DEFERRAL_RE='[Dd]eferred (to|until) (the )?v?[0-9]+\.[0-9]+'

is_markdown_heading() {
  local line="$1"
  local heading_re='^[[:space:]]{0,3}#{1,6}([[:space:]]|$)'
  [[ "$line" =~ $heading_re ]]
}

is_markdown_fence_line() {
  local line="$1"
  local fence_re='^[[:space:]]{0,3}(```|~~~)'
  [[ "$line" =~ $fence_re ]]
}

scan_report_live_content() {
  local file="$1"
  local line
  local record_phase=0
  local record_command=0
  local record_exit_code=0
  local record_claim_source=0
  local in_candidate=0
  local candidate_valid=0
  local candidate_buffer=''
  local in_other_fence=0
  local other_fence_close=''

  while IFS= read -r line || [[ -n "$line" ]]; do
    if (( in_candidate )); then
      candidate_buffer="${candidate_buffer}"$'\n'"${line}"
      if [[ "$line" == '```' ]]; then
        if (( ! candidate_valid )); then
          printf '%s\n' "$candidate_buffer"
        fi
        in_candidate=0
        candidate_valid=0
        candidate_buffer=''
        record_phase=0
        record_command=0
        record_exit_code=0
        record_claim_source=0
      elif is_markdown_heading "$line" || is_markdown_fence_line "$line"; then
        candidate_valid=0
      fi
      continue
    fi

    if (( in_other_fence )); then
      printf '%s\n' "$line"
      if [[ "$line" == "$other_fence_close" ]]; then
        in_other_fence=0
        other_fence_close=''
        record_phase=0
        record_command=0
        record_exit_code=0
        record_claim_source=0
      fi
      continue
    fi

    if is_markdown_heading "$line"; then
      record_phase=0
      record_command=0
      record_exit_code=0
      record_claim_source=0
      printf '%s\n' "$line"
      continue
    fi

    if [[ "$line" == '```text' ]]; then
      in_candidate=1
      candidate_buffer="$line"
      if (( record_phase && record_command && record_exit_code && record_claim_source )); then
        candidate_valid=1
      fi
      record_phase=0
      record_command=0
      record_exit_code=0
      record_claim_source=0
      continue
    fi

    if is_markdown_fence_line "$line"; then
      case "$line" in
        '~~~'*) other_fence_close='~~~' ;;
        *) other_fence_close='```' ;;
      esac
      in_other_fence=1
      record_phase=0
      record_command=0
      record_exit_code=0
      record_claim_source=0
      printf '%s\n' "$line"
      continue
    fi

    [[ "$line" == *'**Phase:**'* ]] && record_phase=1
    if [[ "$line" == *'**Command:**'* || "$line" == *'**Commands:**'* ]]; then
      record_command=1
    fi
    [[ "$line" == *'**Exit Code'*':**'* ]] && record_exit_code=1
    [[ "$line" == *'**Claim Source:** executed'* ]] && record_claim_source=1
    printf '%s\n' "$line"
  done < "$file"

  if (( in_candidate )); then
    printf '%s\n' "$candidate_buffer"
  fi
}

scan_file_refs() {
  local file="$1"
  if [[ "${file##*/}" == "report.md" ]]; then
    scan_report_live_content "$file" \
      | grep -oiE "$DEFERRAL_RE" \
      | grep -oE '[0-9]+\.[0-9]+' \
      || true
  else
    grep -oiE "$DEFERRAL_RE" "$file" 2>/dev/null \
      | grep -oE '[0-9]+\.[0-9]+' \
      || true
  fi
}

FAILED=0
while IFS= read -r f; do
  [[ -n "$f" ]] || continue
  case "$f" in
    "$REPO_ROOT/CHANGELOG.md") continue ;;
    "$REPO_ROOT/docs/v6-mcp-design.md") continue ;;
    "$REPO_ROOT/docs/v5.2-design.md") continue ;;
    "$REPO_ROOT"/docs/decisions/*) continue ;;
    "$REPO_ROOT/bubbles/scripts/stale-deferral-lint-selftest.sh") continue ;;
  esac
  while IFS= read -r ref; do
    [[ -n "$ref" ]] || continue
    ref_major="${ref%%.*}"
    ref_minor="${ref#*.}"
    ref_minor="${ref_minor%%.*}"
    ref_key=$(( ref_major * 1000 + ref_minor ))
    if (( ref_key <= cur_key )); then
      rel="${f#"$REPO_ROOT"/}"
      err "$rel references 'deferred to v$ref' but current VERSION is $CURRENT_VERSION (>= v$ref). The promised release has arrived — implement it, or restate the status without a lapsed version promise."
      FAILED=1
    fi
  done < <(scan_file_refs "$f")
done < <(
  grep -rilE "$DEFERRAL_RE" "$REPO_ROOT" \
    --include='*.md' --include='*.py' --include='*.sh' --include='*.json' \
    --include='*.yaml' --include='*.yml' \
    --exclude-dir='.git' --exclude-dir='node_modules' --exclude-dir='target' \
    --exclude-dir='dist' --exclude-dir='build' --exclude-dir='.pre-push-validated' \
    2>/dev/null | sort -u
)

if [[ "$FAILED" -eq 0 ]]; then
  info "OK — no lapsed forward-references (current VERSION $CURRENT_VERSION)"
fi
exit "$FAILED"
