#!/usr/bin/env bash
# bubbles/adapters/codeindex/codebase-memory.sh — Codebase Memory MCP adapter.
#
# Wraps the `codebase-memory-mcp` CLI (MIT, single static C binary, tree-sitter
# grammars incl. BASH, local-only) and NORMALIZES its output to the canonical
# codeindex shapes. The operator MUST make the binary resolvable — either on
# PATH or via CODEINDEX_CODEBASE_MEMORY_BIN. NO default install path, NO
# auto-install, NO network fetch — fail-fast instead.
#
# WHY A SECOND PROVIDER: codegraph.sh does NOT parse shell. Bubbles is
# shell-dominant, so under CodeGraph the framework's own repo is nearly
# invisible to the seam. This provider indexes shell natively (measured: 418
# .sh files, 907 file nodes, 16,367 nodes / 27,461 edges in ~2.2s), which is
# what makes the seam useful on THIS repo. It is an ADDITION, not a
# replacement — see the capability table below for what it cannot do.
#
# Verbs (contract: 5 core + 3 extensions). Support is NOT uniform, and that is
# reported honestly via the `capabilities` verb rather than faked:
#   symbols  <query>   native      → search_graph            → JSON ARRAY
#   impact   <symbol>  native      → trace_path (both)       → JSON ARRAY
#   routes             native      → search_graph --label Route → JSON ARRAY
#   indexed            derived     → query_graph aggregate    → JSON ARRAY
#   status             native      → index_status             → JSON MAP
#   sync               native      → index_repository (incremental) → JSON MAP
#   affected <file>... UNSUPPORTED → exit 1
#   freshness          UNSUPPORTED → exit 2 (cannot determine ⇒ assume stale)
#   capabilities                   → JSON MAP of verb → support level
#
# WHY `affected` IS UNSUPPORTED RATHER THAN APPROXIMATED:
# The provider does carry TESTS edges (18,087 on a large repo), so a plausible
# `affected` could be written as
#   MATCH (t)-[:TESTS]->(s) WHERE s.file_path = <file> RETURN t.file_path
# and it would return a well-formed, confident, WRONG answer. Measured on
# QuantitativeFinance for one changed file: this derivation yields 6 rows —
# including non-test source files, and unchanged when filtered on
# `t.is_test = true` — where CodeGraph returns 1,193. Shipping that would be a
# silent 200x undercount of the test blast radius: exactly the "answers
# confidently wrong" failure the exit-code contract exists to prevent. A
# consumer that gets exit 1 degrades to running the full suite, which is
# correct. A consumer that gets 6 test files skips 1,187 real ones.
#
# WHY `freshness` EXITS 2 UNCONDITIONALLY:
# The contract states an adapter that CANNOT DETERMINE freshness MUST exit 2,
# not 0. This provider exposes no trustworthy staleness signal: after an
# in-fixture edit, `index_status` still reported "ready", and `detect_changes`
# returned byte-identical output BEFORE and AFTER a full resync (with the same
# file duplicated in its own change list). There is therefore no reading that
# distinguishes fresh from stale, so this adapter never claims fresh. Callers
# wired as `freshness || sync` still self-heal; callers that gate on freshness
# correctly treat this provider's facts as untrusted.
#
# Output: normalized JSON to stdout. Adapter failure exits 1; the framework
# treats that as "code index unavailable", NOT as a framework failure. A
# consumer MUST degrade to its existing behavior on exit 1, never block on it.
#
# Shape selftest (NO index, NO provider required):
#   codebase-memory.sh selftest <verb>
#
# Provider notes (verified 2026-07-29 against codebase-memory-mcp v0.9.0):
#   - Project identity is derived from the indexed path, NOT supplied. This
#     adapter resolves it by matching `list_projects[].root_path` against the
#     canonical CODEINDEX_ROOT, so pointing CODEINDEX_ROOT at another repo can
#     never silently read the wrong project's graph.
#   - `Route` nodes exist and are plentiful, but `file_path` is empty on all of
#     them and only a minority carry HANDLES edges. Route records are therefore
#     useful as an inventory, NOT as a file-attributed map. Test-fixture routes
#     are not distinguished from production routes.
#   - `check_index_coverage` is present on the provider's main branch but is
#     NOT in the v0.9.0 released CLI ("unknown tool"), so `indexed` is derived
#     from the graph instead.
#   - Peak RSS scales with repo size and does NOT honor CBM_MEM_BUDGET_MB
#     (measured 4.65 GiB on a 10,750-file repo). On a memory-constrained host,
#     index deliberately rather than from a git hook.

set -euo pipefail

# Exported, not merely assigned: project resolution below reads this via
# os.environ, so a plain shell variable KeyErrors whenever the caller did not
# already export it — which is the normal wired path.
export CODEINDEX_ROOT="${CODEINDEX_ROOT:-$PWD}"
CBM_BIN="${CODEINDEX_CODEBASE_MEMORY_BIN:-codebase-memory-mcp}"

# Privacy defaults, applied to every provider invocation. Set defensively —
# provider stdout was verified clean during validation, and the framework must
# never emit usage data the operator did not opt into.
export DO_NOT_TRACK=1
export CBM_TELEMETRY=0
export CBM_LOG_LEVEL="${CBM_LOG_LEVEL:-error}"

die() {
  echo "[codebase-memory][ERROR] $1" >&2
  exit 1
}

# Support levels are declared ONCE and reused by both `capabilities` and the
# guard in require_provider, so a verb can never be advertised as unsupported
# yet still attempt a provider call (or vice versa).
CAPABILITIES_JSON='{"symbols":"native","impact":"native","affected":"unsupported","routes":"native","indexed":"derived","status":"native","freshness":"unsupported","sync":"native"}'

PROJECT=""

require_provider() {
  command -v "$CBM_BIN" >/dev/null 2>&1 ||
    die "provider '$CBM_BIN' not found on PATH; set CODEINDEX_CODEBASE_MEMORY_BIN or install it. No auto-install is performed."
  command -v python3 >/dev/null 2>&1 ||
    die "python3 is required to normalize provider JSON safely; hand-rolled parsing of nested JSON is how shape bugs get shipped."

  [ -d "$CODEINDEX_ROOT" ] ||
    die "CODEINDEX_ROOT '$CODEINDEX_ROOT' is not a directory"
  CODEINDEX_ROOT="$(cd "$CODEINDEX_ROOT" && pwd -P)"

  # The provider refuses paths outside its allowed root. Default it to the repo
  # under inspection so the adapter is scoped by construction; respect an
  # operator-supplied value if one is already set.
  export CBM_ALLOWED_ROOT="${CBM_ALLOWED_ROOT:-$CODEINDEX_ROOT}"

  # Resolve the project by ROOT PATH, never by name. Names are auto-derived
  # from the directory (e.g. "tmp-xyz-fixture"), so name-guessing would be both
  # fragile and capable of reading a different repository's graph.
  local projects_json
  projects_json="$("$CBM_BIN" cli list_projects 2>/dev/null)" ||
    die "provider invocation failed: $CBM_BIN cli list_projects"

  PROJECT="$(printf '%s' "$projects_json" | python3 -c '
import json, os, sys
try:
    doc = json.load(sys.stdin)
except Exception:
    sys.exit(3)
want = os.path.realpath(os.environ["CODEINDEX_ROOT"])
for p in doc.get("projects", []) or []:
    root = p.get("root_path")
    if root and os.path.realpath(root) == want:
        print(p.get("name", ""))
        sys.exit(0)
sys.exit(4)
')" || die "no index for '$CODEINDEX_ROOT'; run '$CBM_BIN cli index_repository --repo-path $CODEINDEX_ROOT' first."

  [ -n "$PROJECT" ] || die "resolved an empty project name for '$CODEINDEX_ROOT'"
}

# Run the provider and emit a bare JSON array/map, or fail loudly. Never emit
# partial JSON. $1 is a python expression over `d` (the parsed provider doc)
# yielding the normalized value.
run_normalized() {
  local expr="$1"
  shift
  local out
  out="$("$@" 2>/dev/null)" || die "provider invocation failed: $*"
  [ -n "$out" ] || die "provider returned empty output: $*"
  printf '%s' "$out" | NORM_EXPR="$expr" python3 -c '
import json, os, sys
try:
    d = json.load(sys.stdin)
except Exception as exc:
    sys.stderr.write("[codebase-memory][ERROR] provider emitted non-JSON: %s\n" % exc)
    sys.exit(1)
if isinstance(d, dict) and "error" in d:
    sys.stderr.write("[codebase-memory][ERROR] provider error: %s\n" % d["error"])
    sys.exit(1)
try:
    # Builtins are stripped and re-granted explicitly: the normalizer
    # expressions are adapter-authored constants (never provider input), and
    # this keeps the evaluation surface to exactly the constructors they need.
    safe = {"dict": dict, "int": int, "str": str, "list": list}
    value = eval(os.environ["NORM_EXPR"], {"__builtins__": safe}, {"d": d})
except Exception as exc:
    sys.stderr.write("[codebase-memory][ERROR] provider payload missing expected field: %s\n" % exc)
    sys.exit(1)
json.dump(value, sys.stdout)
sys.stdout.write("\n")
' || exit 1
}

VERB="${1:-}"
shift || true

case "$VERB" in
  symbols)
    [ "$#" -ge 1 ] || die "symbols requires <query>"
    require_provider
    run_normalized 'd["results"]' \
      "$CBM_BIN" cli search_graph --project "$PROJECT" \
      --name-pattern "$1" --limit "${CODEINDEX_LIMIT:-200}"
    ;;
  impact)
    [ "$#" -ge 1 ] || die "impact requires <symbol>"
    require_provider
    # trace_path returns an OBJECT with SEPARATE callees/callers arrays, while
    # the contract declares a single ARRAY. Concatenating without tagging would
    # lose the direction, so each record keeps a "direction" field — the blast
    # radius of a change is both who it calls and who calls it.
    #
    # It also exits 1 with {"error":"function not found"} for a symbol that is
    # simply absent. That is a SUCCESSFUL lookup with no results, not "I could
    # not look", and the contract reserves exit 1 for the latter. Passing the
    # provider's exit code straight through would tell a consumer the index was
    # unavailable every time it asked about a symbol that does not exist —
    # collapsing exactly the distinction this contract exists to preserve. So
    # not-found is normalized to [] exit 0; every OTHER error still exits 1.
    # NOTE: the provider writes this error payload to STDERR, not stdout, so
    # both streams are captured and the JSON object is located by scanning —
    # which also makes this immune to provider log lines appearing on either
    # stream.
    impact_out="$("$CBM_BIN" cli trace_path --project "$PROJECT" \
      --function-name "$1" --direction both --depth "${CODEINDEX_DEPTH:-3}" 2>&1)" || true
    [ -n "$impact_out" ] ||
      die "provider returned empty output: $CBM_BIN cli trace_path --function-name $1"
    printf '%s' "$impact_out" | python3 -c '
import json, sys
doc = None
for line in sys.stdin.read().splitlines():
    line = line.strip()
    if not line.startswith("{"):
        continue
    try:
        doc = json.loads(line)
        break
    except Exception:
        continue
if doc is None:
    sys.stderr.write("[codebase-memory][ERROR] provider emitted no JSON object\n")
    sys.exit(1)
err = doc.get("error") if isinstance(doc, dict) else None
if err:
    if "not found" in err.lower():
        # Known symbol space, no such symbol: a real empty answer.
        json.dump([], sys.stdout)
        sys.stdout.write("\n")
        sys.exit(0)
    sys.stderr.write("[codebase-memory][ERROR] provider error: %s\n" % err)
    sys.exit(1)
# An ambiguous name comes back in a DIFFERENT shape — status/message/suggestions,
# with no callees/callers. Without this branch the KeyError below reported
# "payload missing expected field: callees", which blamed the payload shape for a
# caller-side problem and discarded the qualified names needed to disambiguate.
if doc.get("status") == "ambiguous":
    cands = [s if isinstance(s, str) else s.get("qualified_name", str(s))
             for s in doc.get("suggestions", [])]
    sys.stderr.write(
        "[codebase-memory][ERROR] symbol name is ambiguous (%d matches); "
        "re-run impact with one of these qualified names:\n" % len(cands))
    for c in cands:
        sys.stderr.write("  %s\n" % c)
    sys.exit(1)
try:
    out = ([dict(r, direction="callee") for r in doc["callees"]] +
           [dict(r, direction="caller") for r in doc["callers"]])
except Exception as exc:
    sys.stderr.write("[codebase-memory][ERROR] provider payload missing expected field: %s\n" % exc)
    sys.exit(1)
json.dump(out, sys.stdout)
sys.stdout.write("\n")
' || exit 1
    ;;
  routes)
    require_provider
    run_normalized 'd["results"]' \
      "$CBM_BIN" cli search_graph --project "$PROJECT" \
      --label Route --limit "${CODEINDEX_LIMIT:-1000}"
    ;;
  indexed)
    # Answers "does the index know this file, and does it carry symbols?" —
    # which is what lets a consumer tell "no dependents" apart from "this file
    # is not indexable". `check_index_coverage` would be the direct route but
    # is absent from the v0.9.0 CLI, so this is derived from the graph.
    #
    # Aggregate counts come back as STRINGS ("319"), so they are coerced to int
    # here; a consumer comparing nodeCount > 0 against a string would be
    # comparing types, not counts.
    require_provider
    run_normalized '[{"path": r[0], "nodeCount": int(r[1])} for r in d["rows"]]' \
      "$CBM_BIN" cli query_graph --project "$PROJECT" \
      --query 'MATCH (f:File) OPTIONAL MATCH (f)-[:DEFINES]->(d) RETURN f.file_path, count(d)'
    ;;
  status)
    require_provider
    run_normalized 'd' "$CBM_BIN" cli index_status --project "$PROJECT"
    ;;
  affected)
    [ "$#" -ge 1 ] || die "affected requires <file>..."
    # Deliberately unsupported. See the header: the available derivation is
    # measurably wrong by ~200x and would be indistinguishable from a correct
    # small answer. Exit 1 means "I could not look", which makes the consumer
    # fall back to the full suite — the safe direction to be wrong in.
    die "verb 'affected' is UNSUPPORTED by this provider: its TESTS-edge derivation undercounts the test blast radius (measured 6 vs 1193) and would be silently wrong. Use the codegraph adapter for test-impact selection."
    ;;
  freshness)
    # Exit 2 = STALE. The contract requires exit 2 (not 0) when freshness
    # cannot be determined, because reporting fresh on an answer we could not
    # actually read is the confidently-wrong failure mode. See header for the
    # measurements behind "cannot determine".
    printf '{"stale":true,"determinable":false,"reason":"provider exposes no trustworthy freshness signal: index_status reports ready regardless of worktree edits, and detect_changes returns identical output before and after a full resync"}\n'
    exit 2
    ;;
  sync)
    # Incremental re-index; the ONLY mutating verb. `--persistence false` keeps
    # the provider from writing artifacts into the indexed repository — the
    # framework must never dirty a consumer's working tree as a side effect of
    # asking it a question.
    #
    # Deliberately does NOT chain into `freshness` the way codegraph.sh does:
    # freshness here always exits 2, so chaining would report a SUCCESSFUL sync
    # as a failure. Exit 0 means "the re-index ran"; freshness remaining
    # undeterminable is reported separately, by freshness.
    require_provider
    run_normalized 'd' "$CBM_BIN" cli index_repository \
      --repo-path "$CODEINDEX_ROOT" --persistence false
    exit 0
    ;;
  capabilities)
    # Lets a caller tell "unsupported by design" apart from "broken" WITHOUT
    # invoking a verb — the same distinction [] vs exit 1 draws for results,
    # one level up. A consumer can degrade with an accurate reason, and the
    # contract selftest can skip live assertions for verbs this provider never
    # claimed, instead of failing them.
    printf '%s\n' "$CAPABILITIES_JSON"
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
codebase-memory.sh — Codebase Memory MCP code-index adapter
Usage: codebase-memory.sh <verb> [args...]
Verbs: symbols <query> | impact <symbol> | routes | indexed | status
       affected <file>...  UNSUPPORTED (always exit 1; use the codegraph adapter)
       freshness           UNSUPPORTED (always exit 2 = cannot determine ⇒ stale)
       sync                (incremental re-index; the only mutating verb)
       capabilities        (JSON map: verb → native | derived | unsupported)
       selftest <verb>     (canonical shape, no provider needed)
Env:   CODEINDEX_ROOT (default: $PWD), CODEINDEX_CODEBASE_MEMORY_BIN
       (default: codebase-memory-mcp), CODEINDEX_LIMIT, CODEINDEX_DEPTH
       CBM_ALLOWED_ROOT (defaults to CODEINDEX_ROOT)
Needs: python3 (JSON normalization)
Exit:  0 ok | 1 provider missing / no index / unsupported verb / provider failure
       2 freshness only: index is STALE or undeterminable
Note:  the project is resolved by matching CODEINDEX_ROOT against the provider's
       indexed root_path, so it can never read another repository's graph.
EOF
    exit 0
    ;;
  *)
    die "unknown verb '$VERB'"
    ;;
esac
