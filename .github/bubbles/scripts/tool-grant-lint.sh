#!/usr/bin/env bash
set -uo pipefail

# tool-grant-lint.sh — flag tool families granted to an agent that nothing it
# owns requires (IMP-039 SCOPE-5 / COST-6).
#
# WHY
# Tool definitions are re-sent on every request. Measured: ~40,985 tokens per
# metered request, 12.6% of a 2,606,430-token prompt total. A family granted to
# an agent that never uses it is paid on every dispatch and buys nothing.
#
# ADVISORY BY DEFAULT — and that is a design decision, not timidity. The
# dispatch-control frontmatter is RUNTIME-enforced, so an over-narrow grant
# breaks routing SILENTLY. Reporting the delta first, changing one agent, and
# validating dispatch before the next is the only safe order. `--strict` turns
# findings into exit 1 once a repo has completed that ratchet.
#
# WHAT IT CHECKS
# For each agents/bubbles.*.agent.md, the `tools:` frontmatter list is compared
# against bubbles/registry/tool-grants.yaml. A family is flagged only when it is
# in `restricted` AND the agent is neither named in `justifiedByAgents` nor owns
# a phase named in `justifiedByPhases` (phase ownership read from the `phases:`
# block of bubbles/workflows.yaml). Unrestricted families are never flagged.
#
# Exit codes:
#   0  no findings, or findings in advisory mode (default)
#   1  findings in --strict mode
#   2  usage error / missing inputs
#
# Usage:
#   bash bubbles/scripts/tool-grant-lint.sh [--root DIR] [--strict] [--quiet]

ROOT=""
STRICT=0
QUIET=0

die_usage() {
  printf 'tool-grant-lint: %s\n' "$1" >&2
  printf 'usage: tool-grant-lint.sh [--root DIR] [--strict] [--quiet]\n' >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --root) shift; ROOT="${1:-}" ;;
    --strict) STRICT=1 ;;
    --quiet) QUIET=1 ;;
    -h|--help) sed -n '3,31p' "${BASH_SOURCE[0]}"; exit 0 ;;
    --skip*|--force*|--ignore*) die_usage "bypass-shaped flag '$1' is not supported" ;;
    *) die_usage "unknown argument '$1'" ;;
  esac
  shift
done

[[ -n "$ROOT" ]] || ROOT="$(cd "${BASH_SOURCE[0]%/*}/../.." && pwd)"
[[ -d "$ROOT" ]] || die_usage "root '$ROOT' is not a directory"

REGISTRY="$ROOT/bubbles/registry/tool-grants.yaml"
WORKFLOWS="$ROOT/bubbles/workflows.yaml"
AGENT_DIR="$ROOT/agents"

[[ -f "$REGISTRY" ]] || die_usage "registry not found at $REGISTRY"
[[ -d "$AGENT_DIR" ]] || die_usage "agents directory not found at $AGENT_DIR"

# --- registry: restricted families and their justifications -----------------
# A plain awk block scan, matching the dependency-free convention the other
# registry readers use (no yq/python on the validation PATH).

restricted_families() {
  awk '
    /^restricted:[[:space:]]*$/ { inblock = 1; next }
    inblock && /^[^[:space:]#]/ { inblock = 0 }
    inblock && /^  [a-z][a-z0-9-]*:[[:space:]]*$/ {
      gsub(/[ :]/, "", $0); print
    }
  ' "$REGISTRY"
}

# Emit the space-separated values of `<key>: [ a, b ]` inside the block for
# family $1.
justified_for() {
  awk -v fam="$1" -v key="$2" '
    /^restricted:[[:space:]]*$/ { inrestricted = 1; next }
    inrestricted && /^[^[:space:]#]/ { inrestricted = 0 }
    inrestricted && /^  [a-z][a-z0-9-]*:[[:space:]]*$/ {
      name = $0; gsub(/[ :]/, "", name)
      infam = (name == fam)
      next
    }
    infam && $1 == key ":" { }
    infam {
      if (index($0, key ":") > 0) {
        line = $0
        sub(/^.*\[/, "", line)
        sub(/\].*$/, "", line)
        gsub(/,/, " ", line)
        print line
      }
    }
  ' "$REGISTRY"
}

# --- phases owned by an agent, from workflows.yaml --------------------------

phases_owned_by() {
  awk -v agent="$1" '
    /^phases:[[:space:]]*$/ { inphases = 1; next }
    inphases && /^[^[:space:]#]/ { inphases = 0 }
    inphases && /^  [a-z][a-zA-Z0-9-]*:[[:space:]]*$/ {
      current = $0; gsub(/[ :]/, "", current); next
    }
    inphases && $1 == "owner:" && $2 == agent { print current }
  ' "$WORKFLOWS"
}

# --- agent frontmatter tools ------------------------------------------------

tools_of() {
  awk '
    NR == 1 && $0 != "---" { exit }
    NR > 1 && $0 == "---" { exit }
    /^tools:/ {
      line = $0
      sub(/^tools:[[:space:]]*/, "", line)
      gsub(/[][]/, "", line)
      gsub(/,/, " ", line)
      print line
      exit
    }
  ' "$1"
}

contains_word() {
  case " $2 " in *" $1 "*) return 0 ;; *) return 1 ;; esac
}

FINDINGS=0
AGENTS_SCANNED=0
mapfile -t RESTRICTED < <(restricted_families)

if [[ "${#RESTRICTED[@]}" -eq 0 ]]; then
  die_usage "registry declares no restricted families; nothing to lint"
fi

for agent_file in "$AGENT_DIR"/bubbles.*.agent.md; do
  [[ -f "$agent_file" ]] || continue
  agent_name="$(basename "$agent_file" .agent.md)"
  granted="$(tools_of "$agent_file")"
  [[ -n "$granted" ]] || continue
  AGENTS_SCANNED=$((AGENTS_SCANNED + 1))

  owned="$(phases_owned_by "$agent_name" | tr '\n' ' ')"

  for fam in "${RESTRICTED[@]}"; do
    contains_word "$fam" "$granted" || continue

    ok_agents="$(justified_for "$fam" justifiedByAgents | tr '\n' ' ')"
    if contains_word "$agent_name" "$ok_agents"; then
      continue
    fi

    ok_phases="$(justified_for "$fam" justifiedByPhases | tr '\n' ' ')"
    justified_by_phase=0
    for p in $owned; do
      if contains_word "$p" "$ok_phases"; then
        justified_by_phase=1
        break
      fi
    done
    [[ "$justified_by_phase" -eq 1 ]] && continue

    FINDINGS=$((FINDINGS + 1))
    printf 'tool-grant-lint: %s grants restricted family "%s" with no justification\n' "$agent_name" "$fam" >&2
    printf '  owned phases:  %s\n' "${owned:-none}" >&2
    printf '  justified for: agents=[%s] phases=[%s]\n' "${ok_agents% }" "${ok_phases% }" >&2
    printf '  effect:        the definition for "%s" is re-sent on every dispatch of %s\n' "$fam" "$agent_name" >&2
  done
done

if [[ "$FINDINGS" -gt 0 ]]; then
  printf '\ntool-grant-lint: %d finding(s) across %d agent(s)\n' "$FINDINGS" "$AGENTS_SCANNED" >&2
  if [[ "$STRICT" -eq 1 ]]; then
    printf 'tool-grant-lint: --strict — narrow the grant, then confirm dispatch still resolves.\n' >&2
    printf '  Frontmatter is runtime-enforced, so an over-narrow grant breaks routing silently.\n' >&2
    exit 1
  fi
  printf 'tool-grant-lint: ADVISORY — not blocking. Re-run with --strict once the grants are narrowed.\n' >&2
  exit 0
fi

[[ "$QUIET" -eq 1 ]] || printf '[tool-grant-lint] OK — %d agent(s), no unjustified restricted grants\n' "$AGENTS_SCANNED"
exit 0
