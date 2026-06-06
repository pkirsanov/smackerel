#!/usr/bin/env bash
# Bubbles cheatsheet generator (v6.0 / B7).
#
# Single source of truth: bubbles/cheatsheet/{modes,aliases,vocabulary}.json
# Renders into both docs/CHEATSHEET.md and docs/its-not-rocket-appliances.html.
# Because both outputs come from one input, drift is structurally impossible —
# the v5.0.1 H7 drift check (cheatsheet-drift-selftest.sh) is retired by this
# script's existence.
#
# Usage:
#   bash bubbles/scripts/generate-cheatsheet.sh             # write blocks in place
#   bash bubbles/scripts/generate-cheatsheet.sh --check     # exit non-zero on drift
#
# Dependencies: bash, jq, awk. No pip, no npm.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
REGISTRY_DIR="$REPO_ROOT/bubbles/cheatsheet"
MODES_JSON="$REGISTRY_DIR/modes.json"
ALIASES_JSON="$REGISTRY_DIR/aliases.json"
VOCAB_JSON="$REGISTRY_DIR/vocabulary.json"
MD_FILE="$REPO_ROOT/docs/CHEATSHEET.md"
HTML_FILE="$REPO_ROOT/docs/its-not-rocket-appliances.html"
WORKFLOWS_YAML="$REPO_ROOT/bubbles/workflows.yaml"
# v6.1 (S2 true split): mode definitions live in bubbles/workflows/modes.yaml.
# Validate mode existence against it unless workflows.yaml still embeds an
# inline modes: block (pre-split / fixtures).
MODES_YAML="$REPO_ROOT/bubbles/workflows/modes.yaml"
if grep -qE '^modes:' "$WORKFLOWS_YAML" 2>/dev/null || [[ ! -f "$MODES_YAML" ]]; then
  MODES_YAML="$WORKFLOWS_YAML"
fi

check_only=false
if [[ "${1:-}" == "--check" ]]; then
  check_only=true
fi

require_file() {
  if [[ ! -f "$1" ]]; then
    echo "generate-cheatsheet: missing required file: $1" >&2
    exit 2
  fi
}

require_file "$MODES_JSON"
require_file "$ALIASES_JSON"
require_file "$VOCAB_JSON"
require_file "$MD_FILE"
require_file "$HTML_FILE"
require_file "$WORKFLOWS_YAML"

if ! command -v jq >/dev/null 2>&1; then
  echo "generate-cheatsheet: jq is required" >&2
  exit 2
fi

for f in "$MODES_JSON" "$ALIASES_JSON" "$VOCAB_JSON"; do
  if ! jq -e . "$f" >/dev/null 2>&1; then
    echo "generate-cheatsheet: invalid JSON in $f" >&2
    exit 2
  fi
done

# ---------------------------------------------------------------------------
# Registry validation
# ---------------------------------------------------------------------------

validate_registry() {
  local missing=0

  # Every mode in modes.json must exist in workflows.yaml. Mode names may carry
  # parenthetical variant suffixes like "(with analyze)" — strip them before
  # the lookup.
  while IFS= read -r mode_name; do
    [[ -n "$mode_name" ]] || continue
    local stripped
    stripped="$(echo "$mode_name" | sed -E 's/[[:space:]]*\(.*$//')"
    if ! awk -v target="  ${stripped}:" '
      /^[A-Za-z][A-Za-z0-9_-]*:/ { in_modes = ($0 ~ /^modes:/) ? 1 : 0; next }
      in_modes && $0 == target { found = 1 }
      END { exit found ? 0 : 1 }
    ' "$MODES_YAML"; then
      echo "generate-cheatsheet: modes.json references unknown workflow mode: $mode_name (stripped: $stripped)" >&2
      missing=$((missing + 1))
    fi
  done < <(jq -r '.[].name' "$MODES_JSON" | sort -u)

  # Every aliases.json maps_to should be a bubbles.<agent> token (optionally
  # with trailing args) OR a known workflow-mode name. Reject anything else.
  local known_modes
  known_modes="$(jq -r '.[].name' "$MODES_JSON" | sed -E 's/[[:space:]]*\(.*$//' | sort -u)"
  while IFS= read -r maps_to; do
    [[ -n "$maps_to" ]] || continue
    case "$maps_to" in
      bubbles.*) continue ;;
      *' + bubbles.'*) continue ;;
    esac
    # Strip compound parts (e.g. "product-to-delivery (with existing impl)")
    local first
    first="$(echo "$maps_to" | sed -E 's/[[:space:]]*\(.*$//' | awk '{print $1}')"
    if grep -qxF "$first" <<<"$known_modes"; then
      continue
    fi
    echo "generate-cheatsheet: aliases.json maps_to '$maps_to' resolves to neither bubbles.<agent> nor a known mode" >&2
    missing=$((missing + 1))
  done < <(jq -r '.[].maps_to' "$ALIASES_JSON")

  # Duplicate alias detection (sunnyvale aliases must be unique).
  local dups
  dups="$(jq -r '.[].alias' "$ALIASES_JSON" | sort | uniq -d || true)"
  if [[ -n "$dups" ]]; then
    echo "generate-cheatsheet: duplicate sunnyvale aliases:" >&2
    echo "$dups" | sed 's/^/  /' >&2
    missing=$((missing + 1))
  fi

  # Duplicate vocabulary term detection.
  dups="$(jq -r '.[].term' "$VOCAB_JSON" | sort | uniq -d || true)"
  if [[ -n "$dups" ]]; then
    echo "generate-cheatsheet: duplicate TPB vocabulary terms:" >&2
    echo "$dups" | sed 's/^/  /' >&2
    missing=$((missing + 1))
  fi

  if [[ "$missing" -gt 0 ]]; then
    echo "generate-cheatsheet: registry validation failed ($missing issue(s))" >&2
    exit 1
  fi
}

# ---------------------------------------------------------------------------
# Block renderers
# ---------------------------------------------------------------------------

# Translate `foo` backticks to <code>foo</code> for HTML rendering.
md_backticks_to_html() {
  python3 -c '
import re, sys
text = sys.stdin.read()
print(re.sub(r"`([^`]+)`", r"<code>\1</code>", text), end="")
'
}

render_md_modes() {
  jq -r '.[] | [.name, .alias, .description] | @tsv' "$MODES_JSON" \
    | awk -F'\t' '{
        printf "| `%s` | %s | %s |\n", $1, $2, $3
      }'
}

render_md_aliases() {
  jq -r '.[] | [.alias, .maps_to, .quote] | @tsv' "$ALIASES_JSON" \
    | awk -F'\t' '{
        gsub(/\*/, "\\*", $3)
        printf "| `sunnyvale %s` | `%s` | *\"%s\"* |\n", $1, $2, $3
      }'
}

render_md_vocab() {
  jq -r '.[] | [.term, .meaning] | @tsv' "$VOCAB_JSON" \
    | awk -F'\t' '{
        printf "| `%s` | %s |\n", $1, $2
      }'
}

render_html_modes() {
  jq -c '.[]' "$MODES_JSON" | while IFS= read -r entry; do
    local name alias desc quote
    name="$(jq -r '.name' <<<"$entry")"
    alias="$(jq -r '.alias' <<<"$entry")"
    desc="$(jq -r '.description_html // .description' <<<"$entry")"
    quote="$(jq -r '.html_quote // empty' <<<"$entry")"
    local desc_html
    desc_html="$(printf '%s' "$desc" | md_backticks_to_html)"
    printf '  <div class="workflow-card">\n'
    printf '    <div class="wf-name">%s</div>\n' "$name"
    printf '    <div class="wf-alias">→ sunnyvale %s</div>\n' "$alias"
    printf '    <div class="wf-desc">%s</div>\n' "$desc_html"
    if [[ -n "$quote" ]]; then
      printf '    <div class="wf-quote">"%s"</div>\n' "$quote"
    fi
    printf '  </div>\n'
  done
}

render_html_aliases() {
  jq -c '.[]' "$ALIASES_JSON" | while IFS= read -r entry; do
    local alias maps_to quote
    alias="$(jq -r '.alias' <<<"$entry")"
    maps_to="$(jq -r '.maps_to' <<<"$entry")"
    quote="$(jq -r '.quote' <<<"$entry")"
    printf '    <tr>\n'
    printf '      <td class="cmd">sunnyvale %s</td>\n' "$alias"
    printf '      <td class="maps-to">%s</td>\n' "$maps_to"
    printf '      <td class="quote-col">"%s"</td>\n' "$quote"
    printf '    </tr>\n'
  done
}

render_html_vocab() {
  jq -c '.[]' "$VOCAB_JSON" | while IFS= read -r entry; do
    local term meaning meaning_html
    term="$(jq -r '.term' <<<"$entry")"
    meaning="$(jq -r '.meaning' <<<"$entry")"
    meaning_html="$(printf '%s' "$meaning" | md_backticks_to_html)"
    printf '  <div class="workflow-card">\n'
    printf '    <div class="wf-name">%s</div>\n' "$term"
    printf '    <div class="wf-desc">%s</div>\n' "$meaning_html"
    printf '  </div>\n'
  done
}

# ---------------------------------------------------------------------------
# Block replacement
# ---------------------------------------------------------------------------
# Replace the contents BETWEEN two marker lines (markers themselves preserved).

replace_block() {
  local target_file="$1"
  local start_marker="$2"
  local end_marker="$3"
  local content_file="$4"
  local temp_file
  temp_file="$(mktemp)"

  awk -v start_marker="$start_marker" \
      -v end_marker="$end_marker" \
      -v content_file="$content_file" '
    BEGIN { state = 0; replaced = 0 }
    {
      if (state == 0 && index($0, start_marker)) {
        print
        while ((getline line < content_file) > 0) print line
        close(content_file)
        state = 1
        replaced = 1
        next
      }
      if (state == 1 && index($0, end_marker)) {
        print
        state = 2
        next
      }
      if (state == 1) next
      print
    }
    END { if (!replaced) exit 2 }
  ' "$target_file" > "$temp_file"

  mv "$temp_file" "$target_file"
}

# ---------------------------------------------------------------------------
# Drive
# ---------------------------------------------------------------------------

validate_registry

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

md_modes_block="$work_dir/md_modes.txt"
md_aliases_block="$work_dir/md_aliases.txt"
md_vocab_block="$work_dir/md_vocab.txt"
html_modes_block="$work_dir/html_modes.txt"
html_aliases_block="$work_dir/html_aliases.txt"
html_vocab_block="$work_dir/html_vocab.txt"

render_md_modes > "$md_modes_block"
render_md_aliases > "$md_aliases_block"
render_md_vocab > "$md_vocab_block"
render_html_modes > "$html_modes_block"
render_html_aliases > "$html_aliases_block"
render_html_vocab > "$html_vocab_block"

if [[ "$check_only" == "true" ]]; then
  staged_md="$work_dir/check.md"
  staged_html="$work_dir/check.html"
  cp "$MD_FILE" "$staged_md"
  cp "$HTML_FILE" "$staged_html"

  replace_block "$staged_md" "GENERATED:CHEATSHEET_ALIASES_START" "GENERATED:CHEATSHEET_ALIASES_END" "$md_aliases_block"
  replace_block "$staged_md" "GENERATED:CHEATSHEET_MODES_START" "GENERATED:CHEATSHEET_MODES_END" "$md_modes_block"
  replace_block "$staged_md" "GENERATED:CHEATSHEET_VOCABULARY_START" "GENERATED:CHEATSHEET_VOCABULARY_END" "$md_vocab_block"
  replace_block "$staged_html" "GENERATED:HTML_MODES_CARDS_START" "GENERATED:HTML_MODES_CARDS_END" "$html_modes_block"
  replace_block "$staged_html" "GENERATED:HTML_ALIASES_TABLE_START" "GENERATED:HTML_ALIASES_TABLE_END" "$html_aliases_block"
  replace_block "$staged_html" "GENERATED:HTML_VOCABULARY_CARDS_START" "GENERATED:HTML_VOCABULARY_CARDS_END" "$html_vocab_block"

  drifted=0
  if ! diff -u "$MD_FILE" "$staged_md" >&2; then
    echo "generate-cheatsheet: docs/CHEATSHEET.md is stale. Run: bash bubbles/scripts/generate-cheatsheet.sh" >&2
    drifted=1
  fi
  if ! diff -u "$HTML_FILE" "$staged_html" >&2; then
    echo "generate-cheatsheet: docs/its-not-rocket-appliances.html is stale. Run: bash bubbles/scripts/generate-cheatsheet.sh" >&2
    drifted=1
  fi
  exit "$drifted"
fi

replace_block "$MD_FILE" "GENERATED:CHEATSHEET_ALIASES_START" "GENERATED:CHEATSHEET_ALIASES_END" "$md_aliases_block"
replace_block "$MD_FILE" "GENERATED:CHEATSHEET_MODES_START" "GENERATED:CHEATSHEET_MODES_END" "$md_modes_block"
replace_block "$MD_FILE" "GENERATED:CHEATSHEET_VOCABULARY_START" "GENERATED:CHEATSHEET_VOCABULARY_END" "$md_vocab_block"
replace_block "$HTML_FILE" "GENERATED:HTML_MODES_CARDS_START" "GENERATED:HTML_MODES_CARDS_END" "$html_modes_block"
replace_block "$HTML_FILE" "GENERATED:HTML_ALIASES_TABLE_START" "GENERATED:HTML_ALIASES_TABLE_END" "$html_aliases_block"
replace_block "$HTML_FILE" "GENERATED:HTML_VOCABULARY_CARDS_START" "GENERATED:HTML_VOCABULARY_CARDS_END" "$html_vocab_block"

mode_count=$(jq 'length' "$MODES_JSON")
alias_count=$(jq 'length' "$ALIASES_JSON")
vocab_count=$(jq 'length' "$VOCAB_JSON")
echo "cheatsheet generated: $mode_count modes, $alias_count aliases, $vocab_count vocab terms"
