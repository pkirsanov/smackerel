#!/usr/bin/env bash
# bubbles/scripts/scenario-test-resolve-selftest.sh
#
# Hermetic selftest for scenario-test-resolve.sh (IMP-040 SCOPE-2 / COV-8).
#
# BUG-030 requires two specific red fixtures, and they are cases A1 and A6:
#   A1  a real file with an ABSENT title              (the reproduced false pass)
#   A6  a unit test linked as required E2E coverage   (category substitution)
#
# The green half matters just as much. Cases P3-P6 pin the four live reference
# shapes and both scenario-id spellings, because a resolver that only understood
# one of them would fail every packet written against the other document — and a
# gate that blocks everything carries no more information than one that sleeps.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TARGET="$SCRIPT_DIR/scenario-test-resolve.sh"
NAME="scenario-test-resolve-selftest"

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

# CR-SPEC-01/02: both production consumers must delegate raw manifest-schema
# interpretation to scenario-reference-reader.py. They may parse the reader's
# normalized projection, but must never feed the raw manifest directly to an
# embedded json.load. This static twin makes removing either delegation or
# adding a second schema reader an observable regression.
TRACEABILITY_TARGET="$SCRIPT_DIR/traceability-guard.sh"
if grep -Fq 'python3 "$REFERENCE_READER" "$MANIFEST"' "$TARGET" &&
   grep -Fq 'python3 "$reference_reader" "$scenario_manifest_file"' "$TRACEABILITY_TARGET" &&
   ! grep -Eq 'json\.load\([^)]*(MANIFEST|scenario_manifest_file)' "$TARGET" "$TRACEABILITY_TARGET"; then
  ok "shared-reader ownership: both consumers delegate raw manifests without local schema parsing"
else
  bad "shared-reader ownership: a consumer bypasses the shared reader or locally parses a raw manifest"
fi

# $1 = case name, $2 = manifest JSON body. Creates a repo with one test file.
make_case() {
  local root="$WORK/$1"
  mkdir -p "$root/specs/001-x" "$root/tests"
  git -C "$root" init -q . 2>/dev/null || true
  cat >"$root/tests/demo.spec.ts" <<'EOF'
test("visible outcome renders", () => {});
test("duplicated title", () => {});
test("duplicated title", () => {});
EOF
  printf '%s\n' "$2" >"$root/specs/001-x/scenario-manifest.json"
  printf '%s' "$root"
}

run_resolve() {
  set +e
  OUT="$(bash "$TARGET" "$1/specs/001-x" --repo-root "$1" 2>&1)"
  RC=$?
  set -e
}

REAL_PYTHON3="$(command -v python3)"

run_resolve_with_mode_operation() {
  local root="$1"
  local operation="$2"
  local shim_dir="$WORK/.mode-operation-bin"
  local real_python="$REAL_PYTHON3"
  mkdir -p "$shim_dir"
  cat > "$shim_dir/python3" <<'SHIM'
#!/usr/bin/env bash
set -u
: "${MODE_REAL_PYTHON:?missing delegated python3 path}"
: "${MODE_TARGET:?missing mode target}"
: "${MODE_OPERATION:?missing mode operation}"
capture="$(mktemp)"
trap 'rm -f "$capture"' EXIT INT TERM
status=0
"$MODE_REAL_PYTHON" "$@" >"$capture" || status=$?
if [[ "$status" -eq 0 && "${1:-}" == */scenario-reference-reader.py ]]; then
  current_mode="$(stat -c '%a' "$MODE_TARGET" 2>/dev/null || stat -f '%Lp' "$MODE_TARGET")"
  case "$MODE_OPERATION" in
    change) chmod 600 "$MODE_TARGET" ;;
    same) chmod "$current_mode" "$MODE_TARGET" ;;
    replace)
      replacement="${MODE_TARGET}.replacement"
      printf '%s\n' 'test("replacement title", () => {});' >"$replacement"
      mv "$replacement" "$MODE_TARGET"
      ;;
    symlink)
      replacement="${MODE_TARGET}.replacement"
      printf '%s\n' 'test("replacement title", () => {});' >"$replacement"
      rm -f "$MODE_TARGET"
      ln -s "$replacement" "$MODE_TARGET"
      ;;
    duplicateprojection)
      printf '%s\n' '{"scenarios":[],"scenarios":[]}' >"$capture"
      ;;
  esac
fi
cat "$capture"
exit "$status"
SHIM
  chmod +x "$shim_dir/python3"
  set +e
  OUT="$(PATH="$shim_dir:$PATH" MODE_REAL_PYTHON="$real_python" \
    MODE_TARGET="$root/tests/demo.spec.ts" MODE_OPERATION="$operation" \
    bash "$TARGET" "$root/specs/001-x" --repo-root "$root" 2>&1)"
  RC=$?
  set -e
}

# --- A0b/A0c. opened-object binding rejects replacement and symlink swaps ---
for operation in replace symlink; do
  R="$(make_case "scenario-$operation" '{"schemaVersion":1,"scenarios":[{"id":"SCN-RACE-001","linkedTests":["tests/demo.spec.ts#visible outcome renders"]}]}')"
  run_resolve_with_mode_operation "$R" "$operation"
  if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'STALE-FILE-IDENTITY'; then
    ok "A0-${operation} scenario file $operation after projection is refused before title scanning"
  else
    bad "A0-${operation} scenario file race refusal" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
  fi
done

R="$(make_case scenario-duplicate-projection '{"schemaVersion":1,"scenarios":[{"id":"SCN-DUP-001","linkedTests":["tests/demo.spec.ts"]}]}')"
run_resolve_with_mode_operation "$R" duplicateprojection
if [[ "$RC" -eq 2 ]] && printf '%s' "$OUT" | grep -q 'duplicate JSON member'; then
  ok "A0-duplicate-projection repeated projection members are refused"
else
  bad "A0-duplicate-projection member refusal" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# IMP-047 PD-04. The defect was host-shaped: a bare `timeout` does not exist on
# a stock macOS PATH, the command substitution failed, and the resolver reported
# OK. A selftest that only ever runs on the developer's own PATH cannot see
# that, so this helper runs the resolver with every timeout implementation
# hidden.
#
# The shim mirrors the WHOLE of the caller's PATH and omits exactly two names,
# `timeout` and `gtimeout`. An allowlist of "the tools I think are needed" was
# tried first and silently produced a vacuous pass: `test-inventory-resolve.sh`
# needs `awk`, the allowlist did not have it, the adapter resolved to `none`,
# and the fixture proved nothing. Mirroring instead of allowlisting means a new
# dependency in any resolver cannot quietly hollow this case out.
NOTIMEOUT_BIN="$WORK/.notimeout-bin"
mkdir -p "$NOTIMEOUT_BIN"
required_mirrors="awk bash basename cat chmod dirname grep head kill ln mktemp rm sed sleep stat tr"
missing_mirrors=""
for required_mirror in $required_mirrors; do
  mirror_source="$(type -P "$required_mirror" 2>/dev/null || true)"
  if [[ -n "$mirror_source" && -x "$mirror_source" ]]; then
    ln -sf "$mirror_source" "$NOTIMEOUT_BIN/$required_mirror"
  else
    missing_mirrors="$missing_mirrors $required_mirror"
  fi
done
unset required_mirror mirror_source

mkdir -p "$NOTIMEOUT_BIN"
cat > "$NOTIMEOUT_BIN/python3" <<'SHIM'
#!/usr/bin/env bash
set -u
: "${BUBBLES_REAL_PYTHON3:?missing delegated python3 path}"
: "${BUBBLES_PYTHON_MARKER:?missing python3 invocation marker}"
printf 'python3 invoked\n' >> "$BUBBLES_PYTHON_MARKER"
exec "$BUBBLES_REAL_PYTHON3" "$@"
SHIM
chmod +x "$NOTIMEOUT_BIN/python3"

# Non-vacuity guard on the shim itself. If either binary leaks through, A6b and
# P8b stop testing the no-timeout host and start re-testing A6 and P8.
if [[ -e "$NOTIMEOUT_BIN/timeout" || -e "$NOTIMEOUT_BIN/gtimeout" ]]; then
  bad "shim integrity: the no-timeout PATH still exposes a \`timeout\`/\`gtimeout\` binary"
elif [[ -x "$NOTIMEOUT_BIN/python3" && -z "$missing_mirrors" ]]; then
  ok "shim integrity: deterministic PATH mirror contains dirname and every required tool"
else
  bad "shim integrity: deterministic PATH mirror or delegated python3 shim is incomplete" "missing:$missing_mirrors"
fi

run_resolve_without_timeout() {
  local marker="$2"
  rm -f "$marker"
  set +e
  OUT="$(PATH="$NOTIMEOUT_BIN" BUBBLES_REAL_PYTHON3="$REAL_PYTHON3" \
    BUBBLES_PYTHON_MARKER="$marker" \
    /bin/bash "$TARGET" "$1/specs/001-x" --repo-root "$1" 2>&1)"
  RC=$?
  set -e
}

assert_no_timeout_python_invoked() {
  local label="$1"
  local marker="$2"
  if [[ -s "$marker" ]] && grep -q '^python3 invoked$' "$marker"; then
    ok "$label invokes the delegated python3 interpreter"
  else
    bad "$label did not invoke python3; the timeout-less fixture is vacuous"
  fi
}

# --- P1. no manifest is NA, not a failure -----------------------------------
R="$WORK/p1"; mkdir -p "$R/specs/001-x"
run_resolve "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P1 a spec with no scenario-manifest.json is NA"
else
  bad "P1 no manifest is NA" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P2. a resolvable title passes ------------------------------------------
R="$(make_case p2 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","requiredTestType":"e2e-ui","linkedTests":["tests/demo.spec.ts#visible outcome renders"]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P2 a title that exists resolves"
else
  bad "P2 resolvable title" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A0/P0. mode is identity, while an unchanged chmod is a positive control -
R="$(make_case mode-identity '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-000","linkedTests":["tests/demo.spec.ts#visible outcome renders"]}]}')"
run_resolve_with_mode_operation "$R" change
changed_rc="$RC"; changed_out="$OUT"
chmod 644 "$R/tests/demo.spec.ts"
run_resolve_with_mode_operation "$R" same
if [[ "$changed_rc" -eq 1 ]] && printf '%s' "$changed_out" | grep -q 'STALE-FILE-IDENTITY' &&
   [[ "$RC" -eq 0 ]]; then
  ok "A0/P0 chmod-only identity changes are refused and unchanged-mode operations remain accepted"
else
  bad "A0/P0 permission-mode identity controls" "changed_rc=$changed_rc changed_out=$(printf '%s' "$changed_out" | tr '\n' '|') same_rc=$RC same_out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P3. bare path (no title) is file-existence only ------------------------
# The single most important non-regression case: most existing packets declare
# a path with NO title. Enforcing titles unconditionally would fail all of them.
R="$(make_case p3 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","requiredTestType":"e2e-ui","linkedTests":["tests/demo.spec.ts"]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P3 a bare path with no title is file-existence only"
else
  bad "P3 bare path" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P4. object form without a title ----------------------------------------
R="$(make_case p4 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","requiredTestType":"e2e-ui","linkedTests":[{"file":"tests/demo.spec.ts"}]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P4 object form without a title is accepted"
else
  bad "P4 object form no title" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P5. object form with testId (the schema-guide shape) -------------------
R="$(make_case p5 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","requiredTestType":"e2e-ui","linkedTests":[{"file":"tests/demo.spec.ts","testId":"visible outcome renders"}]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P5 object form with testId resolves"
else
  bad "P5 object testId" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P6. the OTHER scenario-id spelling ------------------------------------
# The JSON schema says `id`; CONTROL_PLANE_SCHEMAS.md and the current guard say
# `scenarioId`. A repo following either document is not wrong.
R="$(make_case p6 '{"schemaVersion":1,"scenarios":[{"scenarioId":"SCN-001-009","requiredTestType":"e2e-ui","linkedTests":["tests/demo.spec.ts#nope not here"]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'SCN-001-009'; then
  ok "P6 the scenarioId spelling is read and named in the finding"
else
  bad "P6 scenarioId spelling" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P6b. equivalent dual-key identity counts the scenario object once ------
R="$(make_case p6b '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-010","scenarioId":"SCN-001-010","requiredTestType":"e2e-ui","linkedTests":["tests/demo.spec.ts#visible outcome renders"]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'OK.*1 reference(s) resolved' &&
  ! printf '%s' "$OUT" | grep -q '2 reference(s) resolved'; then
  ok "P6b equivalent id and scenarioId identify one scenario object and resolve its authored reference once"
else
  bad "P6b equivalent dual-key identity exactly once" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P7. planning sentinel is skipped, not treated as a missing file --------
R="$(make_case p7 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","requiredTestType":"e2e-ui","linkedTests":[{"path":"__FUTURE_TEST__","title":"future behavior","testState":"planned-not-authored"}]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'NA.*planned test reference(s) remain unauthored' &&
  ! printf '%s' "$OUT" | grep -q ' OK '; then
  ok "P7 a __FUTURE_TEST__ sentinel is NA and remains planned, not authored"
else
  bad "P7 sentinel planning classification" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P7b. empty linkedTests + plannedTests is explicitly unauthored ---------
R="$(make_case p7b '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-002","requiredTestType":"e2e-ui","linkedTests":[],"plannedTests":[{"path":"tests/future.spec.ts","title":"future behavior","type":"e2e-ui"}]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'NA.*0 authored linked-test reference(s) checked' &&
  printf '%s' "$OUT" | grep -q '1 planned test reference(s) remain unauthored' &&
  ! printf '%s' "$OUT" | grep -q ' OK '; then
  ok "P7b plannedTests with no linkedTests reports unauthored NA, never OK"
else
  bad "P7b planned-only wording" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P7d. legacy linkedTests testState remains planned ----------------------
R="$(make_case p7d '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-004","linkedTests":[{"file":"tests/not-authored.spec.ts","title":"future behavior","type":"e2e-ui","testState":"planned-not-authored"}]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q '1 planned test reference(s) remain unauthored' &&
  ! printf '%s' "$OUT" | grep -q 'MISSING-FILE\| OK '; then
  ok "P7d legacy testState planned reference is never treated as authored"
else
  bad "P7d legacy testState planning classification" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P7e. mixed authored and planned refs resolve only authored ------------
R="$(make_case p7e '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-005","linkedTests":["tests/demo.spec.ts"],"plannedTests":[{"path":"tests/not-authored.spec.ts","title":"future behavior","type":"e2e-ui"}]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'OK.*1 reference(s) resolved' &&
  ! printf '%s' "$OUT" | grep -q 'MISSING-FILE'; then
  ok "P7e mixed references resolve the authored file without requiring the planned file"
else
  bad "P7e mixed authored/planned resolution" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P7c. no authored or planned references is explicitly unclassified ------
R="$(make_case p7c '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-003","linkedTests":[]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'NA.*manifest is unclassified' &&
  printf '%s' "$OUT" | grep -q 'no authored or planned test references' &&
  ! printf '%s' "$OUT" | grep -q ' OK '; then
  ok "P7c zero references reports unclassified NA, never OK"
else
  bad "P7c zero-reference wording" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A1. ADVERSARIAL (BUG-030 red fixture): real file, ABSENT title ---------
R="$(make_case a1 '{"schemaVersion":1,"scenarios":[{"id":"SCN-011-001","requiredTestType":"e2e-ui","linkedTests":["tests/demo.spec.ts#Regression BS-001: high-persistence forecast stays elevated"]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'MISSING-TITLE'; then
  ok "A1 BUG-030: a real file with an absent title is refused"
else
  bad "A1 absent title refused" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A2. ADVERSARIAL: ambiguous title ---------------------------------------
R="$(make_case a2 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-002","requiredTestType":"e2e-ui","linkedTests":["tests/demo.spec.ts#duplicated title"]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'AMBIGUOUS-TITLE'; then
  ok "A2 a title matching more than one test is refused"
else
  bad "A2 ambiguous title" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A3. ADVERSARIAL: missing file ------------------------------------------
R="$(make_case a3 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-003","requiredTestType":"e2e-ui","linkedTests":["tests/absent.spec.ts"]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 2 ]] && printf '%s' "$OUT" | grep -q 'authored path is not an existing stable regular file'; then
  ok "A3 the shared reader refuses a reference to a non-existent file"
else
  bad "A3 missing file" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A4. ADVERSARIAL: path escaping the repository --------------------------
R="$(make_case a4 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-004","requiredTestType":"e2e-ui","linkedTests":["../../etc/passwd"]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 2 ]] && printf '%s' "$OUT" | grep -q 'lexical traversal'; then
  ok "A4 a path escaping the repository root is refused"
else
  bad "A4 outside repo" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A4b. ADVERSARIAL: normalized-looking authored traversal is rejected ----
R="$(make_case a4b '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-014","linkedTests":["tests/../tests/demo.spec.ts"]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 2 ]] && printf '%s' "$OUT" | grep -q 'lexical traversal'; then
  ok "A4b an authored path with a lexical traversal segment is refused"
else
  bad "A4b authored lexical traversal" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A4c. ADVERSARIAL: planned traversal is rejected without existence check -
R="$(make_case a4c '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-015","linkedTests":[],"plannedTests":[{"path":"tests/../future.spec.ts","title":"future behavior","type":"e2e-ui"}]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 2 ]] && printf '%s' "$OUT" | grep -q 'lexical traversal'; then
  ok "A4c a planned path with lexical traversal is refused"
else
  bad "A4c planned lexical traversal" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A4d/P7f. authored symlinks are refused regardless of containment ------
R="$(make_case a4d '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-016","linkedTests":["tests/escape.spec.ts"]}]}')"
printf 'outside repository\n' > "$WORK/outside.spec.ts"
ln -s "$WORK/outside.spec.ts" "$R/tests/escape.spec.ts"
run_resolve "$R"
if [[ "$RC" -eq 2 ]] && printf '%s' "$OUT" | grep -q 'authored path is not an existing stable regular file'; then
  ok "A4d an escaping authored symlink is refused as unstable"
else
  bad "A4d escaping authored symlink refusal" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

R="$(make_case p7f '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-017","linkedTests":["tests/internal-link.spec.ts"]}]}')"
ln -s demo.spec.ts "$R/tests/internal-link.spec.ts"
run_resolve "$R"
if [[ "$RC" -eq 2 ]] && printf '%s' "$OUT" | grep -q 'authored path is not an existing stable regular file'; then
  ok "P7f a contained authored symlink is refused as unstable"
else
  bad "P7f contained authored symlink refusal" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A5. the finding names the scenario, the reference, and the reason ------
R="$(make_case a5 '{"schemaVersion":1,"scenarios":[{"id":"SCN-042-007","requiredTestType":"e2e-ui","linkedTests":["tests/demo.spec.ts#absent one"]}]}')"
run_resolve "$R"
if printf '%s' "$OUT" | grep -q 'SCN-042-007' &&
  printf '%s' "$OUT" | grep -q 'tests/demo.spec.ts#absent one' &&
  printf '%s' "$OUT" | grep -q 'declares no structural test with this exact title'; then
  ok "A5 the finding names scenario, reference and reason"
else
  bad "A5 finding content" "out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A6. ADVERSARIAL (BUG-030 red fixture): unit test as required E2E -------
# Needs an inventory adapter, because only a runner can report a category.
R="$WORK/a6"
mkdir -p "$R/specs/001-x" "$R/tests" "$R/scripts" "$R/.github"
git -C "$R" init -q . 2>/dev/null || true
printf 'test("computes a total", () => {});\n' >"$R/tests/unit.spec.ts"
cat >"$R/.github/bubbles-project.yaml" <<'EOF'
testDiscovery:
  adapter: command
  command: scripts/inv
EOF
cat >"$R/scripts/inv" <<'EOF'
#!/bin/sh
cat <<'JSON'
{"contractVersion":"bubbles-test-inventory/v1","tests":[
 {"id":"t1","file":"tests/unit.spec.ts","title":"computes a total","category":"unit","runner":"jest","tags":[]}
]}
JSON
EOF
chmod +x "$R/scripts/inv"
cat >"$R/specs/001-x/scenario-manifest.json" <<'EOF'
{"schemaVersion":1,"scenarios":[{"id":"SCN-001-010","requiredTestType":"e2e-ui","linkedTests":["tests/unit.spec.ts#computes a total"]}]}
EOF
run_resolve "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'CATEGORY-MISMATCH'; then
  ok "A6 BUG-030: a unit test linked as required e2e-ui coverage is refused"
else
  bad "A6 unit-as-e2e refused" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A6b. ADVERSARIAL (IMP-047 PD-04): the SAME refusal with NO `timeout` ---
# This is the case that shipped broken. A6's fixture is re-run with every
# timeout implementation removed from PATH. Before the repair the bare `timeout`
# call failed, the resolver degraded to the literal scan, printed
# "category comparison(s) skipped", and exited 0 — a unit test certified as
# e2e-ui coverage on the exact host this framework is developed on.
# Reverting to a bare `timeout` makes this case exit 0 again.
A6B_PYTHON_MARKER="$WORK/.a6b-python-invoked"
run_resolve_without_timeout "$R" "$A6B_PYTHON_MARKER"
assert_no_timeout_python_invoked "A6b" "$A6B_PYTHON_MARKER"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'CATEGORY-MISMATCH'; then
  ok "A6b PD-04: unit-as-e2e is still refused with no \`timeout\`/\`gtimeout\` on PATH"
else
  bad "A6b unit-as-e2e refused without timeout" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P8. the same inventory accepts a correctly-categorised test ------------
# Guards A6 against over-matching: the category check must accept a match.
cat >"$R/scripts/inv" <<'EOF'
#!/bin/sh
cat <<'JSON'
{"contractVersion":"bubbles-test-inventory/v1","tests":[
 {"id":"t1","file":"tests/unit.spec.ts","title":"computes a total","category":"e2e-ui","runner":"playwright","tags":[]}
]}
JSON
EOF
chmod +x "$R/scripts/inv"
run_resolve "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P8 a correctly-categorised test resolves through the inventory"
else
  bad "P8 category match accepted" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A6c-A6e. executable races after approval cannot become execution -------
# The python shim waits until test-inventory-resolve has fstat-approved the
# original executable, then replaces its pathname with a different regular file,
# a symlink, or same-inode/same-size content with the original mtime restored.
# The execution process must compare the emitted identity and digest against its
# own no-follow descriptor and refuse rather than run any substitute.
INVENTORY_RACE_BIN="$WORK/.inventory-race-bin"
INVENTORY_RACE_MARKER="$WORK/.inventory-race-executed"
mkdir -p "$INVENTORY_RACE_BIN"
cat >"$INVENTORY_RACE_BIN/python3" <<'SHIM'
#!/usr/bin/env bash
set -u
: "${INVENTORY_RACE_REAL_PYTHON:?}" "${INVENTORY_RACE_TARGET:?}" "${INVENTORY_RACE_MARKER:?}" "${INVENTORY_RACE_OPERATION:?}"
capture="$(mktemp)"
trap 'rm -f "$capture"' EXIT INT TERM
status=0
"$INVENTORY_RACE_REAL_PYTHON" "$@" >"$capture" || status=$?
if [[ "$status" -eq 0 && "${1:-}" == "-" &&
  ( "${2:-}" == "$INVENTORY_RACE_TARGET" ||
    ( "${2:-}" == "scripts/inv" && "${3:-}" == "$(dirname "$(dirname "$INVENTORY_RACE_TARGET")")" ) ) ]]; then
  replacement="${INVENTORY_RACE_TARGET}.replacement"
  cat >"$replacement" <<EOF
#!/usr/bin/env bash
printf 'replacement executed\\n' >'$INVENTORY_RACE_MARKER'
printf '%s\\n' '{"contractVersion":"bubbles-test-inventory/v1","tests":[]}'
EOF
  chmod +x "$replacement"
  case "$INVENTORY_RACE_OPERATION" in
    replace) mv "$replacement" "$INVENTORY_RACE_TARGET" ;;
    symlink)
      rm -f "$INVENTORY_RACE_TARGET"
      ln -s "$replacement" "$INVENTORY_RACE_TARGET"
      ;;
    content)
      "$INVENTORY_RACE_REAL_PYTHON" -c '
import os, sys
path, marker = sys.argv[1:]
metadata = os.stat(path, follow_symlinks=False)
payload = ("#!/bin/sh\n: >" + repr(marker) + "\nprintf '\''%s\\n'\'' "
           "'\''{\"contractVersion\":\"bubbles-test-inventory/v1\",\"tests\":[]}'\''\n").encode()
if len(payload) > metadata.st_size:
    raise SystemExit("mutation payload exceeds approved file size")
payload += b"#" * (metadata.st_size - len(payload))
with open(path, "r+b", buffering=0) as stream:
    stream.write(payload)
    os.fsync(stream.fileno())
os.utime(path, ns=(metadata.st_atime_ns, metadata.st_mtime_ns), follow_symlinks=False)
' "$INVENTORY_RACE_TARGET" "$INVENTORY_RACE_MARKER"
      rm -f "$replacement"
      ;;
    intermediate)
      rm -f "$replacement"
      mv "$(dirname "$INVENTORY_RACE_TARGET")" "$(dirname "$INVENTORY_RACE_TARGET")-original"
      ln -s "$(basename "$(dirname "$INVENTORY_RACE_TARGET")")-original" "$(dirname "$INVENTORY_RACE_TARGET")"
      ;;
  esac
fi
cat "$capture"
exit "$status"
SHIM
chmod +x "$INVENTORY_RACE_BIN/python3"
for inventory_race_operation in replace symlink content intermediate; do
  if [[ -L "$R/scripts" ]]; then
    rm "$R/scripts"
    mv "$R/scripts-original" "$R/scripts"
  fi
  rm -f "$R/scripts/inv" "$R/scripts/inv.replacement" "$INVENTORY_RACE_MARKER"
  cat >"$R/scripts/inv" <<'EOF'
#!/bin/sh
cat <<'JSON'
{"contractVersion":"bubbles-test-inventory/v1","tests":[
 {"id":"t1","file":"tests/unit.spec.ts","title":"computes a total","category":"e2e-ui","runner":"playwright","tags":[]}
]}
JSON
EOF
  chmod +x "$R/scripts/inv"
  set +e
  OUT="$(PATH="$INVENTORY_RACE_BIN:$PATH" INVENTORY_RACE_REAL_PYTHON="$REAL_PYTHON3" \
    INVENTORY_RACE_TARGET="$R/scripts/inv" INVENTORY_RACE_MARKER="$INVENTORY_RACE_MARKER" \
    INVENTORY_RACE_OPERATION="$inventory_race_operation" \
    bash "$TARGET" "$R/specs/001-x" --repo-root "$R" 2>&1)"
  RC=$?
  set -e
  if [[ "$RC" -eq 3 ]] && printf '%s' "$OUT" | grep -q 'CATEGORY-UNRESOLVED' &&
    [[ ! -e "$INVENTORY_RACE_MARKER" ]]; then
    ok "A6-${inventory_race_operation} inventory executable $inventory_race_operation after approval is refused and never executed"
  else
    bad "A6-${inventory_race_operation} inventory executable race refusal" "rc=$RC marker=$([[ -e "$INVENTORY_RACE_MARKER" ]] && printf yes || printf no) out=$(printf '%s' "$OUT" | tr '\n' '|')"
  fi
done

if [[ -L "$R/scripts" ]]; then
  rm "$R/scripts"
  mv "$R/scripts-original" "$R/scripts"
fi

rm -f "$R/scripts/inv" "$R/scripts/inv.replacement"
cat >"$R/scripts/inv" <<'EOF'
#!/bin/sh
cat <<'JSON'
{"contractVersion":"bubbles-test-inventory/v1","tests":[
 {"id":"t1","file":"tests/unit.spec.ts","title":"computes a total","category":"e2e-ui","runner":"playwright","tags":[]}
]}
JSON
EOF
chmod +x "$R/scripts/inv"

# --- P8b. the accepting path also holds with no `timeout` on PATH -----------
# Non-vacuity guard for A6b: proving the no-timeout path REFUSES is worthless if
# the no-timeout path refuses everything. This pins that it still accepts.
P8B_PYTHON_MARKER="$WORK/.p8b-python-invoked"
run_resolve_without_timeout "$R" "$P8B_PYTHON_MARKER"
assert_no_timeout_python_invoked "P8b" "$P8B_PYTHON_MARKER"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'via inventory'; then
  ok "P8b a correct category is still accepted with no \`timeout\`/\`gtimeout\` on PATH"
else
  bad "P8b category match accepted without timeout" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A7. ADVERSARIAL: a failing inventory must not read as "no tests" -------
# Two invariants, and PD-04 changed only the second:
#   (1) title resolution still falls back to the literal scan, so a title that
#       IS in the file must NOT be reported MISSING-TITLE. Unchanged.
#   (2) the CATEGORY verdict is now UNKNOWN, not clean. A declared adapter that
#       cannot run leaves an applicable category unmeasured, and an unmeasured
#       category exits 3. Reporting OK here was the PD-04 false-PASS.
cat >"$R/scripts/inv" <<'EOF'
#!/bin/sh
exit 3
EOF
chmod +x "$R/scripts/inv"
cat >"$R/specs/001-x/scenario-manifest.json" <<'EOF'
{"schemaVersion":1,"scenarios":[{"id":"SCN-001-011","requiredTestType":"e2e-ui","linkedTests":["tests/unit.spec.ts#computes a total"]}]}
EOF
run_resolve "$R"
if [[ "$RC" -eq 3 ]] &&
  printf '%s' "$OUT" | grep -q 'CATEGORY-UNRESOLVED' &&
  printf '%s' "$OUT" | grep -q 'falling back to literal scan' &&
  ! printf '%s' "$OUT" | grep -q 'MISSING-TITLE'; then
  ok "A7 a failing inventory falls back to the scan for titles and refuses on 3 for the category"
else
  bad "A7 inventory failure fallback" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A7b. ADVERSARIAL: an inventory with NO category is equally unknown -----
# The adapter runs, the title resolves, and the runner declares no category.
# The comparison is applicable and unperformed. Before PD-04 this exited 0.
cat >"$R/scripts/inv" <<'EOF'
#!/bin/sh
cat <<'JSON'
{"contractVersion":"bubbles-test-inventory/v1","tests":[
 {"id":"t1","file":"tests/unit.spec.ts","title":"computes a total","category":"","runner":"jest","tags":[]}
]}
JSON
EOF
chmod +x "$R/scripts/inv"
run_resolve "$R"
if [[ "$RC" -eq 3 ]] && printf '%s' "$OUT" | grep -q 'CATEGORY-UNRESOLVED'; then
  ok "A7b an inventory that declares no category is UNKNOWN, not clean"
else
  bad "A7b uncategorised inventory refused" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A7h-A7k. duplicate inventory JSON members are never accepted ----------
for duplicate_member in tests file title category; do
  case "$duplicate_member" in
    tests)
      inventory_json='{"contractVersion":"bubbles-test-inventory/v1","tests":[],"tests":[{"id":"t1","file":"tests/unit.spec.ts","title":"computes a total","category":"e2e-ui"}]}'
      ;;
    file)
      inventory_json='{"contractVersion":"bubbles-test-inventory/v1","tests":[{"id":"t1","file":"tests/other.spec.ts","file":"tests/unit.spec.ts","title":"computes a total","category":"e2e-ui"}]}'
      ;;
    title)
      inventory_json='{"contractVersion":"bubbles-test-inventory/v1","tests":[{"id":"t1","file":"tests/unit.spec.ts","title":"other","title":"computes a total","category":"e2e-ui"}]}'
      ;;
    category)
      inventory_json='{"contractVersion":"bubbles-test-inventory/v1","tests":[{"id":"t1","file":"tests/unit.spec.ts","title":"computes a total","category":"unit","category":"e2e-ui"}]}'
      ;;
  esac
  cat >"$R/scripts/inv" <<EOF
#!/bin/sh
printf '%s\\n' '$inventory_json'
EOF
  chmod +x "$R/scripts/inv"
  run_resolve "$R"
  if [[ "$RC" -eq 3 ]] && printf '%s' "$OUT" | grep -q 'duplicate JSON member'; then
    ok "A7-duplicate-$duplicate_member duplicate inventory member is refused"
  else
    bad "A7-duplicate-$duplicate_member inventory member refusal" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
  fi
done

# --- A7c-A7g. ADVERSARIAL: resolver output is transactional ----------------
# These cases exercise scenario-test-resolve.sh as the consumer. The resolver
# deliberately prints some fields before rejecting several invalid contracts;
# consuming those partial fields used to turn malformed configuration into a
# valid adapter, or even into the fail-open `none` path.
assert_category_unresolved() {
  local label="$1"
  if [[ "$RC" -eq 3 ]] &&
    printf '%s' "$OUT" | grep -q 'CATEGORY-UNRESOLVED' &&
    ! printf '%s' "$OUT" | grep -q 'Traceback\|category comparison(s) not applicable\|\[scenario-test-resolve\] OK'; then
    ok "$label"
  else
    bad "$label" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
  fi
}

cat >"$R/.github/bubbles-project.yaml" <<'EOF'
testDiscovery:
  adapter: invalid-adapter
EOF
run_resolve "$R"
assert_category_unresolved "A7c an invalid configured adapter is category-unresolved, never not-applicable"

cat >"$R/.github/bubbles-project.yaml" <<'EOF'
testDiscovery:
  adapter: command
EOF
run_resolve "$R"
assert_category_unresolved "A7d a configured command adapter with no command is category-unresolved"

cat >"$R/.github/bubbles-project.yaml" <<'EOF'
testDiscovery:
  adapter: command
  command: ../outside-inventory
EOF
run_resolve "$R"
assert_category_unresolved "A7e an unsafe command path is category-unresolved"

cat >"$R/.github/bubbles-project.yaml" <<'EOF'
testDiscovery:
  adapter: command
  command: scripts/inv
  timeoutSeconds: twelve
EOF
cat >"$R/scripts/inv" <<'EOF'
#!/bin/sh
cat <<'JSON'
{"contractVersion":"bubbles-test-inventory/v1","tests":[
 {"id":"t1","file":"tests/unit.spec.ts","title":"computes a total","category":"e2e-ui","runner":"playwright","tags":[]}
]}
JSON
EOF
chmod +x "$R/scripts/inv"
run_resolve "$R"
# portable-ok:diagnostic label describes malformed timeout configuration; it executes no command
assert_category_unresolved "A7f malformed timeout configuration cannot execute from partial resolver output"

cat >"$R/.github/bubbles-project.yaml" <<'EOF'
testDiscovery:
  adapter: command
  command: scripts/inv
  timeoutSeconds: 10
EOF
cat >"$R/scripts/inv" <<'EOF'
#!/bin/sh
cat <<'JSON'
{"contractVersion":"bubbles-test-inventory/v1","tests":[
 {"id":"t1","file":"tests/unit.spec.ts","title":"computes a total","category":"e2e-ui","runner":"playwright","tags":[]}
]}
JSON
exit 9
EOF
chmod +x "$R/scripts/inv"
run_resolve "$R"
assert_category_unresolved "A7g partial inventory output followed by failure is discarded and category-unresolved"

# --- P10. no requiredTestType means nothing is applicable to be unknown ----
# Bounds A7/A7b. A broken adapter must not become a universal blocker: with no
# declared requiredTestType there is no applicable comparison, so exit 0 stays
# correct. Without this, PD-04's refusal would be a false-block machine.
cat >"$R/scripts/inv" <<'EOF'
#!/bin/sh
exit 3
EOF
chmod +x "$R/scripts/inv"
cat >"$R/specs/001-x/scenario-manifest.json" <<'EOF'
{"schemaVersion":1,"scenarios":[{"id":"SCN-001-012","linkedTests":["tests/unit.spec.ts#computes a total"]}]}
EOF
run_resolve "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P10 a failing adapter with no requiredTestType is not an applicable category"
else
  bad "P10 no requiredTestType tolerated" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P11. adapter 'none' is NOT APPLICABLE, and says so ---------------------
# The declared default. A repository that never asked for category enforcement
# must not be refused, and the message must not read as a skipped check.
R="$(make_case p11 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-013","requiredTestType":"e2e-ui","linkedTests":["tests/demo.spec.ts#visible outcome renders"]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'not applicable'; then
  ok "P11 no declared adapter reports category comparison as not applicable"
else
  bad "P11 adapter none not applicable" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P12/P13. bounded JS/TS parser supports multiline declarations ---------
R="$(make_case p12 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-030","linkedTests":["tests/demo.spec.ts#multiline test title"]}]}')"
cat >"$R/tests/demo.spec.ts" <<'EOF'
describe(
  "multiline suite title",
  () => {
    test(
      "multiline test title",
      () => {});
  });
EOF
run_resolve "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P12 multiline test declarations resolve structurally"
else
  bad "P12 multiline test declaration" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

R="$(make_case p13 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-031","linkedTests":["tests/demo.spec.ts#multiline suite title"]}]}')"
cat >"$R/tests/demo.spec.ts" <<'EOF'
describe(
  "multiline suite title",
  () => {
    test(
      "multiline test title",
      () => {});
  });
EOF
run_resolve "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P13 multiline describe declarations resolve structurally"
else
  bad "P13 multiline describe declaration" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P14-P17. bounded structural-title fallback grammar --------------------
R="$(make_case p14 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-040","linkedTests":["tests/demo.spec.ts#parameterized literal title"]}]}')"
cat >"$R/tests/demo.spec.ts" <<'EOF'
test.each([{value: 1}, {value: 2}])("parameterized literal title", ({value}) => {});
EOF
run_resolve "$R"
[[ "$RC" -eq 0 ]] && ok "P14 test.each(cases)(literal title) resolves" || bad "P14 test.each literal title" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"

R="$(make_case p15 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-041","linkedTests":["tests/demo.spec.ts#deno literal title"]}]}')"
printf '%s\n' 'Deno.test("deno literal title", () => {});' >"$R/tests/demo.spec.ts"
run_resolve "$R"
[[ "$RC" -eq 0 ]] && ok "P15 Deno.test(literal title) resolves" || bad "P15 Deno literal title" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"

R="$(make_case p16 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-042","linkedTests":["tests/demo.py#test_decorated_behavior"]}]}')"
mv "$R/tests/demo.spec.ts" "$R/tests/demo.py"
cat >"$R/tests/demo.py" <<'EOF'
@pytest.mark.integration
@custom_decorator
async def test_decorated_behavior():
    pass
EOF
run_resolve "$R"
[[ "$RC" -eq 0 ]] && ok "P16 decorated Python test function resolves" || bad "P16 decorated Python title" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"

R="$(make_case p17 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-043","linkedTests":["tests/demo.rs#tokio_async_behavior"]}]}')"
mv "$R/tests/demo.spec.ts" "$R/tests/demo.rs"
cat >"$R/tests/demo.rs" <<'EOF'
#[tokio::test(flavor = "multi_thread")]
#[cfg(target_os = "linux")]
async fn tokio_async_behavior() {}
EOF
run_resolve "$R"
[[ "$RC" -eq 0 ]] && ok "P17 attribute-bearing tokio test function resolves" || bad "P17 tokio attribute title" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"

# --- A12. unsupported dynamic titles require an inventory ------------------
R="$(make_case a12 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-044","linkedTests":["tests/demo.spec.ts#dynamic title"]}]}')"
cat >"$R/tests/demo.spec.ts" <<'EOF'
const title = "dynamic title";
test(title, () => {});
EOF
run_resolve "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'MISSING-TITLE'; then
  ok "A12 dynamic titles require an inventory and cannot certify via fallback"
else
  bad "A12 dynamic title rejection" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A9-A11. comments/docs/fixture strings cannot certify a title -----------
for false_form in comment documentation fixture; do
  R="$(make_case "a9-$false_form" "{\"schemaVersion\":1,\"scenarios\":[{\"id\":\"SCN-001-032\",\"linkedTests\":[\"tests/demo.spec.ts#phantom certified title\"]}]}")"
  case "$false_form" in
    comment) printf '%s\n' '// test("phantom certified title", () => {});' >"$R/tests/demo.spec.ts" ;;
    documentation) printf '%s\n' 'const docs = "test(phantom certified title)";' >"$R/tests/demo.spec.ts" ;;
    fixture) printf '%s\n' 'const fixture = '\''test("phantom certified title", () => {});'\'';' >"$R/tests/demo.spec.ts" ;;
  esac
  run_resolve "$R"
  if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'MISSING-TITLE'; then
    ok "A9-$false_form non-declaration text cannot certify a test title"
  else
    bad "A9-$false_form false-positive rejection" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
  fi
done

# --- P9. the legacy bare-list manifest envelope -----------------------------
# Real downstream corpora carry manifests whose top level is the scenario ARRAY
# itself rather than {"scenarios": [...]}. Reading .get() straight off that list
# raised AttributeError and took the whole resolver down, which reads to an
# operator as "the guard is broken", not "your packet is old".
R="$(make_case p8 '[{"id":"SCN-001-020","requiredTestType":"e2e-ui","linkedTests":["tests/demo.spec.ts#visible outcome renders"]}]')"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && ! printf '%s' "$OUT" | grep -q 'Traceback'; then
  ok "P9 a bare-list manifest envelope is read, not crashed on"
else
  bad "P9 bare-list envelope" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A8. ADVERSARIAL: the bare-list envelope is still RESOLVED --------------
# Non-vacuity guard for P9. Swallowing the list into an empty scenario set would
# also make P8 pass, and would silently exempt every legacy packet from the very
# check this script exists to perform. A8 fails unless the references inside a
# bare-list manifest are actually resolved.
R="$(make_case a8 '[{"id":"SCN-001-021","requiredTestType":"e2e-ui","linkedTests":["tests/absent.spec.ts"]}]')"
run_resolve "$R"
if [[ "$RC" -eq 2 ]] && printf '%s' "$OUT" | grep -q 'authored path is not an existing stable regular file' &&
  printf '%s' "$OUT" | grep -q 'SCN-001-021'; then
  ok "A8 a bare-list manifest is still resolved, not silently skipped"
else
  bad "A8 bare-list still resolved" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- U1. usage errors -------------------------------------------------------
set +e
bash "$TARGET" >/dev/null 2>&1; u1=$?
bash "$TARGET" "$WORK/absent-spec" >/dev/null 2>&1; u2=$?
bypass="$(bash "$TARGET" --skip-resolution 2>&1)"; u3=$?
set -e
if [[ "$u1" -eq 2 && "$u2" -eq 2 && "$u3" -eq 2 ]] &&
  printf '%s' "$bypass" | grep -q 'bypass-shaped'; then
  ok "U1 missing arg, absent dir and a bypass flag all exit 2"
else
  bad "U1 usage errors" "noarg=$u1 absent=$u2 bypass=$u3"
fi

# --- S1-S4. owned temporary-directory cleanup identity and signals ----------
run_cleanup_probe() {
  local operation="$1"
  local expected_rc="$2"
  local root="$WORK/cleanup-$operation"
  local parent="$WORK/cleanup-parent-$operation"
  local marker="$WORK/cleanup-marker-$operation"
  local target="$root/tests/demo.spec.ts"
  local shim="$WORK/cleanup-bin-$operation"
  mkdir -p "$root/specs/001-x" "$root/tests" "$parent" "$shim"
  git -C "$root" init -q . 2>/dev/null || true
  printf '%s\n' 'test("cleanup title", () => {});' >"$target"
  printf '%s\n' '{"schemaVersion":1,"scenarios":[{"id":"SCN-CLEANUP-001","linkedTests":["tests/demo.spec.ts#cleanup title"]}]}' >"$root/specs/001-x/scenario-manifest.json"
  cat >"$shim/python3" <<'SHIM'
#!/usr/bin/env bash
set -u
: "${CLEANUP_REAL_PYTHON:?}" "${CLEANUP_OPERATION:?}" "${CLEANUP_MARKER:?}" "${CLEANUP_PARENT:?}"
if [[ "${1:-}" == */scenario-reference-reader.py ]]; then
  work_dir="${BUBBLES_SCENARIO_TEST_RESOLVE_WORK_DIR:?missing resolver work directory}"
  printf '%s\n' "$work_dir" >"$CLEANUP_MARKER"
  case "$CLEANUP_OPERATION" in
    replace) rm -rf "$work_dir"; mkdir "$work_dir"; printf 'replacement\n' >"$work_dir/preserve" ;;
    symlink) rm -rf "$work_dir"; mkdir "$CLEANUP_PARENT/safe"; printf 'parent\n' >"$CLEANUP_PARENT/safe/preserve"; ln -s "$CLEANUP_PARENT/safe" "$work_dir" ;;
    int) kill -INT "$PPID"; sleep 1 ;;
    term) kill -TERM "$PPID"; sleep 1 ;;
  esac
fi
exec "$CLEANUP_REAL_PYTHON" "$@"
SHIM
  chmod +x "$shim/python3"
  set +e
  PATH="$shim:$PATH" CLEANUP_REAL_PYTHON="$REAL_PYTHON3" CLEANUP_OPERATION="$operation" CLEANUP_MARKER="$marker" CLEANUP_PARENT="$parent" \
    bash "$TARGET" "$root/specs/001-x" --repo-root "$root" >/dev/null 2>&1
  local observed_rc=$?
  set -e
  local work_dir=""
  [[ -f "$marker" ]] && work_dir="$(cat "$marker")"
  if [[ "$observed_rc" -ne "$expected_rc" ]]; then
    bad "cleanup $operation exits $expected_rc" "observed=$observed_rc"
  elif [[ "$operation" == "replace" && -f "$work_dir/preserve" ]]; then
    ok "cleanup replacement identity mismatch preserves replacement directory"
  elif [[ "$operation" == "symlink" && -L "$work_dir" && -f "$parent/safe/preserve" ]]; then
    ok "cleanup symlink identity mismatch preserves symlink target parent"
  elif [[ "$operation" == "int" || "$operation" == "term" ]]; then
    [[ -n "$work_dir" && ! -e "$work_dir" ]] && ok "cleanup $operation removes owned directory with signal-specific exit" || bad "cleanup $operation owned directory removal"
  else
    bad "cleanup $operation preservation assertion"
  fi
}
run_cleanup_probe replace 2
run_cleanup_probe symlink 2
run_cleanup_probe int 130
run_cleanup_probe term 143

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
