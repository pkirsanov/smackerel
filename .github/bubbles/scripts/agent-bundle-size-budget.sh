#!/usr/bin/env bash
# agent-bundle-size-budget.sh — ratcheting PER-AGENT effective-bundle size budget
# (IMP-102 / SCOPE-10, COST).
#
# instruction-budget-lint budgets INDIVIDUAL agent files; effective-bundle-budget.sh
# enforces ONE optional GLOBAL ceiling shared by every agent. This guard adds the
# missing middle: a PER-AGENT ratcheting ceiling. Each agent's EFFECTIVE bundle —
# its .agent.md PLUS the transitive closure of the agents/bubbles_shared/*.md
# contracts it imports, exactly as effective-bundle-measure.sh computes it — is
# compared against a committed ceiling recorded in bubbles/agent-bundle-budgets.json
# ( { "<agent>.agent.md": <ceilingBytes>, ... } ). Bloat can grow a bundle a byte
# at a time and never trip a single global ceiling; a per-agent recorded ceiling
# catches the creep on the exact agent that grew.
#
# Modes:
#   --check (DEFAULT)  FAIL (exit 1) if ANY agent's effective size EXCEEDS its
#                      recorded ceiling, OR if an agent has NO recorded ceiling
#                      (a new agent MUST get one — run --seed). Green when every
#                      agent is at or under its ceiling. No --skip/--force bypass.
#   --seed             (Re)write ceilings = ceil(currentBytes * (1 + headroom%)).
#                      RATCHETS DOWN: when an agent SHRINKS, its recorded ceiling
#                      drops with it so reclaimed budget can't be silently refilled.
#                      It NEVER silently RAISES a ceiling on a breach — an agent
#                      whose current size already EXCEEDS its recorded ceiling is
#                      refused (exit 2) unless --accept-growth is passed, which is
#                      the explicit operator act of accepting the larger bundle.
#
# The budget is a ratchet, not a bypass: --seed can only tighten (or, with an
# explicit --accept-growth, deliberately loosen) — there is no flag that lets a
# breach pass --check.
#
# Usage:
#   agent-bundle-size-budget.sh [--check | --seed] [--accept-growth]
#     [--repo-root DIR] [--agents-dir DIR] [--budget-file PATH] [--headroom-pct N]
#
# Exit codes:
#   0  within budget (--check) / budget written (--seed) / nothing to measure
#   1  breach or missing ceiling (--check)
#   2  usage error, or a --seed breach without --accept-growth
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MEASURE="$SCRIPT_DIR/effective-bundle-measure.sh"

MODE="check"
ACCEPT_GROWTH="false"
REPO_ROOT=""
AGENTS_DIR=""
BUDGET_FILE=""
HEADROOM_PCT="${BUBBLES_BUNDLE_BUDGET_HEADROOM_PCT:-5}"

err() { echo "[agent-bundle-size-budget][ERROR] $*" >&2; }
info() { echo "[agent-bundle-size-budget] $*"; }

usage() {
  cat <<'EOF'
Usage: agent-bundle-size-budget.sh [--check | --seed] [--accept-growth]
         [--repo-root DIR] [--agents-dir DIR] [--budget-file PATH] [--headroom-pct N]

  --check         (default) fail if any agent exceeds its recorded ceiling or has none
  --seed          rewrite ceilings from current sizes (+headroom); ratchets DOWN only
  --accept-growth with --seed, permit raising a ceiling for an agent that breached
  --repo-root DIR repo root holding agents/ and bubbles/agent-bundle-budgets.json
  --agents-dir DIR override the agents directory to measure
  --budget-file PATH override the budget JSON path
  --headroom-pct N seed headroom percentage (default 5, or $BUBBLES_BUNDLE_BUDGET_HEADROOM_PCT)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check) MODE="check"; shift ;;
    --seed | --update) MODE="seed"; shift ;;
    --accept-growth) ACCEPT_GROWTH="true"; shift ;;
    --repo-root) [[ $# -ge 2 ]] || { err "--repo-root requires a value"; exit 2; }; REPO_ROOT="$2"; shift 2 ;;
    --agents-dir) [[ $# -ge 2 ]] || { err "--agents-dir requires a value"; exit 2; }; AGENTS_DIR="$2"; shift 2 ;;
    --budget-file) [[ $# -ge 2 ]] || { err "--budget-file requires a value"; exit 2; }; BUDGET_FILE="$2"; shift 2 ;;
    --headroom-pct) [[ $# -ge 2 ]] || { err "--headroom-pct requires a value"; exit 2; }; HEADROOM_PCT="$2"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    *) err "unknown argument: $1"; usage >&2; exit 2 ;;
  esac
done

if ! [[ "$HEADROOM_PCT" =~ ^[0-9]+$ ]]; then
  err "--headroom-pct must be a non-negative integer (got '$HEADROOM_PCT')"
  exit 2
fi

# ── Resolve repo root (source layout: bubbles/scripts; downstream: .github/bubbles/scripts) ──
if [[ -z "$REPO_ROOT" ]]; then
  if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
    REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
  else
    REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
  fi
fi
[[ -d "$REPO_ROOT" ]] || { err "repo root not found: $REPO_ROOT"; exit 2; }
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"

# Agents dir default: prefer top-level agents/ (source), else .github/agents (downstream).
if [[ -z "$AGENTS_DIR" ]]; then
  if [[ -d "$REPO_ROOT/agents" ]]; then
    AGENTS_DIR="$REPO_ROOT/agents"
  else
    AGENTS_DIR="$REPO_ROOT/.github/agents"
  fi
fi

# Budget file default: prefer bubbles/ (source), else .github/bubbles/ (downstream).
if [[ -z "$BUDGET_FILE" ]]; then
  if [[ -d "$REPO_ROOT/bubbles" ]]; then
    BUDGET_FILE="$REPO_ROOT/bubbles/agent-bundle-budgets.json"
  else
    BUDGET_FILE="$REPO_ROOT/.github/bubbles/agent-bundle-budgets.json"
  fi
fi

# ── Graceful skips (never a false failure) ──
if ! command -v jq >/dev/null 2>&1; then
  info "jq not available — skipping (measurement requires jq)"
  exit 0
fi
if [[ ! -x "$MEASURE" ]]; then
  info "effective-bundle-measure.sh not available — skipping"
  exit 0
fi
if [[ ! -d "$AGENTS_DIR" ]]; then
  info "no agents directory at $AGENTS_DIR — skipping"
  exit 0
fi

# Deterministic agent list.
mapfile -t agent_files < <(find "$AGENTS_DIR" -maxdepth 1 -name '*.agent.md' 2>/dev/null | LC_ALL=C sort)
if [[ "${#agent_files[@]}" -eq 0 ]]; then
  info "no *.agent.md files under $AGENTS_DIR — skipping"
  exit 0
fi

# ceil(bytes * (100 + headroom) / 100) with integer math.
ceil_with_headroom() {
  local bytes="$1"
  echo $(((bytes * (100 + HEADROOM_PCT) + 99) / 100))
}

measure_bytes() {
  local agent_file="$1"
  bash "$MEASURE" "$agent_file" --agents-dir "$AGENTS_DIR" 2>/dev/null \
    | jq -r '.totalBytes' 2>/dev/null
}

if [[ "$MODE" == "seed" ]]; then
  existing_json='{}'
  if [[ -f "$BUDGET_FILE" ]]; then
    existing_json="$(cat "$BUDGET_FILE")"
    if ! printf '%s' "$existing_json" | jq empty >/dev/null 2>&1; then
      err "existing budget file is not valid JSON: $BUDGET_FILE"
      exit 2
    fi
  fi

  new_json='{}'
  refused=0
  for agent_file in "${agent_files[@]}"; do
    base="$(basename "$agent_file")"
    bytes="$(measure_bytes "$agent_file")"
    if ! [[ "$bytes" =~ ^[0-9]+$ ]]; then
      err "could not measure effective size for $base"
      exit 2
    fi
    proposed="$(ceil_with_headroom "$bytes")"
    existing="$(printf '%s' "$existing_json" | jq -r --arg k "$base" '.[$k] // empty')"

    if [[ -z "$existing" ]]; then
      chosen="$proposed"
      info "seed-new     $base: ceiling=$chosen (size=$bytes)"
    elif [[ "$proposed" -le "$existing" ]]; then
      chosen="$proposed"
      if [[ "$proposed" -lt "$existing" ]]; then
        info "ratchet-down $base: ceiling $existing -> $chosen (size=$bytes)"
      else
        info "unchanged    $base: ceiling=$chosen (size=$bytes)"
      fi
    elif [[ "$bytes" -le "$existing" ]]; then
      # Grew within headroom but still under the recorded ceiling: HOLD, do not raise.
      chosen="$existing"
      info "hold         $base: ceiling=$existing (size=$bytes; not raising)"
    else
      # Real breach: current size exceeds the recorded ceiling.
      if [[ "$ACCEPT_GROWTH" == "true" ]]; then
        chosen="$proposed"
        info "accept-growth $base: ceiling $existing -> $chosen (size=$bytes)"
      else
        err "breach $base: size $bytes exceeds recorded ceiling $existing — re-run --seed --accept-growth to accept the larger bundle"
        refused=$((refused + 1))
        chosen="$existing"
      fi
    fi
    new_json="$(printf '%s' "$new_json" | jq --arg k "$base" --argjson v "$chosen" '.[$k] = $v')"
  done

  if [[ "$refused" -gt 0 ]]; then
    err "$refused agent(s) breached their ceiling; budget NOT written (no silent raise). Use --accept-growth to accept."
    exit 2
  fi

  tmp="$(mktemp)"
  printf '%s\n' "$new_json" | jq -S '.' > "$tmp"
  mkdir -p "$(dirname "$BUDGET_FILE")"
  mv "$tmp" "$BUDGET_FILE"
  info "OK — wrote ${#agent_files[@]} ceiling(s) to $BUDGET_FILE (headroom ${HEADROOM_PCT}%)"
  exit 0
fi

# ── MODE=check ──
if [[ ! -f "$BUDGET_FILE" ]]; then
  err "budget file not found: $BUDGET_FILE (run --seed to create it)"
  exit 1
fi
if ! jq empty "$BUDGET_FILE" >/dev/null 2>&1; then
  err "budget file is not valid JSON: $BUDGET_FILE"
  exit 1
fi

budget_json="$(cat "$BUDGET_FILE")"
missing=()
breached=()
checked=0
for agent_file in "${agent_files[@]}"; do
  base="$(basename "$agent_file")"
  bytes="$(measure_bytes "$agent_file")"
  if ! [[ "$bytes" =~ ^[0-9]+$ ]]; then
    err "could not measure effective size for $base"
    exit 1
  fi
  ceiling="$(printf '%s' "$budget_json" | jq -r --arg k "$base" '.[$k] // empty')"
  if [[ -z "$ceiling" ]]; then
    missing+=("$base (size=$bytes)")
    continue
  fi
  checked=$((checked + 1))
  if [[ "$bytes" -gt "$ceiling" ]]; then
    breached+=("$base: effective $bytes bytes exceeds ceiling $ceiling bytes (+$((bytes - ceiling)))")
  fi
done

status=0
if [[ "${#missing[@]}" -gt 0 ]]; then
  status=1
  for m in "${missing[@]}"; do
    err "no recorded ceiling for $m — a new agent must get a ceiling (run --seed)"
  done
fi
if [[ "${#breached[@]}" -gt 0 ]]; then
  status=1
  for b in "${breached[@]}"; do
    err "$b"
  done
fi

if [[ "$status" -eq 0 ]]; then
  info "OK — all ${checked} agent bundle(s) within their recorded ceilings ($BUDGET_FILE)"
else
  err "agent bundle-size budget breached — regenerate with --seed only after verifying the growth is intentional"
fi
exit "$status"
