#!/usr/bin/env bash
# intent-routes-lint.sh — validates bubbles/intent-routes.yaml.
#
# Enforces:
#   1. File exists and parses as YAML
#   2. version == 1
#   3. routes[] non-empty
#   4. Every route has phrases[] (non-empty), targetAgent, targetMode, summary
#   5. Every phrase is lowercase
#   6. No duplicate phrases across all routes
#   7. Every targetAgent exists in bubbles/agent-capabilities.yaml under agents:
#   8. Every targetMode exists as a key under workflows: in bubbles/workflows.yaml
#
# Exit 0 = clean. Exit 1 = violations. No --skip / --force flag.

set -euo pipefail

REPO_ROOT="${1:-.}"
ROUTES_FILE="$REPO_ROOT/bubbles/intent-routes.yaml"
CAPS_FILE="$REPO_ROOT/bubbles/agent-capabilities.yaml"
WORKFLOWS_FILE="$REPO_ROOT/bubbles/workflows.yaml"
# v6.1 (S2 true split): mode definitions live in their own registry; fall back
# to workflows.yaml for pre-split repos that still embed an inline modes: block.
MODES_FILE="$REPO_ROOT/bubbles/workflows/modes.yaml"
[[ -f "$MODES_FILE" ]] || MODES_FILE="$WORKFLOWS_FILE"

FAILED=0
err() { echo "[intent-routes-lint][ERROR] $*" >&2; FAILED=1; }
info() { echo "[intent-routes-lint] $*"; }

[[ -f "$ROUTES_FILE" ]] || { err "$ROUTES_FILE not found"; exit 1; }
command -v yq >/dev/null 2>&1 || { err "yq is required"; exit 1; }

# Single yq call: dump everything as TSV-like records to avoid N*per-call overhead.
DUMP="$(yq -r '
  "META\t" + (.version | tostring),
  (.routes // [] | to_entries[] |
    "ROUTE\t" + (.key | tostring) + "\t" + (.value.targetAgent // "") + "\t" + (.value.targetMode // "") + "\t" + (.value.summary // "")),
  (.routes // [] | to_entries[] as $r |
    ($r.value.phrases // [])[] |
    "PHRASE\t" + ($r.key | tostring) + "\t" + .)
' "$ROUTES_FILE" 2>/dev/null || echo "")"

VERSION=""
declare -a ROUTE_AGENT ROUTE_MODE ROUTE_SUMMARY
declare -a PHRASE_TEXT PHRASE_ROUTE_IDX
route_count=0
phrase_count=0

while IFS=$'\t' read -r kind rest1 rest2 rest3 rest4; do
  case "$kind" in
    META)
      VERSION="$rest1"
      ;;
    ROUTE)
      idx="$rest1"
      ROUTE_AGENT[$idx]="$rest2"
      ROUTE_MODE[$idx]="$rest3"
      ROUTE_SUMMARY[$idx]="$rest4"
      route_count=$((route_count + 1))
      ;;
    PHRASE)
      PHRASE_ROUTE_IDX[$phrase_count]="$rest1"
      PHRASE_TEXT[$phrase_count]="$rest2"
      phrase_count=$((phrase_count + 1))
      ;;
  esac
done <<< "$DUMP"

[[ "$VERSION" == "1" ]] || err "version must be 1 (got '$VERSION')"
[[ "$route_count" -gt 0 ]] || err "routes[] is empty"

for ((i = 0; i < route_count; i++)); do
  [[ -n "${ROUTE_AGENT[$i]:-}" ]] || err "routes[$i]: targetAgent missing"
  [[ -n "${ROUTE_MODE[$i]:-}" ]] || err "routes[$i]: targetMode missing"
  [[ -n "${ROUTE_SUMMARY[$i]:-}" ]] || err "routes[$i]: summary missing"
done

declare -A ROUTE_PHRASE_COUNT
for ((p = 0; p < phrase_count; p++)); do
  r="${PHRASE_ROUTE_IDX[$p]}"
  ROUTE_PHRASE_COUNT[$r]=$(( ${ROUTE_PHRASE_COUNT[$r]:-0} + 1 ))
done
for ((i = 0; i < route_count; i++)); do
  [[ "${ROUTE_PHRASE_COUNT[$i]:-0}" -gt 0 ]] || err "routes[$i]: phrases[] empty"
done

declare -A SEEN_PHRASES
for ((p = 0; p < phrase_count; p++)); do
  phrase="${PHRASE_TEXT[$p]}"
  r="${PHRASE_ROUTE_IDX[$p]}"
  lower="$(echo "$phrase" | tr '[:upper:]' '[:lower:]')"
  [[ "$phrase" == "$lower" ]] || err "routes[$r] phrase '$phrase' must be lowercase"
  if [[ -n "${SEEN_PHRASES[$phrase]:-}" ]]; then
    err "duplicate phrase '$phrase' (route $r and route ${SEEN_PHRASES[$phrase]})"
  else
    SEEN_PHRASES[$phrase]="$r"
  fi
done

KNOWN_AGENTS=""
[[ -f "$CAPS_FILE" ]] && KNOWN_AGENTS="$(yq -r '.agents | keys | .[]' "$CAPS_FILE" 2>/dev/null || true)"
if [[ -n "$KNOWN_AGENTS" ]]; then
  for ((i = 0; i < route_count; i++)); do
    agent="${ROUTE_AGENT[$i]:-}"
    [[ -z "$agent" ]] && continue
    found=0
    for a in $KNOWN_AGENTS; do
      [[ "$a" == "$agent" ]] && { found=1; break; }
    done
    [[ "$found" -eq 1 ]] || err "routes[$i]: targetAgent '$agent' not in agent-capabilities.yaml"
  done
fi

KNOWN_MODES=""
[[ -f "$MODES_FILE" ]] && KNOWN_MODES="$(yq -r '.modes | keys | .[]' "$MODES_FILE" 2>/dev/null || true)"
if [[ -n "$KNOWN_MODES" ]]; then
  for ((i = 0; i < route_count; i++)); do
    mode="${ROUTE_MODE[$i]:-}"
    [[ -z "$mode" ]] && continue
    found=0
    for m in $KNOWN_MODES; do
      [[ "$m" == "$mode" ]] && { found=1; break; }
    done
    [[ "$found" -eq 1 ]] || err "routes[$i]: targetMode '$mode' not in workflows.yaml"
  done
fi

if [[ "$FAILED" -ne 0 ]]; then
  echo "[intent-routes-lint] FAILED" >&2
  exit 1
fi

info "OK ($route_count routes, ${#SEEN_PHRASES[@]} unique phrases)"
exit 0
