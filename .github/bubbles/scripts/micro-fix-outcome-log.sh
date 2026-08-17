#!/usr/bin/env bash
# bubbles/scripts/micro-fix-outcome-log.sh
#
# Capability: bug-packet-proportionality
#
# Forward-looking outcome telemetry for the micro-fix route (IMP-047 S-D).
#
# WHY THIS EXISTS
# The compact packet was switched off pending two measurements: authoring time
# and defect-escape rate. No producer for either existed, and none could exist,
# because both are outcomes of a route nobody was allowed to take. A
# precondition the system cannot produce is a permanent veto dressed as rigour —
# it is the clearest single instance of the bureaucracy IMP-047 exists to
# remove.
#
# So the ordering is inverted. The route is ACTIVATED and the measurement runs
# FROM ACTIVATION FORWARD, as an observed outcome rather than a gate on getting
# started. What makes that safe is not optimism: it is that escalation is
# automatic and has no override, so the admission window cannot widen by
# accident while the numbers accumulate.
#
# WHAT IT RECORDS
#   route    — a bug resolved to the compact or full packet, and why.
#   escape   — a bug CLOSED under the compact packet that was later reopened or
#              refiled. This is the number that decides whether the saved
#              ceremony bought anything, and it can only be recorded by a human
#              who observed the reopening.
#
# CONTRACT
#   - Append-only JSONL at <repo-root>/.specify/runtime/micro-fix-outcomes.jsonl.
#     Past lines are never rewritten; a correction is a new line.
#   - `route` is TELEMETRY and MUST NEVER fail its caller. Every failure path
#     returns 0. An admission guard must not start refusing bugs because a log
#     directory was read-only.
#   - Opt out with BUBBLES_MICRO_FIX_LOG=off. Opting out is silent and safe.
#   - The log lives under .specify/runtime/, which the framework gitignores, so
#     telemetry never enters a commit.
#
# Usage:
#   bash bubbles/scripts/micro-fix-outcome-log.sh route \
#     --bug <bugDir> --route <compact|full> --resolution <declared|default|escalated> \
#     [--failed "no-payment-surface no-auth-surface"] [--repo-root <path>]
#
#   bash bubbles/scripts/micro-fix-outcome-log.sh escape \
#     --bug <bugDir> --reopened-as <ref> [--repo-root <path>]
#
#   bash bubbles/scripts/micro-fix-outcome-log.sh report [--repo-root <path>] [--json]
#
# Exit codes:
#   0 = success (route and escape always return 0)
#   2 = usage error, or report could not read a log that was explicitly named

if [[ "${BASH_SOURCE[0]}" != "${0}" ]]; then
  MICRO_FIX_LOG_IS_SOURCED=1
else
  MICRO_FIX_LOG_IS_SOURCED=0
  set -uo pipefail
fi

MICRO_FIX_LOG_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

bubbles_micro_fix_log_repo_root() {
  # Downstream installs live at <repo>/.github/bubbles/scripts; the source repo
  # at <repo>/bubbles/scripts. Both resolve without a caller having to know.
  if [[ "$(basename "$(dirname "$MICRO_FIX_LOG_DIR")")" == "bubbles" &&
    "$(basename "$(dirname "$(dirname "$MICRO_FIX_LOG_DIR")")")" == ".github" ]]; then
    (cd "$MICRO_FIX_LOG_DIR/../../.." && pwd -P)
  else
    (cd "$MICRO_FIX_LOG_DIR/../.." && pwd -P)
  fi
}

bubbles_micro_fix_log_path() {
  printf '%s/.specify/runtime/micro-fix-outcomes.jsonl' "$1"
}

bubbles_micro_fix_log_ts() {
  date -u '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || printf 'unknown'
}

bubbles_micro_fix_log_json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' | tr -d '\n\r'
}

# Never fails its caller. See CONTRACT above.
bubbles_micro_fix_log_append() {
  [[ "${BUBBLES_MICRO_FIX_LOG:-on}" != "off" ]] || return 0
  local json="$1" repo_root="$2"
  local log_file runtime_dir
  log_file="$(bubbles_micro_fix_log_path "$repo_root")" || return 0
  runtime_dir="$(dirname "$log_file")"
  mkdir -p "$runtime_dir" 2>/dev/null || return 0
  printf '%s\n' "$json" >>"$log_file" 2>/dev/null || return 0
  return 0
}

bubbles_micro_fix_log_route() {
  local bug="" route="" resolution="" failed="" repo_root=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --bug) bug="${2:-}"; shift 2 ;;
      --route) route="${2:-}"; shift 2 ;;
      --resolution) resolution="${2:-}"; shift 2 ;;
      --failed) failed="${2:-}"; shift 2 ;;
      --repo-root) repo_root="${2:-}"; shift 2 ;;
      *) shift ;;
    esac
  done
  [[ -n "$repo_root" ]] || repo_root="$(bubbles_micro_fix_log_repo_root)"
  local failed_json="" item
  for item in $failed; do
    [[ -z "$failed_json" ]] && failed_json="\"$(bubbles_micro_fix_log_json_escape "$item")\"" ||
      failed_json="$failed_json,\"$(bubbles_micro_fix_log_json_escape "$item")\""
  done
  bubbles_micro_fix_log_append "$(printf '{"schemaVersion":"micro-fix-outcome/v1","kind":"route","ts":"%s","bug":"%s","route":"%s","resolution":"%s","failedConditions":[%s]}' \
    "$(bubbles_micro_fix_log_ts)" \
    "$(bubbles_micro_fix_log_json_escape "$bug")" \
    "$(bubbles_micro_fix_log_json_escape "$route")" \
    "$(bubbles_micro_fix_log_json_escape "$resolution")" \
    "$failed_json")" "$repo_root"
  return 0
}

bubbles_micro_fix_log_escape() {
  local bug="" reopened="" repo_root=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --bug) bug="${2:-}"; shift 2 ;;
      --reopened-as) reopened="${2:-}"; shift 2 ;;
      --repo-root) repo_root="${2:-}"; shift 2 ;;
      *) shift ;;
    esac
  done
  [[ -n "$repo_root" ]] || repo_root="$(bubbles_micro_fix_log_repo_root)"
  bubbles_micro_fix_log_append "$(printf '{"schemaVersion":"micro-fix-outcome/v1","kind":"escape","ts":"%s","bug":"%s","reopenedAs":"%s"}' \
    "$(bubbles_micro_fix_log_ts)" \
    "$(bubbles_micro_fix_log_json_escape "$bug")" \
    "$(bubbles_micro_fix_log_json_escape "$reopened")")" "$repo_root"
  return 0
}

bubbles_micro_fix_log_report() {
  local repo_root="" as_json=0
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --repo-root) repo_root="${2:-}"; shift 2 ;;
      --json) as_json=1; shift ;;
      *) shift ;;
    esac
  done
  [[ -n "$repo_root" ]] || repo_root="$(bubbles_micro_fix_log_repo_root)"
  local log_file
  log_file="$(bubbles_micro_fix_log_path "$repo_root")"

  local compact=0 full=0 escapes=0
  if [[ -f "$log_file" ]]; then
    compact="$({ grep -c '"route":"compact"' "$log_file"; } || true)"
    full="$({ grep -c '"route":"full"' "$log_file"; } || true)"
    escapes="$({ grep -c '"kind":"escape"' "$log_file"; } || true)"
  fi
  compact="${compact:-0}"
  full="${full:-0}"
  escapes="${escapes:-0}"

  # The escape RATE is deliberately reported as a fraction with its denominator
  # rather than a percentage. A rate quoted without its denominator is how a
  # sample of one becomes a policy argument.
  if [[ "$as_json" -eq 1 ]]; then
    printf '{"compactRoutes":%s,"fullRoutes":%s,"escapes":%s,"escapeDenominator":%s}\n' \
      "$compact" "$full" "$escapes" "$compact"
  else
    printf 'micro-fix outcomes (%s)\n' "$log_file"
    printf '  compact route:  %s\n' "$compact"
    printf '  full route:     %s\n' "$full"
    printf '  defect escapes: %s of %s compact-route bugs\n' "$escapes" "$compact"
    if [[ "$compact" -eq 0 ]]; then
      printf '  no compact-route bug has closed yet; the escape rate has no denominator and must not be quoted\n'
    fi
  fi
  return 0
}

if [[ "$MICRO_FIX_LOG_IS_SOURCED" -eq 0 ]]; then
  case "${1:-}" in
    route)
      shift
      bubbles_micro_fix_log_route "$@"
      ;;
    escape)
      shift
      bubbles_micro_fix_log_escape "$@"
      ;;
    report)
      shift
      bubbles_micro_fix_log_report "$@"
      ;;
    -h | --help)
      sed -n '4,52p' "${BASH_SOURCE[0]}"
      ;;
    *)
      printf 'usage: micro-fix-outcome-log.sh {route|escape|report} [options]\n' >&2
      exit 2
      ;;
  esac
fi
