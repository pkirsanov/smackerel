#!/usr/bin/env bash
# bubbles/scripts/mode-resolver.sh
#
# Resolve workflow mode definitions from bubbles/workflows.yaml using
# template inheritance. Templates are defined under `.modeTemplates` and
# referenced via `inherits: [...]` on modes or other templates.
#
# Resolution semantics:
#   - Maps deep-merge (mode wins over templates; later templates in the
#     `inherits:` list win over earlier templates)
#   - Arrays CONCATENATE then DEDUPLICATE (preserving first-occurrence
#     order, except `.requiredGates` which is sorted alphabetically as a
#     canonical gate set)
#   - Scalars: latest-wins
#   - Cycles in inherits chains are detected and rejected
#   - Unknown template names are rejected
#   - The `inherits:` field is stripped from resolved output
#
# Hard dependency: yq (mikefarah, v4+).
# https://github.com/mikefarah/yq

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORKFLOWS_FILE="${BUBBLES_WORKFLOWS_FILE:-$ROOT_DIR/bubbles/workflows.yaml}"

if ! command -v yq >/dev/null 2>&1; then
  echo "ERROR: yq (mikefarah, v4+) is required." >&2
  echo "Install: https://github.com/mikefarah/yq" >&2
  exit 1
fi

YQ_MAJOR="$(yq --version 2>&1 | grep -oE 'v[0-9]+' | head -1 | tr -d 'v' || true)"
if ! [[ "$YQ_MAJOR" =~ ^[0-9]+$ ]] || (( YQ_MAJOR < 4 )); then
  echo "ERROR: yq v4+ required (found: $(yq --version 2>&1))" >&2
  exit 1
fi

if [[ ! -f "$WORKFLOWS_FILE" ]]; then
  echo "ERROR: workflows file not found: $WORKFLOWS_FILE" >&2
  exit 1
fi

# Use HOME-based TMP_DIR because snap-confined yq cannot access /tmp.
# (Snap binary yq has the `home` interface but not the `tmp` slot.)
_resolver_tmp_base="${TMPDIR:-$HOME/.cache}"
mkdir -p "$_resolver_tmp_base"
TMP_DIR="$(mktemp -d -p "$_resolver_tmp_base" bubbles-mode-resolver.XXXXXX)"
trap 'rm -rf "$TMP_DIR"' EXIT

usage() {
  cat <<EOF
Usage: $(basename "$0") <command-or-mode-name>

Resolve workflow mode definitions with template inheritance.

Commands:
  <mode-name>          Print the fully-resolved mode definition as YAML
  --list-modes         Print all defined mode names (one per line)
  --list-templates     Print all defined template names (one per line)
  --validate           Validate every template and every mode resolves
                       cleanly with no inherits cycles and no unknown
                       template references. Gate-registry consistency
                       is owned by workflow-registry-selftest.sh.
  --help, -h           Show this help

Source file:
  $WORKFLOWS_FILE
  (override with BUBBLES_WORKFLOWS_FILE env var)

Hard dependency: yq (mikefarah, v4+) — https://github.com/mikefarah/yq

Resolution semantics:
  - Maps deep-merge; later sources win over earlier sources
  - Arrays concatenate then deduplicate
  - .requiredGates is sorted alphabetically as a canonical gate set
  - Cycles in inherits chains are rejected
  - Unknown template names are rejected
  - The inherits: field is stripped from resolved output
EOF
}

die() {
  echo "ERROR: $*" >&2
  exit 1
}

node_kind() {
  # node_kind <yq-path>   →   prints the YAML tag (!!map, !!seq, !!null, ...)
  yq "$1 | type" "$WORKFLOWS_FILE" 2>/dev/null || echo '!!null'
}

template_exists() {
  [[ "$(node_kind ".modeTemplates.\"$1\"")" == '!!map' ]]
}

mode_exists() {
  [[ "$(node_kind ".modes.\"$1\"")" == '!!map' ]]
}

# Deep-merge $2 into $1 in place; arrays concatenate (`*+`).
_merge_into() {
  local target="$1"
  local source="$2"
  local merged
  merged="$(mktemp -p "$TMP_DIR")"
  yq eval-all '. as $item ireduce ({}; . *+ $item)' "$target" "$source" > "$merged"
  mv "$merged" "$target"
}

# Resolve a template into a temp file. Returns the path on stdout.
# Args: $1 = template name, $2 = visited CSV (cycle detection).
resolve_template_to_file() {
  local name="$1"
  local visited="$2"

  template_exists "$name" || die "unknown template referenced via inherits: $name"

  if [[ ",$visited," == *",$name,"* ]]; then
    die "cycle detected in template inherits chain: ${visited//,/ -> } -> $name"
  fi
  local new_visited="${visited:+$visited,}$name"

  local out
  out="$(mktemp -p "$TMP_DIR")"
  echo '{}' > "$out"

  local inherits
  inherits="$(yq -r ".modeTemplates.\"$name\".inherits[]?" "$WORKFLOWS_FILE")"
  if [[ -n "$inherits" ]]; then
    while IFS= read -r parent; do
      [[ -z "$parent" ]] && continue
      local parent_file
      # NOTE: explicit `if !` is required; `var=$(fn)` does NOT propagate
      # set -e when the subshell exits non-zero (bash misfeature, see
      # https://mywiki.wooledge.org/BashFAQ/105).
      if ! parent_file="$(resolve_template_to_file "$parent" "$new_visited")"; then
        exit 1
      fi
      _merge_into "$out" "$parent_file"
    done <<< "$inherits"
  fi

  local own
  own="$(mktemp -p "$TMP_DIR")"
  yq ".modeTemplates.\"$name\" | del(.inherits)" "$WORKFLOWS_FILE" > "$own"
  _merge_into "$out" "$own"

  echo "$out"
}

# Resolve a mode into a temp file (full inheritance + own fields). Returns the path on stdout.
resolve_mode_to_file() {
  local name="$1"
  mode_exists "$name" || die "unknown mode: $name"

  local out
  out="$(mktemp -p "$TMP_DIR")"
  echo '{}' > "$out"

  local inherits
  inherits="$(yq -r ".modes.\"$name\".inherits[]?" "$WORKFLOWS_FILE")"
  if [[ -n "$inherits" ]]; then
    while IFS= read -r parent; do
      [[ -z "$parent" ]] && continue
      local parent_file
      # NOTE: explicit `if !` is required; `var=$(fn)` does NOT propagate
      # set -e when the subshell exits non-zero (bash misfeature, see
      # https://mywiki.wooledge.org/BashFAQ/105).
      if ! parent_file="$(resolve_template_to_file "$parent" "")"; then
        exit 1
      fi
      _merge_into "$out" "$parent_file"
    done <<< "$inherits"
  fi

  local own
  own="$(mktemp -p "$TMP_DIR")"
  yq ".modes.\"$name\" | del(.inherits)" "$WORKFLOWS_FILE" > "$own"
  _merge_into "$out" "$own"

  # Dedup all sequences (preserves first-occurrence order; safe because
  # ordered arrays like phaseOrder, tailPhases, findingDeliveryPhases
  # never contain duplicates in practice).
  yq -i '(.. | select(tag == "!!seq")) |= unique' "$out"

  # Sort .requiredGates alphabetically as a canonical gate set.
  if yq -e '.requiredGates | type == "!!seq"' "$out" >/dev/null 2>&1; then
    yq -i '.requiredGates |= sort' "$out"
  fi

  echo "$out"
}

cmd_list_modes() {
  yq -r '.modes | keys | .[]' "$WORKFLOWS_FILE"
}

cmd_list_templates() {
  if [[ "$(node_kind '.modeTemplates')" == '!!map' ]]; then
    yq -r '.modeTemplates | keys | .[]' "$WORKFLOWS_FILE"
  fi
}

cmd_resolve_mode() {
  local name="$1"
  local f
  f="$(resolve_mode_to_file "$name")"
  cat "$f"
}

cmd_validate() {
  local errors=0

  # Validate each template resolves cleanly (catches cycles + unknown parents).
  if [[ "$(node_kind '.modeTemplates')" == '!!map' ]]; then
    while IFS= read -r tname; do
      [[ -z "$tname" ]] && continue
      local err_file
      err_file="$(mktemp -p "$TMP_DIR")"
      if ! resolve_template_to_file "$tname" "" > /dev/null 2> "$err_file"; then
        echo "FAIL: template '$tname' failed to resolve:" >&2
        cat "$err_file" >&2
        errors=$((errors + 1))
      fi
    done < <(yq -r '.modeTemplates | keys | .[]' "$WORKFLOWS_FILE")
  fi

  # Validate each mode resolves cleanly (catches cycles + unknown parents
  # + malformed inheritance). Gate-registry consistency is the dedicated
  # responsibility of workflow-registry-selftest.sh — keep this resolver
  # focused on its single concern: inheritance correctness.
  while IFS= read -r mname; do
    [[ -z "$mname" ]] && continue
    local err_file
    err_file="$(mktemp -p "$TMP_DIR")"
    if ! resolve_mode_to_file "$mname" > /dev/null 2> "$err_file"; then
      echo "FAIL: mode '$mname' failed to resolve:" >&2
      cat "$err_file" >&2
      errors=$((errors + 1))
    fi
  done < <(yq -r '.modes | keys | .[]' "$WORKFLOWS_FILE")

  if (( errors > 0 )); then
    echo "Validation failed with $errors error(s)." >&2
    exit 1
  fi
  echo "Validation passed: all templates and modes resolve cleanly with no inherits cycles."
}

case "${1:-}" in
  ""|--help|-h)
    usage
    exit 0
    ;;
  --list-modes) cmd_list_modes ;;
  --list-templates) cmd_list_templates ;;
  --validate) cmd_validate ;;
  --*) die "unknown option: $1 (try --help)" ;;
  *) cmd_resolve_mode "$1" ;;
esac
