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
#   - Every record carries a `sourceClass`. `report` counts product records only,
#     so a fixture run cannot be mistaken for evidence about a shipped gate
#     (IMP-042 SCOPE-17).
#   - Every gate record carries `fired` and `prevented` as SEPARATE facts
#     (IMP-047 S-A). `fired` says the gate was actually evaluated; `prevented`
#     says its refusal blocked a transition that would otherwise have proceeded.
#     A gate that fires and permits is not the same as a gate that never fired,
#     and only PREVENTION is a valid basis for retirement.
#
# Usage:
#   bash bubbles/scripts/gate-hit-log.sh append \
#     --repo-root <path> --spec <path> --mode <mode> --target-status <status> \
#     --verdict <PASS|FAIL> --exit-status <n> \
#     [--passed "G001 G002"] [--failed "G003"] [--not-evaluated "G004"]
#
#   bash bubbles/scripts/gate-hit-log.sh report [--repo-root <path>] [--json] \
#     [--class product|fixture|selftest|migration] [--all-classes]
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
GATE_HIT_VALID_CLASSES="product fixture selftest migration"

# Derive the record's source class. Returns exactly one of GATE_HIT_VALID_CLASSES
# and never fails, because every path in append is telemetry.
bubbles_gate_hit_source_class() {
  local repo_root="${1:-}" declared="${BUBBLES_GATE_HIT_SOURCE_CLASS:-}" candidate
  if [[ -n "$declared" ]]; then
    for candidate in $GATE_HIT_VALID_CLASSES; do
      [[ "$declared" == "$candidate" ]] && { printf '%s' "$declared"; return 0; }
    done
    # An unrecognised declaration is treated as a fixture rather than trusted as
    # product: a typo must not promote a test record into retirement evidence.
    printf '%s' 'fixture'
    return 0
  fi
  local resolved="$repo_root"
  case "$resolved" in
    "${TMPDIR:-/nonexistent-tmpdir}"*| /tmp/* | /private/tmp/* | /var/tmp/* | /private/var/folders/* | /var/folders/*)
      printf '%s' 'fixture'
      return 0
      ;;
  esac
  printf '%s' 'product'
}

bubbles_gate_hit_usage() {
  cat <<'USAGE'
usage: gate-hit-log.sh append --repo-root R --spec S --mode M --target-status T
                              --verdict V --exit-status N
                              [--passed "G001 G002"] [--failed "G003"]
                              [--not-evaluated "G004"]
       gate-hit-log.sh report [--repo-root R] [--json] [--class C] [--all-classes]

append  records one JSONL line per gate id. Never fails the caller.
        --passed        gates a check evaluated and permitted (fired)
        --failed        gates a check evaluated and refused (fired)
        --not-evaluated gates credited without being evaluated (did NOT fire)
report  aggregates the log: fired, prevented and last-seen per gate.
        Counts sourceClass=product only, so fixture and selftest runs cannot
        be mistaken for evidence that a gate is load-bearing in a product.
        --class C     count class C instead (product|fixture|selftest|migration)
        --all-classes count every record regardless of class

Environment:
  BUBBLES_GATE_HIT_LOG=off   disable appending (silent, exit 0)
  BUBBLES_GATE_HIT_SOURCE_CLASS=<class>
                             declare the record class explicitly. Unset means
                             derive it: a temp-dir repo root is a fixture.
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
  local passed="" failed="" not_evaluated="" parent_expanded=""
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
      --not-evaluated) shift; not_evaluated="${1:-}" ;;
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

  # IMP-042 SCOPE-17. A retirement decision reads "gate G0xx never rejected
  # anything". That is only true of PRODUCT runs. Selftests drive the guard
  # through fixture repositories on purpose, including deliberate rejections, so
  # counting those records makes a gate look busier -- or a fixture-only gate
  # look load-bearing -- on evidence that describes the test suite rather than
  # the product.
  #
  # The class is DERIVED from the repository root, not asked for, because a
  # fixture that forgets to declare itself is exactly the record that pollutes
  # the report. Every selftest here builds its fixture under a temp root, so
  # that location is the signal. The env override exists for a caller that
  # legitimately runs outside a temp dir (a migration replay, say), and is
  # validated so a typo cannot invent a fourth class that the report then
  # silently drops.
  local source_class
  source_class="$(bubbles_gate_hit_source_class "$repo_root")"

  # IMP-047 S-A. Firing and prevention are separate facts.
  #
  # `fired` answers "was this gate actually evaluated on this run?". The caller
  # supplies the answer because only the caller knows: state-transition-guard.sh
  # blanket-credits every REQUIRED gate id on a PASS verdict, so a `pass` record
  # alone never proved a check ran. Gates credited that way arrive on
  # --not-evaluated and are recorded as fired:false, which is what makes "fired
  # and permitted" distinguishable from "never fired" instead of both being an
  # indistinguishable `pass`.
  #
  # `prevented` answers "did this gate's refusal stop a transition that would
  # otherwise have proceeded?". It is DERIVED, never asked for, from two facts
  # already on the record: the gate itself refused, and the run did not proceed.
  # A gate that refused while the guard still exited 0 fired without preventing
  # anything, and recording it as a prevention would manufacture the exact
  # retirement evidence this store exists to make honest.
  local run_blocked="false"
  if [[ -n "$exit_status" && "$exit_status" != "0" ]]; then
    run_blocked="true"
  fi

  local gate outcome group fired prevented
  for group in "pass:$passed" "fail:$failed" "not-evaluated:$not_evaluated"; do
    outcome="${group%%:*}"
    case "$outcome" in
      fail) fired="true"; prevented="$run_blocked" ;;
      not-evaluated) fired="false"; prevented="false" ;;
      *) fired="true"; prevented="false" ;;
    esac
    for gate in ${group#*:}; do
      [[ "$gate" =~ ^G[0-9][0-9][0-9]$ ]] || continue
      printf '{"schemaVersion":"%s","kind":"gate","ts":"%s","sourceClass":"%s","gate":"%s","outcome":"%s","fired":%s,"prevented":%s,"spec":"%s","mode":"%s","targetStatus":"%s","guardVerdict":"%s","exitStatus":"%s"}\n' \
        "$GATE_HIT_SCHEMA_VERSION" "$ts" "$source_class" "$gate" "$outcome" \
        "$fired" "$prevented" \
        "$(bubbles_gate_hit_json_escape "$spec")" "$(bubbles_gate_hit_json_escape "$mode")" \
        "$(bubbles_gate_hit_json_escape "$target_status")" "$(bubbles_gate_hit_json_escape "$verdict")" \
        "$(bubbles_gate_hit_json_escape "$exit_status")" >>"$log_file" 2>/dev/null || return 0
    done
  done

  # IMP-036 SCOPE-2: one run-scoped record carrying the parent-expansion count.
  # Expansion is already gated by G022; this makes the RATE visible, which is the
  # only way to tell whether SCOPE-1's single-orchestrator rule actually moved it.
  if [[ "$parent_expanded" =~ ^[0-9]+$ ]]; then
    printf '{"schemaVersion":"%s","kind":"run","ts":"%s","sourceClass":"%s","spec":"%s","mode":"%s","parentExpanded":%s,"guardVerdict":"%s"}\n' \
      "$GATE_HIT_SCHEMA_VERSION" "$ts" "$source_class" \
      "$(bubbles_gate_hit_json_escape "$spec")" "$(bubbles_gate_hit_json_escape "$mode")" \
      "$parent_expanded" "$(bubbles_gate_hit_json_escape "$verdict")" >>"$log_file" 2>/dev/null || return 0
  fi
  return 0
}

bubbles_gate_hit_report() {
  local repo_root="$GATE_HIT_LOG_DEFAULT_ROOT" as_json="false" class_filter="product"
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --repo-root) shift; repo_root="${1:-}" ;;
      --json) as_json="true" ;;
      # Retirement reads product evidence only. These widen it deliberately.
      --all-classes) class_filter="" ;;
      --class) shift; class_filter="${1:-product}" ;;
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

  awk -v as_json="$as_json" -v schema="$GATE_HIT_SCHEMA_VERSION" -v want_class="$class_filter" '
    {
      gate=""; outcome=""; ts=""; sclass=""; firedf=""; preventedf=""; estatus="";
      if (match($0, /"sourceClass":"[^"]*"/)) { sclass = substr($0, RSTART+15, RLENGTH-16) }
      # Records written before source classing carry no field. Treat them as
      # product, which is what they were, rather than dropping history.
      if (sclass == "") sclass = "product"
      if (want_class != "" && sclass != want_class) { excluded++; next }
      if (match($0, /"gate":"[^"]*"/))    { gate    = substr($0, RSTART+8,  RLENGTH-9) }
      if (match($0, /"outcome":"[^"]*"/)) { outcome = substr($0, RSTART+11, RLENGTH-12) }
      if (match($0, /"ts":"[^"]*"/))      { ts      = substr($0, RSTART+6,  RLENGTH-7) }
      if (match($0, /"exitStatus":"[^"]*"/)) { estatus = substr($0, RSTART+14, RLENGTH-15) }
      if (match($0, /"fired":(true|false)/))     { firedf     = substr($0, RSTART+8,  RLENGTH-8) }
      if (match($0, /"prevented":(true|false)/)) { preventedf = substr($0, RSTART+12, RLENGTH-12) }
      if (match($0, /"parentExpanded":[0-9]+/)) { pe = substr($0, RSTART+17, RLENGTH-17); runs++; pe_total += pe; if (pe+0 > 0) runs_expanded++ }
      if (gate == "") next

      # IMP-047 S-A backward compatibility. Records written before firing and
      # prevention were separate facts carry neither field. They are NOT dropped
      # and NOT re-labelled: the two values are derived from what the old record
      # does carry -- a pass/fail record meant the gate was recorded on that run,
      # and a refusal only stopped the run when the run itself did not proceed --
      # and the derivation is counted so the report can say how much of its
      # evidence is legacy rather than directly observed.
      if (firedf == "") {
        legacy[gate]++
        legacy_total++
        firedf = "true"
        preventedf = (outcome == "fail" && estatus != "" && estatus != "0") ? "true" : "false"
      }

      hits[gate]++
      if (outcome == "fail") fails[gate]++
      else if (outcome == "pass") passes[gate]++
      if (firedf == "true") fired[gate]++; else notfired[gate]++
      if (preventedf == "true") prevented[gate]++
      if (ts > last[gate]) last[gate] = ts
      total++
    }
    END {
      if (as_json == "true") {
        printf "{\"schemaVersion\":\"%s\",\"logPresent\":true,\"sourceClass\":\"%s\",\"excludedRecords\":%d,\"totalRecords\":%d,\"legacyDerivedRecords\":%d,\"gates\":[", schema, (want_class == "" ? "all" : want_class), excluded+0, total+0, legacy_total+0
        first=1
        for (g in hits) {
          if (!first) printf ","
          first=0
          printf "{\"gate\":\"%s\",\"hits\":%d,\"passes\":%d,\"fails\":%d,\"fired\":%d,\"notFired\":%d,\"prevented\":%d,\"legacyDerived\":%d,\"lastSeen\":\"%s\"}", g, hits[g], passes[g]+0, fails[g]+0, fired[g]+0, notfired[g]+0, prevented[g]+0, legacy[g]+0, last[g]
        }
        printf "]}\n"
      } else {
        printf "=== gate-hit report (%d records, sourceClass=%s) ===\n", total+0, (want_class == "" ? "all" : want_class)
        if (excluded+0 > 0) {
          printf "  %d record(s) excluded as non-%s. Re-run with --all-classes to include them.\n", excluded+0, want_class
        }
        printf "  %-8s %8s %8s %8s %10s  %s\n", "gate", "records", "fired", "notFired", "prevented", "lastSeen"
        n=0
        for (g in hits) { n++; order[n]=g }
        for (i=1; i<n; i++) for (j=1; j<=n-i; j++) if (order[j] > order[j+1]) { t=order[j]; order[j]=order[j+1]; order[j+1]=t }
        fired_never_prevented=0
        never_fired=0
        ever_prevented=0
        for (i=1; i<=n; i++) {
          g=order[i]
          printf "  %-8s %8d %8d %8d %10d  %s\n", g, hits[g], fired[g]+0, notfired[g]+0, prevented[g]+0, last[g]
          if (prevented[g]+0 > 0) ever_prevented++
          else if (fired[g]+0 > 0) fired_never_prevented++
          if (fired[g]+0 == 0) never_fired++
        }
        printf "  ---\n"
        printf "  gates with any record: %d\n", n
        printf "  gates that PREVENTED at least once: %d\n", ever_prevented
        printf "  gates that FIRED but never prevented: %d\n", fired_never_prevented
        printf "  gates recorded only as credited-without-evaluation (NEVER FIRED): %d\n", never_fired
        printf "  Prevention is the only valid basis for retirement. A gate that never\n"
        printf "  FIRED has not been exercised, which is not evidence of uselessness; a\n"
        printf "  gate that fired and never prevented is a retirement CANDIDATE, not a\n"
        printf "  decision.\n"
        if (legacy_total+0 > 0) {
          printf "  ---\n"
          printf "  %d record(s) predate the fired/prevented fields; both values were\n", legacy_total+0
          printf "  derived from outcome and exitStatus rather than directly observed.\n"
        }
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
