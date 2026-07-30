#!/usr/bin/env bash
# bubbles/scripts/codeindex-resolve-selftest.sh — hermetic selftest for the
# code-index adapter contract (resolver + adapters).
#
# Fully hermetic: mktemp fixtures only, NO provider binary required, NO network,
# NO repository state read. Every case is adversarial in the sense that it would
# FAIL if the corresponding behavior regressed — in particular the block-scoping
# case (T6) and the path-traversal case (T8), which are the two ways a naive
# implementation silently breaks.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVE="$SCRIPT_DIR/codeindex-resolve.sh"
ADAPTERS="$(cd "$SCRIPT_DIR/.." && pwd)/adapters/codeindex"

PASS=0
FAIL=0

WORK="$(mktemp -d)"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

ok() {
  PASS=$((PASS + 1))
  echo "PASS: $1"
}
bad() {
  FAIL=$((FAIL + 1))
  echo "FAIL: $1"
}

# assert_line <label> <expected-line> <actual-output>
assert_line() {
  case "$3" in
    *"$2"*) ok "$1" ;;
    *) bad "$1 (expected line '$2'; got: $(printf '%s' "$3" | tr '\n' '|'))" ;;
  esac
}

assert_exit() {
  if [ "$2" -eq "$3" ]; then ok "$1"; else bad "$1 (expected exit $3, got $2)"; fi
}

new_repo() {
  local d="$WORK/$1"
  mkdir -p "$d/.github"
  echo "$d"
}

echo "=== codeindex-resolve selftest ==="

# ── T1: no config file at all -> neutral none, exit 0 ────────────────────────
R="$(new_repo t1)"
OUT="$(cd "$R" && bash "$RESOLVE" --repo-root "$R" 2>&1)"
RC=$?
assert_exit "T1 no config exits 0" "$RC" 0
assert_line "T1 no config resolves none" "adapter=none" "$OUT"

# ── T2: config present but no codeIndex block -> none ───────────────────────
R="$(new_repo t2)"
printf 'testImpact:\n  alwaysRun:\n    - smoke\n' >"$R/.github/bubbles-project.yaml"
OUT="$(bash "$RESOLVE" --repo-root "$R" 2>&1)"
RC=$?
assert_exit "T2 unrelated config exits 0" "$RC" 0
assert_line "T2 unrelated config resolves none" "adapter=none" "$OUT"

# ── T3: explicit none ───────────────────────────────────────────────────────
R="$(new_repo t3)"
printf 'codeIndex:\n  adapter: none\n' >"$R/.github/bubbles-project.yaml"
OUT="$(bash "$RESOLVE" --repo-root "$R" 2>&1)"
assert_line "T3 explicit none" "adapter=none" "$OUT"

# ── T4: opted in to codegraph -> name + resolvable path ─────────────────────
R="$(new_repo t4)"
printf 'codeIndex:\n  adapter: codegraph\n' >"$R/.github/bubbles-project.yaml"
OUT="$(bash "$RESOLVE" --repo-root "$R" 2>&1)"
RC=$?
assert_exit "T4 codegraph exits 0" "$RC" 0
assert_line "T4 codegraph resolved" "adapter=codegraph" "$OUT"
assert_line "T4 adapterPath emitted" "adapterPath=$ADAPTERS/codegraph.sh" "$OUT"

# ── T5: quoted value is unquoted ────────────────────────────────────────────
R="$(new_repo t5)"
printf 'codeIndex:\n  adapter: "codegraph"\n' >"$R/.github/bubbles-project.yaml"
OUT="$(bash "$RESOLVE" --repo-root "$R" 2>&1)"
assert_line "T5 quoted value unquoted" "adapter=codegraph" "$OUT"

# ── T6: ADVERSARIAL — `adapter:` OUTSIDE the codeIndex block must be ignored ─
# A naive `grep adapter:` implementation resolves codegraph here and fails.
R="$(new_repo t6)"
printf 'observability:\n  adapter: codegraph\ncodeIndex:\n  adapter: none\n' \
  >"$R/.github/bubbles-project.yaml"
OUT="$(bash "$RESOLVE" --repo-root "$R" 2>&1)"
assert_line "T6 foreign adapter key ignored (block scoping)" "adapter=none" "$OUT"

# ── T7: ADVERSARIAL — commented-out adapter must not be honored ─────────────
R="$(new_repo t7)"
printf 'codeIndex:\n  # adapter: codegraph\n' >"$R/.github/bubbles-project.yaml"
OUT="$(bash "$RESOLVE" --repo-root "$R" 2>&1)"
assert_line "T7 commented adapter ignored" "adapter=none" "$OUT"

# ── T8: ADVERSARIAL — path traversal must fail loud, never resolve ──────────
R="$(new_repo t8)"
printf 'codeIndex:\n  adapter: ../../../etc/passwd\n' >"$R/.github/bubbles-project.yaml"
OUT="$(bash "$RESOLVE" --repo-root "$R" 2>&1)"
RC=$?
assert_exit "T8 path traversal rejected" "$RC" 1

# ── T9: ADVERSARIAL — unknown adapter fails loud (never silent none) ────────
# Silent degradation would hide an operator typo behind a green run.
R="$(new_repo t9)"
printf 'codeIndex:\n  adapter: bogusprovider\n' >"$R/.github/bubbles-project.yaml"
OUT="$(bash "$RESOLVE" --repo-root "$R" 2>&1)"
RC=$?
assert_exit "T9 unknown adapter fails loud" "$RC" 1

# ── T10: --names-only suppresses path/root ──────────────────────────────────
R="$(new_repo t10)"
printf 'codeIndex:\n  adapter: codegraph\n' >"$R/.github/bubbles-project.yaml"
OUT="$(bash "$RESOLVE" --repo-root "$R" --names-only 2>&1)"
assert_line "T10 names-only prints adapter" "adapter=codegraph" "$OUT"
case "$OUT" in
  *adapterPath=*) bad "T10 names-only must not print adapterPath" ;;
  *) ok "T10 names-only suppresses adapterPath" ;;
esac

# ── T11: none.sh neutral shapes ─────────────────────────────────────────────
for v in symbols impact affected routes; do
  OUT="$(bash "$ADAPTERS/none.sh" "$v" 2>&1)"
  if [ "$OUT" = "[]" ]; then ok "T11 none.sh $v -> []"; else bad "T11 none.sh $v -> '$OUT' (expected [])"; fi
done
OUT="$(bash "$ADAPTERS/none.sh" status 2>&1)"
if [ "$OUT" = "{}" ]; then ok "T11 none.sh status -> {}"; else bad "T11 none.sh status -> '$OUT' (expected {})"; fi

# ── T12: none.sh unknown verb fails ─────────────────────────────────────────
bash "$ADAPTERS/none.sh" bogusverb >/dev/null 2>&1
assert_exit "T12 none.sh unknown verb exits 1" "$?" 1

# ── T13: codegraph.sh shape selftest works with NO provider installed ───────
for v in symbols impact affected routes; do
  OUT="$(bash "$ADAPTERS/codegraph.sh" selftest "$v" 2>&1)"
  if [ "$OUT" = "[]" ]; then ok "T13 codegraph.sh selftest $v -> []"; else bad "T13 codegraph.sh selftest $v -> '$OUT'"; fi
done
OUT="$(bash "$ADAPTERS/codegraph.sh" selftest status 2>&1)"
if [ "$OUT" = "{}" ]; then ok "T13 codegraph.sh selftest status -> {}"; else bad "T13 codegraph.sh selftest status -> '$OUT'"; fi

# ── T14: ADVERSARIAL — codegraph.sh live verb with provider absent must exit 1
# It must FAIL LOUD ("unavailable"), never emit a neutral [] that a consumer
# would mistake for "indexed, and nothing found".
R="$(new_repo t14)"
OUT="$(CODEINDEX_ROOT="$R" CODEINDEX_CODEGRAPH_BIN="definitely-not-installed-$$" \
  bash "$ADAPTERS/codegraph.sh" routes 2>&1)"
RC=$?
assert_exit "T14 missing provider exits 1 (not neutral)" "$RC" 1
case "$OUT" in
  '[]' | '{}') bad "T14 missing provider must not emit a neutral value" ;;
  *) ok "T14 missing provider emits an error, not a neutral value" ;;
esac

# ── T15: codegraph.sh unknown verb fails ────────────────────────────────────
bash "$ADAPTERS/codegraph.sh" bogusverb >/dev/null 2>&1
assert_exit "T15 codegraph.sh unknown verb exits 1" "$?" 1

# ── T16: every adapter implements all 8 contract verbs via selftest ─────────
# The contract is EIGHT verbs, not five. This loop previously checked only
# symbols/impact/affected/routes/status, so `indexed`, `freshness` and `sync`
# — the three that carry the staleness and indexability signals — could be
# dropped by a new adapter without any selftest noticing.
for adapter in "$ADAPTERS"/*.sh; do
  name="$(basename "$adapter")"
  missing=''
  for v in symbols impact affected routes indexed status freshness sync; do
    bash "$adapter" selftest "$v" >/dev/null 2>&1 || missing="$missing $v"
  done
  if [ -z "$missing" ]; then
    ok "T16 $name implements all 8 contract verbs"
  else
    bad "T16 $name missing verb selftest:$missing"
  fi
done

# ── T17: `capabilities` is OPTIONAL, but must be a JSON map when present ────
# An adapter may decline to declare capabilities (absent ⇒ no restrictions
# claimed). What it may NOT do is emit something a consumer would misread —
# an array, or a bare token — so the shape is checked whenever it is offered.
for adapter in "$ADAPTERS"/*.sh; do
  name="$(basename "$adapter")"
  if OUT="$(bash "$adapter" capabilities 2>/dev/null)"; then
    case "$OUT" in
      '{'*'}') ok "T17 $name capabilities emits a JSON map" ;;
      *) bad "T17 $name capabilities must emit a JSON map, got: $OUT" ;;
    esac
  else
    ok "T17 $name declines capabilities (optional verb)"
  fi
done

echo ""
echo "codeindex-resolve selftest: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
exit 0
