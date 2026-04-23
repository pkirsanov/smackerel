#!/usr/bin/env bash
# =============================================================================
# batch-promotion-lint.sh
# =============================================================================
# Detect "batch promotion" fabrication: a single git commit (or staged change
# set) that flips multiple specs' state.json `status` fields to "done" at
# once. Real workflows complete one spec at a time with full evidence; mass
# promotions are a documented fabrication pattern (QF 2026-03-15 batch
# promoted 33 specs in commit 4dde8526; QF 2026-03-31 batch promoted 17
# specs with fabricated executionHistory in commit ec7fba88).
#
# Usage:
#   bash bubbles/scripts/batch-promotion-lint.sh [--max=N] [--ref=<git-ref>]
#                                                [--staged] [--paths=p1,p2,…]
#
# Modes:
#   --staged  (default)         Inspect git-staged state.json files
#   --ref=<git-ref>             Inspect state.json files changed in <git-ref>
#                               vs. its parent (e.g. HEAD, HEAD~1, origin/main)
#   --paths=path1,path2,…       Inspect explicit state.json paths
#
# Limits:
#   --max=N (default 1)         Maximum allowed status→"done" promotions per
#                               batch. Exit 1 when exceeded.
#
# Exit codes:
#   0 = OK (within limit, or no promotions detected)
#   1 = Batch limit exceeded — fabrication risk
#   2 = Usage error
# =============================================================================
set -euo pipefail

max_promotions=1
mode="staged"
git_ref=""
explicit_paths=""

for arg in "$@"; do
  case "$arg" in
    --max=*)
      max_promotions="${arg#--max=}"
      ;;
    --ref=*)
      mode="ref"
      git_ref="${arg#--ref=}"
      ;;
    --staged)
      mode="staged"
      ;;
    --paths=*)
      mode="paths"
      explicit_paths="${arg#--paths=}"
      ;;
    --help|-h)
      sed -n '1,40p' "${BASH_SOURCE[0]}"
      exit 0
      ;;
    *)
      echo "ERROR: unknown argument: $arg" >&2
      exit 2
      ;;
  esac
done

if ! [[ "$max_promotions" =~ ^[0-9]+$ ]]; then
  echo "ERROR: --max must be a non-negative integer" >&2
  exit 2
fi

# -----------------------------------------------------------------------------
# Collect (file, before_status, after_status) tuples
# -----------------------------------------------------------------------------
collect_status() {
  # $1 = file content (or empty if file did not exist)
  # Echoes the value of the first top-level "status" string, or empty.
  local content="$1"
  if [[ -z "$content" ]]; then
    echo ""
    return
  fi
  python3 -c 'import json, sys
raw = sys.argv[1]
try:
    data = json.loads(raw)
except Exception:
    raise SystemExit(0)
status = data.get("status")
if isinstance(status, str):
    print(status)
' "$content" 2>/dev/null || true
}

promoted_files=()
unchanged_done_files=()

inspect_one() {
  local file="$1"
  local before_content="$2"
  local after_content="$3"

  local before_status after_status
  before_status="$(collect_status "$before_content")"
  after_status="$(collect_status "$after_content")"

  if [[ "$after_status" != "done" ]]; then
    return
  fi
  if [[ "$before_status" == "done" ]]; then
    unchanged_done_files+=("$file")
    return
  fi
  promoted_files+=("$file (was='${before_status:-<missing>}' → now='done')")
}

case "$mode" in
  staged)
    if ! git rev-parse --git-dir >/dev/null 2>&1; then
      echo "ERROR: --staged requires running inside a git repository" >&2
      exit 2
    fi
    while IFS= read -r path; do
      [[ -n "$path" ]] || continue
      [[ "$(basename "$path")" == "state.json" ]] || continue
      before_content="$(git show ":0:$path" 2>/dev/null || true)"
      # The staged version is what will be committed
      staged_content="$(git show ":$path" 2>/dev/null || true)"
      # Compare to HEAD
      head_content="$(git show "HEAD:$path" 2>/dev/null || true)"
      inspect_one "$path" "$head_content" "$staged_content"
    done < <(git diff --cached --name-only --diff-filter=AM)
    ;;
  ref)
    if [[ -z "$git_ref" ]]; then
      echo "ERROR: --ref requires a git reference" >&2
      exit 2
    fi
    if git rev-parse --verify "${git_ref}^" >/dev/null 2>&1; then
      parent_ref="${git_ref}^"
      diff_range="${parent_ref}..${git_ref}"
      root_commit=0
    else
      parent_ref=""
      diff_range="$git_ref"
      root_commit=1
    fi
    if [[ "$root_commit" -eq 1 ]]; then
      while IFS= read -r path; do
        [[ -n "$path" ]] || continue
        [[ "$(basename "$path")" == "state.json" ]] || continue
        before_content=""
        after_content="$(git show "${git_ref}:$path" 2>/dev/null || true)"
        inspect_one "$path" "$before_content" "$after_content"
      done < <(git diff-tree --root --no-commit-id --name-only -r "$git_ref" -- '*state.json')
    else
      while IFS= read -r path; do
        [[ -n "$path" ]] || continue
        [[ "$(basename "$path")" == "state.json" ]] || continue
        before_content="$(git show "${parent_ref}:$path" 2>/dev/null || true)"
        after_content="$(git show "${git_ref}:$path" 2>/dev/null || true)"
        inspect_one "$path" "$before_content" "$after_content"
      done < <(git diff-tree --no-commit-id --name-only -r "$git_ref" -- '*state.json')
    fi
    ;;
  paths)
    if [[ -z "$explicit_paths" ]]; then
      echo "ERROR: --paths requires a comma-separated list" >&2
      exit 2
    fi
    IFS=',' read -ra path_list <<< "$explicit_paths"
    for path in "${path_list[@]}"; do
      [[ -f "$path" ]] || continue
      after_content="$(cat "$path")"
      head_content=""
      if git rev-parse --git-dir >/dev/null 2>&1; then
        head_content="$(git show "HEAD:$path" 2>/dev/null || true)"
      fi
      inspect_one "$path" "$head_content" "$after_content"
    done
    ;;
esac

echo "============================================================"
echo "  BUBBLES BATCH PROMOTION LINT"
echo "  Mode: $mode"
echo "  Max promotions allowed per batch: $max_promotions"
echo "  Promotions detected: ${#promoted_files[@]}"
echo "  Unchanged done states: ${#unchanged_done_files[@]}"
echo "============================================================"

if [[ ${#promoted_files[@]} -eq 0 ]]; then
  echo "✅ PASS: No new status→\"done\" promotions in this batch"
  exit 0
fi

echo ""
echo "Promotions detected:"
for entry in "${promoted_files[@]}"; do
  echo "  - $entry"
done

if [[ ${#promoted_files[@]} -le "$max_promotions" ]]; then
  echo ""
  echo "✅ PASS: ${#promoted_files[@]} promotion(s) within limit ($max_promotions)"
  exit 0
fi

echo ""
echo "🔴 BLOCK: ${#promoted_files[@]} promotions exceed batch limit ($max_promotions)"
echo "  Batch promotion of multiple specs in a single commit is a documented"
echo "  fabrication pattern. Each spec MUST be promoted in its own commit"
echo "  with its own state-transition-guard run."
echo ""
echo "  To override (requires explicit human approval), set:"
echo "    BUBBLES_BATCH_PROMOTION_OVERRIDE=1"
if [[ "${BUBBLES_BATCH_PROMOTION_OVERRIDE:-0}" == "1" ]]; then
  echo ""
  echo "⚠️  WARN: BUBBLES_BATCH_PROMOTION_OVERRIDE=1 — exiting 0 under override"
  exit 0
fi
exit 1
