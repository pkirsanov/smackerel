#!/usr/bin/env bash
# release-ladder-schema-guard-selftest.sh — hermetic selftest for Gate G137.
#
# Every case builds a throwaway repo in $TMPDIR. Nothing here touches the
# working tree, and no case depends on another case's state.
#
# The suite is deliberately adversarial. A schema guard that only ever sees
# well-formed input proves nothing: the cases that matter are the ones that
# MUST fail, because each of them is a way a release ladder can lie.
#
#   T0  non-existent repo root                          -> 2  (usage)
#   T1  no ladder declaration, no --ladder              -> 0  (EXEMPT)
#   T2  well-formed 3-phase ladder                      -> 0  (CONTROL)
#   T3  header declares a different phase than its dir  -> 1  (ADVERSARIAL)
#   T4  dependsOn omits a predecessor                   -> 1  (ADVERSARIAL)
#   T5  packet schemaVersion != 1                       -> 1  (ADVERSARIAL)
#   T6  ladder names a phase with no directory          -> 1  (ADVERSARIAL)
#   T7  binding missing mandatory delivery              -> 1  (ADVERSARIAL)
#   T8  delivery outside the closed vocabulary          -> 1  (ADVERSARIAL)
#   T9  deferred-to points BACKWARD                     -> 1  (ADVERSARIAL)
#   T10 deferred-to names an unknown phase              -> 1  (ADVERSARIAL)
#   T11 delivery=carried without carried-regression     -> 1  (ADVERSARIAL)
#   T12 assurance on a non-required binding             -> 1  (ADVERSARIAL)
#   T13 same capability id bound to two specs           -> 1  (ADVERSARIAL)
#   T14 ladder declaration with an empty phases= list   -> 1  (ADVERSARIAL)
#   T15 ladder declaring a duplicate phase              -> 1  (ADVERSARIAL)
#   T16 ladder read from the repo declaration itself    -> 0  (declaration path)
#
# NOTE: no `set -e`. Most cases here run the guard EXPECTING a non-zero exit,
# so errexit would abort the suite at the first adversarial case.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/release-ladder-schema-guard.sh"

if [[ ! -f "$GUARD" ]]; then
  echo "selftest: guard not found: $GUARD" >&2
  exit 2
fi

WORKSPACE="$(mktemp -d -t bubbles-ladder-selftest-XXXXXXXX)"
trap 'rm -rf "$WORKSPACE"' EXIT INT TERM

PASS_COUNT=0
FAIL_COUNT=0
RC=0

pass() {
  echo "  PASS: $1"
  PASS_COUNT=$((PASS_COUNT + 1))
}
bad() {
  echo "  FAIL: $1" >&2
  FAIL_COUNT=$((FAIL_COUNT + 1))
}

new_repo() {
  local name="$1"
  local d="$WORKSPACE/$name"
  mkdir -p "$d/docs/releases"
  printf '%s' "$d"
}

# Declare the ladder the way a product repo would.
mk_ladder() {
  local repo="$1" phases="$2"
  printf '# Releases\n\n<!-- bubbles:release-ladder schemaVersion=1 phases=%s -->\n' \
    "$phases" >"$repo/docs/releases/README.md"
}

# One packet: header + optional feature binding lines.
mk_packet() {
  local repo="$1" phase="$2" depends="$3" schema="$4"
  shift 4
  local dir="$repo/docs/releases/$phase"
  mkdir -p "$dir"
  {
    printf '# %s\n\n' "$phase"
    printf '<!-- bubbles:reconciled-packet schemaVersion=%s phase=%s dependsOn=%s -->\n\n' \
      "$schema" "$phase" "$depends"
    local ann
    for ann in "$@"; do
      printf '<!-- %s -->\n' "$ann"
    done
  } >"$dir/features.md"
}

run_guard() {
  bash "$GUARD" "$@" >/dev/null 2>&1
  RC=$?
}

expect_rc() {
  local want="$1" desc="$2"
  if [[ "$RC" -eq "$want" ]]; then
    pass "$desc (rc=$RC)"
  else
    bad "$desc (want $want, got $RC)"
  fi
}

echo "release-ladder-schema-guard selftest"

# T0 — non-existent repo root
run_guard --repo-root "$WORKSPACE/does-not-exist"
expect_rc 2 "T0 non-existent repo root"

# T1 — no declaration anywhere: EXEMPT, never a false finding
R1="$(new_repo t1)"
run_guard --repo-root "$R1"
expect_rc 0 "T1 no ladder declaration is EXEMPT (not a violation)"

# T2 — CONTROL. A well-formed ladder must pass, or every failing case below is meaningless.
R2="$(new_repo t2)"
mk_packet "$R2" alpha none 1 "bubbles:feature id=cap-a spec=specs/001-a delivery=required assurance=implemented"
mk_packet "$R2" beta alpha 1 "bubbles:feature id=cap-b spec=specs/002-b delivery=optional"
mk_packet "$R2" ga alpha,beta 1 "bubbles:feature id=cap-c spec=specs/003-c delivery=carried carried-regression=blocking"
run_guard --repo-root "$R2" --ladder "alpha beta ga"
expect_rc 0 "T2 CONTROL well-formed 3-phase ladder"

# T3 — header claims a phase it is not in
R3="$(new_repo t3)"
mk_packet "$R3" alpha none 1 "bubbles:feature id=cap-a spec=specs/001-a delivery=required"
mk_packet "$R3" beta alpha 1 "bubbles:feature id=cap-b spec=specs/002-b delivery=required"
sed -e 's/phase=beta/phase=alpha/' "$R3/docs/releases/beta/features.md" >"$R3/docs/releases/beta/features.tmp"
mv "$R3/docs/releases/beta/features.tmp" "$R3/docs/releases/beta/features.md"
run_guard --repo-root "$R3" --ladder "alpha beta"
expect_rc 1 "T3 header phase does not match its directory"

# T4 — dependsOn omits a predecessor (the silent prerequisite skip)
R4="$(new_repo t4)"
mk_packet "$R4" alpha none 1 "bubbles:feature id=cap-a spec=specs/001-a delivery=required"
mk_packet "$R4" beta alpha 1 "bubbles:feature id=cap-b spec=specs/002-b delivery=required"
mk_packet "$R4" ga beta 1 "bubbles:feature id=cap-c spec=specs/003-c delivery=required"
run_guard --repo-root "$R4" --ladder "alpha beta ga"
expect_rc 1 "T4 dependsOn omits a transitive predecessor"

# T5 — unsupported schemaVersion must be refused, not best-effort parsed
R5="$(new_repo t5)"
mk_packet "$R5" alpha none 2 "bubbles:feature id=cap-a spec=specs/001-a delivery=required"
run_guard --repo-root "$R5" --ladder "alpha"
expect_rc 1 "T5 packet schemaVersion!=1 is refused"

# T6 — ladder names a phase that does not exist on disk
R6="$(new_repo t6)"
mk_packet "$R6" alpha none 1 "bubbles:feature id=cap-a spec=specs/001-a delivery=required"
run_guard --repo-root "$R6" --ladder "alpha beta"
expect_rc 1 "T6 declared phase has no directory"

# T7 — a binding missing a mandatory field must never be a silent no-op
R7="$(new_repo t7)"
mk_packet "$R7" alpha none 1 "bubbles:feature id=cap-a spec=specs/001-a"
run_guard --repo-root "$R7" --ladder "alpha"
expect_rc 1 "T7 binding missing mandatory delivery"

# T8 — closed vocabulary
R8="$(new_repo t8)"
mk_packet "$R8" alpha none 1 "bubbles:feature id=cap-a spec=specs/001-a delivery=probably"
run_guard --repo-root "$R8" --ladder "alpha"
expect_rc 1 "T8 delivery outside the closed vocabulary"

# T9 — a backward defer is an unreachable promise
R9="$(new_repo t9)"
mk_packet "$R9" alpha none 1 "bubbles:feature id=cap-a spec=specs/001-a delivery=required"
mk_packet "$R9" beta alpha 1 "bubbles:feature id=cap-b spec=specs/002-b delivery=deferred-to:alpha"
run_guard --repo-root "$R9" --ladder "alpha beta"
expect_rc 1 "T9 deferred-to points BACKWARD"

# T10 — defer to a phase that is not on the ladder at all
R10="$(new_repo t10)"
mk_packet "$R10" alpha none 1 "bubbles:feature id=cap-a spec=specs/001-a delivery=deferred-to:omega"
run_guard --repo-root "$R10" --ladder "alpha beta"
mkdir -p "$R10/docs/releases/beta"
mk_packet "$R10" beta alpha 1 "bubbles:feature id=cap-b spec=specs/002-b delivery=required"
run_guard --repo-root "$R10" --ladder "alpha beta"
expect_rc 1 "T10 deferred-to names an unknown phase"

# T11 — carried without its regression posture
R11="$(new_repo t11)"
mk_packet "$R11" alpha none 1 "bubbles:feature id=cap-a spec=specs/001-a delivery=carried"
run_guard --repo-root "$R11" --ladder "alpha"
expect_rc 1 "T11 delivery=carried omits carried-regression"

# T12 — assurance is meaningless off a required binding
R12="$(new_repo t12)"
mk_packet "$R12" alpha none 1 "bubbles:feature id=cap-a spec=specs/001-a delivery=optional assurance=planned"
run_guard --repo-root "$R12" --ladder "alpha"
expect_rc 1 "T12 assurance on a non-required binding"

# T13 — identity drift makes "the same capability" unprovable across phases
R13="$(new_repo t13)"
mk_packet "$R13" alpha none 1 "bubbles:feature id=cap-a spec=specs/001-a delivery=required"
mk_packet "$R13" beta alpha 1 "bubbles:feature id=cap-a spec=specs/999-different delivery=required"
run_guard --repo-root "$R13" --ladder "alpha beta"
expect_rc 1 "T13 same capability id bound to two different specs"

# T14 — a declaration that names nothing would make the gate a silent no-op
R14="$(new_repo t14)"
printf '# Releases\n\n<!-- bubbles:release-ladder schemaVersion=1 -->\n' >"$R14/docs/releases/README.md"
run_guard --repo-root "$R14"
expect_rc 1 "T14 ladder declaration with no phases= list"

# T15 — a duplicated phase makes ladder ordering ambiguous
R15="$(new_repo t15)"
mk_packet "$R15" alpha none 1 "bubbles:feature id=cap-a spec=specs/001-a delivery=required"
run_guard --repo-root "$R15" --ladder "alpha alpha"
expect_rc 1 "T15 ladder declares a duplicate phase"

# T16 — the declaration path (not just --ladder) actually drives the run
R16="$(new_repo t16)"
mk_ladder "$R16" "alpha,beta"
mk_packet "$R16" alpha none 1 "bubbles:feature id=cap-a spec=specs/001-a delivery=required"
mk_packet "$R16" beta alpha 1 "bubbles:feature id=cap-b spec=specs/002-b delivery=required"
run_guard --repo-root "$R16"
expect_rc 0 "T16 ladder resolved from the repo declaration"

echo ""
echo "release-ladder-schema-guard selftest: $PASS_COUNT passed, $FAIL_COUNT failed"
if [[ "$FAIL_COUNT" -gt 0 ]]; then
  exit 1
fi
echo "PASS"
exit 0
