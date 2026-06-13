#!/usr/bin/env bash
set -uo pipefail

# gate-catalog-freshness.sh
#
# IMP-005 SCOPE-2 — curated-gate-catalog freshness ADVISORY (non-blocking).
#
# Compares the highest gate ID declared in the authoritative registry
# (bubbles/registry/gates.yaml) against the highest gate ID documented in the
# curated human/agent gate-reference surfaces:
#
#   * agents/bubbles_shared/quality-gates.md      (rationale module)
#   * skills/bubbles-quality-gates-catalog/SKILL.md (quick-ref ID lookup)
#
# When the registry ceiling exceeds a curated catalog's ceiling, it emits a
# WARN naming the drift so a newly-added gate without a catalog entry surfaces a
# visible maintenance reminder. This is INFORMATIONAL by design — it ALWAYS
# exits 0 (like repo-drift-report.sh) so it never blocks a build or a doc edit.
# It is a nudge, not a completion gate (the gate set is already enforced by the
# registry + workflows.yaml + each guard script; this only keeps the curated
# *rationale docs* honest about the range they cover).
#
# Exit code: always 0 (advisory). Usage errors print to stderr but still exit 0
# to preserve the non-blocking contract when wired into framework-validate.
#
# Usage:
#   bash bubbles/scripts/gate-catalog-freshness.sh [--repo-root <dir>] [--quiet]
#
# Reference: improvements/IMP-005-curated-gate-catalog-backfill.md (SCOPE-2).

SCRIPT_SOURCE="${BASH_SOURCE[0]}"
SCRIPT_DIR="$(cd "${SCRIPT_SOURCE%/*}" 2>/dev/null && pwd)"

QUIET="false"
REPO_ROOT_ARG=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --quiet) QUIET="true"; shift ;;
    --repo-root)
      shift
      REPO_ROOT_ARG="${1:-}"
      shift || true
      ;;
    -h|--help)
      sed -n '3,30p' "$SCRIPT_SOURCE"
      exit 0
      ;;
    *)
      # Advisory: unknown args are ignored (never block).
      shift
      ;;
  esac
done

if [[ -n "$REPO_ROOT_ARG" ]]; then
  REPO_ROOT="$REPO_ROOT_ARG"
elif [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
  REPO_ROOT="$BUBBLES_REPO_ROOT"
else
  REPO_ROOT="$(cd "$SCRIPT_DIR/../.." 2>/dev/null && pwd || true)"
fi

info() { [[ "$QUIET" == "true" ]] || echo "gate-catalog-freshness: $*"; }
warn() { echo "gate-catalog-freshness: WARN: $*" >&2; }

REGISTRY="$REPO_ROOT/bubbles/registry/gates.yaml"

# No registry (downstream product checkout) → nothing to advise on; clean no-op.
if [[ ! -f "$REGISTRY" ]]; then
  info "no registry at bubbles/registry/gates.yaml — advisory no-op."
  exit 0
fi

# Highest gate id in a file (G0NN / G1NN). Prints the integer, or empty.
highest_gate() {
  local file="$1"
  [[ -f "$file" ]] || { printf ''; return 0; }
  grep -oE 'G[0-9]{3}' "$file" 2>/dev/null \
    | sed -E 's/^G0*//' \
    | sort -n \
    | tail -1
}

registry_high="$(grep -oE '^  G[0-9]{3}:' "$REGISTRY" 2>/dev/null | grep -oE 'G[0-9]{3}' | sed -E 's/^G0*//' | sort -n | tail -1)"
if [[ -z "$registry_high" ]]; then
  info "registry has no parseable gate ids — advisory no-op."
  exit 0
fi

drift=0
for rel in "agents/bubbles_shared/quality-gates.md" "skills/bubbles-quality-gates-catalog/SKILL.md"; do
  path="$REPO_ROOT/$rel"
  [[ -f "$path" ]] || continue
  cat_high="$(highest_gate "$path")"
  if [[ -z "$cat_high" ]]; then
    warn "$rel documents no gate ids but the registry ceiling is G$(printf '%03d' "$registry_high"); consider adding a gate reference."
    drift=$((drift + 1))
  elif [[ "$cat_high" -lt "$registry_high" ]]; then
    warn "$rel ceiling is G$(printf '%03d' "$cat_high") but the registry ceiling is G$(printf '%03d' "$registry_high"). A newer gate may lack a curated entry — see improvements/IMP-005-curated-gate-catalog-backfill.md."
    drift=$((drift + 1))
  fi
done

if [[ "$drift" -eq 0 ]]; then
  info "curated gate catalogs are current with the registry (ceiling G$(printf '%03d' "$registry_high"))."
fi

# ADVISORY: always exit 0 — never block.
exit 0
