#!/usr/bin/env bash
# bubbles/scripts/test-inventory-adapter-contract-selftest.sh
#
# Hermetic contract selftest for the test-inventory adapter (IMP-040 SCOPE-1).
#
# The load-bearing half is adversarial. A resolver that quietly degrades a
# BROKEN configuration to `none` makes a typo indistinguishable from a
# deliberate opt-out — and since `none` disables title-based certification, that
# silent degrade would turn a misconfiguration into a permanently weaker gate.
# Cases A1-A9 all assert loud refusal rather than degrade.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESOLVE="$SCRIPT_DIR/test-inventory-resolve.sh"
NONE_ADAPTER="$(cd "$SCRIPT_DIR/.." && pwd)/adapters/test-inventory/none.sh"
NAME="test-inventory-adapter-contract-selftest"

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

# $1 root, $2... lines of the testDiscovery block (omit for no block at all)
make_repo() {
  local root="$1"; shift
  mkdir -p "$root/.github"
  {
    printf 'projectName: fixture\n'
    if [[ $# -gt 0 ]]; then
      printf 'testDiscovery:\n'
      local line
      for line in "$@"; do printf '  %s\n' "$line"; done
    fi
    # A trailing sibling key proves block scoping: keys after the block must
    # not leak into it.
    printf 'codeIndex:\n  adapter: none\n'
  } >"$root/.github/bubbles-project.yaml"
}

run_resolve() {
  set +e
  OUT="$(bash "$RESOLVE" --repo-root "$1" 2>&1)"
  RC=$?
  set -e
}

# --- P1. no testDiscovery block at all resolves neutrally --------------------
R="$WORK/p1"; make_repo "$R"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qx 'adapter=none'; then
  ok "P1 absent testDiscovery block resolves to none"
else
  bad "P1 absent block resolves to none" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P2. explicit none ------------------------------------------------------
R="$WORK/p2"; make_repo "$R" 'adapter: none'
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qx 'adapter=none' &&
  printf '%s' "$OUT" | grep -q 'adapterPath=.*/adapters/test-inventory/none.sh'; then
  ok "P2 explicit none resolves to the framework adapter"
else
  bad "P2 explicit none" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P3. command adapter resolves to an absolute path -----------------------
R="$WORK/p3"; make_repo "$R" 'adapter: command' 'command: scripts/inv' 'timeoutSeconds: 45'
mkdir -p "$R/scripts"; printf '#!/bin/sh\necho "{}"\n' >"$R/scripts/inv"; chmod +x "$R/scripts/inv"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qx 'adapter=command' &&
  printf '%s' "$OUT" | grep -qx "command=$R/scripts/inv" &&
  printf '%s' "$OUT" | grep -qx 'timeoutSeconds=45'; then
  ok "P3 command adapter resolves path and timeout"
else
  bad "P3 command adapter" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- P4. timeout defaults when omitted --------------------------------------
R="$WORK/p4"; make_repo "$R" 'adapter: command' 'command: scripts/inv'
mkdir -p "$R/scripts"; printf '#!/bin/sh\necho "{}"\n' >"$R/scripts/inv"; chmod +x "$R/scripts/inv"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qx 'timeoutSeconds=120'; then
  ok "P4 timeoutSeconds defaults to 120"
else
  bad "P4 timeout default" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A1. ADVERSARIAL: command adapter with no command: refuses loudly -------
R="$WORK/a1"; make_repo "$R" 'adapter: command'
run_resolve "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'no command'; then
  ok "A1 adapter=command without command: fails loud (never degrades to none)"
else
  bad "A1 missing command refuses" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A2/A3. ADVERSARIAL: escaping the repository is refused -----------------
for case_id in a2 a3; do
  case "$case_id" in
    a2) cmd='/usr/bin/env'; label="absolute path" ;;
    a3) cmd='../outside/inv'; label="parent traversal" ;;
  esac
  R="$WORK/$case_id"; make_repo "$R" 'adapter: command' "command: $cmd"
  run_resolve "$R"
  if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'repo-relative'; then
    ok "${case_id^^} $label command is refused"
  else
    bad "${case_id^^} $label refused" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
  fi
done

# --- A4. ADVERSARIAL: missing command file ----------------------------------
R="$WORK/a4"; make_repo "$R" 'adapter: command' 'command: scripts/absent'
run_resolve "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'not found'; then
  ok "A4 missing command file is refused"
else
  bad "A4 missing command file" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A5. ADVERSARIAL: non-executable command --------------------------------
R="$WORK/a5"; make_repo "$R" 'adapter: command' 'command: scripts/inv'
mkdir -p "$R/scripts"; printf '#!/bin/sh\n' >"$R/scripts/inv"; chmod 644 "$R/scripts/inv"
run_resolve "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'not executable'; then
  ok "A5 non-executable command is refused"
else
  bad "A5 non-executable command" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A6/A7. ADVERSARIAL: bad adapter tokens ---------------------------------
R="$WORK/a6"; make_repo "$R" 'adapter: Command!'
run_resolve "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'invalid testDiscovery.adapter'; then
  ok "A6 non-token adapter value is refused before any filesystem access"
else
  bad "A6 non-token adapter" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

R="$WORK/a7"; make_repo "$R" 'adapter: runner'
run_resolve "$R"
if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'unknown testDiscovery.adapter'; then
  ok "A7 unknown adapter kind is refused"
else
  bad "A7 unknown adapter kind" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- A8/A9. ADVERSARIAL: bad timeouts ---------------------------------------
for case_id in a8 a9; do
  case "$case_id" in
    a8) tv='soon'; label="non-integer" ;;
    a9) tv='0'; label="zero" ;;
  esac
  R="$WORK/$case_id"; make_repo "$R" 'adapter: command' 'command: scripts/inv' "timeoutSeconds: $tv"
  mkdir -p "$R/scripts"; printf '#!/bin/sh\n' >"$R/scripts/inv"; chmod +x "$R/scripts/inv"
  run_resolve "$R"
  if [[ "$RC" -eq 1 ]] && printf '%s' "$OUT" | grep -q 'timeoutSeconds'; then
    ok "${case_id^^} $label timeoutSeconds is refused"
  else
    bad "${case_id^^} $label timeout" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
  fi
done

# --- N1. block scoping: a sibling key must not leak in ----------------------
# codeIndex.adapter is `none` in every fixture; if block scoping were broken the
# command fixtures would still resolve, so P3 already half-covers this. This
# case pins the opposite direction explicitly.
R="$WORK/n1"; mkdir -p "$R/.github"
{
  printf 'codeIndex:\n  adapter: command\n'
  printf 'testDiscovery:\n  adapter: none\n'
} >"$R/.github/bubbles-project.yaml"
run_resolve "$R"
if [[ "$RC" -eq 0 ]] && printf '%s' "$OUT" | grep -qx 'adapter=none'; then
  ok "N1 a sibling block's adapter does not leak into testDiscovery"
else
  bad "N1 block scoping" "rc=$RC out=$(printf '%s' "$OUT" | tr '\n' '|')"
fi

# --- C1. neutral adapter verb shapes ----------------------------------------
t_out="$(bash "$NONE_ADAPTER" tests 2>&1)"
s_out="$(bash "$NONE_ADAPTER" status 2>&1)"
c_out="$(bash "$NONE_ADAPTER" capabilities 2>&1)"
if [[ "$t_out" == '{}' && "$c_out" == '{}' ]] &&
  printf '%s' "$s_out" | grep -q '"measured":false'; then
  ok "C1 neutral adapter returns the canonical verb shapes"
else
  bad "C1 neutral verb shapes" "tests=$t_out status=$s_out capabilities=$c_out"
fi

# --- C2. ADVERSARIAL: `tests` must NOT be a bare empty array ----------------
# An empty ARRAY is a positive claim that the repo has no tests; a resolver
# reading that would fail every linked reference. The neutral value must be
# distinguishable from a measured-and-empty inventory.
if [[ "$t_out" != '[]' ]]; then
  ok "C2 neutral 'tests' is not an empty array (unmeasured != measured-empty)"
else
  bad "C2 neutral tests must not be []" "got $t_out"
fi

# --- C3. unknown verb is a usage error --------------------------------------
set +e
bash "$NONE_ADAPTER" bogus >/dev/null 2>&1; verb_rc=$?
set -e
if [[ "$verb_rc" -eq 2 ]]; then
  ok "C3 unknown verb exits 2"
else
  bad "C3 unknown verb exits 2" "rc=$verb_rc"
fi

# --- C4. selftest entry point agrees with the direct verbs ------------------
if [[ "$(bash "$NONE_ADAPTER" selftest tests 2>&1)" == "$t_out" ]] &&
  [[ "$(bash "$NONE_ADAPTER" selftest status 2>&1)" == "$s_out" ]]; then
  ok "C4 selftest entry point yields identical neutral shapes"
else
  bad "C4 selftest entry point agreement"
fi

# --- U1. usage errors -------------------------------------------------------
set +e
bash "$RESOLVE" --repo-root "$WORK/nonexistent" >/dev/null 2>&1; u1=$?
bash "$RESOLVE" --bogus-flag >/dev/null 2>&1; u2=$?
set -e
if [[ "$u1" -eq 2 && "$u2" -eq 2 ]]; then
  ok "U1 missing root and unknown flag both exit 2"
else
  bad "U1 usage errors" "missing-root=$u1 unknown-flag=$u2"
fi

printf '%s: %s check(s), %s failure(s)\n' "$NAME" "$checks" "$failures"
[[ "$failures" -eq 0 ]] || exit 1
exit 0
