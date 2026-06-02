#!/usr/bin/env bash
# release-train-backfill-planner.sh — advisory/dry-run script to identify
# specs/state.json files missing `releaseTrain` and propose assignments.
#
# Reads `config/release-trains.yaml` for valid train ids, scans specs/,
# inspects `git log -- <spec-dir>` to infer the most likely owning train
# from recency + spec status + filename prefix.
#
# This is ADVISORY ONLY — it prints a table; it does NOT mutate any file.
# Operator reviews and applies decisions manually (or via a paired commit
# referencing the script's output).
#
# Heuristic:
#   - If state.json status is `done` and last commit > 90 days old → suggest 'mvp' (or the operator's oldest train)
#   - If status is `in_progress` and committed within 30 days → suggest the train with target_slot='staging' (most likely 'next')
#   - If status is `specs_hardened` → suggest the train with target_slot='staging'
#   - If unclear → emit `NEEDS_REVIEW`

set -euo pipefail

REPO_ROOT="${1:-.}"
TRAINS_FILE="$REPO_ROOT/config/release-trains.yaml"

[[ -f "$TRAINS_FILE" ]] || { echo "[backfill-planner] $TRAINS_FILE not found"; exit 0; }
command -v yq >/dev/null 2>&1 || { echo "[backfill-planner] yq required" >&2; exit 1; }
command -v git >/dev/null 2>&1 || { echo "[backfill-planner] git required" >&2; exit 1; }

# Resolve candidate trains by slot.
OLDEST_PROD_TRAIN="$(yq -r '[.trains[] | select(.target_slot == "prod" or .target_slot == "home-lab")] | .[0].id // ""' "$TRAINS_FILE")"
STAGING_TRAIN="$(yq -r '[.trains[] | select(.target_slot == "staging")] | .[0].id // ""' "$TRAINS_FILE")"

[[ -n "$OLDEST_PROD_TRAIN" ]] || OLDEST_PROD_TRAIN="$(yq -r '.trains[0].id' "$TRAINS_FILE")"
[[ -n "$STAGING_TRAIN" ]] || STAGING_TRAIN="$OLDEST_PROD_TRAIN"

cd "$REPO_ROOT"

printf "%-60s %-15s %-12s %s\n" "SPEC" "STATUS" "AGE_DAYS" "SUGGESTED_TRAIN"
printf -- "-%.0s" {1..110}; echo

missing=0
backfilled=0
needs_review=0

while IFS= read -r state_file; do
  spec_dir="$(dirname "$state_file")"
  rel_spec="${spec_dir#"$REPO_ROOT/"}"

  status="$(yq -r '.status // ""' "$state_file" 2>/dev/null)"
  train="$(yq -r '.releaseTrain // ""' "$state_file" 2>/dev/null)"
  [[ -n "$train" && "$train" != "null" ]] && continue

  # Skip statuses where releaseTrain is not required (per guard policy).
  case "$status" in
    in_progress|done|specs_hardened|delivered_pending_activation|train_cut|train_promoted) ;;
    *) continue ;;
  esac

  missing=$((missing + 1))

  # Age of most recent commit touching the spec dir.
  last_commit_epoch="$(git log -1 --format=%ct -- "$rel_spec" 2>/dev/null || echo 0)"
  if [[ "$last_commit_epoch" -gt 0 ]]; then
    age_days=$(( ( $(date +%s) - last_commit_epoch ) / 86400 ))
  else
    age_days="-"
  fi

  # Suggestion heuristic.
  case "$status" in
    done|specs_hardened|delivered_pending_activation)
      if [[ "$age_days" != "-" && "$age_days" -gt 90 ]]; then
        suggested="$OLDEST_PROD_TRAIN"
      else
        suggested="$STAGING_TRAIN"
      fi
      ;;
    in_progress|train_cut|train_promoted)
      suggested="$STAGING_TRAIN"
      ;;
    *)
      suggested="NEEDS_REVIEW"
      ;;
  esac

  if [[ "$suggested" == "NEEDS_REVIEW" ]]; then
    needs_review=$((needs_review + 1))
  else
    backfilled=$((backfilled + 1))
  fi

  printf "%-60s %-15s %-12s %s\n" "${rel_spec:0:60}" "$status" "$age_days" "$suggested"
done < <(find specs -name state.json -type f 2>/dev/null)

echo
echo "Summary: $missing specs missing releaseTrain; $backfilled have heuristic suggestion; $needs_review need manual review"
echo
echo "To apply suggestions: review each row, then run e.g."
echo "  yq -i '.releaseTrain = \"<train>\"' <spec-dir>/state.json"
echo
echo "Or batch-apply per train (read carefully before committing):"
echo "  $0 \"$REPO_ROOT\" | awk '\$4 == \"$OLDEST_PROD_TRAIN\" {print \$1}' | \\"
echo "    xargs -I{} yq -i '.releaseTrain = \"$OLDEST_PROD_TRAIN\"' {}/state.json"
exit 0
