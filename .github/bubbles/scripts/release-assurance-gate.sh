#!/usr/bin/env bash
# Release Assurance Gate (IMP-100 Phase 3 choke point #3)
# ---------------------------------------------------------------------------
# The deploy-gating payoff of the assurance vertical: for every certified spec
# that carries a recorded achieved assurance level (`certification.assurance.level`,
# written by bubbles.validate at certification), verify that level is
# DEPLOY-ELIGIBLE for the release train it targets — i.e. it meets the train's
# minimum assurance floor — by consuming the shared decision in
# assurance-resolve.sh. A `fast`-assured feature can never ship on a train that
# requires `full`; a `prototype` can never ship at all.
#
# Per-train floor: `config/release-trains.yaml` `.trains[].minimum_assurance`
# (∈ {full, fast}); DEFAULT `fast` when unset (backward-compatible — no existing
# train config must change). A spec's optional `.riskClass` is forwarded so a
# high/unknown-risk feature is escalated to `full` by assurance-resolve.sh
# (defense in depth).
#
# BACKWARD-COMPATIBLE + fail-open on absence, fail-closed on a real breach:
#   - no config/release-trains.yaml            → no-op (exit 0)
#   - yq not installed                         → WARN-and-skip (exit 0)
#   - a spec with no certification.assurance   → skipped (the vast majority)
#   - a spec with no releaseTrain              → skipped (release-train-guard owns that)
# Exit 0 = clean / not-applicable. Exit 1 = an under-assured certified feature
# targets a train it cannot deploy to (REFUSED). No --skip/--force flag.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVE="$SCRIPT_DIR/assurance-resolve.sh"
REPO_ROOT="${1:-.}"
TRAINS_FILE="$REPO_ROOT/config/release-trains.yaml"
SPECS_DIR="$REPO_ROOT/specs"
FAILED=0

err() { echo "[release-assurance-gate][ERROR] $*" >&2; FAILED=1; }
info() { echo "[release-assurance-gate] $*"; }

if [[ ! -f "$TRAINS_FILE" ]]; then
  info "no config/release-trains.yaml at $TRAINS_FILE — no-op."
  exit 0
fi
if ! command -v yq >/dev/null 2>&1; then
  echo "[release-assurance-gate] WARN-and-skip — yq not installed; cannot parse $TRAINS_FILE (exit 0)." >&2
  exit 0
fi
if [[ ! -x "$RESOLVE" ]]; then
  err "assurance-resolve.sh not found/executable at $RESOLVE"
  exit 1
fi

checked=0
if [[ -d "$SPECS_DIR" ]]; then
  while IFS= read -r state_file; do
    level="$(yq -r '(.certification.assurance.level // "")' "$state_file" 2>/dev/null || echo "")"
    [[ -z "$level" || "$level" == "null" ]] && continue

    train="$(yq -r '(.releaseTrain // "")' "$state_file" 2>/dev/null || echo "")"
    [[ -z "$train" || "$train" == "null" ]] && continue

    # Train's minimum assurance floor (default fast when unset).
    train_min="$(yq -r ".trains[] | select(.id==\"$train\") | .minimum_assurance // \"fast\"" "$TRAINS_FILE" 2>/dev/null || echo "")"
    [[ -z "$train_min" || "$train_min" == "null" ]] && train_min="fast"
    case "$train_min" in
      full | fast) ;;
      *)
        err "train '$train' has invalid minimum_assurance '$train_min' (expected full|fast) — referenced by $(dirname "$state_file")"
        continue
        ;;
    esac

    risk="$(yq -r '(.riskClass // "")' "$state_file" 2>/dev/null || echo "")"
    [[ "$risk" == "null" ]] && risk=""

    # Consult the shared deploy-eligibility decision (never re-derive here).
    resolve_out=""
    if [[ -n "$risk" ]]; then
      resolve_out="$(bash "$RESOLVE" --achieved-level "$level" --minimum-assurance "$train_min" --risk-class "$risk" 2>/dev/null || true)"
    else
      resolve_out="$(bash "$RESOLVE" --achieved-level "$level" --minimum-assurance "$train_min" 2>/dev/null || true)"
    fi
    eligible="$(printf '%s\n' "$resolve_out" | sed -n 's/^deployEligible=//p')"
    reason="$(printf '%s\n' "$resolve_out" | sed -n 's/^reason=//p')"

    checked=$((checked + 1))
    if [[ "$eligible" != "true" ]]; then
      err "spec $(dirname "$state_file") achieved assurance '$level' is NOT deployable on train '$train' (floor '$train_min'${risk:+, riskClass=$risk}): ${reason:-below required assurance}"
    fi
  done < <(find "$SPECS_DIR" -name state.json -type f 2>/dev/null)
fi

if [[ "$FAILED" -ne 0 ]]; then
  err "release-assurance-gate FAILED"
  exit 1
fi

info "release-assurance-gate PASSED ($checked certified feature(s) with a recorded assurance level checked against their train floor)."
exit 0
