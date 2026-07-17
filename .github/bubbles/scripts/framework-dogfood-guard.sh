#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

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
#   specs. Downstream and hermetic fixture repositories pass with either
#   current numbered-spec evidence (`.status == "done"`) or a proven first
#   adoption: current numbered states exist, none is done, complete local Git
#   history is available, and no reachable numbered state was ever done.
#
# The guard is source-aware. If pointed at the canonical Bubbles source
# repository, it fails when `specs/` exists and otherwise verifies that
# the validation/release evidence surfaces are present. If pointed at a
# downstream or fixture repository, it checks current state first and
# conditionally classifies repository-local history.
#
# Exit codes:
#   0  source repo has no persistent specs/ and has validation evidence
#      surfaces, OR downstream/fixture repo has current done evidence or a
#      proven first-adoption history
#   1  source repo contains persistent specs/ or lacks validation
#      evidence surfaces, OR downstream policy evidence is absent/removed
#   2  malformed / missing inputs (missing repo root, invalid argv,
#      unparseable state.json, indeterminate Git history) — diagnostic on stderr
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
#   - git     (conditional dependency for zero-current-done downstreams)
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
  0 = source evidence is valid, current downstream done evidence exists, or first adoption is proven
  1 = source/downstream policy violation is proven
  2 = malformed inputs/state or indeterminate repository history
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

# --- Find numbered-feature state.json files ------------------------------
#
# Canonical shape: specs/<NNN>-<slug>/state.json where NNN matches the
# leading-digit pattern documented in the spec/design (e.g.
# specs/900-dogfood-fixture/state.json). The direct shell glob catches only
# top-level numbered features, preserves paths with whitespace, and avoids a
# GNU-only `sort -z` dependency.

STATE_FILES=()
MALFORMED_SPECS=()
if [[ -d "$SPECS_DIR" ]]; then
  for candidate in "$SPECS_DIR"/[0-9]*-*/state.json; do
    [[ -e "$candidate" || -L "$candidate" ]] || continue
    feature_dir="${candidate%/state.json}"
    if [[ -L "$SPECS_DIR" || -L "$feature_dir" || -L "$candidate" || ! -f "$candidate" ]]; then
      MALFORMED_SPECS+=("$candidate")
      continue
    fi
    STATE_FILES+=("$candidate")
  done
fi

TOTAL_SPECS="${#STATE_FILES[@]}"

# --- Count entries whose top-level .status == "done" ---------------------

DONE_COUNT=0
DONE_SPECS=()

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
  {
    echo "G085 framework_dogfood_evidence_gate input integrity failure"
    echo "  failureCode=E085-CURRENT-STATE-MALFORMED"
    echo "  repositoryClass=downstream-or-fixture"
    echo "  malformedCurrentStates=${#MALFORMED_SPECS[@]}"
    echo "  error=${#MALFORMED_SPECS[@]} state.json path(s) failed trust or JSON validation"
    echo "  requirement=current numbered state.json files must be regular non-symbolic-link files containing valid JSON"
    echo "  malformed paths:"
    for s in "${MALFORMED_SPECS[@]}"; do
      echo "    - $s"
    done
    echo "  recipe=docs/recipes/framework-dogfood.md"
  } >&2
  exit 2
fi

# --- Decision -----------------------------------------------------------

if [[ "$DONE_COUNT" -gt 0 ]]; then
  info "repositoryClass=downstream-or-fixture specsDir=$SPECS_DIR totalSpecs=$TOTAL_SPECS doneCount=$DONE_COUNT"
  echo "PASS Gate G085 (framework_dogfood_evidence_gate) decisionCode=G085-CURRENT-DONE currentDone=$DONE_COUNT doneCount=$DONE_COUNT/$TOTAL_SPECS totalSpecs=$TOTAL_SPECS specsDir=$SPECS_DIR"
  exit 0
fi

if [[ "$TOTAL_SPECS" -eq 0 ]]; then
  {
    echo "G085 framework_dogfood_evidence_gate violation"
    echo "  failureCode=E085-NO-CURRENT-SPEC"
    echo "  repositoryClass=downstream-or-fixture"
    echo "  specsDir=$SPECS_DIR"
    echo "  currentSpecs=0"
    echo "  currentDone=0"
    echo "  numbered-feature state.json files found: 0"
    echo "  count with status==done:                 0"
    echo "  requirement=first adoption requires at least one current specs/NNN-*/state.json"
    echo "  recipe=docs/recipes/framework-dogfood.md"
  } >&2
  exit 1
fi

# --- Conditional first-adoption history classifier ----------------------

history_integrity_failure() {
  local failure_code="$1"
  local integrity="$2"
  local check="$3"
  {
    echo "G085 framework_dogfood_evidence_gate input integrity failure"
    echo "  failureCode=$failure_code"
    echo "  repositoryClass=downstream-or-fixture"
    echo "  currentSpecs=$TOTAL_SPECS"
    echo "  currentDone=0"
    echo "  historyIntegrity=$integrity"
    echo "  failedCheck=$check"
    echo "  requirement=first adoption requires complete local Git history at the exact repository root"
    echo "  remediation=restore complete local Git metadata and rerun the guard"
    echo "  recipe=docs/recipes/framework-dogfood.md"
  } >&2
  exit 2
}

if ! command -v git >/dev/null 2>&1; then
  history_integrity_failure "E085-HISTORY-UNAVAILABLE" "missing" "git executable is unavailable"
fi

if ! REQUESTED_ROOT_PHYSICAL="$(cd "$REPO_ROOT" 2>/dev/null && pwd -P)"; then
  history_integrity_failure "E085-HISTORY-UNAVAILABLE" "missing" "requested repository root cannot be resolved physically"
fi
if ! INSIDE_WORK_TREE="$(git -C "$REPO_ROOT" rev-parse --is-inside-work-tree 2>/dev/null)"; then
  history_integrity_failure "E085-HISTORY-UNAVAILABLE" "missing" "requested repository is not a Git worktree"
fi
if [[ "$INSIDE_WORK_TREE" != "true" ]]; then
  history_integrity_failure "E085-HISTORY-UNAVAILABLE" "missing" "Git did not identify the requested repository as a worktree"
fi
if ! GIT_TOP_LEVEL="$(git -C "$REPO_ROOT" rev-parse --show-toplevel 2>/dev/null)"; then
  history_integrity_failure "E085-HISTORY-UNAVAILABLE" "missing" "Git worktree root cannot be resolved"
fi
if ! GIT_TOP_LEVEL_PHYSICAL="$(cd "$GIT_TOP_LEVEL" 2>/dev/null && pwd -P)"; then
  history_integrity_failure "E085-HISTORY-UNAVAILABLE" "missing" "Git worktree root cannot be resolved physically"
fi
if [[ "$REQUESTED_ROOT_PHYSICAL" != "$GIT_TOP_LEVEL_PHYSICAL" ]]; then
  history_integrity_failure "E085-HISTORY-UNAVAILABLE" "missing" "requested repository root is not the exact Git worktree root"
fi

if ! SHALLOW_STATE="$(git -C "$REPO_ROOT" rev-parse --is-shallow-repository 2>/dev/null)"; then
  history_integrity_failure "E085-HISTORY-QUERY-FAILED" "query-failed" "Git shallow-history query failed"
fi
case "$SHALLOW_STATE" in
  true)
    history_integrity_failure "E085-HISTORY-SHALLOW" "shallow" "Git reports a shallow repository"
    ;;
  false)
    ;;
  *)
    history_integrity_failure "E085-HISTORY-QUERY-FAILED" "malformed" "Git returned an invalid shallow-history response"
    ;;
esac

HISTORY_WORKSPACE="$(mktemp -d -t bubbles-g085-XXXXXXXX 2>/dev/null || true)"
if [[ -z "$HISTORY_WORKSPACE" || ! -d "$HISTORY_WORKSPACE" ]]; then
  history_integrity_failure "E085-HISTORY-QUERY-FAILED" "query-failed" "history scratch directory could not be created"
fi

cleanup_history_workspace() {
  local cleanup_rc=$?
  trap - EXIT
  rm -rf "$HISTORY_WORKSPACE" 2>/dev/null || true
  exit "$cleanup_rc"
}
trap cleanup_history_workspace EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

HISTORY_STDERR="$HISTORY_WORKSPACE/history.stderr"
PARTIAL_CLONE_CONFIG="$HISTORY_WORKSPACE/partial-clone-config"
PROMISOR_CONFIG="$HISTORY_WORKSPACE/promisor-config"

set +e
git -C "$REPO_ROOT" config --local --get extensions.partialClone > "$PARTIAL_CLONE_CONFIG" 2> "$HISTORY_STDERR"
PARTIAL_CLONE_RC=$?
set -e
if [[ "$PARTIAL_CLONE_RC" -eq 0 ]]; then
  history_integrity_failure "E085-HISTORY-PARTIAL" "partial" "extensions.partialClone metadata is present"
fi
if [[ "$PARTIAL_CLONE_RC" -ne 1 ]]; then
  history_integrity_failure "E085-HISTORY-QUERY-FAILED" "query-failed" "partial-clone metadata query failed"
fi

set +e
git -C "$REPO_ROOT" config --local --bool --get-regexp '^remote\..*\.promisor$' > "$PROMISOR_CONFIG" 2> "$HISTORY_STDERR"
PROMISOR_CONFIG_RC=$?
set -e
if [[ "$PROMISOR_CONFIG_RC" -eq 0 ]]; then
  while IFS=' ' read -r promisor_key promisor_value; do
    if [[ "$promisor_value" == "true" ]]; then
      history_integrity_failure "E085-HISTORY-PARTIAL" "partial" "remote promisor metadata is enabled for $promisor_key"
    fi
  done < "$PROMISOR_CONFIG"
elif [[ "$PROMISOR_CONFIG_RC" -ne 1 ]]; then
  history_integrity_failure "E085-HISTORY-QUERY-FAILED" "query-failed" "promisor metadata query failed"
fi

COMMITS_FILE="$HISTORY_WORKSPACE/commits"
STATE_PATHS_FILE="$HISTORY_WORKSPACE/state-paths"
HISTORICAL_BLOB_FILE="$HISTORY_WORKSPACE/historical-state.json"
HISTORY_PATHSPEC=':(glob)specs/[0-9]*-*/state.json'

if ! GIT_NO_LAZY_FETCH=1 git -C "$REPO_ROOT" rev-list --all -- "$HISTORY_PATHSPEC" > "$COMMITS_FILE" 2> "$HISTORY_STDERR"; then
  history_integrity_failure "E085-HISTORY-QUERY-FAILED" "query-failed" "reachable commit traversal failed"
fi

is_numbered_top_level_state_path() {
  local candidate_path="$1"
  local feature_dir
  case "$candidate_path" in
    specs/[0-9]*-*/state.json)
      feature_dir="${candidate_path#specs/}"
      feature_dir="${feature_dir%/state.json}"
      [[ "$feature_dir" != */* ]]
      ;;
    *)
      return 1
      ;;
  esac
}

while IFS= read -r history_commit; do
  [[ -n "$history_commit" ]] || continue
  if ! GIT_NO_LAZY_FETCH=1 git -C "$REPO_ROOT" ls-tree -r -z --name-only "$history_commit" -- specs > "$STATE_PATHS_FILE" 2> "$HISTORY_STDERR"; then
    history_integrity_failure "E085-HISTORY-QUERY-FAILED" "query-failed" "historical tree traversal failed at commit $history_commit"
  fi

  while IFS= read -r -d '' history_path; do
    is_numbered_top_level_state_path "$history_path" || continue
    if ! GIT_NO_LAZY_FETCH=1 git -C "$REPO_ROOT" cat-file blob "$history_commit:$history_path" > "$HISTORICAL_BLOB_FILE" 2> "$HISTORY_STDERR"; then
      history_integrity_failure "E085-HISTORY-QUERY-FAILED" "query-failed" "historical blob read failed at commit $history_commit path $history_path"
    fi
    if ! jq empty "$HISTORICAL_BLOB_FILE" >/dev/null 2>&1; then
      {
        echo "G085 framework_dogfood_evidence_gate input integrity failure"
        echo "  failureCode=E085-HISTORICAL-STATE-MALFORMED"
        echo "  repositoryClass=downstream-or-fixture"
        echo "  currentSpecs=$TOTAL_SPECS"
        echo "  currentDone=0"
        echo "  historyIntegrity=malformed"
        echo "  historyCommit=$history_commit"
        echo "  historyPath=$history_path"
        echo "  requirement=reachable numbered historical state.json blobs must contain valid JSON"
        echo "  recipe=docs/recipes/framework-dogfood.md"
      } >&2
      exit 2
    fi
    if jq -e '.status == "done"' "$HISTORICAL_BLOB_FILE" >/dev/null 2>&1; then
      {
        echo "G085 framework_dogfood_evidence_gate violation"
        echo "  failureCode=E085-ESTABLISHED-DONE-REMOVED"
        echo "  repositoryClass=downstream-or-fixture"
        echo "  currentSpecs=$TOTAL_SPECS"
        echo "  currentDone=0"
        echo "  historicalDone=1"
        echo "  historyCommit=$history_commit"
        echo "  historyPath=$history_path"
        echo "  requirement=an established downstream repository must retain current top-level status done evidence"
        echo "  recipe=docs/recipes/framework-dogfood.md"
      } >&2
      exit 1
    fi
  done < "$STATE_PATHS_FILE"
done < "$COMMITS_FILE"

info "repositoryClass=downstream-or-fixture specsDir=$SPECS_DIR totalSpecs=$TOTAL_SPECS currentDone=0 historicalDone=0 historyIntegrity=complete"
echo "PASS Gate G085 (framework_dogfood_evidence_gate) decisionCode=G085-FIRST-ADOPTION currentDone=0 historicalDone=0 historyIntegrity=complete totalSpecs=$TOTAL_SPECS specsDir=$SPECS_DIR"
exit 0
