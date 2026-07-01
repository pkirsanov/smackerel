#!/usr/bin/env bash
set -euo pipefail

# observability-opt-out-guard.sh
#
# Gate G099 — observability_opt_out_freshness_gate.
#
# When `traceContracts.observability.posture: opted-out`, this guard enforces
# that the opt-out is RECORDED and EXPIRING, then keeps an opt-in reminder
# alive once the committed `revisitAfter` date has passed.
#
#   * REQUIRED fields when opted-out: `optOut.reasonCode`, `optOut.reason`,
#     `optOut.revisitAfter`. Missing ANY of them = malformed = fail loud
#     (exit 1). A missing `revisitAfter` would make THIS guard a silent no-op,
#     so absence is what makes the freshness gate non-silent.
#   * `revisitAfter` is AUTHORITATIVE. It is NOT re-derived from `reasonCode`
#     (reasonCode only seeded the setup-proposed default; once written,
#     revisitAfter wins).
#   * When the committed `revisitAfter` is in the PAST, emit a route-required
#     opt-in reminder. This is a NON-BLOCKING WARN by default (exit 0) — an
#     expired opt-out escalates a reminder, it does not hard-fail a build.
#
# Unsupported `schemaVersion` fails loud (exit 1) BEFORE any opt-out semantics
# (INV-13). The Bubbles framework SOURCE checkout is auto-exempt (no runtime to
# monitor) and is a clean no-op. `yq` is the parser dependency; when it is
# missing this WARN-level guard WARN-and-skips (exit 0).
#
# There is NO bypass flag. `--skip` / `--force` / `--ignore` do not exist.
#
# Exit codes:
#   0  posture is not opted-out (no-op), OR opted-out & fresh, OR opted-out &
#      expired (reminder emitted; non-blocking), OR framework-source EXEMPT,
#      OR yq missing (WARN-and-skip).
#   1  blocking: opted-out with a missing/empty required optOut field
#      (reasonCode/reason/revisitAfter), unparseable revisitAfter, unsupported
#      schemaVersion, OR malformed YAML.
#   2  usage error (unknown flag / too many positionals / bad invocation).
#
# Usage:
#   bash bubbles/scripts/observability-opt-out-guard.sh [--repo-root <dir>] [--quiet]
#   bash bubbles/scripts/observability-opt-out-guard.sh <dir>   # positional repo root
#
# Reference: improvements/IMP-001-observability-first-class.md (SCOPE-2, T2.2)

SUPPORTED_SCHEMA_VERSION="1"

# --- Resolve own location WITHOUT external tools (see G098 for rationale) --
SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"

QUIET="false"
REPO_ROOT_ARG=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/observability-opt-out-guard.sh [options] [<repo-root>]

Gate G099 — observability_opt_out_freshness_gate.
When traceContracts.observability.posture: opted-out, requires a well-formed,
expiring optOut block and keeps an opt-in reminder alive once the committed
revisitAfter date has passed.

Options:
  --repo-root <dir>  Repo root to scan (default: the repo this guard lives in).
  <repo-root>        Same as --repo-root, positional.
  --quiet            Suppress informational stdout (warnings/errors still emit).
  -h, --help         Print this usage and exit 0.

Exit codes:
  0 = not opted-out (no-op) / opted-out+fresh / opted-out+expired (reminder,
      non-blocking) / EXEMPT / yq missing (WARN-and-skip)
  1 = missing required optOut field / unparseable revisitAfter /
      unsupported schemaVersion / malformed YAML
  2 = usage error

revisitAfter is authoritative (never re-derived from reasonCode). There is NO
--skip/--force/--ignore bypass.
EOF
}

# --- Argument parsing (builtins only) ------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --quiet)
      QUIET="true"
      shift
      ;;
    --repo-root)
      shift
      if [[ $# -eq 0 ]]; then
        echo "observability-opt-out-guard: --repo-root requires a directory argument" >&2
        usage >&2
        exit 2
      fi
      REPO_ROOT_ARG="$1"
      shift
      ;;
    --*)
      echo "observability-opt-out-guard: unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -z "$REPO_ROOT_ARG" ]]; then
        REPO_ROOT_ARG="$1"
      else
        echo "observability-opt-out-guard: unexpected positional argument: $1" >&2
        usage >&2
        exit 2
      fi
      shift
      ;;
  esac
done

info() { [[ "$QUIET" == "true" ]] || echo "observability-opt-out-guard: $*"; }
warn() { echo "observability-opt-out-guard: $*" >&2; }
err()  { echo "observability-opt-out-guard: $*" >&2; }

# --- Repo-root resolution (builtins only) --------------------------------
resolve_repo_root() {
  if [[ -n "$REPO_ROOT_ARG" ]]; then
    ( cd "$REPO_ROOT_ARG" 2>/dev/null && pwd ) || printf '%s' "$REPO_ROOT_ARG"
    return 0
  fi
  if [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
    printf '%s' "$BUBBLES_REPO_ROOT"
    return 0
  fi
  if [[ "$SCRIPT_DIR" == */.github/bubbles/scripts ]]; then
    ( cd "$SCRIPT_DIR/../../.." 2>/dev/null && pwd )
  else
    ( cd "$SCRIPT_DIR/../.." 2>/dev/null && pwd )
  fi
}

REPO_ROOT_RESOLVED="$(resolve_repo_root)"

# --- Framework SOURCE detection (mirrors cli.sh is_framework_repo intent) --
repo_is_framework_source() {
  local root="$1"
  [[ -f "$root/VERSION" \
     && -f "$root/install.sh" \
     && -d "$root/bubbles/scripts" \
     && ! -d "$root/.github/bubbles/scripts" ]]
}

locate_config() {
  local root="$1"
  if [[ -f "$root/.github/bubbles-project.yaml" ]]; then
    printf '%s' "$root/.github/bubbles-project.yaml"
  elif [[ -f "$root/bubbles-project.yaml" ]]; then
    printf '%s' "$root/bubbles-project.yaml"
  else
    printf '%s' ''
  fi
}

# --- 1. Framework-source exemption (builtins only) -----------------------
if repo_is_framework_source "$REPO_ROOT_RESOLVED"; then
  info "Observability opt-out freshness: EXEMPT (no-runtime) — Bubbles framework source repo. (G099 no-op)"
  exit 0
fi

# --- 2. Parser dependency (builtin check; WARN-and-skip if missing) ------
if ! command -v yq >/dev/null 2>&1; then
  warn "G099 WARN-and-skip: yq (mikefarah v4) not found in PATH; cannot resolve opt-out freshness. Install yq to enable the reminder. (non-blocking)"
  exit 0
fi

# --- 3. Config file -------------------------------------------------------
CFG="$(locate_config "$REPO_ROOT_RESOLVED")"
if [[ -z "$CFG" ]]; then
  info "No bubbles-project.yaml found; no observability opt-out to evaluate. (G099 no-op)"
  exit 0
fi

if ! yq '.' "$CFG" >/dev/null 2>&1; then
  err "G099: project config is not valid YAML ($CFG); cannot resolve opt-out freshness. Fix the YAML first."
  exit 1
fi

# --- 4. Observability block present? -------------------------------------
OBS_PRESENT="$(yq '.traceContracts.observability != null' "$CFG" 2>/dev/null || printf 'false')"
if [[ "$OBS_PRESENT" != "true" ]]; then
  info "No traceContracts.observability block; nothing opted-out. (G099 no-op)"
  exit 0
fi

# --- 5. Schema version enforced BEFORE semantics (INV-13) ----------------
SCHEMA_VERSION="$(yq '.traceContracts.observability.schemaVersion // ""' "$CFG" 2>/dev/null || printf '')"
if [[ "$SCHEMA_VERSION" != "$SUPPORTED_SCHEMA_VERSION" ]]; then
  err "G099 (observability_opt_out_freshness_gate): unsupported traceContracts.observability.schemaVersion '${SCHEMA_VERSION:-<absent>}' (supported: ${SUPPORTED_SCHEMA_VERSION}). Failing loud BEFORE opt-out semantics (INV-13)."
  exit 1
fi

# --- 6. Only opted-out repos are in scope --------------------------------
POSTURE="$(yq '.traceContracts.observability.posture // ""' "$CFG" 2>/dev/null || printf '')"
if [[ "$POSTURE" != "opted-out" ]]; then
  info "Observability posture is '${POSTURE:-undeclared}', not opted-out; freshness gate is a no-op. (G099 OK)"
  exit 0
fi

# --- 7. Required optOut fields (missing any = malformed = fail loud) ------
OPTOUT_PRESENT="$(yq '.traceContracts.observability.optOut != null' "$CFG" 2>/dev/null || printf 'false')"
if [[ "$OPTOUT_PRESENT" != "true" ]]; then
  err "G099 (observability_opt_out_freshness_gate): posture: opted-out but the required optOut block is absent (malformed). A missing optOut/revisitAfter would make this freshness gate a silent no-op."
  exit 1
fi

REASON_CODE="$(yq '.traceContracts.observability.optOut.reasonCode // ""' "$CFG" 2>/dev/null || printf '')"
REASON="$(yq '.traceContracts.observability.optOut.reason // ""' "$CFG" 2>/dev/null || printf '')"
REVISIT_AFTER="$(yq '.traceContracts.observability.optOut.revisitAfter // ""' "$CFG" 2>/dev/null || printf '')"

MISSING=()
[[ -z "$REASON_CODE" ]] && MISSING+=("reasonCode")
[[ -z "$REASON" ]] && MISSING+=("reason")
[[ -z "$REVISIT_AFTER" ]] && MISSING+=("revisitAfter")
if [[ ${#MISSING[@]} -gt 0 ]]; then
  err "G099 (observability_opt_out_freshness_gate): opted-out optOut block is malformed — missing required field(s): ${MISSING[*]}. All of {reasonCode, reason, revisitAfter} are REQUIRED; a missing revisitAfter would make this guard a silent no-op."
  exit 1
fi

# --- 8. Freshness: revisitAfter is authoritative -------------------------
REVISIT_EPOCH="$(date -d "$REVISIT_AFTER" +%s 2>/dev/null || date -u -j -f "%Y-%m-%d %H:%M:%S" "${REVISIT_AFTER} 00:00:00" +%s 2>/dev/null || printf '')"
if [[ -z "$REVISIT_EPOCH" ]]; then
  err "G099 (observability_opt_out_freshness_gate): optOut.revisitAfter '$REVISIT_AFTER' is not a parseable date (expected YYYY-MM-DD)."
  exit 1
fi
TODAY_EPOCH="$(date +%s)"

if [[ "$REVISIT_EPOCH" -lt "$TODAY_EPOCH" ]]; then
  warn "G099 (observability_opt_out_freshness_gate): OPT-OUT EXPIRED — committed revisitAfter '$REVISIT_AFTER' is in the past (reasonCode: $REASON_CODE). route-required: revisit the observability opt-out via /bubbles.setup focus: observability — has monitoring become available? (WARN, non-blocking by default.)"
  exit 0
fi

info "Observability opt-out is FRESH — revisitAfter '$REVISIT_AFTER' is in the future (reasonCode: $REASON_CODE). (G099 OK)"
exit 0
