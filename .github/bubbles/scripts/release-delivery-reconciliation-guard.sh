#!/usr/bin/env bash
# release-delivery-reconciliation-guard.sh — Gate G101.
#
# Closes the scenario-level "claimed delivered / actually skipped" hole
# (IMP-006). The per-spec anti-fabrication gates (G021/G024/G025/G028/G029/
# G097, downstream repo lints) only fire for specs that EXIST and are routed
# through validate. A release phase can promise features in
# docs/releases/<phase>/features.md that were never specced (so no per-spec
# gate ever fires) or specced-but-implement-self-certified. This guard
# reconciles the PROMISED required-feature set against the DELIVERED
# (validate-certified, terminal) spec truth.
#
# Machine binding (authored by bubbles.releases inside features.md, as
# HTML-comment annotations so the human prose tables are untouched):
#
#   <!-- bubbles:reconciled-packet schemaVersion=1 phase=mvp -->
#   <!-- bubbles:feature id=auth-real spec=specs/074-real-authentication delivery=required -->
#   <!-- bubbles:feature id=enterprise-sso spec=none delivery=deferred-to:v2.0 -->
#
#   id       — stable, unique-within-packet feature id
#   spec     — bound spec dir path, or "none" (only legal for non-required delivery)
#   delivery — required | optional | carried | deferred-to:<phase>
#
# Only delivery=required is enforced by the delivery layer.
#
# Triggers / posture:
#   * A packet is RECONCILED (blocking) when it carries a
#     `bubbles:reconciled-packet` header OR --require-coverage is passed.
#   * Otherwise it is GRANDFATHERED (WARN-only, exit 0) so existing downstream
#     packets backfill at their own pace.
#   * A RECONCILED packet that binds nothing / has malformed annotations FAILS
#     LOUD (exit 1) — a missing field must never make the gate a silent no-op.
#   * A scanned root with no docs/releases/*/features.md (the Bubbles source
#     checkout) resolves EXEMPT (exit 0), mirroring the observability gates.
#
# Usage:
#   release-delivery-reconciliation-guard.sh --repo-root <dir> [--phase <phase>] [--require-coverage]
#
# Exit codes:
#   0 = clean / grandfathered-warn / EXEMPT
#   1 = violation (missing/non-terminal/self-certified required feature, or
#       malformed reconciled packet)
#   2 = usage / runtime error
#
# There is NO --skip / --force / --ignore bypass by design.
#
# Reference: improvements/IMP-006-release-delivery-reconciliation.md

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

REPO_ROOT=""
PHASE_FILTER=""
REQUIRE_COVERAGE="false"

usage() {
  cat <<'EOF'
Usage: release-delivery-reconciliation-guard.sh --repo-root <dir> [--phase <phase>] [--require-coverage]

  --repo-root <dir>     repo to scan (required)
  --phase <phase>       reconcile only docs/releases/<phase>/features.md
  --require-coverage    treat every packet as RECONCILED (blocking) even
                        without a bubbles:reconciled-packet header — the
                        scenario/convergence path (bubbles.goal/sprint)

Exit: 0 clean/warn/exempt ; 1 violation ; 2 usage/runtime
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      REPO_ROOT="${2:-}"
      shift 2
      ;;
    --phase)
      PHASE_FILTER="${2:-}"
      shift 2
      ;;
    --require-coverage)
      REQUIRE_COVERAGE="true"
      shift
      ;;
    -h | --help)
      usage
      exit 2
      ;;
    *)
      # Tolerate a bare positional repo-root for parity with other guards.
      if [[ -z "$REPO_ROOT" && -d "$1" ]]; then
        REPO_ROOT="$1"
        shift
      else
        echo "[release-delivery-reconciliation-guard][ERROR] unknown arg: $1" >&2
        usage >&2
        exit 2
      fi
      ;;
  esac
done

[[ -n "$REPO_ROOT" ]] || {
  echo "[release-delivery-reconciliation-guard][ERROR] --repo-root is required" >&2
  exit 2
}
[[ -d "$REPO_ROOT" ]] || {
  echo "[release-delivery-reconciliation-guard][ERROR] repo root not found: $REPO_ROOT" >&2
  exit 2
}
command -v jq >/dev/null 2>&1 || {
  echo "[release-delivery-reconciliation-guard][ERROR] jq is required" >&2
  exit 2
}

REPO_ROOT_ABS="$(cd "$REPO_ROOT" && pwd -P)"
RELEASES_DIR="$REPO_ROOT_ABS/docs/releases"

# ---- collect target features.md files ----
FEATURE_FILES=()
if [[ -n "$PHASE_FILTER" ]]; then
  f="$RELEASES_DIR/$PHASE_FILTER/features.md"
  if [[ -f "$f" ]]; then
    FEATURE_FILES+=("$f")
  else
    echo "[release-delivery-reconciliation-guard][ERROR] no features.md for phase '$PHASE_FILTER' at $f" >&2
    exit 2
  fi
else
  if [[ -d "$RELEASES_DIR" ]]; then
    while IFS= read -r -d '' f; do
      FEATURE_FILES+=("$f")
    done < <(find "$RELEASES_DIR" -mindepth 2 -maxdepth 2 -type f -name features.md -print0 2>/dev/null)
  fi
fi

# EXEMPT: no release packets at all (e.g. the Bubbles source checkout).
if [[ ${#FEATURE_FILES[@]} -eq 0 ]]; then
  echo "[release-delivery-reconciliation-guard] EXEMPT (no docs/releases/<phase>/features.md found under $REPO_ROOT_ABS)"
  exit 0
fi

RUNTIME_DIR="$REPO_ROOT_ABS/.specify/runtime"

# ---- helpers ----------------------------------------------------------------

# Extract an annotation field value: ann_field "<line>" "id"
ann_field() {
  local line="$1" key="$2"
  # match key=value where value is non-space
  echo "$line" | grep -oE "${key}=[^[:space:]>]+" | head -1 | sed -E "s/^${key}=//"
}

# Is a status terminal-for-the-spec? Reuse is-terminal-for-mode.sh when present,
# else fall back to a hardcoded terminal allowlist. "done" is always terminal.
# in_progress / not_started / blocked / done_with_concerns are NEVER terminal
# for a required feature.
is_terminal_status() {
  local status="$1" mode="$2"
  case "$status" in
    done) return 0 ;;
    in_progress | not_started | blocked | done_with_concerns | "") return 1 ;;
  esac
  if [[ -n "$mode" && -x "$SCRIPT_DIR/is-terminal-for-mode.sh" ]]; then
    if bash "$SCRIPT_DIR/is-terminal-for-mode.sh" "$status" "$mode" >/dev/null 2>&1; then
      return 0
    fi
    # is-terminal-for-mode said NOT terminal (rc1) or errored (rc2); fall through
    # to the hardcoded ceiling allowlist so a parser/mode gap never hard-passes
    # a non-terminal status.
  fi
  case "$status" in
    validated | docs_updated | specs_hardened | delivered_pending_activation)
      return 0
      ;;
    *) return 1 ;;
  esac
}

# Does the spec's effective completed-phases record include "validate"?
# Tolerates both the v3 certification.certifiedCompletedPhases[] and the older
# top-level completedPhases[] shapes.
is_validate_certified() {
  local state_json="$1"
  local phases
  phases="$(jq -r '
    ((.certification.certifiedCompletedPhases // []) + (.completedPhases // []))
    | .[]? | ascii_downcase' "$state_json" 2>/dev/null || true)"
  grep -qx "validate" <<<"$phases"
}

# ---- main loop --------------------------------------------------------------

OVERALL_RC=0
SUMMARY_ROWS=()

for FFILE in "${FEATURE_FILES[@]}"; do
  phase_dir="$(basename "$(dirname "$FFILE")")"

  header_line="$(grep -oE 'bubbles:reconciled-packet[^>]*' "$FFILE" 2>/dev/null | head -1 || true)"
  reconciled="false"
  if [[ -n "$header_line" || "$REQUIRE_COVERAGE" == "true" ]]; then
    reconciled="true"
  fi

  # Collect feature annotations.
  mapfile -t ANN_LINES < <(grep -oE 'bubbles:feature[^>]*' "$FFILE" 2>/dev/null || true)

  if [[ "$reconciled" != "true" ]]; then
    echo "[release-delivery-reconciliation-guard][WARN] $phase_dir: packet is not reconciled (no bubbles:reconciled-packet header; --require-coverage not set). Skipping enforcement; backfill machine bindings to enable G101. (grandfathered, exit 0)"
    continue
  fi

  echo "[release-delivery-reconciliation-guard] reconciling phase '$phase_dir' (${#ANN_LINES[@]} feature annotation(s))"

  # Malformed: reconciled packet that binds nothing is a silent-no-op trap.
  if [[ ${#ANN_LINES[@]} -eq 0 ]]; then
    echo "[release-delivery-reconciliation-guard][ERROR] $phase_dir: RECONCILED packet declares no bubbles:feature annotations — a reconciled packet that binds nothing would make this gate a silent no-op. Add per-feature annotations or remove the reconciled-packet header." >&2
    OVERALL_RC=1
    continue
  fi

  declare -A SEEN_IDS=()
  phase_rc=0

  for line in "${ANN_LINES[@]}"; do
    fid="$(ann_field "$line" id)"
    fspec="$(ann_field "$line" spec)"
    fdelivery="$(ann_field "$line" delivery)"

    # Malformed: every annotation MUST carry id + spec + delivery.
    if [[ -z "$fid" || -z "$fspec" || -z "$fdelivery" ]]; then
      echo "[release-delivery-reconciliation-guard][ERROR] $phase_dir: malformed annotation (id/spec/delivery all required): <!-- $line -->" >&2
      phase_rc=1
      continue
    fi

    # Duplicate id.
    if [[ -n "${SEEN_IDS[$fid]:-}" ]]; then
      echo "[release-delivery-reconciliation-guard][ERROR] $phase_dir: duplicate feature id '$fid'" >&2
      phase_rc=1
      continue
    fi
    SEEN_IDS[$fid]=1

    # Validate delivery vocabulary.
    case "$fdelivery" in
      required | optional | carried) : ;;
      deferred-to:*) : ;;
      *)
        echo "[release-delivery-reconciliation-guard][ERROR] $phase_dir: feature '$fid' has invalid delivery '$fdelivery' (required|optional|carried|deferred-to:<phase>)" >&2
        phase_rc=1
        continue
        ;;
    esac

    # Only delivery=required is enforced by the delivery layer.
    if [[ "$fdelivery" != "required" ]]; then
      SUMMARY_ROWS+=("$phase_dir|$fid|$fdelivery|$fspec|NOT-ENFORCED")
      continue
    fi

    # required + spec=none is malformed.
    if [[ "$fspec" == "none" ]]; then
      echo "[release-delivery-reconciliation-guard][ERROR] $phase_dir: required feature '$fid' has spec=none (a required feature MUST bind a spec)" >&2
      phase_rc=1
      SUMMARY_ROWS+=("$phase_dir|$fid|required|none|MALFORMED")
      continue
    fi

    spec_dir="$REPO_ROOT_ABS/$fspec"
    state_json="$spec_dir/state.json"

    if [[ ! -d "$spec_dir" ]]; then
      echo "[release-delivery-reconciliation-guard][ERROR] $phase_dir: required feature '$fid' → spec dir MISSING: $fspec (promised but never specced)" >&2
      phase_rc=1
      SUMMARY_ROWS+=("$phase_dir|$fid|required|$fspec|NOT-DELIVERED (no-spec)")
      continue
    fi
    if [[ ! -f "$state_json" ]]; then
      echo "[release-delivery-reconciliation-guard][ERROR] $phase_dir: required feature '$fid' → spec '$fspec' has no state.json" >&2
      phase_rc=1
      SUMMARY_ROWS+=("$phase_dir|$fid|required|$fspec|NOT-DELIVERED (no-state)")
      continue
    fi
    if ! jq -e . "$state_json" >/dev/null 2>&1; then
      echo "[release-delivery-reconciliation-guard][ERROR] $phase_dir: required feature '$fid' → spec '$fspec' state.json is not valid JSON" >&2
      phase_rc=1
      SUMMARY_ROWS+=("$phase_dir|$fid|required|$fspec|NOT-DELIVERED (bad-state)")
      continue
    fi

    status="$(jq -r '.status // ""' "$state_json")"
    mode="$(jq -r '.workflowMode // .policySnapshot.workflowMode.mode // ""' "$state_json")"

    # Honest blocked distinction.
    if [[ "$status" == "blocked" ]]; then
      reason="$(jq -r '.blockedReason // ""' "$state_json" | tr '\n' ' ' | cut -c1-80)"
      echo "[release-delivery-reconciliation-guard][ERROR] $phase_dir: required feature '$fid' → spec '$fspec' is BLOCKED: ${reason:-<no reason recorded>}" >&2
      phase_rc=1
      SUMMARY_ROWS+=("$phase_dir|$fid|required|$fspec|NOT-DELIVERED (blocked)")
      continue
    fi

    if ! is_terminal_status "$status" "$mode"; then
      echo "[release-delivery-reconciliation-guard][ERROR] $phase_dir: required feature '$fid' → spec '$fspec' status '$status' is NOT terminal (mode '$mode')" >&2
      phase_rc=1
      SUMMARY_ROWS+=("$phase_dir|$fid|required|$fspec|NOT-DELIVERED ($status)")
      continue
    fi

    if ! is_validate_certified "$state_json"; then
      echo "[release-delivery-reconciliation-guard][ERROR] $phase_dir: required feature '$fid' → spec '$fspec' status '$status' but 'validate' is absent from completed phases — implement self-certification, not validate-certified" >&2
      phase_rc=1
      SUMMARY_ROWS+=("$phase_dir|$fid|required|$fspec|NOT-DELIVERED (self-certified)")
      continue
    fi

    SUMMARY_ROWS+=("$phase_dir|$fid|required|$fspec|DELIVERED")
  done

  unset SEEN_IDS
  if [[ "$phase_rc" -ne 0 ]]; then
    OVERALL_RC=1
  fi
done

# ---- durable runtime summary (ledger; never the committed tree) -------------
if [[ ${#SUMMARY_ROWS[@]} -gt 0 ]]; then
  mkdir -p "$RUNTIME_DIR" 2>/dev/null || true
  if [[ -d "$RUNTIME_DIR" ]]; then
    ledger_phase="${PHASE_FILTER:-all}"
    out="$RUNTIME_DIR/release-reconciliation-${ledger_phase}.json"
    {
      echo "{"
      echo "  \"generatedAt\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\","
      echo "  \"overallResult\": \"$([[ "$OVERALL_RC" -eq 0 ]] && echo pass || echo fail)\","
      echo "  \"features\": ["
      first=1
      for row in "${SUMMARY_ROWS[@]}"; do
        IFS='|' read -r rphase rid rdelivery rspec rstatus <<<"$row"
        [[ $first -eq 1 ]] && first=0 || echo ","
        printf '    {"phase":"%s","id":"%s","delivery":"%s","spec":"%s","status":"%s"}' \
          "$rphase" "$rid" "$rdelivery" "$rspec" "$rstatus"
      done
      echo ""
      echo "  ]"
      echo "}"
    } >"$out" 2>/dev/null || true
    [[ -f "$out" ]] && echo "[release-delivery-reconciliation-guard] wrote runtime summary: ${out#"$REPO_ROOT_ABS"/}"
  fi

  echo ""
  echo "Required-feature delivery reconciliation:"
  printf '  %-10s %-28s %-12s %s\n' "PHASE" "FEATURE" "DELIVERY" "STATUS"
  for row in "${SUMMARY_ROWS[@]}"; do
    IFS='|' read -r rphase rid rdelivery rspec rstatus <<<"$row"
    printf '  %-10s %-28s %-12s %s\n' "$rphase" "$rid" "$rdelivery" "$rstatus"
  done
fi

if [[ "$OVERALL_RC" -ne 0 ]]; then
  echo "" >&2
  echo "[release-delivery-reconciliation-guard][ERROR] G101: one or more REQUIRED features are not delivered (validate-certified + terminal). A release phase cannot be reported delivered while required features are missing, non-terminal, blocked, or implement-self-certified." >&2
  exit 1
fi

echo "[release-delivery-reconciliation-guard] OK (G101: all required features delivered + validate-certified)"
exit 0
