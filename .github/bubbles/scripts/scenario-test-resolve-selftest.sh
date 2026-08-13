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
R="$(make_case p6 '{"version":1,"scenarios":[{"scenarioId":"SCN-001-009","requiredTestType":"e2e-ui","linkedTests":["tests/demo.spec.ts#nope not here"]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'SCN-001-009'; then
  ok "P6 the scenarioId spelling is read and named in the finding"
else
  bad "P6 scenarioId spelling" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P7. planning sentinel is skipped, not treated as a missing file --------
R="$(make_case p7 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-001","requiredTestType":"e2e-ui","linkedTests":["__FUTURE_TEST__"]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 0 ]]; then
  ok "P7 a __FUTURE_TEST__ sentinel does not fail a planning packet"
else
  bad "P7 sentinel skipped" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
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
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'MISSING-FILE'; then
  ok "A3 a reference to a non-existent file is refused"
else
  bad "A3 missing file" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A4. ADVERSARIAL: path escaping the repository --------------------------
R="$(make_case a4 '{"schemaVersion":1,"scenarios":[{"id":"SCN-001-004","requiredTestType":"e2e-ui","linkedTests":["../../etc/passwd"]}]}')"
run_resolve "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'OUTSIDE-REPO'; then
  ok "A4 a path escaping the repository root is refused"
else
  bad "A4 outside repo" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A5. the finding names the scenario, the reference, and the reason ------
R="$(make_case a5 '{"schemaVersion":1,"scenarios":[{"id":"SCN-042-007","requiredTestType":"e2e-ui","linkedTests":["tests/demo.spec.ts#absent one"]}]}')"
run_resolve "$R"
if printf '%s' "$OUT" | grep -q 'SCN-042-007' &&
  printf '%s' "$OUT" | grep -q 'tests/demo.spec.ts#absent one' &&
  printf '%s' "$OUT" | grep -q 'no test with this exact title'; then
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

# --- A7. ADVERSARIAL: a failing inventory must not read as "no tests" -------
# An empty inventory would fail EVERY declared title. Falling back to the scan
# keeps a broken adapter from manufacturing findings.
cat >"$R/scripts/inv" <<'EOF'
#!/bin/sh
exit 3
EOF
chmod +x "$R/scripts/inv"
cat >"$R/specs/001-x/scenario-manifest.json" <<'EOF'
{"schemaVersion":1,"scenarios":[{"id":"SCN-001-011","requiredTestType":"e2e-ui","linkedTests":["tests/unit.spec.ts#computes a total"]}]}
EOF
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -q 'falling back to literal scan'; then
  ok "A7 a failing inventory falls back to the scan, not to 'no tests exist'"
else
  bad "A7 inventory failure fallback" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
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

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
