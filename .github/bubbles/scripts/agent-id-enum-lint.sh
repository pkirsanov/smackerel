#!/usr/bin/env bash
# bubbles/scripts/agent-id-enum-lint.sh
#
# Constrain executionHistory[].agent to a registered agent id (IMP-036 SCOPE-7).
#
# WHY THIS EXISTS
# The control plane's primary key was free text. A portfolio audit measured 163
# distinct `agent` values across 15,685 recorded invocations, 60 of them
# appearing exactly once, including values like:
#
#   "bubbles.workflow (parent-expanded via stochastic-quality-sweep Round 14/20)"
#
# Nothing downstream can aggregate that. It is why a framework-wide dispatch
# failure stayed invisible for months: no query could group the runs that showed
# it. This lint makes the field groupable again.
#
# THE CONTRACT
#   agent            MUST be a registered id from bubbles/agent-capabilities.yaml
#   expansionReason  the "(parent-expanded ...)" qualifier, as its own field
#   sweepRound       the "Round 14/20" qualifier, as its own field
#
# Qualifiers belong in sibling fields, never inside the id. See
# agents/bubbles_shared/feature-templates.md.
#
# RATCHET, NOT A CLIFF
# Six consuming repos already carry thousands of records written under the old
# free-text rule. Hard-failing them would make this lint unrunnable, so the
# pre-existing unknown ids are frozen in agent-id-enum-lint.baseline and the
# lint fails ONLY on ids that are neither registered nor baselined. A baselined
# id that disappears is reported as stale so the file can only shrink.
#
# Usage:
#   bash bubbles/scripts/agent-id-enum-lint.sh <repo-root-or-spec-dir> [--verbose]
#   bash bubbles/scripts/agent-id-enum-lint.sh <repo-root> --update-baseline
#
# Exit codes:
#   0 = clean (or baseline updated)
#   1 = an agent id is neither registered nor baselined
#   2 = usage error, missing target, or a bypass-shaped flag

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
CAPABILITIES_FILE="${BUBBLES_AGENT_CAPABILITIES_FILE:-$ROOT_DIR/bubbles/agent-capabilities.yaml}"
# The baseline is a PER-REPO ratchet, so it lives in the consuming repo and not
# beside this script. Downstream this script installs under
# .github/bubbles/scripts/, which downstream-framework-write-guard.sh forbids the
# consuming repo from editing, so a baseline there could never be updated by the
# only party entitled to update it. A framework-side baseline would also carry
# downstream spec paths into the portable tree.
resolve_baseline_root() {
  local d
  d="$(cd "$TARGET" && pwd -P)"
  if command -v git >/dev/null 2>&1 && git -C "$d" rev-parse --show-toplevel >/dev/null 2>&1; then
    git -C "$d" rev-parse --show-toplevel
    return 0
  fi
  printf '%s' "$d"
}

# Non-agent actors that legitimately appear in executionHistory. These are not
# agents and never will be, so they are part of the contract rather than debt.
NON_AGENT_ACTORS="manual operator human"

# grep -c already prints 0 on no match and then exits 1, so the fallback must be
# `true` and never `echo 0` - the latter emits a second zero.
count_lines() { printf '%s' "${1:-}" | grep -c . 2>/dev/null || true; }

usage() {
  cat <<'USAGE'
usage: agent-id-enum-lint.sh <repo-root-or-spec-dir> [--verbose]
       agent-id-enum-lint.sh <repo-root> --update-baseline

Validates that every executionHistory[].agent resolves to a registered agent id.
A trailing "(...)" qualifier is stripped before the check; carrying one is
reported as migration debt because it belongs in a sibling field.

There is no --skip, --force or --ignore flag. A newly legitimate id is added by
registering the agent, never by bypassing the check.
USAGE
}

die_usage() {
  printf 'agent-id-enum-lint: %s\n' "$1" >&2
  usage >&2
  exit 2
}

TARGET=""
VERBOSE="false"
UPDATE_BASELINE="false"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --verbose) VERBOSE="true" ;;
    --update-baseline) UPDATE_BASELINE="true" ;;
    -h|--help) usage; exit 0 ;;
    --skip*|--force*|--ignore*|--no-verify*|--bypass*|--allow*)
      die_usage "bypass-shaped flag '$1' is not supported and never will be" ;;
    -*) die_usage "unknown flag '$1'" ;;
    *) [[ -n "$TARGET" ]] && die_usage "unexpected extra argument '$1'"; TARGET="$1" ;;
  esac
  shift
done

[[ -n "$TARGET" ]] || die_usage "a repo root or spec directory is required (this tool has no default surface)"
[[ -d "$TARGET" ]] || die_usage "target does not exist: $TARGET"
[[ -f "$CAPABILITIES_FILE" ]] || die_usage "agent capabilities file not found: $CAPABILITIES_FILE"

# Resolved only after TARGET is known, because the ratchet is per-repo.
BASELINE_FILE="${BUBBLES_AGENT_ID_BASELINE_FILE:-$(resolve_baseline_root)/.specify/agent-id-enum-lint.baseline}"

# --- registered agent ids ----------------------------------------------------
registered="$(grep -oE '^  bubbles\.[a-z][a-z-]*:' "$CAPABILITIES_FILE" 2>/dev/null | tr -d ' :' | LC_ALL=C sort -u)"
if [[ -z "$registered" ]]; then
  printf 'agent-id-enum-lint: no agent ids found in %s\n' "$CAPABILITIES_FILE" >&2
  exit 2
fi

baseline=""
[[ -f "$BASELINE_FILE" ]] && baseline="$(grep -vE '^\s*(#|$)' "$BASELINE_FILE" 2>/dev/null | LC_ALL=C sort -u)"

# --- collect observed ids ----------------------------------------------------
state_files="$(find "$TARGET" -type f -name state.json \
  -not -path '*/node_modules/*' -not -path '*/.git/*' 2>/dev/null | LC_ALL=C sort)"

observed=""
qualified_count=0
scanned=0
while IFS= read -r sf; do
  [[ -n "$sf" ]] || continue
  scanned=$((scanned + 1))
  while IFS= read -r raw; do
    [[ -n "$raw" ]] || continue
    # Strip a trailing " (...)" qualifier to recover the base id.
    base="${raw%%(*}"
    base="${base%"${base##*[![:space:]]}"}"
    [[ "$raw" == *"("* ]] && qualified_count=$((qualified_count + 1))
    [[ -n "$base" ]] && observed="$observed$base"$'\n'
  done < <(grep -oE '"agent"[[:space:]]*:[[:space:]]*"[^"]*"' "$sf" 2>/dev/null |
    sed 's/.*:[[:space:]]*"//; s/"$//')
done <<EOF
$state_files
EOF

observed="$(printf '%s' "$observed" | grep -v '^$' | LC_ALL=C sort -u || true)"

# --- classify ----------------------------------------------------------------
unknown=""
while IFS= read -r id; do
  [[ -n "$id" ]] || continue
  printf '%s\n' "$registered" | grep -qxF "$id" && continue
  printf '%s\n' "$NON_AGENT_ACTORS" | tr ' ' '\n' | grep -qxF "$id" && continue
  unknown="$unknown$id"$'\n'
done <<EOF
$observed
EOF
unknown="$(printf '%s' "$unknown" | grep -v '^$' || true)"

if [[ "$UPDATE_BASELINE" == "true" ]]; then
  mkdir -p "$(dirname "$BASELINE_FILE")" 2>/dev/null || true
  {
    printf '# agent-id-enum-lint baseline (IMP-036 SCOPE-7)\n'
    printf '# Pre-existing free-text agent ids, frozen so the lint can run at all.\n'
    printf '# This file may only SHRINK. Never add a new id here to silence a failure.\n'
    printf '# Regenerate deliberately: agent-id-enum-lint.sh <repo> --update-baseline\n'
    printf '%s\n' "$unknown"
  } >"$BASELINE_FILE"
  printf 'agent-id-enum-lint: baseline updated with %s id(s) at %s\n' \
    "$(count_lines "$unknown")" "$BASELINE_FILE"
  exit 0
fi

new_unknown=""
while IFS= read -r id; do
  [[ -n "$id" ]] || continue
  printf '%s\n' "$baseline" | grep -qxF "$id" && continue
  new_unknown="$new_unknown  $id"$'\n'
done <<EOF
$unknown
EOF

stale=""
while IFS= read -r id; do
  [[ -n "$id" ]] || continue
  printf '%s\n' "$unknown" | grep -qxF "$id" && continue
  stale="$stale  $id"$'\n'
done <<EOF
$baseline
EOF

printf '[agent-id-enum-lint] scanned %d state.json file(s), %s distinct agent id(s)\n' \
  "$scanned" "$(count_lines "$observed")"

if [[ "$qualified_count" -gt 0 ]]; then
  printf '[agent-id-enum-lint] %d record(s) carry an inline "(...)" qualifier.\n' "$qualified_count"
  printf '                     Those belong in expansionReason / sweepRound sibling fields.\n'
fi

if [[ -n "$stale" ]]; then
  printf '[agent-id-enum-lint] %s baseline entr(y/ies) no longer observed - remove them:\n' \
    "$(count_lines "$stale")"
  printf '%s' "$stale"
fi

if [[ "$VERBOSE" == "true" ]]; then
  printf '[agent-id-enum-lint] registered ids: %s\n' "$(count_lines "$registered")"
  printf '[agent-id-enum-lint] baselined ids:  %s\n' "$(count_lines "$baseline")"
fi

if [[ -n "$new_unknown" ]]; then
  printf '\n[agent-id-enum-lint] FAIL: agent id(s) neither registered nor baselined:\n' >&2
  printf '%s' "$new_unknown" >&2
  printf '\nRegister the agent in %s, or use a sibling field for the qualifier.\n' \
    "${CAPABILITIES_FILE#"$ROOT_DIR"/}" >&2
  exit 1
fi

printf '[agent-id-enum-lint] OK - every agent id is registered or baselined\n'
exit 0
