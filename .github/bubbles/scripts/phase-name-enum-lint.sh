#!/usr/bin/env bash
# phase-name-enum-lint.sh (Gate G140: phase_name_enum_integrity_gate) — constrain phase names to the workflows registry
# (IMP-052 SCOPE-3).
#
# executionHistory[].agent is enum-constrained by agent-id-enum-lint.sh. The
# phase fields beside it in the SAME record were unconstrained free text, and the
# two shipped surfaces that name phases disagree: bubbles/workflows.yaml
# registers the phase set, while agents/bubbles.bug.agent.md instructs writing
# currentPhase values the registry does not register. That contradiction is what
# let a truthful packet reach a gate no truthful record could satisfy.
#
# Two checks:
#   PACKET    — every phase named in a state.json is registered or baselined.
#   AUTHORING — every currentPhase value a shipped agent definition instructs an
#               agent to write is registered. This is the reciprocal half: the
#               contradiction becomes a finding at authoring time instead of an
#               unsatisfiable packet gate weeks later.
#
# Pre-existing names are frozen in a per-repo baseline; the lint fails ONLY on
# names neither registered nor baselined. A baselined name that disappears is
# reported as stale, so the file can only shrink.
#
# Exit: 0 clean (or baseline updated), 1 unregistered+unbaselined, 2 usage.
# There is no --skip/--force/--ignore. A newly legitimate phase is added by
# registering it in workflows.yaml, never by bypassing the check.

set -uo pipefail

GATE_ID="G140"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ "$(basename "$(dirname "$SCRIPT_DIR")")" == "bubbles" && "$(basename "$(dirname "$(dirname "$SCRIPT_DIR")")")" == ".github" ]]; then
  ROOT_DIR="$(cd "$SCRIPT_DIR/../../.." && pwd)"
  FRAMEWORK_DIR="$ROOT_DIR/.github/bubbles"
else
  ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
  FRAMEWORK_DIR="$ROOT_DIR/bubbles"
fi
WORKFLOWS_FILE="${BUBBLES_WORKFLOWS_FILE:-$FRAMEWORK_DIR/workflows.yaml}"

# Values that legitimately occupy a phase field but are not workflow phases.
NON_PHASE_VALUES="none pending null unknown"

count_lines() { printf '%s' "${1:-}" | grep -c . 2>/dev/null || true; }

usage() {
  cat <<'USAGE'
usage: phase-name-enum-lint.sh <repo-root-or-spec-dir> [--verbose]
       phase-name-enum-lint.sh <repo-root> --update-baseline

Validates that every phase named in a state.json, and every currentPhase value a
shipped agent definition instructs an agent to write, is registered in
bubbles/workflows.yaml.

There is no --skip, --force or --ignore flag.
USAGE
}

die_usage() {
  printf 'phase-name-enum-lint: %s\n' "$1" >&2
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
    -h | --help) usage; exit 0 ;;
    --skip* | --force* | --ignore* | --no-verify* | --bypass* | --allow*)
      die_usage "bypass-shaped flag '$1' is not supported and never will be" ;;
    -*) die_usage "unknown flag '$1'" ;;
    *)
      [[ -n "$TARGET" ]] && die_usage "unexpected extra argument '$1'"
      TARGET="$1"
      ;;
  esac
  shift
done

[[ -n "$TARGET" ]] || die_usage "a repo root or spec directory is required (this tool has no default surface)"
[[ -d "$TARGET" ]] || die_usage "target does not exist: $TARGET"
[[ -f "$WORKFLOWS_FILE" ]] || die_usage "workflows registry not found: $WORKFLOWS_FILE"

# Per-repo ratchet. Downstream this script installs under .github/bubbles/scripts,
# which the consuming repo may not edit, so a framework-side baseline could never
# be updated by the party entitled to update it.
resolve_baseline_root() {
  local d
  d="$(cd "$TARGET" && pwd -P)"
  if command -v git >/dev/null 2>&1 && git -C "$d" rev-parse --show-toplevel >/dev/null 2>&1; then
    git -C "$d" rev-parse --show-toplevel
    return 0
  fi
  printf '%s' "$d"
}
BASELINE_FILE="${BUBBLES_PHASE_NAME_BASELINE_FILE:-$(resolve_baseline_root)/.specify/phase-name-enum-lint.baseline}"

registered=""
if command -v yq >/dev/null 2>&1; then
  registered="$(yq -r '.phases | keys | .[]' "$WORKFLOWS_FILE" 2>/dev/null | LC_ALL=C sort -u)"
fi
if [[ -z "$registered" ]]; then
  registered="$(awk '
    /^phases:[[:space:]]*$/ { inp = 1; next }
    inp && /^[^[:space:]]/ { inp = 0 }
    inp && /^  [a-z][a-z0-9-]*:[[:space:]]*$/ {
      line = $0; sub(/^  /, "", line); sub(/:[[:space:]]*$/, "", line); print line
    }
  ' "$WORKFLOWS_FILE" | LC_ALL=C sort -u)"
fi
if [[ -z "$registered" ]]; then
  printf 'phase-name-enum-lint: no phases found in %s\n' "$WORKFLOWS_FILE" >&2
  exit 2
fi

baseline=""
[[ -f "$BASELINE_FILE" ]] && baseline="$(grep -vE '^[[:space:]]*(#|$)' "$BASELINE_FILE" 2>/dev/null | LC_ALL=C sort -u)"

state_files="$(find "$TARGET" -type f -name state.json \
  -not -path '*/node_modules/*' -not -path '*/.git/*' 2>/dev/null | LC_ALL=C sort)"

observed=""
scanned=0
while IFS= read -r sf; do
  [[ -n "$sf" ]] || continue
  scanned=$((scanned + 1))
  while IFS= read -r raw; do
    [[ -n "$raw" ]] || continue
    observed="$observed$raw"$'\n'
  done < <(grep -oE '"(currentPhase|phase)"[[:space:]]*:[[:space:]]*"[^"]*"' "$sf" 2>/dev/null |
    sed 's/.*:[[:space:]]*"//; s/"$//')
  while IFS= read -r raw; do
    [[ -n "$raw" ]] || continue
    observed="$observed$raw"$'\n'
  done < <(awk '
    /"(phasesExecuted|completedPhaseClaims)"[[:space:]]*:[[:space:]]*\[/ { ina = 1 }
    ina {
      line = $0
      while (match(line, /"[a-zA-Z][a-zA-Z0-9_-]*"/)) {
        tok = substr(line, RSTART + 1, RLENGTH - 2)
        if (tok != "phasesExecuted" && tok != "completedPhaseClaims") print tok
        line = substr(line, RSTART + RLENGTH)
      }
      if (index($0, "]") > 0) ina = 0
    }
  ' "$sf" 2>/dev/null)
done <<EOF
$state_files
EOF
observed="$(printf '%s' "$observed" | grep -v '^$' | LC_ALL=C sort -u || true)"

authored=""
authored_detail=""
# Resolved from TARGET, not from the framework root: linting a repo must check
# THAT repo's authoring surfaces. Resolving from the framework root made every
# fixture inherit the framework's own agent definitions.
agents_dir="$TARGET/agents"
[[ -d "$agents_dir" ]] || agents_dir="$TARGET/.github/agents"
if [[ -d "$agents_dir" ]]; then
  while IFS= read -r af; do
    [[ -n "$af" ]] || continue
    while IFS= read -r hit; do
      [[ -n "$hit" ]] || continue
      ln="${hit%%:*}"
      val="${hit#*:}"
      [[ -n "$val" ]] || continue
      authored="$authored$val"$'\n'
      authored_detail="$authored_detail  $val  ($(basename "$af"):$ln)"$'\n'
    done < <(grep -noE 'currentPhase[^a-zA-Z0-9]{1,6}"[a-z][a-z0-9-]*"' "$af" 2>/dev/null |
      sed -E 's/^([0-9]+):.*"([a-z][a-z0-9-]*)"$/\1:\2/')
  done < <(find "$agents_dir" -maxdepth 1 -type f -name '*.agent.md' 2>/dev/null | LC_ALL=C sort)
fi
authored="$(printf '%s' "$authored" | grep -v '^$' | LC_ALL=C sort -u || true)"

classify_unknown() {
  local candidates="$1" out="" name
  while IFS= read -r name; do
    [[ -n "$name" ]] || continue
    printf '%s\n' "$registered" | grep -qxF "$name" && continue
    printf '%s\n' "$NON_PHASE_VALUES" | tr ' ' '\n' | grep -qxF "$name" && continue
    out="$out$name"$'\n'
  done <<EOF
$candidates
EOF
  printf '%s' "$out" | grep -v '^$' || true
}

unknown_packet="$(classify_unknown "$observed")"
unknown_authored="$(classify_unknown "$authored")"
unknown_all="$(printf '%s\n%s' "$unknown_packet" "$unknown_authored" | grep -v '^$' | LC_ALL=C sort -u || true)"

if [[ "$UPDATE_BASELINE" == "true" ]]; then
  mkdir -p "$(dirname "$BASELINE_FILE")" 2>/dev/null || true
  {
    printf '# phase-name-enum-lint baseline (IMP-052 SCOPE-3)\n'
    printf '# Pre-existing free-text phase names, frozen so the lint can run at all.\n'
    printf '# This file may only SHRINK. Never add a new name here to silence a failure.\n'
    printf '# The durable fix is to register the phase in bubbles/workflows.yaml, or to\n'
    printf '# change the authoring surface to name a registered phase.\n'
    printf '%s\n' "$unknown_all"
  } >"$BASELINE_FILE"
  printf 'phase-name-enum-lint: baseline updated with %s name(s) at %s\n' \
    "$(count_lines "$unknown_all")" "$BASELINE_FILE"
  exit 0
fi

new_unknown=""
while IFS= read -r name; do
  [[ -n "$name" ]] || continue
  printf '%s\n' "$baseline" | grep -qxF "$name" && continue
  new_unknown="$new_unknown  $name"$'\n'
done <<EOF
$unknown_all
EOF

stale=""
while IFS= read -r name; do
  [[ -n "$name" ]] || continue
  printf '%s\n' "$unknown_all" | grep -qxF "$name" && continue
  stale="$stale  $name"$'\n'
done <<EOF
$baseline
EOF

printf '[phase-name-enum-lint] scanned %d state.json file(s), %s packet phase(s), %s authored phase(s)\n' \
  "$scanned" "$(count_lines "$observed")" "$(count_lines "$authored")"

if [[ -n "$unknown_authored" ]]; then
  printf '[phase-name-enum-lint] %s authoring surface value(s) name an unregistered phase:\n' \
    "$(count_lines "$unknown_authored")"
  while IFS= read -r name; do
    [[ -n "$name" ]] || continue
    printf '%s' "$authored_detail" | grep -E "^  ${name}  " || true
  done <<EOF
$unknown_authored
EOF
fi

if [[ -n "$stale" ]]; then
  printf '[phase-name-enum-lint] %s baseline entr(y/ies) no longer observed - remove them:\n' \
    "$(count_lines "$stale")"
  printf '%s' "$stale"
fi

if [[ "$VERBOSE" == "true" ]]; then
  printf '[phase-name-enum-lint] registered phases: %s\n' "$(count_lines "$registered")"
  printf '[phase-name-enum-lint] baselined names:   %s\n' "$(count_lines "$baseline")"
fi

if [[ -n "$new_unknown" ]]; then
  printf '\n[phase-name-enum-lint] FAIL [%s]: phase name(s) neither registered nor baselined:\n' "$GATE_ID" >&2
  printf '%s' "$new_unknown" >&2
  printf '\nRegister the phase in %s, or name a registered phase at the authoring surface.\n' \
    "${WORKFLOWS_FILE#"$ROOT_DIR"/}" >&2
  exit 1
fi

printf '[phase-name-enum-lint] OK [%s] - every phase name is registered or baselined\n' "$GATE_ID"
exit 0
