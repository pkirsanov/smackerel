#!/usr/bin/env bash
# bubbles/adapters/codeindex/codegraph.sh — CodeGraph code-index adapter.
#
# Wraps the `codegraph` CLI (MIT, 100% local, tree-sitter -> SQLite/FTS5; no
# API key, no embeddings, no network) and NORMALIZES its output to the
# canonical codeindex shapes. The operator MUST make `codegraph` resolvable —
# either on PATH or via CODEINDEX_CODEGRAPH_BIN. NO default install path, NO
# auto-install, NO network fetch — fail-fast instead.
#
# Verbs (the 5 CORE verbs are mandatory per the codeindex adapter contract):
#   symbols  <query>   → `codegraph query`    → JSON ARRAY of symbol records
#   impact   <symbol>  → `codegraph impact`   → JSON ARRAY of affected records
#   affected <file>... → `codegraph affected` → JSON ARRAY of test file paths
#   routes             → `codegraph query --kind route` → JSON ARRAY of routes
#   status             → `codegraph status`   → JSON MAP of index health
#
# Plus four contract EXTENSIONS:
#   indexed            → JSON ARRAY of {path,nodeCount}; which files the index
#                        knows about, and which of those carry graph nodes
#   freshness          → JSON MAP; exit 0 fresh, 2 STALE, 1 cannot determine
#   sync               → JSON MAP; re-syncs the index, then reports freshness
#   capabilities       → JSON MAP of verb → native | derived | unsupported
#
# `freshness` exists because a stale index is the quiet failure mode: it returns
# the right shape and plausible data derived from code that no longer exists.
# Without it a consumer cannot distinguish "no dependents" from "index is a week
# behind" — the same trap as [] vs exit 1, one level up.
#
# `sync` is the ONLY mutating verb, and exists so index maintenance is a
# contract capability rather than provider-specific operator knowledge:
#
#   <adapter> freshness || <adapter> sync
#
# is wireable into any repo CLI or git hook, stays correct if the provider is
# swapped, and is a safe no-op under `adapter: none`.
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
    # Same trap as `affected`, different verb: the provider's `impact --json`
    # emits an OBJECT ({symbol, depth, nodeCount, edgeCount, affected}) while
    # the contract — and `selftest impact` — declares an ARRAY. A length check
    # on the object counts 5 KEYS instead of N affected symbols, so a consumer
    # would read a 31-symbol blast radius as "5" and under-react.
    #
    # Found by validating the LIVE adapter after `affected` was already fixed:
    # the first contract selftest exercised routes/status/symbols/affected but
    # not impact, so the sibling instance survived. Part B now covers every
    # record verb for exactly this reason.
    #
    # `node` does the extraction rather than python/jq because the provider is
    # itself a node package: if the provider can run at all, node is present.
    impact_out="$("$CG_BIN" impact "$1" --depth "${CODEINDEX_DEPTH:-3}" --json 2>/dev/null)" ||
      die "provider invocation failed: $CG_BIN impact $1"
    [ -n "$impact_out" ] || die "provider returned empty output: impact $1"
    printf '%s' "$impact_out" | node -e '
      let s = "";
      process.stdin.on("data", d => (s += d)).on("end", () => {
        let o;
        try { o = JSON.parse(s); } catch (e) { process.exit(1); }
        const a = Array.isArray(o) ? o
                : (o && Array.isArray(o.affected) ? o.affected : null);
        if (a === null) process.exit(1);
        process.stdout.write(JSON.stringify(a) + "\n");
      });
    ' || die "could not normalize impact output to a JSON array"
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
  indexed)
    require_provider
    # Which files does the index actually KNOW about, and which of those carry
    # graph nodes? A changed file with nodeCount == 0 — shell, YAML, Markdown, or
    # any language this provider cannot parse — participates in no edges, so NO
    # dependent can ever be derived from it.
    #
    # Without this, a consumer cannot tell "nothing indexable changed" apart from
    # "the graph missed an edge", and those demand OPPOSITE reactions: the first
    # is correct and boring, the second is a correctness risk. That exact
    # ambiguity produced a false alarm during validation — a shadow report of
    # "0 affected, would skip 100%" looked like a graph gap when the only changed
    # file was a shell script the provider cannot parse.
    run_provider "$CG_BIN" files --json
    ;;
  status)
    require_provider
    run_provider "$CG_BIN" status --json
    ;;
  freshness)
    # A stale index answers CONFIDENTLY WRONG: correct shape, correct-looking
    # data, derived from code that no longer exists. That is the same class of
    # failure as returning [] when the index is missing, so it gets the same
    # treatment — a distinct, checkable signal rather than silence.
    #
    # Exit codes are the contract here (shell-native, no JSON parsing needed by
    # the caller):
    #   0 = fresh   — index matches the worktree
    #   2 = STALE   — index is behind; findings may be wrong
    #   1 = unavailable / cannot determine (same as every other verb)
    #
    # Determinability is NOT optional: if the provider stops reporting
    # pendingChanges, this exits 1 rather than assuming fresh. Never report
    # fresh on an answer we could not actually read.
    require_provider
    status_json="$(run_provider "$CG_BIN" status --json)"
    compact="$(printf '%s' "$status_json" | tr -d ' \n')"

    pend="$(printf '%s' "$compact" | sed -n 's/.*"pendingChanges":{\([^}]*\)}.*/\1/p')"
    [ -n "$pend" ] ||
      die "cannot determine freshness: provider status exposes no pendingChanges field"

    added="$(printf '%s' "$pend" | sed -n 's/.*"added":\([0-9][0-9]*\).*/\1/p')"
    modified="$(printf '%s' "$pend" | sed -n 's/.*"modified":\([0-9][0-9]*\).*/\1/p')"
    removed="$(printf '%s' "$pend" | sed -n 's/.*"removed":\([0-9][0-9]*\).*/\1/p')"
    added="${added:-0}"
    modified="${modified:-0}"
    removed="${removed:-0}"
    total=$((added + modified + removed))

    mismatch="$(printf '%s' "$compact" | sed -n 's/.*"worktreeMismatch":\([^,}]*\).*/\1/p')"

    stale="false"
    reason="null"
    if [ "$total" -gt 0 ]; then
      stale="true"
      reason="\"$total pending change(s): added=$added modified=$modified removed=$removed\""
    elif [ -n "$mismatch" ] && [ "$mismatch" != "null" ]; then
      stale="true"
      reason="\"worktree mismatch reported by provider\""
    fi

    printf '{"stale":%s,"pendingChanges":{"added":%s,"modified":%s,"removed":%s},"reason":%s}\n' \
      "$stale" "$added" "$modified" "$removed" "$reason"

    if [ "$stale" = "true" ]; then
      exit 2
    fi
    exit 0
    ;;
  sync)
    # Bring the index back in line with the worktree. This is the ONLY mutating
    # verb; every other one is read-only.
    #
    # It exists so a repository can self-heal without hard-coding provider
    # knowledge: `freshness || sync` is wireable into any repo CLI or git hook
    # and stays correct if the provider is swapped. With `adapter: none` it is a
    # no-op, so the same wiring is safe in a repo that never opted in.
    #
    # Incremental, not a rebuild: measured ~1.5s for a no-op on a 4,896-file
    # index. Use the provider's `init`/`index` for a full rebuild.
    require_provider
    "$CG_BIN" sync --quiet >/dev/null 2>&1 ||
      die "provider sync failed: $CG_BIN sync --quiet"
    # Report the post-sync state so a caller can confirm the heal actually took.
    exec "$0" freshness
    ;;
  capabilities)
    # This provider implements every contract verb against a real provider
    # call, so all eight are declared `native`. Declaring them explicitly (as
    # opposed to relying on the absent-means-native default) keeps the
    # provider-neutral contract selftest exercising the FULL live surface here
    # rather than silently skipping verbs.
    printf '{"symbols":"native","impact":"native","affected":"native","routes":"native","indexed":"native","status":"native","freshness":"native","sync":"native"}\n'
    exit 0
    ;;
  selftest)
    # Canonical shapes, provider-free, for offline shape validation.
    case "${1:-}" in
      symbols|impact|affected|routes|indexed) echo '[]'; exit 0 ;;
      status|freshness|sync|capabilities) echo '{}'; exit 0 ;;
      *) die "selftest requires a known verb" ;;
    esac
    ;;
  -h|--help|"")
    cat >&2 <<'EOF'
codegraph.sh — CodeGraph code-index adapter
Usage: codegraph.sh <verb> [args...]
Verbs: symbols <query> | impact <symbol> | affected <file>... | routes | status
       indexed           (which files the index knows, and their node counts)
       freshness         (is the index behind the worktree?)
       sync              (bring the index back in line; the only mutating verb)
       capabilities      (JSON map: verb -> native | derived | unsupported)
       selftest <verb>   (canonical shape, no provider needed)
Env:   CODEINDEX_ROOT (default: $PWD), CODEINDEX_CODEGRAPH_BIN (default: codegraph),
       CODEINDEX_LIMIT, CODEINDEX_DEPTH
Exit:  0 ok | 1 provider missing / no index / provider failure (= "unavailable")
       2 freshness/sync only: index is STALE (never emitted by other verbs)
Note:  <file> arguments are REPO-RELATIVE (relative to CODEINDEX_ROOT).
EOF
    exit 0
    ;;
  *)
    die "unknown verb '$VERB'"
    ;;
esac
