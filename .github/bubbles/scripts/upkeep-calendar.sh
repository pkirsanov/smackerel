#!/usr/bin/env bash
# upkeep-calendar.sh — reads config/upkeep-calendar.yaml + the per-host
# /srv/backups/upkeep-ledger.jsonl, computes which upkeep tasks are due,
# and prints them ordered by priority.
#
# Output format:
#   TASK_ID CADENCE LAST_RUN_ISO STATUS NEXT_DUE_ISO
#
# Exit 0 = always (this is an advisory tool). No mutations.

set -euo pipefail

REPO_ROOT="${1:-.}"
CAL_FILE="$REPO_ROOT/config/upkeep-calendar.yaml"
LEDGER="${UPKEEP_LEDGER:-/srv/backups/upkeep-ledger.jsonl}"

command -v yq >/dev/null 2>&1 || { echo "[upkeep-calendar] yq required" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "[upkeep-calendar] jq required" >&2; exit 1; }

if [[ ! -f "$CAL_FILE" ]]; then
  echo "[upkeep-calendar] config/upkeep-calendar.yaml not found at $CAL_FILE"
  echo "[upkeep-calendar] no upkeep tasks configured"
  exit 0
fi

now_epoch="$(date -u +%s)"

cadence_to_seconds() {
  case "$1" in
    daily)     echo 86400 ;;
    weekly)    echo 604800 ;;
    monthly)   echo 2592000 ;;
    quarterly) echo 7776000 ;;
    *)         echo 0 ;;
  esac
}

epoch_to_iso() {
  date -u -d "@$1" +%FT%TZ 2>/dev/null || date -u -r "$1" +%FT%TZ 2>/dev/null
}

printf "%-25s %-10s %-22s %-10s %s\n" "TASK" "CADENCE" "LAST_RUN" "STATUS" "NEXT_DUE"
printf -- "-%.0s" {1..90}
echo

while IFS=$'\t' read -r tid cadence; do
  [[ -z "$tid" ]] && continue
  step="$(cadence_to_seconds "$cadence")"
  if [[ "$step" -eq 0 ]]; then
    printf "%-25s %-10s %-22s %-10s %s\n" "$tid" "$cadence" "INVALID-CADENCE" "ERROR" "-"
    continue
  fi

  last_epoch=0
  last_iso="never"
  if [[ -f "$LEDGER" ]]; then
    last_iso="$(jq -r --arg t "$tid" 'select(.task==$t and .outcome=="success") | .finished_at' "$LEDGER" 2>/dev/null | tail -1)"
    if [[ -n "$last_iso" && "$last_iso" != "null" ]]; then
      last_epoch="$(date -u -d "$last_iso" +%s 2>/dev/null || echo 0)"
    else
      last_iso="never"
    fi
  fi

  if [[ "$last_epoch" -eq 0 ]]; then
    status="DUE"
    next_iso="now"
  else
    next_epoch=$((last_epoch + step))
    next_iso="$(epoch_to_iso "$next_epoch")"
    if [[ "$now_epoch" -ge "$next_epoch" ]]; then
      status="DUE"
    else
      status="ok"
    fi
  fi

  printf "%-25s %-10s %-22s %-10s %s\n" "$tid" "$cadence" "$last_iso" "$status" "$next_iso"
done < <(yq -r '.tasks[] | [.id, .cadence] | @tsv' "$CAL_FILE")

exit 0
