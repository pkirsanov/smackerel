#!/usr/bin/env bash
# bubbles/adapters/codeindex/codegraph.sh — CodeGraph code-index adapter.
#
# Wraps the `codegraph` CLI (MIT, 100% local, tree-sitter -> SQLite/FTS5; no
# API key, no embeddings, no network) and NORMALIZES its output to the
# canonical codeindex shapes. The operator MUST make `codegraph` resolvable —
# either on PATH or via CODEINDEX_CODEGRAPH_BIN. NO default install path, NO
# auto-install, NO network fetch — fail-fast instead.
#
# Verbs (all 5 are mandatory per the codeindex adapter contract):
#   symbols  <query>   → `codegraph query`    → JSON ARRAY of symbol records
#   impact   <symbol>  → `codegraph impact`   → JSON ARRAY of affected records
#   affected <file>... → `codegraph affected` → JSON ARRAY of test file paths
#   routes             → `codegraph query --kind route` → JSON ARRAY of routes
#   status             → `codegraph status`   → JSON MAP of index health
#
# Output: normalized JSON to stdout. Adapter failure exits 1; the framework
# treats that as "code index unavailable", NOT as a framework failure. A
# consumer MUST degrade to its existing behavior on exit 1, never block on it.
#
# Shape selftest (NO index, NO provider required):
#   codegraph.sh selftest <verb>
# emits the canonical shape for <verb> so a shape lint can validate this
# adapter offline, exactly as the observability adapters do.
#
# Provider notes (verified 2026-07-28 against codegraph 1.5.0):
#   - `codegraph` does NOT parse shell/bash. A shell-dominant repository gains
#     little here; prefer a provider whose grammar set covers the repo.
#   - Telemetry is ON by default upstream. This adapter forces it OFF on every
#     invocation (CODEGRAPH_TELEMETRY=0, DO_NOT_TRACK=1) so framework use never
#     emits usage data the operator did not opt into.
#   - CODEGRAPH_NO_DAEMON=1 keeps invocations self-contained (no shared
#     background server), which matters in multi-root workspaces.

set -euo pipefail

CODEINDEX_ROOT="${CODEINDEX_ROOT:-$PWD}"
CG_BIN="${CODEINDEX_CODEGRAPH_BIN:-codegraph}"

# Privacy + isolation defaults, applied to every provider invocation.
export CODEGRAPH_TELEMETRY=0
export DO_NOT_TRACK=1
export CODEGRAPH_NO_DAEMON=1

die() {
  echo "[codegraph][ERROR] $1" >&2
  exit 1
}

require_provider() {
  command -v "$CG_BIN" >/dev/null 2>&1 ||
    die "provider '$CG_BIN' not found on PATH; set CODEINDEX_CODEGRAPH_BIN or install it. No auto-install is performed."
  [ -d "$CODEINDEX_ROOT/.codegraph" ] ||
    die "no index at '$CODEINDEX_ROOT/.codegraph'; run '$CG_BIN init' in that repository first."
  # The provider resolves its project by CWD, and `status`/`sync` accept no
  # --path flag at all (only query/impact/affected do). Without this cd,
  # CODEINDEX_ROOT was validated and then IGNORED: pointing it at another
  # repository silently read whichever index happened to sit in the caller's
  # CWD, or failed with a confusing provider error that named the wrong repo.
  # Enter the validated root so every verb agrees on one target.
  #
  # Consequence, and it is deliberate: <file> arguments to `affected` are
  # REPO-RELATIVE (relative to CODEINDEX_ROOT), not relative to the caller's
  # CWD. That is the only stable contract when the adapter may be invoked from
  # anywhere in a multi-root workspace.
  cd "$CODEINDEX_ROOT" ||
    die "cannot enter CODEINDEX_ROOT '$CODEINDEX_ROOT'"
}

# Emit a bare JSON array/map on stdout, or fail loudly. Never emit partial JSON.
run_provider() {
  local out
  out="$("$@" 2>/dev/null)" || die "provider invocation failed: $*"
  [ -n "$out" ] || die "provider returned empty output: $*"
  printf '%s\n' "$out"
}

VERB="${1:-}"
shift || true

case "$VERB" in
  symbols)
    [ "$#" -ge 1 ] || die "symbols requires <query>"
    require_provider
    run_provider "$CG_BIN" query "$1" --limit "${CODEINDEX_LIMIT:-200}" --json
    ;;
  impact)
    [ "$#" -ge 1 ] || die "impact requires <symbol>"
    require_provider
    run_provider "$CG_BIN" impact "$1" --depth "${CODEINDEX_DEPTH:-3}" --json
    ;;
  affected)
    [ "$#" -ge 1 ] || die "affected requires at least one <file>"
    require_provider
    # The provider's `affected --json` emits an OBJECT
    # ({changedFiles, affectedTests, totalDependentsTraversed}), while every
    # other verb here — and `selftest affected` — emits a JSON ARRAY. Passing the
    # object through hands consumers two shapes for one contract, and a naive
    # length check counts 3 KEYS instead of N affected tests. That is a
    # silent-undercount trap of exactly the kind this adapter exists to prevent:
    # a test-impact consumer would "helpfully" skip nearly every test.
    # Emit the affectedTests list itself, as an array, so the contract holds.
    # `--quiet` is the provider's own line-per-path mode; empty is a legitimate
    # "no affected tests" result and MUST yield [] rather than a failure.
    affected_out="$("$CG_BIN" affected "$@" --quiet 2>/dev/null)" ||
      die "provider invocation failed: $CG_BIN affected $* --quiet"
    printf '%s' "$affected_out" | awk '
      BEGIN { printf "["; sep = "" }
      NF {
        line = $0
        gsub(/\\/, "\\\\", line)
        gsub(/"/, "\\\"", line)
        printf "%s\"%s\"", sep, line
        sep = ","
      }
      END { print "]" }
    '
    ;;
  routes)
    require_provider
    run_provider "$CG_BIN" query "" --kind route --limit "${CODEINDEX_LIMIT:-5000}" --json
    ;;
  status)
    require_provider
    run_provider "$CG_BIN" status --json
    ;;
  selftest)
    # Canonical shapes, provider-free, for offline shape validation.
    case "${1:-}" in
      symbols|impact|affected|routes) echo '[]'; exit 0 ;;
      status) echo '{}'; exit 0 ;;
      *) die "selftest requires a known verb" ;;
    esac
    ;;
  -h|--help|"")
    cat >&2 <<'EOF'
codegraph.sh — CodeGraph code-index adapter
Usage: codegraph.sh <verb> [args...]
Verbs: symbols <query> | impact <symbol> | affected <file>... | routes | status
       selftest <verb>   (canonical shape, no provider needed)
Env:   CODEINDEX_ROOT (default: $PWD), CODEINDEX_CODEGRAPH_BIN (default: codegraph),
       CODEINDEX_LIMIT, CODEINDEX_DEPTH
Exit:  0 ok | 1 provider missing / no index / provider failure (= "unavailable")
EOF
    exit 0
    ;;
  *)
    die "unknown verb '$VERB'"
    ;;
esac
