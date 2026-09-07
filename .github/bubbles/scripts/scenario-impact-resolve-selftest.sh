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

install_v2_schema() {
  local root="$1"
  mkdir -p "$root/bubbles/schemas"
  cp "$SCRIPT_DIR/../schemas/scenario-manifest-v2.schema.json" \
    "$root/bubbles/schemas/scenario-manifest-v2.schema.json"
}

run_impact() {
  local dir="$1"; shift
  set +e
  OUT="$(bash "$TARGET" "$dir" "$@" 2>&1)"
  RC=$?
  set -e
}

if grep -Fq 'scenario-reference-reader.py' "$TARGET" \
  && ! grep -Fq 'json.load(fh)' "$TARGET"; then
  ok "S1 manifest semantics are delegated to the canonical reader"
else
  bad "S1 canonical reader ownership"
fi

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

# --- V1/V2 controls and closed-parser adversaries ---------------------------
R="$(make_case v1 '{"schemaVersion":1,"scenarios":[{"scenarioId":"SCN-001-001","title":"t","requiredTestType":"unit","evidenceRefs":["report.md#v1"],"implementationRefs":["src/v1.ts"]}]}')"
run_impact "$R" --changed src/v1.ts --format ids
if [[ "$RC" -eq 1 && "$OUT" == "SCN-001-001" ]]; then
  ok "V1 a valid v1 identity alias is normalized and preserves impact output"
else
  bad "V1 valid v1 control" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

R="$(make_case v2 '{"schemaVersion":2,"scenarios":[{"id":"SCN-001-002","title":"t","requiredTestType":"unit","evidenceRefs":["report.md#v2"],"implementationRefs":["src/v2.ts"]}]}')"
install_v2_schema "$R"
run_impact "$R" --changed src/v2.ts --format ids
if [[ "$RC" -eq 1 && "$OUT" == "SCN-001-002" ]]; then
  ok "V2 a valid closed v2 manifest preserves impact output"
else
  bad "V2 valid v2 control" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

R="$(make_case a7 '{"schemaVersion":1,"schemaVersion":2,"scenarios":[]}')"
run_impact "$R"
if [[ "$RC" -eq 2 && "$OUT" == *"duplicate JSON member"* ]]; then
  ok "A7 duplicate JSON members fail closed through the canonical reader"
else
  bad "A7 duplicate member" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

R="$(make_case a8 '{"schemaVersion":2,"unexpected":true,"scenarios":[]}')"
install_v2_schema "$R"
run_impact "$R"
if [[ "$RC" -eq 2 && "$OUT" == *"Additional properties are not allowed"* ]]; then
  ok "A8 strict-v2 unknown fields fail closed through the canonical reader"
else
  bad "A8 v2 closed object" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

READER_SHIM="$WORK/reader-failure-bin"
mkdir -p "$READER_SHIM"
REAL_PYTHON3="$(command -v python3)"
cat >"$READER_SHIM/python3" <<'SHIM'
#!/usr/bin/env bash
if [[ "${1:-}" == */scenario-reference-reader.py ]]; then
  printf 'injected reader failure\n' >&2
  exit 19
fi
exec "$REAL_PYTHON3" "$@"
SHIM
chmod +x "$READER_SHIM/python3"
R="$(make_case a9 '{"schemaVersion":1,"scenarios":[]}')"
set +e
OUT="$(PATH="$READER_SHIM:$PATH" REAL_PYTHON3="$REAL_PYTHON3" bash "$TARGET" "$R" 2>&1)"
RC=$?
set -e
if [[ "$RC" -eq 2 && "$OUT" == *"injected reader failure"* ]]; then
  ok "A9 canonical reader failure is preserved and fails closed"
else
  bad "A9 reader failure" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A10. projection larger than the process environment is transported ----
# This payload exceeds common ARG_MAX/environment limits. The resolver must
# carry the canonical reader projection out-of-band rather than exporting it.
LARGE_ROOT="$WORK/a10"
mkdir -p "$LARGE_ROOT"
python3 - "$LARGE_ROOT/scenario-manifest.json" <<'PY'
import json
import sys

scenarios = []
for index in range(30000):
    scenarios.append({
        "id": f"SCN-900-{index:05d}",
        "title": "large projection transport adversary",
        "requiredTestType": "unit",
        "evidenceRefs": [f"report.md#large-{index}"],
        "implementationRefs": [f"src/large/component-{index}.ts"],
    })
with open(sys.argv[1], "w", encoding="utf-8") as manifest_file:
    json.dump({"schemaVersion": 1, "scenarios": scenarios}, manifest_file)
PY
run_impact "$LARGE_ROOT" --changed src/large/component-29999.ts --format ids
if [[ "$RC" -eq 1 && "$OUT" == "SCN-900-29999" ]]; then
  ok "A10 a projection larger than the process environment resolves without env transport"
else
  bad "A10 large projection transport" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A11. replaced projection path is contained and fully cleaned ----------
# An adversarial parser shim replaces the projection file after the reader
# writes it. Cleanup must remove the replacement and displaced original while
# never escaping the resolver-owned private directory.
REPLACEMENT_TMP="$WORK/a11-tmp"
REPLACEMENT_SHIM="$WORK/a11-bin"
mkdir -p "$REPLACEMENT_TMP" "$REPLACEMENT_SHIM"
cat >"$REPLACEMENT_SHIM/python3" <<'SHIM'
#!/usr/bin/env bash
if [[ "${1:-}" == "-" && "${2:-}" == */reference-projection.json ]]; then
  mv "$2" "$2.displaced"
  printf '{"replacement":true}\n' >"$2"
fi
exec "$REAL_PYTHON3" "$@"
SHIM
chmod +x "$REPLACEMENT_SHIM/python3"
R="$(make_case a11 '{"schemaVersion":1,"scenarios":[]}')"
set +e
OUT="$(TMPDIR="$REPLACEMENT_TMP" PATH="$REPLACEMENT_SHIM:$PATH" REAL_PYTHON3="$REAL_PYTHON3" bash "$TARGET" "$R" 2>&1)"
RC=$?
set -e
replacement_residue="$(find "$REPLACEMENT_TMP" -mindepth 1 -print)"
if [[ "$RC" -eq 2 && -z "$replacement_residue" ]]; then
  ok "A11 replacement of the projection path fails closed and leaves no owned temp residue"
else
  bad "A11 replacement-safe cleanup" "rc=$RC residue=$replacement_residue out=$(printf '%s' "$OUT" | tr '\n' '|')"
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
