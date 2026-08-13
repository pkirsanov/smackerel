#!/usr/bin/env bash
# goal-boundary-receipt.sh — the canonical G134 boundary-receipt producer
# (IMP-041 SCOPE-3 / GF-7).
#
# WHY THIS EXISTS
#
# goal-fidelity-guard.sh already defines six boundaries, and workflows.yaml
# already lists G134 as a universal static gate. Neither fact makes the guard
# RUN: repository search finds a direct pre-certification call in
# bubbles.validate and nowhere else, and state-transition-guard.sh does not
# invoke G134 at all. A mutable goal could therefore reach planning and dispatch
# having passed no boundary check, while every registry surface said otherwise.
#
# Registry membership is not execution. A prose instruction telling a runner to
# "check the boundary" is not execution either — nothing downstream can tell
# whether it happened. This script closes that gap by making the check produce a
# TRANSFERABLE ARTEFACT: a receipt is emitted ONLY after the guard exits 0, so a
# consumer that demands a valid receipt is demanding the check actually ran.
#
# Subcommands
#   emit    run a boundary through goal-fidelity-guard.sh; on success print a
#           canonical receipt to stdout. On guard failure print nothing and
#           propagate the guard's exit code.
#   verify  re-derive a receipt's bindings from the CURRENT contract and refuse
#           a receipt that is stale, substituted, or for another boundary.
#
# Exit codes
#   0  boundary held (emit) / receipt is current and matching (verify)
#   1  REFUSED — boundary failed, or receipt stale/substituted/mismatched
#   2  usage or runtime error (missing input, missing jq, unreadable file)
#
# There is no --force / --skip / --assume-passed. A receipt that can be minted
# without the guard passing is exactly the theatre this script removes.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="${BUBBLES_GOAL_FIDELITY_GUARD:-$SCRIPT_DIR/goal-fidelity-guard.sh}"
RECEIPT_VERSION="goal-boundary-receipt/v1"

BOUNDARIES="pre-planning post-planning pre-dispatch post-finding post-compaction pre-certification"

usage() {
  cat <<'EOF'
Usage: goal-boundary-receipt.sh emit --boundary <name> --session-file <path> [guard args...]
       goal-boundary-receipt.sh verify --receipt-file <path> --session-file <path> [--expect-boundary <name>]

  Boundaries: pre-planning post-planning pre-dispatch post-finding
              post-compaction pre-certification

  emit passes every unrecognised argument straight through to
  goal-fidelity-guard.sh, so each boundary keeps its own required inputs
  (--spec-dir, --candidate-repo, --changed-path, --ref-file, ...).

  Optional binding inputs (emit):
    --scenario-file <path>   binds the compiled scenario's digest
    --planned-delta <json>   binds a canonical planned-delta digest

There is no --force / --skip / --assume-passed.
EOF
}

fail_usage() {
  echo "goal-boundary-receipt: $*" >&2
  exit 2
}
fail_refuse() {
  echo "goal-boundary-receipt: REFUSED — $*" >&2
  exit 1
}

command -v jq >/dev/null 2>&1 || fail_usage "jq is required"

sha256_stdin() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 | awk '{print $1}'
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 | awk '{print $NF}'
  else
    echo "goal-boundary-receipt: sha256sum, shasum, or openssl is required" >&2
    exit 2
  fi
}

# Canonical digest of a JSON value: sorted keys, compact, so two structurally
# identical bindings always digest identically regardless of key order.
canon_digest() {
  local value="$1"
  if [[ -z "$value" || "$value" == "null" ]]; then
    printf 'sha256:absent'
    return
  fi
  printf 'sha256:%s' "$(jq -S -c '.' <<< "$value" | sha256_stdin)"
}

in_enum() {
  case " $2 " in
    *" $1 "*) return 0 ;;
    *) return 1 ;;
  esac
}

# --- emit -------------------------------------------------------------------
cmd_emit() {
  local boundary="" session_file="" scenario_file="" planned_delta=""
  local passthrough=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --boundary) boundary="${2:-}"; shift 2 ;;
      --session-file)
        session_file="${2:-}"
        passthrough[${#passthrough[@]}]="--session-file"
        passthrough[${#passthrough[@]}]="${2:-}"
        shift 2 ;;
      --scenario-file) scenario_file="${2:-}"; shift 2 ;;
      --planned-delta) planned_delta="${2:-}"; shift 2 ;;
      -h|--help) usage; exit 0 ;;
      --force|--skip|--assume-passed|--no-verify)
        fail_usage "bypass-shaped flag '$1' does not exist — a receipt is only valid when the boundary actually passed" ;;
      *) passthrough[${#passthrough[@]}]="$1"; shift ;;
    esac
  done

  [[ -n "$boundary" ]] || fail_usage "emit requires --boundary"
  in_enum "$boundary" "$BOUNDARIES" || fail_usage "--boundary must be one of: $BOUNDARIES (observed: $boundary)"
  [[ -n "$session_file" ]] || fail_usage "emit requires --session-file"
  [[ -f "$session_file" ]] || fail_usage "session file not found: $session_file"
  [[ -x "$GUARD" || -f "$GUARD" ]] || fail_usage "goal-fidelity-guard.sh not found at $GUARD"

  # THE ORDERING THAT MATTERS: the guard runs FIRST and its exit code is
  # propagated unchanged. Nothing is printed on failure, so a caller cannot
  # salvage a receipt from a boundary that did not hold.
  local guard_rc=0
  bash "$GUARD" --boundary "$boundary" ${passthrough[@]+"${passthrough[@]}"} >&2 || guard_rc=$?
  if [[ "$guard_rc" -ne 0 ]]; then
    echo "goal-boundary-receipt: no receipt minted — boundary '$boundary' did not hold (goal-fidelity-guard exit $guard_rc)" >&2
    exit "$guard_rc"
  fi

  local contract
  contract="$(jq -c '.goalContract // empty' "$session_file")"
  [[ -n "$contract" ]] || fail_refuse "no Goal Contract at .goalContract in $session_file"

  local goal_id revision digest semantic scenario_digest delta_digest emitted_at
  goal_id="$(jq -r '.goalId // ""' <<< "$contract")"
  revision="$(jq -r '.revision // 0' <<< "$contract")"
  digest="$(jq -r '.sourceRequestDigest // ""' <<< "$contract")"
  semantic="$(jq -c '.semanticBoundary // null' <<< "$contract")"

  scenario_digest="sha256:absent"
  if [[ -n "$scenario_file" ]]; then
    [[ -f "$scenario_file" ]] || fail_usage "scenario file not found: $scenario_file"
    scenario_digest="$(canon_digest "$(jq -c '.' "$scenario_file")")"
  fi
  delta_digest="$(canon_digest "${planned_delta:-null}")"
  emitted_at="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

  local body
  body="$(jq -n \
    --arg v "$RECEIPT_VERSION" \
    --arg b "$boundary" \
    --arg gid "$goal_id" \
    --argjson rev "$revision" \
    --arg srd "$digest" \
    --arg sbd "$(canon_digest "$semantic")" \
    --arg scd "$scenario_digest" \
    --arg pdd "$delta_digest" \
    '{
      receiptVersion: $v,
      boundary: $b,
      goalId: $gid,
      revision: $rev,
      sourceRequestDigest: $srd,
      semanticBoundaryDigest: $sbd,
      scenarioDigest: $scd,
      plannedDeltaDigest: $pdd
    }')"

  # receiptDigest covers every binding above, so altering any one of them after
  # the fact invalidates the receipt rather than silently re-pointing it.
  jq -n --argjson body "$body" --arg at "$emitted_at" --arg rd "$(canon_digest "$body")" \
    '$body + { emittedAt: $at, receiptDigest: $rd }'
}

# --- verify -----------------------------------------------------------------
cmd_verify() {
  local receipt_file="" session_file="" expect_boundary=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --receipt-file) receipt_file="${2:-}"; shift 2 ;;
      --session-file) session_file="${2:-}"; shift 2 ;;
      --expect-boundary) expect_boundary="${2:-}"; shift 2 ;;
      -h|--help) usage; exit 0 ;;
      --force|--skip|--assume-passed|--no-verify)
        fail_usage "bypass-shaped flag '$1' does not exist" ;;
      *) fail_usage "unknown option: $1" ;;
    esac
  done

  [[ -n "$receipt_file" ]] || fail_usage "verify requires --receipt-file"
  [[ -f "$receipt_file" ]] || fail_usage "receipt file not found: $receipt_file"
  [[ -n "$session_file" ]] || fail_usage "verify requires --session-file"
  [[ -f "$session_file" ]] || fail_usage "session file not found: $session_file"
  jq empty "$receipt_file" 2>/dev/null || fail_usage "receipt file is not valid JSON: $receipt_file"

  local receipt contract
  receipt="$(jq -c '.' "$receipt_file")"
  contract="$(jq -c '.goalContract // empty' "$session_file")"
  [[ -n "$contract" ]] || fail_refuse "no Goal Contract at .goalContract in $session_file"

  local r_version r_boundary r_goal r_rev r_srd r_sbd r_rd
  r_version="$(jq -r '.receiptVersion // ""' <<< "$receipt")"
  r_boundary="$(jq -r '.boundary // ""' <<< "$receipt")"
  r_goal="$(jq -r '.goalId // ""' <<< "$receipt")"
  r_rev="$(jq -r '.revision // -1' <<< "$receipt")"
  r_srd="$(jq -r '.sourceRequestDigest // ""' <<< "$receipt")"
  r_sbd="$(jq -r '.semanticBoundaryDigest // ""' <<< "$receipt")"
  r_rd="$(jq -r '.receiptDigest // ""' <<< "$receipt")"

  [[ "$r_version" == "$RECEIPT_VERSION" ]] ||
    fail_refuse "receiptVersion '$r_version' is not '$RECEIPT_VERSION'"
  in_enum "$r_boundary" "$BOUNDARIES" ||
    fail_refuse "receipt names unknown boundary '$r_boundary'"
  if [[ -n "$expect_boundary" && "$r_boundary" != "$expect_boundary" ]]; then
    fail_refuse "receipt is for boundary '$r_boundary' but '$expect_boundary' was required — a receipt from another boundary proves nothing about this one"
  fi

  # Tamper check BEFORE the contract comparison: a receipt whose body was edited
  # cannot be trusted to describe what the guard actually saw.
  local recomputed
  recomputed="$(canon_digest "$(jq -c '{receiptVersion,boundary,goalId,revision,sourceRequestDigest,semanticBoundaryDigest,scenarioDigest,plannedDeltaDigest}' <<< "$receipt")")"
  [[ "$recomputed" == "$r_rd" ]] ||
    fail_refuse "receiptDigest does not cover the receipt body — the receipt was edited after it was minted"

  local c_goal c_rev c_srd c_sbd
  c_goal="$(jq -r '.goalId // ""' <<< "$contract")"
  c_rev="$(jq -r '.revision // -2' <<< "$contract")"
  c_srd="$(jq -r '.sourceRequestDigest // ""' <<< "$contract")"
  c_sbd="$(canon_digest "$(jq -c '.semanticBoundary // null' <<< "$contract")")"

  [[ "$r_goal" == "$c_goal" ]] ||
    fail_refuse "receipt goalId '$r_goal' does not match the current contract '$c_goal'"
  [[ "$r_rev" == "$c_rev" ]] ||
    fail_refuse "receipt revision $r_rev is stale against the current contract revision $c_rev — re-run the boundary after a revision"
  [[ "$r_srd" == "$c_srd" ]] ||
    fail_refuse "receipt sourceRequestDigest does not match the current contract"
  [[ "$r_sbd" == "$c_sbd" ]] ||
    fail_refuse "receipt semanticBoundaryDigest does not match the current contract — the declared boundary changed after this receipt was minted"

  echo "goal-boundary-receipt: OK (boundary '$r_boundary' for $c_goal revision $c_rev)"
}

case "${1:-}" in
  emit) shift; cmd_emit "$@" ;;
  verify) shift; cmd_verify "$@" ;;
  -h|--help|"") usage; [[ -n "${1:-}" ]] && exit 0 || exit 2 ;;
  *) fail_usage "unknown subcommand: $1" ;;
esac
