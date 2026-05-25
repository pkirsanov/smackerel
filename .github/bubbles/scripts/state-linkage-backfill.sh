#!/usr/bin/env bash
set -euo pipefail

# state-linkage-backfill.sh
#
# Additively backfill v3 state.json linkage/revalidation fields for
# Spec ↔ Implementation Alignment Pass 2. The default mode is dry-run:
# print the would-be JSON to stdout and leave the state file untouched.
# Use --apply to rewrite the state.json file.

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/state-linkage-backfill.sh <spec-dir|state.json> [options]

Options:
  --apply
      Write additive schema fields back to state.json. Without --apply,
      the patched JSON is printed to stdout and the file is not modified.
  --dry-run
      Explicitly request the default dry-run behavior.
  --planning-only
      Classify the state as planning-only. Requires a non-empty
      --planning-only-justification value.
  --planning-only-justification <text>
      Non-empty justification used with --planning-only.
  -h, --help
      Print this help text.

Exit codes:
  0 = state parsed and additive schema operation succeeded
  2 = usage, dependency, malformed JSON, or invalid planning-only request

Backfilled fields:
  linkedImplementationSpec: null
  linkedPlanningPacket: null
  planningOnly: false
  planningOnlyJustification: null
  specDependsOn: []
  certifiedAt: null or latest certification.scopeProgress[].certifiedAt /
               certification.scopeProgress[].certifiedCompletedAt
  requiresRevalidation: false
EOF
}

TARGET=""
MODE="dry-run"
PLANNING_ONLY="false"
PLANNING_ONLY_JUSTIFICATION=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --apply)
      MODE="apply"
      shift
      ;;
    --dry-run)
      MODE="dry-run"
      shift
      ;;
    --planning-only)
      PLANNING_ONLY="true"
      shift
      ;;
    --planning-only-justification)
      [[ $# -ge 2 ]] || { echo "state-linkage-backfill: --planning-only-justification requires a value" >&2; exit 2; }
      PLANNING_ONLY_JUSTIFICATION="$2"
      shift 2
      ;;
    --*)
      echo "state-linkage-backfill: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$TARGET" ]]; then
        echo "state-linkage-backfill: only one target may be supplied" >&2
        usage >&2
        exit 2
      fi
      TARGET="$1"
      shift
      ;;
  esac
done

if [[ -z "$TARGET" ]]; then
  echo "state-linkage-backfill: missing target spec directory or state.json" >&2
  usage >&2
  exit 2
fi

if [[ "$PLANNING_ONLY" == "true" && -z "$PLANNING_ONLY_JUSTIFICATION" ]]; then
  echo "state-linkage-backfill: --planning-only requires non-empty --planning-only-justification" >&2
  exit 2
fi
if [[ "$PLANNING_ONLY" != "true" && -n "$PLANNING_ONLY_JUSTIFICATION" ]]; then
  echo "state-linkage-backfill: --planning-only-justification requires --planning-only" >&2
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "state-linkage-backfill: jq is required but not found in PATH" >&2
  exit 2
fi

STATE_FILE="$TARGET"
if [[ -d "$TARGET" ]]; then
  STATE_FILE="$TARGET/state.json"
fi

if [[ ! -f "$STATE_FILE" ]]; then
  echo "state-linkage-backfill: state.json not found: $STATE_FILE" >&2
  exit 2
fi

if ! jq -e 'type == "object"' "$STATE_FILE" >/dev/null 2>&1; then
  echo "state-linkage-backfill: malformed or non-object JSON: $STATE_FILE" >&2
  exit 2
fi

TMP_FILE="$(mktemp)"
cleanup() {
  rm -f "$TMP_FILE" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

if ! jq \
  --arg planningOnly "$PLANNING_ONLY" \
  --arg planningOnlyJustification "$PLANNING_ONLY_JUSTIFICATION" \
  '
  def latest_scope_certified_at:
    ([
      .certification.scopeProgress[]?
      | (.certifiedAt? // .certifiedCompletedAt? // empty)
      | select(type == "string" and length > 0)
    ] | sort | last) // null;

  if has("linkedImplementationSpec") then . else .linkedImplementationSpec = null end
  | if has("linkedPlanningPacket") then . else .linkedPlanningPacket = null end
  | if has("planningOnly") then . else .planningOnly = false end
  | if has("planningOnlyJustification") then . else .planningOnlyJustification = null end
  | if has("specDependsOn") then . else .specDependsOn = [] end
  | .certifiedAt = (if (has("certifiedAt") and .certifiedAt != null) then .certifiedAt else latest_scope_certified_at end)
  | if has("requiresRevalidation") then . else .requiresRevalidation = false end
  | if $planningOnly == "true" then
      .planningOnly = true
      | .planningOnlyJustification = $planningOnlyJustification
    else
      .
    end
  | if (.linkedImplementationSpec == null or (.linkedImplementationSpec | type) == "string") then . else error("linkedImplementationSpec must be string or null") end
  | if (.linkedPlanningPacket == null or (.linkedPlanningPacket | type) == "string") then . else error("linkedPlanningPacket must be string or null") end
  | if ((.planningOnly | type) == "boolean") then . else error("planningOnly must be boolean") end
  | if (.planningOnlyJustification == null or (.planningOnlyJustification | type) == "string") then . else error("planningOnlyJustification must be string or null") end
  | if ((.specDependsOn | type) == "array") then . else error("specDependsOn must be an array") end
  | if ([.specDependsOn[]? | select(type != "string")] | length) == 0 then . else error("specDependsOn entries must be strings") end
  | if (.certifiedAt == null or (.certifiedAt | type) == "string") then . else error("certifiedAt must be string or null") end
  | if ((.requiresRevalidation | type) == "boolean") then . else error("requiresRevalidation must be boolean") end
  | if (.planningOnly == true and ((.planningOnlyJustification | type) != "string" or (.planningOnlyJustification | length) == 0)) then
      error("planningOnly true requires non-empty planningOnlyJustification")
    else
      .
    end
  ' "$STATE_FILE" > "$TMP_FILE"; then
  echo "state-linkage-backfill: failed to backfill additive schema for $STATE_FILE" >&2
  exit 2
fi

if cmp -s "$STATE_FILE" "$TMP_FILE"; then
  if [[ "$MODE" == "dry-run" ]]; then
    cat "$TMP_FILE"
  fi
  echo "state-linkage-backfill: already backfilled: $STATE_FILE" >&2
  exit 0
fi

if [[ "$MODE" == "apply" ]]; then
  mv "$TMP_FILE" "$STATE_FILE"
  trap - EXIT INT TERM
  echo "state-linkage-backfill: applied additive schema fields: $STATE_FILE"
  exit 0
fi

cat "$TMP_FILE"
echo "state-linkage-backfill: dry-run additive schema changes available: $STATE_FILE" >&2
exit 0