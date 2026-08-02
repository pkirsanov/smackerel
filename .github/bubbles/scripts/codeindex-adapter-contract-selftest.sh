#!/usr/bin/env bash
# bubbles/scripts/codeindex-adapter-contract-selftest.sh
#
# Contract selftest for the codeindex adapters.
#
# WHY THIS EXISTS
# ---------------
# The adapters ship a `selftest <verb>` verb that emits a CANNED canonical shape
# ([] for array verbs, {} for status). Nothing compared those canned shapes to
# what the adapter actually returns against a real index — so the canned shape
# was a self-fulfilling assertion.
#
# That gap shipped a real bug: `codegraph affected --json` returns an OBJECT
# ({changedFiles, affectedTests, totalDependentsTraversed}), and the adapter
# passed it straight through. `selftest affected` said "[]", live `affected` said
# "{...}". A consumer taking a length got 3 (dict KEYS) instead of 497 test
# files. A test-impact consumer would have silently skipped ~99% of the suite
# and reported green. That is precisely the silent-undercount failure the
# codeindex contract exists to prevent.
#
# STRUCTURE
# ---------
#   Part A (HERMETIC, always runs): drives the shape checker with synthetic
#     adapters — including one that reproduces the pre-fix object-returning
#     `affected`. If the checker ever stops rejecting that, Part A fails. This
#     is the adversarial case; without it the suite would pass whether or not
#     the checker works.
#   Part B (LIVE, opportunistic): if a provider AND a real index are reachable,
#     assert the adapter's real output matches its own declared shapes.
#     SKIPs cleanly when no index is available (framework CI has none).
#
# Point Part B at a repository with an index via:
#   CODEINDEX_SELFTEST_ROOT=/path/to/indexed/repo
#
# Exit 0 = pass (or legitimately skipped). Exit 1 = contract violation.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
ADAPTER_DIR="$REPO_ROOT/bubbles/adapters/codeindex"

PASS=0
FAIL=0

ok()   { echo "  ✅ $1"; PASS=$((PASS + 1)); }
bad()  { echo "  ❌ $1"; FAIL=$((FAIL + 1)); }
note() { echo "  ℹ️  $1"; }

if ! command -v python3 >/dev/null 2>&1; then
  echo "codeindex-adapter-contract-selftest: SKIP (python3 not installed)"
  exit 0
fi

# Assert stdin is valid JSON of the expected top-level type.
# Usage: printf '%s' "$json" | assert_shape array|object
assert_shape() {
  python3 -c '
import sys, json
want = sys.argv[1]
raw = sys.stdin.read()
try:
    d = json.loads(raw)
except Exception:
    sys.exit(2)          # not JSON at all
got = "array" if isinstance(d, list) else "object" if isinstance(d, dict) else "scalar"
sys.exit(0 if got == want else 1)
' "$1"
}

echo "codeindex-adapter-contract-selftest"
echo ""

# ---------------------------------------------------------------------------
# Part A — HERMETIC. Drives the checker with synthetic payloads.
# ---------------------------------------------------------------------------
echo "Part A — shape checker (hermetic, adversarial)"

printf '%s' '[]'                      | assert_shape array  && ok "empty array accepted as array"        || bad "empty array rejected"
printf '%s' '["a","b"]'               | assert_shape array  && ok "populated array accepted as array"    || bad "populated array rejected"
printf '%s' '{}'                      | assert_shape object && ok "empty object accepted as object"      || bad "empty object rejected"
printf '%s' '{"initialized":true}'    | assert_shape object && ok "populated object accepted as object"  || bad "populated object rejected"

# THE regression case: the pre-fix `affected` payload. If the checker ever
# accepts this as an array, the bug can silently return.
PREFIX_AFFECTED='{"changedFiles":["a.rs"],"affectedTests":["t1.rs","t2.rs"],"totalDependentsTraversed":9}'
if printf '%s' "$PREFIX_AFFECTED" | assert_shape array; then
  bad "ADVERSARIAL: pre-fix affected OBJECT was accepted as an array"
else
  ok "ADVERSARIAL: pre-fix affected OBJECT rejected as an array"
fi

# A length check on that object yields 3 (keys), not 2 (tests) — the exact
# undercount. Pin the arithmetic so the rationale cannot rot.
KEYS=$(printf '%s' "$PREFIX_AFFECTED" | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))')
TESTS=$(printf '%s' "$PREFIX_AFFECTED" | python3 -c 'import sys,json; print(len(json.load(sys.stdin)["affectedTests"]))')
if [ "$KEYS" = "3" ] && [ "$TESTS" = "2" ]; then
  ok "ADVERSARIAL: object length is $KEYS keys vs $TESTS real tests (undercount pinned)"
else
  bad "undercount arithmetic changed: keys=$KEYS tests=$TESTS"
fi

printf '%s' 'not json'                | assert_shape array; [ $? -eq 2 ] && ok "non-JSON rejected"        || bad "non-JSON not rejected"

echo ""

# ---------------------------------------------------------------------------
# Part A2 — every adapter's DECLARED selftest shapes must match the contract.
# ---------------------------------------------------------------------------
echo "Part A2 — declared selftest shapes"

for adapter in "$ADAPTER_DIR"/*.sh; do
  [ -f "$adapter" ] || continue
  name="$(basename "$adapter" .sh)"
  for verb in symbols impact affected routes indexed; do
    out="$(bash "$adapter" selftest "$verb" 2>/dev/null)"
    printf '%s' "$out" | assert_shape array &&
      ok "$name selftest $verb declares an array" ||
      bad "$name selftest $verb is not an array (got: ${out:0:40})"
  done
  out="$(bash "$adapter" selftest status 2>/dev/null)"
  printf '%s' "$out" | assert_shape object &&
    ok "$name selftest status declares an object" ||
    bad "$name selftest status is not an object (got: ${out:0:40})"
  out="$(bash "$adapter" selftest freshness 2>/dev/null)"
  printf '%s' "$out" | assert_shape object &&
    ok "$name selftest freshness declares an object" ||
    bad "$name selftest freshness is not an object (got: ${out:0:40})"
  out="$(bash "$adapter" selftest sync 2>/dev/null)"
  printf '%s' "$out" | assert_shape object &&
    ok "$name selftest sync declares an object" ||
    bad "$name selftest sync is not an object (got: ${out:0:40})"
done

echo ""

# ---------------------------------------------------------------------------
# Part A3 — the `none` adapter must be neutral AND succeed.
# ---------------------------------------------------------------------------
echo "Part A3 — none adapter neutrality"

NONE="$ADAPTER_DIR/none.sh"
if [ -f "$NONE" ]; then
  for verb in symbols impact affected routes; do
    out="$(bash "$NONE" "$verb" 2>/dev/null)"; rc=$?
    if [ "$rc" -eq 0 ] && [ "$out" = "[]" ]; then
      ok "none $verb -> [] exit 0"
    else
      bad "none $verb -> '$out' exit $rc (want [] exit 0)"
    fi
  done
  for verb in status freshness; do
    out="$(bash "$NONE" "$verb" 2>/dev/null)"; rc=$?
    if [ "$rc" -eq 0 ] && [ "$out" = "{}" ]; then
      ok "none $verb -> {} exit 0"
    else
      bad "none $verb -> '$out' exit $rc (want {} exit 0)"
    fi
  done
  # `sync` under `none` MUST be a succeeding no-op, otherwise a repo cannot wire
  # `freshness || sync` unconditionally without breaking non-adopting repos.
  out="$(bash "$NONE" sync 2>/dev/null)"; rc=$?
  if [ "$rc" -eq 0 ] && [ "$out" = "{}" ]; then
    ok "none sync -> {} exit 0 (safe no-op)"
  else
    bad "none sync -> '$out' exit $rc (want {} exit 0)"
  fi
else
  bad "none.sh adapter is missing"
fi

echo ""

# ---------------------------------------------------------------------------
# Part A4 — no adapter may re-enter itself via `exec "$0"`.
# ---------------------------------------------------------------------------
# Every verb is invoked as `bash <adapter>`, so an adapter never needs its own
# executable bit — EXCEPT at a bare `exec "$0"`, which runs the file as a
# program. That made `sync` the single verb that could die with an opaque exit
# 126 wherever an install path drops modes (archive extraction, `cp` without
# -p, a Docker COPY), while every other verb kept working. Measured: the bare
# form exits 126 with the bit stripped, `exec bash "$0"` exits 0.
echo "Part A4 — no self-exec that depends on the adapter's own mode bit"

selfexec_hits=0
selfexec_scanned=0
for adapter in "$ADAPTER_DIR"/*.sh; do
  [ -f "$adapter" ] || continue
  selfexec_scanned=$((selfexec_scanned + 1))
  if grep -qE '^[[:space:]]*exec[[:space:]]+"\$0"' "$adapter"; then
    bad "$(basename "$adapter") re-enters itself with bare exec \"\$0\" (use: exec bash \"\$0\")"
    selfexec_hits=$((selfexec_hits + 1))
  fi
done
[ "$selfexec_hits" -eq 0 ] && ok "no adapter uses bare exec \"\$0\" ($selfexec_scanned scanned)"

echo ""

# ---------------------------------------------------------------------------
# Part A5 — an AMBIGUOUS symbol must be reported as ambiguous, not as a
# malformed payload.
# ---------------------------------------------------------------------------
# codebase-memory answers an ambiguous name with a different shape
# (status/message/suggestions, no callees/callers). The adapter used to fall
# straight through to doc["callees"] and report "provider payload missing
# expected field: 'callees'" — blaming the payload for a caller-side problem and
# throwing away the qualified names needed to disambiguate. Driven through a
# stub provider so this stays hermetic.
echo "Part A5 — ambiguous symbol is reported as ambiguous"

CBM_ADAPTER="$ADAPTER_DIR/codebase-memory.sh"
if [ -f "$CBM_ADAPTER" ] && command -v python3 >/dev/null 2>&1; then
  a5_dir="$(mktemp -d)"
  a5_repo="$a5_dir/repo"; mkdir -p "$a5_repo"
  a5_stub="$a5_dir/stub-provider"
  cat >"$a5_stub" <<STUB
#!/usr/bin/env bash
# Minimal codebase-memory stand-in: resolves one project, then answers
# trace_path with the provider's real ambiguous-result shape.
for a in "\$@"; do
  case "\$a" in
    list_projects)
      printf '{"projects":[{"name":"stub-proj","root_path":"%s"}]}' "$a5_repo"; exit 0 ;;
    trace_path)
      printf '{"status":"ambiguous","message":"2 matches for \\\\"dup\\\\".","suggestions":["stub-proj.a.dup","stub-proj.b.dup"]}'; exit 0 ;;
  esac
done
exit 0
STUB
  chmod +x "$a5_stub"

  a5_err="$a5_dir/err"
  CODEINDEX_ROOT="$a5_repo" CODEINDEX_CODEBASE_MEMORY_BIN="$a5_stub" \
    bash "$CBM_ADAPTER" impact dup >/dev/null 2>"$a5_err"
  a5_rc=$?

  [ "$a5_rc" -eq 1 ] \
    && ok "ambiguous symbol exits 1 (could-not-look)" \
    || bad "ambiguous symbol exited $a5_rc (want 1)"
  grep -q 'ambiguous' "$a5_err" \
    && ok "error names the ambiguity" \
    || bad "error does not mention ambiguity: $(head -1 "$a5_err")"
  grep -q 'stub-proj.a.dup' "$a5_err" && grep -q 'stub-proj.b.dup' "$a5_err" \
    && ok "error lists the candidate qualified names" \
    || bad "error drops the disambiguating candidates"
  # ADVERSARIAL: the pre-fix message must not come back.
  grep -q 'missing expected field' "$a5_err" \
    && bad "ADVERSARIAL: reports a malformed payload instead of an ambiguous name" \
    || ok "ADVERSARIAL: does not blame the payload shape"

  rm -rf "$a5_dir"
else
  ok "SKIP Part A5 (codebase-memory adapter or python3 unavailable)"
fi

echo ""

# ---------------------------------------------------------------------------
# Part B — LIVE. Only when a provider and a real index are reachable.
# ---------------------------------------------------------------------------
echo "Part B — live adapter output vs declared shape"

LIVE_ROOT="${CODEINDEX_SELFTEST_ROOT:-}"
# WHICH adapter to exercise. Defaults to codegraph so existing operator wiring
# keeps working, but this section is no longer CodeGraph-specific: it used to
# probe for a `.codegraph` directory and the `codegraph` binary by name, which
# meant a second provider could never be live-tested at all.
LIVE_ADAPTER="${CODEINDEX_SELFTEST_ADAPTER:-codegraph}"
ADAPTER_SH="$ADAPTER_DIR/$LIVE_ADAPTER.sh"

if [ -z "$LIVE_ROOT" ]; then
  note "SKIP (set CODEINDEX_SELFTEST_ROOT to an indexed repository to enable)"
elif [ ! -f "$ADAPTER_SH" ]; then
  note "SKIP (no adapter '$LIVE_ADAPTER' at $ADAPTER_SH)"
elif ! CODEINDEX_ROOT="$LIVE_ROOT" bash "$ADAPTER_SH" status >/dev/null 2>&1; then
  # Availability is probed THROUGH THE CONTRACT rather than by looking for a
  # provider-specific marker directory. `status` exiting 0 is precisely the
  # contract's definition of "the index is reachable", so this works for any
  # provider without the selftest knowing anything about its on-disk layout.
  note "SKIP ($LIVE_ADAPTER status not reachable for $LIVE_ROOT — provider missing or repo not indexed)"
else
  CGA="$ADAPTER_SH"
  export CODEINDEX_ROOT="$LIVE_ROOT"

  # Capability declaration, if the adapter offers one. Absent ⇒ every verb is
  # assumed native, which is exactly how this suite behaved before the verb
  # existed. This is what lets an honestly-partial provider be live-tested for
  # what it DOES support, while still being held to failing loudly on what it
  # does not — rather than the suite either failing it wholesale or skipping it.
  CAPS="$(bash "$CGA" capabilities 2>/dev/null || echo '{}')"
  cap_of() {
    printf '%s' "$CAPS" | CAP_VERB="$1" python3 -c '
import json, os, sys
try:
    d = json.load(sys.stdin)
except Exception:
    d = {}
if not isinstance(d, dict):
    d = {}
print(d.get(os.environ["CAP_VERB"], "native"))
' 2>/dev/null || echo native
  }

  # An UNSUPPORTED verb still has a contract: it must fail loudly, never emit a
  # neutral [] or {}. "I cannot do this" and "I looked and found nothing" must
  # stay distinguishable, which is the whole point of the exit-code contract.
  assert_unsupported() {
    local verb="$1" expect_rc="$2" out rc
    shift 2
    out="$(bash "$CGA" "$verb" "$@" 2>/dev/null)"
    rc=$?
    case "$out" in
      '[]' | '{}')
        bad "unsupported $verb emitted a NEUTRAL value — indistinguishable from a real empty result"
        return
        ;;
    esac
    if [ "$rc" -eq "$expect_rc" ]; then
      ok "unsupported $verb fails loudly with the contract exit ($rc)"
    else
      bad "unsupported $verb exited $rc, want $expect_rc"
    fi
  }

  # EVERY record verb, ENUMERATED — never hand-picked.
  #
  # The first version of this file checked routes/status/symbols/affected by
  # hand and simply omitted `impact`. So when `impact` turned out to carry the
  # identical object-instead-of-array defect that `affected` had, this suite
  # reported 37/37 while the bug was live in four repositories. A verb that is
  # not listed cannot fail loudly; the omission WAS the bug.
  LIVE_RECORD_VERBS="routes symbols impact indexed"
  for verb in $LIVE_RECORD_VERBS; do
    if [ "$(cap_of "$verb")" = "unsupported" ]; then
      case "$verb" in
        routes|indexed) assert_unsupported "$verb" 1 ;;
        *)              assert_unsupported "$verb" 1 Handler ;;
      esac
      continue
    fi
    case "$verb" in
      routes|indexed) out="$(bash "$CGA" "$verb" 2>/dev/null)" ;;
      *)              out="$(bash "$CGA" "$verb" Handler 2>/dev/null)" ;;
    esac
    n="$(printf '%s' "$out" | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
    print(len(d) if isinstance(d, list) else -1)
except Exception:
    print(-1)
' 2>/dev/null || echo -1)"
    if printf '%s' "$out" | assert_shape array; then
      ok "live $verb is an array ($n entries)"
    else
      bad "live $verb is NOT an array — object-instead-of-array regression"
    fi
  done

  out="$(bash "$CGA" status 2>/dev/null)"
  printf '%s' "$out" | assert_shape object &&
    ok "live status is an object" || bad "live status is NOT an object"

  # META-ASSERTION: every verb the adapter DECLARES via `selftest` must appear
  # in the live coverage above. This is the check that would have caught the
  # missing `impact` case without anyone noticing the omission by eye.
  declared=""
  for v in symbols impact affected routes indexed status freshness sync; do
    bash "$CGA" selftest "$v" >/dev/null 2>&1 && declared="$declared $v"
  done
  covered="$LIVE_RECORD_VERBS status affected freshness sync"
  uncovered=""
  for v in $declared; do
    case " $covered " in
      *" $v "*) ;;
      *) uncovered="$uncovered $v" ;;
    esac
  done
  if [ -z "$uncovered" ]; then
    ok "live coverage includes every declared verb ($(echo "$declared" | wc -w) verbs)"
  else
    bad "declared but NOT live-tested:$uncovered — a verb that is not exercised cannot regress loudly"
  fi

  # The verb that regressed. Assert BOTH shape and that it can carry real
  # entries — an array that is always empty would satisfy the shape check while
  # hiding the same undercount. Sample several candidates: any single file may
  # legitimately have no dependent tests.
  probes="$(cd "$LIVE_ROOT" && git ls-files '*.rs' '*.go' '*.ts' '*.py' 2>/dev/null | head -25)"
  if [ "$(cap_of affected)" = "unsupported" ]; then
    # A provider may honestly decline test-impact selection — that is a valid
    # capability profile, not a defect. What it must NOT do is return a
    # plausible-but-wrong list, because a silently undercounted test blast
    # radius is indistinguishable from a correct small one. So the assertion
    # here is that it REFUSES, not that it answers.
    assert_unsupported affected 1 "$(printf '%s' "$probes" | head -1)"
  elif [ -n "$probes" ]; then
    shape_ok=1
    max_n=0
    max_file=""
    sampled=0
    while IFS= read -r probe; do
      [ -n "$probe" ] || continue
      sampled=$((sampled + 1))
      out="$(bash "$CGA" affected "$probe" 2>/dev/null)"
      if printf '%s' "$out" | assert_shape array; then
        n="$(printf '%s' "$out" | python3 -c 'import sys,json; print(len(json.load(sys.stdin)))' 2>/dev/null || echo 0)"
        if [ "$n" -gt "$max_n" ]; then max_n="$n"; max_file="$probe"; fi
      else
        shape_ok=0
        break
      fi
    done <<EOF
$probes
EOF

    if [ "$shape_ok" -eq 1 ]; then
      ok "live affected is an array across $sampled sampled files"
    else
      bad "live affected is NOT an array — the pre-fix object regression is back"
    fi

    # Guard the "always empty" degenerate case explicitly.
    if [ "$max_n" -gt 0 ]; then
      ok "live affected carries real entries (max $max_n for $max_file)"
    else
      bad "live affected was EMPTY for all $sampled sampled files — shape passes but the verb returns nothing usable"
    fi
  else
    note "no probe source files found; affected live check skipped"
  fi

  # Error paths must fail loudly, never emit a neutral [].
  bash "$CGA" symbols >/dev/null 2>&1
  [ $? -eq 1 ] && ok "missing-argument exits 1 (not a silent [])" || bad "missing argument did not exit 1"

  # freshness: shape plus a contract exit. An UNSUPPORTED freshness must still
  # exit 2, never 0 — the contract requires "cannot determine" to be treated as
  # STALE, because reporting fresh on an answer the adapter could not actually
  # read is precisely the false-confidence failure this verb exists to prevent.
  out="$(bash "$CGA" freshness 2>/dev/null)"; rc=$?
  freshness_cap="$(cap_of freshness)"
  if [ "$freshness_cap" = "unsupported" ]; then
    if printf '%s' "$out" | assert_shape object && [ "$rc" -eq 2 ]; then
      ok "unsupported freshness reports STALE (exit 2), never a false 'fresh'"
    else
      bad "unsupported freshness must emit an object and exit 2 (got rc=$rc)"
    fi
  elif printf '%s' "$out" | assert_shape object && { [ "$rc" -eq 0 ] || [ "$rc" -eq 2 ]; }; then
    ok "live freshness is an object with a contract exit ($rc)"
  else
    bad "live freshness bad shape or exit (rc=$rc)"
  fi

  # The stale round trip needs a THROWAWAY index, and building one is the only
  # genuinely provider-specific step in this file. Rather than hard-code
  # `codegraph init`, the operator supplies it via CODEINDEX_SELFTEST_INIT_CMD
  # (evaluated with CWD set to the throwaway dir). Absent ⇒ noted skip, not a
  # silent pass.
  INIT_CMD="${CODEINDEX_SELFTEST_INIT_CMD:-}"
  if [ "$freshness_cap" = "unsupported" ]; then
    note "stale round trip skipped ($LIVE_ADAPTER declares freshness unsupported)"
  elif [ -z "$INIT_CMD" ]; then
    note "stale round trip skipped (set CODEINDEX_SELFTEST_INIT_CMD to build a throwaway index)"
  else
    probe_dir="$(mktemp -d)"
    printf 'def a(x):\n    return x + 1\n' > "$probe_dir/m.py"
    if (cd "$probe_dir" && DO_NOT_TRACK=1 eval "$INIT_CMD" >/dev/null 2>&1); then
      rc_fresh=0; rc_stale=0
      CODEINDEX_ROOT="$probe_dir" bash "$CGA" freshness >/dev/null 2>&1 || rc_fresh=$?
      printf 'def b(y):\n    return y - 1\n' > "$probe_dir/extra.py"
      CODEINDEX_ROOT="$probe_dir" bash "$CGA" freshness >/dev/null 2>&1 || rc_stale=$?
      if [ "$rc_fresh" -eq 0 ] && [ "$rc_stale" -eq 2 ]; then
        ok "ADVERSARIAL: freshness flips 0 -> 2 when the tree changes"
      else
        bad "ADVERSARIAL: freshness did not flip (fresh=$rc_fresh stale=$rc_stale; want 0 then 2)"
      fi
      # sync must actually HEAL, not merely exit 0. A sync that reports success
      # while leaving the index stale is the same false-confidence failure.
      rc_sync=0
      CODEINDEX_ROOT="$probe_dir" bash "$CGA" sync >/dev/null 2>&1 || rc_sync=$?
      rc_after=0
      CODEINDEX_ROOT="$probe_dir" bash "$CGA" freshness >/dev/null 2>&1 || rc_after=$?
      if [ "$rc_sync" -eq 0 ] && [ "$rc_after" -eq 0 ]; then
        ok "sync heals a stale index (stale -> sync -> fresh)"
      else
        bad "sync did not heal (sync=$rc_sync after=$rc_after; want 0 and 0)"
      fi
    else
      note "could not build throwaway index; stale round trip skipped"
    fi
    rm -rf "$probe_dir"
  fi
fi

echo ""
echo "  passed: $PASS   failed: $FAIL"
[ "$FAIL" -eq 0 ] || exit 1
echo "✅ codeindex-adapter-contract-selftest: all assertions passed"
exit 0
