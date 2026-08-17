#!/usr/bin/env bash
# validation-debt.sh — the append-only validation debt ledger.
#
# Capability: validation-debt-settlement
#
# WHY THIS EXISTS
# Tier selection, changed-only selection and the result cache all REMOVE work
# from a run. None of them recorded what they removed. A fast run therefore
# produced the same terminal shape as a complete one, and the difference existed
# only in a scrollback nobody keeps. "We skipped it and will do it later" is a
# promise; without a ledger it is a promise with no creditor.
#
# Every deliberate deferral writes EXACTLY ONE open entry naming the check, the
# obligation class, the source revision it was deferred at, the spec or scope
# that owns it, a reason token and the boundary at which it must be settled.
#
# THE RULE THAT KEEPS IT HONEST
# A MISSING LEDGER WRITE FORCES IMMEDIATE EXECUTION. `record` exits non-zero
# when it cannot append, and its caller must then run the check instead of
# skipping it. A deferral that could not be recorded is not a deferral, it is a
# silent skip, which is the failure this whole file exists to remove.
#
# THE BASIC FLOOR
# `basic` is not an assurance level. It is the immediate affected-validation
# floor INSIDE `fast`: before any heavy obligation may be deferred, the affected
# set must already have run at this revision. `record` therefore requires a
# floor receipt naming the same source revision, so debt can never be taken out
# against work that has not started.
#
# APPEND-ONLY
# Entries are never rewritten and never deleted. A settlement is a NEW record
# that references the entry it settles. Rewriting history is how a ledger stops
# being evidence.
#
# Usage:
#   validation-debt.sh record --check <id> --class <class> --source-revision <rev> \
#                             --spec <ref> --reason <token> --boundary <boundary> \
#                             --floor-receipt <path>
#   validation-debt.sh list [--open|--settled]
#   validation-debt.sh count [--open] [--boundary <boundary>]
#   validation-debt.sh settle --entry <id> --receipt <path>
#   validation-debt.sh assurance --level <full|fast|prototype> [--boundary <boundary>]
#
# There is no --skip, --force, --clear or --forgive flag. Debt is settled by
# running the work, never by declaring it settled.
#
# Exit codes:
#   0  the operation succeeded (for `assurance`: the level is satisfiable)
#   1  a refusal: open debt blocks the requested level/boundary, or a settlement
#      was rejected because its receipt does not match the entry
#   2  usage error, or the ledger directory could not be prepared
#   3  the append failed, or a duplicate open entry already exists — the caller
#      MUST execute the check rather than treat it as deferred

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
NAME="validation-debt"

LEDGER_DIR="${BUBBLES_VALIDATION_DEBT_DIR:-$REPO_ROOT/.specify/runtime/validation-debt}"
LEDGER="$LEDGER_DIR/ledger.jsonl"

VALID_CLASSES="heavy-selftest integration e2e stress live-guard regression"
VALID_BOUNDARIES="certification promotion deploy release"

SUBCOMMAND="${1:-}"
[[ $# -gt 0 ]] && shift

CHECK=""
CLASS=""
SOURCE_REVISION=""
SPEC_REF=""
REASON=""
BOUNDARY=""
FLOOR_RECEIPT=""
ENTRY=""
RECEIPT=""
LEVEL=""
FILTER="open"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check) CHECK="${2:-}" && shift 2 ;;
    --class) CLASS="${2:-}" && shift 2 ;;
    --source-revision) SOURCE_REVISION="${2:-}" && shift 2 ;;
    --spec) SPEC_REF="${2:-}" && shift 2 ;;
    --reason) REASON="${2:-}" && shift 2 ;;
    --boundary) BOUNDARY="${2:-}" && shift 2 ;;
    --floor-receipt) FLOOR_RECEIPT="${2:-}" && shift 2 ;;
    --entry) ENTRY="${2:-}" && shift 2 ;;
    --receipt) RECEIPT="${2:-}" && shift 2 ;;
    --level) LEVEL="${2:-}" && shift 2 ;;
    --ledger-dir)
      LEDGER_DIR="${2:-}"
      LEDGER="$LEDGER_DIR/ledger.jsonl"
      shift 2
      ;;
    --open)
      FILTER="open"
      shift
      ;;
    --settled)
      FILTER="settled"
      shift
      ;;
    --all)
      FILTER="all"
      shift
      ;;
    -h | --help)
      printf 'usage: %s.sh <record|list|count|settle|assurance> [options]\n' "$NAME"
      exit 0
      ;;
    *)
      printf '%s: unsupported flag "%s". This ledger has no bypass.\n' "$NAME" "$1" >&2
      exit 2
      ;;
  esac
done

json_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  printf '%s' "$s"
}

# Field read from a JSONL row without a JSON parser. The rows are written by this
# script alone, in one fixed shape, so a positional read is exact rather than a
# guess about arbitrary JSON.
row_field() {
  local row="$1" key="$2" rest
  rest="${row#*\"$key\":\"}"
  [[ "$rest" == "$row" ]] && return 1
  printf '%s' "${rest%%\"*}"
}

open_entries() {
  [[ -f "$LEDGER" ]] || return 0
  local row id settled=$'\n'
  while IFS= read -r row; do
    [[ "$row" == *'"event":"settle"'* ]] || continue
    id="$(row_field "$row" "entry" || true)"
    [[ -n "$id" ]] && settled+="$id"$'\n'
  done <"$LEDGER"
  while IFS= read -r row; do
    [[ "$row" == *'"event":"defer"'* ]] || continue
    id="$(row_field "$row" "id" || true)"
    [[ -n "$id" ]] || continue
    [[ "$settled" == *$'\n'"$id"$'\n'* ]] && continue
    printf '%s\n' "$row"
  done <"$LEDGER"
}

settled_entries() {
  [[ -f "$LEDGER" ]] || return 0
  local row
  while IFS= read -r row; do
    [[ "$row" == *'"event":"settle"'* ]] && printf '%s\n' "$row"
  done <"$LEDGER"
}

case "$SUBCOMMAND" in
  record)
    for pair in "check:$CHECK" "class:$CLASS" "source-revision:$SOURCE_REVISION" \
      "spec:$SPEC_REF" "reason:$REASON" "boundary:$BOUNDARY" "floor-receipt:$FLOOR_RECEIPT"; do
      if [[ -z "${pair#*:}" ]]; then
        printf '%s: record requires --%s\n' "$NAME" "${pair%%:*}" >&2
        exit 2
      fi
    done
    case " $VALID_CLASSES " in
      *" $CLASS "*) ;;
      *)
        printf '%s: --class must be one of: %s\n' "$NAME" "$VALID_CLASSES" >&2
        exit 2
        ;;
    esac
    case " $VALID_BOUNDARIES " in
      *" $BOUNDARY "*) ;;
      *)
        printf '%s: --boundary must be one of: %s\n' "$NAME" "$VALID_BOUNDARIES" >&2
        exit 2
        ;;
    esac

    # The basic floor. Heavy debt may only be taken out once the affected set has
    # already run AT THIS REVISION; otherwise "fast" would mean "ran nothing and
    # promised everything".
    if [[ ! -f "$FLOOR_RECEIPT" ]]; then
      printf '%s: floor receipt not found: %s\n' "$NAME" "$FLOOR_RECEIPT" >&2
      printf '%s: the basic affected-validation floor must run BEFORE heavy debt is recorded.\n' "$NAME" >&2
      exit 3
    fi
    if ! LC_ALL=C grep -Fq "$SOURCE_REVISION" "$FLOOR_RECEIPT" 2>/dev/null; then
      printf '%s: floor receipt %s does not name source revision %s\n' "$NAME" "$FLOOR_RECEIPT" "$SOURCE_REVISION" >&2
      exit 3
    fi

    if ! mkdir -p "$LEDGER_DIR" 2>/dev/null; then
      printf '%s: cannot create the ledger directory: %s\n' "$NAME" "$LEDGER_DIR" >&2
      printf '%s: a deferral that cannot be recorded is a silent skip — EXECUTE the check.\n' "$NAME" >&2
      exit 3
    fi

    entry_id="vd-$CHECK-$SOURCE_REVISION-$CLASS"
    if open_entries | LC_ALL=C grep -Fq "\"id\":\"$entry_id\""; then
      printf '%s: an open entry already exists for %s at %s (%s).\n' "$NAME" "$CHECK" "$SOURCE_REVISION" "$CLASS" >&2
      printf '%s: exactly one open entry per deferral — refusing to create a second.\n' "$NAME" >&2
      exit 3
    fi

    row="{\"event\":\"defer\",\"id\":\"$(json_escape "$entry_id")\",\"check\":\"$(json_escape "$CHECK")\",\"class\":\"$(json_escape "$CLASS")\",\"sourceRevision\":\"$(json_escape "$SOURCE_REVISION")\",\"spec\":\"$(json_escape "$SPEC_REF")\",\"reason\":\"$(json_escape "$REASON")\",\"boundary\":\"$(json_escape "$BOUNDARY")\",\"recordedAt\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}"
    if ! printf '%s\n' "$row" >>"$LEDGER" 2>/dev/null; then
      printf '%s: append to %s failed — EXECUTE the check instead of deferring it.\n' "$NAME" "$LEDGER" >&2
      exit 3
    fi
    printf '%s\n' "$entry_id"
    exit 0
    ;;

  list)
    case "$FILTER" in
      open) open_entries ;;
      settled) settled_entries ;;
      all) [[ -f "$LEDGER" ]] && cat "$LEDGER" ;;
    esac
    exit 0
    ;;

  count)
    if [[ -n "$BOUNDARY" ]]; then
      open_entries | LC_ALL=C grep -c "\"boundary\":\"$BOUNDARY\"" || true
    else
      open_entries | LC_ALL=C grep -c '"event":"defer"' || true
    fi
    exit 0
    ;;

  settle)
    [[ -n "$ENTRY" ]] || {
      printf '%s: settle requires --entry\n' "$NAME" >&2
      exit 2
    }
    [[ -n "$RECEIPT" ]] || {
      printf '%s: settle requires --receipt\n' "$NAME" >&2
      exit 2
    }
    [[ -f "$RECEIPT" ]] || {
      printf '%s: receipt not found: %s\n' "$NAME" "$RECEIPT" >&2
      exit 1
    }

    entry_row="$(open_entries | LC_ALL=C grep -F "\"id\":\"$ENTRY\"" | head -1)"
    if [[ -z "$entry_row" ]]; then
      printf '%s: no OPEN entry with id %s\n' "$NAME" "$ENTRY" >&2
      exit 1
    fi
    entry_check="$(row_field "$entry_row" "check" || true)"
    entry_rev="$(row_field "$entry_row" "sourceRevision" || true)"

    receipt_body="$(cat "$RECEIPT" 2>/dev/null || true)"
    receipt_check="$(row_field "$receipt_body" "check" || true)"
    receipt_rev="$(row_field "$receipt_body" "sourceRevision" || true)"
    receipt_exit="$(row_field "$receipt_body" "exitCode" || true)"
    receipt_closure="$(row_field "$receipt_body" "closureDigest" || true)"

    # A settlement is only real when the receipt describes THIS check, at THIS
    # revision, and the work actually passed. Anything looser lets one green run
    # discharge unrelated obligations.
    if [[ "$receipt_check" != "$entry_check" || "$receipt_rev" != "$entry_rev" ]]; then
      printf '%s: receipt does not match the entry (check=%s/%s revision=%s/%s)\n' \
        "$NAME" "$receipt_check" "$entry_check" "$receipt_rev" "$entry_rev" >&2
      exit 1
    fi
    if [[ "$receipt_exit" != "0" ]]; then
      printf '%s: receipt records exitCode=%s — a failed run settles nothing.\n' "$NAME" "${receipt_exit:-<absent>}" >&2
      exit 1
    fi
    if [[ -z "$receipt_closure" ]]; then
      printf '%s: receipt carries no closureDigest — the settled work is not bound to an input set.\n' "$NAME" >&2
      exit 1
    fi

    mkdir -p "$LEDGER_DIR" 2>/dev/null || {
      printf '%s: cannot create the ledger directory: %s\n' "$NAME" "$LEDGER_DIR" >&2
      exit 2
    }
    row="{\"event\":\"settle\",\"entry\":\"$(json_escape "$ENTRY")\",\"check\":\"$(json_escape "$entry_check")\",\"sourceRevision\":\"$(json_escape "$entry_rev")\",\"closureDigest\":\"$(json_escape "$receipt_closure")\",\"receipt\":\"$(json_escape "$RECEIPT")\",\"settledAt\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}"
    printf '%s\n' "$row" >>"$LEDGER" || exit 2
    printf '%s\n' "$ENTRY"
    exit 0
    ;;

  assurance)
    case "$LEVEL" in
      full | fast | prototype) ;;
      *)
        printf '%s: --level must be full|fast|prototype\n' "$NAME" >&2
        exit 2
        ;;
    esac
    open_count="$(open_entries | LC_ALL=C grep -c '"event":"defer"' || true)"
    open_count="${open_count//[^0-9]/}"
    [[ -n "$open_count" ]] || open_count=0

    if [[ "$LEVEL" == "prototype" ]]; then
      printf '%s: prototype is never deployable (R5 invariant); open debt=%s\n' "$NAME" "$open_count" >&2
      exit 1
    fi

    if [[ "$LEVEL" == "full" ]]; then
      if [[ "$open_count" -gt 0 ]]; then
        printf '%s: REFUSED — full assurance requires ZERO open validation debt, found %s.\n' "$NAME" "$open_count" >&2
        open_entries >&2
        exit 1
      fi
      printf '%s: OK — full assurance, zero open validation debt.\n' "$NAME"
      exit 0
    fi

    # fast: debt is permitted only while it stays BOUND. An entry with no
    # settlement boundary, or one outside the closed set, is unbounded debt,
    # which is indistinguishable from a silent skip.
    unbound=0
    while IFS= read -r row; do
      [[ -n "$row" ]] || continue
      b="$(row_field "$row" "boundary" || true)"
      case " $VALID_BOUNDARIES " in
        *" $b "*) ;;
        *) unbound=$((unbound + 1)) ;;
      esac
    done < <(open_entries)
    if [[ "$unbound" -gt 0 ]]; then
      printf '%s: REFUSED — fast assurance permits only BOUND debt; %s entry/entries have no valid settlement boundary.\n' "$NAME" "$unbound" >&2
      exit 1
    fi
    if [[ -n "$BOUNDARY" ]]; then
      due="$(open_entries | LC_ALL=C grep -c "\"boundary\":\"$BOUNDARY\"" || true)"
      due="${due//[^0-9]/}"
      [[ -n "$due" ]] || due=0
      if [[ "$due" -gt 0 ]]; then
        printf '%s: REFUSED — %s open obligation(s) are due at the %s boundary.\n' "$NAME" "$due" "$BOUNDARY" >&2
        exit 1
      fi
    fi
    printf '%s: OK — fast assurance with %s bound open obligation(s).\n' "$NAME" "$open_count"
    exit 0
    ;;

  "" | -h | --help)
    printf 'usage: %s.sh <record|list|count|settle|assurance> [options]\n' "$NAME"
    exit 0
    ;;
  *)
    printf '%s: unknown subcommand "%s"\n' "$NAME" "$SUBCOMMAND" >&2
    exit 2
    ;;
esac
