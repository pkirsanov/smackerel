#!/usr/bin/env bash
set -euo pipefail

# observability-posture-guard.sh
#
# Gate G098 — observability_posture_declared_gate.
#
# Makes the observability posture decision FIRST-CLASS: every repo MUST
# declare a posture (`wired` or `opted-out`) under
# `traceContracts.observability.posture` in `bubbles-project.yaml` (or
# `.github/bubbles-project.yaml`). An UNDECLARED posture is the only
# "uncomfortable" state — it emits a WARN nag and exits 0 by default, and is
# flipped to a hard block only when the project sets
# `traceContracts.observability.policy.undeclaredPosture: block`.
#
# The guard ALSO fails loud (exit 1) on three malformed/unsafe shapes, so the
# posture contract is never silently mis-parsed:
#   * fake-wired — `posture: wired` but EVERY validate+operate signal is
#     `adapter: none` (no usable evidence path; INV-14).
#   * opted-out-malformed — `posture: opted-out` with NO `optOut` block (a
#     missing optOut/revisitAfter would make the G099 freshness guard a silent
#     no-op).
#   * unsupported schemaVersion — checked BEFORE any posture semantics
#     (INV-13); an unknown breaking schema version must never be mis-parsed.
#
# The Bubbles framework SOURCE checkout is auto-exempt: it has no runtime to
# monitor, so it resolves permanently to EXEMPT (no-runtime) with NO nag. This
# mirrors the intent of `is_framework_repo()` in `bubbles/scripts/cli.sh`
# (≈ line 181), replicated here and keyed to the SCANNED repo root so the same
# binary stays correct whether it scans its own tree or a fixture.
#
# Parser dependency: `yq` (mikefarah v4). This is a WARN-level gate, so when
# yq is NOT installed the guard WARN-and-skips (exit 0) rather than hard-error
# — a missing developer tool must never block a non-blocking nag.
#
# There is NO bypass flag. `--skip` / `--force` / `--ignore` do not exist and
# never will; the only project-facing knob is the declared
# `policy.undeclaredPosture` value.
#
# Exit codes:
#   0  posture declared & well-formed (wired / opted-out), OR undeclared with
#      policy=warn (nag printed), OR framework-source EXEMPT, OR yq missing
#      (WARN-and-skip).
#   1  blocking: fake-wired, opted-out-malformed, unsupported schemaVersion,
#      invalid posture value, malformed YAML, OR undeclared with
#      policy.undeclaredPosture=block.
#   2  usage error (unknown flag / too many positionals / bad invocation).
#
# Usage:
#   bash bubbles/scripts/observability-posture-guard.sh [--repo-root <dir>] [--quiet]
#   bash bubbles/scripts/observability-posture-guard.sh <dir>            # positional repo root
#   bash bubbles/scripts/observability-posture-guard.sh --print-state [--repo-root <dir>]
#
# --print-state emits ONE machine-readable token to stdout and exits 0 (it is a
# read-only query used by `bubbles doctor`; it never enforces). Tokens:
#   EXEMPT | UNAVAILABLE | UNDECLARED | WIRED | FAKE-WIRED |
#   OPTED-OUT-FRESH|<date> | OPTED-OUT-EXPIRED|<date> | OPTED-OUT-INCOMPLETE |
#   OPTED-OUT-MALFORMED | UNSUPPORTED-SCHEMA|<v> | INVALID-POSTURE|<v> |
#   MALFORMED-YAML
#
# Reference: improvements/IMP-001-observability-first-class.md (SCOPE-2, T2.1)

SUPPORTED_SCHEMA_VERSION="1"

# --- Resolve own location WITHOUT external tools -------------------------
# (param-expansion + cd/pwd are bash builtins; avoiding `dirname` keeps the
# missing-parser path — which strips PATH to prove WARN-and-skip — working.)
SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"

QUIET="false"
PRINT_STATE="false"
REPO_ROOT_ARG=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/observability-posture-guard.sh [options] [<repo-root>]

Gate G098 — observability_posture_declared_gate.
Resolves traceContracts.observability.posture from <repo-root>'s
bubbles-project.yaml (or .github/bubbles-project.yaml) and enforces that the
posture decision exists and is well-formed.

Options:
  --repo-root <dir>  Repo root to scan (default: the repo this guard lives in).
  <repo-root>        Same as --repo-root, positional.
  --print-state      Emit ONE resolved state token to stdout and exit 0
                     (read-only query used by `bubbles doctor`).
  --quiet            Suppress informational PASS/EXEMPT stdout (warnings and
                     errors are still emitted).
  -h, --help         Print this usage and exit 0.

Exit codes:
  0 = declared & well-formed, OR undeclared+policy=warn (nag), OR EXEMPT,
      OR yq missing (WARN-and-skip)
  1 = fake-wired / opted-out-malformed / unsupported schemaVersion /
      invalid posture / malformed YAML / undeclared+policy=block
  2 = usage error

There is NO --skip/--force/--ignore bypass. The only project knob is
traceContracts.observability.policy.undeclaredPosture (warn|block).
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
    --print-state)
      PRINT_STATE="true"
      shift
      ;;
    --repo-root)
      shift
      if [[ $# -eq 0 ]]; then
        echo "observability-posture-guard: --repo-root requires a directory argument" >&2
        usage >&2
        exit 2
      fi
      REPO_ROOT_ARG="$1"
      shift
      ;;
    --*)
      echo "observability-posture-guard: unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -z "$REPO_ROOT_ARG" ]]; then
        REPO_ROOT_ARG="$1"
      else
        echo "observability-posture-guard: unexpected positional argument: $1" >&2
        usage >&2
        exit 2
      fi
      shift
      ;;
  esac
done

info() { [[ "$QUIET" == "true" ]] || echo "observability-posture-guard: $*"; }
warn() { echo "observability-posture-guard: $*" >&2; }
err()  { echo "observability-posture-guard: $*" >&2; }

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
  # Mirror cli.sh: scripts live at <root>/bubbles/scripts OR
  # <root>/.github/bubbles/scripts.
  if [[ "$SCRIPT_DIR" == */.github/bubbles/scripts ]]; then
    ( cd "$SCRIPT_DIR/../../.." 2>/dev/null && pwd )
  else
    ( cd "$SCRIPT_DIR/../.." 2>/dev/null && pwd )
  fi
}

REPO_ROOT_RESOLVED="$(resolve_repo_root)"

# --- Framework SOURCE detection (builtins only) --------------------------
# Replicates the INTENT of is_framework_repo() from cli.sh (≈ line 181),
# keyed to the SCANNED root: the framework SOURCE checkout keeps scripts at
# <root>/bubbles/scripts and carries the canonical VERSION + install.sh, while
# a downstream install only ever carries <root>/.github/bubbles/scripts.
repo_is_framework_source() {
  local root="$1"
  [[ -f "$root/VERSION" \
     && -f "$root/install.sh" \
     && -d "$root/bubbles/scripts" \
     && ! -d "$root/.github/bubbles/scripts" ]]
}

# --- Config file location ------------------------------------------------
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

# --- yq helpers (only reached AFTER the UNAVAILABLE short-circuit) --------
yq_get() {
  # $1 = expression, $2 = file. Returns "" on missing key via the // default
  # already embedded by callers.
  yq "$1" "$2"
}

# --- Base-state resolution -----------------------------------------------
# Echoes exactly ONE token. Pure (no exit). yq is only invoked after the
# EXEMPT and UNAVAILABLE short-circuits, so the missing-parser path needs no
# external tool.
compute_base_state() {
  if repo_is_framework_source "$REPO_ROOT_RESOLVED"; then
    printf '%s' 'EXEMPT'
    return 0
  fi

  if ! command -v yq >/dev/null 2>&1; then
    printf '%s' 'UNAVAILABLE'
    return 0
  fi

  local cfg
  cfg="$(locate_config "$REPO_ROOT_RESOLVED")"
  if [[ -z "$cfg" ]]; then
    printf '%s' 'UNDECLARED'
    return 0
  fi

  if ! yq '.' "$cfg" >/dev/null 2>&1; then
    printf '%s' 'MALFORMED-YAML'
    return 0
  fi

  local obs_present
  obs_present="$(yq '.traceContracts.observability != null' "$cfg" 2>/dev/null || printf 'false')"
  if [[ "$obs_present" != "true" ]]; then
    printf '%s' 'UNDECLARED'
    return 0
  fi

  # Schema version is enforced BEFORE any posture semantics (INV-13).
  local schema_version
  schema_version="$(yq '.traceContracts.observability.schemaVersion // ""' "$cfg" 2>/dev/null || printf '')"
  if [[ "$schema_version" != "$SUPPORTED_SCHEMA_VERSION" ]]; then
    printf '%s' "UNSUPPORTED-SCHEMA|${schema_version:-<absent>}"
    return 0
  fi

  local posture
  posture="$(yq '.traceContracts.observability.posture // ""' "$cfg" 2>/dev/null || printf '')"

  case "$posture" in
    wired)
      local non_none
      non_none="$(yq '[(.traceContracts.observability.endpoints.validate // {} | .[].adapter), (.traceContracts.observability.endpoints.operate // {} | .[].adapter)] | map(select(. != "none" and . != null)) | length' "$cfg" 2>/dev/null || printf '0')"
      if [[ "${non_none:-0}" == "0" ]]; then
        printf '%s' 'FAKE-WIRED'
      else
        printf '%s' 'WIRED'
      fi
      ;;
    opted-out)
      local optout_present
      optout_present="$(yq '.traceContracts.observability.optOut != null' "$cfg" 2>/dev/null || printf 'false')"
      if [[ "$optout_present" != "true" ]]; then
        printf '%s' 'OPTED-OUT-MALFORMED'
      else
        printf '%s' 'OPTED-OUT'
      fi
      ;;
    "")
      printf '%s' 'UNDECLARED'
      ;;
    *)
      printf '%s' "INVALID-POSTURE|${posture}"
      ;;
  esac
}

# Refine an OPTED-OUT base state into FRESH/EXPIRED/INCOMPLETE for --print-state
# (read-only display only; G098's gate path treats any well-formed opted-out as
# accepted and lets G099 own freshness + required-field enforcement).
refine_optout_for_display() {
  local cfg revisit revisit_epoch today_epoch
  cfg="$(locate_config "$REPO_ROOT_RESOLVED")"
  revisit="$(yq '.traceContracts.observability.optOut.revisitAfter // ""' "$cfg" 2>/dev/null || printf '')"
  if [[ -z "$revisit" ]]; then
    printf '%s' 'OPTED-OUT-INCOMPLETE'
    return 0
  fi
  revisit_epoch="$(date -d "$revisit" +%s 2>/dev/null || date -u -j -f "%Y-%m-%d %H:%M:%S" "${revisit} 00:00:00" +%s 2>/dev/null || printf '')"
  if [[ -z "$revisit_epoch" ]]; then
    printf '%s' 'OPTED-OUT-INCOMPLETE'
    return 0
  fi
  today_epoch="$(date +%s)"
  if [[ "$revisit_epoch" -lt "$today_epoch" ]]; then
    printf '%s' "OPTED-OUT-EXPIRED|${revisit}"
  else
    printf '%s' "OPTED-OUT-FRESH|${revisit}"
  fi
}

STATE="$(compute_base_state)"

# --- --print-state: read-only query --------------------------------------
if [[ "$PRINT_STATE" == "true" ]]; then
  if [[ "$STATE" == "OPTED-OUT" ]]; then
    refine_optout_for_display
    echo ""
  else
    echo "$STATE"
  fi
  exit 0
fi

# --- Gate mode ------------------------------------------------------------
case "$STATE" in
  EXEMPT)
    info "Observability posture: EXEMPT (no-runtime) — Bubbles framework source repo; nothing to monitor. (G098 OK)"
    exit 0
    ;;
  UNAVAILABLE)
    warn "G098 WARN-and-skip: yq (mikefarah v4) not found in PATH; cannot resolve observability posture. Install yq to enable the posture nag. (non-blocking)"
    exit 0
    ;;
  MALFORMED-YAML)
    err "G098: project config is not valid YAML; cannot resolve observability posture. Fix the YAML before posture can be validated."
    exit 1
    ;;
  WIRED)
    info "Observability posture: WIRED — at least one non-none telemetry signal declared. (G098 OK)"
    exit 0
    ;;
  FAKE-WIRED)
    err "G098 (observability_posture_declared_gate): posture: wired but EVERY validate+operate signal is 'adapter: none' (fake-wired). A wired repo MUST declare at least one usable non-none signal (INV-14). Set a real adapter or change posture to opted-out."
    exit 1
    ;;
  OPTED-OUT)
    info "Observability posture: OPTED-OUT (declared, optOut block present). Freshness + required fields enforced by G099. (G098 OK)"
    exit 0
    ;;
  OPTED-OUT-MALFORMED)
    err "G098 (observability_posture_declared_gate): posture: opted-out but the required optOut block is absent (malformed). A missing optOut/revisitAfter would make the G099 freshness guard a silent no-op. Add traceContracts.observability.optOut {reasonCode, reason, revisitAfter}."
    exit 1
    ;;
  UNSUPPORTED-SCHEMA*)
    err "G098 (observability_posture_declared_gate): unsupported traceContracts.observability.schemaVersion '${STATE#UNSUPPORTED-SCHEMA|}' (supported: ${SUPPORTED_SCHEMA_VERSION}). Failing loud BEFORE applying posture semantics (INV-13)."
    exit 1
    ;;
  INVALID-POSTURE*)
    err "G098 (observability_posture_declared_gate): invalid posture '${STATE#INVALID-POSTURE|}' (expected: wired | opted-out)."
    exit 1
    ;;
  UNDECLARED)
    # Undeclared is the only "uncomfortable" state. WARN nag by default;
    # blocking only when the project sets policy.undeclaredPosture: block.
    local_policy=""
    cfg_for_policy="$(locate_config "$REPO_ROOT_RESOLVED")"
    if [[ -n "$cfg_for_policy" ]] && command -v yq >/dev/null 2>&1; then
      local_policy="$(yq '.traceContracts.observability.policy.undeclaredPosture // ""' "$cfg_for_policy" 2>/dev/null || printf '')"
    fi
    if [[ "$local_policy" == "block" ]]; then
      err "G098 (observability_posture_declared_gate): observability posture is UNDECLARED and traceContracts.observability.policy.undeclaredPosture=block. Declare a posture (wired | opted-out) via /bubbles.setup focus: observability."
      exit 1
    fi
    warn "G098 (observability_posture_declared_gate): observability posture is UNDECLARED. Observability is a first-class decision — declare 'wired' or 'opted-out' via /bubbles.setup focus: observability. (WARN by default; non-blocking — set policy.undeclaredPosture: block to enforce.)"
    exit 0
    ;;
  *)
    err "G098: internal error — unresolved state '$STATE'."
    exit 1
    ;;
esac
