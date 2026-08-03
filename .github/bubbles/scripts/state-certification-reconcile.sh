#!/usr/bin/env bash
set -euo pipefail

# state-certification-reconcile.sh
#
# Repair a status-mirror divergence the legal way (IMP-032 SCOPE-4b).
#
# `status` and `certification.status` are one fact recorded twice. When they
# disagree the spec is unresolvable: transition-contract-resolver.sh refuses it
# with E009-STATUS-MIRROR before it reads the mode registry, so every later
# guard run reports targetStatus UNRESOLVED. Until now there was no third move.
# Reverting `status` discards the true observation that work shipped, and
# hand-writing `certification.status` forges a certification.
#
# This tool takes the third move, and it is deliberately not a file editor:
# it asks the transition guard whether the spec would pass at the status it
# already claims, and writes the mirror ONLY on a PASS verdict. A refusal is
# the expected outcome for a spec whose status really is ahead of its evidence.
#
# Ownership: certification.* is bubbles.validate-owned. --apply therefore
# requires BUBBLES_AGENT_NAME=bubbles.validate. That is an ownership
# declaration recorded at the call site, not an authentication boundary --
# it stops accidental misuse by another agent, and an operator running this
# by hand is asserting they are performing the validate role.
#
# The candidate is evaluated IN PLACE rather than in a copied directory,
# because the guard's git-history checks (Gate G053 / commit provenance)
# silently skip outside a work tree. A copy would therefore be evaluated more
# permissively than reality and could certify a spec the guard would refuse.
# The original state.json is restored by an EXIT/INT/TERM trap.

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/state-certification-reconcile.sh <spec-dir|state.json> [options]

Reconcile a status-mirror divergence by certifying the status the spec already
claims -- but only if the transition guard passes at that status.

Options:
  --apply
      Write certification.status back to state.json. Requires
      BUBBLES_AGENT_NAME=bubbles.validate. Without --apply, the candidate JSON
      is printed to stdout and the file is not modified.
  --dry-run
      Explicitly request the default dry-run behavior.
  -h, --help
      Print this help text.

Exit codes:
  0 = mirrors already agree, or the guard passed (candidate printed or applied)
  2 = usage, dependency, malformed JSON, or missing ownership declaration
  3 = refused: the guard did not pass at the claimed status, so certifying it
      would forge a certification. Route the spec to bubbles.validate.

Guarantees:
  - Never invents certifiedAt. Only certification.status is written.
  - Never promotes a status the transition guard has not evaluated and passed.
  - Never lowers top-level status: discarding the observation that work
    shipped is the other wrong move, not a repair.
EOF
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/state-transition-guard.sh"

TARGET=""
MODE="dry-run"

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --apply)
      MODE="apply"
      shift
      ;;
    --dry-run)
      MODE="dry-run"
      shift
      ;;
    --*)
      echo "state-certification-reconcile: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$TARGET" ]]; then
        echo "state-certification-reconcile: only one target may be supplied" >&2
        usage >&2
        exit 2
      fi
      TARGET="$1"
      shift
      ;;
  esac
done

if [[ -z "$TARGET" ]]; then
  echo "state-certification-reconcile: missing target spec directory or state.json" >&2
  usage >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "state-certification-reconcile: jq is required but not found in PATH" >&2
  exit 2
fi

if [[ ! -f "$GUARD" ]]; then
  echo "state-certification-reconcile: transition guard not found: $GUARD" >&2
  exit 2
fi

STATE_FILE="$TARGET"
SPEC_DIR="$TARGET"
if [[ -d "$TARGET" ]]; then
  STATE_FILE="$TARGET/state.json"
else
  SPEC_DIR="$(dirname "$TARGET")"
fi

if [[ ! -f "$STATE_FILE" ]]; then
  echo "state-certification-reconcile: state.json not found: $STATE_FILE" >&2
  exit 2
fi

if ! jq -e 'type == "object"' "$STATE_FILE" >/dev/null 2>&1; then
  echo "state-certification-reconcile: malformed or non-object JSON: $STATE_FILE" >&2
  exit 2
fi

current_status="$(jq -r 'if (.status | type) == "string" then .status else "" end' "$STATE_FILE")"
cert_status="$(jq -r 'if ((.certification // {}) | .status | type) == "string" then .certification.status else "" end' "$STATE_FILE")"

if [[ -z "$current_status" ]]; then
  echo "state-certification-reconcile: state.json has no top-level string status: $STATE_FILE" >&2
  exit 2
fi

# A spec with no certification block has nothing to reconcile. The block is
# created by a real certification run, so synthesizing one here would be the
# forging vector this tool exists to avoid.
if ! jq -e '(.certification | type) == "object"' "$STATE_FILE" >/dev/null 2>&1; then
  echo "state-certification-reconcile: no certification object to reconcile; certification is created by bubbles.validate, not by this tool: $STATE_FILE" >&2
  exit 2
fi

if [[ -z "$cert_status" ]]; then
  echo "state-certification-reconcile: certification.status is absent or not a string; route to bubbles.validate: $STATE_FILE" >&2
  exit 2
fi

if [[ "$current_status" == "$cert_status" ]]; then
  echo "state-certification-reconcile: mirrors already agree at '$current_status': $STATE_FILE" >&2
  exit 0
fi

if [[ "$MODE" == "apply" && "${BUBBLES_AGENT_NAME:-}" != "bubbles.validate" ]]; then
  echo "state-certification-reconcile: --apply writes certification.status, which is bubbles.validate-owned" >&2
  echo "state-certification-reconcile: re-run with BUBBLES_AGENT_NAME=bubbles.validate to declare that role (dry-run needs no declaration)" >&2
  exit 2
fi

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/bubbles-cert-reconcile.XXXXXXXX")"
CANDIDATE="$WORK_DIR/candidate.json"
BACKUP="$WORK_DIR/original.json"
GUARD_LOG="$WORK_DIR/guard.log"
RESTORE_NEEDED="false"

cleanup() {
  if [[ "$RESTORE_NEEDED" == "true" && -f "$BACKUP" ]]; then
    cp "$BACKUP" "$STATE_FILE" 2>/dev/null || true
  fi
  rm -rf "$WORK_DIR" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

cp "$STATE_FILE" "$BACKUP"

# The candidate advances certification.status to the already-claimed status and
# touches nothing else. certifiedAt is deliberately left exactly as found: an
# absent timestamp stays absent rather than being invented.
if ! jq --arg status "$current_status" '.certification.status = $status' "$BACKUP" > "$CANDIDATE"; then
  echo "state-certification-reconcile: failed to build the reconciled candidate: $STATE_FILE" >&2
  exit 2
fi

# Evaluate the candidate at full strength, in place, then restore.
RESTORE_NEEDED="true"
cp "$CANDIDATE" "$STATE_FILE"
guard_exit=0
bash "$GUARD" "$SPEC_DIR" > "$GUARD_LOG" 2>&1 || guard_exit=$?
cp "$BACKUP" "$STATE_FILE"
RESTORE_NEEDED="false"

guard_verdict="$(awk -F': ' '/^verdict: /{print $2}' "$GUARD_LOG" | tail -1)"
guard_blocking="$(awk -F': ' '/^blockingCode: /{print $2}' "$GUARD_LOG" | tail -1)"
[[ -n "$guard_verdict" ]] || guard_verdict="UNKNOWN"
[[ -n "$guard_blocking" ]] || guard_blocking="none"

if [[ "$guard_exit" -ne 0 || "$guard_verdict" != "PASS" ]]; then
  echo "state-certification-reconcile: REFUSED -- the transition guard does not pass at '$current_status'" >&2
  echo "state-certification-reconcile:   guard verdict=$guard_verdict blockingCode=$guard_blocking exit=$guard_exit" >&2
  echo "state-certification-reconcile: certifying this status would forge a certification the evidence does not support." >&2
  echo "state-certification-reconcile: route the spec to bubbles.validate to either earn the status or record the real one." >&2
  exit 3
fi

if [[ "$MODE" == "apply" ]]; then
  cp "$CANDIDATE" "$STATE_FILE"
  echo "state-certification-reconcile: reconciled certification.status '$cert_status' -> '$current_status' after a PASS verdict: $STATE_FILE"
  exit 0
fi

cat "$CANDIDATE"
echo "state-certification-reconcile: guard PASSES at '$current_status'; --apply would reconcile certification.status '$cert_status' -> '$current_status': $STATE_FILE" >&2
exit 0
