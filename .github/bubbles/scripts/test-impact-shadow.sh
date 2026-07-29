#!/usr/bin/env bash
# bubbles/scripts/test-impact-shadow.sh
#
# SHADOW-MODE test-impact reporting. Reports what a code-index-derived test
# subset WOULD select for the current change. It NEVER selects, skips, filters,
# or executes anything.
#
# WHY SHADOW MODE, AND WHY IT STAYS THAT WAY FOR A WHILE
# ------------------------------------------------------
# Deriving "which tests can this diff reach" from a code index is the single
# biggest measurable win available from an index — and the single most dangerous
# thing to wire directly into a gate.
#
# The failure is silent by construction: if the graph misses one edge, the
# subset comes back green while the full suite would have failed, and NOTHING in
# the output looks wrong. A skipped test and a passing test are indistinguishable
# in a summary line.
#
# The graph is a tree-sitter SYMBOL graph, not a build graph. Bazel/Nx/Turbo
# derive affected-ness from the dependency graph the build actually uses; this
# infers it from parsed references. Known blind spots, none exotic:
#
#   - macro-generated call sites (Rust proc macros, Go generate, TS decorators)
#   - build scripts that alter compilation (build.rs, codegen steps)
#   - fixtures and golden files loaded by PATH at runtime, never imported
#   - reflection / dynamic dispatch / interface satisfaction without a literal ref
#   - test harnesses that discover cases at runtime rather than by import
#
# So: this script produces evidence for a divergence log. Promotion to gating is
# a SEPARATE, later decision that requires a clean record over a meaningful
# stretch — and even then the full-suite fallback stays permanently.
#
# CONTRACT
#   Exit 0 — a report was produced (including "no index", which reports degraded)
#   Exit 2 — usage error
#   It has no exit code meaning "safe to skip tests", because that is not a
#   judgment this script is permitted to make.
#
# USAGE
#   test-impact-shadow.sh [--repo-root PATH] [--base REF] [--json]
#                         [--test-pattern GLOB]...
#
#   --base REF        compare against REF (default: unstaged+staged working tree)
#   --test-pattern    repeatable; defaults cover Rust/Go/TS/JS/Python conventions

set -uo pipefail

REPO_ROOT="$PWD"
BASE_REF=""
AS_JSON=0
TEST_PATTERNS=()

while [ $# -gt 0 ]; do
  case "$1" in
    --repo-root)    REPO_ROOT="${2:?--repo-root needs a path}"; shift 2 ;;
    --base)         BASE_REF="${2:?--base needs a ref}"; shift 2 ;;
    --test-pattern) TEST_PATTERNS+=("${2:?--test-pattern needs a glob}"); shift 2 ;;
    --json)         AS_JSON=1; shift ;;
    -h|--help)      sed -n '2,45p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *)              echo "test-impact-shadow: unknown argument '$1'" >&2; exit 2 ;;
  esac
done

if [ "${#TEST_PATTERNS[@]}" -eq 0 ]; then
  TEST_PATTERNS=(
    '*_test.go' '*_test.rs' '*test*.rs' 'tests/*'
    '*.test.ts' '*.test.tsx' '*.spec.ts' '*.spec.tsx' '*.spec.mjs'
    'test_*.py' '*_test.py' '*Spec.scala' '*Test.scala'
  )
fi

[ -d "$REPO_ROOT" ] || { echo "test-impact-shadow: no such repo root '$REPO_ROOT'" >&2; exit 2; }
cd "$REPO_ROOT" || exit 2

RESOLVER=".github/bubbles/scripts/codeindex-resolve.sh"
[ -f "$RESOLVER" ] || RESOLVER="bubbles/scripts/codeindex-resolve.sh"

emit_degraded() {
  local reason="$1"
  if [ "$AS_JSON" -eq 1 ]; then
    printf '{"mode":"shadow","degraded":true,"reason":"%s","changedFiles":0,"affectedTests":0,"totalTests":0}\n' "$reason"
  else
    echo "test-impact-shadow: DEGRADED — $reason"
    echo "  No subset was derived. The full suite remains the only correct plan."
  fi
  exit 0
}

# --- resolve the adapter -----------------------------------------------------
[ -f "$RESOLVER" ] || emit_degraded "no codeindex resolver in this repo"
ADAPTER_LINE="$(bash "$RESOLVER" --repo-root . 2>/dev/null | grep '^adapterPath=' || true)"
ADAPTER="${ADAPTER_LINE#adapterPath=}"
[ -n "$ADAPTER" ] && [ -f "$ADAPTER" ] || emit_degraded "code index adapter unresolved"

case "$ADAPTER" in
  *none.sh) emit_degraded "adapter is 'none' (no provider configured)" ;;
esac

export CODEINDEX_ROOT="$REPO_ROOT"

# --- freshness gate ----------------------------------------------------------
# A stale index yields a stale subset, and a stale subset looks exactly like a
# fresh one. Self-heal, and degrade honestly if that is not possible.
bash "$ADAPTER" freshness >/dev/null 2>&1
fresh_rc=$?
if [ "$fresh_rc" -eq 2 ]; then
  bash "$ADAPTER" sync >/dev/null 2>&1 || emit_degraded "index is STALE and sync failed"
  bash "$ADAPTER" freshness >/dev/null 2>&1 || emit_degraded "index still STALE after sync"
elif [ "$fresh_rc" -ne 0 ]; then
  emit_degraded "cannot determine index freshness (adapter exit $fresh_rc)"
fi

# --- changed files -----------------------------------------------------------
if [ -n "$BASE_REF" ]; then
  CHANGED="$(git diff --name-only "$BASE_REF" 2>/dev/null || true)"
else
  CHANGED="$( { git diff --name-only 2>/dev/null; git diff --name-only --cached 2>/dev/null; } | sort -u )"
fi
CHANGED="$(printf '%s\n' "$CHANGED" | grep -v '^$' || true)"
CHANGED_N=$(printf '%s\n' "$CHANGED" | grep -c . || true)

[ "$CHANGED_N" -gt 0 ] || emit_degraded "no changed files to analyze"

# --- total test inventory ----------------------------------------------------
TOTAL_TESTS=0
for pat in "${TEST_PATTERNS[@]}"; do
  n=$(git ls-files "$pat" 2>/dev/null | grep -c . || true)
  TOTAL_TESTS=$((TOTAL_TESTS + n))
done

# --- derived affected set ----------------------------------------------------
# shellcheck disable=SC2086
AFFECTED_JSON="$(printf '%s\n' "$CHANGED" | xargs -r bash "$ADAPTER" affected 2>/dev/null)"
if [ -z "$AFFECTED_JSON" ]; then
  emit_degraded "adapter returned nothing for affected"
fi

AFFECTED_N="$(printf '%s' "$AFFECTED_JSON" | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
    print(len(d) if isinstance(d, list) else -1)
except Exception:
    print(-1)
' 2>/dev/null || echo -1)"

[ "$AFFECTED_N" -ge 0 ] || emit_degraded "adapter affected output was not a JSON array"

# --- report ------------------------------------------------------------------
if [ "$TOTAL_TESTS" -gt 0 ] && [ "$AFFECTED_N" -le "$TOTAL_TESTS" ]; then
  WOULD_SKIP=$((TOTAL_TESTS - AFFECTED_N))
  PCT=$(( WOULD_SKIP * 100 / TOTAL_TESTS ))
else
  WOULD_SKIP=0
  PCT=0
fi

if [ "$AS_JSON" -eq 1 ]; then
  printf '{"mode":"shadow","degraded":false,"adapter":"%s","changedFiles":%s,"totalTests":%s,"affectedTests":%s,"wouldSkip":%s,"wouldSkipPct":%s,"gating":false}\n' \
    "$(basename "$ADAPTER" .sh)" "$CHANGED_N" "$TOTAL_TESTS" "$AFFECTED_N" "$WOULD_SKIP" "$PCT"
  exit 0
fi

echo "test-impact-shadow (REPORT ONLY — nothing was skipped)"
echo ""
echo "  adapter:        $(basename "$ADAPTER" .sh)"
echo "  changed files:  $CHANGED_N"
echo "  test inventory: $TOTAL_TESTS"
echo "  derived subset: $AFFECTED_N"
echo "  would skip:     $WOULD_SKIP  (${PCT}%)"
echo ""
if [ "$AFFECTED_N" -eq 0 ]; then
  echo "  ⚠️  The derived subset is EMPTY while $CHANGED_N file(s) changed."
  echo "      Treat that as a graph gap, not as 'no tests needed'."
fi
echo "  This is evidence for a divergence log, NOT a plan."
echo "  Run the FULL suite. If the full suite fails while this subset would have"
echo "  passed, the graph missed an edge — record it before trusting the subset."
exit 0
