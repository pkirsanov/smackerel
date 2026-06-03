#!/usr/bin/env bash
# propagation-policy-guard.sh — validates propagation-policy.yaml schema and
# cross-references against config/release-trains.yaml.
#
# Enforces (Gates G121, G122, G123):
#   1. propagation-policy.yaml exists at repo root OR under config/
#   2. version == 1
#   3. trains[] non-empty; every id exists in config/release-trains.yaml
#   4. defaultFlow[] edges have valid from/to (referencing declared trains)
#   5. Each edge declares receivingTrainValidationMode ∈ {validate-only, full-delivery, none}
#   6. Edges with receivingTrainValidationMode=none MUST declare validationSkipReason
#   7. backportRequiresApproval is boolean
#   8. ledgerPath (if present) is a relative path (no leading /)
#   9. ledger file (if exists) is valid append-only JSONL with required fields
#  10. tracked ledger file is append-only relative to git (no removed lines)
#
# Exit 0 = clean. Exit 1 = violations. No --skip / --force / --ignore flag.

set -euo pipefail

REPO_ROOT="${1:-.}"
REQUIRE_POLICY=false
if [[ "${1:-}" == "--require-policy" ]]; then
  REQUIRE_POLICY=true
  REPO_ROOT="${2:-.}"
elif [[ "${2:-}" == "--require-policy" ]]; then
  REQUIRE_POLICY=true
fi
TRAINS_FILE="$REPO_ROOT/config/release-trains.yaml"
POLICY_FILE=""

# Resolve policy file
if [[ -f "$REPO_ROOT/propagation-policy.yaml" ]]; then
  POLICY_FILE="$REPO_ROOT/propagation-policy.yaml"
elif [[ -f "$REPO_ROOT/config/propagation-policy.yaml" ]]; then
  POLICY_FILE="$REPO_ROOT/config/propagation-policy.yaml"
fi

FAILED=0

err() { echo "[propagation-policy-guard][ERROR] $*" >&2; FAILED=1; }
info() { echo "[propagation-policy-guard] $*"; }

# A repo without policy is valid (J-Roc just refuses propagation operations).
# The guard reports informationally and exits 0 in that case so framework-validate
# does not fail on repos that don't use cross-train propagation.
if [[ -z "$POLICY_FILE" && "$REQUIRE_POLICY" == "true" ]]; then
  err "propagation-policy.yaml required for propagate-* workflows (G121)"
  exit 1
fi

if [[ -z "$POLICY_FILE" ]]; then
  info "No propagation-policy.yaml found at root or config/. This is OK — bubbles.propagate will refuse operations until policy exists. (skip)"
  exit 0
fi

if ! command -v yq >/dev/null 2>&1; then
  err "yq is required (https://github.com/mikefarah/yq)"
  exit 1
fi

# Check 2: version
VERSION="$(yq -r '.version // ""' "$POLICY_FILE")"
[[ "$VERSION" == "1" ]] || err "propagation-policy.yaml: version must be 1 (got '$VERSION')"

# Check 3: trains[] non-empty + cross-ref with release-trains.yaml
POLICY_TRAIN_IDS="$(yq -r '.trains[].id' "$POLICY_FILE" 2>/dev/null || echo "")"
if [[ -z "$POLICY_TRAIN_IDS" ]]; then
  err "propagation-policy.yaml: trains[] is empty"
fi

KNOWN_TRAIN_IDS=""
if [[ -f "$TRAINS_FILE" ]]; then
  KNOWN_TRAIN_IDS="$(yq -r '.trains[].id' "$TRAINS_FILE" 2>/dev/null || echo "")"
else
  err "config/release-trains.yaml not found; cannot cross-reference policy trains"
fi

for tid in $POLICY_TRAIN_IDS; do
  if [[ -n "$KNOWN_TRAIN_IDS" ]]; then
    found=0
    for ktid in $KNOWN_TRAIN_IDS; do
      [[ "$tid" == "$ktid" ]] && { found=1; break; }
    done
    [[ "$found" -eq 1 ]] || err "propagation-policy.yaml: train '$tid' not declared in config/release-trains.yaml"
  fi
done

# Check 4+5+6: defaultFlow edges
EDGE_COUNT="$(yq -r '.defaultFlow | length' "$POLICY_FILE")"
if [[ "$EDGE_COUNT" == "0" || "$EDGE_COUNT" == "null" || ! "$EDGE_COUNT" =~ ^[0-9]+$ ]]; then
  err "propagation-policy.yaml: defaultFlow[] is empty"
fi

if [[ "$EDGE_COUNT" =~ ^[0-9]+$ && "$EDGE_COUNT" -gt 0 ]]; then
  for i in $(seq 0 $((EDGE_COUNT - 1))); do
    from="$(yq -r ".defaultFlow[$i].from // \"\"" "$POLICY_FILE")"
    to="$(yq -r ".defaultFlow[$i].to // \"\"" "$POLICY_FILE")"
    mode="$(yq -r ".defaultFlow[$i].receivingTrainValidationMode // \"\"" "$POLICY_FILE")"
    skip_reason="$(yq -r ".defaultFlow[$i].validationSkipReason // \"\"" "$POLICY_FILE")"

    # from/to must be in policy trains
    for t in "$from" "$to"; do
      [[ -z "$t" ]] && { err "defaultFlow[$i]: from/to missing"; continue; }
      found=0
      for tid in $POLICY_TRAIN_IDS; do
        [[ "$t" == "$tid" ]] && { found=1; break; }
      done
      [[ "$found" -eq 1 ]] || err "defaultFlow[$i]: '$t' not in trains[]"
    done

    # mode is required
    case "$mode" in
      validate-only|full-delivery|none) ;;
      "") err "defaultFlow[$i]: receivingTrainValidationMode required (validate-only|full-delivery|none) (G122)" ;;
      *) err "defaultFlow[$i]: receivingTrainValidationMode invalid '$mode' (G122)" ;;
    esac

    # mode=none requires validationSkipReason
    if [[ "$mode" == "none" && ( -z "$skip_reason" || "$skip_reason" == "null" ) ]]; then
      err "defaultFlow[$i] $from→$to: receivingTrainValidationMode=none requires validationSkipReason (G122)"
    fi
  done
fi

# Check 7: backportRequiresApproval is boolean
BACKPORT_APPROVAL="$(yq -r '.backportRequiresApproval // "null"' "$POLICY_FILE")"
case "$BACKPORT_APPROVAL" in
  true|false) ;;
  null) err "propagation-policy.yaml: backportRequiresApproval is required (boolean)" ;;
  *) err "propagation-policy.yaml: backportRequiresApproval must be boolean (got '$BACKPORT_APPROVAL')" ;;
esac

# Check 8: ledgerPath
LEDGER_PATH="$(yq -r '.ledgerPath // "propagation-ledger.yaml"' "$POLICY_FILE")"
case "$LEDGER_PATH" in
  /*) err "propagation-policy.yaml: ledgerPath must be relative (got absolute '$LEDGER_PATH')" ;;
esac

# Check 9: ledger format if file exists (each line must parse as JSON)
LEDGER_FILE="$REPO_ROOT/$LEDGER_PATH"
if [[ -f "$LEDGER_FILE" ]]; then
  if ! command -v jq >/dev/null 2>&1; then
    info "jq not installed; skipping ledger JSONL validation"
  else
    line_no=0
    while IFS= read -r line; do
      line_no=$((line_no + 1))
      [[ -z "$line" ]] && continue
      if ! jq empty >/dev/null 2>&1 <<< "$line"; then
        err "propagation-ledger ($LEDGER_PATH:$line_no): invalid JSON (G123)"
        continue
      fi
      if ! jq -e '
        .timestamp and .operator and .operation and .fromTrain and .toTrain and
        (.commits | type == "array") and .validationMode and .validationOutcome and
        (if .operation == "backport" then (.approvalToken != null and .approvalToken != "") else true end)
      ' >/dev/null 2>&1 <<< "$line"; then
        err "propagation-ledger ($LEDGER_PATH:$line_no): missing required fields (G123)"
      fi
    done < "$LEDGER_FILE"

    # Best-effort append-only check when ledger is tracked in git. Any removed
    # line in working-tree or staged diff means a historical ledger entry was
    # edited or deleted instead of appending a corrective entry.
    if git -C "$REPO_ROOT" ls-files --error-unmatch "$LEDGER_PATH" >/dev/null 2>&1; then
      while IFS= read -r diff_line; do
        [[ "$diff_line" == ---* ]] && continue
        [[ "$diff_line" == -* ]] || continue
        err "propagation-ledger ($LEDGER_PATH): non-append diff detected (G123): ${diff_line#-}"
      done < <(git -C "$REPO_ROOT" diff --unified=0 -- "$LEDGER_PATH" 2>/dev/null || true)
      while IFS= read -r diff_line; do
        [[ "$diff_line" == ---* ]] && continue
        [[ "$diff_line" == -* ]] || continue
        err "propagation-ledger ($LEDGER_PATH): staged non-append diff detected (G123): ${diff_line#-}"
      done < <(git -C "$REPO_ROOT" diff --cached --unified=0 -- "$LEDGER_PATH" 2>/dev/null || true)
    fi
  fi
fi

if [[ "$FAILED" -ne 0 ]]; then
  echo "[propagation-policy-guard] FAILED" >&2
  exit 1
fi

info "OK"
exit 0
