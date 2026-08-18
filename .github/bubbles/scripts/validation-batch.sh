#!/usr/bin/env bash
# validation-batch.sh — run compatible heavy obligations once, settle each.
#
# Capability: validation-debt-settlement
#
# WHY THIS EXISTS
# Deferred heavy work accumulates across changes, and running it one entry at a
# time repeats the same expensive setup for every obligation. Batching is the
# saving. The danger of batching is the accounting: one command runs, and it is
# tempting to call every grouped obligation settled because the batch "went
# green".
#
# THE TWO RULES THAT MAKE BATCHING SAFE
#   1. ONE SETTLING RECEIPT PER OBLIGATION. The batch runs once, but it writes a
#      separate receipt for each entry, each naming that entry's check, source
#      revision and closure digest. A settlement therefore still points at the
#      specific obligation it discharges.
#   2. A FAILED BATCH SETTLES NOTHING. Not the entries that "would have passed",
#      not the ones unrelated to the failure. Partial credit from a failed run is
#      how a red result becomes a green ledger.
#
# Compatibility: entries are groupable only when they share an obligation CLASS
# and a source REVISION. Two revisions in one batch would mean a single run
# claiming to have validated two different trees.
#
# Usage:
#   validation-batch.sh --class <class> --source-revision <rev> \
#                       --entry <id> [--entry <id> ...] -- <command> [args...]
#
# There is no --force, --partial or --settle-anyway flag.
#
# Exit codes:
#   0  the batch command passed and every grouped entry was settled
#   1  the batch command failed (nothing settled), or a settlement was refused
#   2  usage error

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
NAME="validation-batch"
DEBT="$SCRIPT_DIR/validation-debt.sh"
CLOSURE="$SCRIPT_DIR/validation-closure.sh"

LEDGER_DIR="${BUBBLES_VALIDATION_DEBT_DIR:-$REPO_ROOT/.specify/runtime/validation-debt}"
RECEIPT_DIR="$LEDGER_DIR/receipts"

CLASS=""
SOURCE_REVISION=""
declare -a ENTRIES=()
declare -a COMMAND=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --class) CLASS="${2:-}" && shift 2 ;;
    --source-revision) SOURCE_REVISION="${2:-}" && shift 2 ;;
    --entry)
      ENTRIES+=("${2:-}")
      shift 2
      ;;
    --ledger-dir)
      LEDGER_DIR="${2:-}"
      RECEIPT_DIR="$LEDGER_DIR/receipts"
      shift 2
      ;;
    --)
      shift
      COMMAND=("$@")
      break
      ;;
    -h | --help)
      printf 'usage: %s.sh --class <class> --source-revision <rev> --entry <id> [...] -- <command>\n' "$NAME"
      exit 0
      ;;
    *)
      printf '%s: unsupported flag "%s". This executor has no bypass.\n' "$NAME" "$1" >&2
      exit 2
      ;;
  esac
done

[[ -n "$CLASS" ]] || {
  printf '%s: --class is required\n' "$NAME" >&2
  exit 2
}
[[ -n "$SOURCE_REVISION" ]] || {
  printf '%s: --source-revision is required\n' "$NAME" >&2
  exit 2
}
[[ "${#ENTRIES[@]}" -gt 0 ]] || {
  printf '%s: at least one --entry is required\n' "$NAME" >&2
  exit 2
}
[[ "${#COMMAND[@]}" -gt 0 ]] || {
  printf '%s: a batch command is required after --\n' "$NAME" >&2
  exit 2
}

# Resolve each entry BEFORE running anything, so an unknown or already-settled
# obligation is refused up front rather than after an expensive run.
declare -a ENTRY_CHECKS=()
open_rows="$(bash "$DEBT" list --open --ledger-dir "$LEDGER_DIR" 2>/dev/null || true)"
for entry in "${ENTRIES[@]}"; do
  row="$(printf '%s\n' "$open_rows" | LC_ALL=C grep -F "\"id\":\"$entry\"" | head -1)"
  if [[ -z "$row" ]]; then
    printf '%s: no OPEN debt entry with id %s\n' "$NAME" "$entry" >&2
    exit 1
  fi
  if [[ "$row" != *"\"class\":\"$CLASS\""* ]]; then
    printf '%s: entry %s is not in obligation class %s — incompatible entries cannot share a batch.\n' "$NAME" "$entry" "$CLASS" >&2
    exit 1
  fi
  if [[ "$row" != *"\"sourceRevision\":\"$SOURCE_REVISION\""* ]]; then
    printf '%s: entry %s is not at source revision %s — one batch cannot validate two trees.\n' "$NAME" "$entry" "$SOURCE_REVISION" >&2
    exit 1
  fi
  check="${row#*\"check\":\"}"
  check="${check%%\"*}"
  ENTRY_CHECKS+=("$check")
done

batch_id="vb-$CLASS-$SOURCE_REVISION-$$"
printf '%s: running batch %s over %s obligation(s)\n' "$NAME" "$batch_id" "${#ENTRIES[@]}"

rc=0
"${COMMAND[@]}" || rc=$?

if [[ "$rc" -ne 0 ]]; then
  # The whole point. No receipts are written and no settlement is attempted, so
  # the ledger is byte-identical to what it was before the failed batch.
  printf '%s: batch %s FAILED (exit %s) — settling NOTHING. All %s obligation(s) remain open.\n' \
    "$NAME" "$batch_id" "$rc" "${#ENTRIES[@]}" >&2
  exit 1
fi

mkdir -p "$RECEIPT_DIR" 2>/dev/null || {
  printf '%s: cannot create the receipt directory: %s\n' "$NAME" "$RECEIPT_DIR" >&2
  exit 1
}

settled=0
i=0
for entry in "${ENTRIES[@]}"; do
  check="${ENTRY_CHECKS[$i]}"
  i=$((i + 1))
  closure_digest="$(bash "$CLOSURE" digest "$check" 2>/dev/null || true)"
  # A check with an unknown closure has no digest. It still gets a receipt, but
  # the digest records the honest value rather than a fabricated one.
  [[ -n "$closure_digest" ]] || closure_digest="unknown-closure"
  receipt="$RECEIPT_DIR/$batch_id-$entry.json"
  printf '{"batch":"%s","entry":"%s","check":"%s","class":"%s","sourceRevision":"%s","closureDigest":"%s","exitCode":"0","command":"%s","writtenAt":"%s"}\n' \
    "$batch_id" "$entry" "$check" "$CLASS" "$SOURCE_REVISION" "$closure_digest" \
    "${COMMAND[0]}" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$receipt"

  if bash "$DEBT" settle --entry "$entry" --receipt "$receipt" --ledger-dir "$LEDGER_DIR" >/dev/null; then
    settled=$((settled + 1))
  else
    printf '%s: settlement REFUSED for %s — the ledger rejected its receipt.\n' "$NAME" "$entry" >&2
    exit 1
  fi
done

printf '%s: batch %s passed; settled %s obligation(s) with one receipt each.\n' "$NAME" "$batch_id" "$settled"
exit 0
