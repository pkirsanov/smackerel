#!/usr/bin/env bash
set -euo pipefail

# adversarial-resolve.sh
#
# Adversarial-verification posture resolver (IMP-002 SCOPE-0 / control plane, C8).
#
# Resolves the EFFECTIVE adversarial posture (mode / passes / teeth) for a run
# from one layered precedence chain (highest wins), mirroring the shipped
# observability-endpoint-resolve.sh and the dual-trust resolver:
#
#   1. Per-run directive   (--mode/--passes/--teeth, or --directive "<string>")
#   2. Environment         (BUBBLES_ADVERSARIAL / _PASSES / _TEETH)
#   3. Project config      (.github/bubbles-project.yaml `adversarial:` block)
#   4. Framework default   (mode=off, passes=1, teeth=warn)
#
# The capability is OFF BY DEFAULT (mode=off): a zero-config repo resolves to
# `off` and adversarial verification does nothing — zero behavior change on
# upgrade. A team opts in seamlessly: session/CI via BUBBLES_ADVERSARIAL=auto,
# repo-wide via the `adversarial:` config block, or for a single agent/workflow
# run via the directive layer.
#
# Vocabulary:
#   mode   = off  → never run; auto → run only on high-risk scopes (riskClass
#                   gated); on → run for this scope regardless of risk.
#   passes = N    → number of independent validators for the voting ensemble
#                   (1 = single red-team pass; >=2 = voting). Positive integer.
#   teeth  = warn → a red-team counterexample is a recorded finding (default,
#                   grandfathered like G101); blocking → it blocks certification.
#
# Parser dependency: `yq` (mikefarah v4) for the config layer ONLY. A missing yq
# WARN-and-skips the config layer (stderr note, resolution continues) — a missing
# developer tool must not hard-fail a resolution helper. Directive + env + the
# off-by-default still resolve without yq.
#
# There is NO bypass flag. `--skip` / `--force` / `--ignore` do not exist.
#
# Output (stdout) — eval-friendly KEY=VALUE lines:
#   mode=<off|auto|on>
#   passes=<N>
#   teeth=<warn|blocking>
#   source=<directive|env|config|default>   (which layer set `mode`)
#
# Exit codes:
#   0  resolved (incl. mode=off and yq-missing config WARN-and-skip)
#   1  validation failure (invalid mode/passes/teeth)
#   2  usage error (unknown flag / missing required option value)
#
# Reference: improvements/IMP-002-adversarial-verification.md (SCOPE-0, C8)

DEFAULT_MODE="off"
DEFAULT_PASSES="1"
DEFAULT_TEETH="warn"

SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"

REPO_ROOT_ARG=""
DIR_MODE=""
DIR_PASSES=""
DIR_TEETH=""
DIRECTIVE_STR=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/adversarial-resolve.sh [--mode <m>] [--passes <n>] [--teeth <t>] [--directive "<str>"] [--repo-root <dir>]

Resolve the effective adversarial posture (mode/passes/teeth) from the precedence
chain: per-run directive -> BUBBLES_ADVERSARIAL* env -> bubbles-project.yaml
`adversarial:` block -> framework default (off).

Options:
  --mode <off|auto|on>      Per-run mode (directive layer, highest precedence).
  --passes <N>              Per-run validator count (positive integer).
  --teeth <warn|blocking>   Per-run teeth.
  --directive "<str>"       Free-form per-run string; mode:/passes:/teeth: tokens
                            are extracted (what an orchestrator forwards from
                            $ADDITIONAL_CONTEXT, e.g. "adversarial: on passes: 3").
                            Explicit --mode/--passes/--teeth override the same
                            token inside --directive.
  --repo-root <dir>         Repo root to scan for bubbles-project.yaml
                            (default: the repo this script lives in).
  -h, --help                Print this usage and exit 0.

Exit codes: 0 resolved (incl. off / yq-missing) | 1 invalid value | 2 usage.
There is NO --skip/--force/--ignore bypass. OFF BY DEFAULT.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --mode)
      shift
      [[ $# -gt 0 ]] || { echo "adversarial-resolve: --mode requires a value" >&2; exit 2; }
      DIR_MODE="$1"
      shift
      ;;
    --passes)
      shift
      [[ $# -gt 0 ]] || { echo "adversarial-resolve: --passes requires a value" >&2; exit 2; }
      DIR_PASSES="$1"
      shift
      ;;
    --teeth)
      shift
      [[ $# -gt 0 ]] || { echo "adversarial-resolve: --teeth requires a value" >&2; exit 2; }
      DIR_TEETH="$1"
      shift
      ;;
    --directive)
      shift
      [[ $# -gt 0 ]] || { echo "adversarial-resolve: --directive requires a value" >&2; exit 2; }
      DIRECTIVE_STR="$1"
      shift
      ;;
    --repo-root)
      shift
      [[ $# -gt 0 ]] || { echo "adversarial-resolve: --repo-root requires a value" >&2; exit 2; }
      REPO_ROOT_ARG="$1"
      shift
      ;;
    *)
      echo "adversarial-resolve: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

# Extract a token (mode|passes|teeth) value from the free-form directive string.
directive_token() {
  local key="$1"
  [[ -n "$DIRECTIVE_STR" ]] || return 0
  printf '%s\n' "$DIRECTIVE_STR" \
    | grep -oiE "(${key})[[:space:]]*[:=][[:space:]]*[A-Za-z0-9]+" \
    | head -n1 \
    | sed -E 's/.*[:=][[:space:]]*//' \
    | tr '[:upper:]' '[:lower:]' \
    || true
}

# Resolve repo root.
REPO_ROOT="${REPO_ROOT_ARG:-}"
if [[ -z "$REPO_ROOT" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." 2>/dev/null && pwd)"
fi

# --- Config layer: adversarial.{mode,passes,teeth} from bubbles-project.yaml ---
CFG_MODE=""
CFG_PASSES=""
CFG_TEETH=""
config_file=""
for c in "$REPO_ROOT/.github/bubbles-project.yaml" "$REPO_ROOT/bubbles-project.yaml"; do
  if [[ -f "$c" ]]; then
    config_file="$c"
    break
  fi
done
if [[ -n "$config_file" ]]; then
  if command -v yq >/dev/null 2>&1; then
    CFG_MODE="$(yq '.adversarial.mode' "$config_file" 2>/dev/null || true)"
    CFG_PASSES="$(yq '.adversarial.passes' "$config_file" 2>/dev/null || true)"
    CFG_TEETH="$(yq '.adversarial.teeth' "$config_file" 2>/dev/null || true)"
    if [[ "$CFG_MODE" == "null" ]]; then CFG_MODE=""; fi
    if [[ "$CFG_PASSES" == "null" ]]; then CFG_PASSES=""; fi
    if [[ "$CFG_TEETH" == "null" ]]; then CFG_TEETH=""; fi
  else
    echo "adversarial-resolve: yq not found — skipping config layer (directive/env/default still apply)" >&2
  fi
fi

# --- Directive layer (explicit flags override --directive tokens) ---
if [[ -z "$DIR_MODE" ]]; then DIR_MODE="$(directive_token 'adversarial|mode')"; fi
if [[ -z "$DIR_PASSES" ]]; then DIR_PASSES="$(directive_token 'passes')"; fi
if [[ -z "$DIR_TEETH" ]]; then DIR_TEETH="$(directive_token 'teeth')"; fi

# --- Env layer ---
ENV_MODE="${BUBBLES_ADVERSARIAL:-}"
ENV_PASSES="${BUBBLES_ADVERSARIAL_PASSES:-}"
ENV_TEETH="${BUBBLES_ADVERSARIAL_TEETH:-}"

# --- Precedence resolution: directive > env > config > default ---
# Echoes "value|source".
resolve_layer() {
  local directive="$1" env_val="$2" cfg="$3" def="$4"
  if [[ -n "$directive" ]]; then printf '%s|directive\n' "$directive"; return; fi
  if [[ -n "$env_val" ]]; then printf '%s|env\n' "$env_val"; return; fi
  if [[ -n "$cfg" ]]; then printf '%s|config\n' "$cfg"; return; fi
  printf '%s|default\n' "$def"
}

mode_r="$(resolve_layer "$DIR_MODE" "$ENV_MODE" "$CFG_MODE" "$DEFAULT_MODE")"
passes_r="$(resolve_layer "$DIR_PASSES" "$ENV_PASSES" "$CFG_PASSES" "$DEFAULT_PASSES")"
teeth_r="$(resolve_layer "$DIR_TEETH" "$ENV_TEETH" "$CFG_TEETH" "$DEFAULT_TEETH")"

MODE="${mode_r%%|*}"
MODE_SRC="${mode_r##*|}"
PASSES="${passes_r%%|*}"
TEETH="${teeth_r%%|*}"

MODE="$(printf '%s' "$MODE" | tr '[:upper:]' '[:lower:]')"
TEETH="$(printf '%s' "$TEETH" | tr '[:upper:]' '[:lower:]')"

# --- Validate ---
case "$MODE" in
  off|auto|on) ;;
  *) echo "adversarial-resolve: invalid mode '$MODE' (expected off|auto|on)" >&2; exit 1 ;;
esac
case "$TEETH" in
  warn|blocking) ;;
  *) echo "adversarial-resolve: invalid teeth '$TEETH' (expected warn|blocking)" >&2; exit 1 ;;
esac
if ! printf '%s' "$PASSES" | grep -qE '^[1-9][0-9]*$'; then
  echo "adversarial-resolve: invalid passes '$PASSES' (expected positive integer)" >&2
  exit 1
fi

printf 'mode=%s\n' "$MODE"
printf 'passes=%s\n' "$PASSES"
printf 'teeth=%s\n' "$TEETH"
printf 'source=%s\n' "$MODE_SRC"
