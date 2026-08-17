#!/usr/bin/env bash
# bubbles/scripts/gates-block-reader-lint.sh
#
# Capability: registry-consolidation-safety
#
# Maintain the inventory of scripts that would break if the generated `gates:`
# block were removed from `bubbles/workflows.yaml` (IMP-042 SCOPE-13).
#
# WHY THIS EXISTS
# The block is a generated COPY of `bubbles/registry/gates.yaml`. Deleting it is
# the right destination, and the scope permitted removal "only after
# byte-equivalent queries pass for every remaining reader". That inventory was
# built by looking for scripts that parse gate DEFINITIONS out of the file, so it
# missed every script that greps the same file for a gate NAME. The removal
# shipped, three regressions followed, and it was reverted.
#
# The lesson is not "be more careful". A precondition that depends on a human
# remembering to grep a second way is not a precondition. This lint computes the
# reader set mechanically and holds it as a declared inventory, so the set can
# only shrink deliberately and a NEW dependency cannot be added silently.
#
# HOW A READER IS DETECTED
# Pattern-guessing which greps "look like" gate lookups is what failed before, so
# detection is empirical instead: it asks whether deleting the block would remove
# a token the script depends on.
#
#   1. Compute the line range of the `gates:` block.
#   2. Collect gate identifiers (three-digit `G` ids and `*_gate` names) that
#      appear INSIDE the block and NOWHERE ELSE in the file. Those are exactly
#      the tokens that disappear when the block is deleted. A gate id that also
#      appears in a per-mode `requiredGates` list survives deletion and is
#      therefore not evidence of a dependency.
#   3. A script that references `workflows.yaml` and contains any block-exclusive
#      token is a reader.
#
# Full-line comments are stripped before both tests. Prose ABOUT the block is not
# a dependency on it, and without this the check that WIRES this lint became its
# own first finding: a comment naming workflows.yaml, plus any gate id elsewhere
# in a long file, was enough. Inline trailing comments are left in place, because
# stripping them means stripping `#` inside strings, and a false negative here is
# the failure this whole guard exists to prevent.
#
# The inventory is still deliberately CONSERVATIVE. A script that names a
# block-exclusive gate in executable code while also touching workflows.yaml for
# an unrelated reason is included. Over-inclusion costs a line in a declared
# list; under-inclusion is what caused the revert.
#
# Usage:
#   bash bubbles/scripts/gates-block-reader-lint.sh [--repo-root DIR] [--list] [--seed]
#
# Exit codes:
#   0 = the discovered reader set matches the declared inventory
#   1 = an undeclared reader, a stale declaration, or a missing declared file
#   2 = usage error, or a required input could not be read

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
NAME="gates-block-reader-lint"
MODE="check"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      shift
      REPO_ROOT="${1:-}"
      ;;
    --list)
      MODE="list"
      ;;
    --seed)
      MODE="seed"
      ;;
    -h | --help)
      printf 'usage: %s.sh [--repo-root DIR] [--list] [--seed]\n' "$NAME"
      printf 'Holds the inventory of scripts that depend on the generated gates: block.\n'
      exit 0
      ;;
    *)
      printf '%s: unsupported flag "%s". This lint has no bypass.\n' "$NAME" "$1" >&2
      exit 2
      ;;
  esac
  shift || true
done

WORKFLOWS="$REPO_ROOT/bubbles/workflows.yaml"
INVENTORY="$REPO_ROOT/bubbles/registry/gates-block-readers.txt"
SCRIPTS_DIR="$REPO_ROOT/bubbles/scripts"

[[ -f "$WORKFLOWS" ]] || {
  printf '%s: workflows.yaml not found: %s\n' "$NAME" "$WORKFLOWS" >&2
  exit 2
}
[[ -d "$SCRIPTS_DIR" ]] || {
  printf '%s: scripts directory not found: %s\n' "$NAME" "$SCRIPTS_DIR" >&2
  exit 2
}

# Tokens that exist only inside the gates: block. Empty output is a legitimate
# state -- it means the block is gone, or carries nothing unique -- and is
# reported rather than treated as a parse failure.
block_exclusive_tokens="$(awk '
  function harvest(line, store,   rest, tok) {
    rest = line
    while (match(rest, /G[0-9][0-9][0-9]|[A-Za-z0-9_]+_gate/)) {
      tok = substr(rest, RSTART, RLENGTH)
      store[tok] = 1
      rest = substr(rest, RSTART + RLENGTH)
    }
  }
  /^gates:[[:space:]]*$/ && !seen_block { inside = 1; seen_block = 1; next }
  inside && /^[^[:space:]#]/           { inside = 0 }
  {
    if (inside) harvest($0, in_block)
    else        harvest($0, outside)
  }
  END {
    for (tok in in_block) {
      if (!(tok in outside)) print tok
    }
  }
' "$WORKFLOWS" | LC_ALL=C sort)"

if [[ -z "$block_exclusive_tokens" ]]; then
  printf '%s: the gates: block defines no unique gate identifier.\n' "$NAME"
  printf '%s: nothing can depend on it; the block is safe to remove.\n' "$NAME"
fi

# A single alternation is far cheaper than one grep per token.
token_alternation="$(printf '%s\n' "$block_exclusive_tokens" | paste -sd '|' - 2>/dev/null)"

discovered=""
while IFS= read -r script_path; do
  [[ -f "$script_path" ]] || continue
  relative_path="${script_path#"$REPO_ROOT"/}"
  # The lint itself names the file it guards; excluding it keeps the inventory
  # about real dependencies rather than about this check.
  [[ "$relative_path" == "bubbles/scripts/$NAME.sh" ]] && continue
  script_body="$(grep -v '^[[:space:]]*#' "$script_path" 2>/dev/null)"
  printf '%s\n' "$script_body" | grep -q 'workflows\.yaml' || continue
  [[ -n "$token_alternation" ]] || continue
  if printf '%s\n' "$script_body" | grep -Eq "$token_alternation"; then
    discovered="${discovered}${relative_path}"$'\n'
  fi
done < <(find "$SCRIPTS_DIR" -maxdepth 2 -type f -name '*.sh' 2>/dev/null | LC_ALL=C sort)

discovered="$(printf '%s' "$discovered" | LC_ALL=C sort -u)"
discovered_count="$(printf '%s\n' "$discovered" | grep -c '[^[:space:]]' || true)"

if [[ "$MODE" == "list" ]]; then
  printf '%s\n' "$discovered"
  exit 0
fi

if [[ "$MODE" == "seed" ]]; then
  mkdir -p "$(dirname "$INVENTORY")"
  {
    printf '# Scripts that depend on the generated gates: block in bubbles/workflows.yaml.\n'
    printf '#\n'
    printf '# Generated by bubbles/scripts/gates-block-reader-lint.sh --seed and held by\n'
    printf '# the same lint. IMP-042 SCOPE-13 permits deleting the block only once this\n'
    printf '# list is EMPTY, because every entry breaks when the block goes.\n'
    printf '#\n'
    printf '# To remove an entry, repoint the script at bubbles/registry/gates.yaml and\n'
    printf '# prove its output is byte-identical before and after.\n'
    printf '#\n'
    printf '%s\n' "$discovered"
  } >"$INVENTORY"
  printf '%s: seeded %s reader(s) into %s\n' "$NAME" "$discovered_count" "${INVENTORY#"$REPO_ROOT"/}"
  exit 0
fi

if [[ ! -f "$INVENTORY" ]]; then
  printf '%s: declared inventory missing: %s\n' "$NAME" "$INVENTORY" >&2
  printf '%s: run with --seed to create it.\n' "$NAME" >&2
  exit 2
fi

declared="$(grep -vE '^[[:space:]]*(#|$)' "$INVENTORY" | LC_ALL=C sort -u)"

findings=0

while IFS= read -r reader; do
  [[ -n "$reader" ]] || continue
  if ! printf '%s\n' "$declared" | grep -Fxq "$reader"; then
    printf '%s: UNDECLARED reader of the gates: block: %s\n' "$NAME" "$reader" >&2
    findings=$((findings + 1))
  fi
done <<<"$discovered"

while IFS= read -r reader; do
  [[ -n "$reader" ]] || continue
  if [[ ! -f "$REPO_ROOT/$reader" ]]; then
    printf '%s: declared reader does not exist: %s\n' "$NAME" "$reader" >&2
    findings=$((findings + 1))
    continue
  fi
  if ! printf '%s\n' "$discovered" | grep -Fxq "$reader"; then
    printf '%s: declared reader no longer depends on the block: %s\n' "$NAME" "$reader" >&2
    printf '%s: remove it from %s.\n' "$NAME" "${INVENTORY#"$REPO_ROOT"/}" >&2
    findings=$((findings + 1))
  fi
done <<<"$declared"

if [[ "$findings" -gt 0 ]]; then
  printf '%s: FAIL (%s finding(s))\n' "$NAME" "$findings" >&2
  exit 1
fi

printf '%s: PASS (%s declared reader(s) of the gates: block)\n' "$NAME" "$discovered_count"
if [[ "$discovered_count" -eq 0 ]]; then
  printf '%s: the inventory is empty; SCOPE-13 removal precondition is met.\n' "$NAME"
fi
exit 0
