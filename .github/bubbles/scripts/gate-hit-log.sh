#!/usr/bin/env bash
# bubbles/scripts/gate-hit-log.sh
#
# Append-only gate-hit telemetry (IMP-036 SCOPE-4).
#
# WHY THIS EXISTS
# The framework declared 134 gate ids and had ZERO telemetry on which ones ever
# reject anything. That made gate retirement an opinion rather than a finding:
# nobody could say whether a given gate had ever changed an outcome. This script
# records one line per gate per evaluation so that question becomes answerable.
#
# It DOES NOT retire, disable, or weaken any gate. It only observes. The
# retirement decision is deliberately deferred until the log has accumulated
# enough evidence to support it.
#
# CONTRACT
#   - Append-only JSONL at <repo-root>/.specify/runtime/gate-hits.jsonl.
#     Past lines are never rewritten. A correction is a new line.
#   - `append` is TELEMETRY and MUST NEVER fail its caller. Every failure path
#     is swallowed and returns 0. A guard must not start blocking commits
#     because a log directory was read-only.
#   - Opt out with BUBBLES_GATE_HIT_LOG=off. Opting out is silent and safe.
#   - The log lives under .specify/runtime/, which the framework gitignores, so
#     telemetry never enters a commit.
#
# Usage:
#   bash bubbles/scripts/gate-hit-log.sh append \
#     --repo-root <path> --spec <path> --mode <mode> --target-status <status> \
#     --verdict <PASS|FAIL> --exit-status <n> \
#     [--passed "G001 G002"] [--failed "G003"]
#
#   bash bubbles/scripts/gate-hit-log.sh report [--repo-root <path>] [--json]
#
# Exit codes:
#   0 = success (append always returns 0)
#   2 = usage error, or report could not read a log that was explicitly named

# Sourceable. A caller that sources this file gets bubbles_gate_hit_append
# directly and pays no subprocess per guard run. state-transition-guard.sh does
# exactly that, because an UNBOUNDED external call inside a guard with a
# documented hang history (BUG-001, see state-transition-guard-perf-selftest.sh)
# is a defect rather than a convenience. Every identifier below is namespaced so
# sourcing cannot clobber a caller's own usage()/main()/SCRIPT_DIR.
if [[ "${BASH_SOURCE[0]}" != "${0}" ]]; then
  GATE_HIT_LOG_IS_SOURCED=1
else
  GATE_HIT_LOG_IS_SOURCED=0
  set -uo pipefail
fi

GATE_HIT_LOG_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GATE_HIT_LOG_DEFAULT_ROOT="$(cd "$GATE_HIT_LOG_DIR/../.." && pwd)"
GATE_HIT_SCHEMA_VERSION="gate-hit/v1"

bubbles_gate_hit_usage() {
  cat <<'USAGE'
usage: gate-hit-log.sh append --repo-root R --spec S --mode M --target-status T
                              --verdict V --exit-status N
                              [--passed "G001 G002"] [--failed "G003"]
       gate-hit-log.sh report [--repo-root R] [--json]

append  records one JSONL line per gate id. Never fails the caller.
report  aggregates the log: hits, passes, fails and last-seen per gate.

Environment:
  BUBBLES_GATE_HIT_LOG=off   disable appending (silent, exit 0)
USAGE
}

bubbles_gate_hit_log_path() {
  printf '%s/.specify/runtime/gate-hits.jsonl' "$1"
}

# Minimal JSON string escaping. Paths and mode names are the only untrusted
# inputs and neither legitimately contains a control character.
bubbles_gate_hit_json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  printf '%s' "$s"
}

bubbles_gate_hit_append() {
  # Telemetry must never break a guard. Everything below is best-effort.
  [[ "${BUBBLES_GATE_HIT_LOG:-on}" == "off" ]] && return 0

  local repo_root="" spec="" mode="" target_status="" verdict="" exit_status=""
  local passed="" failed="" parent_expanded=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --repo-root) shift; repo_root="${1:-}" ;;
      --spec) shift; spec="${1:-}" ;;
      --mode) shift; mode="${1:-}" ;;
      --target-status) shift; target_status="${1:-}" ;;
      --verdict) shift; verdict="${1:-}" ;;
      --exit-status) shift; exit_status="${1:-}" ;;
      --passed) shift; passed="${1:-}" ;;
      --failed) shift; failed="${1:-}" ;;
      --parent-expanded) shift; parent_expanded="${1:-}" ;;
      *) return 0 ;;
    esac
    shift || true
  done

  [[ -n "$repo_root" ]] || repo_root="$GATE_HIT_LOG_DEFAULT_ROOT"
  # Store the spec path relative to the repo. An absolute path carries the
  # operator's home directory, which the PII scan rejects the moment anyone
  # pastes this log as evidence.
  case "$spec" in
    "$repo_root"/*) spec="${spec#"$repo_root"/}" ;;
    "$repo_root") spec="." ;;
  esac
  local log_file runtime_dir ts
  log_file="$(bubbles_gate_hit_log_path "$repo_root")"
  runtime_dir="$(dirname "$log_file")"
  mkdir -p "$runtime_dir" 2>/dev/null || return 0
  # Portable UTC timestamp: no GNU-only date flags (WSL + macOS).
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)" || return 0

  local gate outcome group
  for group in "pass:$passed" "fail:$failed"; do
    outcome="${group%%:*}"
    for gate in ${group#*:}; do
      [[ "$gate" =~ ^G[0-9][0-9][0-9]$ ]] || continue
      printf '{"schemaVersion":"%s","kind":"gate","ts":"%s","gate":"%s","outcome":"%s","spec":"%s","mode":"%s","targetStatus":"%s","guardVerdict":"%s","exitStatus":"%s"}\n' \
        "$GATE_HIT_SCHEMA_VERSION" "$ts" "$gate" "$outcome" \
        "$(bubbles_gate_hit_json_escape "$spec")" "$(bubbles_gate_hit_json_escape "$mode")" \
        "$(bubbles_gate_hit_json_escape "$target_status")" "$(bubbles_gate_hit_json_escape "$verdict")" \
        "$(bubbles_gate_hit_json_escape "$exit_status")" >>"$log_file" 2>/dev/null || return 0
    done
  done

  # IMP-036 SCOPE-2: one run-scoped record carrying the parent-expansion count.
  # Expansion is already gated by G022; this makes the RATE visible, which is the
  # only way to tell whether SCOPE-1's single-orchestrator rule actually moved it.
  if [[ "$parent_expanded" =~ ^[0-9]+$ ]]; then
    printf '{"schemaVersion":"%s","kind":"run","ts":"%s","spec":"%s","mode":"%s","parentExpanded":%s,"guardVerdict":"%s"}\n' \
      "$GATE_HIT_SCHEMA_VERSION" "$ts" \
      "$(bubbles_gate_hit_json_escape "$spec")" "$(bubbles_gate_hit_json_escape "$mode")" \
      "$parent_expanded" "$(bubbles_gate_hit_json_escape "$verdict")" >>"$log_file" 2>/dev/null || return 0
  fi
  return 0
}

bubbles_gate_hit_report() {
  local repo_root="$GATE_HIT_LOG_DEFAULT_ROOT" as_json="false"
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --repo-root) shift; repo_root="${1:-}" ;;
      --json) as_json="true" ;;
      -h|--help) bubbles_gate_hit_usage; return 0 ;;
      *) bubbles_gate_hit_usage >&2; return 2 ;;
    esac
    shift || true
  done

  local log_file
  log_file="$(bubbles_gate_hit_log_path "$repo_root")"
  if [[ ! -f "$log_file" ]]; then
    if [[ "$as_json" == "true" ]]; then
      printf '{"schemaVersion":"%s","logPresent":false,"gates":[]}\n' "$GATE_HIT_SCHEMA_VERSION"
    else
      printf '[gate-hit-log] no telemetry yet at %s\n' "$log_file"
      printf '[gate-hit-log] this is expected until the guard has run at least once.\n'
    fi
    return 0
  fi

  awk -v as_json="$as_json" -v schema="$GATE_HIT_SCHEMA_VERSION" '
    {
      gate=""; outcome=""; ts="";
      if (match($0, /"gate":"[^"]*"/))    { gate    = substr($0, RSTART+8,  RLENGTH-9) }
      if (match($0, /"outcome":"[^"]*"/)) { outcome = substr($0, RSTART+11, RLENGTH-12) }
      if (match($0, /"ts":"[^"]*"/))      { ts      = substr($0, RSTART+6,  RLENGTH-7) }
      if (match($0, /"parentExpanded":[0-9]+/)) { pe = substr($0, RSTART+17, RLENGTH-17); runs++; pe_total += pe; if (pe+0 > 0) runs_expanded++ }
      if (gate == "") next
      hits[gate]++
      if (outcome == "fail") fails[gate]++; else passes[gate]++
      if (ts > last[gate]) last[gate] = ts
      total++
    }
    END {
      if (as_json == "true") {
        printf "{\"schemaVersion\":\"%s\",\"logPresent\":true,\"totalRecords\":%d,\"gates\":[", schema, total+0
        first=1
        for (g in hits) {
          if (!first) printf ","
          first=0
          printf "{\"gate\":\"%s\",\"hits\":%d,\"passes\":%d,\"fails\":%d,\"lastSeen\":\"%s\"}", g, hits[g], passes[g]+0, fails[g]+0, last[g]
        }
        printf "]}\n"
      } else {
        printf "=== gate-hit report (%d records) ===\n", total+0
        printf "  %-8s %8s %8s %8s  %s\n", "gate", "hits", "passes", "fails", "lastSeen"
        n=0
        for (g in hits) { n++; order[n]=g }
        for (i=1; i<n; i++) for (j=1; j<=n-i; j++) if (order[j] > order[j+1]) { t=order[j]; order[j]=order[j+1]; order[j+1]=t }
        never=0
        for (i=1; i<=n; i++) {
          g=order[i]
          printf "  %-8s %8d %8d %8d  %s\n", g, hits[g], passes[g]+0, fails[g]+0, last[g]
          if (fails[g]+0 == 0) never++
        }
        printf "  ---\n"
        printf "  gates observed: %d\n", n
        printf "  gates that have NEVER rejected anything: %d\n", never
        printf "  A gate with zero rejections is a retirement CANDIDATE, not a decision.\n"
        if (runs+0 > 0) {
          printf "  ---\n"
          printf "  guard runs recorded: %d\n", runs+0
          printf "  runs using parent-expansion: %d (%.1f%%)\n", runs_expanded+0, 100*(runs_expanded+0)/runs
          printf "  phases parent-expanded in total: %d\n", pe_total+0
          printf "  Expansion is legal under G022 but means a specialist did not run.\n"
        }
      }
    }
  ' "$log_file"
}

bubbles_gate_hit_main() {
  local sub="${1:-}"
  [[ $# -gt 0 ]] && shift
  case "$sub" in
    append) bubbles_gate_hit_append "$@" ;;
    report) bubbles_gate_hit_report "$@" ;;
    -h|--help|"") bubbles_gate_hit_usage; [[ -z "$sub" ]] && return 2 || return 0 ;;
    *) bubbles_gate_hit_usage >&2; return 2 ;;
  esac
}

[[ "$GATE_HIT_LOG_IS_SOURCED" -eq 1 ]] || bubbles_gate_hit_main "$@"
