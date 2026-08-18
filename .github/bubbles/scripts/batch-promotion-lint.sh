#!/usr/bin/env bash
# =============================================================================
# batch-promotion-lint.sh
# =============================================================================
# Gate G078 — batch_promotion_limit_gate.
#
# Detect "batch promotion" fabrication: a single git commit (or staged change
# set) that flips multiple specs' state.json `status` fields to "done" at
# once. Real workflows complete one spec at a time with full evidence; mass
# promotions are a documented fabrication pattern (one downstream batch
# promoted 33 specs in a single commit; another batch promoted 17
# specs with fabricated executionHistory in one commit).
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
#
# Override (hardened — NOT a bare replayable flag):
#   BUBBLES_BATCH_PROMOTION_OVERRIDE="<actor>:<expiryEpoch>:<sha>"
#   Honored ONLY when the token is well-formed, unexpired (expiry >= now), and
#   its <sha> is a prefix of the current target commit (staged/paths → HEAD,
#   ref → the inspected ref). Every honored override is appended to an
#   append-only audit ledger (BUBBLES_BATCH_PROMOTION_LEDGER; default
#   <repo>/.specify/runtime/batch-promotion-override-ledger.jsonl). A bare "1"
#   is refused, an expired token is refused, and a token bound to a different
#   sha is refused — so the flag can no longer be trivially replayed.
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

# -----------------------------------------------------------------------------
# Override hardening (BUBBLES_BATCH_PROMOTION_OVERRIDE)
# -----------------------------------------------------------------------------
# The override is NOT a bare replayable flag. It requires a token that binds an
# actor, an expiry (epoch seconds), and the target commit sha the promotion is
# based on:  BUBBLES_BATCH_PROMOTION_OVERRIDE="<actor>:<expiryEpoch>:<sha>".
# The token is REFUSED when malformed, expired, or bound to a sha that does not
# match the current target commit. A legacy bare "1" no longer works. Every
# honored override appends an append-only audit line to the ledger.

resolve_target_sha() {
  # Echo the commit sha the current batch is based on, or empty if unavailable.
  git rev-parse --git-dir >/dev/null 2>&1 || { echo ""; return; }
  case "$mode" in
    ref) git rev-parse --verify --quiet "${git_ref}^{commit}" 2>/dev/null || echo "" ;;
    *) git rev-parse --verify --quiet "HEAD^{commit}" 2>/dev/null || echo "" ;;
  esac
}

default_override_ledger() {
  local root
  if root="$(git rev-parse --show-toplevel 2>/dev/null)" && [[ -n "$root" ]]; then
    echo "$root/.specify/runtime/batch-promotion-override-ledger.jsonl"
  else
    echo "$PWD/.specify/runtime/batch-promotion-override-ledger.jsonl"
  fi
}

# Returns 0 (honored) or 1 (not honored / refused). Prints the reason on refusal.
honor_batch_promotion_override() {
  local raw="${BUBBLES_BATCH_PROMOTION_OVERRIDE:-}"
  [[ -n "$raw" && "$raw" != "0" ]] || return 1

  if [[ "$raw" != *:*:* ]]; then
    echo "🔴 REFUSED: BUBBLES_BATCH_PROMOTION_OVERRIDE is not a valid token." >&2
    echo "   A bare value (e.g. '1') is NOT accepted. Use:" >&2
    echo "     BUBBLES_BATCH_PROMOTION_OVERRIDE=<actor>:<expiryEpoch>:<sha>" >&2
    return 1
  fi

  local actor rest expiry token_sha
  actor="${raw%%:*}"
  rest="${raw#*:}"
  expiry="${rest%%:*}"
  token_sha="${rest#*:}"

  if [[ -z "$actor" || -z "$expiry" || -z "$token_sha" || "$token_sha" == *:* ]]; then
    echo "🔴 REFUSED: override token must be exactly <actor>:<expiryEpoch>:<sha>." >&2
    return 1
  fi
  if ! [[ "$expiry" =~ ^[0-9]+$ ]]; then
    echo "🔴 REFUSED: override expiry '$expiry' is not an epoch integer." >&2
    return 1
  fi
  local now
  now="$(date -u +%s)"
  if (( expiry < now )); then
    echo "🔴 REFUSED: override token EXPIRED (expiry=$expiry < now=$now)." >&2
    return 1
  fi
  if [[ ${#token_sha} -lt 7 ]]; then
    echo "🔴 REFUSED: override sha '$token_sha' too short (need >=7 hex chars)." >&2
    return 1
  fi
  local target_sha
  target_sha="$(resolve_target_sha)"
  if [[ -z "$target_sha" ]]; then
    echo "🔴 REFUSED: cannot resolve a target commit sha to bind the override to." >&2
    return 1
  fi
  if [[ "$target_sha" != "$token_sha"* ]]; then
    echo "🔴 REFUSED: override sha '$token_sha' does not match target commit '${target_sha:0:12}'." >&2
    return 1
  fi

  # Honored — binding an append-only audit line is MANDATORY. If the ledger
  # cannot be written, the override is refused (no silent unaudited bypass).
  local ledger ts
  ledger="${BUBBLES_BATCH_PROMOTION_LEDGER:-$(default_override_ledger)}"
  ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  if ! mkdir -p "$(dirname "$ledger")" 2>/dev/null; then
    echo "🔴 REFUSED: cannot create ledger dir for $ledger — refusing to honor override." >&2
    return 1
  fi
  if ! printf '{"ts":"%s","actor":"%s","targetSha":"%s","tokenSha":"%s","expiry":%s,"mode":"%s","promotions":%s}\n' \
    "$ts" "$actor" "$target_sha" "$token_sha" "$expiry" "$mode" "${#promoted_files[@]}" >> "$ledger"; then
    echo "🔴 REFUSED: cannot append audit line to $ledger — refusing to honor override." >&2
    return 1
  fi

  echo ""
  echo "⚠️  WARN: BUBBLES_BATCH_PROMOTION_OVERRIDE honored — actor='$actor' sha='${target_sha:0:12}' expiry=$expiry"
  echo "   Append-only audit line recorded: $ledger"
  return 0
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
      root_commit=0
    else
      parent_ref=""
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
echo "  To override (requires explicit, expiring, sha-bound human approval), set:"
echo "    BUBBLES_BATCH_PROMOTION_OVERRIDE=<actor>:<expiryEpoch>:<sha>"
echo "    (a bare '1' is no longer accepted; the token binds actor+expiry+target"
echo "     sha, is refused when expired or sha-mismatched, and is audited)"
if honor_batch_promotion_override; then
  exit 0
fi
exit 1
