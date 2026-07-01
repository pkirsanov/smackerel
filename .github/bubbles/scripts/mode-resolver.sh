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
MODES_FILE="${BUBBLES_MODES_FILE:-$ROOT_DIR/bubbles/workflows/modes.yaml}"
ALIASES_FILE="${BUBBLES_WORKFLOW_ALIASES_FILE:-$ROOT_DIR/bubbles/workflows/aliases.yaml}"
# Preserve the operator-facing path for --help; WORKFLOWS_FILE may be
# reassigned to a composed temp file below once modes live in their own file.
WORKFLOWS_DISPLAY="$WORKFLOWS_FILE"

# v7 grandfather switch. v7.0 removes bare v5 mode NAMES as operator input
# (they remain the canonical registry KEYS). Tools that resolve a PERSISTED
# mode from an existing artifact (state-transition-guard, artifact-lint,
# is-terminal-for-mode) set BUBBLES_MODE_GRANDFATHER=1 (or pass --grandfather)
# so stored v5-key modes keep resolving. New operator input that types a bare
# v5 name is rejected with a pointer to the v6 primitive+tag form.
GRANDFATHER="0"
if [[ "${BUBBLES_MODE_GRANDFATHER:-0}" == "1" ]]; then
  GRANDFATHER="1"
fi
_mr_args=()
for _mr_a in "$@"; do
  if [[ "$_mr_a" == "--grandfather" ]]; then
    GRANDFATHER="1"
  else
    _mr_args+=("$_mr_a")
  fi
done
if (( ${#_mr_args[@]} )); then
  set -- "${_mr_args[@]}"
else
  set --
fi
unset _mr_args _mr_a

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

# v6.1 (S2 true split): the canonical mode registry lives in its own file
# (bubbles/workflows/modes.yaml) and is NO LONGER duplicated inside
# workflows.yaml. Compose the two so every .modes / .modeTemplates read below
# sees one unified document. Composition is skipped when the workflows file
# already carries an inline `modes:` block, which keeps two cases working
# unchanged: (a) selftests that pass self-contained fixtures, and (b) any
# pre-split or downstream-transitional workflows.yaml that still embeds modes.
if [[ -f "$MODES_FILE" ]] && ! grep -qE '^modes:' "$WORKFLOWS_FILE"; then
  _composed_file="$(mktemp -p "$TMP_DIR" workflows-composed.XXXXXX.yaml)"
  yq eval-all '. as $item ireduce ({}; . * $item)' "$WORKFLOWS_FILE" "$MODES_FILE" > "$_composed_file"
  WORKFLOWS_FILE="$_composed_file"
fi

usage() {
  cat <<EOF
Usage: $(basename "$0") <command-or-mode-name>

Resolve workflow mode definitions with template inheritance.

Commands:
  <mode-name>          Print the fully-resolved mode definition as YAML.
                       If <mode-name> is a v5 mode, prints a stderr
                       deprecation hint pointing at the v6 primitive+tag
                       form.
  <primitive> tag:val [tag:val ...]
                       Resolve a v6 primitive+tag invocation to its
                       backing v5 mode, then print the fully-resolved
                       definition. Example:
                         mode-resolver.sh ship action:promote
                         mode-resolver.sh upkeep task:restore-drill
                         mode-resolver.sh fix target:bug action:fastlane
  --list-modes         Print all defined mode names (one per line)
  --list-aliases       Print every v5 mode and its v6 primitive+tag tuple
                       (TSV: v5-name<TAB>primitive<TAB>tag:val,tag:val)
  --resolve-v6 ARGS    Resolve v6 primitive+tag form (ARGS is space-
                       separated) to the v5 mode name. Output: bare
                       mode name on stdout. Exit 1 on unknown tuple.
  --list-templates     Print all defined template names (one per line)
  --validate           Validate every template and every mode resolves
                       cleanly with no inherits cycles and no unknown
                       template references. Gate-registry consistency
                       is owned by workflow-registry-selftest.sh.
  --help, -h           Show this help

Source file:
  $WORKFLOWS_DISPLAY
  (override with BUBBLES_WORKFLOWS_FILE env var)
  Modes registry: $MODES_FILE
  (override with BUBBLES_MODES_FILE env var)

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

# Validate inheritance references without materializing fully merged modes.
# Full resolution is covered by targeted selftests; --validate only needs to
# prove that every inherits edge is known and acyclic.
validate_template_chain_cached() {
  local name="$1"
  local visited="$2"

  if [[ -z "${template_seen[$name]+x}" ]]; then
    echo "ERROR: unknown template referenced via inherits: $name" >&2
    return 1
  fi

  if [[ ",$visited," == *",$name,"* ]]; then
    echo "ERROR: cycle detected in template inherits chain: ${visited//,/ -> } -> $name" >&2
    return 1
  fi

  local new_visited="${visited:+$visited,}$name"
  local parents="${template_parents[$name]-}"
  if [[ -n "$parents" ]]; then
    while IFS= read -r parent; do
      [[ -z "$parent" ]] && continue
      validate_template_chain_cached "$parent" "$new_visited" || return 1
    done <<< "$parents"
  fi
}

validate_mode_inherits_cached() {
  local name="$1"
  local parents="${mode_parents[$name]-}"
  if [[ -n "$parents" ]]; then
    while IFS= read -r parent; do
      [[ -z "$parent" ]] && continue
      validate_template_chain_cached "$parent" "" || return 1
    done <<< "$parents"
  fi
}

cmd_list_modes() {
  yq -r '.modes | keys | .[]' "$WORKFLOWS_FILE"
}

# ── v6 primitive+tag alias support (B4) ───────────────────────────────

aliases_available() {
  [[ -f "$ALIASES_FILE" ]]
}

# Normalize a tag set string (space-separated key:value pairs) into a
# stable sorted form. Empty input becomes the empty string.
_normalize_tags() {
  local input="$1"
  if [[ -z "$input" ]]; then
    echo ""
    return
  fi
  # `paste -sd ' '` (no file operand) reads stdin on GNU paste but fails on
  # BSD/macOS paste ("usage: paste ..."). The explicit `-` operand reads stdin
  # on both.
  echo "$input" | tr ' ' '\n' | sort -u | paste -sd ' ' -
}

# List every v5 alias as TSV: v5<TAB>primitive<TAB>sorted-tag-set
# Example row: ship<TAB>release-train-promote<TAB>action:promote
_list_alias_tsv() {
  aliases_available || return 0
  yq -r '
    .v5Aliases
    | to_entries[]
    | .key as $v5
    | .value.primitive as $prim
    | (.value.tags // {} | to_entries | sort_by(.key) | map(.key + ":" + .value) | join(" ")) as $tags
    | [$v5, $prim, $tags] | @tsv
  ' "$ALIASES_FILE"
}

cmd_list_aliases() {
  _list_alias_tsv
}

# Resolve a v6 primitive+tag invocation -> v5 mode name on stdout.
# Args: $1=primitive, then space-separated tag:val pairs.
resolve_v6_to_v5() {
  local primitive="$1"
  shift
  local tags_input="$*"
  local tags_norm
  tags_norm="$(_normalize_tags "$tags_input")"

  aliases_available || {
    echo "ERROR: aliases file not found: $ALIASES_FILE (v6 primitive+tag form requires bubbles/workflows/aliases.yaml)" >&2
    return 1
  }

  local match_count=0
  local matched_v5=""
  while IFS=$'\t' read -r v5 prim tags; do
    [[ -z "$v5" ]] && continue
    [[ "$prim" == "$primitive" ]] || continue
    [[ "$tags" == "$tags_norm" ]] || continue
    matched_v5="$v5"
    match_count=$((match_count + 1))
  done < <(_list_alias_tsv)

  if (( match_count == 0 )); then
    echo "ERROR: no v5 alias matches v6 form '$primitive $tags_norm'" >&2
    return 1
  fi
  if (( match_count > 1 )); then
    echo "ERROR: ambiguous v6 form '$primitive $tags_norm' matches $match_count v5 modes (alias map invariant violation)" >&2
    return 1
  fi
  echo "$matched_v5"
}

# Resolve a v5 mode -> v6 primitive+tag form on stdout.
# Format: "primitive tag:val tag:val ..." (sorted by tag key).
resolve_v5_to_v6() {
  local v5="$1"
  aliases_available || return 1
  local found=""
  while IFS=$'\t' read -r name prim tags; do
    [[ "$name" == "$v5" ]] || continue
    found="${prim}${tags:+ $tags}"
    break
  done < <(_list_alias_tsv)
  [[ -n "$found" ]] || return 1
  echo "$found"
}

cmd_resolve_v6() {
  if (( $# == 0 )); then
    die "--resolve-v6 requires arguments (primitive plus tag:val pairs)"
  fi
  resolve_v6_to_v5 "$@"
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
  local template_names mode_names template_edges mode_edges

  template_names="$(yq -r '(.modeTemplates // {}) | keys | .[]' "$WORKFLOWS_FILE")"
  mode_names="$(yq -r '(.modes // {}) | keys | .[]' "$WORKFLOWS_FILE")"
  template_edges="$(yq -r '(.modeTemplates // {}) | to_entries[] | .key as $name | (.value.inherits // [])[] | [$name, .] | @tsv' "$WORKFLOWS_FILE")"
  mode_edges="$(yq -r '(.modes // {}) | to_entries[] | .key as $name | (.value.inherits // [])[] | [$name, .] | @tsv' "$WORKFLOWS_FILE")"

  declare -A template_seen=()
  declare -A template_parents=()
  declare -A mode_parents=()

  while IFS= read -r tname; do
    [[ -z "$tname" ]] && continue
    template_seen["$tname"]=1
  done <<< "$template_names"

  while IFS=$'\t' read -r child parent; do
    [[ -z "$child" && -z "$parent" ]] && continue
    template_parents["$child"]+="${parent}"$'\n'
  done <<< "$template_edges"

  while IFS=$'\t' read -r child parent; do
    [[ -z "$child" && -z "$parent" ]] && continue
    mode_parents["$child"]+="${parent}"$'\n'
  done <<< "$mode_edges"

  # Validate each template resolves cleanly (catches cycles + unknown parents).
  while IFS= read -r tname; do
    [[ -z "$tname" ]] && continue
    local err_file
    err_file="$(mktemp -p "$TMP_DIR")"
    if ! validate_template_chain_cached "$tname" "" > /dev/null 2> "$err_file"; then
      echo "FAIL: template '$tname' failed to resolve:" >&2
      cat "$err_file" >&2
      errors=$((errors + 1))
    fi
  done <<< "$template_names"

  # Validate each mode resolves cleanly (catches cycles + unknown parents
  # + malformed inheritance). Gate-registry consistency is the dedicated
  # responsibility of workflow-registry-selftest.sh — keep this resolver
  # focused on its single concern: inheritance correctness.
  while IFS= read -r mname; do
    [[ -z "$mname" ]] && continue
    local err_file
    err_file="$(mktemp -p "$TMP_DIR")"
    if ! validate_mode_inherits_cached "$mname" > /dev/null 2> "$err_file"; then
      echo "FAIL: mode '$mname' failed to resolve:" >&2
      cat "$err_file" >&2
      errors=$((errors + 1))
    fi
  done <<< "$mode_names"

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
  --list-aliases) cmd_list_aliases ;;
  --resolve-v6)
    shift
    cmd_resolve_v6 "$@"
    ;;
  --validate) cmd_validate ;;
  --*) die "unknown option: $1 (try --help)" ;;
  *)
    primitive="$1"
    shift || true
    # If extra tag:val args follow, treat as v6 primitive+tag invocation.
    if (( $# > 0 )); then
      v5_mode="$(resolve_v6_to_v5 "$primitive" "$@")" || exit 1
      cmd_resolve_mode "$v5_mode"
      exit 0
    fi
    # Bare token: try as v5 mode first. If unknown, see if it is a v6
    # primitive with no tags (rare — only `analyze` and friends).
    if mode_exists "$primitive"; then
      # v7: bare v5 mode names are REMOVED as operator input. They remain the
      # canonical registry KEYS — state.json.workflowMode stores them and the
      # guards resolve status ceilings by direct registry lookup — so existing
      # artifacts are completely unaffected. Typing a v5 name to START new work
      # is rejected; the operator must use the v6 primitive+tag form. Tools that
      # resolve a PERSISTED mode (guards, is-terminal) set
      # BUBBLES_MODE_GRANDFATHER=1 / pass --grandfather to keep resolving stored
      # v5-key modes.
      v6_form=""
      if aliases_available; then
        v6_form="$(resolve_v5_to_v6 "$primitive" || true)"
      fi
      if [[ -n "$v6_form" && "$GRANDFATHER" != "1" ]]; then
        echo "ERROR: v5 mode name '$primitive' was removed in v7. Use the v6 form: '$v6_form'." >&2
        echo "       Existing artifacts that already store '$primitive' keep working unchanged; this rejection only applies to new operator input. To resolve a persisted mode programmatically, set BUBBLES_MODE_GRANDFATHER=1 or pass --grandfather." >&2
        exit 3
      fi
      if [[ -n "$v6_form" && "$GRANDFATHER" == "1" ]]; then
        echo "DEPRECATION (v7 grandfather): resolving removed v5 mode '$primitive' (v6 form: '$v6_form'). New work must use the v6 form." >&2
      fi
      cmd_resolve_mode "$primitive"
    else
      # Bare primitive with no tags — try matching aliases that have no tags.
      v5_mode="$(resolve_v6_to_v5 "$primitive" 2>/dev/null || true)"
      if [[ -n "$v5_mode" ]]; then
        cmd_resolve_mode "$v5_mode"
      else
        die "unknown mode or unmappable v6 primitive: $primitive"
      fi
    fi
    ;;
esac
