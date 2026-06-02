#!/usr/bin/env bash
# release-train-flag-audit.sh — identifies feature flags that are overdue
# for cleanup. A flag is overdue when its owning train is in `frozen` or
# `retired` phase. The "flag dies + 1 cycle" rule means flags should be
# cleaned up during the train's `frozen` phase (the grace cycle) and MUST
# be gone before `retired`.
#
# Output: list of (flag, train, phase, owner-spec) lines.
# Exit 0 always; this is an audit, not a gate.

set -euo pipefail

REPO_ROOT="${1:-.}"
TRAINS_FILE="$REPO_ROOT/config/release-trains.yaml"
SPECS_DIR="$REPO_ROOT/specs"

if [[ ! -f "$TRAINS_FILE" ]]; then
  echo "[flag-audit] config/release-trains.yaml not found; skipping"
  exit 0
fi

command -v yq >/dev/null 2>&1 || { echo "[flag-audit] yq required" >&2; exit 1; }

declare -A train_phase
while IFS=$'\t' read -r tid phase; do
  train_phase[$tid]="$phase"
done < <(yq -r '.trains[] | [.id, .phase] | @tsv' "$TRAINS_FILE")

echo "## Flag cleanup audit ($(date -u +%FT%TZ))"
echo
printf "%-30s %-20s %-15s %s\n" "FLAG" "TRAIN" "PHASE" "OWNER_SPEC"
printf -- "-%.0s" {1..90}
echo

overdue_count=0
retired_with_live_flags=0

while IFS= read -r state_file; do
  spec_dir="$(dirname "$state_file")"
  train="$(yq -r '.releaseTrain // ""' "$state_file" 2>/dev/null || echo "")"
  [[ -z "$train" || "$train" == "null" ]] && continue

  flags="$(yq -r '.flagsIntroduced[]? // ""' "$state_file" 2>/dev/null || echo "")"
  [[ -z "$flags" ]] && continue

  phase="${train_phase[$train]:-unknown}"

  # frozen = grace cycle (overdue, should clean up).
  # retired = MUST not have live flags (this is a policy violation).
  case "$phase" in
    frozen)
      for flag in $flags; do
        printf "%-30s %-20s %-15s %s\n" "$flag" "$train" "$phase (grace)" "$spec_dir"
        overdue_count=$((overdue_count + 1))
      done
      ;;
    retired)
      for flag in $flags; do
        printf "%-30s %-20s %-15s %s\n" "$flag" "$train" "$phase (VIOLATION)" "$spec_dir"
        overdue_count=$((overdue_count + 1))
        retired_with_live_flags=$((retired_with_live_flags + 1))
      done
      ;;
  esac
done < <(find "$SPECS_DIR" -name state.json -type f 2>/dev/null)

echo
echo "Overdue flags: $overdue_count"
if [[ "$retired_with_live_flags" -gt 0 ]]; then
  echo "VIOLATION: $retired_with_live_flags flag(s) still live on retired train(s) — cleanup is overdue"
fi
exit 0
