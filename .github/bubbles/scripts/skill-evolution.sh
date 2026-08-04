#!/usr/bin/env bash

set -euo pipefail

# Temp-file cleanup: register every mktemp via _btmp so EXIT/INT/TERM removes them.
_BTMPS=()
trap '[[ ${#_BTMPS[@]} -gt 0 ]] && rm -rf "${_BTMPS[@]}" 2>/dev/null || true' EXIT INT TERM
# shellcheck disable=SC2120  # _btmp forwards optional flags to mktemp (e.g. -d); also called bare.
_btmp() { local t; t="$(mktemp "$@")"; _BTMPS+=("$t"); printf '%s' "$t"; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
WORKFLOWS_FILE="$REPO_ROOT/bubbles/workflows.yaml"
LESSONS_FILE="$REPO_ROOT/.specify/memory/lessons.md"
PROPOSALS_FILE="$REPO_ROOT/.specify/memory/skill-proposals.md"
DISMISSED_FILE="$REPO_ROOT/.specify/memory/skill-proposals-dismissed.md"

threshold_from_registry() {
  local threshold
  threshold="$({ grep -m1 '^  triggerThreshold:' "$WORKFLOWS_FILE" | sed -E 's/.*: *([0-9]+).*/\1/'; } || true)"
  if [[ -n "$threshold" ]]; then
    printf '%s\n' "$threshold"
  else
    printf '3\n'
  fi
}

similarity_from_registry() {
  local similarity
  similarity="$({ grep -m1 '^  similarityThreshold:' "$WORKFLOWS_FILE" | sed -E 's/.*: *([0-9.]+).*/\1/'; } || true)"
  if [[ -n "$similarity" ]]; then
    printf '%s\n' "$similarity"
  else
    printf '0.6\n'
  fi
}

# Group lessons by token overlap rather than whole-line equality, because
# lessons are written by models in free prose: two agents recording the same
# root cause in different words each scored 1 under exact matching and never
# reached triggerThreshold, so the natural-language case the loop exists to
# serve was the one case it could not detect (IMP-034 LRN-2).
#
# Deliberately NOT reusing the G068 word-overlap mechanism in
# state-transition-guard.sh: its comment freezes today's behavior for adopted
# SCN-* IDs, it counts >=3 words plus >=50% (tuned for scenario-title/DoD
# matching, not prose clustering), and it keeps modal words like "should" and
# "must" significant — which is exactly what over-merges lesson prose here.
# Sharing it would couple two unrelated thresholds. See IMP-034 R6.
normalize_lessons() {
  local threshold="$1"
  local similarity="${2:-}"

  if [[ ! -f "$LESSONS_FILE" ]]; then
    return 0
  fi

  [[ -n "$similarity" ]] || similarity="$(similarity_from_registry)"

  # The sort between the two passes is load-bearing: greedy cluster assignment
  # depends on input order, and awk's array iteration order is unspecified, so
  # without it the same corpus could produce different proposals per run.
  awk '
    BEGIN { in_code = 0 }
    /^```/ { in_code = !in_code; next }
    in_code { next }
    /^[[:space:]]*#/ { next }
    /^[[:space:]]*$/ { next }
    {
      line = $0
      sub(/^[[:space:]]*[-*+]/, "", line)
      sub(/^[[:space:]]*[0-9]+[.)][[:space:]]*/, "", line)
      gsub(/\|/, " ", line)
      gsub(/[[:space:]]+/, " ", line)
      sub(/^[[:space:]]+/, "", line)
      sub(/[[:space:]]+$/, "", line)
      line = tolower(line)
      if (length(line) >= 20) {
        print line
      }
    }
  ' "$LESSONS_FILE" | LC_ALL=C sort | awk -v threshold="$threshold" -v similarity="$similarity" '
    function tokenize(s,   m, cnt, j, w, out) {
      cnt = split(s, m, /[^a-z0-9]+/)
      out = " "
      for (j = 1; j <= cnt; j++) {
        w = m[j]
        if (length(w) < 3) continue
        if (w in STOP) continue
        if (index(out, " " w " ") > 0) continue
        out = out w " "
      }
      return out
    }
    function overlap(a, b,   ta, tb, na, nb, k, inter, denom) {
      if (a == b) return 1
      na = split(a, ta, " ")
      nb = split(b, tb, " ")
      if (na == 0 || nb == 0) return 0
      inter = 0
      for (k = 1; k <= na; k++) {
        if (index(b, " " ta[k] " ") > 0) inter++
      }
      # Floor on absolute shared tokens: the ratio alone would merge a short
      # generic lesson into a long specific one, since a subset scores 1.0.
      if (inter < MIN_SHARED) return 0
      denom = (na < nb) ? na : nb
      if (denom <= 0) return 0
      return inter / denom
    }
    BEGIN {
      MIN_SHARED = 3
      split("the and for that with this from have has had was were are not but you your they them their there here when what which who why how all any can will would should must does did into out over under about after before more most some such only same than too very just also then once because while during above below off again further each few nor its it is as at be by do he her him his if in me my no of on or our she so to up us we", sw, " ")
      for (k in sw) STOP[sw[k]] = 1
    }
    { lines[++n] = $0 }
    END {
      for (i = 1; i <= n; i++) toks[i] = tokenize(lines[i])
      nc = 0
      for (i = 1; i <= n; i++) {
        placed = 0
        for (c = 1; c <= nc; c++) {
          if (overlap(toks[i], toks[rep[c]]) >= similarity) {
            size[c]++
            members[c] = members[c] "|" lines[i]
            placed = 1
            break
          }
        }
        if (!placed) {
          nc++
          rep[nc] = i
          size[nc] = 1
          members[nc] = ""
        }
      }
      for (c = 1; c <= nc; c++) {
        if (size[c] >= threshold) {
          printf "%d|%s%s\n", size[c], lines[rep[c]], members[c]
        }
      }
    }
  ' | sort -t'|' -k1,1nr -k2,2
}

slugify() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//; s/-{2,}/-/g'
}

write_proposals() {
  local threshold="$1"
  local temp_file
  local proposals

  mkdir -p "$(dirname "$PROPOSALS_FILE")"
  proposals="$(normalize_lessons "$threshold")"

  if [[ -z "$proposals" ]]; then
    rm -f "$PROPOSALS_FILE"
    return 0
  fi

  temp_file="$(_btmp)"
  {
    echo "# Skill Proposals"
    echo
    echo "Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    echo "Trigger threshold: ${threshold} repeated lesson entries"
    echo
    echo "## Before approving any proposal"
    echo "- Decision rule: do it once → a prompt is fine; recurring + non-obvious + verified → promote to a skill."
    echo "- Quality bar: Reusable · Non-trivial · Specific · Verified."
    echo "- Dedup first: search existing .github/skills/ and skills/INVENTORY.md first — prefer UPDATING an existing skill over creating a new one."
    echo "- Anti-hoarding: when the skill set is large, review least-recently-modified skills for deprecation."
    echo
    while IFS='|' read -r count pattern variants; do
      [[ -n "$pattern" ]] || continue
      local_slug="$(slugify "$pattern")"
      if [[ -z "$local_slug" ]]; then
        local_slug="generated-skill"
      fi
      echo "## Skill Proposal: ${local_slug}"
      echo "- Pattern: ${pattern}"
      echo "- Observed: ${count} times"
      if [[ -n "$variants" ]]; then
        # Print what was grouped so a reviewer can reject a bad merge (IMP-034 R1).
        IFS='|' read -r -a variant_list <<< "$variants"
        echo "- Grouped variants:"
        for variant in "${variant_list[@]}"; do
          [[ -n "$variant" ]] || continue
          echo "  - ${variant}"
        done
      fi
      echo "- Proposed skill: .github/skills/${local_slug}/SKILL.md"
      echo "- Action: Create / Dismiss / Later"
      echo
    done <<< "$proposals"
  } > "$temp_file"

  mv "$temp_file" "$PROPOSALS_FILE"
}

show_proposals() {
  local threshold
  threshold="$(threshold_from_registry)"
  write_proposals "$threshold"

  if [[ -f "$PROPOSALS_FILE" ]]; then
    cat "$PROPOSALS_FILE"
  else
    echo "No skill proposals. Repeated lesson patterns have not crossed the threshold yet."
  fi
}

dismiss_proposals() {
  if [[ ! -f "$PROPOSALS_FILE" ]]; then
    echo "No skill proposals to dismiss."
    return 0
  fi

  mkdir -p "$(dirname "$DISMISSED_FILE")"
  {
    echo "## Dismissed $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    cat "$PROPOSALS_FILE"
    echo
  } >> "$DISMISSED_FILE"

  rm -f "$PROPOSALS_FILE"
  echo "Dismissed all pending skill proposals."
}

case "${1:-show}" in
  show|refresh)
    show_proposals
    ;;
  dismiss|--dismiss)
    dismiss_proposals
    ;;
  *)
    echo "Usage: $(basename "$0") [show|refresh|dismiss]" >&2
    exit 1
    ;;
esac