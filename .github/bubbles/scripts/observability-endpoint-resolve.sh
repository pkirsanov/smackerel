#!/usr/bin/env bash
set -euo pipefail

# observability-endpoint-resolve.sh
#
# Plane-aware observability endpoint resolver (IMP-001 SCOPE-3, T3.3/T3.7/T3.8).
#
# Maps a (plane, signal) pair to the configured provider adapter + profile by
# reading `traceContracts.observability.endpoints.<plane>.<signal>` from a
# repo's `bubbles-project.yaml` (or `.github/bubbles-project.yaml`), and
# materializes the PLANE-SCOPED input env into the adapter's NATIVE env so the
# caller can invoke `bubbles/adapters/observability/<adapter>.sh` directly.
#
# Two planes, one provider adapter, a profile per plane (INV-3/INV-11):
#   --plane validate  → resolves to the EPHEMERAL per-run test stack; reads
#                       ONLY `endpoints.validate.*` and `BUBBLES_OBS_VALIDATE_*`
#                       env. It NEVER reads operate-plane config or env, even
#                       when prod env vars are present (T3.7 prod-block).
#   --plane operate   → resolves to PROD; reads `endpoints.operate.*` and
#                       `BUBBLES_OBS_OPERATE_*` env.
#
# Adapter names are PROVIDER names (INV-11), never environment names. The
# profile (`test`/`prod`) only selects which plane-scoped env prefix is read.
#
# Profile-to-env binding (provider `prometheus`):
#   <PREFIX>PROMETHEUS_BASE_URL          → PROMETHEUS_BASE_URL          (required)
#   <PREFIX>PROMETHEUS_CURL_MAX_TIME     → PROMETHEUS_CURL_MAX_TIME     (required)
#   <PREFIX>PROMETHEUS_BEARER_TOKEN      → PROMETHEUS_BEARER_TOKEN      (optional)
#   <PREFIX>PROMETHEUS_QUERY_SLO_BURN    → PROMETHEUS_QUERY_SLO_BURN    (sloBurn)
#   <PREFIX>PROMETHEUS_QUERY_ERROR_RATE  → PROMETHEUS_QUERY_ERROR_RATE  (errorRate)
#   <PREFIX>PROMETHEUS_QUERY_DEPLOY_IMPACT→ PROMETHEUS_QUERY_DEPLOY_IMPACT (deployImpact)
# where <PREFIX> = BUBBLES_OBS_VALIDATE_ | BUBBLES_OBS_OPERATE_ per plane.
# Missing a REQUIRED profile var fails LOUD (exit 1) — never silently defaulted.
#
# Parser dependency: `yq` (mikefarah v4). Like the posture guard, a missing yq
# WARN-and-skips (exit 0) emitting the neutral `adapter=none` resolution — a
# missing developer tool must not hard-fail a resolution helper.
#
# There is NO bypass flag. `--skip` / `--force` / `--ignore` do not exist.
#
# Output (stdout) on success — eval-friendly `KEY=VALUE` lines:
#   adapter=<provider|none>
#   profile=<test|prod|>            (empty for adapter=none)
#   <NATIVE_ENV_KEY>=<value>        (one per materialized adapter-native var)
#
# `--names-only` reports just `adapter=`/`profile=` and exits 0 WITHOUT reading
# or requiring any plane-scoped secret env — the read-only wiring query used by
# observability-check.sh (the check_observability MCP twin) to report which
# adapter is wired per (plane, signal) without holding the plane's secrets.
#
# Exit codes:
#   0  resolved (incl. adapter=none and yq-missing WARN-and-skip)
#   1  resolution/validation failure (missing required profile env, malformed
#      YAML, unsupported schemaVersion, unknown signal/plane in config)
#   2  usage error (unknown flag / missing required option / bad value)
#
# Reference: improvements/IMP-001-observability-first-class.md (SCOPE-3, T3.3)

SUPPORTED_SCHEMA_VERSION="1"

SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"

PLANE=""
SIGNAL=""
REPO_ROOT_ARG=""
NAMES_ONLY="false"

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/observability-endpoint-resolve.sh --plane <plane> --signal <signal> [--repo-root <dir>]

Resolve (plane, signal) → { adapter, profile } from a repo's bubbles-project.yaml
traceContracts.observability.endpoints, and materialize the plane-scoped env
into the adapter's native env.

Options:
  --plane <validate|operate>   REQUIRED. validate = ephemeral test stack;
                               operate = prod (read-only ops scopes only).
  --signal <alerts|sloBurn|errorRate|deployImpact>   REQUIRED telemetry signal.
  --repo-root <dir>            Repo root to scan (default: the repo this script
                               lives in).
  --names-only                 Report `adapter=`/`profile=` ONLY and exit 0
                               without materializing or requiring any
                               plane-scoped secret env. Read-only wiring query
                               for health checks (e.g. observability-check.sh);
                               never touches BUBBLES_OBS_*_ env.
  -h, --help                   Print this usage and exit 0.

Exit codes:
  0 = resolved (incl. adapter=none / yq-missing WARN-and-skip)
  1 = missing required profile env / malformed config / unsupported schema
  2 = usage error

There is NO --skip/--force/--ignore bypass. A --plane validate resolution NEVER
reads operate-plane config or BUBBLES_OBS_OPERATE_* env (prod-block).
EOF
}

# --- Argument parsing (builtins only) ------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --plane)
      shift
      [[ $# -gt 0 ]] || { echo "observability-endpoint-resolve: --plane requires a value" >&2; usage >&2; exit 2; }
      PLANE="$1"
      shift
      ;;
    --signal)
      shift
      [[ $# -gt 0 ]] || { echo "observability-endpoint-resolve: --signal requires a value" >&2; usage >&2; exit 2; }
      SIGNAL="$1"
      shift
      ;;
    --repo-root)
      shift
      [[ $# -gt 0 ]] || { echo "observability-endpoint-resolve: --repo-root requires a directory argument" >&2; usage >&2; exit 2; }
      REPO_ROOT_ARG="$1"
      shift
      ;;
    --names-only)
      NAMES_ONLY="true"
      shift
      ;;
    --*)
      echo "observability-endpoint-resolve: unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      echo "observability-endpoint-resolve: unexpected positional argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

warn() { echo "observability-endpoint-resolve: $*" >&2; }
err()  { echo "observability-endpoint-resolve: $*" >&2; }

case "$PLANE" in
  validate) PLANE_PREFIX="BUBBLES_OBS_VALIDATE_" ;;
  operate)  PLANE_PREFIX="BUBBLES_OBS_OPERATE_" ;;
  "") err "--plane is required (validate|operate)"; usage >&2; exit 2 ;;
  *)  err "invalid --plane '$PLANE' (expected validate|operate)"; usage >&2; exit 2 ;;
esac

case "$SIGNAL" in
  alerts|sloBurn|errorRate|deployImpact) : ;;
  "") err "--signal is required (alerts|sloBurn|errorRate|deployImpact)"; usage >&2; exit 2 ;;
  *)  err "invalid --signal '$SIGNAL' (expected alerts|sloBurn|errorRate|deployImpact)"; usage >&2; exit 2 ;;
esac

# --- Repo-root resolution (mirrors the posture guard) --------------------
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

emit_none() {
  printf 'adapter=none\n'
  printf 'profile=\n'
}

# --- Missing parser → WARN-and-skip (neutral none resolution) ------------
if ! command -v yq >/dev/null 2>&1; then
  warn "yq (mikefarah v4) not found in PATH; cannot resolve observability endpoints. Emitting neutral adapter=none. Install yq to enable resolution. (non-blocking)"
  emit_none
  exit 0
fi

CONFIG="$(locate_config "$REPO_ROOT_RESOLVED")"
if [[ -z "$CONFIG" ]]; then
  # No project config → no telemetry configured → neutral none.
  emit_none
  exit 0
fi

if ! yq '.' "$CONFIG" >/dev/null 2>&1; then
  err "project config is not valid YAML: $CONFIG"
  exit 1
fi

OBS_PRESENT="$(yq '.traceContracts.observability != null' "$CONFIG" 2>/dev/null || printf 'false')"
if [[ "$OBS_PRESENT" != "true" ]]; then
  # No observability block → neutral none (undeclared posture is the posture
  # guard's concern; the resolver simply has nothing to resolve).
  emit_none
  exit 0
fi

# Schema version is enforced before semantics (INV-13).
SCHEMA_VERSION="$(yq '.traceContracts.observability.schemaVersion // ""' "$CONFIG" 2>/dev/null || printf '')"
if [[ "$SCHEMA_VERSION" != "$SUPPORTED_SCHEMA_VERSION" ]]; then
  err "unsupported traceContracts.observability.schemaVersion '${SCHEMA_VERSION:-<absent>}' (supported: ${SUPPORTED_SCHEMA_VERSION}). Failing loud before resolution (INV-13)."
  exit 1
fi

ADAPTER="$(yq ".traceContracts.observability.endpoints.${PLANE}.${SIGNAL}.adapter // \"\"" "$CONFIG" 2>/dev/null || printf '')"
PROFILE="$(yq ".traceContracts.observability.endpoints.${PLANE}.${SIGNAL}.profile // \"\"" "$CONFIG" 2>/dev/null || printf '')"

# Unconfigured signal OR explicit none → neutral none resolution.
if [[ -z "$ADAPTER" || "$ADAPTER" == "none" || "$ADAPTER" == "null" ]]; then
  emit_none
  exit 0
fi

# --- Names-only read-only query: report wiring, never touch secret env ----
# A health-check consumer (observability-check.sh) needs to know WHICH adapter
# is wired for a (plane, signal) without holding the plane's secrets. Emit the
# resolved adapter+profile and exit 0 BEFORE any env materialization, so this
# path never reads or requires BUBBLES_OBS_*_ env and can never fail-loud on a
# missing secret.
if [[ "$NAMES_ONLY" == "true" ]]; then
  printf 'adapter=%s\n' "$ADAPTER"
  printf 'profile=%s\n' "$PROFILE"
  exit 0
fi

# --- Provider env materialization ----------------------------------------
# Reads ONLY the plane-scoped prefix (PLANE_PREFIX). A validate resolution can
# therefore never read BUBBLES_OBS_OPERATE_* env (prod-block, T3.7).
materialize_var() {
  # $1 = plane-scoped input var (without prefix), $2 = adapter-native var,
  # $3 = required(1)/optional(0)
  local in_suffix="$1" native="$2" required="$3"
  local in_name="${PLANE_PREFIX}${in_suffix}"
  local value="${!in_name:-}"
  if [[ -z "$value" ]]; then
    if [[ "$required" == "1" ]]; then
      err "missing required profile env '${in_name}' for plane=${PLANE} signal=${SIGNAL} adapter=${ADAPTER} (profile=${PROFILE:-<none>}). Set it in the ${PLANE}-plane env. Failing loud."
      return 1
    fi
    return 0
  fi
  printf '%s=%s\n' "$native" "$value"
  return 0
}

case "$ADAPTER" in
  prometheus)
    printf 'adapter=%s\n' "$ADAPTER"
    printf 'profile=%s\n' "$PROFILE"
    materialize_var "PROMETHEUS_BASE_URL"      "PROMETHEUS_BASE_URL"      1 || exit 1
    materialize_var "PROMETHEUS_CURL_MAX_TIME" "PROMETHEUS_CURL_MAX_TIME" 1 || exit 1
    materialize_var "PROMETHEUS_BEARER_TOKEN"  "PROMETHEUS_BEARER_TOKEN"  0 || exit 1
    case "$SIGNAL" in
      sloBurn)      materialize_var "PROMETHEUS_QUERY_SLO_BURN"      "PROMETHEUS_QUERY_SLO_BURN"      1 || exit 1 ;;
      errorRate)    materialize_var "PROMETHEUS_QUERY_ERROR_RATE"    "PROMETHEUS_QUERY_ERROR_RATE"    1 || exit 1 ;;
      deployImpact) materialize_var "PROMETHEUS_QUERY_DEPLOY_IMPACT" "PROMETHEUS_QUERY_DEPLOY_IMPACT" 1 || exit 1 ;;
      alerts)       : ;;  # no query var for alerts
    esac
    exit 0
    ;;
  *)
    # Unknown provider: emit adapter+profile so the caller can act, but the
    # resolver does not know this provider's native env mapping.
    printf 'adapter=%s\n' "$ADAPTER"
    printf 'profile=%s\n' "$PROFILE"
    warn "no env-materialization mapping for provider '$ADAPTER'; the caller must set the adapter's native env itself."
    exit 0
    ;;
esac
