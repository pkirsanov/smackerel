#!/usr/bin/env bash
# goal-fidelity-telemetry.sh  (IMP-038 SCOPE-7 / GF-5)
#
# Emits bounded framework telemetry for goal-fidelity events into the EXISTING
# ledger at .specify/runtime/framework-events.jsonl, using the same record shape
# cli.sh writes. One ledger, one format — a second telemetry file would need its
# own readers, rotation and retention, and would drift.
#
# "Telemetry must not store raw operator prompts" is enforced STRUCTURALLY, not
# by asking callers to be careful: there is no free-text field to put a prompt
# in. Every field is either a closed enum or an already-hashed identifier. The
# `sourceRequestDigest` is safe precisely because it is a SHA-256 of the request
# — it identifies a goal without reproducing what the operator typed. A caller
# that wants to record "why" records an enum value; if no enum fits, the right
# change is a new enum value in review, not a prose field.
#
# Events (closed set):
#   contract-frozen        a Goal Contract was frozen at revision 1
#   contract-revised       an operator-approved expansion incremented the revision
#   expansion-requested    a runner asked for reach it did not have
#   expansion-rejected     an expansion was refused (no approval note, or a widen)
#   boundary-refusal       work-boundary-resolve.sh refused or routed a candidate
#   finding-routed         a finding was handed to another owner
#   finding-goal-blocking  a finding blocked the parent (required/blocking-external)
#   phase-relevance        a phase run/skip verdict, attributed to its runner
#
# Usage:
#   goal-fidelity-telemetry.sh --event <type> [--repo-root <dir>] [fields...]
#
# Exit codes:
#   0  recorded (or silently skipped when telemetry is disabled)
#   2  usage error, including any attempt to pass a free-text field
#
# Disable with BUBBLES_TELEMETRY=0. Telemetry is observability, never a gate: a
# failure to write MUST NOT block delivery, so an unwritable ledger is reported
# on stderr and exits 0.

set -uo pipefail

usage() {
  cat <<'EOF'
Usage: goal-fidelity-telemetry.sh --event <type> [options]

Events (closed set):
  contract-frozen | contract-revised | expansion-requested | expansion-rejected
  boundary-refusal | finding-routed | finding-goal-blocking | phase-relevance

Options (all optional; each is a closed enum or a hashed identifier):
  --repo-root <dir>        Repository root (default: resolved from this script)
  --goal-id <id>           gc:<sessionId>:<revision>
  --revision <n>           Integer contract revision
  --digest <sha256:...>    sourceRequestDigest — a hash, never the prompt text
  --runner <agent>         bubbles.goal | bubbles.sprint | bubbles.iterate | bubbles.workflow
  --phase <name>           Phase name, for phase-relevance
  --verdict <run|skip>     Phase-relevance verdict
  --rule <token>           Registry rule token that decided a verdict
  --disposition <token>    in-boundary | route-same-repo | route-cross-repo | refuse-cross-repo
  --goal-impact <token>    required | blocking-external | independent
  --finding-id <id>        Finding identifier
  --outcome <token>        accepted | refused

There is deliberately NO --details / --note / --reason / --message field: a
free-text field is how an operator prompt would end up in the ledger.

Set BUBBLES_TELEMETRY=0 to disable.
EOF
}

fail_usage() { echo "goal-fidelity-telemetry: $*" >&2; exit 2; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$SCRIPT_DIR/../.." && pwd)"

event=""
goal_id=""
revision=""
digest=""
runner=""
phase=""
verdict=""
rule=""
disposition=""
goal_impact=""
finding_id=""
outcome=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --event) event="${2:-}"; shift 2 ;;
    --repo-root) repo_root="${2:-}"; shift 2 ;;
    --goal-id) goal_id="${2:-}"; shift 2 ;;
    --revision) revision="${2:-}"; shift 2 ;;
    --digest) digest="${2:-}"; shift 2 ;;
    --runner) runner="${2:-}"; shift 2 ;;
    --phase) phase="${2:-}"; shift 2 ;;
    --verdict) verdict="${2:-}"; shift 2 ;;
    --rule) rule="${2:-}"; shift 2 ;;
    --disposition) disposition="${2:-}"; shift 2 ;;
    --goal-impact) goal_impact="${2:-}"; shift 2 ;;
    --finding-id) finding_id="${2:-}"; shift 2 ;;
    --outcome) outcome="${2:-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    # Named explicitly so the refusal is a message rather than a generic
    # "unknown option" a caller might work around by picking another name.
    --details|--note|--reason|--message|--prompt|--text|--request)
      fail_usage "$1 is not accepted. Telemetry carries no free-text field, because that is how an operator prompt would reach the ledger. Record a closed enum value instead, or propose a new enum value." ;;
    *) fail_usage "unknown option: $1" ;;
  esac
done

[[ -n "$event" ]] || fail_usage "--event is required"
case "$event" in
  contract-frozen|contract-revised|expansion-requested|expansion-rejected| \
  boundary-refusal|finding-routed|finding-goal-blocking|phase-relevance) ;;
  *) fail_usage "unknown event type: $event" ;;
esac

# Enum validation. An out-of-enum value is a caller error, not a free-text
# escape hatch — otherwise the closed sets above would only be advisory.
[[ -z "$verdict" || "$verdict" == "run" || "$verdict" == "skip" ]] \
  || fail_usage "--verdict must be run or skip (observed: $verdict)"
case "${disposition:-}" in
  ""|in-boundary|route-same-repo|route-cross-repo|refuse-cross-repo) ;;
  *) fail_usage "--disposition must be one of in-boundary, route-same-repo, route-cross-repo, refuse-cross-repo (observed: $disposition)" ;;
esac
case "${goal_impact:-}" in
  ""|required|blocking-external|independent) ;;
  *) fail_usage "--goal-impact must be one of required, blocking-external, independent (observed: $goal_impact)" ;;
esac
case "${outcome:-}" in
  ""|accepted|refused) ;;
  *) fail_usage "--outcome must be accepted or refused (observed: $outcome)" ;;
esac
if [[ -n "$revision" && ! "$revision" =~ ^[0-9]+$ ]]; then
  fail_usage "--revision must be an integer (observed: $revision)"
fi
if [[ -n "$digest" && ! "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
  fail_usage "--digest must be sha256:<64 hex> (observed: $digest)"
fi

[[ "${BUBBLES_TELEMETRY:-1}" == "0" ]] && exit 0

runtime_dir="$repo_root/.specify/runtime"
ledger="$runtime_dir/framework-events.jsonl"

if ! mkdir -p "$runtime_dir" 2>/dev/null; then
  echo "goal-fidelity-telemetry: cannot create $runtime_dir — telemetry skipped (not a gate)" >&2
  exit 0
fi

timestamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

if command -v jq >/dev/null 2>&1; then
  record="$(jq -nc \
    --arg ts "$timestamp" \
    --arg type "goal-fidelity.$event" \
    --arg goalId "$goal_id" \
    --arg digest "$digest" \
    --arg runner "$runner" \
    --arg phase "$phase" \
    --arg verdict "$verdict" \
    --arg rule "$rule" \
    --arg disposition "$disposition" \
    --arg goalImpact "$goal_impact" \
    --arg findingId "$finding_id" \
    --arg outcome "$outcome" \
    --arg revision "$revision" \
    '{ version: 1, type: $type, timestamp: $ts }
     + (if $goalId       != "" then { goalId: $goalId } else {} end)
     + (if $revision     != "" then { revision: ($revision | tonumber) } else {} end)
     + (if $digest       != "" then { sourceRequestDigest: $digest } else {} end)
     + (if $runner       != "" then { runner: $runner } else {} end)
     + (if $phase        != "" then { phase: $phase } else {} end)
     + (if $verdict      != "" then { verdict: $verdict } else {} end)
     + (if $rule         != "" then { rule: $rule } else {} end)
     + (if $disposition  != "" then { disposition: $disposition } else {} end)
     + (if $goalImpact   != "" then { goalImpact: $goalImpact } else {} end)
     + (if $findingId    != "" then { findingId: $findingId } else {} end)
     + (if $outcome      != "" then { outcome: $outcome } else {} end)')" || {
    echo "goal-fidelity-telemetry: record construction failed — telemetry skipped (not a gate)" >&2
    exit 0
  }
else
  record="{\"version\":1,\"type\":\"goal-fidelity.$event\",\"timestamp\":\"$timestamp\"}"
fi

if ! printf '%s\n' "$record" >> "$ledger" 2>/dev/null; then
  echo "goal-fidelity-telemetry: cannot append to $ledger — telemetry skipped (not a gate)" >&2
  exit 0
fi
exit 0
