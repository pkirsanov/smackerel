#!/usr/bin/env bash
# bubbles/scripts/scenario-impact-resolve-selftest.sh
#
# Hermetic selftest for scenario-impact-resolve.sh (IMP-040 SCOPE-9 / REG-8).
#
# The adversarial cases are the ones that prove the resolver closes the blind
# spot rather than restating it. A1 is the whole point: a diff that touches ONLY
# source, no spec folder, must still mark the certified scenario. A5 is its
# shared-consumer form — one edit, several specs' scenarios.
#
# P3 and P4 are the guards against over-reach: an UNCERTIFIED scenario has
# nothing to invalidate, and a scenario that declares no implementationRefs must
# stay inert rather than being flagged on every diff.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/scenario-impact-resolve.sh"
NAME="scenario-impact-resolve-selftest"

failures=0
checks=0
ok() { checks=$((checks + 1)); printf '  ok   %s\n' "$1"; }
bad() {
  checks=$((checks + 1)); failures=$((failures + 1))
  printf '  FAIL %s\n' "$1"
  [[ $# -gt 1 ]] && printf '       %s\n' "$2"
  return 0
}

WORK="$(mktemp -d)" || exit 2
trap 'rm -rf "$WORK"' EXIT INT TERM

make_case() {
  local root="$WORK/$1"
  mkdir -p "$root"
  printf '%s\n' "$2" >"$root/scenario-manifest.json"
  printf '%s' "$root"
}

run_impact() {
  local dir="$1"; shift
  set +e
  OUT="$(bash "$TARGET" "$dir" "$@" 2>&1)"
  RC=$?
  set -e
}

CERT='"evidenceRefs":["report.md#scn-1"]'

# --- A1. THE BLIND SPOT: a source-only diff marks a certified scenario ------
R="$(make_case a1 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",$CERT,\"implementationRefs\":[\"src/pricing/total.ts\"]}]}")"
run_impact "$R" --changed src/pricing/total.ts
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'REVALIDATE: SCN-001-001'; then
  ok "A1 a source-only diff marks the certified scenario for revalidation"
else
  bad "A1 source-only diff" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A2. a symbol-qualified ref still matches the file ----------------------
R="$(make_case a2 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",$CERT,\"implementationRefs\":[\"src/pricing/total.ts#computeTotal\"]}]}")"
run_impact "$R" --changed src/pricing/total.ts
if [[ "$RC" -eq 1 ]]; then
  ok "A2 a symbol-qualified ref matches a change to its file"
else
  bad "A2 symbol ref" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A3. a directory ref matches a file beneath it --------------------------
R="$(make_case a3 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",$CERT,\"implementationRefs\":[\"src/pricing/\"]}]}")"
run_impact "$R" --changed src/pricing/nested/rate.ts
if [[ "$RC" -eq 1 ]]; then
  ok "A3 a directory ref matches a file beneath it"
else
  bad "A3 directory ref" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A4. a lockdown counts as certification even with no evidenceRefs -------
R="$(make_case a4 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","lockdown":"certified-2026-08-12","implementationRefs":["src/a.ts"]}]}')"
run_impact "$R" --changed src/a.ts
if [[ "$RC" -eq 1 ]]; then
  ok "A4 a locked-down scenario is treated as certified"
else
  bad "A4 lockdown" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A5. SHARED CONSUMER: one edit marks every scenario that names it -------
R="$(make_case a5 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"a\",\"requiredTestType\":\"e2e-ui\",$CERT,\"implementationRefs\":[\"src/shared/client.ts\"]},{\"id\":\"SCN-001-002\",\"title\":\"b\",\"requiredTestType\":\"e2e-ui\",$CERT,\"implementationRefs\":[\"src/shared/client.ts\",\"src/ui/list.tsx\"]}]}")"
run_impact "$R" --changed src/shared/client.ts --format ids
if [[ "$RC" -eq 1 ]] \
  && printf '%s' "$OUT" | grep -qx 'SCN-001-001' \
  && printf '%s' "$OUT" | grep -qx 'SCN-001-002'; then
  ok "A5 one shared-consumer edit marks every scenario that names it"
else
  bad "A5 shared consumer fanout" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P1. an unrelated diff leaves everything certified ----------------------
R="$(make_case p1 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",$CERT,\"implementationRefs\":[\"src/pricing/total.ts\"]}]}")"
run_impact "$R" --changed docs/README.md
if [[ "$RC" -eq 0 ]]; then
  ok "P1 an unrelated diff impacts nothing"
else
  bad "P1 unrelated diff" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P2. a sibling with a shared prefix is NOT a match ----------------------
# `src/pricing/total.ts` must not match `src/pricing/total.ts.bak` or
# `src/pricing/totals.ts`; a substring test would, and would over-report so
# broadly the output would stop being read.
R="$(make_case p2 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",$CERT,\"implementationRefs\":[\"src/pricing/total.ts\"]}]}")"
run_impact "$R" --changed src/pricing/totals.ts
if [[ "$RC" -eq 0 ]]; then
  ok "P2 a sibling path sharing a prefix is not a match"
else
  bad "P2 prefix sibling" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P3. GUARD: an UNCERTIFIED scenario has nothing to invalidate -----------
R="$(make_case p3 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","title":"t","requiredTestType":"e2e-ui","implementationRefs":["src/pricing/total.ts"]}]}')"
run_impact "$R" --changed src/pricing/total.ts
if [[ "$RC" -eq 0 ]]; then
  ok "P3 an uncertified scenario is not flagged"
else
  bad "P3 uncertified" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P4. GUARD: no implementationRefs stays inert ---------------------------
R="$(make_case p4 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",$CERT}]}")"
run_impact "$R" --changed src/pricing/total.ts
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'inert'; then
  ok "P4 a scenario with no implementationRefs stays inert"
else
  bad "P4 inert" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P5. an empty diff impacts nothing --------------------------------------
R="$(make_case p5 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",$CERT,\"implementationRefs\":[\"src/a.ts\"]}]}")"
run_impact "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P5 an empty diff impacts nothing"
else
  bad "P5 empty diff" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P6. absent manifest is inert -------------------------------------------
mkdir -p "$WORK/p6"
run_impact "$WORK/p6" --changed src/a.ts
if [[ "$RC" -eq 0 ]]; then
  ok "P6 a spec dir with no manifest is inert"
else
  bad "P6 absent manifest" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P7. stdin diff input ----------------------------------------------------
R="$(make_case p7 "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",$CERT,\"implementationRefs\":[\"src/a.ts\"]}]}")"
set +e
OUT="$(printf 'src/a.ts\n' | bash "$TARGET" "$R" --changed-from - 2>&1)"
RC=$?
set -e
if [[ "$RC" -eq 1 ]]; then
  ok "P7 changed paths can be piped from a git diff on stdin"
else
  bad "P7 stdin" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A6. BARE-LIST envelope still marks impacted scenarios ------------------
# Real downstream manifests ship a top-level list. Reading only the object form
# raised AttributeError; silently skipping them would stop marking impacted
# scenarios in those specs, which is the failure this resolver exists to remove.
R="$(make_case a6 "[{\"id\":\"SCN-001-001\",\"title\":\"t\",\"requiredTestType\":\"e2e-ui\",$CERT,\"implementationRefs\":[\"src/pricing/total.ts\"]}]")"
run_impact "$R" --changed src/pricing/total.ts
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'REVALIDATE: SCN-001-001'; then
  ok "A6 a bare-list manifest still marks the impacted scenario"
else
  bad "A6 bare-list envelope" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- U1. usage ---------------------------------------------------------------
set +e
bash "$TARGET" >/dev/null 2>&1; u1=$?
bash "$TARGET" "$WORK/absent-dir" >/dev/null 2>&1; u2=$?
bypass="$(bash "$TARGET" --skip-impact 2>&1)"; u3=$?
set -e
if [[ "$u1" -eq 2 && "$u2" -eq 2 && "$u3" -eq 2 ]] && printf '%s' "$bypass" | grep -q 'bypass-shaped'; then
  ok "U1 missing arg, absent dir and a bypass flag all exit 2"
else
  bad "U1 usage" "noarg=$u1 absent=$u2 bypass=$u3"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
