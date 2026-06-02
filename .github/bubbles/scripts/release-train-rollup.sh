#!/usr/bin/env bash
# release-train-rollup.sh — multi-train status rollup.
#
# Read-only summary of every train declared in config/release-trains.yaml:
#   - id
#   - phase (active|maintained|frozen|retired)
#   - target_slot (prod|staging|home-lab|none)
#   - flags_bundle (path)
#   - retention (per-train or via defaults)
#   - pii (per-train or via defaults)
#   - open_flag_count (specs with `releaseTrain==<id>` and non-empty `flagsIntroduced`)
#
# Owned by bubbles.train (DVS), invoked as `status --all-trains`.
# Never mutates any file.
#
# Exit 0 = report printed. Exit 0 with informational message when no trains file exists.
# Exit 1 only on hard tooling failure (missing yq).

set -euo pipefail

REPO_ROOT="${1:-.}"
TRAINS_FILE="$REPO_ROOT/config/release-trains.yaml"
SPECS_DIR="$REPO_ROOT/specs"

if [[ ! -f "$TRAINS_FILE" ]]; then
  echo "[release-train-rollup] No config/release-trains.yaml found. (no trains declared)"
  exit 0
fi

command -v yq >/dev/null 2>&1 || { echo "[release-train-rollup][ERROR] yq required" >&2; exit 1; }

DEFAULT_RETENTION="$(yq -r '.defaults.retention // ""' "$TRAINS_FILE")"
DEFAULT_PII="$(yq -r '.defaults.pii // ""' "$TRAINS_FILE")"

# Collect train ids
TRAIN_IDS="$(yq -r '.trains[].id' "$TRAINS_FILE" 2>/dev/null || echo "")"
if [[ -z "$TRAIN_IDS" ]]; then
  echo "[release-train-rollup] config/release-trains.yaml declares no trains. (no rows)"
  exit 0
fi

# Build per-train open-flag count (from specs state.json files)
declare -A OPEN_FLAG_COUNT
if [[ -d "$SPECS_DIR" ]]; then
  while IFS= read -r state_file; do
    train="$(yq -r '.releaseTrain // ""' "$state_file" 2>/dev/null || echo "")"
    [[ -z "$train" || "$train" == "null" ]] && continue
    fcount="$(yq -r '.flagsIntroduced // [] | length' "$state_file" 2>/dev/null || echo "0")"
    [[ "$fcount" -eq 0 ]] && continue
    OPEN_FLAG_COUNT[$train]=$(( ${OPEN_FLAG_COUNT[$train]:-0} + fcount ))
  done < <(find "$SPECS_DIR" -name state.json -type f 2>/dev/null)
fi

# Header
printf "## Release-train rollup (%s)\n\n" "$(date -u +%FT%TZ)"
printf "| %-20s | %-12s | %-12s | %-40s | %-25s | %-20s | %-10s |\n" \
  "TRAIN" "PHASE" "SLOT" "FLAG_BUNDLE" "RETENTION" "PII" "OPEN_FLAGS"
printf "| %s | %s | %s | %s | %s | %s | %s |\n" \
  "$(printf -- '-%.0s' {1..20})" \
  "$(printf -- '-%.0s' {1..12})" \
  "$(printf -- '-%.0s' {1..12})" \
  "$(printf -- '-%.0s' {1..40})" \
  "$(printf -- '-%.0s' {1..25})" \
  "$(printf -- '-%.0s' {1..20})" \
  "$(printf -- '-%.0s' {1..10})"

for tid in $TRAIN_IDS; do
  phase="$(yq -r ".trains[] | select(.id==\"$tid\") | .phase // \"\"" "$TRAINS_FILE")"
  slot="$(yq -r ".trains[] | select(.id==\"$tid\") | .target_slot // \"\"" "$TRAINS_FILE")"
  bundle="$(yq -r ".trains[] | select(.id==\"$tid\") | .flags_bundle // \"\"" "$TRAINS_FILE")"
  retention="$(yq -r ".trains[] | select(.id==\"$tid\") | .retention // \"\"" "$TRAINS_FILE")"
  pii="$(yq -r ".trains[] | select(.id==\"$tid\") | .pii // \"\"" "$TRAINS_FILE")"
  [[ -z "$retention" || "$retention" == "null" ]] && retention="$DEFAULT_RETENTION"
  [[ -z "$pii" || "$pii" == "null" ]] && pii="$DEFAULT_PII"
  flags="${OPEN_FLAG_COUNT[$tid]:-0}"

  printf "| %-20s | %-12s | %-12s | %-40s | %-25s | %-20s | %-10s |\n" \
    "$tid" "$phase" "$slot" "$bundle" "$retention" "$pii" "$flags"
done

echo
echo "[release-train-rollup] $(echo "$TRAIN_IDS" | wc -l) train(s) reported (read-only)."
exit 0
