#!/usr/bin/env bash
set -euo pipefail

# framework-dogfood-guard.sh
#
# Gate G085 — framework_dogfood_evidence_gate.
#
# Mechanically enforces the framework dogfooding rule documented in
# `docs/recipes/framework-dogfood.md`:
#
#   The Bubbles source repository MUST NOT keep a persistent `specs/`
#   tree. Its dogfood evidence comes from framework validation,
#   hermetic selftests, the release manifest, and downstream/fixture
#   specs. Downstream and hermetic fixture repositories may still use
#   the traditional numbered-spec evidence model: at least one
#   `specs/[0-9]*-*/state.json` file with top-level `.status == "done"`.
#
# The guard is source-aware. If pointed at the canonical Bubbles source
# repository, it fails when `specs/` exists and otherwise verifies that
# the validation/release evidence surfaces are present. If pointed at a
# downstream or fixture repository, it scans `specs/` for done specs.
#
# Exit codes:
#   0  source repo has no persistent specs/ and has validation evidence
#      surfaces, OR downstream/fixture repo has at least one done spec
#   1  source repo contains persistent specs/ or lacks validation
#      evidence surfaces, OR downstream/fixture repo has zero done specs
#   2  malformed / missing inputs (missing repo root, invalid argv,
#      unparseable state.json) — diagnostic on stderr
#
# Usage:
#   bash bubbles/scripts/framework-dogfood-guard.sh [--repo-root <path>] [--quiet]
#
# Inputs:
#   --repo-root <path>  Optional. Path to the Bubbles repo root. When
#                       omitted, falls back to the BUBBLES_REPO_ROOT
#                       env var, then walks upward from $PWD looking
#                       for a `.specify/memory` directory (same
#                       resolution pattern as the sibling guards).
#   --quiet             Suppress informational stdout on success.
#   -h, --help          Print this usage and exit.
#
# Dependencies:
#   - jq      (hard dependency for downstream/fixture spec counting)
#   - find    (POSIX)
#
# Reference:
#   docs/recipes/framework-dogfood.md
#   docs/Framework_Convergence_Health.md

QUIET="false"
REPO_ROOT_FLAG=""

usage() {
  cat <<'EOF'
Usage: bash bubbles/scripts/framework-dogfood-guard.sh [--repo-root <path>] [--quiet]

Optional:
  --repo-root <path>  Bubbles repo root (defaults to $BUBBLES_REPO_ROOT
                      or auto-detected via .specify/memory lookup).
  --quiet             Suppress informational stdout; the final PASS or
                      VIOLATION line is still emitted (stdout on pass,
                      stderr on fail).
  -h, --help          Print this usage and exit.

Exit codes:
  0 = source repo has no persistent specs/ and evidence surfaces exist, or downstream/fixture done spec exists
  1 = source repo contains specs/ or lacks evidence surfaces, or downstream/fixture has zero done specs
  2 = malformed inputs, missing repo root, or unparseable state.json
EOF
}

# --- Argument parsing ----------------------------------------------------

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --quiet)
      QUIET="true"
      shift
      ;;
    --repo-root)
      shift
      if [[ $# -eq 0 ]]; then
        echo "framework-dogfood-guard: --repo-root requires a path argument" >&2
        usage >&2
        exit 2
      fi
      REPO_ROOT_FLAG="$1"
      shift
      ;;
    --repo-root=*)
      REPO_ROOT_FLAG="${1#--repo-root=}"
      shift
      ;;
    --*)
      echo "framework-dogfood-guard: unknown flag: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      echo "framework-dogfood-guard: unexpected positional argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

info() {
  if [[ "$QUIET" != "true" ]]; then
    echo "framework-dogfood-guard: $*"
  fi
}

# --- Repo root resolution ------------------------------------------------

resolve_repo_root() {
  if [[ -n "$REPO_ROOT_FLAG" ]]; then
    printf '%s' "$REPO_ROOT_FLAG"
    return 0
  fi
  if [[ -n "${BUBBLES_REPO_ROOT:-}" ]]; then
    printf '%s' "$BUBBLES_REPO_ROOT"
    return 0
  fi
  local dir
  dir="$(pwd)"
  while [[ "$dir" != "/" ]]; do
    if [[ -d "$dir/.specify/memory" ]]; then
      printf '%s' "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done
  return 1
}

REPO_ROOT="$(resolve_repo_root || true)"
if [[ -z "$REPO_ROOT" ]]; then
  echo "framework-dogfood-guard: unable to resolve repo root" >&2
  echo "  Pass --repo-root <path>, set BUBBLES_REPO_ROOT explicitly, or" >&2
  echo "  run from inside a Bubbles repo (one with .specify/memory)." >&2
  exit 2
fi

if [[ ! -d "$REPO_ROOT" ]]; then
  echo "framework-dogfood-guard: repo root does not exist: $REPO_ROOT" >&2
  exit 2
fi

SPECS_DIR="$REPO_ROOT/specs"

is_bubbles_source_repo() {
  [[ -f "$REPO_ROOT/install.sh" ]] \
    && [[ -f "$REPO_ROOT/VERSION" ]] \
    && [[ -f "$REPO_ROOT/bubbles/scripts/framework-validate.sh" ]] \
    && [[ -d "$REPO_ROOT/agents" ]]
}

if is_bubbles_source_repo; then
  missing_surfaces=()
  [[ -f "$REPO_ROOT/bubbles/release-manifest.json" ]] || missing_surfaces+=("bubbles/release-manifest.json")
  [[ -x "$REPO_ROOT/bubbles/scripts/framework-validate.sh" ]] || missing_surfaces+=("bubbles/scripts/framework-validate.sh")
  [[ -x "$REPO_ROOT/bubbles/scripts/framework-dogfood-guard-selftest.sh" ]] || missing_surfaces+=("bubbles/scripts/framework-dogfood-guard-selftest.sh")
  if ! grep -Fq 'Framework dogfood guard selftest' "$REPO_ROOT/bubbles/scripts/framework-validate.sh" 2>/dev/null; then
    missing_surfaces+=("framework-validate wiring for Framework dogfood guard selftest")
  fi

  if [[ -d "$SPECS_DIR" ]]; then
    {
      echo "G085 framework_dogfood_evidence_gate violation"
      echo "  repositoryClass:   Bubbles source"
      echo "  specsDir:          $SPECS_DIR"
      echo "  requirement:       the Bubbles source repository MUST NOT contain persistent specs/"
      echo "  evidenceModel:     framework validation, hermetic selftests, release manifest, downstream/fixture specs"
      echo "  recipe:            docs/recipes/framework-dogfood.md"
      echo "  remediation:       migrate durable behavior into docs/framework assets and remove specs/ from the source repo"
    } >&2
    exit 1
  fi

  if [[ "${#missing_surfaces[@]}" -gt 0 ]]; then
    {
      echo "G085 framework_dogfood_evidence_gate violation"
      echo "  repositoryClass:   Bubbles source"
      echo "  requirement:       source dogfood evidence surfaces must exist when specs/ is absent"
      echo "  missing surfaces:"
      for surface in "${missing_surfaces[@]}"; do
        echo "    - $surface"
      done
      echo "  recipe:            docs/recipes/framework-dogfood.md"
    } >&2
    exit 1
  fi

  info "repositoryClass=Bubbles source specsDir=$SPECS_DIR persistentSpecs=absent"
  echo "PASS Gate G085 (framework_dogfood_evidence_gate) — source repo has no persistent specs/; validation/selftest/release evidence surfaces are present"
  exit 0
fi

# --- jq dependency check -------------------------------------------------

if ! command -v jq >/dev/null 2>&1; then
  echo "framework-dogfood-guard: jq is required but not found in PATH" >&2
  exit 2
fi

# A missing specs/ directory is NOT a parse error — it is a textbook
# zero-done condition for downstream/fixture repositories. Treat it as
# count=0 and fall through to the violation path below.

# --- Find numbered-feature state.json files ------------------------------
#
# Canonical shape: specs/<NNN>-<slug>/state.json where NNN matches the
# leading-digit pattern documented in the spec/design (e.g.
# specs/900-dogfood-fixture/state.json). We deliberately
# use `find -maxdepth 2` so we only catch top-level numbered features
# (per-scope state.json files, if any, are NOT counted).

STATE_FILES=()
if [[ -d "$SPECS_DIR" ]]; then
  while IFS= read -r -d '' candidate; do
    STATE_FILES+=("$candidate")
  done < <(find "$SPECS_DIR" -mindepth 2 -maxdepth 2 \
              -type f -name state.json \
              -path "$SPECS_DIR/[0-9]*-*/state.json" \
              -print0 2>/dev/null | sort -z)
fi

TOTAL_SPECS="${#STATE_FILES[@]}"

# --- Count entries whose top-level .status == "done" ---------------------

DONE_COUNT=0
DONE_SPECS=()
MALFORMED_SPECS=()

for state_file in "${STATE_FILES[@]}"; do
  if ! jq empty "$state_file" >/dev/null 2>&1; then
    MALFORMED_SPECS+=("$state_file")
    continue
  fi
  status_value="$(jq -r '.status // ""' "$state_file" 2>/dev/null || echo "")"
  if [[ "$status_value" == "done" ]]; then
    DONE_COUNT=$((DONE_COUNT + 1))
    DONE_SPECS+=("$state_file")
  fi
done

if [[ "${#MALFORMED_SPECS[@]}" -gt 0 ]]; then
  echo "framework-dogfood-guard: ${#MALFORMED_SPECS[@]} state.json file(s) failed to parse:" >&2
  for s in "${MALFORMED_SPECS[@]}"; do
    echo "  - $s" >&2
  done
  exit 2
fi

# --- Decision -----------------------------------------------------------

if [[ "$DONE_COUNT" -lt 1 ]]; then
  {
    echo "G085 framework_dogfood_evidence_gate violation"
    echo "  repositoryClass:    downstream-or-fixture"
    echo "  specsDir:           $SPECS_DIR"
    echo "  numbered-feature state.json files found: $TOTAL_SPECS"
    echo "  count with status==done:                 $DONE_COUNT"
    echo "  requirement:        downstream/fixture dogfood evidence needs at least one specs/NNN-*/state.json with top-level \"status\": \"done\""
    echo "  recipe:             docs/recipes/framework-dogfood.md"
    echo "  remediation:        certify at least one downstream or fixture spec to done, or run against the Bubbles source repo where persistent specs/ is forbidden"
    if [[ "$TOTAL_SPECS" -gt 0 ]]; then
      echo "  candidate specs currently in-flight:"
      for s in "${STATE_FILES[@]}"; do
        cur_status="$(jq -r '.status // "<missing>"' "$s" 2>/dev/null || echo "<unreadable>")"
        echo "    - $s  (status=$cur_status)"
      done
    fi
  } >&2
  exit 1
fi

info "repositoryClass=downstream-or-fixture specsDir=$SPECS_DIR totalSpecs=$TOTAL_SPECS doneCount=$DONE_COUNT"
echo "PASS Gate G085 (framework_dogfood_evidence_gate) — downstream/fixture doneCount=$DONE_COUNT/$TOTAL_SPECS, specsDir=$SPECS_DIR"
exit 0
