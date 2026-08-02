#!/usr/bin/env bash
# surface-reachability-guard-selftest.sh — hermetic cases for IMP-031 SCOPE-3.
#
# Every fixture is built under a mktemp -d workspace: a fake consumer repo with
# a .github/bubbles-project.yaml, executable derive commands, and specs carrying
# Exposure Contract tables. Nothing here reads or writes the real repository.
#
# Cases:
#   S1  no bubbles-project.yaml                    -> exit 0, "not applicable"
#   S2  config present but no surfaces: block      -> exit 0, "not applicable"
#   S3  Bubbles framework source layout            -> exit 0, "EXEMPT"
#   S4  every derived surface is declared          -> exit 0, clean reconciliation
#   S5  a derived surface no spec declares         -> exit 0, ORPHANED SURFACES
#   S6  a 'delivered' claim absent from source     -> exit 0, UNDELIVERED CLAIMS
#   S7  derive command returns ZERO records        -> exit 1, integrity failure
#   S8  derive command exits non-zero              -> exit 1, integrity failure
#   S9  derive command does not exist              -> exit 1, integrity failure
#   S10 class declares no derive command           -> exit 1, integrity failure
#   S11 derive mislabels its own class             -> exit 1, integrity failure
#   S12 expanded (non-inline) derive form parses   -> exit 0, clean reconciliation
#   S13 Exposure Contract inside a ``` fence is    -> exit 0, ORPHANED SURFACES
#       NOT read as a declaration (detector precision)
#   S14 comment and blank lines in derive output   -> exit 0, clean reconciliation
#       are ignored, not counted as surfaces
#   S15 codeIndex route with adapter=none          -> exit 1, integrity failure
#   S16 no --skip/--force/--ignore bypass exists   -> flags rejected

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GUARD="$SCRIPT_DIR/surface-reachability-guard.sh"
FAILURES=0

pass() { echo "PASS: $1"; }
fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/surface-reachability-selftest.XXXXXX")"
trap 'rm -rf "$TMP_ROOT"' EXIT

# new_repo <name> — a bare consumer repo skeleton; echoes its path.
new_repo() {
  local d="$TMP_ROOT/$1"
  mkdir -p "$d/.github" "$d/specs/010-example"
  printf '%s\n' "$d"
}

# write_config <repo> <body>
write_config() {
  printf '%s\n' "$2" > "$1/.github/bubbles-project.yaml"
}

# write_derive <repo> <relpath> <stdout-body>
write_derive() {
  local repo="$1" rel="$2" body="$3"
  mkdir -p "$repo/$(dirname "$rel")"
  {
    echo '#!/usr/bin/env bash'
    echo "cat <<'RECORDS'"
    printf '%s\n' "$body"
    echo 'RECORDS'
  } > "$repo/$rel"
  chmod +x "$repo/$rel"
}

# write_spec <repo> <rows>
write_spec() {
  local repo="$1" rows="$2"
  {
    echo '# Spec: 010 Example'
    echo ''
    echo '## Exposure Contract'
    echo ''
    echo '| Capability | Surface class | Surface id | Status | Plan |'
    echo '|---|---|---|---|---|'
    printf '%s\n' "$rows"
    echo ''
    echo '## Notes'
  } > "$repo/specs/010-example/spec.md"
}

STD_CONFIG='surfaces:
  schemaVersion: 1
  classes:
    httpRoute:  { derive: "inv/http.sh" }'

run_guard() {
  local out rc
  out="$("$GUARD" --repo-root "$1" 2>&1)" && rc=0 || rc=$?
  GUARD_OUT="$out"
  GUARD_RC="$rc"
}

# --- S1 -------------------------------------------------------------------
echo "--- S1: no bubbles-project.yaml is a clean no-op ---"
d="$(new_repo s1)"
run_guard "$d"
if [[ "$GUARD_RC" -eq 0 ]] && grep -q 'not applicable' <<< "$GUARD_OUT"; then
  pass "S1 no config -> not applicable, exit 0"
else
  fail "S1 expected exit 0 + 'not applicable', got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S2 -------------------------------------------------------------------
echo "--- S2: config without a surfaces: block is a clean no-op ---"
d="$(new_repo s2)"
write_config "$d" 'domainModel:
  entities:
    Order:
      states: [created, paid]'
run_guard "$d"
if [[ "$GUARD_RC" -eq 0 ]] && grep -q 'no surfaces: block' <<< "$GUARD_OUT"; then
  pass "S2 no surfaces block -> not applicable, exit 0"
else
  fail "S2 expected exit 0 + 'no surfaces: block', got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S3 -------------------------------------------------------------------
echo "--- S3: the framework source checkout is EXEMPT ---"
d="$(new_repo s3)"
mkdir -p "$d/bubbles/scripts" "$d/agents/bubbles_shared"
touch "$d/VERSION" "$d/install.sh"
write_config "$d" "$STD_CONFIG"
run_guard "$d"
if [[ "$GUARD_RC" -eq 0 ]] && grep -q 'EXEMPT' <<< "$GUARD_OUT"; then
  pass "S3 framework source -> EXEMPT, exit 0"
else
  fail "S3 expected exit 0 + EXEMPT, got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S4 -------------------------------------------------------------------
echo "--- S4: fully reconciled inventory reports clean ---"
d="$(new_repo s4)"
write_config "$d" "$STD_CONFIG"
write_derive "$d" inv/http.sh 'httpRoute	POST /api/v1/signals/emit	/api/v1/signals/emit	src/routes.rs'
write_spec "$d" '| signal-emit | httpRoute | POST /api/v1/signals/emit | delivered | — |'
run_guard "$d"
if [[ "$GUARD_RC" -eq 0 ]] && grep -q 'every derived surface is declared' <<< "$GUARD_OUT"; then
  pass "S4 reconciled -> clean, exit 0"
else
  fail "S4 expected clean reconciliation, got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S5 -------------------------------------------------------------------
echo "--- S5: a surface no spec declares is reported as an orphan ---"
d="$(new_repo s5)"
write_config "$d" "$STD_CONFIG"
write_derive "$d" inv/http.sh 'httpRoute	POST /api/v1/signals/emit	/api/v1/signals/emit	src/routes.rs
httpRoute	GET /api/v1/internal/dump	/api/v1/internal/dump	src/routes.rs'
write_spec "$d" '| signal-emit | httpRoute | POST /api/v1/signals/emit | delivered | — |'
run_guard "$d"
if [[ "$GUARD_RC" -eq 0 ]] \
  && grep -q 'ORPHANED SURFACES' <<< "$GUARD_OUT" \
  && grep -q '/api/v1/internal/dump' <<< "$GUARD_OUT"; then
  pass "S5 undeclared surface -> ORPHANED SURFACES, exit 0 (report-only)"
else
  fail "S5 expected orphan report at exit 0, got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S6 -------------------------------------------------------------------
echo "--- S6: a delivered claim with no matching surface is reported ---"
d="$(new_repo s6)"
write_config "$d" "$STD_CONFIG"
write_derive "$d" inv/http.sh 'httpRoute	POST /api/v1/signals/emit	/api/v1/signals/emit	src/routes.rs'
write_spec "$d" '| signal-emit | httpRoute | POST /api/v1/signals/emit | delivered | — |
| ghost-cap | httpRoute | POST /api/v1/ghost | delivered | — |'
run_guard "$d"
if [[ "$GUARD_RC" -eq 0 ]] \
  && grep -q 'UNDELIVERED CLAIMS' <<< "$GUARD_OUT" \
  && grep -q '/api/v1/ghost' <<< "$GUARD_OUT"; then
  pass "S6 phantom delivery -> UNDELIVERED CLAIMS, exit 0 (report-only)"
else
  fail "S6 expected undelivered report at exit 0, got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S7 -------------------------------------------------------------------
echo "--- S7: a derive command returning nothing is a HARD failure ---"
d="$(new_repo s7)"
write_config "$d" "$STD_CONFIG"
write_derive "$d" inv/http.sh ''
write_spec "$d" '| signal-emit | httpRoute | POST /api/v1/signals/emit | delivered | — |'
run_guard "$d"
if [[ "$GUARD_RC" -eq 1 ]] \
  && grep -q 'DERIVATION INTEGRITY FAILURE' <<< "$GUARD_OUT" \
  && grep -q 'ZERO surfaces' <<< "$GUARD_OUT"; then
  pass "S7 empty denominator -> exit 1, never a silent pass"
else
  fail "S7 expected exit 1 on empty derivation, got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S8 -------------------------------------------------------------------
echo "--- S8: a derive command that errors is a HARD failure ---"
d="$(new_repo s8)"
write_config "$d" "$STD_CONFIG"
mkdir -p "$d/inv"
printf '%s\n' '#!/usr/bin/env bash' 'echo "boom" >&2' 'exit 3' > "$d/inv/http.sh"
chmod +x "$d/inv/http.sh"
run_guard "$d"
if [[ "$GUARD_RC" -eq 1 ]] && grep -q 'derive command failed' <<< "$GUARD_OUT"; then
  pass "S8 failing derivation -> exit 1"
else
  fail "S8 expected exit 1 on failing derivation, got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S9 -------------------------------------------------------------------
echo "--- S9: a missing derive command is a HARD failure ---"
d="$(new_repo s9)"
write_config "$d" "$STD_CONFIG"
run_guard "$d"
if [[ "$GUARD_RC" -eq 1 ]] && grep -q 'derive command failed' <<< "$GUARD_OUT"; then
  pass "S9 missing derivation script -> exit 1"
else
  fail "S9 expected exit 1 on missing derivation, got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S10 ------------------------------------------------------------------
echo "--- S10: a class with no derive command is a HARD failure ---"
d="$(new_repo s10)"
write_config "$d" 'surfaces:
  schemaVersion: 1
  classes:
    httpRoute:'
run_guard "$d"
if [[ "$GUARD_RC" -eq 1 ]] && grep -q 'declares no derive command' <<< "$GUARD_OUT"; then
  pass "S10 class without derive -> exit 1"
else
  fail "S10 expected exit 1 on class without derive, got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S11 ------------------------------------------------------------------
echo "--- S11: a derivation that mislabels its class is a HARD failure ---"
d="$(new_repo s11)"
write_config "$d" "$STD_CONFIG"
write_derive "$d" inv/http.sh 'cliCommand	deploy	deploy	src/cli.rs'
run_guard "$d"
if [[ "$GUARD_RC" -eq 1 ]] && grep -q 'mislabels its own output' <<< "$GUARD_OUT"; then
  pass "S11 mislabelled records -> exit 1"
else
  fail "S11 expected exit 1 on mislabelled records, got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S12 ------------------------------------------------------------------
echo "--- S12: the expanded derive form parses identically ---"
d="$(new_repo s12)"
write_config "$d" 'surfaces:
  schemaVersion: 1
  classes:
    httpRoute:
      derive: inv/http.sh'
write_derive "$d" inv/http.sh 'httpRoute	POST /api/v1/signals/emit	/api/v1/signals/emit	src/routes.rs'
write_spec "$d" '| signal-emit | httpRoute | POST /api/v1/signals/emit | delivered | — |'
run_guard "$d"
if [[ "$GUARD_RC" -eq 0 ]] && grep -q 'every derived surface is declared' <<< "$GUARD_OUT"; then
  pass "S12 expanded derive form -> clean, exit 0"
else
  fail "S12 expected clean reconciliation, got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S13 ------------------------------------------------------------------
echo "--- S13: an Exposure Contract inside a code fence is an example, not a declaration ---"
d="$(new_repo s13)"
write_config "$d" "$STD_CONFIG"
write_derive "$d" inv/http.sh 'httpRoute	POST /api/v1/signals/emit	/api/v1/signals/emit	src/routes.rs'
{
  echo '# Spec: 010 Example'
  echo ''
  echo '## Exposure Contract'
  echo ''
  echo 'The table below is copied from the template for illustration:'
  echo ''
  echo '```markdown'
  echo '| Capability | Surface class | Surface id | Status | Plan |'
  echo '|---|---|---|---|---|'
  echo '| signal-emit | httpRoute | POST /api/v1/signals/emit | delivered | — |'
  echo '```'
} > "$d/specs/010-example/spec.md"
run_guard "$d"
if [[ "$GUARD_RC" -eq 0 ]] \
  && grep -q 'ORPHANED SURFACES' <<< "$GUARD_OUT" \
  && grep -q 'declared exposures: 0' <<< "$GUARD_OUT"; then
  pass "S13 fenced example is not counted as a declaration"
else
  fail "S13 expected the fenced table to be ignored, got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S14 ------------------------------------------------------------------
echo "--- S14: comments and blank lines in derive output are not surfaces ---"
d="$(new_repo s14)"
write_config "$d" "$STD_CONFIG"
write_derive "$d" inv/http.sh '# generated by scripts/inventory/http-routes.sh

httpRoute	POST /api/v1/signals/emit	/api/v1/signals/emit	src/routes.rs
'
write_spec "$d" '| signal-emit | httpRoute | POST /api/v1/signals/emit | delivered | — |'
run_guard "$d"
if [[ "$GUARD_RC" -eq 0 ]] \
  && grep -q 'derived surfaces:   1' <<< "$GUARD_OUT" \
  && grep -q 'every derived surface is declared' <<< "$GUARD_OUT"; then
  pass "S14 comments/blank lines ignored -> 1 surface, clean"
else
  fail "S14 expected exactly 1 derived surface, got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S15 ------------------------------------------------------------------
echo "--- S15: routing a class to an unconfigured code index is a HARD failure ---"
d="$(new_repo s15)"
write_config "$d" 'codeIndex:
  adapter: none
surfaces:
  schemaVersion: 1
  classes:
    httpRoute:  { derive: "codeIndex" }'
run_guard "$d"
if [[ "$GUARD_RC" -eq 1 ]] && grep -q "adapter='none'" <<< "$GUARD_OUT"; then
  pass "S15 codeIndex -> none is an empty denominator, exit 1"
else
  fail "S15 expected exit 1 for adapter=none, got rc=$GUARD_RC: $GUARD_OUT"
fi

# --- S16 ------------------------------------------------------------------
echo "--- S16: there is no --skip/--force/--ignore bypass ---"
bypass_ok=1
for flag in --skip --force --ignore; do
  out="$("$GUARD" "$flag" 2>&1)" && rc=0 || rc=$?
  if [[ "$rc" -ne 2 ]] || ! grep -q 'unknown argument' <<< "$out"; then
    bypass_ok=0
    echo "  $flag was not rejected (rc=$rc)"
  fi
done
if [[ "$bypass_ok" -eq 1 ]]; then
  pass "S16 bypass flags are rejected as unknown arguments"
else
  fail "S16 a bypass flag was accepted"
fi

echo ""
if [[ "$FAILURES" -eq 0 ]]; then
  echo "surface-reachability-guard-selftest: all cases passed."
  exit 0
fi
echo "surface-reachability-guard-selftest: $FAILURES case(s) failed."
exit 1
