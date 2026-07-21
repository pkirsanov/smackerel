#!/usr/bin/env bash
# Execution Substate Guard (IMP-100 Phase 2 / IMP-024 SCOPE-3)
# ---------------------------------------------------------------------------
# Honest, visible EXECUTION progress that is distinct from validate-owned
# CERTIFICATION. During execution a scope may advance through substates before a
# certifying agent ever runs:
#   - implemented           — code/tests written by bubbles.implement / bubbles.test
#   - independently_verified — a separate check confirmed the behavior
#   - needs_reverification   — a later change invalidated a prior verification
# These live ONLY in `execution.substate`. They are NOT terminal statuses and NOT
# certification — `bubbles.implement` / `bubbles.test` write them; only
# `bubbles.validate` owns `certification.*`.
#
# This guard enforces two integrity invariants (BLOCKING — a violation is a real
# fabrication risk, not a style nit):
#   1. VALID VOCABULARY — when present, `execution.substate` MUST be one of the
#      three values above (a string).
#   2. NAMESPACE SEPARATION — none of the three substate values may appear in any
#      terminal-status / certification-status field (`status`, `certification.status`,
#      `proposedStatus`, `certification.proposedStatus`, `targetStatus`,
#      `terminalStatus`, `execution.terminalOutcome`, `resultEnvelope.outcome`).
#      An execution progress marker can NEVER masquerade as a certified terminal
#      status.
#
# BACKWARD-COMPATIBLE: a state.json without `execution.substate` and with normal
# terminal/cert statuses passes untouched (exit 0). There is no bypass flag.
#
# Usage:
#   bash bubbles/scripts/execution-substate-guard.sh <feature-dir>
#
# Exit codes:
#   0  clean / not-applicable
#   1  an execution-substate integrity violation
#   2  usage / runtime error
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: execution-substate-guard.sh <feature-dir>

Validates state.json execution.substate is a valid execution progress marker
(implemented | independently_verified | needs_reverification) and that none of
those values leak into a terminal-status or certification-status field. Blocking.
EOF
}

feature_dir="${1:-}"
if [[ -z "$feature_dir" ]]; then
  usage >&2
  exit 2
fi
if [[ ! -d "$feature_dir" ]]; then
  echo "execution-substate-guard: feature dir not found: $feature_dir" >&2
  exit 2
fi

state_file="$feature_dir/state.json"
if [[ ! -f "$state_file" ]]; then
  echo "[execution-substate-guard] no state.json in $feature_dir — nothing to check"
  exit 0
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "execution-substate-guard: jq is required but not found in PATH" >&2
  exit 2
fi
if ! jq -e 'type == "object"' "$state_file" >/dev/null 2>&1; then
  echo "execution-substate-guard: malformed or non-object JSON: $state_file" >&2
  exit 2
fi

findings=0
violation() {
  echo "❌ execution-substate-guard: $*" >&2
  findings=$((findings + 1))
}

# ---------------------------------------------------------------------------
# 1. Valid vocabulary — execution.substate must be a string in the closed set.
# ---------------------------------------------------------------------------
substate_type="$(jq -r 'if (.execution? | type) == "object" and (.execution | has("substate")) then (.execution.substate | type) else "missing" end' "$state_file")"
if [[ "$substate_type" != "missing" ]]; then
  if [[ "$substate_type" != "string" ]]; then
    echo "execution-substate-guard: execution.substate must be a string when present in $state_file" >&2
    exit 2
  fi
  substate="$(jq -r '.execution.substate' "$state_file")"
  case "$substate" in
    implemented | independently_verified | needs_reverification) ;;
    *)
      violation "execution.substate '$substate' is not a valid execution progress marker (expected implemented | independently_verified | needs_reverification)"
      ;;
  esac
fi

# ---------------------------------------------------------------------------
# 2. Namespace separation — no substate value may appear in a terminal/cert field.
# ---------------------------------------------------------------------------
while IFS=$'\t' read -r field value; do
  [[ -n "$field" ]] || continue
  case "$value" in
    implemented | independently_verified | needs_reverification)
      violation "$field is '$value' — an execution substate must never appear in a terminal/certification status field (write it only to execution.substate)"
      ;;
  esac
done < <(jq -r '
  [
    {f: "status",                         v: (.status? // null)},
    {f: "certification.status",           v: (.certification?.status? // null)},
    {f: "proposedStatus",                 v: (.proposedStatus? // null)},
    {f: "certification.proposedStatus",   v: (.certification?.proposedStatus? // null)},
    {f: "targetStatus",                   v: (.targetStatus? // null)},
    {f: "certification.targetStatus",     v: (.certification?.targetStatus? // null)},
    {f: "terminalStatus",                 v: (.terminalStatus? // null)},
    {f: "execution.terminalOutcome",      v: (.execution?.terminalOutcome? // null)},
    {f: "resultEnvelope.outcome",         v: (.resultEnvelope?.outcome? // null)}
  ]
  | .[]
  | select(.v != null)
  | [.f, (.v | tostring)]
  | @tsv
' "$state_file")

if [[ "$findings" -gt 0 ]]; then
  echo "execution-substate-guard: $findings integrity violation(s) in $feature_dir" >&2
  exit 1
fi

echo "[execution-substate-guard] OK — execution substate (if any) is valid and distinct from certification in $feature_dir."
exit 0
