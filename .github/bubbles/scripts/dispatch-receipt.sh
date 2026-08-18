#!/usr/bin/env bash
# dispatch-receipt.sh — executable dispatch-failure checkpointing (IMP-048 SCOPE-2, HO-4).
#
# Owner: bubbles.workflow
#
# WHY THIS EXISTS
# `workflow-orchestration-core.md` -> "Dispatch-Failure Checkpointing
# (HOST-101)" already says the right thing: a dispatch that never returned a
# result must never be counted as a passed phase. Nothing enforced it. The
# envelope validator scans `agents/*.agent.md` for AUTHORED envelope blocks and
# never inspects an actual `runSubagent` response, and `retryPolicy` in
# `bubbles/workflows.yaml` keys retries on error context, so a transport
# termination and a genuine test failure consumed the same budget and were
# treated the same way. A rule with no mechanism is a claim about behaviour,
# not a behaviour.
#
# This script is that mechanism. The parent records a receipt for EVERY
# dispatch, independent of what the child returned, and the receipt decides
# whether the phase may advance.
#
# WHAT IT GUARANTEES
#   earned receipts   A receipt is derived from the ACTUAL packet bytes that
#                     were dispatched (`--packet-file`). A digest that is merely
#                     ASSERTED, absent, or contradicted by those bytes is
#                     REFUSED. This mirrors phase-coordinator.sh: an occurrence
#                     is resolved by running it, never by asserting it.
#   no false advance  A phase advances on exactly ONE class -- a valid envelope
#                     reporting success. Every other class exits non-zero.
#   preserved identity A transport retry reuses the SAME occurrence id and the
#                     SAME packet digest. A retry is another ATTEMPT at one
#                     occurrence, never a new occurrence, because a new id is
#                     how a resultless dispatch disappears from the ledger.
#   per-class budgets Retry budgets are keyed on the FAILURE CLASS, so an
#                     infrastructure fault cannot consume the budget reserved
#                     for genuine defect remediation.
#   honest escalation A second identical transport failure is recorded as
#                     REPEAT_INFRASTRUCTURE and STOPS, rather than looping.
#
# CLOSED CLASS SET (7 -- one success class plus the six failure classes of
# IMP-048 SCOPE-2). It is closed on purpose: a seventh failure class would let
# an unclassified outcome pick its own handling.
#
#   Class                  Action                             Retry budget class
#   ENVELOPE_OK            advance                            (none)
#   TRANSPORT_TERMINATED   retry-identical-once               infrastructure
#   NO_RESULT              retry-identical-once               infrastructure
#   NARRATIVE_ONLY         envelope-only-recovery             envelope-recovery
#   ENVELOPE_FAILURE       route-to-fix-loop                  defect-remediation
#   TIMEOUT                resume-at-unresolved-leaf          infrastructure
#   REPEAT_INFRASTRUCTURE  stop-typed-infrastructure-blocked  infrastructure
#
# NARRATIVE_ONLY recovers the ENVELOPE from durable evidence; it does NOT
# re-run tests. TIMEOUT resumes at the unresolved leaf; it does NOT re-run the
# phase. ENVELOPE_FAILURE is a real result and routes to the fix loop; it is
# NOT a dispatch retry.
#
# DEFAULT OFF, per repo. With no `dispatchReceipts:` block, no config file, or
# an explicit `adapter: none`, every subcommand is a clean no-op that writes
# ZERO records and exits 0 -- an unconfigured repository behaves exactly as it
# does today. Same config shape as `mutationExecution:` and `experienceRecall:`.
#
# Store: append-only JSONL at <repo-root>/.specify/runtime/dispatch-receipts.jsonl,
# schemaVersion `dispatch-receipt/v1`, one object per ATTEMPT. Past lines are
# never rewritten; a correction is a new line.
#
# Usage:
#   dispatch-receipt.sh resolve [--repo-root PATH] [--names-only]
#   dispatch-receipt.sh record  --occurrence-id ID --agent NAME
#                               --packet-file PATH --dispatch-status CLASS
#                               [--packet-digest SHA] [--result-envelope-status S]
#                               [--started-at TS] [--finished-at TS]
#                               [--evidence-ref REF]... [--repo-root PATH]
#   dispatch-receipt.sh status  [--occurrence-id ID] [--repo-root PATH]
#
# Project config (project-owned, never framework-managed):
#
#   dispatchReceipts:
#     adapter: none | jsonl
#
# Exit codes (record):
#   0  receipt recorded and the phase MAY advance (ENVELOPE_OK only), or the
#      adapter is `none` so nothing is enforced
#   1  receipt recorded and the phase MUST NOT advance; work remains
#   2  usage error, or a receipt that was not earned (unverifiable packet)
#   3  terminal infrastructure stop (REPEAT_INFRASTRUCTURE)
#
# Exit codes (resolve/status): 0 ok - 1 configured-but-broken adapter - 2 usage
#
# There is no --skip, --force, --ignore, --replay or --assume flag.

set -euo pipefail

NAME="dispatch-receipt"
SCHEMA_VERSION="dispatch-receipt/v1"
STORE_REL=".specify/runtime/dispatch-receipts.jsonl"

# Per-class retry budgets. Separate counters are the whole point: an
# infrastructure fault must not spend the allowance kept for real remediation.
# The defect budget matches `retryPolicy.maxIdenticalFailures: 2` in
# bubbles/workflows.yaml so the two surfaces cannot disagree.
BUDGET_INFRASTRUCTURE=1
BUDGET_ENVELOPE_RECOVERY=1
BUDGET_DEFECT_REMEDIATION=2

usage() {
  cat <<'EOF'
Usage:
  dispatch-receipt.sh resolve [--repo-root PATH] [--names-only]
  dispatch-receipt.sh record  --occurrence-id ID --agent NAME --packet-file PATH
                              --dispatch-status CLASS [--packet-digest SHA]
                              [--result-envelope-status STATUS]
                              [--started-at TS] [--finished-at TS]
                              [--evidence-ref REF]... [--repo-root PATH]
  dispatch-receipt.sh status  [--occurrence-id ID] [--repo-root PATH]

Dispatch classes (closed set):
  ENVELOPE_OK            a valid envelope reporting success -> phase may advance
  TRANSPORT_TERMINATED   host ended the call before a result
  NO_RESULT              the dispatch returned empty
  NARRATIVE_ONLY         prose without a valid envelope
  ENVELOPE_FAILURE       a valid envelope reporting failure
  TIMEOUT                the command budget was exceeded
  REPEAT_INFRASTRUCTURE  a second identical transport failure (derived, not asserted)

A receipt is EARNED: --packet-file must name the packet bytes that were
actually dispatched, and a supplied --packet-digest must match them.

Project config (default OFF):

  dispatchReceipts:
    adapter: none | jsonl
EOF
}

fail() {
  printf '%s: %s\n' "$NAME" "$1" >&2
  exit "${2:-1}"
}

die_usage() {
  printf '%s: %s\n' "$NAME" "$1" >&2
  usage >&2
  exit 2
}

# sha256 over a file. macOS ships `shasum`, GNU ships `sha256sum`; neither is
# guaranteed, so both are probed and absence is loud rather than degrading to an
# unverified digest.
sha256_file() {
  if command -v sha256sum > /dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum > /dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    fail "no sha256 tool (sha256sum/shasum) available to earn a receipt" 2
  fi
}

# Minimal JSON string escaping. Ids, agent names and evidence refs are the only
# untrusted inputs and none legitimately carries a control character.
json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  printf '%s' "$s"
}

valid_class() {
  case "$1" in
    ENVELOPE_OK | TRANSPORT_TERMINATED | NO_RESULT | NARRATIVE_ONLY | ENVELOPE_FAILURE | TIMEOUT | REPEAT_INFRASTRUCTURE) return 0 ;;
    *) return 1 ;;
  esac
}

class_action() {
  case "$1" in
    ENVELOPE_OK) printf 'advance' ;;
    TRANSPORT_TERMINATED | NO_RESULT) printf 'retry-identical-once' ;;
    NARRATIVE_ONLY) printf 'envelope-only-recovery' ;;
    ENVELOPE_FAILURE) printf 'route-to-fix-loop' ;;
    TIMEOUT) printf 'resume-at-unresolved-leaf' ;;
    REPEAT_INFRASTRUCTURE) printf 'stop-typed-infrastructure-blocked' ;;
  esac
}

class_retry_class() {
  case "$1" in
    ENVELOPE_OK) printf 'none' ;;
    NARRATIVE_ONLY) printf 'envelope-recovery' ;;
    ENVELOPE_FAILURE) printf 'defect-remediation' ;;
    *) printf 'infrastructure' ;;
  esac
}

# Exactly one class advances a phase. Everything else -- including a valid
# envelope that REPORTS failure -- leaves the phase where it was.
class_advance() {
  case "$1" in
    ENVELOPE_OK) printf 'permitted' ;;
    *) printf 'refused' ;;
  esac
}

# A success-shaped envelope outcome, using the vocabulary of
# agents/bubbles_shared/evidence-rules.md. `blocked` and `route_required` are
# valid envelopes but are NOT successes, so they can never be ENVELOPE_OK.
success_envelope_status() {
  case "$1" in
    completed | completed_owned | completed_diagnostic) return 0 ;;
    *) return 1 ;;
  esac
}

resolve_adapter() {
  local repo_root="$1" config_file='' adapter=''
  if [ -f "$repo_root/.github/bubbles-project.yaml" ]; then
    config_file="$repo_root/.github/bubbles-project.yaml"
  elif [ -f "$repo_root/bubbles-project.yaml" ]; then
    config_file="$repo_root/bubbles-project.yaml"
  fi

  if [ -n "$config_file" ]; then
    adapter="$(awk '
      /^[[:space:]]*#/ { next }
      /^dispatchReceipts:[[:space:]]*$/ { inblock = 1; next }
      inblock && /^[^[:space:]]/ { inblock = 0 }
      inblock && $1 == "adapter:" {
        value = $2
        gsub(/["\047]/, "", value)
        print value
        exit
      }
    ' "$config_file" 2> /dev/null || true)"
  fi

  [ -n "$adapter" ] || adapter='none'

  case "$adapter" in
    *[!a-z0-9-]* | '' | -*)
      fail "invalid dispatchReceipts.adapter '$adapter' (expected none or jsonl)"
      ;;
  esac

  # A configured-but-unknown adapter fails LOUD instead of degrading to `none`.
  # A typo that silently produced "unenforced" would be indistinguishable from a
  # deliberate opt-out, and a resultless dispatch would then advance a phase on
  # the strength of a misspelling.
  case "$adapter" in
    none | jsonl) ;;
    *) fail "unknown dispatchReceipts.adapter '$adapter' (expected none or jsonl)" ;;
  esac

  printf '%s' "$adapter"
}

# One pass over the store answering every question the decision needs:
# how many attempts this occurrence already has, how many prior receipts carry
# the SAME class and the SAME packet digest (the escalation trigger), what each
# per-class budget has consumed, and what the most recent class was.
scan_store() {
  local store="$1" occ="$2" cls="$3" dig="$4"
  if [ ! -f "$store" ]; then
    printf 'attempts=0 same=0 infra=0 recovery=0 defect=0 last=\n'
    return 0
  fi
  awk -v OCC="$occ" -v CLS="$cls" -v DIG="$dig" '
    {
      occ=""; cls=""; dig=""; rc="";
      if (match($0, /"occurrenceId":"[^"]*"/)) occ = substr($0, RSTART+16, RLENGTH-17);
      if (OCC != "" && occ != OCC) next;
      if (match($0, /"dispatchStatus":"[^"]*"/)) cls = substr($0, RSTART+18, RLENGTH-19);
      if (match($0, /"packetDigest":"[^"]*"/))   dig = substr($0, RSTART+16, RLENGTH-17);
      if (match($0, /"retryClass":"[^"]*"/))     rc  = substr($0, RSTART+14, RLENGTH-15);
      attempts++;
      last = cls;
      if (CLS != "" && cls == CLS && dig == DIG) same++;
      if (rc == "infrastructure")      infra++;
      else if (rc == "envelope-recovery")  recovery++;
      else if (rc == "defect-remediation") defect++;
    }
    END {
      printf "attempts=%d same=%d infra=%d recovery=%d defect=%d last=%s\n",
        attempts+0, same+0, infra+0, recovery+0, defect+0, last
    }
  ' "$store"
}

budget_limit() {
  case "$1" in
    infrastructure) printf '%s' "$BUDGET_INFRASTRUCTURE" ;;
    envelope-recovery) printf '%s' "$BUDGET_ENVELOPE_RECOVERY" ;;
    defect-remediation) printf '%s' "$BUDGET_DEFECT_REMEDIATION" ;;
    *) printf '0' ;;
  esac
}

cmd_resolve() {
  local repo_root="$PWD" names_only=0
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        ;;
      --names-only)
        names_only=1
        shift
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      *) die_usage "unknown option: $1" ;;
    esac
  done
  [ -d "$repo_root" ] || die_usage "repo root not found: $repo_root"
  repo_root="$(cd "$repo_root" && pwd)"

  local adapter
  adapter="$(resolve_adapter "$repo_root")"
  printf 'adapter=%s\n' "$adapter"
  if [ "$names_only" = "1" ]; then
    return 0
  fi
  printf 'schemaVersion=%s\n' "$SCHEMA_VERSION"
  printf 'store=%s/%s\n' "$repo_root" "$STORE_REL"
  printf 'repoRoot=%s\n' "$repo_root"
  return 0
}

cmd_record() {
  local repo_root="$PWD" occurrence_id='' agent='' packet_file='' packet_digest=''
  local dispatch_status='' envelope_status='' started_at='' finished_at=''
  local evidence_refs=''

  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        ;;
      --occurrence-id)
        [ "$#" -ge 2 ] || die_usage "--occurrence-id requires a value"
        occurrence_id="$2"
        shift 2
        ;;
      --agent)
        [ "$#" -ge 2 ] || die_usage "--agent requires a value"
        agent="$2"
        shift 2
        ;;
      --packet-file)
        [ "$#" -ge 2 ] || die_usage "--packet-file requires a value"
        packet_file="$2"
        shift 2
        ;;
      --packet-digest)
        [ "$#" -ge 2 ] || die_usage "--packet-digest requires a value"
        packet_digest="$2"
        shift 2
        ;;
      --dispatch-status)
        [ "$#" -ge 2 ] || die_usage "--dispatch-status requires a value"
        dispatch_status="$2"
        shift 2
        ;;
      --result-envelope-status)
        [ "$#" -ge 2 ] || die_usage "--result-envelope-status requires a value"
        envelope_status="$2"
        shift 2
        ;;
      --started-at)
        [ "$#" -ge 2 ] || die_usage "--started-at requires a value"
        started_at="$2"
        shift 2
        ;;
      --finished-at)
        [ "$#" -ge 2 ] || die_usage "--finished-at requires a value"
        finished_at="$2"
        shift 2
        ;;
      --evidence-ref)
        [ "$#" -ge 2 ] || die_usage "--evidence-ref requires a value"
        evidence_refs="${evidence_refs}${evidence_refs:+$'\n'}$2"
        shift 2
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      --skip* | --force* | --ignore* | --replay* | --assume*)
        printf '%s: "%s" does not exist. A receipt is earned by dispatching, never by asserting it.\n' "$NAME" "$1" >&2
        exit 2
        ;;
      *) die_usage "unknown option: $1" ;;
    esac
  done

  [ -d "$repo_root" ] || die_usage "repo root not found: $repo_root"
  repo_root="$(cd "$repo_root" && pwd)"

  local adapter
  adapter="$(resolve_adapter "$repo_root")"
  if [ "$adapter" = "none" ]; then
    # DEFAULT OFF. Nothing recorded, nothing enforced, exit 0. An unconfigured
    # repository behaves exactly as it does today.
    printf 'adapter=none\n'
    printf 'receipt=skipped\n'
    return 0
  fi

  [ -n "$occurrence_id" ] || die_usage "--occurrence-id is required"
  [ -n "$agent" ] || die_usage "--agent is required"
  [ -n "$dispatch_status" ] || die_usage "--dispatch-status is required"
  valid_class "$dispatch_status" || die_usage "unknown --dispatch-status '$dispatch_status'"

  # REPEAT_INFRASTRUCTURE is DERIVED from a second identical transport failure,
  # never declared. Letting a caller assert it would let a first failure be
  # renamed into a terminal stop, and letting one be asserted without a prior
  # failure would manufacture the escalation.
  if [ "$dispatch_status" = "REPEAT_INFRASTRUCTURE" ]; then
    fail "REPEAT_INFRASTRUCTURE is derived from a second identical transport failure; it cannot be asserted" 2
  fi

  # A RECEIPT MUST BE EARNED. The digest comes from the bytes that were actually
  # dispatched. A missing packet file, or a digest that contradicts those bytes,
  # is a receipt asserted rather than run, and it is refused.
  [ -n "$packet_file" ] || fail "--packet-file is required: a receipt is derived from the dispatched packet, not asserted" 2
  [ -f "$packet_file" ] || fail "--packet-file not found: $packet_file (a receipt cannot be earned without the dispatched packet)" 2
  local computed_digest
  computed_digest="$(sha256_file "$packet_file")"
  [ -n "$computed_digest" ] || fail "could not compute a digest for $packet_file" 2
  if [ -n "$packet_digest" ] && [ "$packet_digest" != "$computed_digest" ]; then
    fail "--packet-digest does not match the dispatched packet bytes (asserted $packet_digest, computed $computed_digest)" 2
  fi
  packet_digest="$computed_digest"

  # An advance is earned by an envelope, so ENVELOPE_OK must name a
  # success-shaped envelope outcome. A blocked or route_required envelope is a
  # real result but not a success, and it belongs to ENVELOPE_FAILURE.
  if [ "$dispatch_status" = "ENVELOPE_OK" ]; then
    [ -n "$envelope_status" ] || fail "ENVELOPE_OK requires --result-envelope-status; an advance is earned by an envelope" 2
    success_envelope_status "$envelope_status" ||
      fail "ENVELOPE_OK requires a success outcome (completed, completed_owned, completed_diagnostic); got '$envelope_status'" 2
  fi
  if [ "$dispatch_status" = "ENVELOPE_FAILURE" ]; then
    [ -n "$envelope_status" ] || fail "ENVELOPE_FAILURE requires --result-envelope-status naming the failing outcome" 2
    if success_envelope_status "$envelope_status"; then
      fail "ENVELOPE_FAILURE cannot carry a success outcome '$envelope_status'" 2
    fi
  fi

  local store runtime_dir now
  store="$repo_root/$STORE_REL"
  runtime_dir="$(dirname "$store")"
  mkdir -p "$runtime_dir" || fail "cannot create the receipt store directory $runtime_dir"
  # Portable UTC timestamp: no GNU-only date flags (WSL + macOS).
  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  [ -n "$finished_at" ] || finished_at="$now"
  [ -n "$started_at" ] || started_at="$now"

  local scan attempts same infra recovery defect
  scan="$(scan_store "$store" "$occurrence_id" "$dispatch_status" "$packet_digest")"
  # Read the scan with a herestring, never `... | grep -q` on unbounded input:
  # a discarding pipe under `pipefail` is the BUG-009 SIGPIPE race.
  attempts="$(awk '{for(i=1;i<=NF;i++){split($i,kv,"=");if(kv[1]=="attempts")print kv[2]}}' <<< "$scan")"
  same="$(awk '{for(i=1;i<=NF;i++){split($i,kv,"=");if(kv[1]=="same")print kv[2]}}' <<< "$scan")"
  infra="$(awk '{for(i=1;i<=NF;i++){split($i,kv,"=");if(kv[1]=="infra")print kv[2]}}' <<< "$scan")"
  recovery="$(awk '{for(i=1;i<=NF;i++){split($i,kv,"=");if(kv[1]=="recovery")print kv[2]}}' <<< "$scan")"
  defect="$(awk '{for(i=1;i<=NF;i++){split($i,kv,"=");if(kv[1]=="defect")print kv[2]}}' <<< "$scan")"

  # ESCALATION. A second transport-class failure on the SAME occurrence with the
  # SAME packet is not another retry -- retrying identical bytes that already
  # failed identically is how a loop is mistaken for progress.
  local effective_class="$dispatch_status" escalated_from=''
  case "$dispatch_status" in
    TRANSPORT_TERMINATED | NO_RESULT)
      if [ "$same" -ge 1 ]; then
        effective_class="REPEAT_INFRASTRUCTURE"
        escalated_from="$dispatch_status"
      fi
      ;;
  esac

  local action advance retry_class attempt_id
  action="$(class_action "$effective_class")"
  advance="$(class_advance "$effective_class")"
  retry_class="$(class_retry_class "$effective_class")"
  attempt_id=$((attempts + 1))

  # Budget accounting AFTER this receipt, per class and only for this class.
  case "$retry_class" in
    infrastructure) infra=$((infra + 1)) ;;
    envelope-recovery) recovery=$((recovery + 1)) ;;
    defect-remediation) defect=$((defect + 1)) ;;
  esac

  local consumed limit exhausted='false'
  case "$retry_class" in
    none)
      consumed=0
      limit=0
      ;;
    infrastructure)
      consumed="$infra"
      limit="$BUDGET_INFRASTRUCTURE"
      ;;
    envelope-recovery)
      consumed="$recovery"
      limit="$BUDGET_ENVELOPE_RECOVERY"
      ;;
    defect-remediation)
      consumed="$defect"
      limit="$BUDGET_DEFECT_REMEDIATION"
      ;;
  esac
  if [ "$retry_class" != "none" ] && [ "$consumed" -gt "$limit" ]; then
    exhausted='true'
  fi

  local terminal='false'
  if [ "$effective_class" = "REPEAT_INFRASTRUCTURE" ]; then
    terminal='true'
  fi

  local refs_json='' ref
  while IFS= read -r ref; do
    [ -n "$ref" ] || continue
    refs_json="${refs_json}${refs_json:+,}\"$(json_escape "$ref")\""
  done <<< "$evidence_refs"

  local escalated_json='null'
  if [ -n "$escalated_from" ]; then
    escalated_json="\"$(json_escape "$escalated_from")\""
  fi

  printf '{"schemaVersion":"%s","occurrenceId":"%s","attemptId":%d,"packetDigest":"%s","agent":"%s","startedAt":"%s","finishedAt":"%s","dispatchStatus":"%s","resultEnvelopeStatus":"%s","evidenceRefs":[%s],"escalatedFrom":%s,"retryClass":"%s","action":"%s","advance":"%s","terminal":%s}\n' \
    "$SCHEMA_VERSION" \
    "$(json_escape "$occurrence_id")" \
    "$attempt_id" \
    "$(json_escape "$packet_digest")" \
    "$(json_escape "$agent")" \
    "$(json_escape "$started_at")" \
    "$(json_escape "$finished_at")" \
    "$effective_class" \
    "$(json_escape "$envelope_status")" \
    "$refs_json" \
    "$escalated_json" \
    "$retry_class" \
    "$action" \
    "$advance" \
    "$terminal" \
    >> "$store" || fail "cannot append to the receipt store $store"

  printf 'adapter=jsonl\n'
  printf 'receipt=recorded\n'
  printf 'occurrenceId=%s\n' "$occurrence_id"
  printf 'attemptId=%s\n' "$attempt_id"
  printf 'packetDigest=%s\n' "$packet_digest"
  printf 'agent=%s\n' "$agent"
  printf 'dispatchStatus=%s\n' "$effective_class"
  printf 'escalatedFrom=%s\n' "${escalated_from:-none}"
  printf 'resultEnvelopeStatus=%s\n' "${envelope_status:-none}"
  printf 'action=%s\n' "$action"
  printf 'advance=%s\n' "$advance"
  printf 'retryClass=%s\n' "$retry_class"
  # A retry re-dispatches the SAME occurrence and the SAME packet. Printing both
  # makes the identity contract checkable by the caller rather than assumed.
  printf 'retryOccurrenceId=%s\n' "$occurrence_id"
  printf 'retryPacketDigest=%s\n' "$packet_digest"
  printf 'budget.infrastructure=%s/%s\n' "$infra" "$BUDGET_INFRASTRUCTURE"
  printf 'budget.envelope-recovery=%s/%s\n' "$recovery" "$BUDGET_ENVELOPE_RECOVERY"
  printf 'budget.defect-remediation=%s/%s\n' "$defect" "$BUDGET_DEFECT_REMEDIATION"
  printf 'budgetExhausted=%s\n' "$exhausted"
  printf 'terminal=%s\n' "$terminal"
  printf 'store=%s\n' "$store"

  if [ "$terminal" = "true" ]; then
    return 3
  fi
  if [ "$advance" = "permitted" ]; then
    return 0
  fi
  return 1
}

cmd_status() {
  local repo_root="$PWD" occurrence_id=''
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --repo-root)
        [ "$#" -ge 2 ] || die_usage "--repo-root requires a value"
        repo_root="$2"
        shift 2
        ;;
      --occurrence-id)
        [ "$#" -ge 2 ] || die_usage "--occurrence-id requires a value"
        occurrence_id="$2"
        shift 2
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      *) die_usage "unknown option: $1" ;;
    esac
  done
  [ -d "$repo_root" ] || die_usage "repo root not found: $repo_root"
  repo_root="$(cd "$repo_root" && pwd)"

  local adapter store
  adapter="$(resolve_adapter "$repo_root")"
  store="$repo_root/$STORE_REL"
  printf 'adapter=%s\n' "$adapter"
  printf 'store=%s\n' "$store"
  if [ "$adapter" = "none" ]; then
    printf 'records=0\n'
    printf 'advancePermitted=false\n'
    return 0
  fi

  local scan attempts last infra recovery defect
  scan="$(scan_store "$store" "$occurrence_id" '' '')"
  attempts="$(awk '{for(i=1;i<=NF;i++){split($i,kv,"=");if(kv[1]=="attempts")print kv[2]}}' <<< "$scan")"
  infra="$(awk '{for(i=1;i<=NF;i++){split($i,kv,"=");if(kv[1]=="infra")print kv[2]}}' <<< "$scan")"
  recovery="$(awk '{for(i=1;i<=NF;i++){split($i,kv,"=");if(kv[1]=="recovery")print kv[2]}}' <<< "$scan")"
  defect="$(awk '{for(i=1;i<=NF;i++){split($i,kv,"=");if(kv[1]=="defect")print kv[2]}}' <<< "$scan")"
  last="$(awk '{for(i=1;i<=NF;i++){split($i,kv,"=");if(kv[1]=="last")print kv[2]}}' <<< "$scan")"

  # Advancement is a property of the MOST RECENT receipt. A phase that once had
  # a passing envelope and then a resultless retry has not advanced.
  local advance_permitted='false'
  if [ "$last" = "ENVELOPE_OK" ]; then
    advance_permitted='true'
  fi

  printf 'occurrenceId=%s\n' "${occurrence_id:-<all>}"
  printf 'records=%s\n' "$attempts"
  printf 'attempts=%s\n' "$attempts"
  printf 'lastClass=%s\n' "${last:-none}"
  printf 'advancePermitted=%s\n' "$advance_permitted"
  local cls consumed limit
  for cls in infrastructure envelope-recovery defect-remediation; do
    case "$cls" in
      infrastructure) consumed="$infra" ;;
      envelope-recovery) consumed="$recovery" ;;
      defect-remediation) consumed="$defect" ;;
    esac
    limit="$(budget_limit "$cls")"
    printf 'budget.%s.consumed=%s\n' "$cls" "$consumed"
    printf 'budget.%s.limit=%s\n' "$cls" "$limit"
    if [ "$consumed" -gt "$limit" ]; then
      printf 'budget.%s.exhausted=true\n' "$cls"
    else
      printf 'budget.%s.exhausted=false\n' "$cls"
    fi
  done
  return 0
}

main() {
  local sub="${1:-}"
  if [ "$#" -gt 0 ]; then
    shift
  fi
  case "$sub" in
    resolve) cmd_resolve "$@" ;;
    record) cmd_record "$@" ;;
    status) cmd_status "$@" ;;
    -h | --help)
      usage
      exit 0
      ;;
    '')
      usage >&2
      exit 2
      ;;
    *) die_usage "unknown subcommand: $sub" ;;
  esac
}

main "$@"
