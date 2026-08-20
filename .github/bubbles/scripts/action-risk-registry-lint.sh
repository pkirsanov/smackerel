#!/usr/bin/env bash
# action-risk-registry-lint.sh — validates the action risk classification
# contract (Gate G139, IMP-052).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
  FRAMEWORK_DIR="$REPO_ROOT/.github/bubbles"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
  FRAMEWORK_DIR="$REPO_ROOT/bubbles"
fi

# shellcheck source=/dev/null
. "$SCRIPT_DIR/action-risk-classes-lib.sh"

REGISTRY_FILE="${BUBBLES_ACTION_RISK_REGISTRY:-$FRAMEWORK_DIR/action-risk-registry.yaml}"

valid_classes="$(action_risk_classes_list)"
required_commands='doctor runtime framework-validate release-check repo-readiness framework-events run-state policy recall'

if [[ ! -f "$REGISTRY_FILE" ]]; then
  echo "Missing action risk registry: $REGISTRY_FILE" >&2
  exit 1
fi

failures=0
fail() {
  echo "$1" >&2
  failures=$((failures + 1))
}

# --- Extract every classification as a flat record stream --------------------
# CMD <name> | DEFAULT <name> <class> | OVERRIDE <name> <key> <class>
records="$(
  awk '
    /^commands:[[:space:]]*$/ { inc = 1; next }
    inc && /^[^[:space:]]/ { inc = 0 }
    !inc { next }
    /^  [A-Za-z0-9_-]+:[[:space:]]*$/ {
      line = $0; sub(/^  /, "", line); sub(/:[[:space:]]*$/, "", line)
      cur = line; in_over = 0
      printf "CMD\t%s\n", cur
      next
    }
    /^    defaultRiskClass:/ {
      v = $0; sub(/^    defaultRiskClass:[[:space:]]*/, "", v)
      printf "DEFAULT\t%s\t%s\n", cur, v
      next
    }
    /^    overrides:[[:space:]]*$/ { in_over = 1; next }
    /^    [A-Za-z0-9_-]+:/ { in_over = 0; next }
    in_over && /^      / {
      line = $0; sub(/^      /, "", line)
      p = index(line, ":")
      if (p > 0) {
        k = substr(line, 1, p - 1)
        v = substr(line, p + 1); gsub(/^[[:space:]]+/, "", v)
        printf "OVERRIDE\t%s\t%s\t%s\n", cur, k, v
      }
      next
    }
  ' "$REGISTRY_FILE"
)"

if [[ -z "$records" ]]; then
  echo "Action risk registry declares no commands: $REGISTRY_FILE" >&2
  exit 1
fi

# --- The registry's own validRiskClasses must match the shared vocabulary ----
registry_classes="$(
  awk '
    /^validRiskClasses:[[:space:]]*$/ { inv = 1; next }
    inv && /^[^[:space:]-]/ { inv = 0 }
    inv && /^-[[:space:]]*/ { v = $0; sub(/^-[[:space:]]*/, "", v); print v }
  ' "$REGISTRY_FILE" | sort | tr '\n' ' '
)"
lib_classes_sorted="$(printf '%s' "$valid_classes" | tr ' ' '\n' | sort | tr '\n' ' ')"
if [[ "$registry_classes" != "$lib_classes_sorted" ]]; then
  fail "Registry validRiskClasses disagrees with action-risk-classes-lib.sh"
  fail "  registry: ${registry_classes:-<empty>}"
  fail "  library:  $lib_classes_sorted"
fi

# --- Every command must carry exactly one valid defaultRiskClass -------------
all_commands="$(printf '%s\n' "$records" | awk -F'\t' '$1 == "CMD" { print $2 }')"
command_count=0
while IFS= read -r command_name; do
  [[ -n "$command_name" ]] || continue
  command_count=$((command_count + 1))
  default_count="$(printf '%s\n' "$records" | awk -F'\t' -v c="$command_name" '$1 == "DEFAULT" && $2 == c' | wc -l | tr -d ' ')"
  if [[ "$default_count" -eq 0 ]]; then
    fail "Missing defaultRiskClass for command: $command_name"
    continue
  fi
  if [[ "$default_count" -gt 1 ]]; then
    fail "Duplicate defaultRiskClass for command: $command_name ($default_count entries)"
  fi
  default_class="$(printf '%s\n' "$records" | awk -F'\t' -v c="$command_name" '$1 == "DEFAULT" && $2 == c { print $3; exit }')"
  if ! action_risk_is_valid_class "$default_class"; then
    fail "Invalid defaultRiskClass '$default_class' for command: $command_name (valid: $valid_classes)"
  fi
done <<EOF
$all_commands
EOF

# --- Every override value must also be a valid class ------------------------
while IFS=$'\t' read -r _kind command_name override_key override_class; do
  [[ -n "${command_name:-}" ]] || continue
  if ! action_risk_is_valid_class "$override_class"; then
    fail "Invalid override risk class '$override_class' for $command_name.$override_key (valid: $valid_classes)"
  fi
done <<EOF
$(printf '%s\n' "$records" | awk -F'\t' '$1 == "OVERRIDE"')
EOF

# --- The nine load-bearing commands must be present -------------------------
for command_name in $required_commands; do
  if ! printf '%s\n' "$all_commands" | grep -qx "$command_name"; then
    fail "Missing action risk command entry: $command_name"
  fi
done

# --- Recall's unknown-operation default must stay fail-safe -----------------
recall_default_class="$(printf '%s\n' "$records" | awk -F'\t' '$1 == "DEFAULT" && $2 == "recall" { print $3; exit }')"
if [[ "$recall_default_class" != "owned_mutation" ]]; then
  fail "Recall unknown-operation default must be owned_mutation (found: ${recall_default_class:-<absent>})"
fi

for recall_operation in search read status freshness sync; do
  recall_class="$(printf '%s\n' "$records" | awk -F'\t' -v op="$recall_operation" '$1 == "OVERRIDE" && $2 == "recall" && $3 == op { print $4; exit }')"
  recall_expected="read_only"
  [[ "$recall_operation" == "sync" ]] && recall_expected="owned_mutation"
  if [[ "$recall_class" != "$recall_expected" ]]; then
    fail "Recall $recall_operation must be classified as $recall_expected (found: ${recall_class:-<absent>})"
  fi
done

# --- Registry <-> CLI parity ------------------------------------------------
# An unregistered command resolves to read_only in pre-tool-risk-gate.sh (the
# ${value:-read_only} default), so a mutating command that nobody registered is
# silently treated as the lowest-risk class. A stale entry is the mirror defect:
# it implies coverage that does not exist. Enforce BOTH directions so neither can
# reappear through ordinary drift. Skipped when cli.sh is absent, which is the
# normal downstream shape.
CLI_FILE="$FRAMEWORK_DIR/scripts/cli.sh"
if [[ -f "$CLI_FILE" ]]; then
  cli_commands="$(
    awk '
      /^main\(\)/ { in_main = 1 }
      !in_main { next }
      /^[[:space:]]+[a-z0-9|_-]+\)[[:space:]]*cmd_/ {
        line = $0
        sub(/^[[:space:]]+/, "", line)
        sub(/\).*$/, "", line)
        n = split(line, parts, "|")
        for (i = 1; i <= n; i++) {
          if (parts[i] ~ /^[a-z][a-z0-9-]*$/) print parts[i]
        }
      }
    ' "$CLI_FILE" | sort -u
  )"
  if [[ -z "$cli_commands" ]]; then
    fail "Could not extract any command from $CLI_FILE main() — parity check would be inert"
  else
    sorted_registry="$(printf '%s\n' "$all_commands" | sort -u)"
    while IFS= read -r cli_cmd; do
      [[ -n "$cli_cmd" ]] || continue
      if ! printf '%s\n' "$sorted_registry" | grep -qx "$cli_cmd"; then
        fail "CLI command '$cli_cmd' has no action risk registry entry (would default to read_only)"
      fi
    done <<EOF
$cli_commands
EOF
    while IFS= read -r reg_cmd; do
      [[ -n "$reg_cmd" ]] || continue
      if ! printf '%s\n' "$cli_commands" | grep -qx "$reg_cmd"; then
        fail "Registry entry '$reg_cmd' has no CLI command (stale classification)"
      fi
    done <<EOF
$sorted_registry
EOF
  fi
fi

if [[ "$failures" -gt 0 ]]; then
  echo "Action risk registry FAILED with $failures finding(s): $REGISTRY_FILE" >&2
  exit 1
fi

echo "Action risk registry OK: $REGISTRY_FILE ($command_count commands validated)"
